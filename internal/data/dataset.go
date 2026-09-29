// Package data handles loading and in-memory storage of single-cell
// datasets (markers, UMAP/t-SNE coordinates, cluster id...).
//
// Design choices (important to hold millions of rows without blowing up
// memory):
//   - COLUMNAR storage: Data[col] is a []float32 of length N. 12 columns x
//     5 million rows x 4 bytes ~= 240 MB, which is very manageable. Storing
//     one Go struct per cell would cost much more due to struct padding
//     and GC pressure.
//   - Cell identifiers (the "id" column) are only kept in memory as
//     []string if requested (KeepIDs): this is the main avoidable memory
//     cost for callers who don't need to export selected IDs.
//   - Rows are counted up front, then filled into pre-sized slices (no
//     append-in-a-loop), avoiding repeated reallocation/copy for large
//     files.
package data

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

// ColumnKind distinguishes numeric columns (markers, coordinates...) from
// text/categorical ones (cell type, cluster name...).
type ColumnKind uint8

const (
	KindFloat  ColumnKind = iota // parsed as float32; may be used as X/Y axes or a "color by" gradient
	KindString                   // kept as text, encoded internally as a category index; "color by" only
)

// Dataset represents a single-cell table loaded in memory.
type Dataset struct {
	Columns  []string       // column names (excluding id), in file order
	Kinds    []ColumnKind   // parallel to Columns
	colIndex map[string]int // column name -> index into Data
	N        int            // number of rows (cells)
	Data     [][]float32    // Data[col][row], columnar storage.
	// For a KindString column, Data holds a 0-based CATEGORY INDEX per row
	// (first-seen order) rather than a numeric value — this lets X/Y/grid
	// code work unchanged even though String columns are never actually
	// offered as X/Y in the UI. CategoryLevels[col][index] recovers the
	// original text.
	CategoryLevels [][]string // parallel to Columns; nil for KindFloat columns
	IDs            []string   // optional, length N or nil
	Path           string
}

// ColumnIndex returns the index of a column by name.
func (d *Dataset) ColumnIndex(name string) (int, bool) {
	i, ok := d.colIndex[name]
	return i, ok
}

// Column returns the value slice for a column (by index).
func (d *Dataset) Column(idx int) []float32 {
	return d.Data[idx]
}

// ColumnsOfKind returns the names of every column of the given kind, in
// file order. Used to offer only numeric columns as X/Y axes (a String
// column's encoded category indices would make a technically-valid but
// meaningless scatter axis) while still offering every column, numeric or
// text, as a "color by" choice.
func (d *Dataset) ColumnsOfKind(k ColumnKind) []string {
	var out []string
	for i, c := range d.Columns {
		kind := KindFloat
		if i < len(d.Kinds) {
			kind = d.Kinds[i]
		}
		if kind == k {
			out = append(out, c)
		}
	}
	return out
}

// FormatValue renders the value of column col, row row as text: the
// original label for a KindString column, or a plain number for
// KindFloat. Used by CSV export so re-exported files show human-readable
// categories rather than their internal numeric encoding.
func (d *Dataset) FormatValue(col, row int) string {
	v := d.Data[col][row]
	if col < len(d.Kinds) && d.Kinds[col] == KindString {
		idx := int(v)
		if col < len(d.CategoryLevels) && idx >= 0 && idx < len(d.CategoryLevels[col]) {
			return d.CategoryLevels[col][idx]
		}
		return ""
	}
	return strconv.FormatFloat(float64(v), 'g', -1, 32)
}

// LoadOptions controls loading behaviour.
type LoadOptions struct {
	// KeepIDs: keep the "id" column in memory (needed to export selected /
	// gated cells later; costs roughly N*20 bytes).
	KeepIDs bool
	// Progress is called periodically with a 0..1 fraction. May be nil.
	Progress func(fraction float64)
}

// Load reads a delimited text file (tab, comma or semicolon, auto-detected)
// whose first column is a cell identifier ("id", "barcode"...) and whose
// following columns are numeric (markers, UMAP_1/UMAP_2, cluster...).
func Load(path string, opts LoadOptions) (*Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	totalSize := fi.Size()

	br := bufio.NewReaderSize(f, 1<<20)
	headerLine, err := br.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("reading header: %w", err)
	}
	headerLine = strings.TrimRight(headerLine, "\r\n")
	delim := detectDelimiter(headerLine)
	fields := strings.Split(headerLine, delim)
	if len(fields) < 2 {
		return nil, fmt.Errorf("invalid header (only one column detected, wrong delimiter?)")
	}
	columns := fields[1:] // the first column is assumed to be a cell id
	ncols := len(columns)

	// --- Pass 1: quick line count (so slices can be pre-allocated once,
	// with no reallocation while parsing). ---
	headerBytes := int64(len(headerLine)) + 1
	n, err := countLines(f, headerBytes)
	if err != nil {
		return nil, fmt.Errorf("counting lines: %w", err)
	}

	// Classify each column as numeric or text by peeking at the first
	// data row (br has already buffered ahead of the header; this read
	// doesn't disturb Pass 2 below, which re-seeks and re-reads from
	// scratch). A column is treated as text for the whole file as soon as
	// its first value fails to parse as a number — occasional NA/missing
	// values in an otherwise-numeric column still resolve to NaN as
	// before, they just can't be what decides the column's kind.
	kinds := make([]ColumnKind, ncols)
	if peekLine, _ := br.ReadString('\n'); peekLine != "" {
		peekFields := strings.Split(strings.TrimRight(peekLine, "\r\n"), delim)
		for c := 0; c < ncols; c++ {
			kinds[c] = KindString
			if c+1 < len(peekFields) {
				if _, perr := strconv.ParseFloat(strings.TrimSpace(peekFields[c+1]), 32); perr == nil {
					kinds[c] = KindFloat
				}
			}
		}
	}

	ds := &Dataset{
		Columns:  columns,
		Kinds:    kinds,
		colIndex: make(map[string]int, ncols),
		N:        n,
		Data:     make([][]float32, ncols),
		Path:     path,
	}
	for i, c := range columns {
		ds.colIndex[c] = i
	}
	for c := 0; c < ncols; c++ {
		ds.Data[c] = make([]float32, n)
	}
	if opts.KeepIDs {
		ds.IDs = make([]string, n)
	}

	// Per-column category encoding state (KindString columns only): the
	// index assigned to a text value is its first-seen order within that
	// column, which is what gets stored in ds.Data; ds.CategoryLevels
	// recovers the original text from that index.
	levelIndex := make([]map[string]int32, ncols)
	levels := make([][]string, ncols)
	for c := 0; c < ncols; c++ {
		if kinds[c] == KindString {
			levelIndex[c] = make(map[string]int32)
		}
	}

	// --- Pass 2: sequential re-read, filling the pre-sized slices. ---
	if _, err := f.Seek(headerBytes, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seeking past header: %w", err)
	}
	br = bufio.NewReaderSize(f, 1<<20)

	row := 0
	var readBytes int64 = headerBytes
	lastReported := -1.0
	for row < n {
		line, err := br.ReadString('\n')
		readBytes += int64(len(line))
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("reading line %d: %w", row, err)
			}
			continue // skip stray blank lines in the middle of the file
		}
		parts := strings.Split(line, delim)
		if len(parts) < ncols+1 {
			return nil, fmt.Errorf("line %d: expected %d columns, found %d", row+2, ncols+1, len(parts))
		}
		if opts.KeepIDs {
			ds.IDs[row] = parts[0]
		}
		for c := 0; c < ncols; c++ {
			raw := strings.TrimSpace(parts[c+1])
			if kinds[c] == KindString {
				idx, ok := levelIndex[c][raw]
				if !ok {
					idx = int32(len(levels[c]))
					levels[c] = append(levels[c], raw)
					levelIndex[c][raw] = idx
				}
				ds.Data[c][row] = float32(idx)
				continue
			}
			v, perr := strconv.ParseFloat(raw, 32)
			if perr != nil {
				// Missing/NA value -> NaN rather than failing the whole load.
				v = float64(nan32())
			}
			ds.Data[c][row] = float32(v)
		}
		row++

		if opts.Progress != nil {
			frac := float64(readBytes) / float64(totalSize)
			if frac-lastReported > 0.01 { // throttle callback frequency
				opts.Progress(frac)
				lastReported = frac
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading line %d: %w", row, err)
		}
	}
	ds.N = row
	for c := 0; c < ncols; c++ {
		ds.Data[c] = ds.Data[c][:row]
	}
	ds.CategoryLevels = levels
	if opts.KeepIDs {
		ds.IDs = ds.IDs[:row]
	}
	if opts.Progress != nil {
		opts.Progress(1.0)
	}
	return ds, nil
}

// countLines counts the number of remaining non-empty lines in f starting
// at the given offset, without loading them into memory (block reads).
func countLines(f *os.File, from int64) (int, error) {
	if _, err := f.Seek(from, io.SeekStart); err != nil {
		return 0, err
	}
	buf := make([]byte, 1<<20)
	count := 0
	lastByte := byte('\n')
	for {
		n, err := f.Read(buf)
		if n > 0 {
			for _, b := range buf[:n] {
				if b == '\n' {
					count++
				}
			}
			lastByte = buf[n-1]
		}
		if err == io.EOF {
			if n > 0 && lastByte != '\n' {
				count++ // last line with no trailing newline
			}
			break
		}
		if err != nil {
			return 0, err
		}
	}
	if _, err := f.Seek(from, io.SeekStart); err != nil {
		return 0, err
	}
	return count, nil
}

// detectDelimiter picks between tab, semicolon and comma based on which
// appears most often in the header line. The sample files shipped with
// Single-Cell Virtual Cytometer are actually tab-separated despite the
// ".csv" extension.
func detectDelimiter(header string) string {
	tabs := strings.Count(header, "\t")
	semis := strings.Count(header, ";")
	commas := strings.Count(header, ",")
	switch {
	case tabs >= semis && tabs >= commas && tabs > 0:
		return "\t"
	case semis >= commas && semis > 0:
		return ";"
	default:
		return ","
	}
}

// WriteCSV writes a tab-separated table (id + all columns) for the given
// row indices, sorted back into their original file order so the export
// is reproducible regardless of the order cells were visited in during
// selection.
func (d *Dataset) WriteCSV(w io.Writer, indices []int32) error {
	sorted := append([]int32(nil), indices...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	bw := bufio.NewWriterSize(w, 1<<20)

	bw.WriteString("id")
	for _, c := range d.Columns {
		bw.WriteByte('\t')
		bw.WriteString(c)
	}
	bw.WriteByte('\n')

	for _, idx := range sorted {
		if int(idx) < 0 || int(idx) >= d.N {
			continue
		}
		id := ""
		if int(idx) < len(d.IDs) {
			id = d.IDs[idx]
		}
		bw.WriteString(id)
		for c := range d.Columns {
			bw.WriteByte('\t')
			bw.WriteString(d.FormatValue(c, int(idx)))
		}
		bw.WriteByte('\n')
	}
	return bw.Flush()
}

// WriteQuadrantCSV writes every row of the dataset tagged with which
// quadrant it falls into relative to a crosshair at (threshX, threshY) on
// columns (xCol, yCol) — Q1/Q2/Q3/Q4 for top-left/top-right/bottom-left/
// bottom-right, matching the on-screen quadrant coloring. The quadrant
// label is the first column, followed by the cell id and then every data
// column. A handful of "#"-prefixed comment lines up front record the
// gate definition and the count/percentage of cells in each quadrant.
//
// This always covers the WHOLE dataset (not just the current selection,
// and not filtered by "Gate only"), so the four quadrant counts in the
// header always add up to the total cell count.
func (d *Dataset) WriteQuadrantCSV(w io.Writer, xCol, yCol string, threshX, threshY float64) error {
	xi, ok1 := d.ColumnIndex(xCol)
	yi, ok2 := d.ColumnIndex(yCol)
	if !ok1 || !ok2 {
		return fmt.Errorf("quadrant export: unknown column(s) %q / %q", xCol, yCol)
	}
	xArr, yArr := d.Data[xi], d.Data[yi]

	quadOf := make([]string, d.N)
	var nQ1, nQ2, nQ3, nQ4 int
	for i := 0; i < d.N; i++ {
		left := xArr[i] < float32(threshX)
		top := yArr[i] >= float32(threshY)
		switch {
		case left && top:
			quadOf[i] = "Q1"
			nQ1++
		case !left && top:
			quadOf[i] = "Q2"
			nQ2++
		case left && !top:
			quadOf[i] = "Q3"
			nQ3++
		default:
			quadOf[i] = "Q4"
			nQ4++
		}
	}
	pct := func(n int) float64 {
		if d.N == 0 {
			return 0
		}
		return 100 * float64(n) / float64(d.N)
	}

	bw := bufio.NewWriterSize(w, 1<<20)
	fmt.Fprintf(bw, "# Quadrant gate: X=%s (threshold %.6g), Y=%s (threshold %.6g)\n", xCol, threshX, yCol, threshY)
	fmt.Fprintf(bw, "# Q1 (%s < thr, %s >= thr, top-left):     %d cells (%.1f%%)\n", xCol, yCol, nQ1, pct(nQ1))
	fmt.Fprintf(bw, "# Q2 (%s >= thr, %s >= thr, top-right):   %d cells (%.1f%%)\n", xCol, yCol, nQ2, pct(nQ2))
	fmt.Fprintf(bw, "# Q3 (%s < thr, %s < thr, bottom-left):   %d cells (%.1f%%)\n", xCol, yCol, nQ3, pct(nQ3))
	fmt.Fprintf(bw, "# Q4 (%s >= thr, %s < thr, bottom-right): %d cells (%.1f%%)\n", xCol, yCol, nQ4, pct(nQ4))
	fmt.Fprintf(bw, "# total: %d cells\n", d.N)

	bw.WriteString("quadrant\tid")
	for _, c := range d.Columns {
		bw.WriteByte('\t')
		bw.WriteString(c)
	}
	bw.WriteByte('\n')

	for i := 0; i < d.N; i++ {
		bw.WriteString(quadOf[i])
		bw.WriteByte('\t')
		if i < len(d.IDs) {
			bw.WriteString(d.IDs[i])
		}
		for c := range d.Columns {
			bw.WriteByte('\t')
			bw.WriteString(d.FormatValue(c, i))
		}
		bw.WriteByte('\n')
	}
	return bw.Flush()
}

// ParseIDList reads a list of cell identifiers from r: one id per line,
// optionally with a header row and/or extra tab/comma-separated columns
// after the id (only the first field of each line is used) — this covers
// both a plain one-id-per-line file and files produced by this app's own
// "Export selection (CSV)" (which starts with an "id" header column).
// Blank lines and "#"-prefixed comment lines are ignored.
func ParseIDList(r io.Reader) ([]string, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<16), 1<<24)
	var ids []string
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		field := line
		if i := strings.IndexAny(line, "\t,"); i >= 0 {
			field = line[:i]
		}
		field = strings.TrimSpace(field)
		if first {
			first = false
			if strings.EqualFold(field, "id") {
				continue // header row, e.g. from our own CSV export
			}
		}
		if field != "" {
			ids = append(ids, field)
		}
	}
	return ids, sc.Err()
}

// IndicesForIDs looks up row indices for a list of cell identifiers
// against this dataset's IDs (only meaningful if it was loaded with
// LoadOptions.KeepIDs). Returns the matched row indices and a count of
// how many input ids had no match.
func (d *Dataset) IndicesForIDs(ids []string) (found []int32, missing int) {
	byID := make(map[string]int32, len(d.IDs))
	for i, id := range d.IDs {
		byID[id] = int32(i)
	}
	for _, raw := range ids {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if idx, ok := byID[name]; ok {
			found = append(found, idx)
		} else {
			missing++
		}
	}
	return found, missing
}

func nan32() float32 {
	var f float32
	return f / f // NaN, without pulling in math for such a small need
}
