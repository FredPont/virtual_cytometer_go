package plot

// The cheap overlay raster used to show the live rectangle/lasso being
// drawn and the persistent dashed outline of the last selection made on
// this plot. Deliberately kept separate from the (expensive) point raster
// in render.go: this one redraws on every mouse-move during a drag, so it
// must never touch the actual data.

import (
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
)

// updateSelectionOverlay redraws the cheap overlay raster: the live
// in-progress drag shape while dragging, otherwise the persisted outline
// of the last selection made by this plot (if any). It never touches the
// (expensive) main data raster, so it is safe to call on every mouse-move
// event during a drag.
func (p *ScatterPlot) updateSelectionOverlay(w, h int) {
	if w < 1 || h < 1 {
		return
	}
	if p.overlay == nil || p.overlay.Bounds().Dx() != w || p.overlay.Bounds().Dy() != h {
		p.overlay = image.NewNRGBA(image.Rect(0, 0, w, h))
	} else {
		clear(p.overlay.Pix)
	}

	fg := contrastColor(p.Background)
	if p.dragging {
		switch p.Tool {
		case ToolLasso:
			drawDashedPolyline(p.overlay, p.lassoPts, fg, false)
		case ToolRect:
			drawDashedRectPx(p.overlay, p.dragStartPx, p.dragCurPx, fg)
		case ToolPan:
			// nothing to draw while panning
		}
	} else if p.outlineShape != nil {
		p.drawPersistedShape(fg)
	}

	p.overlayObj.Image = p.overlay
	p.overlayObj.Refresh()
}

func (p *ScatterPlot) dataToPx(x, y float64, w, h, scale int) fyne.Position {
	x0, y0, pw, ph := p.computePlotRect(w, h, scale)
	px := float64(x0) + (x-p.view.Xmin)/p.view.Width()*float64(pw)
	py := float64(y0+ph) - (y-p.view.Ymin)/p.view.Height()*float64(ph)
	return fyne.NewPos(float32(px), float32(py))
}

func (p *ScatterPlot) drawPersistedShape(c color.NRGBA) {
	w, h := p.pixW, p.pixH
	if w < 1 || h < 1 || p.view.Width() <= 0 || p.view.Height() <= 0 {
		return
	}
	switch p.outlineShape.kind {
	case 'r':
		r := p.outlineShape.rect
		p1 := p.dataToPx(r.Xmin, r.Ymin, w, h, 1)
		p2 := p.dataToPx(r.Xmax, r.Ymax, w, h, 1)
		drawDashedRectPx(p.overlay, p1, p2, c)
	case 'l':
		pts := make([]fyne.Position, len(p.outlineShape.poly))
		for i, dp := range p.outlineShape.poly {
			pts[i] = p.dataToPx(dp.X, dp.Y, w, h, 1)
		}
		drawDashedPolyline(p.overlay, pts, c, true)
	}
}

func drawDashedRectPx(img *image.NRGBA, p1, p2 fyne.Position, c color.NRGBA) {
	x0, x1 := float64(p1.X), float64(p2.X)
	y0, y1 := float64(p1.Y), float64(p2.Y)
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	drawDashedLine(img, x0, y0, x1, y0, c)
	drawDashedLine(img, x1, y0, x1, y1, c)
	drawDashedLine(img, x1, y1, x0, y1, c)
	drawDashedLine(img, x0, y1, x0, y0, c)
}

func drawDashedPolyline(img *image.NRGBA, pts []fyne.Position, c color.NRGBA, closed bool) {
	if len(pts) < 2 {
		return
	}
	for i := 0; i < len(pts)-1; i++ {
		drawDashedLine(img, float64(pts[i].X), float64(pts[i].Y), float64(pts[i+1].X), float64(pts[i+1].Y), c)
	}
	if closed {
		last, first := pts[len(pts)-1], pts[0]
		drawDashedLine(img, float64(last.X), float64(last.Y), float64(first.X), float64(first.Y), c)
	}
}

// drawDashedLine draws a dashed segment (6px on, 4px off) between two
// points in the overlay raster.
func drawDashedLine(img *image.NRGBA, x0, y0, x1, y1 float64, c color.NRGBA) {
	const dash, gap = 6.0, 4.0
	dx, dy := x1-x0, y1-y0
	dist := math.Hypot(dx, dy)
	if dist < 1e-9 {
		return
	}
	ux, uy := dx/dist, dy/dist
	pos := 0.0
	on := true
	for pos < dist {
		segLen := dash
		if !on {
			segLen = gap
		}
		end := pos + segLen
		if end > dist {
			end = dist
		}
		if on {
			drawLineSegment(img, x0+ux*pos, y0+uy*pos, x0+ux*end, y0+uy*end, c)
		}
		pos = end
		on = !on
	}
}

func drawLineSegment(img *image.NRGBA, x0, y0, x1, y1 float64, c color.NRGBA) {
	b := img.Bounds()
	steps := int(math.Hypot(x1-x0, y1-y0)) + 1
	setPx := func(x, y int) {
		if x >= b.Min.X && x < b.Max.X && y >= b.Min.Y && y < b.Max.Y {
			img.SetNRGBA(x, y, c)
		}
	}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(x0 + (x1-x0)*t)
		y := int(y0 + (y1-y0)*t)
		setPx(x, y)
		setPx(x+1, y) // ~2px thickness so the dashes stay visible
		setPx(x, y+1)
	}
}

func distPos(a, b fyne.Position) float64 {
	return math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y))
}
