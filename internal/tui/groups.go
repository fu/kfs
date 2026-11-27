package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The header is a frame like the resources', in the theme's frame-color-kfs:
// what is watched in its top border, inside the resources by their keys,
// each a chip with its name after it (wrapped when they do not fit), and
// the scopes in its bottom border, like the commands at the bottom of the
// selected row's frame (commandItems). The footer has the keys that work
// the TUI itself:
//
//	┌┐ kfs ┌┐ctx: dev┌┐ns: shop┌┐filter: none┌──────────┐● watching┌┐   statusLine
//	│ F1 gitops  F2 deployments  …  F10 secrets                      │   keyLines
//	└┘ 1 contexts  2 namespaces  3 filter └───────────────────────────┘   scopeItems
//	    … resources, change log …
//	    the TUI's own keys                                               footer

const footerLines = 1

func (m *model) header() []string {
	c := frameStyle("kfs")
	out := []string{m.statusLine()}
	for _, l := range m.keyLines() {
		out = append(out, frameRow(c, c.Render("│"), l, m.width))
	}
	return append(out, frameBottom(c, m.scopeItems(), nil, m.width))
}

func (m *model) headerView() string { return strings.Join(m.header(), "\n") }

// headerLines is how many lines the header takes at the current width.
func (m *model) headerLines() int { return len(m.header()) }

// frameBar is a top border with items in notches: left ones from the left,
// right ones at the right end (dropped first when it does not fit).
func frameBar(c lipgloss.Style, left, right []string, width int) string {
	notches := func(items []string) string {
		var b strings.Builder
		for _, it := range items {
			b.WriteString(c.Render("┐") + it + c.Render("┌"))
		}
		return b.String()
	}
	l := c.Render("┌") + notches(left)
	r := notches(right) + c.Render("┐")
	if gap := width - lipgloss.Width(l) - lipgloss.Width(r); gap >= 0 {
		return l + c.Render(strings.Repeat("─", gap)) + r
	}
	l = clip(l, max(0, width-1))
	return l + c.Render(strings.Repeat("─", max(0, width-1-lipgloss.Width(l)))+"┐")
}

// part is one entry of a header line and what separates it from the one
// before.
type part struct{ text, sep string }

const thinSep = "  "

// flow lays parts out after a space, wrapping before a part that does not
// fit in width.
func flow(parts []part, width int) []string {
	var lines []string
	line, w := " ", 1
	for i, p := range parts {
		pw := lipgloss.Width(p.text)
		sep := p.sep
		if i > 0 && w+lipgloss.Width(sep)+pw > width && w > 1 {
			lines = append(lines, line)
			line, sep = " ", ""
		} else if i == 0 {
			sep = ""
		}
		line += sep + p.text
		w = lipgloss.Width(line)
	}
	return append(lines, line)
}

// chip is a key on its ground, followed by what it does: plain while on,
// dim while off (a hidden resource).
func chip(key, name string, keyColor lipgloss.Color, on bool) string {
	k := lipgloss.NewStyle().Background(chipGround).Foreground(keyColor).Bold(true)
	n := plain
	if !on {
		k, n = k.Foreground(dim.GetForeground()).Bold(false), dim
	}
	return k.Render(" "+key+" ") + n.Render(" "+name)
}

// scope changes what the page shows, through a picker.
type scope struct {
	name, key string
	open      func(*model) tea.Cmd
}

func scopes() []scope {
	return []scope{
		{"contexts", "1", (*model).openContexts},
		{"namespaces", "2", (*model).openNamespaces},
		{"filter", "3", (*model).openFilter},
	}
}

// keyLines: every resource by its toggle key (F1, F2, …), dim while
// hidden.
func (m *model) keyLines() []string {
	var parts []part
	for i, k := range m.kinds {
		parts = append(parts, part{chip(keyLabel(sectionKey(i)), k.Name(), globalKey, m.page.shown(k)), thinSep})
	}
	return flow(parts, max(1, m.width-2))
}

// scopeItems are the scopes for the header's bottom border, their keys lit,
// the open one on the menu ground.
func (m *model) scopeItems() []string {
	open := ""
	if p, ok := m.top().(*pickerScreen); ok {
		open = p.scope
	}
	var out []string
	for _, s := range scopes() {
		st := plain
		if s.name == open {
			st = menu
		}
		out = append(out, marked(commandHint(s.name, s.key), st, globalKey))
	}
	return out
}

// ---- footer: the TUI's own keys --------------------------------------------------

// tuiKey works the page itself: moves the selection, scrolls and resizes the
// change log, quits.
type tuiKey struct {
	keys  string            // space-separated
	match func(string) bool // or a pattern of keys
	hint  string            // in the footer; "" for the other half of a pair
	do    func(m *model, key string) tea.Cmd
}

func (t tuiKey) matches(key string) bool {
	if t.match != nil {
		return t.match(key)
	}
	return strings.Contains(" "+t.keys+" ", " "+key+" ")
}

func tuiKeys() []tuiKey {
	run := func(f func(m *model)) func(*model, string) tea.Cmd {
		return func(m *model, _ string) tea.Cmd { f(m); return nil }
	}
	page := func(m *model) int { return m.layout().body }
	return []tuiKey{
		{keys: "up k", hint: "[↑↓] move", do: run(func(m *model) { m.moveSelection(-1) })},
		{keys: "down j", do: run(func(m *model) { m.moveSelection(1) })},
		{keys: "pgup", do: run(func(m *model) { m.moveSelection(-page(m)) })},
		{keys: "pgdown", do: run(func(m *model) { m.moveSelection(page(m)) })},
		{keys: "home", do: run(func(m *model) { m.moveSelection(-1 << 30) })},
		{keys: "end", do: run(func(m *model) { m.moveSelection(1 << 30) })},
		{keys: "tab", hint: "[tab] next section", do: run(func(m *model) { m.moveSection(1) })},
		{keys: "shift+tab", do: run(func(m *model) { m.moveSection(-1) })},
		{keys: "q", hint: "[q/a] scroll changes", do: run(func(m *model) { m.changes.back++ })}, // the pane clamps it
		{keys: "a", do: run(func(m *model) { m.changes.back = max(0, m.changes.back-1) })},
		{keys: "+", hint: "[+/−] resize", do: run(func(m *model) { m.moveDivider(1) })},
		{keys: "-", do: run(func(m *model) { m.moveDivider(-1) })},
		{keys: "Q", hint: "[Q] quit", do: func(*model, string) tea.Cmd { return tea.Quit }},
		{keys: "esc", do: func(*model, string) tea.Cmd { return tea.Quit }},
	}
}

// tuiHints are the TUI keys' hints, for the footer.
func tuiHints() string {
	var hs []string
	for _, t := range tuiKeys() {
		if t.hint != "" {
			hs = append(hs, t.hint)
		}
	}
	return strings.Join(hs, "  ")
}

// footer is the keys of what is on screen (rendered), after a recent notice
// such as an error.
func (m *model) footer(keys string) string {
	if m.notice != "" && time.Since(m.noticeAt) < 6*time.Second {
		keys = yellow.Render(m.notice) + "  " + keys
	}
	return clip(keys, m.width)
}

// ---- items -----------------------------------------------------------------------

var plain = lipgloss.NewStyle()
