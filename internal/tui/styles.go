package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/fu/kfs/internal/config"
	"github.com/fu/kfs/internal/resources"
)

// Every color kfs uses, and the text helpers all screens draw with.

// Set from the theme (theme.yaml).
var (
	header, dim, struck                                        lipgloss.Style
	badge, green, yellow, amber, red, cyan, flash, menu, index lipgloss.Style
	selectedBG, linkedBG, globalKey, commandKey, pulseColor    lipgloss.Color
	chipGround                                                 lipgloss.Color
)

func init() { setPalette(config.DefaultTheme()) }

// setPalette sets the colors: the defaults, or the theme's.
func setPalette(t config.Theme) {
	col := func(s string) lipgloss.Color { return lipgloss.Color(s) }
	fg := func(s string) lipgloss.Style { return lipgloss.NewStyle().Foreground(col(s)) }
	green, amber, yellow, red, cyan = fg(t.StatusOK), fg(t.StatusBusy), fg(t.StatusWarn), fg(t.StatusFailed), fg(t.Info)
	index = fg(t.Index)
	globalKey, commandKey = col(t.KeyGlobal), col(t.KeyCommand)
	header = fg(t.TextHeading).Bold(true)
	dim = fg(t.TextDim)
	struck = dim.Strikethrough(true)
	onColor := col(t.TextOnColor)
	badge = lipgloss.NewStyle().Bold(true).Foreground(onColor).Background(col(t.Badge)).Padding(0, 1)
	chipGround = col(t.Menu)
	flash = lipgloss.NewStyle().Bold(true).Foreground(onColor).Background(col(t.Flash))
	menu = lipgloss.NewStyle().Foreground(col(t.TextMenu)).Background(col(t.Menu))
	selectedBG, linkedBG = col(t.RowHighlighted), col(t.RowReferenced)
	setRailPalette(t)
	pulseColor = col(t.Pulse)
	theme = t
}

// theme is the theme the palette was set from, for the per-section frames.
var theme config.Theme

// frameStyle is the color of a section's frame.
func frameStyle(section string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(theme.FrameColor(section)))
}

// shade is color c ("#rrggbb") at f of its brightness (0–1); other colors
// (ANSI numbers) cannot be shaded and stay as they are.
func shade(c string, f float64) lipgloss.Color {
	var r, g, b int
	if len(c) != 7 || c[0] != '#' {
		return lipgloss.Color(c)
	}
	if _, err := fmt.Sscanf(c, "#%02x%02x%02x", &r, &g, &b); err != nil {
		return lipgloss.Color(c)
	}
	s := func(v int) int { return min(255, int(float64(v)*f+0.5)) }
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", s(r), s(g), s(b)))
}

// style is the color of a tone.
func style(t resources.Tone) lipgloss.Style {
	switch t {
	case resources.Dim:
		return dim
	case resources.Good:
		return green
	case resources.Busy:
		return amber
	case resources.Warn:
		return yellow
	case resources.Bad:
		return red
	case resources.Alarm:
		return red.Bold(true)
	case resources.Info:
		return cyan
	case resources.Flash:
		return flash
	case resources.Fresh:
		return green.Bold(true)
	case resources.Strong:
		return header
	case resources.Index:
		return index
	}
	return lipgloss.NewStyle()
}

// cells renders cells one after the other, e.g. a status part.
func cells(cs []resources.Cell) string {
	var b strings.Builder
	for _, c := range cs {
		b.WriteString(style(c.Tone).Render(c.Text))
	}
	return b.String()
}

func pad(s string, w int, right bool) string {
	gap := w - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	if right {
		return strings.Repeat(" ", gap) + s
	}
	return s + strings.Repeat(" ", gap)
}

// truncate shortens plain text to w cells, ending in "…".
func truncate(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > w {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// truncateMiddle shortens plain text to w cells as "prefix…suffix".
func truncateMiddle(s string, w int) string {
	r := []rune(s)
	if len(r) <= w || w < 5 {
		return truncate(s, w)
	}
	tail := (w - 1) / 2
	head := w - 1 - tail
	return string(r[:head]) + "…" + string(r[len(r)-tail:])
}

// wrap soft-wraps a (possibly styled) line to width, breaking long words
// too. The first line is indented by first spaces, continuations by rest.
// Every long text (logs, change log, secret values) wraps through here.
func wrap(s string, width, first, rest int) []string {
	avail := width - max(first, rest)
	if avail < 10 {
		return []string{clip(strings.Repeat(" ", first)+s, width)}
	}
	var parts []string
	if lipgloss.Width(s) <= avail {
		parts = []string{s}
	} else {
		parts = strings.Split(lipgloss.NewStyle().Width(avail).Render(s), "\n")
	}
	for i := range parts {
		indent := rest
		if i == 0 {
			indent = first
		}
		parts[i] = strings.Repeat(" ", indent) + strings.TrimRight(parts[i], " ")
	}
	return parts
}

// clip cuts a styled line to w cells without breaking ANSI sequences.
func clip(s string, w int) string { return lipgloss.NewStyle().MaxWidth(w).Render(s) }

// hints renders keys that work everywhere: the part in [brackets] is the
// key and shows in the global key color (ideally a letter of the word, as in
// "[l]ogs"), the rest dim.
func hints(s string) string { return marked(s, dim, globalKey) }

// marked renders s in text's style, the parts in [brackets] (the keys) in
// color key on the same ground.
func marked(s string, text lipgloss.Style, keyColor lipgloss.Color) string {
	key := text.Faint(false).Foreground(keyColor)
	var b strings.Builder
	for {
		i, j := strings.IndexByte(s, '['), strings.IndexByte(s, ']')
		if i < 0 || j < i {
			b.WriteString(text.Render(s))
			return b.String()
		}
		b.WriteString(text.Render(s[:i]) + key.Render(s[i+1:j]))
		s = s[j+1:]
	}
}

// joinParts joins status-line parts with a dim separator.
func joinParts(parts []string) string { return strings.Join(parts, dim.Render(" │ ")) }
