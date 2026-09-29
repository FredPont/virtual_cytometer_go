package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// ColumnPicker replaces the original app's HTML <select> elements (and
// Fyne's native widget.Select) with a button that opens a popup containing
// a search field + a widget.Table.
//
// Why not widget.Select: it loads ALL options into the popup at once (no
// virtualization), its width doesn't account for the option text (only the
// PlaceHolder, see fyne-io/fyne#1247), and opening it gets noticeably
// slower past a hundred or so entries (fyne-io/fyne#3499). A single-cell
// dataset often has 15-40+ markers/columns: still fine with a Select, but
// a widget.Table (virtualized viewport, like widget.List) is more robust,
// supports search, and scales cleanly to hundreds of columns if needed.
type ColumnPicker struct {
	button   *widget.Button
	label    string
	columns  []string
	filtered []string
	selected string

	OnSelected func(name string)
}

// NewColumnPicker creates a picker for the given list of columns. win is
// the window in which to show the popup.
func NewColumnPicker(win fyne.Window, label string, columns []string, onSelected func(string)) *ColumnPicker {
	cp := &ColumnPicker{
		label:      label,
		columns:    columns,
		filtered:   columns,
		OnSelected: onSelected,
	}
	cp.button = widget.NewButton(label+": —", func() { cp.open(win) })
	return cp
}

// Widget returns the Fyne object to place in the layout.
func (cp *ColumnPicker) Widget() fyne.CanvasObject { return cp.button }

// SetColumns replaces the list of proposed columns (e.g. after loading a
// new file).
func (cp *ColumnPicker) SetColumns(columns []string) {
	cp.columns = columns
	cp.filtered = columns
}

// Select sets the current selection without opening the popup (e.g. a
// default value right after loading a file).
func (cp *ColumnPicker) Select(name string) {
	cp.selected = name
	cp.button.SetText(cp.label + ": " + name)
}

func (cp *ColumnPicker) open(win fyne.Window) {
	search := widget.NewEntry()
	search.SetPlaceHolder("Search a column…")

	table := widget.NewTable(
		func() (int, int) { return len(cp.filtered), 1 },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.TableCellID, o fyne.CanvasObject) {
			lbl := o.(*widget.Label)
			if id.Row < len(cp.filtered) {
				lbl.SetText(cp.filtered[id.Row])
			}
		},
	)
	table.SetColumnWidth(0, 280)

	content := container.NewBorder(search, nil, nil, nil, table)

	pop := widget.NewModalPopUp(content, win.Canvas())
	pop.Resize(fyne.NewSize(320, 420))

	table.OnSelected = func(id widget.TableCellID) {
		if id.Row >= len(cp.filtered) {
			return
		}
		name := cp.filtered[id.Row]
		cp.Select(name)
		if cp.OnSelected != nil {
			cp.OnSelected(name)
		}
		pop.Hide()
	}

	search.OnChanged = func(s string) {
		cp.filtered = filterColumns(cp.columns, s)
		table.Refresh()
	}

	pop.Show()
	// Note: we deliberately do NOT auto-focus the search entry here. A
	// modal popup is properly registered in the canvas' overlay stack as
	// soon as Show() returns, which is what a plain (non-modal), manually
	// positioned popup did not reliably guarantee — that mismatch was the
	// source of the "Failed to focus object..." errors. The search field
	// can still be focused with a normal click.
}

func filterColumns(all []string, query string) []string {
	if query == "" {
		return all
	}
	q := strings.ToLower(query)
	var out []string
	for _, c := range all {
		if strings.Contains(strings.ToLower(c), q) {
			out = append(out, c)
		}
	}
	return out
}
