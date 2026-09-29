package plot

// The axis-graduated frame (tick marks + numeric labels around the inner
// plot rectangle) and the optional marginal density histograms — the
// classic flow-cytometry biaxial plot layout: a graduated frame with a
// density-per-slice histogram along the top (X) and right (Y) edges.
//
// Both are computed fresh from whatever is currently visible (p.view) on
// every redraw, so they always reflect the current zoom level — but their
// SCREEN position and size never change with zoom, only their content
// does. That's what keeps the histograms visible at any zoom level: only
// the inner plot rectangle they surround is what actually pans/zooms.

import (
	"image"
	"image/color"
	"math"
	"strconv"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
)

// niceTicks picks roughly targetCount "nice" (1/2/5 x power-of-ten) tick
// values covering [min,max] — the standard approach used by essentially
// every charting library, so ticks land on round numbers like 0, 5000,
// 10000 rather than awkward fractions.
func niceTicks(min, max float64, targetCount int) []float64 {
	if !(max > min) || targetCount < 2 {
		return nil
	}
	rawStep := (max - min) / float64(targetCount)
	mag := math.Pow(10, math.Floor(math.Log10(rawStep)))
	norm := rawStep / mag
	var step float64
	switch {
	case norm < 1.5:
		step = 1
	case norm < 3:
		step = 2
	case norm < 7:
		step = 5
	default:
		step = 10
	}
	step *= mag
	if step <= 0 {
		return nil
	}
	start := math.Ceil(min/step) * step
	var ticks []float64
	for t := start; t <= max+step*1e-9; t += step {
		ticks = append(ticks, t)
		if len(ticks) > 64 {
			break // sanity cap, should never trigger with the step logic above
		}
	}
	return ticks
}

// formatTick renders a tick value compactly: whole numbers with no
// decimals, otherwise a couple of significant digits, switching to
// scientific notation for very small/large magnitudes.
func formatTick(v float64) string {
	av := math.Abs(v)
	switch {
	case v == 0:
		return "0"
	case av < 0.001 || av >= 1_000_000:
		return strconv.FormatFloat(v, 'e', 1, 64)
	case v == math.Trunc(v):
		return strconv.FormatFloat(v, 'f', 0, 64)
	default:
		return strconv.FormatFloat(v, 'f', 2, 64)
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func measureText(s string) int {
	d := font.Drawer{Face: basicfont.Face7x13}
	return d.MeasureString(s).Ceil()
}

func drawTextCentered(img *image.NRGBA, cx, baselineY int, s string, c color.NRGBA) {
	drawText(img, cx-measureText(s)/2, baselineY, s, c)
}

func drawTextRightAligned(img *image.NRGBA, rightX, baselineY int, s string, c color.NRGBA) {
	drawText(img, rightX-measureText(s), baselineY, s, c)
}

// fillRect alpha-blends a color into [x0,x1) x [y0,y1) (clipped to img's
// bounds) — used for the (semi-transparent) histogram bars. Blending
// against the already-drawn background, rather than writing raw
// semi-transparent pixels, matters here: img is handed directly to Fyne
// as the final on-screen texture, so any literal transparency in it would
// let whatever is behind the widget show through instead of our own
// chosen plot background.
func fillRect(img *image.NRGBA, x0, y0, x1, y1 int, c color.NRGBA) {
	b := img.Bounds()
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 > b.Max.X {
		x1 = b.Max.X
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			blendPixel(img, x, y, c)
		}
	}
}

// drawFrameAndTicks draws the graduated border around the inner plot
// rectangle [x0,y0,x0+pw,y0+ph]: a solid frame, tick marks, and numeric
// axis labels in the current data range (p.view).
func (p *ScatterPlot) drawFrameAndTicks(img *image.NRGBA, x0, y0, pw, ph int, fg color.NRGBA) {
	fx0, fy0, fx1, fy1 := float64(x0), float64(y0), float64(x0+pw), float64(y0+ph)
	drawLineSegment(img, fx0, fy0, fx1, fy0, fg)
	drawLineSegment(img, fx0, fy1, fx1, fy1, fg)
	drawLineSegment(img, fx0, fy0, fx0, fy1, fg)
	drawLineSegment(img, fx1, fy0, fx1, fy1, fg)

	view := p.view
	if view.Width() <= 0 || view.Height() <= 0 {
		return
	}

	for _, t := range niceTicks(view.Xmin, view.Xmax, 6) {
		px := x0 + int((t-view.Xmin)/view.Width()*float64(pw))
		drawLineSegment(img, float64(px), fy1, float64(px), fy1+4, fg)
		drawTextCentered(img, px, y0+ph+17, formatTick(t), fg)
	}
	for _, t := range niceTicks(view.Ymin, view.Ymax, 6) {
		py := y0 + ph - int((t-view.Ymin)/view.Height()*float64(ph))
		drawLineSegment(img, fx0-4, float64(py), fx0, float64(py), fg)
		drawTextRightAligned(img, x0-7, py+4, formatTick(t), fg)
	}

	if p.XName != "" {
		drawTextCentered(img, x0+pw/2, y0+ph+axisBottomMargin-2, p.XName, fg)
	}
	if p.YName != "" {
		// No vertical/rotated text support (would need real font
		// rendering rather than the bitmap font) — printed horizontally
		// in the top-left corner of the left margin instead.
		drawText(img, 2, y0-4, p.YName, fg)
	}
}

// drawMarginalHistograms bins the currently visible points (full
// precision, never sub-sampled — this pass is a simple per-bin counter,
// much cheaper than the main point rendering) along X and Y, and draws
// them as bar histograms in the margins directly above and to the right
// of the inner plot rectangle.
func (p *ScatterPlot) drawMarginalHistograms(img *image.NRGBA, x0, y0, pw, ph, w, h, scale int, fg color.NRGBA) {
	if p.ds == nil || p.grid == nil {
		return
	}
	view := p.view
	if view.Width() <= 0 || view.Height() <= 0 {
		return
	}
	xArr := p.ds.Column(p.xIdx)
	yArr := p.ds.Column(p.yIdx)

	const bins = 60
	var xCounts, yCounts [bins]int
	p.grid.ForEachInRect(xArr, yArr, view.Xmin, view.Xmax, view.Ymin, view.Ymax, func(idx int32) {
		fx, fy := float64(xArr[idx]), float64(yArr[idx])
		bx := int((fx - view.Xmin) / view.Width() * bins)
		bx = clampInt(bx, 0, bins-1)
		xCounts[bx]++
		by := int((fy - view.Ymin) / view.Height() * bins)
		by = clampInt(by, 0, bins-1)
		yCounts[by]++
	})

	maxX, maxY := 1, 1
	for _, c := range xCounts {
		if c > maxX {
			maxX = c
		}
	}
	for _, c := range yCounts {
		if c > maxY {
			maxY = c
		}
	}
	fill := histogramFillColor(fg)

	// Top histogram (X distribution): bars grow upward from the frame's
	// top edge into the top margin.
	histBottom := y0
	histTop := plotFramePad * scale
	availH := histBottom - histTop
	for i := 0; i < bins; i++ {
		barH := int(float64(xCounts[i]) / float64(maxX) * float64(availH))
		bx0 := x0 + i*pw/bins
		bx1 := x0 + (i+1)*pw/bins
		if bx1 <= bx0 {
			bx1 = bx0 + 1
		}
		fillRect(img, bx0, histBottom-barH, bx1, histBottom, fill)
	}

	// Right histogram (Y distribution): bars grow rightward from the
	// frame's right edge into the right margin.
	histLeft := x0 + pw
	histRight := w - plotFramePad*scale
	availW := histRight - histLeft
	for i := 0; i < bins; i++ {
		barW := int(float64(yCounts[i]) / float64(maxY) * float64(availW))
		by1 := y0 + ph - i*ph/bins
		by0 := y0 + ph - (i+1)*ph/bins
		if by1 <= by0 {
			by1 = by0 + 1
		}
		fillRect(img, histLeft, by0, histLeft+barW, by1, fill)
	}
}

// histogramFillColor derives a translucent fill from the frame/text color
// so the histograms read clearly on both white and black backgrounds
// without needing their own separate theme color.
func histogramFillColor(fg color.NRGBA) color.NRGBA {
	return color.NRGBA{R: fg.R, G: fg.G, B: fg.B, A: 110}
}
