package tui

import (
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestRailSlotsByAge(t *testing.T) {
	now := time.Now()
	r := rail{window: 10 * time.Minute}
	r.mark(now.Add(-30*time.Second), false, nil) // newest slot
	r.mark(now.Add(-9*time.Minute), true, nil)   // last slot of 10
	r.mark(now.Add(-11*time.Minute), true, nil)  // walked off the end
	s := r.slots(10, now)
	if s[0].changes != 1 || s[0].alert {
		t.Errorf("slot 0: %+v", s[0])
	}
	if s[9].changes != 1 || !s[9].alert {
		t.Errorf("slot 9: %+v", s[9])
	}
	if len(r.events) != 2 {
		t.Errorf("the event older than the window is forgotten: %d left", len(r.events))
	}
}

// A bigger window shows the same time finer: alerts in one slot of 10 spread
// over two slots of 20.
func TestRailResizeSpreadsSlots(t *testing.T) {
	now := time.Now()
	r := rail{window: 10 * time.Minute}
	r.mark(now.Add(-4*time.Minute-10*time.Second), true, nil)
	r.mark(now.Add(-4*time.Minute-40*time.Second), true, nil)
	count := func(n int) int {
		c := 0
		for _, s := range r.slots(n, now) {
			if s.alert {
				c++
			}
		}
		return c
	}
	if c := count(10); c != 1 {
		t.Errorf("10 slots: %d red, want 1", c)
	}
	if c := count(20); c != 2 {
		t.Errorf("20 slots: %d red, want 2", c)
	}
}

func TestRailSkipsHiddenChanges(t *testing.T) {
	now := time.Now()
	r := rail{window: time.Minute}
	r.mark(now, true, func() bool { return false })
	if s := r.slots(5, now); s[0].changes != 0 || s[0].alert {
		t.Errorf("a change not on the page is not on the rail: %+v", s[0])
	}
}

func TestRailViewShape(t *testing.T) {
	r := rail{window: time.Minute}
	v := r.view(4, 30, time.Now())
	if len(v.down) != 4 || lipgloss.Width(v.corner) != 1 || lipgloss.Width(v.along) != 30 {
		t.Errorf("down %d, corner width %d, along width %d", len(v.down), lipgloss.Width(v.corner), lipgloss.Width(v.along))
	}
}
