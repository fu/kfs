package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fu/kfs/internal/resources"
)

// rowsKind is a kind with a name and fixed rows.
type rowsKind struct {
	listing
	name string
	rows []string
}

func (k rowsKind) Name() string { return k.name }
func (k rowsKind) Rows() []resources.Row {
	var out []resources.Row
	for _, r := range k.rows {
		out = append(out, resources.Row{Key: r, Cells: []resources.Cell{{Text: r}}})
	}
	return out
}

func navModel() *model {
	return &model{width: 120, height: 40, kinds: []resources.Resource{
		rowsKind{name: "apps", rows: []string{"a1", "a2"}},
		rowsKind{name: "empty"},
		rowsKind{name: "pods", rows: []string{"p1", "p2", "p3"}},
	}}
}

func TestTabCyclesSections(t *testing.T) {
	m := navModel()
	m.page.selected = "apps/a2"
	for _, want := range []string{"pods/p1", "apps/a1", "pods/p1"} {
		m.pageKey(tea.KeyMsg{Type: tea.KeyTab})
		if m.page.selected != want {
			t.Fatalf("tab: at %q, want %q", m.page.selected, want)
		}
	}
	m.pageKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.page.selected != "apps/a1" {
		t.Errorf("shift+tab: at %q", m.page.selected)
	}
}

func TestWheelMovesSelectionOrScrollsLog(t *testing.T) {
	m := navModel()
	m.page.selected = "pods/p1"
	m.wheel(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Y: 5})
	if m.page.selected != "pods/p2" {
		t.Errorf("wheel down over the sections: at %q", m.page.selected)
	}
	m.wheel(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Y: m.height - 4})
	if m.changes.back != 1 || m.page.selected != "pods/p2" {
		t.Errorf("wheel up over the log: back %d, at %q", m.changes.back, m.page.selected)
	}
}
