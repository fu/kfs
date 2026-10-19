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
// large one.
//
// A slot where something changed is cyan, red when an alert came in, and it
// fades as it travels: bright and solid near now, then thinner, then dotted
// (┃ │ : .) towards the far end; a busy slot stays one step brighter. Where
// nothing happened the rail is a plain grey line with a faint pattern in it.
// The pattern belongs to the time, not the screen, so it walks along with
// the changes and the rail never stands still.

// railWidth is the column the rail takes left of the sections.
const railWidth = 1

// Where the rail passes the selected row it shows an arrow instead of time,
// and by every row linked to it a bulb breathing with the "watching" dot.
const (
	railCursor = "▶"
	railLinked = "●"
)

var railCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00e5ff")).Bold(true)

// railBusy is how many changes in one slot keep it one fade step brighter.
const railBusy = 3

// railGlyphs are the characters of one stretch of the rail: fade from fresh
// to old, and the background pattern (mostly the plain line).
type railGlyphs struct {
	fade [4]string
	bg   [4]string
}

var (
	railDown   = railGlyphs{fade: [4]string{"┃", "│", ":", "."}, bg: [4]string{"│", "│", "╎", "┆"}}
	railCorner = railGlyphs{fade: [4]string{"┗", "╰", "╰", "╰"}, bg: [4]string{"╰", "╰", "╰", "╰"}}
	railAlong  = railGlyphs{fade: [4]string{"━", "─", "┄", "·"}, bg: [4]string{"─", "─", "╌", "┄"}}
)

// The fade steps' colors, and the background's two greys.
var (
	railCyan = fadeStyles("#00e5ff", "#00afc8", "#1a7f8e", "#0f5560")
	railRed  = fadeStyles("#ff3b3b", "#d23232", "#9e2a2a", "#6e2222")
	railBg   = fadeStyles("#4e4e4e", "#585858", "#4e4e4e", "#444444")
)

func fadeStyles(colors ...string) [4]lipgloss.Style {
	var out [4]lipgloss.Style
	for i, c := range colors {
		out[i] = lipgloss.NewStyle().Foreground(lipgloss.Color(c))
	}
	return out
}

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
	window := r.span()
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

// span is the time the whole rail covers.
func (r *rail) span() time.Duration {
	if r.window <= 0 {
		return 10 * time.Minute
	}
	return r.window
}

// view cuts the rail into rows slots down, the corner and along slots along.
func (r *rail) view(rows, along int, now time.Time) railView {
	n := rows + 1 + along
	s := r.slots(n, now)
	// The background pattern hangs on absolute time: slot i shows the
	// pattern of the slot-long stretch of time it covers, so it moves on by
	// one slot whenever a slot's worth of time has passed.
	step := max(1, int64(r.span())/int64(max(1, n)))
	first := now.UnixNano() / step
	draw := func(i int, g railGlyphs) string {
		return s[i].glyph(g, i, n, first-int64(i))
	}
	v := railView{down: make([]string, rows)}
	for i := 0; i < rows; i++ {
		v.down[i] = draw(i, railDown)
	}
	v.corner = draw(rows, railCorner)
	var b strings.Builder
	for i := rows + 1; i < n; i++ {
		b.WriteString(draw(i, railAlong))
	}
	v.along = b.String()
	return v
}

// glyph draws slot i of n: a change in cyan (an alert in red), faded by how
// far along the rail it is; else the background pattern of time bucket t.
func (s railSlot) glyph(g railGlyphs, i, n int, t int64) string {
	if s.changes == 0 {
		p := railPattern(t)
		return railBg[p].Render(g.bg[p])
	}
	step := min(3, i*4/max(1, n))
	if s.changes >= railBusy {
		step = max(0, step-1)
	}
	if s.alert {
		return railRed[step].Render(g.fade[step])
	}
	return railCyan[step].Render(g.fade[step])
}

// railPattern picks the background of a time bucket: mostly the plain line
// (0), now and then a lighter (1), broken (2) or dotted (3) piece.
func railPattern(t int64) int {
	x := uint64(t) * 0x9e3779b97f4a7c15 // splitmix64
	x = (x ^ x>>30) * 0xbf58476d1ce4e5b9
	x = (x ^ x>>27) * 0x94d049bb133111eb
	x ^= x >> 31
	switch r := x % 20; {
	case r < 13:
		return 0
	case r < 16:
		return 1
	case r < 18:
		return 2
	}
	return 3
}
