package tui

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/fu/kfs/internal/resources"
)

// Sorting a section by its columns: Shift+<section key> steps through them.
// A section starts in its own order, NAME ↑; each press turns the column
// around (↓), then moves on to the next column ↑, cycling back to NAME ↑. A
// small arrow in the header marks the column. Sub-rows (a pod's containers)
// stay under their row.

// sortState is how a section is sorted: by column col (index into its
// columns), descending or not.
type sortState struct {
	col  int
	desc bool
}

// nameCol is the NAME column (else the first): the order a section starts in.
func nameCol(cols []resources.Column) int {
	return max(0, slices.IndexFunc(cols, func(c resources.Column) bool { return c.Title == "NAME" }))
}

// cycle moves a section's sort one step on.
func (p *page) cycle(section string, cols []resources.Column) {
	if len(cols) == 0 {
		return
	}
	s, ok := p.sorts[section]
	if !ok {
		s = sortState{col: nameCol(cols)}
	}
	if s.desc {
		s = sortState{col: (s.col + 1) % len(cols)}
	} else {
		s.desc = true
	}
	if s == (sortState{col: nameCol(cols)}) {
		delete(p.sorts, section) // back to the section's own order
		return
	}
	if p.sorts == nil {
		p.sorts = map[string]sortState{}
	}
	p.sorts[section] = s
}

// sortCols are a section's columns with the arrow on the sorted one.
func (m *model) sortCols(k resources.Resource) []resources.Column {
	cols := slices.Clone(k.Columns())
	if len(cols) == 0 {
		return cols
	}
	s, ok := m.page.sorts[k.Name()]
	if !ok {
		s = sortState{col: nameCol(cols)}
	}
	if s.col < len(cols) {
		arrow := " ↑"
		if s.desc {
			arrow = " ↓"
		}
		cols[s.col].Title += arrow
	}
	return cols
}

// sortRows orders rows by column s.col, keeping each row's sub-rows under it.
func sortRows(rows []resources.Row, s sortState) []resources.Row {
	type group struct{ rows []resources.Row }
	var groups []group
	for _, r := range rows {
		if r.Sub != "" && len(groups) > 0 {
			groups[len(groups)-1].rows = append(groups[len(groups)-1].rows, r)
			continue
		}
		groups = append(groups, group{rows: []resources.Row{r}})
	}
	text := func(g group) string {
		if s.col < len(g.rows[0].Cells) {
			return g.rows[0].Cells[s.col].Text
		}
		return ""
	}
	slices.SortStableFunc(groups, func(a, b group) int {
		c := compareCells(text(a), text(b))
		if s.desc {
			c = -c
		}
		return c
	})
	out := make([]resources.Row, 0, len(rows))
	for _, g := range groups {
		out = append(out, g.rows...)
	}
	return out
}

// compareCells compares cell texts the way a person reads them: numbers,
// sizes ("42Mi", "1.5Gi"), CPU ("85m"), percentages and ages ("3d4h") by
// value, everything else alphabetically; empty and "-" last.
func compareCells(a, b string) int {
	a, b = cleanCell(a), cleanCell(b)
	switch ea, eb := a == "" || a == "-", b == "" || b == "-"; {
	case ea && eb:
		return 0
	case ea:
		return 1
	case eb:
		return -1
	}
	va, oka := cellValue(a)
	vb, okb := cellValue(b)
	if oka && okb {
		return cmp.Compare(va, vb)
	}
	return cmp.Compare(strings.ToLower(a), strings.ToLower(b))
}

// cleanCell drops what decorates a value: trend arrows, forward markers.
func cleanCell(s string) string {
	s = strings.TrimSpace(strings.TrimRight(s, "↑↓ "))
	if i := strings.Index(s, " ⇄"); i >= 0 {
		s = s[:i]
	}
	return s
}

var (
	ageRe   = regexp.MustCompile(`^(\d+[ydhms])+$`)
	agePart = regexp.MustCompile(`(\d+)([ydhms])`)
	numRe   = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(Ki|Mi|Gi|Ti|m|%)?`)
)

// cellValue reads a cell as a number, if it is one.
func cellValue(s string) (float64, bool) {
	if ageRe.MatchString(s) {
		secs := map[string]float64{"y": 365 * 86400, "d": 86400, "h": 3600, "m": 60, "s": 1}
		var total float64
		for _, p := range agePart.FindAllStringSubmatch(s, -1) {
			n, _ := strconv.ParseFloat(p[1], 64)
			total += n * secs[p[2]]
		}
		return total, true
	}
	m := numRe.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	switch m[2] {
	case "Ki":
		v *= 1 << 10
	case "Mi":
		v *= 1 << 20
	case "Gi":
		v *= 1 << 30
	case "Ti":
		v *= 1 << 40
	}
	return v, true
}
