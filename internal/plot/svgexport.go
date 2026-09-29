package plot

// SVG export: a hybrid of raster and true vector content.
//
// The point cloud itself is embedded as a base64 PNG <image> rather than
// one SVG shape per cell: a real per-point vector export (a <circle> or
// <rect> per cell) would produce multi-million-element files for exactly
// the large datasets this app is built for — slow to generate, huge on
// disk, and often too much for a browser or vector editor to open at all.
// Everything ELSE — the graduated frame, tick marks and axis labels, the
// marginal density histograms, the quadrant crosshair and its stats, and
// the selection outline — IS real, scalable SVG: text stays crisp at any
// zoom, dashes use SVG's native stroke-dasharray, and all of it can be
// recolored/edited in Illustrator or Inkscape after export, independent
// of the embedded point-cloud image.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
)

// ExportSVG renders the same content as ExportImage (points, quadrant
// crosshair/stats, selection outline, graduated frame, marginal
// histograms) as an SVG document string. bg and scale have the same
// meaning as in ExportImage.
func (p *ScatterPlot) ExportSVG(bg color.NRGBA, scale int) (string, error) {
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
	x0, y0, pw, ph := p.computePlotRect(w, h, scale)
	fg := contrastColor(bg)

	// Raster layer: points only (background-filled), no frame/ticks/
	// histogram/quadrant/outline — those are added below as real SVG.
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	p.renderPoints(img, bg, p.DotRadius*scale, false, scale)
	pngData, err := encodePNG(img)
	if err != nil {
		return "", fmt.Errorf("encoding embedded point-cloud image: %w", err)
	}
	b64 := base64.StdEncoding.EncodeToString(pngData)

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`+"\n", w, h, w, h)
	fmt.Fprintf(&b, `<image x="0" y="0" width="%d" height="%d" href="data:image/png;base64,%s"/>`+"\n", w, h, b64)

	p.svgFrameAndTicks(&b, x0, y0, pw, ph, fg, scale)
	if p.ShowDensity {
		p.svgMarginalHistograms(&b, x0, y0, pw, ph, w, h, fg)
	}
	if p.quadrantsOn && p.hasQuadrant && p.grid != nil {
		p.svgQuadrants(&b, x0, y0, pw, ph, fg, scale)
	}
	if p.outlineShape != nil {
		p.svgShape(&b, p.outlineShape, w, h, scale, fg)
	}

	b.WriteString("</svg>\n")
	return b.String(), nil
}

func encodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// svgColorAttr formats a color as an SVG-compatible rgba() paint value.
func svgColorAttr(c color.NRGBA) string {
	return fmt.Sprintf("rgba(%d,%d,%d,%.3f)", c.R, c.G, c.B, float64(c.A)/255)
}

func svgLine(b *strings.Builder, x0, y0, x1, y1 float64, c color.NRGBA, width float64) {
	fmt.Fprintf(b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="%.1f"/>`+"\n",
		x0, y0, x1, y1, svgColorAttr(c), width)
}

func svgRect(b *strings.Builder, x0, y0, x1, y1 float64, c color.NRGBA) {
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	fmt.Fprintf(b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`+"\n",
		x0, y0, x1-x0, y1-y0, svgColorAttr(c))
}

func svgTextAnchored(b *strings.Builder, x, y float64, s, anchor string, c color.NRGBA, size int) {
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" text-anchor="%s" font-family="monospace" font-size="%d" fill="%s">%s</text>`+"\n",
		x, y, anchor, size, svgColorAttr(c), svgEscape(s))
}

func svgEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func (p *ScatterPlot) svgFrameAndTicks(b *strings.Builder, x0, y0, pw, ph int, fg color.NRGBA, scale int) {
	fx0, fy0, fx1, fy1 := float64(x0), float64(y0), float64(x0+pw), float64(y0+ph)
	svgLine(b, fx0, fy0, fx1, fy0, fg, float64(scale))
	svgLine(b, fx0, fy1, fx1, fy1, fg, float64(scale))
	svgLine(b, fx0, fy0, fx0, fy1, fg, float64(scale))
	svgLine(b, fx1, fy0, fx1, fy1, fg, float64(scale))

	view := p.view
	if view.Width() <= 0 || view.Height() <= 0 {
		return
	}
	fontSize := 12 * scale
	for _, t := range niceTicks(view.Xmin, view.Xmax, 6) {
		px := float64(x0) + (t-view.Xmin)/view.Width()*float64(pw)
		svgLine(b, px, fy1, px, fy1+float64(4*scale), fg, float64(scale))
		svgTextAnchored(b, px, fy1+float64(17*scale), formatTick(t), "middle", fg, fontSize)
	}
	for _, t := range niceTicks(view.Ymin, view.Ymax, 6) {
		py := float64(y0+ph) - (t-view.Ymin)/view.Height()*float64(ph)
		svgLine(b, fx0-float64(4*scale), py, fx0, py, fg, float64(scale))
		svgTextAnchored(b, fx0-float64(7*scale), py+float64(4*scale), formatTick(t), "end", fg, fontSize)
	}
	if p.XName != "" {
		svgTextAnchored(b, float64(x0+pw/2), float64(y0+ph+axisBottomMargin*scale-2*scale), p.XName, "middle", fg, fontSize)
	}
	if p.YName != "" {
		svgTextAnchored(b, 2, float64(y0-4*scale), p.YName, "start", fg, fontSize)
	}
}

func (p *ScatterPlot) svgMarginalHistograms(b *strings.Builder, x0, y0, pw, ph, w, h int, fg color.NRGBA) {
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
		xCounts[clampInt(int((fx-view.Xmin)/view.Width()*bins), 0, bins-1)]++
		yCounts[clampInt(int((fy-view.Ymin)/view.Height()*bins), 0, bins-1)]++
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

	histBottom := y0
	histTop := plotFramePad
	availH := histBottom - histTop
	for i := 0; i < bins; i++ {
		barH := int(float64(xCounts[i]) / float64(maxX) * float64(availH))
		bx0 := x0 + i*pw/bins
		bx1 := x0 + (i+1)*pw/bins
		svgRect(b, float64(bx0), float64(histBottom-barH), float64(bx1), float64(histBottom), fill)
	}
	histLeft := x0 + pw
	histRight := w - plotFramePad
	availW := histRight - histLeft
	for i := 0; i < bins; i++ {
		barW := int(float64(yCounts[i]) / float64(maxY) * float64(availW))
		by1 := y0 + ph - i*ph/bins
		by0 := y0 + ph - (i+1)*ph/bins
		svgRect(b, float64(histLeft), float64(by0), float64(histLeft+barW), float64(by1), fill)
	}
}

func (p *ScatterPlot) svgQuadrants(b *strings.Builder, x0, y0, pw, ph int, fg color.NRGBA, scale int) {
	view := p.view
	if p.quadX < view.Xmin || p.quadX > view.Xmax || p.quadY < view.Ymin || p.quadY > view.Ymax {
		return
	}
	px := float64(x0) + (p.quadX-view.Xmin)/view.Width()*float64(pw)
	py := float64(y0+ph) - (p.quadY-view.Ymin)/view.Height()*float64(ph)
	svgLine(b, px, float64(y0), px, float64(y0+ph), fg, float64(scale))
	svgLine(b, float64(x0), py, float64(x0+pw), py, fg, float64(scale))

	tl, tr, bl, br := p.quadrantCounts()
	total := tl + tr + bl + br
	pct := func(n int) float64 {
		if total == 0 {
			return 0
		}
		return 100 * float64(n) / float64(total)
	}
	fontSize := 12 * scale
	label := func(n int) string { return fmt.Sprintf("%d (%.1f%%)", n, pct(n)) }
	svgTextAnchored(b, float64(x0+6*scale), float64(y0+16*scale), label(tl), "start", fg, fontSize)
	svgTextAnchored(b, float64(x0+pw-6*scale), float64(y0+16*scale), label(tr), "end", fg, fontSize)
	svgTextAnchored(b, float64(x0+6*scale), float64(y0+ph-8*scale), label(bl), "start", fg, fontSize)
	svgTextAnchored(b, float64(x0+pw-6*scale), float64(y0+ph-8*scale), label(br), "end", fg, fontSize)
}

func (p *ScatterPlot) svgShape(b *strings.Builder, shape *selShape, w, h, scale int, c color.NRGBA) {
	dash := fmt.Sprintf(`stroke="%s" stroke-width="%d" fill="none" stroke-dasharray="%d,%d"`,
		svgColorAttr(c), scale, 6*scale, 4*scale)
	switch shape.kind {
	case 'r':
		p1 := p.dataToPx(shape.rect.Xmin, shape.rect.Ymin, w, h, scale)
		p2 := p.dataToPx(shape.rect.Xmax, shape.rect.Ymax, w, h, scale)
		x0, x1 := float64(p1.X), float64(p2.X)
		y0, y1 := float64(p1.Y), float64(p2.Y)
		if x1 < x0 {
			x0, x1 = x1, x0
		}
		if y1 < y0 {
			y0, y1 = y1, y0
		}
		fmt.Fprintf(b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" %s/>`+"\n", x0, y0, x1-x0, y1-y0, dash)
	case 'l':
		var pts strings.Builder
		for i, dp := range shape.poly {
			pos := p.dataToPx(dp.X, dp.Y, w, h, scale)
			if i > 0 {
				pts.WriteByte(' ')
			}
			fmt.Fprintf(&pts, "%.1f,%.1f", pos.X, pos.Y)
		}
		fmt.Fprintf(b, `<polygon points="%s" %s/>`+"\n", pts.String(), dash)
	}
}
