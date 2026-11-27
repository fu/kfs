package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/fu/kfs/internal/config"
)

// The rail is a time line framing the change log. It starts right after
// the change log's title (now), runs along the top to the right, down the
// right edge, back along the bottom, up the left edge and ends in the
// top-left corner: `rail.minutes` ago (config).
//
// The rail keeps no slots, only what happened and when (railEvent). Every
// frame it is cut afresh into as many slots as the frame has room for, each
// slot covering window/slots of time — so everything walks one slot further
// every window/slots, and a bigger window simply shows the same time finer.
//
// A slot where something changed is cyan when all was fine, amber for a
// warning, red for something failing (the worst change in it wins), and it
// fades as it travels: bright and solid near now, then thinner, then dotted
// towards the far end; a busy slot stays one step brighter. Where nothing
// happened the rail is grey packets that swell and fade, with gaps between
// them. They flow on a steady beat of their own, faster than the changes: a
// new piece appears right after the title every railFlow and the rest move
// on by one, so the rail never stands still.

// On a section's frame, by the selected row: an arrow; by every row linked
// to it: a bulb breathing with the "watching" dot.
const (
	railCursor = "▶"
	railLinked = "●"
)

// railBusy is how many changes in one slot keep it one fade step brighter.
const railBusy = 3

// railGlyphs are the characters of one stretch of the rail: a change faded
// from fresh to old, and the grey packets by height (0 is the gap).
type railGlyphs struct {
	fade [4]string
	bg   [4]string
}

var (
	railDown        = railGlyphs{fade: [4]string{"┃", "│", ":", "."}, bg: [4]string{" ", ".", ":", "│"}}
	railAlong       = railGlyphs{fade: [4]string{"━", "─", "┄", "·"}, bg: [4]string{" ", "·", "╌", "─"}}
	railTopRight    = corner("┓", "╮")
	railBottomRight = corner("┛", "╯")
	railBottomLeft  = corner("┗", "╰")
	railTopLeft     = corner("┏", "╭")
)

// corner is a corner's glyphs: heavy while fresh, else always drawn.
func corner(fresh, other string) railGlyphs {
	return railGlyphs{fade: [4]string{fresh, other, other, other}, bg: [4]string{other, other, other, other}}
}

// The fade steps' colors by level, the packets' greys by height, and the
// cursor: from the theme (setRailPalette).
var (
	railColors      map[railLevel][4]lipgloss.Style
	railBg          [4]lipgloss.Style
	railCursorStyle lipgloss.Style
)

// setRailPalette shades the theme's rail colors: each level fades towards
// black as it ages; the idle packets are greys around rail-idle.
func setRailPalette(t config.Theme) {
	fade := func(c string, fs ...float64) [4]lipgloss.Style {
		var out [4]lipgloss.Style
		for i, f := range fs {
			out[i] = lipgloss.NewStyle().Foreground(shade(c, f))
		}
		return out
	}
	steps := []float64{1, 0.78, 0.55, 0.38}
	railColors = map[railLevel][4]lipgloss.Style{
		railGood:  fade(t.RailOK, steps...),
		railWarn:  fade(t.RailWarn, steps...),
		railError: fade(t.RailFailed, steps...),
	}
	railBg = fade(t.RailIdle, 0.82, 0.71, 0.82, 1)
	railCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(t.RailCursor)).Bold(true)
}

// railLevel is how a change reads on the rail; within a slot the highest
// wins: red over amber over cyan.
type railLevel int

const (
	railNone  railLevel = iota
	railGood            // cyan: something happened, all fine
	railWarn            // amber: a warning, something on its way
	railError           // red: something failing
)

// railHistory is how far back the events kfs finds at start go (Kubernetes
// keeps them about an hour). They are squeezed into the rail's window so it
// starts full, then walk off at the normal pace.
const railHistory = time.Hour

// The grey packets: they flow down and along one slot every railFlow,
// whatever the rail's time scale, cut into blocks of railBlock slots; a block
// holds one packet of up to railPacket slots, or none (a longer gap).
const (
	railFlow   = 300 * time.Millisecond
	railBlock  = 12
	railPacket = 9
)

// railEvent is one change: when, how it reads, and whether it (still)
// belongs on the page, like its change-log entry. at is where it sits on the
// rail: for history, its squeezed time.
type railEvent struct {
	at    time.Time
	level railLevel
	show  func() bool
}

type rail struct {
	window time.Duration
	start  time.Time // when the page started: history is what came before
	events []railEvent
}

func (r *rail) reset(now time.Time) { r.events, r.start = nil, now }

// mark records a change. A change from before the page started is placed at
// its age squeezed from railHistory into the window, so the history fills
// the rail at start instead of falling off it.
func (r *rail) mark(at time.Time, level railLevel, show func() bool) {
	if !r.start.IsZero() && at.Before(r.start) {
		if w := r.span(); w < railHistory {
			at = r.start.Add(-time.Duration(float64(r.start.Sub(at)) * float64(w) / float64(railHistory)))
		}
	}
	r.events = append(r.events, railEvent{at: at, level: level, show: show})
}

// railSlot is what happened in one slot's time: how many changes, and the
// highest level among them.
type railSlot struct {
	changes int
	level   railLevel
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
		out[i].level = max(out[i].level, e.level)
	}
	clear(r.events[len(keep):]) // let the dropped closures go
	r.events = keep
	return out
}

// railFrame is the rail cut around the change log: the top-left corner
// (the oldest slot), the run from after the title to the top-right corner,
// the right and left edges top to bottom, and the bottom line.
type railFrame struct {
	topLeft, top string
	left, right  []string
	bottom       string
}

// span is the time the whole rail covers.
func (r *rail) span() time.Duration {
	if r.window <= 0 {
		return 10 * time.Minute
	}
	return r.window
}

// frame cuts the rail around a box width wide with rows lines inside and a
// title title cells wide after the top-left corner.
func (r *rail) frame(width, rows, title int, now time.Time) railFrame {
	top, bottom := max(0, width-2-title), max(0, width-2)
	n := top + 1 + rows + 1 + bottom + 1 + rows + 1
	s := r.slots(n, now)
	// The grey packets flow on their own steady beat: slot i shows the
	// piece born i beats ago, so a new piece appears after the title every
	// railFlow and the rest move on by one.
	first := now.UnixNano() / int64(railFlow)
	i := 0
	next := func(g railGlyphs) string {
		out := s[i].glyph(g, i, n, first-int64(i))
		i++
		return out
	}
	var f railFrame
	var b strings.Builder
	for range top {
		b.WriteString(next(railAlong))
	}
	b.WriteString(next(railTopRight))
	f.top = b.String()
	f.right = make([]string, rows)
	for j := range rows {
		f.right[j] = next(railDown)
	}
	br := next(railBottomRight)
	along := make([]string, bottom)
	for j := bottom - 1; j >= 0; j-- {
		along[j] = next(railAlong)
	}
	f.bottom = next(railBottomLeft) + strings.Join(along, "") + br
	f.left = make([]string, rows)
	for j := rows - 1; j >= 0; j-- {
		f.left[j] = next(railDown)
	}
	f.topLeft = next(railTopLeft)
	return f
}

// glyph draws slot i of n: its changes in their level's color (red, amber,
// cyan), faded by how far along the rail it is; else the background pattern
// of time bucket t.
func (s railSlot) glyph(g railGlyphs, i, n int, t int64) string {
	if s.changes == 0 || s.level == railNone {
		p := railPattern(t)
		return railBg[p].Render(g.bg[p])
	}
	step := min(3, i*4/max(1, n))
	if s.changes >= railBusy {
		step = max(0, step-1)
	}
	return railColors[s.level][step].Render(g.fade[step])
}

// railPattern is the height (0 gap, 1 ".", 2 ":", 3 "│") of the grey packet
// at time bucket t. Each block of railBlock buckets draws its packet's
// length and peak from a hash of the block: it swells up to the peak and
// fades again (". : │ │ : ."), the rest of the block is gap.
func railPattern(t int64) int {
	block, j := t/railBlock, int(t%railBlock)
	if j < 0 {
		block, j = block-1, j+railBlock
	}
	x := hash64(uint64(block))
	length := int(x % (railPacket + 1)) // 0: no packet in this block
	// Longer packets swell higher; now and then one stays a step lower.
	peak := min(3, (length+1)/2)
	if (x>>8)%4 == 0 {
		peak = max(1, peak-1)
	}
	if j >= length {
		return 0
	}
	return min(j+1, length-j, peak)
}

// hash64 is splitmix64's finalizer: a well-mixed number for every block.
func hash64(x uint64) uint64 {
	x = (x + 0x9e3779b97f4a7c15)
	x = (x ^ x>>30) * 0xbf58476d1ce4e5b9
	x = (x ^ x>>27) * 0x94d049bb133111eb
	return x ^ x>>31
}
