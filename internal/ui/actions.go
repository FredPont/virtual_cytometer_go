package ui

// File loading and export actions for MainWindow: opening a dataset file
// (in a background goroutine, with a progress bar), applying axis column
// choices to each plot, exporting the current selection to CSV, and
// exporting a plot to a PNG image.

import (
	"fmt"
	"image/color"
	"image/png"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"

	"scvirtualcytometer/internal/data"
	"scvirtualcytometer/internal/plot"
)

// colorByNone is the synthetic first entry offered by the "Color by"
// pickers, meaning "use the plain point color, don't color by a column".
const colorByNone = "(none)"

// importCellsColor is the fixed highlight color used for cells brought in
// via "Overlay cells from file…" — deliberately not part of stackPalette,
// so an import never gets confused with a manually stacked selection
// layer even if their colors happened to coincide.
var importCellsColor = color.NRGBA{R: 220, G: 30, B: 30, A: 235}

func (mw *MainWindow) exportSelection() {
	if mw.ds == nil {
		return
	}
	indices := mw.sel.Get()
	if len(indices) == 0 {
		dialog.ShowInformation("Export selection", "No cells are currently selected.", mw.win)
		return
	}
	fd := dialog.NewFileSave(func(uc fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, mw.win)
			return
		}
		if uc == nil {
			return // canceled by the user
		}
		defer uc.Close()
		if werr := mw.ds.WriteCSV(uc, indices); werr != nil {
			dialog.ShowError(werr, mw.win)
		}
	}, mw.win)
	fd.SetFileName("selection.csv")
	fd.Show()
}

// exportQuadrants exports the WHOLE dataset tagged by quadrant membership
// (Q1..Q4), using whichever quadrant crosshair is currently active on
// either plot (data.QuadrantState is shared between them, so it doesn't
// matter which one placed it).
func (mw *MainWindow) exportQuadrants() {
	if mw.ds == nil {
		dialog.ShowInformation("Export quadrants", "Load a file first.", mw.win)
		return
	}
	active, xCol, yCol, x, y, _ := mw.quad.Get()
	if !active {
		dialog.ShowInformation("Export quadrants",
			"No quadrant gate is active. Enable \"Quadrants\" on a plot and click on it to place a crosshair first.",
			mw.win)
		return
	}
	fd := dialog.NewFileSave(func(uc fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, mw.win)
			return
		}
		if uc == nil {
			return // canceled by the user
		}
		defer uc.Close()
		if werr := mw.ds.WriteQuadrantCSV(uc, xCol, yCol, x, y); werr != nil {
			dialog.ShowError(werr, mw.win)
		}
	}, mw.win)
	fd.SetFileName("quadrants.csv")
	fd.Show()
}

// exportStatistics writes a per-column (marker) statistics table for the
// current selection — the whole dataset as a reference row plus, for each
// column, mean/median/SD/variance/CV within the gate. If Stack mode
// produced more than one layer, each layer gets its own row block plus a
// combined "All selected" one, so several gated populations can be
// compared side by side in the same file.
func (mw *MainWindow) exportStatistics() {
	if mw.ds == nil {
		dialog.ShowInformation("Export gate statistics", "Load a file first.", mw.win)
		return
	}
	if mw.sel.Count() == 0 {
		dialog.ShowInformation("Export gate statistics", "No cells are currently selected/gated.", mw.win)
		return
	}

	pops := []data.StatPopulation{{Name: "All cells", Indices: nil}}
	layers := mw.sel.Layers()
	if len(layers) > 1 {
		for i, l := range layers {
			pops = append(pops, data.StatPopulation{Name: fmt.Sprintf("Layer %d", i+1), Indices: l.Indices})
		}
		pops = append(pops, data.StatPopulation{Name: "All selected (all layers)", Indices: mw.sel.Get()})
	} else {
		pops = append(pops, data.StatPopulation{Name: "Gated cells", Indices: mw.sel.Get()})
	}

	fd := dialog.NewFileSave(func(uc fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, mw.win)
			return
		}
		if uc == nil {
			return // canceled by the user
		}
		defer uc.Close()
		if werr := mw.ds.WriteStatsReport(uc, pops); werr != nil {
			dialog.ShowError(werr, mw.win)
		}
	}, mw.win)
	fd.SetFileName("gate_statistics.csv")
	fd.Show()
}

// importCells reads a list of cell ids from a file (see data.ParseIDList
// for the accepted formats) and highlights the matching cells — on BOTH
// linked plots — as a new colored layer on top of whatever is already
// selected, the same mechanism used by Stack mode. This mirrors the
// original app's "overlay cells" feature, but with one consistent color
// across both plots instead of two different ones.
func (mw *MainWindow) importCells() {
	if mw.ds == nil {
		dialog.ShowInformation("Import cells", "Load a dataset first.", mw.win)
		return
	}
	fd := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, mw.win)
			return
		}
		if uc == nil {
			return // canceled by the user
		}
		defer uc.Close()

		ids, perr := data.ParseIDList(uc)
		if perr != nil {
			dialog.ShowError(perr, mw.win)
			return
		}
		indices, missing := mw.ds.IndicesForIDs(ids)
		if len(indices) == 0 {
			dialog.ShowInformation("Import cells",
				"None of the cell ids in that file were found in the currently loaded dataset.", mw.win)
			return
		}
		mw.sel.AddLayer(indices, importCellsColor, "import")

		msg := fmt.Sprintf("%d cell(s) imported and highlighted on both plots.", len(indices))
		if missing > 0 {
			msg += fmt.Sprintf(" %d id(s) from the file had no match in the current dataset and were skipped.", missing)
		}
		dialog.ShowInformation("Import cells", msg, mw.win)
	}, mw.win)
	fd.SetFilter(storage.NewExtensionFileFilter([]string{".csv", ".tsv", ".txt"}))
	fd.Show()
}

func (mw *MainWindow) exportImage(p *plot.ScatterPlot, baseName string) {
	if mw.ds == nil {
		dialog.ShowInformation("Export image", "Load a file first.", mw.win)
		return
	}
	// scale=2: a bit crisper than the current on-screen size, good enough
	// for slides/figures without costing much render time. Uses whatever
	// background is currently shown on screen (see the Background radio
	// buttons at the top).
	if mw.exportFormat == "SVG" {
		svg, err := p.ExportSVG(p.Background, 2)
		if err != nil {
			dialog.ShowError(err, mw.win)
			return
		}
		fd := dialog.NewFileSave(func(uc fyne.URIWriteCloser, err error) {
			if err != nil {
				dialog.ShowError(err, mw.win)
				return
			}
			if uc == nil {
				return // canceled by the user
			}
			defer uc.Close()
			if _, werr := uc.Write([]byte(svg)); werr != nil {
				dialog.ShowError(werr, mw.win)
			}
		}, mw.win)
		fd.SetFileName(baseName + ".svg")
		fd.Show()
		return
	}

	img := p.ExportImage(p.Background, 2)
	fd := dialog.NewFileSave(func(uc fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, mw.win)
			return
		}
		if uc == nil {
			return // canceled by the user
		}
		defer uc.Close()
		if werr := png.Encode(uc, img); werr != nil {
			dialog.ShowError(werr, mw.win)
		}
	}, mw.win)
	fd.SetFileName(baseName + ".png")
	fd.Show()
}

func (mw *MainWindow) applyColumnsLeft() {
	if mw.ds == nil || mw.xPickLeft.selected == "" || mw.yPickLeft.selected == "" {
		return
	}
	mw.plotLeft.SetColumns(mw.xPickLeft.selected, mw.yPickLeft.selected)
}

func (mw *MainWindow) applyColumnsRight() {
	if mw.ds == nil || mw.xPickRight.selected == "" || mw.yPickRight.selected == "" {
		return
	}
	mw.plotRight.SetColumns(mw.xPickRight.selected, mw.yPickRight.selected)
}

// applyColorByLeft/Right translate the picker's synthetic "(none)" entry
// into turning color-by off, and otherwise apply the chosen column.
func (mw *MainWindow) applyColorByLeft(name string) {
	if mw.ds == nil {
		return
	}
	if name == colorByNone {
		name = ""
	}
	mw.plotLeft.SetColorBy(name)
}

func (mw *MainWindow) applyColorByRight(name string) {
	if mw.ds == nil {
		return
	}
	if name == colorByNone {
		name = ""
	}
	mw.plotRight.SetColorBy(name)
}

func (mw *MainWindow) openFile() {
	fd := dialog.NewFileOpen(func(uc fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, mw.win)
			return
		}
		if uc == nil {
			return // canceled by the user
		}
		path := uc.URI().Path()
		uc.Close()
		mw.loadPath(path)
	}, mw.win)
	fd.SetFilter(storage.NewExtensionFileFilter([]string{".csv", ".tsv", ".txt"}))
	fd.Show()
}

func (mw *MainWindow) loadPath(path string) {
	mw.openBtn.Disable()
	mw.progress.Show()
	mw.progress.SetValue(0)
	mw.fileLabel.SetText("Loading…")

	go func() {
		ds, err := data.Load(path, data.LoadOptions{
			KeepIDs: true,
			Progress: func(frac float64) {
				fyne.Do(func() { mw.progress.SetValue(frac) })
			},
		})
		fyne.Do(func() {
			mw.openBtn.Enable()
			mw.progress.Hide()
			if err != nil {
				dialog.ShowError(err, mw.win)
				mw.fileLabel.SetText("Failed to load file")
				return
			}
			mw.onLoaded(ds, path)
		})
	}()
}

func (mw *MainWindow) onLoaded(ds *data.Dataset, path string) {
	mw.ds = ds
	mw.sel.Clear()
	mw.fileLabel.SetText(fmt.Sprintf("%s  —  %d cells x %d columns", path, ds.N, len(ds.Columns)))

	mw.plotLeft.SetDataset(ds)
	mw.plotRight.SetDataset(ds)

	numericCols := ds.ColumnsOfKind(data.KindFloat)
	colorByCols := append([]string{colorByNone}, ds.Columns...)

	mw.xPickLeft.SetColumns(numericCols)
	mw.yPickLeft.SetColumns(numericCols)
	mw.xPickRight.SetColumns(numericCols)
	mw.yPickRight.SetColumns(numericCols)
	mw.colorPickLeft.SetColumns(colorByCols)
	mw.colorPickRight.SetColumns(colorByCols)
	mw.colorPickLeft.Select(colorByNone)
	mw.colorPickRight.Select(colorByNone)

	// Reasonable defaults: the first two markers on the left (classic
	// biaxial cytometry plot), UMAP_1/UMAP_2 on the right (dimensionality
	// reduction map) if present, otherwise the last two columns. Drawn
	// only from NUMERIC columns — a text column (cell type, cluster
	// name...) wouldn't make sense as a scatter axis, which is also why
	// the X/Y pickers above only offer numeric columns in the first place.
	if len(numericCols) >= 2 {
		mw.xPickLeft.Select(numericCols[0])
		mw.yPickLeft.Select(numericCols[1])
		mw.applyColumnsLeft()
	}
	xr, yr := guessMapColumns(numericCols)
	mw.xPickRight.Select(xr)
	mw.yPickRight.Select(yr)
	mw.applyColumnsRight()
}

