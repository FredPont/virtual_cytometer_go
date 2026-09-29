package plot

// Standalone, publication-quality image export (always full-resolution,
// never sub-sampled — see ExportImage's doc comment) plus the small bitmap
// text renderer used to burn axis labels and quadrant stats into it.

import (
	"fmt"
	"image"
	"image/color"

	"fyne.io/fyne/v2"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// ExportImage renders a standalone, publication-quality snapshot of the
// plot: points, the quadrant crosshair and stats (if quadrant mode is on),
// the persisted selection outline, and simple axis labels — all baked
// into the returned image, independent of the live Fyne canvas.
//
// bg lets the caller choose a white or black (or any other) background;
// lines, text and the selection outline automatically switch to a
// contrasting color. scale multiplies the current on-screen pixel size
// (2 or 3 gives a crisper image for print/figures) and the dot radius is
// scaled to match, so points keep the same relative size.
func (p *ScatterPlot) ExportImage(bg color.NRGBA, scale int) *image.NRGBA {
	p.mu.Lock()
	defer p.mu.Unlock()

	if scale < 1 {
		scale = 1
	}
	w, h := p.pixW*scale, p.pixH*scale
	if w < 10 {
		w = 800
	}
	if h < 10 {
		h = 600
	}

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	p.renderPoints(img, bg, p.DotRadius*scale, false, scale) // false = no sub-sampling, export gets every point

	fg := contrastColor(bg)
	x0, y0, pw, ph := p.computePlotRect(w, h, scale)
	p.drawFrameAndTicks(img, x0, y0, pw, ph, fg)
	if p.ShowDensity {
		p.drawMarginalHistograms(img, x0, y0, pw, ph, w, h, scale, fg)
	}
	if p.quadrantsOn && p.hasQuadrant && p.grid != nil {
		p.drawQuadrantsInto(img, x0, y0, pw, ph, fg)
	}
	if p.outlineShape != nil {
		p.drawShapeInto(img, p.outlineShape, w, h, scale, fg)
	}
	return img
}

func (p *ScatterPlot) drawQuadrantsInto(img *image.NRGBA, x0, y0, pw, ph int, c color.NRGBA) {
	view := p.view
	if p.quadX < view.Xmin || p.quadX > view.Xmax || p.quadY < view.Ymin || p.quadY > view.Ymax {
		return
	}
	sx := float64(pw) / view.Width()
	sy := float64(ph) / view.Height()
	px := float64(x0) + (p.quadX-view.Xmin)*sx
	py := float64(y0+ph) - (p.quadY-view.Ymin)*sy
	drawLineSegment(img, px, float64(y0), px, float64(y0+ph), c)
	drawLineSegment(img, float64(x0), py, float64(x0+pw), py, c)

	tl, tr, bl, br := p.quadrantCounts()
	total := tl + tr + bl + br
	pct := func(n int) float64 {
		if total == 0 {
			return 0
		}
		return 100 * float64(n) / float64(total)
	}
	drawText(img, x0+6, y0+16, fmt.Sprintf("%d (%.1f%%)", tl, pct(tl)), c)
	drawText(img, x0+pw-100, y0+16, fmt.Sprintf("%d (%.1f%%)", tr, pct(tr)), c)
	drawText(img, x0+6, y0+ph-8, fmt.Sprintf("%d (%.1f%%)", bl, pct(bl)), c)
	drawText(img, x0+pw-100, y0+ph-8, fmt.Sprintf("%d (%.1f%%)", br, pct(br)), c)
}

func (p *ScatterPlot) drawShapeInto(img *image.NRGBA, shape *selShape, w, h, scale int, c color.NRGBA) {
	switch shape.kind {
	case 'r':
		p1 := p.dataToPx(shape.rect.Xmin, shape.rect.Ymin, w, h, scale)
		p2 := p.dataToPx(shape.rect.Xmax, shape.rect.Ymax, w, h, scale)
		drawDashedRectPx(img, p1, p2, c)
	case 'l':
		pts := make([]fyne.Position, len(shape.poly))
		for i, dp := range shape.poly {
			pts[i] = p.dataToPx(dp.X, dp.Y, w, h, scale)
		}
		drawDashedPolyline(img, pts, c, true)
	}
}

// drawText draws s with its baseline at (x,y) using a small fixed-size
// bitmap font (no external font file needed). Used only for export images
// — the live on-screen widget uses canvas.Text for quadrant labels.
func drawText(img *image.NRGBA, x, y int, s string, c color.NRGBA) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(c),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(s)
}

// --- mouse interactions: rectangle brush, lasso, wheel zoom ---
