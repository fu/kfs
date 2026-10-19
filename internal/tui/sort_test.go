package tui

import (
	"strings"
	"testing"

	"github.com/fu/kfs/internal/resources"
)

func TestCompareCellsByValue(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"9", "10", -1},
		{"512Mi", "1Gi", -1},
		{"85m", "120m", -1},
		{"3h", "2d4h", -1},
		{"-", "1", 1},
		{"beta", "Alpha", 1},
	} {
		if got := compareCells(c.a, c.b); got != c.want {
			t.Errorf("compareCells(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestSortRowsKeepsSubRows(t *testing.T) {
	row := func(name, age, sub string) resources.Row {
		return resources.Row{Sub: sub, Cells: []resources.Cell{{Text: name}, {Text: age}}}
	}
	rows := []resources.Row{
		row("a", "5m", ""), row("a/c1", "", "c1"),
		row("b", "2m", ""), row("b/c1", "", "c1"),
	}
	out := sortRows(rows, sortState{col: 1})
	var got []string
	for _, r := range out {
		got = append(got, r.Cells[0].Text)
	}
	if want := "b b/c1 a a/c1"; strings.Join(got, " ") != want {
		t.Errorf("got %v, want %s", got, want)
	}
}

func TestCycleReturnsToName(t *testing.T) {
	cols := []resources.Column{{Title: "NAME"}, {Title: "AGE"}}
	var p page
	p.cycle("pods", cols) // NAME ↓
	if s := p.sorts["pods"]; s != (sortState{col: 0, desc: true}) {
		t.Fatalf("first press: %+v", s)
	}
	p.cycle("pods", cols) // AGE ↑
	p.cycle("pods", cols) // AGE ↓
	p.cycle("pods", cols) // back to NAME ↑
	if _, ok := p.sorts["pods"]; ok {
		t.Errorf("back at NAME ↑ the section has its own order again: %+v", p.sorts)
	}
}
