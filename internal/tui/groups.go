package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// What kfs offers comes in groups. The header has a line for each, the logo
// down its left edge; the footer has the keys that work the TUI itself:
//
//	k │ status: context, namespace, filter, watching                statusLine
//	f │ scopes, every row's commands │ the row's kind's own         commandLine
//	s │ the sections, shown or hidden                              sectionLine
//	    … sections, change log …
//	    the TUI's own keys                                         footer

const (
	logo        = "kfs" // a letter per header line
	headerLines = len(logo)
	footerLines = 1
)

// logoCell is the logo's letter at the start of header line i.
func logoCell(i int) string { return badge.Render(logo[i:i+1]) + " " }

func (m *model) headerView() string {
	return m.statusLine() + "\n" + m.commandLine() + "\n" + m.sectionLine()
}

// ---- f: scopes and commands ----------------------------------------------------

// scope changes what the page shows, through a picker.
type scope struct {
	name, key string
	open      func(*model) tea.Cmd
}

func scopes() []scope {
	return []scope{
		{"contexts", "c", (*model).openContexts},
		{"namespaces", "n", (*model).openNamespaces},
		{"filter", "f", (*model).openFilter},
	}
}

// commandLine: the scopes and the commands every row has (rowCommands),
// then those the selected row's kind adds. A screen takes every key, so
// over one only the scopes show, the open one lit.
func (m *model) commandLine() string {
	open := ""
	if p, ok := m.top().(*pickerScreen); ok {
		open = p.scope
	}
	var items []string
	for _, s := range scopes() {
		st := plain
		if s.name == open {
			st = menu
		}
		items = append(items, item(commandHint(s.name, s.key), st))
	}
	line := joinParts(items)
	if m.top() == nil {
		every, own := m.rowCommands()
		line = joinParts(append(items, actionItems(every)...))
		if k, _ := m.selection(); len(own) > 0 {
			// Set apart: the heavier rule, and whose commands they are.
			line += dim.Render(" ┃ ") + header.Render(k.Name()) + " " + joinParts(actionItems(own))
		}
	}
	return clip(logoCell(1)+line, m.width)
}

// actionItems are the commands' items (a command on several keys once).
func actionItems(as []action) []string {
	var out []string
	for _, a := range as {
		if a.hint != "" {
			out = append(out, item(a.hint, plain))
		}
	}
	return out
}

// ---- s: sections -----------------------------------------------------------------

// sectionLine: every section by its toggle key, on a grey ground while
// shown; Shift+its key sorts it.
func (m *model) sectionLine() string {
	var items []string
	for _, k := range m.kinds {
		st := dim
		if m.page.shown(k) {
			st = menu
		}
		items = append(items, item(commandHint(k.Name(), toggleKey(k)), st))
	}
	items = append(items, hints("[shift+key] sort column"))
	return clip(logoCell(2)+joinParts(items), m.width)
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
		{match: isDigit, hint: "[0-9] row #", do: func(m *model, key string) tea.Cmd { m.jumpTo(key); return nil }},
		{keys: "q", hint: "[q/a] scroll changes", do: run(func(m *model) { m.changes.back++ })}, // the pane clamps it
		{keys: "a", do: run(func(m *model) { m.changes.back = max(0, m.changes.back-1) })},
		{keys: "Q", hint: "[Q/A] resize", do: run(func(m *model) { m.moveDivider(1) })},
		{keys: "A", do: run(func(m *model) { m.moveDivider(-1) })},
		{keys: "esc", hint: "[esc] quit", do: func(*model, string) tea.Cmd { return tea.Quit }},
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

// item is an entry of a header line in st (plain, dim while hidden, menu's
// grey ground while on): the hint with its key in cyan, "s[h]ell". The
// entries of a line are joined like the status line's parts (joinParts).
func item(hint string, st lipgloss.Style) string { return marked(hint, st) }
