package plot

import (
	"image"
	"image/color"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"scvirtualcytometer/internal/data"
)

// Bounds describes a viewing window in data space (column units, not
// pixels).
type Bounds struct {
	Xmin, Xmax, Ymin, Ymax float64
}

func (b Bounds) Width() float64  { return b.Xmax - b.Xmin }

func (b Bounds) Height() float64 { return b.Ymax - b.Ymin }

var defaultBackground = color.NRGBA{R: 255, G: 255, B: 255, A: 255}

// stackPalette assigns a distinct color to each successive stacked layer
// (and to each quadrant), cycling once exhausted. This is the Okabe-Ito
// palette — the colorblind-safe categorical set recommended by Nature,
// Science and Cell for exactly this purpose: colors chosen to stay
// mutually distinguishable, including under the common forms of color
// vision deficiency, rather than picked for looking nice on one screen.
// Ordered to start with the reddish-purple tone, echoing the violet used
// for selections in the original JS app.
var stackPalette = []color.NRGBA{
	{R: 204, G: 121, B: 167, A: 255}, // #CC79A7 reddish purple — default single selection
	{R: 0, G: 114, B: 178, A: 255},   // #0072B2 blue
	{R: 213, G: 94, B: 0, A: 255},    // #D55E00 vermillion
	{R: 0, G: 158, B: 115, A: 255},   // #009E73 bluish green
	{R: 230, G: 159, B: 0, A: 255},   // #E69F00 orange
	{R: 86, G: 180, B: 233, A: 255},  // #56B4E9 sky blue
	{R: 240, G: 228, B: 66, A: 255},  // #F0E442 yellow
}

// highlightColor is the color for a plain (non-stacked) selection. It's
// simply the first stack color, not a separate color of its own — that
// way a normal selection and the first layer of a Stack-mode selection
// are, by construction, never visually confusable with each other: they
// ARE the same color, and only diverge from stack layer 2 onward.
var highlightColor = stackPalette[0]

// SelectTool selects what a mouse drag does on a ScatterPlot.
type SelectTool int

const (
	ToolRect  SelectTool = iota // rectangular brush (default)
	ToolLasso                   // freehand lasso
	ToolPan                     // pan the view instead of selecting
)

// dataPoint is a point expressed in data-space coordinates (used to store
// the selection outline so it re-projects correctly across zoom/pan).
type dataPoint struct{ X, Y float64 }

// selShape is the shape of the last selection made *by this plot*
// (rectangle brush or lasso), kept only for the persistent dashed outline.
type selShape struct {
	kind byte // 'r' = rectangle, 'l' = lasso polygon
	rect Bounds
	poly []dataPoint
}

// ScatterPlot is a Fyne widget that displays a point cloud for a dataset
// that may hold several million rows.
//
// Key point (this is what lets us show millions of cells without blowing
// up memory): we NEVER create one fyne.CanvasObject per cell. Points are
// rasterized by hand into an image whose size is the widget's on-screen
// size (a few hundred pixels), regardless of N. Render memory is therefore
// bounded by display resolution, not by dataset size. The (relatively)
// expensive full recompute only happens on interaction (zoom, pan,
// selection, column change, gate toggle, resize) — never in a render loop.
// A second, cheap overlay image is used for the live drag/lasso preview
// and the persistent dashed selection outline, so that dragging the mouse
// never triggers a full point re-render.
//
// Points are drawn as small flat squares rather than anti-aliased circles
// (no per-pixel distance/sqrt test) — this is intentional, it is
// noticeably cheaper at millions of points and the visual difference is
// negligible at typical dot sizes.
type ScatterPlot struct {
	widget.BaseWidget

	ds  *data.Dataset
	sel *data.Selection
	quad *data.QuadrantState // shared with the linked plot so quadrant coloring carries over

	xIdx, yIdx   int
	XName, YName string
	grid         *data.GridIndex

	full Bounds // full extent of the data for the current columns
	view Bounds // currently displayed window (zoom/pan)

	DotRadius  int
	PointColor color.NRGBA // translucent so overlapping cells look denser
	Background color.NRGBA // live plot background; White/Black toggle from the GUI changes this

	// ShowDensity toggles the axis-graduated frame's marginal histograms
	// (density-per-slice bars along the top and right edges, classic
	// flow-cytometry biaxial plot style). The graduated frame and tick
	// labels themselves are always drawn — they're essentially free —
	// only the histograms are optional, since they take up plot area.
	ShowDensity bool

	// ColorBy: color points by a data column instead of the flat
	// PointColor. -1 means "off". See colorby.go for the auto
	// gradient-vs-classes decision and SetColorBy/SetColorByClasses.
	ColorByIdx     int
	ColorByClasses bool   // force per-class colors even for a numeric column
	ColorByGradient string // one of GradientNames (colormap.go); also the source for >7-class categorical colors

	colorByCategorical bool
	colorByMin         float64
	colorByMax         float64
	colorByLevelColor  map[int]color.NRGBA

	// ColorByMinPct/MaxPct clip the gradient to a sub-range of the
	// column's actual [colorByMin, colorByMax], expressed as a percentage
	// of that range (0..100, default 0/100 = no clipping). Cells below
	// the low clip show the gradient's first color, cells above the high
	// clip show its last — this is what lets a handful of abnormally
	// low/high outlier cells stop washing out the color contrast for the
	// bulk of the population. See SetColorByRange.
	ColorByMinPct float64
	ColorByMaxPct float64

	// RenderBudget caps how many points are actually splatted on screen
	// per redraw, regardless of how many fall in the current view. Above
	// this count, points are evenly sub-sampled (a deterministic stride
	// over the candidates) purely for display — selection, gating,
	// quadrant statistics and image export always use the full,
	// un-sampled dataset. Keeps interaction smooth into the millions of
	// cells; lower it further on slower machines, or raise it for
	// tiny/fast datasets where you want every point drawn.
	RenderBudget int

	resizeTimer *time.Timer

	// Gating.
	GateOnly   bool // when true, only currently selected cells are drawn
	FreezeGate bool // when true, a new brush/lasso only refines the current selection (sequential gating)
	StackMode  bool // when true, a new brush/lasso ADDS a new colored layer instead of replacing the selection

	quadrantsOn  bool
	hasQuadrant  bool
	quadX, quadY float64

	// Selection interaction: rectangle brush (default) or lasso.
	Tool         SelectTool
	dragging     bool
	dragStartPx  fyne.Position
	dragCurPx    fyne.Position
	lassoPts     []fyne.Position
	outlineShape *selShape

	pixW, pixH int
	img        *image.NRGBA // main data raster (expensive to recompute)
	overlay    *image.NRGBA // selection overlay raster (cheap, updated on every drag move)
	imgObj     *canvas.Image
	overlayObj *canvas.Image

	qLineV, qLineH                          *canvas.Line
	qLabelTL, qLabelTR, qLabelBL, qLabelBR *canvas.Text

	// OnBrush is called after a rectangular or lasso selection completes.
	OnBrush func(b Bounds, n int)
	// OnViewChanged is called after a zoom/pan.
	OnViewChanged func(v Bounds)

	mu sync.Mutex
}

// NewScatterPlot creates an empty plot, ready to be attached to a dataset
// via SetDataset + SetColumns. sel is the selection SHARED with the other
// plot: this is what lets a selection made here automatically show up in
// the other panel. quad is likewise shared, and carries the active
// quadrant crosshair's coloring over to the other plot.
func NewScatterPlot(ds *data.Dataset, sel *data.Selection, quad *data.QuadrantState) *ScatterPlot {
	p := &ScatterPlot{
		ds:           ds,
		sel:          sel,
		quad:         quad,
		DotRadius:    1,
		PointColor:   color.NRGBA{R: 140, G: 140, B: 145, A: 140}, // neutral grey base, like the original app
		Background:   defaultBackground,
		RenderBudget: 250_000,
		ShowDensity:  true,
		ColorByIdx:   -1,
		ColorByGradient: "Turbo",
		ColorByMinPct: 0,
		ColorByMaxPct: 100,
	}
	p.ExtendBaseWidget(p)

	p.img = image.NewNRGBA(image.Rect(0, 0, 1, 1))
	p.imgObj = canvas.NewImageFromImage(p.img)
	p.imgObj.FillMode = canvas.ImageFillStretch
	p.imgObj.ScaleMode = canvas.ImageScalePixels // no smoothing: we do our own edges

	p.overlay = image.NewNRGBA(image.Rect(0, 0, 1, 1))
	p.overlayObj = canvas.NewImageFromImage(p.overlay)
	p.overlayObj.FillMode = canvas.ImageFillStretch
	p.overlayObj.ScaleMode = canvas.ImageScalePixels

	p.qLineV = canvas.NewLine(color.NRGBA{40, 40, 40, 220})
	p.qLineV.StrokeWidth = 1
	p.qLineV.Hide()
	p.qLineH = canvas.NewLine(color.NRGBA{40, 40, 40, 220})
	p.qLineH.StrokeWidth = 1
	p.qLineH.Hide()

	newQLabel := func() *canvas.Text {
		t := canvas.NewText("", color.NRGBA{20, 20, 20, 255})
		t.TextSize = 12
		t.Hide()
		return t
	}
	p.qLabelTL = newQLabel()
	p.qLabelTR = newQLabel()
	p.qLabelBL = newQLabel()
	p.qLabelBR = newQLabel()

	sel.Subscribe(func() {
		if p.sel.Source() != p {
			// This selection change was made by the other plot (or was a
			// global clear): our own outline no longer applies.
			p.outlineShape = nil
		}
		p.redraw(p.Size())
	})
	if quad != nil {
		quad.Subscribe(func() {
			p.redraw(p.Size())
		})
	}
	return p
}

// SetDataset attaches (or replaces) the dataset shown by this plot. Call
// SetColumns afterwards to pick the axes and trigger the first render.
func (p *ScatterPlot) SetDataset(ds *data.Dataset) {
	p.mu.Lock()
	p.ds = ds
	p.grid = nil
	p.ColorByIdx = -1
	p.colorByLevelColor = nil
	p.mu.Unlock()
}

// SetColumns attaches the plot to a pair of dataset columns. Rebuilds the
// spatial index (O(N) cost, done once per axis change, not per frame) and
// resets the zoom to the full extent.
func (p *ScatterPlot) SetColumns(xName, yName string) bool {
	xi, ok1 := p.ds.ColumnIndex(xName)
	yi, ok2 := p.ds.ColumnIndex(yName)
	if !ok1 || !ok2 {
		return false
	}
	p.xIdx, p.yIdx = xi, yi
	p.XName, p.YName = xName, yName
	xArr, yArr := p.ds.Column(xi), p.ds.Column(yi)
	p.grid = data.BuildGridIndex(xArr, yArr, 512)
	p.full = Bounds{p.grid.Xmin, p.grid.Xmax, p.grid.Ymin, p.grid.Ymax}
	p.view = p.full
	p.hasQuadrant = false
	if p.quad != nil {
		p.quad.Clear(p) // a crosshair placed on the old axes no longer makes sense
	}
	p.redraw(p.Size())
	return true
}

// ZoomToFull resets the zoom to the full extent of the data.
func (p *ScatterPlot) ZoomToFull() {
	p.view = p.full
	p.redraw(p.Size())
}

// SetDotRadius adjusts point size (pixel radius, 0 = single pixel).
func (p *ScatterPlot) SetDotRadius(r int) {
	if r < 0 {
		r = 0
	}
	p.DotRadius = r
	p.redraw(p.Size())
}

// SetGateOnly toggles the "cumulative gate" display: when on, only cells
// currently in the shared selection are drawn (and counted for quadrant
// stats) — everything else is hidden, in both linked plots if both are
// toggled together.
func (p *ScatterPlot) SetGateOnly(on bool) {
	p.GateOnly = on
	p.redraw(p.Size())
}

// SetFreezeGate toggles sequential gating: when on, the next brush or
// lasso only keeps cells that were already part of the current selection,
// instead of starting over from the full dataset. This lets you gate a
// population in one plot, then refine it further in the other.
func (p *ScatterPlot) SetFreezeGate(on bool) {
	p.FreezeGate = on
}

// SetStackMode toggles selection stacking: when on, each new brush or
// lasso ADDS a new, independently colored population on top of whatever
// is already selected (cycling through stackPalette) instead of replacing
// it. Useful for comparing several gates picked on different marker
// combinations — e.g. select a population here, change the X/Y columns,
// select another, and both stay visible in distinct colors on both plots.
func (p *ScatterPlot) SetStackMode(on bool) {
	p.StackMode = on
}

// SetTool switches the mouse-drag behaviour: rectangular brush (default),
// freehand lasso, or pan (drag to move around the current zoom instead of
// selecting — useful once you're zoomed in and part of the plot is off
// screen).
func (p *ScatterPlot) SetTool(t SelectTool) {
	p.Tool = t
}

// SetBackground changes the plot's live background color (also used as
// the default background for ExportImage). Overlay lines, dashed outlines
// and quadrant labels automatically switch to a contrasting color.
func (p *ScatterPlot) SetBackground(bg color.NRGBA) {
	p.Background = bg
	p.redraw(p.Size())
}

// SetShowDensity toggles the marginal density histograms.
func (p *ScatterPlot) SetShowDensity(on bool) {
	p.ShowDensity = on
	p.redraw(p.Size())
}

// --- axis-graduated frame layout ---
//
// The plot is drawn inside an INNER rectangle, inset from the widget's
// full pixel bounds to leave room for the axis frame, tick labels, and
// (when ShowDensity is on) the marginal histograms. Zooming/panning only
// ever affects what's shown inside that inner rectangle — the margins
// keep their fixed on-screen size and position at every zoom level, which
// is what keeps the density histograms visible however far you zoom in.
const (
	axisLeftMargin   = 54 // room for Y tick labels
	axisBottomMargin = 28 // room for X tick labels + axis name
	densityMargin    = 64 // room for a marginal histogram, when ShowDensity is on
	plotFramePad     = 6  // small gap around the whole plot area
)

// computePlotRect returns the inner plot rectangle (x0, y0, width, height)
// for a raster of size w x h. scale multiplies every margin (1 for the
// live on-screen raster, the export scale factor for ExportImage) so
// margins stay proportionally sized regardless of output resolution.
func (p *ScatterPlot) computePlotRect(w, h, scale int) (x0, y0, pw, ph int) {
	if scale < 1 {
		scale = 1
	}
	pad := plotFramePad * scale
	left := axisLeftMargin * scale
	bottom := axisBottomMargin * scale
	x0 = left
	y0 = pad
	right := w - pad
	bottomY := h - bottom
	if p.ShowDensity {
		right -= densityMargin * scale
		y0 += densityMargin * scale
	}
	pw = right - x0
	ph = bottomY - y0
	if pw < 10 {
		pw = 10
	}
	if ph < 10 {
		ph = 10
	}
	return x0, y0, pw, ph
}

// pxToData converts a widget-local pixel position to data-space
// coordinates, using the current size and the inner plot rectangle.
// Positions outside the inner rectangle (i.e. over the axis margins or
// the density histograms) are clamped to its edges, so a drag that
// strays into a margin still produces a sensible in-range result instead
// of extrapolating outside the data.
func (p *ScatterPlot) pxToData(pos fyne.Position) (float64, float64) {
	x0, y0, pw, ph := p.computePlotRect(p.pixW, p.pixH, 1)
	fx := clampFloat32(pos.X, float32(x0), float32(x0+pw))
	fy := clampFloat32(pos.Y, float32(y0), float32(y0+ph))
	dx := p.view.Xmin + float64(fx-float32(x0))/float64(pw)*p.view.Width()
	dy := p.view.Ymin + float64(float32(y0+ph)-fy)/float64(ph)*p.view.Height()
	return dx, dy
}

// insidePlotRect reports whether a widget-local pixel position falls
// inside the inner plot rectangle (as opposed to the axis/histogram
// margins around it).
func (p *ScatterPlot) insidePlotRect(pos fyne.Position) bool {
	x0, y0, pw, ph := p.computePlotRect(p.pixW, p.pixH, 1)
	return pos.X >= float32(x0) && pos.X <= float32(x0+pw) && pos.Y >= float32(y0) && pos.Y <= float32(y0+ph)
}

func clampFloat32(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// SetQuadrantsEnabled toggles quadrant-gate mode. While enabled, clicking
// on the plot places a crosshair and the widget displays the cell
// count/percentage in each of the four quadrants; points are also colored
// by which quadrant they fall into.
func (p *ScatterPlot) SetQuadrantsEnabled(on bool) {
	p.quadrantsOn = on
	if !on && p.quad != nil {
		p.quad.Clear(p)
	}
	p.redraw(p.Size())
}

// ClearOutline drops this plot's persisted selection-outline shape without
// forcing a redraw (the caller — typically an Undo — is expected to
// trigger one right after via the shared Selection). Undo can restore an
// older set of selected indices, but it has no way to know what rectangle
// or lasso shape produced an older state several steps back, so the
// safest thing is to simply stop showing a (now possibly incorrect)
// outline rather than show a stale one.
func (p *ScatterPlot) ClearOutline() {
	p.outlineShape = nil
}

// --- fyne.WidgetRenderer ---

func (p *ScatterPlot) CreateRenderer() fyne.WidgetRenderer {
	return &scatterRenderer{p: p}
}

type scatterRenderer struct {
	p *ScatterPlot
}

func (r *scatterRenderer) Layout(size fyne.Size) {
	r.p.imgObj.Resize(size)
	r.p.overlayObj.Resize(size)
	if r.p.pixW == 0 {
		// First layout: render right away so the plot doesn't stay blank.
		r.p.redraw(size)
		return
	}
	// A window resize (dragging an edge/corner) fires Layout continuously,
	// many times per second. With a large dataset, a full redraw on every
	// single one of those events is what makes the window feel frozen
	// while resizing. Coalesce them: the existing raster is simply
	// stretched to the new size in the meantime (cheap, GPU-side), and the
	// real, sharp redraw only happens once resizing has been idle for a
	// short moment.
	r.p.scheduleRedraw()
}

func (r *scatterRenderer) MinSize() fyne.Size { return fyne.NewSize(320, 280) }

func (r *scatterRenderer) Refresh() {
	r.p.imgObj.Refresh()
	r.p.overlayObj.Refresh()
}

func (r *scatterRenderer) Destroy() {
	if r.p.resizeTimer != nil {
		r.p.resizeTimer.Stop()
	}
}

func (r *scatterRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{
		r.p.imgObj, r.p.overlayObj,
		r.p.qLineV, r.p.qLineH,
		r.p.qLabelTL, r.p.qLabelTR, r.p.qLabelBL, r.p.qLabelBR,
	}
}

// scheduleRedraw coalesces bursts of Layout calls (typically: the user
// actively dragging a window edge) into a single redraw fired shortly
// after things settle, instead of one full re-render per event. Safe to
// call repeatedly: each call cancels the pending timer and starts a new
// one.
// scheduleRedraw coalesces bursts of Layout calls (typically: the user
// actively dragging a window edge) into a single redraw fired shortly
// after things settle, instead of one full re-render per event. Safe to
// call repeatedly: each call cancels the pending timer and starts a new
// one.
//
// Deliberately re-reads the widget's CURRENT size inside the timer
// callback (via p.Size()) rather than closing over the size seen at
// schedule time: if another resize happens before the timer fires, using
// a stale size here would resize the internal raster back to an outdated
// value while the on-screen canvas objects have already moved on to the
// new size — the two textures then disagree, which is what could leave
// the plot looking stuck/misaligned until an unrelated interaction (like
// changing columns, which forces an immediate, non-debounced redraw)
// happened to resync them.
func (p *ScatterPlot) scheduleRedraw() {
	p.mu.Lock()
	if p.resizeTimer != nil {
		p.resizeTimer.Stop()
	}
	p.resizeTimer = time.AfterFunc(120*time.Millisecond, func() {
		fyne.Do(func() { p.redraw(p.Size()) })
	})
	p.mu.Unlock()
}

// redraw recomputes the full data raster for the current p.view. This is
// the only "expensive" step; it only runs on interaction, never in a
// render loop. It always finishes by refreshing the (cheap) overlay too,
// so the persistent selection outline stays correctly positioned.
func (p *ScatterPlot) redraw(size fyne.Size) {
	p.mu.Lock()
	defer p.mu.Unlock()

	w, h := int(size.Width), int(size.Height)
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	p.pixW, p.pixH = w, h
	if p.img == nil || p.img.Bounds().Dx() != w || p.img.Bounds().Dy() != h {
		p.img = image.NewNRGBA(image.Rect(0, 0, w, h))
	}
	p.renderPoints(p.img, p.Background, p.DotRadius, true, 1)
	fg := contrastColor(p.Background)
	x0, y0, pw, ph := p.computePlotRect(w, h, 1)
	p.drawFrameAndTicks(p.img, x0, y0, pw, ph, fg)
	if p.ShowDensity {
		p.drawMarginalHistograms(p.img, x0, y0, pw, ph, w, h, 1, fg)
	}
	p.imgObj.Image = p.img
	p.imgObj.Refresh()

	p.updateQuadrantOverlay(w, h)
	p.updateSelectionOverlay(w, h)
}
