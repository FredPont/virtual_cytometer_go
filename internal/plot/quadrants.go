package plot

// Quadrant-gate mode: click to place a crosshair, see live counts and
// percentages in each of the four quadrants.

import (
	"fmt"

	"fyne.io/fyne/v2"
)

func (p *ScatterPlot) quadrantCounts() (tl, tr, bl, br int) {
	xArr := p.ds.Column(p.xIdx)
	yArr := p.ds.Column(p.yIdx)
	xlo, xhi := p.grid.Xmin-1, p.grid.Xmax+1
	ylo, yhi := p.grid.Ymin-1, p.grid.Ymax+1

	count := func(x0, x1, y0, y1 float64) int {
		if p.GateOnly {
			n := 0
			for _, idx := range p.sel.Get() {
				if int(idx) >= len(xArr) {
					continue
				}
				x, y := float64(xArr[idx]), float64(yArr[idx])
				if x >= x0 && x < x1 && y >= y0 && y < y1 {
					n++
				}
			}
			return n
		}
		n := 0
		p.grid.ForEachInRect(xArr, yArr, x0, x1, y0, y1, func(int32) { n++ })
		return n
	}
	tl = count(xlo, p.quadX, p.quadY, yhi)
	tr = count(p.quadX, xhi, p.quadY, yhi)
	bl = count(xlo, p.quadX, ylo, p.quadY)
	br = count(p.quadX, xhi, ylo, p.quadY)
	return
}

func (p *ScatterPlot) updateQuadrantOverlay(w, h int) {
	if !p.quadrantsOn || !p.hasQuadrant || p.grid == nil {
		p.qLineV.Hide()
		p.qLineH.Hide()
		p.qLabelTL.Hide()
		p.qLabelTR.Hide()
		p.qLabelBL.Hide()
		p.qLabelBR.Hide()
		return
	}
	view := p.view
	if p.quadX < view.Xmin || p.quadX > view.Xmax || p.quadY < view.Ymin || p.quadY > view.Ymax {
		p.qLineV.Hide()
		p.qLineH.Hide()
		p.qLabelTL.Hide()
		p.qLabelTR.Hide()
		p.qLabelBL.Hide()
		p.qLabelBR.Hide()
		return
	}
	x0, y0, pw, ph := p.computePlotRect(w, h, 1)
	pos := p.dataToPx(p.quadX, p.quadY, w, h, 1)
	px, py := float64(pos.X), float64(pos.Y)
	fg := contrastColor(p.Background)

	p.qLineV.Position1 = fyne.NewPos(float32(px), float32(y0))
	p.qLineV.Position2 = fyne.NewPos(float32(px), float32(y0+ph))
	p.qLineV.StrokeColor = fg
	p.qLineV.Show()
	p.qLineV.Refresh()

	p.qLineH.Position1 = fyne.NewPos(float32(x0), float32(py))
	p.qLineH.Position2 = fyne.NewPos(float32(x0+pw), float32(py))
	p.qLineH.StrokeColor = fg
	p.qLineH.Show()
	p.qLineH.Refresh()

	tl, tr, bl, br := p.quadrantCounts()
	total := tl + tr + bl + br
	pct := func(n int) float64 {
		if total == 0 {
			return 0
		}
		return 100 * float64(n) / float64(total)
	}
	p.qLabelTL.Text = fmt.Sprintf("%d (%.1f%%)", tl, pct(tl))
	p.qLabelTR.Text = fmt.Sprintf("%d (%.1f%%)", tr, pct(tr))
	p.qLabelBL.Text = fmt.Sprintf("%d (%.1f%%)", bl, pct(bl))
	p.qLabelBR.Text = fmt.Sprintf("%d (%.1f%%)", br, pct(br))
	p.qLabelTL.Color = fg
	p.qLabelTR.Color = fg
	p.qLabelBL.Color = fg
	p.qLabelBR.Color = fg

	p.qLabelTL.Move(fyne.NewPos(float32(x0)+4, float32(y0)+4))
	p.qLabelTR.Move(fyne.NewPos(float32(x0+pw)-80, float32(y0)+4))
	p.qLabelBL.Move(fyne.NewPos(float32(x0)+4, float32(y0+ph)-20))
	p.qLabelBR.Move(fyne.NewPos(float32(x0+pw)-80, float32(y0+ph)-20))
	p.qLabelTL.Show()
	p.qLabelTR.Show()
	p.qLabelBL.Show()
	p.qLabelBR.Show()
	p.qLabelTL.Refresh()
	p.qLabelTR.Refresh()
	p.qLabelBL.Refresh()
	p.qLabelBR.Refresh()
}

// --- click-to-place crosshair ---

func (p *ScatterPlot) setQuadrantAt(pos fyne.Position) {
	p.quadX, p.quadY = p.pxToData(pos)
	p.hasQuadrant = true
	if p.quad != nil {
		// Broadcast so the linked plot can color its own points by
		// quadrant membership too (using these same thresholds against
		// THIS plot's X/Y columns, regardless of what the other plot
		// displays on its own axes).
		p.quad.Set(p.XName, p.YName, p.quadX, p.quadY, p)
	}
	p.redraw(p.Size())
}
