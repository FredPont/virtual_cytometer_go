package plot

// Mouse interactions: rectangle/lasso/pan drag handling, wheel zoom, and
// the selection logic (including freeze-gate and stack-mode) they feed
// into.
//
// All coordinate conversions go through pxToData/dataToPx (see scatter.go
// and selection_overlay.go), which account for the axis/histogram margins
// around the inner plot rectangle — so a brush, lasso, pan or zoom always
// operates on the actual data area, never on the graduated frame or the
// marginal histograms around it.

import (
	"math"

	"fyne.io/fyne/v2"
)

// Dragged implements fyne.Draggable: tracks the rectangle or lasso
// currently being drawn (refreshing only the cheap overlay), or pans the
// view directly when Tool is ToolPan.
func (p *ScatterPlot) Dragged(ev *fyne.DragEvent) {
	switch p.Tool {
	case ToolPan:
		p.dragging = true
		p.panBy(ev.Dragged.DX, ev.Dragged.DY)
		return
	case ToolLasso:
		if !p.dragging {
			start := ev.Position
			start.X -= ev.Dragged.DX
			start.Y -= ev.Dragged.DY
			if !p.insidePlotRect(start) {
				return // ignore drags starting over the axes/histograms
			}
			p.dragging = true
			p.lassoPts = []fyne.Position{start}
		}
		if !p.dragging {
			return
		}
		last := p.lassoPts[len(p.lassoPts)-1]
		if distPos(last, ev.Position) >= 3 {
			p.lassoPts = append(p.lassoPts, ev.Position)
		}
		p.updateSelectionOverlay(p.pixW, p.pixH)
	default: // ToolRect
		if !p.dragging {
			start := ev.Position
			start.X -= ev.Dragged.DX
			start.Y -= ev.Dragged.DY
			if !p.insidePlotRect(start) {
				return // ignore drags starting over the axes/histograms
			}
			p.dragging = true
			p.dragStartPx = start
		}
		if !p.dragging {
			return
		}
		p.dragCurPx = ev.Position
		p.updateSelectionOverlay(p.pixW, p.pixH)
	}
}

// panBy shifts the current view by a pixel delta, keeping the data point
// that was under the cursor pinned under the cursor as you drag — this is
// what lets you reach cells that are off screen after zooming in. The
// delta is interpreted in the inner plot rectangle's scale, so panning
// speed doesn't depend on how large the axis/histogram margins are.
func (p *ScatterPlot) panBy(dxPx, dyPx float32) {
	if p.view.Width() <= 0 || p.view.Height() <= 0 {
		return
	}
	_, _, pw, ph := p.computePlotRect(p.pixW, p.pixH, 1)
	if pw < 1 || ph < 1 {
		return
	}
	sx := float64(pw) / p.view.Width()
	sy := float64(ph) / p.view.Height()
	dx := float64(dxPx) / sx
	dy := float64(dyPx) / sy
	p.view.Xmin -= dx
	p.view.Xmax -= dx
	p.view.Ymin += dy
	p.view.Ymax += dy
	p.redraw(p.Size())
	if p.OnViewChanged != nil {
		p.OnViewChanged(p.view)
	}
}

// DragEnd implements fyne.Draggable: applies the pending rectangle/lasso
// selection (a no-op when panning, or when the drag never started because
// it began outside the inner plot rectangle).
func (p *ScatterPlot) DragEnd() {
	dragged := p.dragging
	p.dragging = false
	switch p.Tool {
	case ToolPan:
		return
	case ToolLasso:
		if dragged {
			p.applyLasso()
		}
	default:
		if dragged {
			p.applyBrush()
		}
	}
}

func (p *ScatterPlot) applyBrush() {
	if p.grid == nil {
		return
	}
	x0, y0 := p.dragStartPx.X, p.dragStartPx.Y
	x1, y1 := p.dragCurPx.X, p.dragCurPx.Y
	if math.Abs(float64(x1-x0)) < 3 && math.Abs(float64(y1-y0)) < 3 {
		p.updateSelectionOverlay(p.pixW, p.pixH) // clear the transient drag preview
		return
	}
	xa, ya := p.pxToData(p.dragStartPx)
	xb, yb := p.pxToData(p.dragCurPx)
	xlo, xhi := xa, xb
	if xhi < xlo {
		xlo, xhi = xhi, xlo
	}
	ylo, yhi := ya, yb
	if yhi < ylo {
		ylo, yhi = yhi, ylo
	}

	xArr := p.ds.Column(p.xIdx)
	yArr := p.ds.Column(p.yIdx)
	frozen := p.frozenSet()
	var indices []int32
	p.grid.ForEachInRect(xArr, yArr, xlo, xhi, ylo, yhi, func(idx int32) {
		if frozen != nil {
			if _, ok := frozen[idx]; !ok {
				return
			}
		}
		indices = append(indices, idx)
	})
	p.outlineShape = &selShape{kind: 'r', rect: Bounds{xlo, xhi, ylo, yhi}}
	p.commitSelection(indices)
	if p.OnBrush != nil {
		p.OnBrush(Bounds{xlo, xhi, ylo, yhi}, len(indices))
	}
}

func (p *ScatterPlot) applyLasso() {
	defer func() { p.lassoPts = nil }()
	if p.grid == nil || len(p.lassoPts) < 3 {
		p.updateSelectionOverlay(p.pixW, p.pixH)
		return
	}

	poly := make([]dataPoint, len(p.lassoPts))
	xlo, xhi := math.Inf(1), math.Inf(-1)
	ylo, yhi := math.Inf(1), math.Inf(-1)
	for i, pt := range p.lassoPts {
		dx, dy := p.pxToData(pt)
		poly[i] = dataPoint{dx, dy}
		if dx < xlo {
			xlo = dx
		}
		if dx > xhi {
			xhi = dx
		}
		if dy < ylo {
			ylo = dy
		}
		if dy > yhi {
			yhi = dy
		}
	}

	xArr := p.ds.Column(p.xIdx)
	yArr := p.ds.Column(p.yIdx)
	frozen := p.frozenSet()
	var indices []int32
	p.grid.ForEachInRect(xArr, yArr, xlo, xhi, ylo, yhi, func(idx int32) {
		if frozen != nil {
			if _, ok := frozen[idx]; !ok {
				return
			}
		}
		if pointInPolygon(float64(xArr[idx]), float64(yArr[idx]), poly) {
			indices = append(indices, idx)
		}
	})

	p.outlineShape = &selShape{kind: 'l', poly: poly}
	p.commitSelection(indices)
	if p.OnBrush != nil {
		p.OnBrush(Bounds{xlo, xhi, ylo, yhi}, len(indices))
	}
}

// commitSelection applies a freshly brushed/lassoed set of indices to the
// shared Selection: replacing it (default), or adding it as a new colored
// layer on top of whatever is already selected (StackMode).
func (p *ScatterPlot) commitSelection(indices []int32) {
	if p.StackMode {
		c := stackPalette[p.sel.LayerCount()%len(stackPalette)]
		p.sel.AddLayer(indices, c, p)
		return
	}
	p.sel.SetFrom(indices, p)
}

// frozenSet builds a membership set from the currently selected cells, for
// sequential (freeze-gate) selection. Returns nil when freeze-gate is off.
func (p *ScatterPlot) frozenSet() map[int32]struct{} {
	if !p.FreezeGate {
		return nil
	}
	sel := p.sel.Get()
	set := make(map[int32]struct{}, len(sel))
	for _, idx := range sel {
		set[idx] = struct{}{}
	}
	return set
}

// pointInPolygon is a standard ray-casting point-in-polygon test.
func pointInPolygon(x, y float64, poly []dataPoint) bool {
	inside := false
	n := len(poly)
	if n < 3 {
		return false
	}
	j := n - 1
	for i := 0; i < n; i++ {
		xi, yi := poly[i].X, poly[i].Y
		xj, yj := poly[j].X, poly[j].Y
		if (yi > y) != (yj > y) {
			xIntersect := (xj-xi)*(y-yi)/(yj-yi) + xi
			if x < xIntersect {
				inside = !inside
			}
		}
		j = i
	}
	return inside
}

// Tapped implements fyne.Tappable: in quadrant mode, a click inside the
// plot area places the crosshair; otherwise a plain click (no drag)
// clears the shared selection.
func (p *ScatterPlot) Tapped(ev *fyne.PointEvent) {
	if p.quadrantsOn {
		if p.insidePlotRect(ev.Position) {
			p.setQuadrantAt(ev.Position)
		}
		return
	}
	p.sel.Clear()
}

// Scrolled implements fyne.Scrollable: zoom centered under the cursor.
func (p *ScatterPlot) Scrolled(ev *fyne.ScrollEvent) {
	if p.grid == nil || ev.Scrolled.DY == 0 {
		return
	}
	cx, cy := p.pxToData(ev.Position)

	factor := 1.0 / 0.9
	if ev.Scrolled.DY > 0 {
		factor = 0.9
	}
	newW := p.view.Width() * factor
	newH := p.view.Height() * factor
	if newW < p.full.Width()/2000 || newH < p.full.Height()/2000 {
		return // cap zoom-in at roughly 2000x the full extent
	}
	fracX := (cx - p.view.Xmin) / p.view.Width()
	fracY := (cy - p.view.Ymin) / p.view.Height()
	newXmin := cx - fracX*newW
	newYmin := cy - fracY*newH
	p.view = Bounds{newXmin, newXmin + newW, newYmin, newYmin + newH}
	p.redraw(p.Size())
	if p.OnViewChanged != nil {
		p.OnViewChanged(p.view)
	}
}
