package ui

// Small, stateless helpers used while building the UI in mainwindow.go.

import "scvirtualcytometer/internal/plot"

// toolFromLabel maps a radio-button label to the corresponding plot tool.
func toolFromLabel(s string) plot.SelectTool {
	switch s {
	case "Lasso":
		return plot.ToolLasso
	case "Pan":
		return plot.ToolPan
	default:
		return plot.ToolRect
	}
}

// guessMapColumns tries to spot a UMAP_1/UMAP_2 or tSNE_1/tSNE_2 pair; if
// none is found, it falls back to the last two columns of the file (the
// convention typically used by Single-Cell Signature Explorer / Seurat for
// dimensionality-reduction coordinates).
func guessMapColumns(columns []string) (string, string) {
	candidates := [][2]string{
		{"UMAP_1", "UMAP_2"}, {"UMAP1", "UMAP2"},
		{"tSNE_1", "tSNE_2"}, {"tSNE1", "tSNE2"},
	}
	set := make(map[string]bool, len(columns))
	for _, c := range columns {
		set[c] = true
	}
	for _, cand := range candidates {
		if set[cand[0]] && set[cand[1]] {
			return cand[0], cand[1]
		}
	}
	n := len(columns)
	if n >= 2 {
		return columns[n-2], columns[n-1]
	}
	if n == 1 {
		return columns[0], columns[0]
	}
	return "", ""
}
