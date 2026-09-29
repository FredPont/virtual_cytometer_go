package plot

// This file contains the point rasterization itself: turning the current
// view + dataset into pixels. Kept separate from scatter.go (widget
// plumbing) and interaction.go (mouse handling) so the actual rendering
// algorithm is easy to find and read on its own.

import (
	"image"
	"image/color"
)

// quadrantColor returns which of the 4 quadrant colors a point (x,y)
// falls into relative to a crosshair at (qx,qy).
func quadrantColor(x, y float32, qx, qy float64) color.NRGBA {
	left := x < float32(qx)
	top := y >= float32(qy)
	switch {
	case left && top:
		return stackPalette[0] // top-left
	case !left && top:
		return stackPalette[1] // top-right
	case left && !top:
		return stackPalette[2] // bottom-left
	default:
		return stackPalette[3] // bottom-right
	}
}

// renderPoints does the actual point rasterization into img, using img's
// own size (so it works both for the on-screen raster and for a
// higher-resolution export image) and the given background color and dot
// radius. Shared by redraw() and ExportImage().
//
// Points are positioned within the INNER plot rectangle only (see
// computePlotRect) — the margins reserved for axis ticks/labels and, when
// ShowDensity is on, the marginal histograms, are left to
// drawFrameAndTicks/drawMarginalHistograms. scale should be 1 for the
// live on-screen raster and the export scale factor for ExportImage, so
// margins stay proportionally sized either way.
//
// When subsample is true and more than RenderBudget points fall in the
// current view, only an evenly-spaced fraction of them is actually
// splatted (see renderStride) — this is what keeps interaction smooth
// with very large datasets. subsample is always false for image export,
// so exported images always contain every single point.
func (p *ScatterPlot) renderPoints(img *image.NRGBA, bg color.NRGBA, radius int, subsample bool, scale int) {
	fillOpaque(img, bg)
	if p.ds == nil || p.grid == nil || p.view.Width() <= 0 || p.view.Height() <= 0 {
		return
	}
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	x0, y0, pw, ph := p.computePlotRect(w, h, scale)
	xArr := p.ds.Column(p.xIdx)
	yArr := p.ds.Column(p.yIdx)
	view := p.view
	sx := float64(pw) / view.Width()
	sy := float64(ph) / view.Height()

	splat := func(idx int32, c color.NRGBA) {
		if int(idx) >= len(xArr) {
			return
		}
		fx, fy := float64(xArr[idx]), float64(yArr[idx])
		if fx < view.Xmin || fx > view.Xmax || fy < view.Ymin || fy > view.Ymax {
			return
		}
		px := x0 + int((fx-view.Xmin)*sx)
		py := y0 + ph - 1 - int((fy-view.Ymin)*sy)
		for oy := -radius; oy <= radius; oy++ {
			yy := py + oy
			if yy < y0 || yy >= y0+ph {
				continue
			}
			for ox := -radius; ox <= radius; ox++ {
				xx := px + ox
				if xx < x0 || xx >= x0+pw {
					continue
				}
				blendPixel(img, xx, yy, c)
			}
		}
	}

	// quadrantColorer, when non-nil, returns a point's quadrant color for
	// a given row index. It prefers this plot's OWN crosshair (placed
	// directly on it); if this plot has none but the OTHER, linked plot
	// currently has one active, membership is computed from that other
	// plot's source columns instead — this is what makes quadrant
	// coloring carry over to the linked plot, matching the original app.
	var quadrantColorer func(idx int32) color.NRGBA
	switch {
	case p.quadrantsOn && p.hasQuadrant:
		qx, qy := p.quadX, p.quadY
		quadrantColorer = func(idx int32) color.NRGBA {
			return quadrantColor(xArr[idx], yArr[idx], qx, qy)
		}
	case p.quad != nil:
		if active, xCol, yCol, qx, qy, source := p.quad.Get(); active && source != p {
			if xi, ok1 := p.ds.ColumnIndex(xCol); ok1 {
				if yi, ok2 := p.ds.ColumnIndex(yCol); ok2 {
					srcX, srcY := p.ds.Column(xi), p.ds.Column(yi)
					quadrantColorer = func(idx int32) color.NRGBA {
						if int(idx) >= len(srcX) {
							return p.PointColor
						}
						return quadrantColor(srcX[idx], srcY[idx], qx, qy)
					}
				}
			}
		}
	}

	// pointColor picks a point's color, in priority order:
	//  1. "Color by" a data column, if one is chosen — an explicit,
	//     persistent visualization choice, so it wins over anything else.
	//  2. One of the 4 quadrant colors, when Quadrants mode is active
	//     (matches the original app, which colors each quadrant's cells
	//     with a distinct color from the same palette used for stacked
	//     selections) — quadrant coloring only shows through when no
	//     "color by" column is set.
	//  3. The plot's single, flat PointColor.
	pointColor := func(idx int32) color.NRGBA {
		if p.ColorByIdx >= 0 {
			return p.colorByColor(idx)
		}
		if quadrantColorer != nil {
			return quadrantColorer(idx)
		}
		return p.PointColor
	}

	if p.GateOnly {
		// Only the gated population is drawn; it IS the plot, so no
		// separate highlight pass is needed. Each stacked layer keeps its
		// own color.
		for _, layer := range p.sel.Layers() {
			c := layer.Color
			if c.A == 0 {
				c = highlightColor
			}
			for _, idx := range layer.Indices {
				splat(idx, c)
			}
		}
		return
	}

	stride := 1
	if subsample {
		stride = p.renderStride(view)
	}
	if stride <= 1 {
		p.grid.ForEachInRect(xArr, yArr, view.Xmin, view.Xmax, view.Ymin, view.Ymax, func(idx int32) {
			splat(idx, pointColor(idx))
		})
	} else {
		n := 0
		p.grid.ForEachInRect(xArr, yArr, view.Xmin, view.Xmax, view.Ymin, view.Ymax, func(idx int32) {
			if n%stride == 0 {
				splat(idx, pointColor(idx))
			}
			n++
		})
	}
	// Selected cells are always drawn in full (never sub-sampled) and on
	// top. Each stacked layer keeps its own color; a plain (non-stacked)
	// selection falls back to the single highlight color. Cost is
	// proportional to the number of SELECTED cells (usually << N), not N.
	for _, layer := range p.sel.Layers() {
		c := layer.Color
		if c.A == 0 {
			c = highlightColor
		}
		for _, idx := range layer.Indices {
			splat(idx, c)
		}
	}
}

// renderStride returns how many candidate points to skip between two
// splatted ones so that roughly RenderBudget points end up drawn for the
// current view. It relies on GridIndex.CountInRect, which is a handful of
// integer subtractions — not a per-point scan — so it stays cheap however
// large the dataset is.
func (p *ScatterPlot) renderStride(view Bounds) int {
	if p.RenderBudget <= 0 {
		return 1
	}
	approx := p.grid.CountInRect(view.Xmin, view.Xmax, view.Ymin, view.Ymax)
	if approx <= p.RenderBudget {
		return 1
	}
	return (approx + p.RenderBudget - 1) / p.RenderBudget
}

// contrastColor returns a light gray for dark backgrounds and a dark gray
// for light backgrounds, so overlay lines/text stay legible either way.
func contrastColor(bg color.NRGBA) color.NRGBA {
	lum := 0.299*float64(bg.R) + 0.587*float64(bg.G) + 0.114*float64(bg.B)
	if lum > 140 {
		return color.NRGBA{R: 20, G: 20, B: 20, A: 255}
	}
	return color.NRGBA{R: 235, G: 235, B: 235, A: 255}
}

func fillOpaque(img *image.NRGBA, c color.NRGBA) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
}

// blendPixel performs standard "source-over" alpha compositing of c onto
// an already-opaque background pixel.
func blendPixel(img *image.NRGBA, x, y int, c color.NRGBA) {
	if c.A == 0 {
		return
	}
	if c.A == 255 {
		img.SetNRGBA(x, y, c)
		return
	}
	bg := img.NRGBAAt(x, y)
	a := float64(c.A) / 255
	ia := 1 - a
	img.SetNRGBA(x, y, color.NRGBA{
		R: uint8(float64(c.R)*a + float64(bg.R)*ia),
		G: uint8(float64(c.G)*a + float64(bg.G)*ia),
		B: uint8(float64(c.B)*a + float64(bg.B)*ia),
		A: 255,
	})
}

// --- quadrant gate ---
