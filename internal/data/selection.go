package data

import (
	"image/color"
	"sync"
)

// Layer is one "stacked" selection: a set of row indices highlighted in a
// single color. Non-stacked selections are simply a Selection holding
// exactly one Layer.
type Layer struct {
	Indices []int32
	Color   color.NRGBA
}

// maxUndoHistory caps how many past selection states are kept. Each entry
// only stores slice headers + small Layer structs (the underlying index
// arrays are never copied, just re-referenced), so even a fairly deep
// history costs very little memory — this limit exists mainly so an
// unbounded work session doesn't grow the history forever, not because
// individual entries are expensive.
const maxUndoHistory = 50

type histEntry struct {
	layers []Layer
	source any
}

// Selection represents the set of currently selected cells (from a
// rectangular brush or lasso in either plot, or from a gate). It is shared
// by reference between the two ScatterPlot widgets: when one modifies the
// selection, the other is notified and redraws, highlighting the same row
// indices projected through its own axes.
//
// A Selection can hold several stacked Layers at once (see AddLayer): each
// call in "Stack" mode adds a new, independently colored population
// instead of replacing the previous one — handy for comparing several
// gates picked on different marker combinations without losing earlier
// ones.
//
// Every change is recorded in a small undo history (see Undo), so the last
// several selections can be stepped back through one at a time.
//
// Source identifies which widget produced the most recent layer (typically
// a *plot.ScatterPlot pointer, opaque to this package). This lets a plot
// widget tell "I made this selection" apart from "the other plot changed
// it", which is what allows each plot to keep (or clear) its own
// selection-outline overlay correctly.
type Selection struct {
	mu        sync.RWMutex
	layers    []Layer
	source    any
	history   []histEntry
	listeners []func()
}

// NewSelection creates an empty selection.
func NewSelection() *Selection {
	return &Selection{}
}

// SetFrom replaces the selection with a single layer, records who produced
// it, and notifies subscribers. This is the non-stacking path.
func (s *Selection) SetFrom(indices []int32, source any) {
	s.setLayers([]Layer{{Indices: indices}}, source)
}

// AddLayer appends a new, independently colored layer on top of whatever
// is already selected, and notifies subscribers. This is the stacking
// path (see ScatterPlot.StackMode).
func (s *Selection) AddLayer(indices []int32, c color.NRGBA, source any) {
	s.mu.Lock()
	s.pushHistory()
	layers := append(append([]Layer{}, s.layers...), Layer{Indices: indices, Color: c})
	s.layers = layers
	s.source = source
	listeners := append([]func(){}, s.listeners...)
	s.mu.Unlock()
	for _, fn := range listeners {
		fn()
	}
}

func (s *Selection) setLayers(layers []Layer, source any) {
	s.mu.Lock()
	s.pushHistory()
	s.layers = layers
	s.source = source
	listeners := append([]func(){}, s.listeners...)
	s.mu.Unlock()
	for _, fn := range listeners {
		fn()
	}
}

// pushHistory saves the current (about-to-be-replaced) state onto the undo
// stack. Callers must hold s.mu for writing.
func (s *Selection) pushHistory() {
	s.history = append(s.history, histEntry{layers: s.layers, source: s.source})
	if len(s.history) > maxUndoHistory {
		s.history = s.history[len(s.history)-maxUndoHistory:]
	}
}

// Undo reverts to the state before the most recent selection change (brush,
// lasso, stack layer, or clear) and notifies subscribers. Returns false
// (and does nothing) if there is nothing left to undo.
func (s *Selection) Undo() bool {
	s.mu.Lock()
	if len(s.history) == 0 {
		s.mu.Unlock()
		return false
	}
	last := s.history[len(s.history)-1]
	s.history = s.history[:len(s.history)-1]
	s.layers = last.layers
	s.source = last.source
	listeners := append([]func(){}, s.listeners...)
	s.mu.Unlock()
	for _, fn := range listeners {
		fn()
	}
	return true
}

// CanUndo reports whether Undo would currently do anything — useful to
// enable/disable an "Undo" button in the UI.
func (s *Selection) CanUndo() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.history) > 0
}

// Set replaces the selected set with no associated source (single layer).
func (s *Selection) Set(indices []int32) {
	s.SetFrom(indices, nil)
}

// Clear empties the selection, dropping every stacked layer. Like any
// other change, this can be undone.
func (s *Selection) Clear() {
	s.setLayers(nil, nil)
}

// Layers returns a copy of the current stacked layers (may be empty).
func (s *Selection) Layers() []Layer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Layer, len(s.layers))
	for i, l := range s.layers {
		idx := make([]int32, len(l.Indices))
		copy(idx, l.Indices)
		out[i] = Layer{Indices: idx, Color: l.Color}
	}
	return out
}

// LayerCount returns how many stacked layers currently exist.
func (s *Selection) LayerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.layers)
}

// Get returns the union of every selected index across all layers (a copy).
func (s *Selection) Get() []int32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []int32
	for _, l := range s.layers {
		out = append(out, l.Indices...)
	}
	return out
}

// Count returns the total number of selected cells across all layers.
// Cells selected in more than one overlapping layer are counted once per
// layer (an approximation — fine for the on-screen counter).
func (s *Selection) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, l := range s.layers {
		n += len(l.Indices)
	}
	return n
}

// Source returns whatever was passed for the most recent layer.
func (s *Selection) Source() any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.source
}

// Subscribe registers a function called on every selection change
// (typically: a ScatterPlot's redraw).
func (s *Selection) Subscribe(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, fn)
}
