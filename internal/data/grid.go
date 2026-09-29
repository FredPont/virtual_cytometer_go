package data

import "math"

// GridIndex is a uniform spatial index (regular grid) built over a pair of
// columns (X, Y). It lets zoom and rectangular/lasso selection queries
// visit only the cells that actually fall inside the visible/selected
// region instead of scanning all N rows of the dataset on every
// interaction.
//
// Storage uses the CSR (Compressed Sparse Row) layout: two int32 slices,
// no per-point structs -> minimal memory overhead (~8 bytes/point,
// independent of how many columns the dataset has).
type GridIndex struct {
	Resolution             int
	Xmin, Xmax, Ymin, Ymax float64

	cellStart  []int32 // length Resolution*Resolution+1
	cellPoints []int32 // length N, row indices sorted by cell
}

// BuildGridIndex builds the index for columns x,y (same length).
// resolution=512 is a good default trade-off (262144 cells).
func BuildGridIndex(x, y []float32, resolution int) *GridIndex {
	n := len(x)
	xmin, xmax := minMaxFinite(x)
	ymin, ymax := minMaxFinite(y)
	if xmax <= xmin {
		xmax = xmin + 1
	}
	if ymax <= ymin {
		ymax = ymin + 1
	}

	g := &GridIndex{Resolution: resolution, Xmin: xmin, Xmax: xmax, Ymin: ymin, Ymax: ymax}

	cellOf := make([]int32, n)
	counts := make([]int32, resolution*resolution+1)
	sx := float64(resolution) / (xmax - xmin)
	sy := float64(resolution) / (ymax - ymin)
	for i := 0; i < n; i++ {
		xi, yi := x[i], y[i]
		if isNaN32(xi) || isNaN32(yi) {
			cellOf[i] = -1
			continue
		}
		cx := int((float64(xi) - xmin) * sx)
		cy := int((float64(yi) - ymin) * sy)
		if cx < 0 {
			cx = 0
		} else if cx >= resolution {
			cx = resolution - 1
		}
		if cy < 0 {
			cy = 0
		} else if cy >= resolution {
			cy = resolution - 1
		}
		c := int32(cy*resolution + cx)
		cellOf[i] = c
		counts[c+1]++
	}
	for i := 1; i < len(counts); i++ {
		counts[i] += counts[i-1]
	}
	cursor := append([]int32(nil), counts...)
	points := make([]int32, n)
	for i := 0; i < n; i++ {
		c := cellOf[i]
		if c < 0 {
			continue // NaN point: simply omitted from the index
		}
		points[cursor[c]] = int32(i)
		cursor[c]++
	}
	g.cellStart = counts
	g.cellPoints = points
	return g
}

// CountInRect returns an approximate number of points inside the
// rectangle [xlo,xhi] x [ylo,yhi], computed from per-cell counts only
// (no per-point work at all): cells fully or partially inside the
// rectangle's cell-range are counted as a whole row-strip in one
// subtraction, since a grid row's cells are contiguous in the CSR arrays.
// This over-counts slightly at the rectangle's edges (whole cells are
// counted even if only partially covered) but that's fine — it is only
// used to decide how aggressively to sub-sample points for on-screen
// rendering, not for anything that needs to be exact.
func (g *GridIndex) CountInRect(xlo, xhi, ylo, yhi float64) int {
	if g == nil {
		return 0
	}
	res := g.Resolution
	sx := float64(res) / (g.Xmax - g.Xmin)
	sy := float64(res) / (g.Ymax - g.Ymin)

	cxlo := clampInt(int((xlo-g.Xmin)*sx), 0, res-1)
	cxhi := clampInt(int((xhi-g.Xmin)*sx), 0, res-1)
	cylo := clampInt(int((ylo-g.Ymin)*sy), 0, res-1)
	cyhi := clampInt(int((yhi-g.Ymin)*sy), 0, res-1)

	total := 0
	for cy := cylo; cy <= cyhi; cy++ {
		rowBase := cy * res
		total += int(g.cellStart[rowBase+cxhi+1] - g.cellStart[rowBase+cxlo])
	}
	return total
}

// ForEachInRect visits every row index i whose point (x[i],y[i]) falls
// inside the rectangle [xlo,xhi] x [ylo,yhi]. Only the grid cells that
// intersect this rectangle are scanned.
func (g *GridIndex) ForEachInRect(x, y []float32, xlo, xhi, ylo, yhi float64, visit func(idx int32)) {
	if g == nil {
		return
	}
	res := g.Resolution
	sx := float64(res) / (g.Xmax - g.Xmin)
	sy := float64(res) / (g.Ymax - g.Ymin)

	cxlo := clampInt(int((xlo-g.Xmin)*sx), 0, res-1)
	cxhi := clampInt(int((xhi-g.Xmin)*sx), 0, res-1)
	cylo := clampInt(int((ylo-g.Ymin)*sy), 0, res-1)
	cyhi := clampInt(int((yhi-g.Ymin)*sy), 0, res-1)

	for cy := cylo; cy <= cyhi; cy++ {
		rowBase := cy * res
		for cx := cxlo; cx <= cxhi; cx++ {
			c := rowBase + cx
			start, end := g.cellStart[c], g.cellStart[c+1]
			for k := start; k < end; k++ {
				idx := g.cellPoints[k]
				xi, yi := x[idx], y[idx]
				if float64(xi) >= xlo && float64(xi) <= xhi && float64(yi) >= ylo && float64(yi) <= yhi {
					visit(idx)
				}
			}
		}
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

func minMaxFinite(v []float32) (float64, float64) {
	min := math.Inf(1)
	max := math.Inf(-1)
	for _, f := range v {
		if isNaN32(f) {
			continue
		}
		x := float64(f)
		if x < min {
			min = x
		}
		if x > max {
			max = x
		}
	}
	if math.IsInf(min, 1) {
		return 0, 1
	}
	return min, max
}

func isNaN32(f float32) bool {
	return f != f
}
