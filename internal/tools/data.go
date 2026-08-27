// Package tools — dataAnalysis: a pure-Go data-analysis engine for the agent.
//
// Loads CSV/TSV/JSON datasets with type inference, then profiles, computes
// descriptive statistics, correlations, group-by aggregations, filtering,
// sorting, histograms, format conversion, and renders publication-quality
// fire-themed SVG charts. No external dependencies — everything is stdlib.
//
// Interoperability: dataset paths resolve through ResolvePath (app folder),
// so a CSV written with the `files` tool is immediately analyzable with the
// same relative path.
package tools

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// --- dataset model ---

type colType string

const (
	typeNumber colType = "number"
	typeString colType = "string"
	typeBool   colType = "bool"
)

var missingTokens = map[string]bool{
	"": true, "-": true, "NA": true, "N/A": true, "na": true,
	"null": true, "NULL": true, "Null": true, "NaN": true, "nan": true,
	"none": true, "None": true,
}

type dataset struct {
	Path    string // absolute path the data was loaded from
	Columns []string
	Types   []colType
	Rows    [][]string // raw cells, aligned with Columns
}

// numeric extracts the float value of a cell (NaN when missing/non-numeric).
func (d *dataset) numeric(row int, col int) float64 {
	v := parseNumber(d.Rows[row][col])
	return v
}

func parseNumber(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	if s == "" {
		return math.NaN()
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return math.NaN()
	}
	return f
}

func isMissing(s string) bool { return missingTokens[strings.TrimSpace(s)] }

func (d *dataset) colIndex(name string) (int, error) {
	for i, c := range d.Columns {
		if strings.EqualFold(c, name) {
			return i, nil
		}
	}
	return -1, fmt.Errorf("unknown column %q (available: %s)", name, strings.Join(d.Columns, ", "))
}

// numericCols returns the indexes of all numeric columns.
func (d *dataset) numericCols() []int {
	var out []int
	for i, t := range d.Types {
		if t == typeNumber {
			out = append(out, i)
		}
	}
	return out
}

// --- dataset cache (keyed by path+mtime+size so chained calls don't reparse) ---

type cacheKey struct {
	path  string
	mtime int64
	size  int64
}

var (
	dsMu    sync.Mutex
	dsCache = map[cacheKey]*dataset{}
)

func (t *DataTool) load(path string) (*dataset, error) {
	abs := ResolvePath(path)
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("dataset not found: %s (relative paths resolve against the app folder)", abs)
	}
	if fi.Size() > 64<<20 {
		return nil, fmt.Errorf("dataset too large (>%d MB)", 64)
	}
	key := cacheKey{path: abs, mtime: fi.ModTime().UnixNano(), size: fi.Size()}
	dsMu.Lock()
	if ds, ok := dsCache[key]; ok {
		dsMu.Unlock()
		return ds, nil
	}
	dsMu.Unlock()

	ext := strings.ToLower(filepath.Ext(abs))
	var ds *dataset
	switch ext {
	case ".json", ".jsonl":
		ds, err = loadJSON(abs)
	default: // .csv, .tsv, .txt → delimiter sniffing
		ds, err = loadDelimited(abs)
	}
	if err != nil {
		return nil, err
	}
	ds.Path = abs

	dsMu.Lock()
	if len(dsCache) > 16 { // bounded cache
		dsCache = map[cacheKey]*dataset{}
	}
	dsCache[key] = ds
	dsMu.Unlock()
	return ds, nil
}

// --- loaders ---

func loadDelimited(path string) (*dataset, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.TrimPrefix(string(raw), "\uFEFF") // strip BOM
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := splitLines(text)
	if len(lines) == 0 {
		return nil, fmt.Errorf("file is empty")
	}

	delim := "\t"
	if strings.Count(lines[0], ",") >= strings.Count(lines[0], "\t") {
		delim = ","
	}

	var records [][]string
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		records = append(records, splitCSVLine(ln, delim))
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("need a header row plus at least one data row")
	}
	return buildDataset(records[0], records[1:]), nil
}

func loadJSON(path string) (*dataset, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Support both a JSON array and JSONL.
	trimmed := strings.TrimSpace(string(raw))
	var objs []map[string]any
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal(raw, &objs); err != nil {
			return nil, fmt.Errorf("parse JSON array: %w", err)
		}
	} else {
		for _, ln := range splitLines(trimmed) {
			ln = strings.TrimSpace(ln)
			if ln == "" {
				continue
			}
			var obj map[string]any
			if err := json.Unmarshal([]byte(ln), &obj); err != nil {
				return nil, fmt.Errorf("parse JSONL line: %w", err)
			}
			objs = append(objs, obj)
		}
	}
	if len(objs) == 0 {
		return nil, fmt.Errorf("no records in JSON")
	}

	// Column order: first-seen order across all objects.
	colSet := map[string]bool{}
	var cols []string
	for _, o := range objs {
		for k := range o {
			if !colSet[k] {
				colSet[k] = true
				cols = append(cols, k)
			}
		}
	}
	rows := make([][]string, 0, len(objs))
	for _, o := range objs {
		row := make([]string, len(cols))
		for i, c := range cols {
			v, ok := o[c]
			if !ok || v == nil {
				row[i] = ""
				continue
			}
			switch x := v.(type) {
			case string:
				row[i] = x
			case float64:
				row[i] = strconv.FormatFloat(x, 'g', -1, 64)
			case bool:
				row[i] = strconv.FormatBool(x)
			default:
				b, _ := json.Marshal(x)
				row[i] = string(b)
			}
		}
		rows = append(rows, row)
	}
	return buildDataset(cols, rows), nil
}

// buildDataset infers column types from the values.
func buildDataset(header []string, rows [][]string) *dataset {
	cols := make([]string, len(header))
	for i, h := range header {
		h = strings.TrimSpace(h)
		if h == "" {
			h = fmt.Sprintf("col%d", i+1)
		}
		cols[i] = h
	}
	// Pad/truncate rows to header width.
	for r := range rows {
		if len(rows[r]) < len(cols) {
			rows[r] = append(rows[r], make([]string, len(cols)-len(rows[r]))...)
		} else if len(rows[r]) > len(cols) {
			rows[r] = rows[r][:len(cols)]
		}
	}
	types := make([]colType, len(cols))
	for c := range cols {
		num, boolN, total := 0, 0, 0
		for r := range rows {
			v := strings.TrimSpace(rows[r][c])
			if isMissing(v) {
				continue
			}
			total++
			if !math.IsNaN(parseNumber(v)) {
				num++
			}
			switch strings.ToLower(v) {
			case "true", "false", "yes", "no":
				boolN++
			}
		}
		switch {
		case total > 0 && num == total:
			types[c] = typeNumber
		case total > 0 && boolN == total:
			types[c] = typeBool
		default:
			types[c] = typeString
		}
	}
	return &dataset{Columns: cols, Types: types, Rows: rows}
}

// --- CSV parsing helpers (RFC-4180-ish with quotes) ---

func splitCSVLine(line, delim string) []string {
	var out []string
	var cur strings.Builder
	inQuotes := false
	d := delim[0]
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if inQuotes {
			if ch == '"' {
				if i+1 < len(line) && line[i+1] == '"' { // escaped quote
					cur.WriteByte('"')
					i++
				} else {
					inQuotes = false
				}
			} else {
				cur.WriteByte(ch)
			}
			continue
		}
		switch ch {
		case '"':
			inQuotes = true
		case d:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(ch)
		}
	}
	out = append(out, cur.String())
	return out
}

func splitLines(s string) []string {
	var out []string
	start := 0
	inQuotes := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			inQuotes = !inQuotes
		case '\n':
			if !inQuotes {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// --- statistics helpers ---

type colStats struct {
	Count   int
	Missing int
	Mean    float64
	Std     float64
	Min     float64
	Q1      float64
	Median  float64
	Q3      float64
	Max     float64
	Sum     float64
}

func computeStats(d *dataset, col int) colStats {
	var vals []float64
	missing := 0
	for r := range d.Rows {
		v := strings.TrimSpace(d.Rows[r][col])
		if isMissing(v) {
			missing++
			continue
		}
		if f := parseNumber(v); !math.IsNaN(f) {
			vals = append(vals, f)
		} else {
			missing++
		}
	}
	st := colStats{Count: len(vals), Missing: missing}
	if len(vals) == 0 {
		return st
	}
	sorted := append([]float64(nil), vals...)
	sort.Float64s(sorted)
	n := len(sorted)
	sum := 0.0
	for _, v := range sorted {
		sum += v
	}
	st.Sum = sum
	st.Mean = sum / float64(n)
	st.Min = sorted[0]
	st.Max = sorted[n-1]
	st.Median = quantile(sorted, 0.5)
	st.Q1 = quantile(sorted, 0.25)
	st.Q3 = quantile(sorted, 0.75)
	if n > 1 {
		varSS := 0.0
		for _, v := range vals {
			varSS += (v - st.Mean) * (v - st.Mean)
		}
		st.Std = math.Sqrt(varSS / float64(n-1))
	}
	return st
}

func quantile(sorted []float64, q float64) float64 {
	n := len(sorted)
	if n == 0 {
		return math.NaN()
	}
	pos := q * float64(n-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	frac := pos - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

func pearson(xs, ys []float64) float64 {
	n := len(xs)
	if n < 2 {
		return math.NaN()
	}
	var sx, sy float64
	for i := 0; i < n; i++ {
		sx += xs[i]
		sy += ys[i]
	}
	mx, my := sx/float64(n), sy/float64(n)
	var num, dx, dy float64
	for i := 0; i < n; i++ {
		a, b := xs[i]-mx, ys[i]-my
		num += a * b
		dx += a * a
		dy += b * b
	}
	if dx == 0 || dy == 0 {
		return math.NaN()
	}
	return num / math.Sqrt(dx*dy)
}

// fmtNum renders a float compactly for tables.
func fmtNum(f float64) string {
	if math.IsNaN(f) {
		return "—"
	}
	if math.Abs(f) >= 1e12 || (math.Abs(f) < 1e-4 && f != 0) {
		return strconv.FormatFloat(f, 'g', 4, 64)
	}
	s := strconv.FormatFloat(f, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}
