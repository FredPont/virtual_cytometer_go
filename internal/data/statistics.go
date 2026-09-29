package data

// Per-column summary statistics for a population of cells (a gate), the
// equivalent of the original app's statistics() but generalized from "just
// the X/Y columns of the density plot" to every numeric column — the
// usual convention for a per-marker statistics table in flow cytometry
// software.

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
)

// ColumnStats holds summary statistics for one column over some subset of
// rows.
type ColumnStats struct {
	Column                          string
	N                                int // non-NaN values found
	Mean, Median, SD, Variance, CV float64
}

// StatPopulation names a set of rows to report statistics for — e.g. the
// whole dataset, the current selection, or one Stack layer.
// Indices == nil means "every row in the dataset".
type StatPopulation struct {
	Name    string
	Indices []int32
}

// ComputeStats computes per-column statistics (numeric columns only —
// text columns are skipped, since mean/median/etc. don't apply to them)
// over the given row indices. A nil indices slice means "the whole
// dataset".
func (d *Dataset) ComputeStats(indices []int32) []ColumnStats {
	out := make([]ColumnStats, 0, len(d.Columns))
	for c, name := range d.Columns {
		if c < len(d.Kinds) && d.Kinds[c] == KindString {
			continue
		}
		var vals []float64
		if indices == nil {
			vals = make([]float64, 0, d.N)
			for i := 0; i < d.N; i++ {
				v := d.Data[c][i]
				if v != v { // NaN
					continue
				}
				vals = append(vals, float64(v))
			}
		} else {
			vals = make([]float64, 0, len(indices))
			for _, idx := range indices {
				if int(idx) < 0 || int(idx) >= len(d.Data[c]) {
					continue
				}
				v := d.Data[c][idx]
				if v != v {
					continue
				}
				vals = append(vals, float64(v))
			}
		}
		out = append(out, columnStats(name, vals))
	}
	return out
}

func columnStats(name string, vals []float64) ColumnStats {
	st := ColumnStats{Column: name, N: len(vals)}
	n := len(vals)
	if n == 0 {
		st.Mean, st.Median, st.SD, st.Variance, st.CV = math.NaN(), math.NaN(), math.NaN(), math.NaN(), math.NaN()
		return st
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	mean := sum / float64(n)
	st.Mean = mean

	sorted := append([]float64(nil), vals...)
	sort.Float64s(sorted)
	if n%2 == 1 {
		st.Median = sorted[n/2]
	} else {
		st.Median = (sorted[n/2-1] + sorted[n/2]) / 2
	}

	if n > 1 {
		ss := 0.0
		for _, v := range vals {
			d := v - mean
			ss += d * d
		}
		st.Variance = ss / float64(n-1) // sample variance, matches the original app's math.js default
		st.SD = math.Sqrt(st.Variance)
	} else {
		st.Variance, st.SD = math.NaN(), math.NaN()
	}
	if mean != 0 {
		st.CV = st.SD / mean
	} else {
		st.CV = math.NaN()
	}
	return st
}

// WriteStatsReport writes a per-column statistics table covering every
// population in pops, one row per (population, column) pair — a "long"/
// tidy table, easy to filter or pivot in a spreadsheet, e.g. to compare
// several Stack layers side by side for one marker.
func (d *Dataset) WriteStatsReport(w io.Writer, pops []StatPopulation) error {
	bw := bufio.NewWriterSize(w, 1<<16)
	bw.WriteString("population\tn_cells\tpct_of_total\tcolumn\tmean\tmedian\tsd\tvariance\tcv\n")
	for _, pop := range pops {
		n := len(pop.Indices)
		if pop.Indices == nil {
			n = d.N
		}
		pct := 0.0
		if d.N > 0 {
			pct = 100 * float64(n) / float64(d.N)
		}
		for _, s := range d.ComputeStats(pop.Indices) {
			fmt.Fprintf(bw, "%s\t%d\t%.2f\t%s\t%s\t%s\t%s\t%s\t%s\n",
				pop.Name, n, pct, s.Column,
				formatStatValue(s.Mean), formatStatValue(s.Median), formatStatValue(s.SD),
				formatStatValue(s.Variance), formatStatValue(s.CV))
		}
	}
	return bw.Flush()
}

func formatStatValue(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "NA"
	}
	return strconv.FormatFloat(v, 'g', 6, 64)
}
