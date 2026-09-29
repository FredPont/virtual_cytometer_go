// Single-Cell Virtual Cytometer — Go + Fyne port
//
// A native desktop port of the "Single-Cell Virtual Cytometer"
// JavaScript/Plotly application, with a few improvements over the
// original:
//
//  1. Cross-plot selection: a rectangular or lasso brush in either panel
//     highlights the same cells in the other panel (see
//     internal/data.Selection).
//  2. Sequential (freeze) gating and cumulative "gate only" display,
//     plus a quadrant-gate mode with live counts/percentages
//     (see internal/plot.ScatterPlot).
//  3. Rendering that can show millions of cells with smooth zoom without
//     blowing up memory: points are never individual Fyne objects, they
//     are rasterized directly into an image whose size is the display
//     size, not the dataset size (see internal/plot.ScatterPlot and
//     internal/data.GridIndex).
package main

import (
	"fyne.io/fyne/v2/app"

	"scvirtualcytometer/internal/ui"
)

func main() {
	a := app.NewWithID("io.github.scvirtualcytometer")
	a.Settings().SetTheme(ui.NewDarkPurpleTheme())
	w := a.NewWindow("Single-Cell Virtual Cytometer")
	ui.NewMainWindow(w)
	w.ShowAndRun()
}
