package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// The rail is a time line. It starts under the k7s badge, runs down the left
// edge of the sections, turns at the divider and runs along it to the right
// edge: top-left is now, the far right end is `rail.minutes` ago (config).
//
// The rail keeps no slots, only what happened and when (railEvent). Every
// frame it is cut afresh into as many slots as the window has room for, each
// slot covering window/slots of time — so everything walks one slot further
// every window/slots, and a bigger window simply shows the same time finer:
// an alert burst that fills one slot on a small screen may fill two on a
// large one. A slot is dim when nothing happened in its time, grey when
// something changed (brighter the more), red when an alert came in.

// railWidth is the column the rail takes left of the sections.
const railWidth = 1

// Where the rail passes the selected row it shows an arrow instead of time,
// and by every row linked to it a bulb breathing with the "watching" dot.
const (
	railCursor = "▶"
	railLinked = "●"
)

var railCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00e5ff")).Bold(true)

// railBusy is how many changes in one slot make it bright.
const railBusy = 3

var (
	railEmptyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#4e4e4e"))
	railQuietStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#8a8a8a"))
	railBusyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#c6c6c6"))
)

// railEvent is one change: when, whether it was an alert, and whether it
// (still) belongs on the page, like its change-log entry.
type railEvent struct {
	at    time.Time
	alert bool
	show  func() bool
}

type rail struct {
	window time.Duration
	events []railEvent
}

func (r *rail) reset() { r.events = nil }

// mark records a change.
func (r *rail) mark(at time.Time, alert bool, show func() bool) {
	r.events = append(r.events, railEvent{at: at, alert: alert, show: show})
}

// railSlot is what happened in one slot's time.
type railSlot struct {
	changes int
	alert   bool
}

// slots cuts the window into n slots, slot 0 the newest, and forgets events
// older than the window.
func (r *rail) slots(n int, now time.Time) []railSlot {
	out := make([]railSlot, max(0, n))
	window := r.window
	if window <= 0 {
		window = 10 * time.Minute
	}
	keep := r.events[:0]
	for _, e := range r.events {
		age := now.Sub(e.at)
		if age > window {
			continue // walked off the end
		}
		keep = append(keep, e)
		if n == 0 || (e.show != nil && !e.show()) {
			continue
		}
		i := int(float64(max(0, age)) / float64(window) * float64(n))
		i = min(i, n-1)
		out[i].changes++
		out[i].alert = out[i].alert || e.alert
	}
	clear(r.events[len(keep):]) // let the dropped closures go
	r.events = keep
	return out
}

// railView is the rail cut for the page: a glyph for each row from under the
// badge down to the divider, the corner where it turns, and the run along the
// divider (after the change log's title).
type railView struct {
	down   []string
	corner string
	along  string
}

// view cuts the rail into rows slots down, the corner and along slots along.
func (r *rail) view(rows, along int, now time.Time) railView {
	s := r.slots(rows+1+along, now)
	v := railView{down: make([]string, rows)}
	for i := 0; i < rows; i++ {
		v.down[i] = s[i].glyph("┆", "│", "┃")
	}
	v.corner = s[rows].glyph("╰", "╰", "┗")
	var b strings.Builder
	for _, sl := range s[rows+1:] {
		b.WriteString(sl.glyph("┄", "─", "━"))
	}
	v.along = b.String()
	return v
}

// glyph draws a slot: empty, with some changes, busy or with an alert.
func (s railSlot) glyph(empty, quiet, busy string) string {
	switch {
	case s.alert:
		return red.Render(busy)
	case s.changes >= railBusy:
		return railBusyStyle.Render(busy)
	case s.changes > 0:
		return railQuietStyle.Render(quiet)
	}
	return railEmptyStyle.Render(empty)
}
