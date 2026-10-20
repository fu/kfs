package tui

import (
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/fu/kfs/internal/kube"
	"github.com/fu/kfs/internal/resources"
)

// The way kfs starts: the change log is reset, then the events Kubernetes
// still keeps arrive (older than the start). They land on the rail in their
// colors instead of falling off it.
func TestStartupEventsFillTheRail(t *testing.T) {
	m := &model{}
	m.changes.rail.window = 10 * time.Minute
	m.changes.reset()
	now := time.Now()
	for _, e := range []kube.Event{
		{Namespace: "shop", Kind: "Pod", Name: "web-1", Type: "Normal", Reason: "Pulled", Message: "pulled", At: now.Add(-50 * time.Minute), Initial: true},
		{Namespace: "shop", Kind: "Pod", Name: "web-1", Type: "Warning", Reason: "Unhealthy", Message: "probe failed", At: now.Add(-30 * time.Minute), Initial: true},
	} {
		m.addEvent(eventMsg{e: e})
	}
	m.Log(resources.Change{At: now.Add(-time.Second), Kind: resources.State, Object: "web-1",
		State: []resources.Cell{{Text: "CrashLoopBackOff", Tone: resources.Alarm}}})
	s := m.changes.rail.slots(60, now)
	var got []railLevel
	for _, sl := range s {
		if sl.level != railNone {
			got = append(got, sl.level)
		}
	}
	// newest first: the failing pod (red), the warning (amber), the pull (cyan)
	if len(got) != 3 || got[0] != railError || got[1] != railWarn || got[2] != railGood {
		t.Errorf("rail levels newest first: %v, want [red amber cyan]", got)
	}
}

func TestRailSlotsByAge(t *testing.T) {
	now := time.Now()
	r := rail{window: 10 * time.Minute}
	r.mark(now.Add(-30*time.Second), railGood, nil)  // newest slot
	r.mark(now.Add(-9*time.Minute), railError, nil)  // last slot of 10
	r.mark(now.Add(-11*time.Minute), railError, nil) // walked off the end
	s := r.slots(10, now)
	if s[0].changes != 1 || s[0].level != railGood {
		t.Errorf("slot 0: %+v", s[0])
	}
	if s[9].changes != 1 || s[9].level != railError {
		t.Errorf("slot 9: %+v", s[9])
	}
	if len(r.events) != 2 {
		t.Errorf("the event older than the window is forgotten: %d left", len(r.events))
	}
}

// Within a slot the worst change wins: red over amber over cyan.
func TestRailWorstLevelWins(t *testing.T) {
	now := time.Now()
	r := rail{window: 10 * time.Minute}
	for _, l := range []railLevel{railGood, railError, railWarn, railGood} {
		r.mark(now.Add(-10*time.Second), l, nil)
	}
	r.mark(now.Add(-5*time.Minute), railGood, nil)
	r.mark(now.Add(-5*time.Minute), railWarn, nil)
	s := r.slots(10, now)
	if s[0].level != railError || s[0].changes != 4 {
		t.Errorf("slot 0: %+v, want red from 4 changes", s[0])
	}
	if s[5].level != railWarn {
		t.Errorf("slot 5: %+v, want amber", s[5])
	}
}

// What happened before the page started (the last hour of events) is
// squeezed into the window, so the rail starts full and in order, then
// walks off at the normal pace.
func TestRailHistoryFillsTheWindow(t *testing.T) {
	start := time.Now()
	r := rail{window: 10 * time.Minute}
	r.reset(start)
	r.mark(start.Add(-55*time.Minute), railError, nil) // old: far end
	r.mark(start.Add(-6*time.Minute), railWarn, nil)   // recent: near the top
	r.mark(start.Add(time.Second), railGood, nil)      // after the start: as is
	s := r.slots(60, start.Add(time.Second))
	at := func(l railLevel) int {
		for i, sl := range s {
			if sl.level == l {
				return i
			}
		}
		return -1
	}
	if i := at(railError); i < 50 {
		t.Errorf("hour-old error at slot %d, want near the far end of 60", i)
	}
	if i := at(railWarn); i < 4 || i > 8 {
		t.Errorf("6-minute-old warning at slot %d, want about 6", i)
	}
	if s[0].level != railGood {
		t.Errorf("the change after the start is not at the top: %+v", s[0])
	}
	// Ten minutes on, the history has walked off.
	if s := r.slots(60, start.Add(10*time.Minute+time.Second)); s[59].level == railError {
		t.Errorf("the old error should have left the rail")
	}
}

// A bigger window shows the same time finer: alerts in one slot of 10 spread
// over two slots of 20.
func TestRailResizeSpreadsSlots(t *testing.T) {
	now := time.Now()
	r := rail{window: 10 * time.Minute}
	r.mark(now.Add(-4*time.Minute-10*time.Second), railError, nil)
	r.mark(now.Add(-4*time.Minute-40*time.Second), railError, nil)
	count := func(n int) int {
		c := 0
		for _, s := range r.slots(n, now) {
			if s.level == railError {
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
	r.mark(now, railError, func() bool { return false })
	if s := r.slots(5, now); s[0].changes != 0 || s[0].level != railNone {
		t.Errorf("a change not on the page is not on the rail: %+v", s[0])
	}
}

// The grey packets flow: one railFlow later, every slot shows what the slot
// before it showed, whatever the rail's time scale.
func TestRailPatternWalks(t *testing.T) {
	r := rail{window: 10 * time.Minute}
	now := time.Unix(1000, 0)
	before := []rune(ansi.Strip(r.view(0, 60, now).along))
	after := []rune(ansi.Strip(r.view(0, 60, now.Add(railFlow)).along))
	if string(after[1:]) != string(before[:len(before)-1]) {
		t.Errorf("pattern did not move on by one:\n%s\n%s", string(before), string(after))
	}
}

// The grey packets swell and fade one step at a time (". : │ : ."), with
// gaps between them, and there are packets of every height.
func TestRailPatternIsPackets(t *testing.T) {
	heights := map[int]bool{}
	prev := 0
	for b := int64(-500); b < 5000; b++ {
		h := railPattern(b)
		heights[h] = true
		if d := h - prev; d > 1 || d < -1 {
			t.Fatalf("bucket %d: height %d after %d, want one step at a time", b, h, prev)
		}
		prev = h
	}
	for h := 0; h <= 3; h++ {
		if !heights[h] {
			t.Errorf("no packet piece of height %d", h)
		}
	}
}

// A change fades as it travels: solid near now, dotted near the far end.
func TestRailChangesFade(t *testing.T) {
	now := time.Now()
	r := rail{window: 100 * time.Second}
	r.mark(now, railGood, nil)
	r.mark(now.Add(-98*time.Second), railGood, nil)
	v := r.view(9, 90, now) // 100 slots
	if got := ansi.Strip(v.down[0]); got != "┃" {
		t.Errorf("fresh change: %q", got)
	}
	along := []rune(ansi.Strip(v.along))
	if got := string(along[len(along)-2]); got != "·" {
		t.Errorf("old change: %q", got)
	}
}

func TestChangeLevels(t *testing.T) {
	c := func(kind resources.ChangeKind, tones ...resources.Tone) resources.Change {
		ch := resources.Change{Kind: kind}
		for _, tn := range tones {
			ch.State = append(ch.State, resources.Cell{Text: "x", Tone: tn})
		}
		return ch
	}
	for _, tc := range []struct {
		name string
		ch   resources.Change
		want railLevel
	}{
		{"an event", c(resources.Event), railGood},
		{"a pod running", c(resources.Modified, resources.Good), railGood},
		{"a warning event", c(resources.Warning), railWarn},
		{"a pod pending", c(resources.Added, resources.Busy), railWarn},
		{"a pod failing", c(resources.Modified, resources.Alarm), railError},
		{"a warning about a failing pod", c(resources.Warning, resources.Bad), railError},
		{"already unhealthy at start", c(resources.State, resources.Busy, resources.Alarm), railError},
	} {
		if got := level(tc.ch); got != tc.want {
			t.Errorf("%s: level %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestRailViewShape(t *testing.T) {
	r := rail{window: time.Minute}
	v := r.view(4, 30, time.Now())
	if len(v.down) != 4 || lipgloss.Width(v.corner) != 1 || lipgloss.Width(v.along) != 30 {
		t.Errorf("down %d, corner width %d, along width %d", len(v.down), lipgloss.Width(v.corner), lipgloss.Width(v.along))
	}
}
