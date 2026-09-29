package ui

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"scvirtualcytometer/internal/data"
	"scvirtualcytometer/internal/plot"
)


// MainWindow assembles the full application UI.
type MainWindow struct {
	win fyne.Window

	ds  *data.Dataset
	sel  *data.Selection
	quad *data.QuadrantState

	plotLeft  *plot.ScatterPlot
	plotRight *plot.ScatterPlot

	xPickLeft, yPickLeft   *ColumnPicker
	xPickRight, yPickRight *ColumnPicker
	colorPickLeft, colorPickRight *ColumnPicker

	fileLabel     *widget.Label
	progress      *widget.ProgressBar
	selCountLabel *widget.Label
	openBtn       *widget.Button
	undoBtn       *widget.Button

	exportFormat string // "PNG" or "SVG"
}

// NewMainWindow builds and returns the main window, ready for
// win.ShowAndRun().
func NewMainWindow(win fyne.Window) *MainWindow {
	mw := &MainWindow{
		win: win,
		sel:  data.NewSelection(),
		quad: data.NewQuadrantState(),
	}

	mw.plotLeft = plot.NewScatterPlot(nil, mw.sel, mw.quad)
	mw.plotRight = plot.NewScatterPlot(nil, mw.sel, mw.quad)

	mw.fileLabel = widget.NewLabel("No file loaded — Single-Cell Virtual Cytometer")
	mw.fileLabel.Wrapping = fyne.TextWrapWord
	mw.progress = widget.NewProgressBar()
	mw.progress.Hide()
	mw.selCountLabel = widget.NewLabel("Selection: 0 cell(s)")

	mw.sel.Subscribe(func() {
		fyne.Do(func() {
			mw.selCountLabel.SetText(fmt.Sprintf("Selection: %d cell(s)", mw.sel.Count()))
			if mw.sel.CanUndo() {
				mw.undoBtn.Enable()
			} else {
				mw.undoBtn.Disable()
			}
		})
	})

	mw.xPickLeft = NewColumnPicker(win, "X (left)", nil, func(string) { mw.applyColumnsLeft() })
	mw.yPickLeft = NewColumnPicker(win, "Y (left)", nil, func(string) { mw.applyColumnsLeft() })
	mw.xPickRight = NewColumnPicker(win, "X (right)", nil, func(string) { mw.applyColumnsRight() })
	mw.yPickRight = NewColumnPicker(win, "Y (right)", nil, func(string) { mw.applyColumnsRight() })
	mw.colorPickLeft = NewColumnPicker(win, "Color by (left)", nil, func(name string) { mw.applyColorByLeft(name) })
	mw.colorPickRight = NewColumnPicker(win, "Color by (right)", nil, func(name string) { mw.applyColorByRight(name) })

	mw.openBtn = widget.NewButtonWithIcon("Open file…", theme.FolderOpenIcon(), mw.openFile)

	importBtn := widget.NewButton("Overlay cells from file…", mw.importCells)

	// --- Background: affects both the live plots and the default export
	// background, so what you see is what you get. ---
	bgRadio := widget.NewRadioGroup([]string{"White background", "Black background"}, func(s string) {
		bg := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		if s == "Black background" {
			bg = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
		}
		mw.plotLeft.SetBackground(bg)
		mw.plotRight.SetBackground(bg)
	})
	bgRadio.Horizontal = true
	bgRadio.SetSelected("White background")

	// --- Left panel controls ---
	resetLeftBtn := widget.NewButton("Reset zoom", func() { mw.plotLeft.ZoomToFull() })
	toolLeft := widget.NewRadioGroup([]string{"Rectangle", "Lasso", "Pan"}, func(s string) {
		mw.plotLeft.SetTool(toolFromLabel(s))
	})
	toolLeft.Horizontal = true
	toolLeft.SetSelected("Rectangle")
	quadLeft := widget.NewCheck("Quadrants", func(v bool) { mw.plotLeft.SetQuadrantsEnabled(v) })
	densityLeft := widget.NewCheck("Plot density", func(v bool) { mw.plotLeft.SetShowDensity(v) })
	densityLeft.SetChecked(true)
	classesLeft := widget.NewCheck("Color by classes", func(v bool) { mw.plotLeft.SetColorByClasses(v) })
	gradientLeft := widget.NewSelect(plot.GradientNames, func(name string) { mw.plotLeft.SetColorByGradient(name) })
	gradientLeft.SetSelected("Turbo")
	colorMinLeft, colorMaxLeft := 0.0, 100.0
	colorMinSliderLeft := widget.NewSlider(0, 100)
	colorMaxSliderLeft := widget.NewSlider(0, 100)
	colorMaxSliderLeft.SetValue(100)
	colorMinSliderLeft.OnChanged = func(v float64) {
		colorMinLeft = v
		mw.plotLeft.SetColorByRange(colorMinLeft, colorMaxLeft)
	}
	colorMaxSliderLeft.OnChanged = func(v float64) {
		colorMaxLeft = v
		mw.plotLeft.SetColorByRange(colorMinLeft, colorMaxLeft)
	}
	dotLeft := widget.NewSlider(0, 6)
	dotLeft.OnChanged = func(v float64) { mw.plotLeft.SetDotRadius(int(v)) }
	dotLeft.SetValue(1)

	// --- Right panel controls ---
	resetRightBtn := widget.NewButton("Reset zoom", func() { mw.plotRight.ZoomToFull() })
	toolRight := widget.NewRadioGroup([]string{"Rectangle", "Lasso", "Pan"}, func(s string) {
		mw.plotRight.SetTool(toolFromLabel(s))
	})
	toolRight.Horizontal = true
	toolRight.SetSelected("Rectangle")
	quadRight := widget.NewCheck("Quadrants", func(v bool) { mw.plotRight.SetQuadrantsEnabled(v) })
	densityRight := widget.NewCheck("Plot density", func(v bool) { mw.plotRight.SetShowDensity(v) })
	densityRight.SetChecked(true)
	classesRight := widget.NewCheck("Color by classes", func(v bool) { mw.plotRight.SetColorByClasses(v) })
	gradientRight := widget.NewSelect(plot.GradientNames, func(name string) { mw.plotRight.SetColorByGradient(name) })
	gradientRight.SetSelected("Turbo")
	colorMinRight, colorMaxRight := 0.0, 100.0
	colorMinSliderRight := widget.NewSlider(0, 100)
	colorMaxSliderRight := widget.NewSlider(0, 100)
	colorMaxSliderRight.SetValue(100)
	colorMinSliderRight.OnChanged = func(v float64) {
		colorMinRight = v
		mw.plotRight.SetColorByRange(colorMinRight, colorMaxRight)
	}
	colorMaxSliderRight.OnChanged = func(v float64) {
		colorMaxRight = v
		mw.plotRight.SetColorByRange(colorMinRight, colorMaxRight)
	}
	dotRight := widget.NewSlider(0, 6)
	dotRight.OnChanged = func(v float64) { mw.plotRight.SetDotRadius(int(v)) }
	dotRight.SetValue(1)

	// --- Shared gating controls (apply to both linked plots) ---
	freezeGate := widget.NewCheck("Freeze gate (sequential gating)", func(v bool) {
		mw.plotLeft.SetFreezeGate(v)
		mw.plotRight.SetFreezeGate(v)
	})
	gateOnly := widget.NewCheck("Gate only (hide ungated cells — display only, pair with Freeze gate to narrow)", func(v bool) {
		mw.plotLeft.SetGateOnly(v)
		mw.plotRight.SetGateOnly(v)
	})
	stackMode := widget.NewCheck("Stack (keep past selections, new color each time)", func(v bool) {
		mw.plotLeft.SetStackMode(v)
		mw.plotRight.SetStackMode(v)
	})
	clearSelBtn := widget.NewButton("Clear selection", func() { mw.sel.Clear() })

	mw.undoBtn = widget.NewButton("Undo selection", func() {
		// Drop each plot's own dashed-outline memory first: Undo can jump
		// back to a state from several selections ago, and we have no
		// record of what shape (rectangle or lasso, and where) produced
		// it, so the honest thing is to stop showing an outline rather
		// than show a wrong one. The highlighted cells themselves are
		// always restored correctly regardless.
		mw.plotLeft.ClearOutline()
		mw.plotRight.ClearOutline()
		mw.sel.Undo()
	})
	mw.undoBtn.Disable()

	// --- Export ---
	mw.exportFormat = "PNG"
	formatSelect := widget.NewSelect([]string{"PNG", "SVG"}, func(s string) { mw.exportFormat = s })
	formatSelect.SetSelected("PNG")

	exportSelBtn := widget.NewButton("Export selection (CSV)…", mw.exportSelection)
	exportQuadBtn := widget.NewButton("Export quadrants (CSV)…", mw.exportQuadrants)
	exportStatsBtn := widget.NewButton("Export gate statistics (CSV)…", mw.exportStatistics)
	exportLeftBtn := widget.NewButton("Export left image…", func() { mw.exportImage(mw.plotLeft, "left") })
	exportRightBtn := widget.NewButton("Export right image…", func() { mw.exportImage(mw.plotRight, "right") })

	exportRow := container.NewHBox(
		exportSelBtn, exportQuadBtn, exportStatsBtn,
		widget.NewLabel("Image format:"), formatSelect,
		exportLeftBtn, exportRightBtn,
	)

	quitBtn := widget.NewButton("Quit", func() { fyne.CurrentApp().Quit() })

	// Two rows per panel — column pickers on their own row, everything
	// else below — and each row wrapped in a horizontal scroller. Column
	// names (markers, fluorophore/antibody combos...) can be long and
	// unpredictable; without the scroller, a single wide row of picker
	// buttons doubled by the two side-by-side panels could force the
	// whole window wider than the screen and, since Fyne never lets a
	// window shrink below its computed minimum size, stuck that way.
	// Wrapping in HScroll keeps each row's minimum size small regardless
	// of what it contains — excess controls just scroll instead.
	leftColumnsRow := container.NewHScroll(container.NewHBox(
		mw.xPickLeft.Widget(), mw.yPickLeft.Widget(), mw.colorPickLeft.Widget(), classesLeft,
		widget.NewLabel("Gradient:"), gradientLeft,
		widget.NewLabel("Min%:"), colorMinSliderLeft, widget.NewLabel("Max%:"), colorMaxSliderLeft,
	))
	leftToolsRow := container.NewHScroll(container.NewHBox(
		widget.NewLabel("Dot size:"), dotLeft, toolLeft, quadLeft, densityLeft, resetLeftBtn,
	))
	rightColumnsRow := container.NewHScroll(container.NewHBox(
		mw.xPickRight.Widget(), mw.yPickRight.Widget(), mw.colorPickRight.Widget(), classesRight,
		widget.NewLabel("Gradient:"), gradientRight,
		widget.NewLabel("Min%:"), colorMinSliderRight, widget.NewLabel("Max%:"), colorMaxSliderRight,
	))
	rightToolsRow := container.NewHScroll(container.NewHBox(
		widget.NewLabel("Dot size:"), dotRight, toolRight, quadRight, densityRight, resetRightBtn,
	))
	leftControls := container.NewVBox(leftColumnsRow, leftToolsRow)
	rightControls := container.NewVBox(rightColumnsRow, rightToolsRow)

	leftPanel := container.NewBorder(leftControls, nil, nil, nil, mw.plotLeft)
	rightPanel := container.NewBorder(rightControls, nil, nil, nil, mw.plotRight)

	plots := container.New(layout.NewGridLayout(2), leftPanel, rightPanel)

	top := container.NewVBox(
		container.NewBorder(nil, nil, mw.openBtn, quitBtn, mw.fileLabel),
		container.NewHBox(widget.NewLabel("Background:"), bgRadio, layout.NewSpacer(), importBtn),
		mw.progress,
	)
	bottom := container.NewVBox(
		container.NewHScroll(container.NewHBox(
			mw.selCountLabel, layout.NewSpacer(),
			freezeGate, gateOnly, stackMode,
			widget.NewSeparator(),
			mw.undoBtn, clearSelBtn,
		)),
		container.NewHScroll(exportRow),
	)

	win.SetContent(container.NewBorder(top, bottom, nil, nil, plots))
	win.Resize(fyne.NewSize(1200, 800))
	return mw
}

