package plot

// "Color by" support: color points according to a data column instead of
// the flat PointColor. A column is either:
//   - a continuous gradient (one of GradientNames, colormap.go), for
//     numeric columns with many distinct values, or
//   - one flat color per class, for text columns and for numeric columns
//     with few distinct values (or when ColorByClasses forces it).
//
// The auto gradient-vs-classes decision is a simple cardinality check: a
// column with at most categoricalCardinalityThreshold distinct values is
// assumed to be categorical (cluster ids, small integer codes...) even
// when stored as a number. This mirrors what most plotting libraries do
// in the absence of real "is this a factor" type information, since our
// dataset format doesn't distinguish "integer" from "float" — everything
// numeric is a float32 column; only text columns carry a real, separate
// Kind (see data.ColumnKind).
//
// Class colors come from one of two sources depending on how many
// distinct classes there are: the Okabe-Ito qualitative palette
// (stackPalette, scatter.go) for up to 7 — it's the best choice at that
// size, picked for maximum mutual distinctness rather than even hue
// spacing — and, beyond that, N evenly-spaced samples along the chosen
// gradient (ColorByGradient), the same technique used for a continuous
// scale. This is what lets a 20-cluster column get 20 genuinely different
// colors instead of the qualitative palette repeating after 7.

import (
	"image/color"
	"math"

	"scvirtualcytometer/internal/data"
)

// categoricalCardinalityThreshold: at or below this many distinct values,
// a numeric column defaults to per-class colors instead of a gradient.
const categoricalCardinalityThreshold = 20

// SetColorBy chooses which column drives point color. Pass "" (or the
// picker's "(none)" entry, filtered out by the caller) to go back to the
// plain PointColor. Returns false if the column doesn't exist.
func (p *ScatterPlot) SetColorBy(colName string) bool {
	if p.ds == nil || colName == "" {
		p.ColorByIdx = -1
		p.colorByLevelColor = nil
		p.redraw(p.Size())
		return true
	}
	ci, ok := p.ds.ColumnIndex(colName)
	if !ok {
		return false
	}
	p.ColorByIdx = ci
	p.recomputeColorByScale()
	p.redraw(p.Size())
	return true
}

// SetColorByClasses forces (or un-forces) per-class coloring for the
// current "color by" column, overriding the automatic cardinality-based
// decision. Only matters once a column is selected via SetColorBy; safe
// to call regardless.
func (p *ScatterPlot) SetColorByClasses(force bool) {
	p.ColorByClasses = force
	p.recomputeColorByScale()
	p.redraw(p.Size())
}

// SetColorByGradient picks which continuous gradient to use — both for a
// numeric "color by" column shown as a gradient, and, sampled at N
// points, as the color source for a categorical column with more than 7
// distinct classes (see classColors). name should be one of GradientNames
// (colormap.go); an unrecognized name falls back to Viridis.
func (p *ScatterPlot) SetColorByGradient(name string) {
	p.ColorByGradient = name
	p.recomputeColorByScale()
	p.redraw(p.Size())
}

// SetColorByRange clips the gradient to a sub-range of the "color by"
// column's actual data range, expressed as percentages (0..100) of that
// range — the same "windowed" gradient technique as a levels/contrast
// adjustment: cells below minPct show the gradient's first color, cells
// above maxPct show its last, rather than a handful of unusually low/high
// cells stretching the color scale so far that most cells end up looking
// the same muddy middle color. Has no effect on categorical (per-class)
// coloring. Invalid ranges (maxPct <= minPct) are ignored.
func (p *ScatterPlot) SetColorByRange(minPct, maxPct float64) {
	if maxPct <= minPct {
		return
	}
	p.ColorByMinPct, p.ColorByMaxPct = minPct, maxPct
	p.redraw(p.Size())
}

// recomputeColorByScale rebuilds whatever's needed to color points by the
// current ColorByIdx column: either a global min/max (gradient) or a
// level -> color table (classes). Called once per column/mode/gradient
// change, never per point — the per-point cost is then just a slice/map
// lookup.
func (p *ScatterPlot) recomputeColorByScale() {
	p.colorByLevelColor = nil
	p.colorByMin, p.colorByMax = 0, 1
	p.colorByCategorical = false
	if p.ColorByIdx < 0 || p.ds == nil || p.ColorByIdx >= len(p.ds.Data) {
		return
	}
	col := p.ds.Data[p.ColorByIdx]
	isString := p.ColorByIdx < len(p.ds.Kinds) && p.ds.Kinds[p.ColorByIdx] == data.KindString

	categorical := isString || p.ColorByClasses
	if !categorical {
		if distinctUpTo(col, categoricalCardinalityThreshold) <= categoricalCardinalityThreshold {
			categorical = true
		}
	}
	p.colorByCategorical = categorical

	if !categorical {
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, v := range col {
			if isNaN32(v) {
				continue
			}
			f := float64(v)
			if f < lo {
				lo = f
			}
			if f > hi {
				hi = f
			}
		}
		if !math.IsInf(lo, 1) {
			p.colorByMin, p.colorByMax = lo, hi
		}
		return
	}

	// Categorical: build a stable level -> color table. For text columns
	// the levels (and their index encoding) already come from the loader
	// in first-seen order; for numeric columns we derive the same kind of
	// first-seen ordering directly from the data here. Either way, a
	// FULL scan is needed to know exactly how many distinct classes there
	// are (the cheap capped scan above only answers "is it <= threshold
	// or not" — with ColorByClasses forced on, there could be far more).
	// This only runs once per column/mode change, so an O(N) pass is
	// cheap in context.
	m := make(map[int]color.NRGBA)
	if isString && p.ColorByIdx < len(p.ds.CategoryLevels) {
		n := len(p.ds.CategoryLevels[p.ColorByIdx])
		colors := classColors(n, p.ColorByGradient)
		for i := range colors {
			m[i] = colors[i]
		}
	} else {
		order := make(map[int]int)
		next := 0
		for _, v := range col {
			if isNaN32(v) {
				continue
			}
			key := int(math.Round(float64(v)))
			if _, ok := order[key]; !ok {
				order[key] = next
				next++
			}
		}
		colors := classColors(next, p.ColorByGradient)
		for key, idx := range order {
			m[key] = colors[idx]
		}
	}
	p.colorByLevelColor = m
}

// classColors returns n colors for categorical coloring: the Okabe-Ito
// qualitative palette directly when it comfortably covers n classes, or n
// evenly-spaced samples along the named gradient otherwise — the same
// approach as sampling a gradient at "Sharp" steps for cluster coloring.
func classColors(n int, gradientName string) []color.NRGBA {
	if n <= 0 {
		return nil
	}
	if n <= len(stackPalette) {
		return stackPalette[:n]
	}
	stops := gradientByName(gradientName)
	cols := make([]color.NRGBA, n)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(n-1)
		cols[i] = lerpPalette(stops, t)
	}
	return cols
}

// colorByColor returns the "color by" color for row idx. Only called when
// p.ColorByIdx >= 0.
func (p *ScatterPlot) colorByColor(idx int32) color.NRGBA {
	col := p.ds.Data[p.ColorByIdx]
	if int(idx) >= len(col) {
		return p.PointColor
	}
	v := col[idx]
	if isNaN32(v) {
		return color.NRGBA{R: 140, G: 140, B: 140, A: 90} // muted grey for missing/NA
	}
	if p.colorByCategorical {
		key := int(math.Round(float64(v)))
		if c, ok := p.colorByLevelColor[key]; ok {
			return c
		}
		return p.PointColor
	}
	t := 0.0001 // never exactly 0: some gradients (e.g. Viridis via densityColor elsewhere) treat 0 as transparent
	if p.colorByMax > p.colorByMin {
		span := p.colorByMax - p.colorByMin
		lo := p.colorByMin + p.ColorByMinPct/100*span
		hi := p.colorByMin + p.ColorByMaxPct/100*span
		if hi > lo {
			t = (float64(v) - lo) / (hi - lo)
			if t < 0.0001 {
				t = 0.0001
			} else if t > 1 {
				t = 1
			}
		}
	}
	return lerpPalette(gradientByName(p.ColorByGradient), t)
}

// distinctUpTo counts distinct (non-NaN) values in col, stopping as soon
// as it exceeds cap — so a high-cardinality column (the common case,
// since most numeric columns are continuous measurements) is recognized
// as "definitely not categorical" almost immediately, without scanning
// the rest of possibly millions of rows.
func distinctUpTo(col []float32, cap int) int {
	seen := make(map[float32]struct{}, cap+1)
	for _, v := range col {
		if isNaN32(v) {
			continue
		}
		seen[v] = struct{}{}
		if len(seen) > cap {
			return len(seen)
		}
	}
	return len(seen)
}

func isNaN32(f float32) bool {
	return f != f
}
