package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// A resource is drawn in a frame, btop style, in its theme color: its name
// and the column headers sit in the top border, each in a notch; the bottom
// border has, while a row of it is selected, what that row can do on the
// left, and its status items on the right:
//
//	┌┐ pods ┌┐NAME ↑┌──────┐READY┌┐STATUS┌──────────────────────┐
//	▶ 1  web-7d4b9c6f5-x2k8q   1/1    ●       …                   │
//	└┘ logs  shell  forward  copy  ` sort └───┘ 8 ●5 ●1 ●0 └┘ ⇄ 1 └┘
//
// The left border carries the selection: an arrow by the selected row, a
// bulb by every row linked to it.

// frameTop is the top border, width cells wide: the title, then each label
// over the cell it starts at (x counts from the first cell inside the
// frame), pushed right when the one before is in the way; labels that do
// not fit are left out.
func frameTop(c lipgloss.Style, title string, labels []label, width int) string {
	if width < 2 {
		return ""
	}
	var b strings.Builder
	head := " " + title + " "
	pos := 0
	put := func(s string, st lipgloss.Style) {
		b.WriteString(st.Render(s))
		pos += lipgloss.Width(s)
	}
	put("┌", c)
	if pos+lipgloss.Width(head)+3 <= width {
		put("┐", c)
		put(head, header)
		put("┌", c)
	}
	for _, l := range labels {
		at := max(pos, l.x) // the notch's ┐ sits left of the label's first cell
		if at+lipgloss.Width(l.text)+3 > width {
			break
		}
		put(strings.Repeat("─", at-pos), c)
		put("┐", c)
		put(l.text, header)
		put("┌", c)
	}
	put(strings.Repeat("─", max(0, width-1-pos)), c)
	put("┐", c)
	return b.String()
}

// frameBottom is the bottom border, width cells wide: the commands in one
// notch at its left end, the items (each in a notch) at its right end.
// What does not fit goes: items first ones first, then the commands from
// the end.
func frameBottom(c lipgloss.Style, cmds, items []string, width int) string {
	if width < 2 {
		return ""
	}
	notch := func(it string) string { return c.Render("┘") + " " + it + " " + c.Render("└") }
	render := func(cmds, items []string) (string, string) {
		var left, right string
		if len(cmds) > 0 {
			left = notch(strings.Join(cmds, "  "))
		}
		for _, it := range items {
			right += notch(it)
		}
		return left, right
	}
	for {
		left, right := render(cmds, items)
		if gap := width - 2 - lipgloss.Width(left) - lipgloss.Width(right); gap >= 0 {
			return c.Render("└") + left + c.Render(strings.Repeat("─", gap)) + right + c.Render("┘")
		}
		switch {
		case len(items) > 0:
			items = items[1:]
		case len(cmds) > 0:
			cmds = cmds[:len(cmds)-1]
		default:
			return c.Render("└" + strings.Repeat("─", width-2) + "┘")
		}
	}
}

// frameRow is a line inside the frame: left is the left border (│, or the
// arrow or bulb), the line padded to the inside, the right border.
func frameRow(c lipgloss.Style, left, line string, width int) string {
	inner := max(0, width-2)
	line = clip(line, inner)
	return left + line + strings.Repeat(" ", max(0, inner-lipgloss.Width(line))) + c.Render("│")
}
