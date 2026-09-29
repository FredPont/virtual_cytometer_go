package data

import "sync"

// QuadrantState broadcasts the currently active quadrant crosshair (from
// whichever plot the user placed it on) so the OTHER, linked plot can
// color its own points by quadrant membership too — matching the original
// app, where selecting quadrants on the density plot also colors the
// corresponding populations on the map.
//
// Only the crosshair itself (which columns, which thresholds, who set it)
// is shared; each plot keeps and draws its own crosshair line and
// percentage labels locally, in its own axes — those wouldn't make sense
// projected onto a plot with different X/Y columns. What DOES carry over
// cleanly is "which quadrant does row i belong to", since that only
// depends on the two source columns' values for that row, not on which
// columns the other plot happens to be displaying.
type QuadrantState struct {
	mu     sync.RWMutex
	active bool
	xCol   string
	yCol   string
	x, y   float64
	source any

	listeners []func()
}

// NewQuadrantState creates an inactive shared quadrant state.
func NewQuadrantState() *QuadrantState {
	return &QuadrantState{}
}

// Set marks the quadrant crosshair as active, recording which columns and
// thresholds define it and who placed it, then notifies subscribers.
func (q *QuadrantState) Set(xCol, yCol string, x, y float64, source any) {
	q.mu.Lock()
	q.active = true
	q.xCol, q.yCol = xCol, yCol
	q.x, q.y = x, y
	q.source = source
	listeners := append([]func(){}, q.listeners...)
	q.mu.Unlock()
	for _, fn := range listeners {
		fn()
	}
}

// Clear deactivates the shared crosshair, but only if it currently belongs
// to source (so turning off quadrant mode on a plot that doesn't own the
// active crosshair does nothing). Pass nil to force-clear unconditionally.
func (q *QuadrantState) Clear(source any) {
	q.mu.Lock()
	if source != nil && q.source != source {
		q.mu.Unlock()
		return
	}
	q.active = false
	listeners := append([]func(){}, q.listeners...)
	q.mu.Unlock()
	for _, fn := range listeners {
		fn()
	}
}

// Get returns the current shared crosshair state.
func (q *QuadrantState) Get() (active bool, xCol, yCol string, x, y float64, source any) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.active, q.xCol, q.yCol, q.x, q.y, q.source
}

// Subscribe registers a function called whenever the shared crosshair
// changes (typically: the other plot's redraw).
func (q *QuadrantState) Subscribe(fn func()) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.listeners = append(q.listeners, fn)
}
