package tui

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fu/kfs/internal/config"
	"github.com/fu/kfs/internal/resources"
	"github.com/fu/kfs/internal/resources/commands"
)

// The page: a status line, a section per shown kind (title and table), the
// change log and the navigation bar. One row is selected; the commands the
// config binds for its section are its keys.

type page struct {
	cfg      *config.Config       // which sections and row states are hidden
	selected string               // "<kind>/<row key>"
	moved    bool                 // the user moved the selection; until then it stays on the top row
	offset   int                  // first visible body line
	grow     int                  // lines the change log got (Q) or gave (A) over its default height
	sorts    map[string]sortState // per section: the column Shift+key sorted it to
}

func (p *page) reset() {
	p.selected, p.moved, p.offset = "", false, 0
	p.sorts = nil
}

func (p *page) shown(k resources.Resource) bool { return p.cfg == nil || !p.cfg.Hidden(k.Name()) }

func (p *page) toggle(k resources.Resource) {
	if p.cfg == nil {
		p.cfg = &config.Config{}
	}
	p.cfg.SetHidden(k.Name(), p.shown(k))
}

// sections are the kinds shown on the page.
func (m *model) sections() []resources.Resource {
	var out []resources.Resource
	for _, k := range m.kinds {
		if m.page.shown(k) && k.Available() {
			out = append(out, k)
		}
	}
	return out
}

// rows are a kind's rows without those in hidden states, in the page's sort
// order for that section (natural order until Shift+key engages one).
func (m *model) rows(k resources.Resource) []resources.Row {
	all := k.Rows()
	out := all[:0:0]
	for _, r := range all {
		if !m.page.cfg.StateHidden(k.Name(), r.State) {
			out = append(out, r)
		}
	}
	// Only re-order once the user has cycled a sort for this section; the
	// natural (NAME-up) default is the order the kind already lists in.
	if s, ok := m.page.sorts[k.Name()]; ok {
		out = sortRows(out, s)
	}
	return out
}

// hiddenRows counts a kind's rows in hidden states.
func (m *model) hiddenRows(k resources.Resource) int {
	return len(k.Rows()) - len(m.rows(k))
}

func keys(rows []resources.Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Key
	}
	return out
}

// entries lists the selectable rows of the shown sections, in order.
func (m *model) entries() []string {
	var out []string
	for _, k := range m.sections() {
		for _, key := range keys(m.rows(k)) {
			out = append(out, k.Name()+"/"+key)
		}
	}
	return out
}

// selection returns the kind and row key of the selected row.
func (m *model) selection() (resources.Resource, string) {
	name, key, _ := strings.Cut(m.page.selected, "/")
	for _, k := range m.kinds {
		if k.Name() == name && slices.Contains(keys(m.rows(k)), key) {
			return k, key
		}
	}
	return nil, ""
}

// action is a command bound to a key for the selected row.
type action struct {
	key  string
	name string // the command
	hint string // empty: bound but not shown (a second key, or enter)
	do   func() resources.Step
}

// rowCommands are the selected row's commands the config offers, with their
// keys, in the two groups of the header's commands line: those every row
// has (resources/commands, only on rows that show an object) and those its
// kind adds (a resources.Commander).
func (m *model) rowCommands() (every, own []action) {
	k, key := m.selection()
	if k == nil {
		return nil, nil
	}
	var ref resources.Ref
	var sub string
	for _, r := range m.rows(k) {
		if r.Key == key {
			ref = r.Ref
			sub = r.Sub
		}
	}
	target := resources.Target{Host: kindHost{m, k}, Kind: k, Key: key, Ref: ref, Sub: sub}
	bind := func(cmds []resources.Command) []action {
		var out []action
		for _, c := range cmds {
			if c.Enabled != nil && !c.Enabled(target) {
				continue
			}
			hinted := false
			for _, keyName := range m.page.cfg.CommandKeys(k.Name(), c.Name) {
				a := action{key: keyName, name: c.Name, do: func() resources.Step { return c.Run(target) }}
				// enter is what a row does by default (describe): not worth a hint.
				if !hinted && keyName != "enter" {
					a.hint, hinted = commandHint(c.Name, keyName), true
				}
				out = append(out, a)
			}
		}
		return out
	}
	if ref.Kind != "" {
		every = bind(commands.All())
	}
	if c, ok := k.(resources.Commander); ok {
		own = bind(c.Commands())
	}
	return every, own
}

// actions are all the selected row's commands: its kind's own, then those
// every row has.
func (m *model) actions() []action {
	every, own := m.rowCommands()
	return append(own, every...)
}

// commandItems are what the selected row can do, for the bottom of its
// resource's frame: each command with its key lit in the word ("logs"),
// those every row has and sorting in key-global, the kind's own in
// key-command.
func (m *model) commandItems() []string {
	every, own := m.rowCommands()
	var out []string
	add := func(as []action, key lipgloss.Color) {
		for _, a := range as {
			if a.hint != "" {
				out = append(out, marked(a.hint, plain, key))
			}
		}
	}
	add(own, commandKey)
	add(every, globalKey)
	return append(out, marked("[`] sort", plain, globalKey))
}

// commandHint shows a command with its key: the key's letter in the name
// ("[s]hell"), else before it ("[x] open", "[F1] pods").
func commandHint(name, key string) string {
	if len(key) == 1 {
		if i := strings.Index(name, key); i >= 0 {
			return name[:i] + "[" + key + "]" + name[i+1:]
		}
	}
	return "[" + key + "] " + name
}

func (m *model) moveSelection(delta int) {
	entries := m.entries()
	if len(entries) == 0 {
		return
	}
	i := max(0, slices.Index(entries, m.page.selected))
	m.page.selected, m.page.moved = entries[clamp(i, delta, len(entries))], true
}

// moveSection moves the selection to the first row of the next (delta 1) or
// previous (-1) section that has rows, cycling round.
func (m *model) moveSection(delta int) {
	var firsts []string // each section's first row
	cur := 0
	for _, k := range m.sections() {
		ks := keys(m.rows(k))
		if len(ks) == 0 {
			continue
		}
		if name, _, _ := strings.Cut(m.page.selected, "/"); name == k.Name() {
			cur = len(firsts)
		}
		firsts = append(firsts, k.Name()+"/"+ks[0])
	}
	if len(firsts) == 0 {
		return
	}
	n := len(firsts)
	m.page.selected, m.page.moved = firsts[((cur+delta)%n+n)%n], true
}

// pageKey goes through the groups in turn: the scopes, the sections, the
// TUI's own keys, then the selected row's commands (so the page's keys win).
func (m *model) pageKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()
	for _, s := range scopes() {
		if key == s.key {
			return s.open(m)
		}
	}
	for i, k := range m.kinds {
		if key == sectionKey(i) {
			return m.toggleSection(k)
		}
	}
	if key == sortKey {
		if k, _ := m.selection(); k != nil {
			m.page.cycle(k.Name(), k.Columns())
		}
		return nil
	}
	for _, t := range tuiKeys() {
		if t.matches(key) {
			return t.do(m, key)
		}
	}
	for _, a := range m.actions() {
		if a.key == key {
			cmd, _ := m.follow(a.do())
			return cmd
		}
	}
	return nil
}

// toggleSection shows or hides k's section, starting or stopping its watch.
func (m *model) toggleSection(k resources.Resource) tea.Cmd {
	m.page.toggle(k)
	var cmd tea.Cmd
	if m.page.shown(k) && !m.started[k.Name()] {
		cmd = k.Start(kindHost{m, k})
		m.started[k.Name()] = true
	} else if !m.page.shown(k) && m.started[k.Name()] {
		k.Stop()
		delete(m.started, k.Name())
	}
	m.listedAt = time.Time{} // invalidate cache: sections changed
	if err := m.page.cfg.Save(); err != nil {
		m.Notice("config: " + err.Error())
	}
	return cmd
}

// moveDivider moves the divider by delta lines (up: a taller change log).
func (m *model) moveDivider(delta int) {
	m.page.grow = m.layout().changes - m.defaultChanges() + delta
	m.saveChangesPct()
}

// saveChangesPct folds the divider's current position into the config so the
// next start opens with the same split. The in-session offset (page.grow) is
// left alone: defaultChanges() is fixed within a session, so grow keeps
// meaning "lines off from default".
func (m *model) saveChangesPct() {
	if m.page.cfg == nil {
		return
	}
	avail := m.maxChanges()
	if avail <= 0 {
		return
	}
	m.page.cfg.SetChangesPct(m.layout().changes * 100 / avail)
	_ = m.page.cfg.Save()
}

// ---- rendering ---------------------------------------------------------------

type layout struct {
	body    int // lines for the sections
	changes int
}

// defaultChanges is the change log's height before any resize: kept stable
// within a session so Q / A move the divider by one line and nothing else.
// The pct saved in the config is applied once, at start and on resize,
// through applySavedPct.
func (m *model) defaultChanges() int { return max(4, min(12, m.height/3)) }

// applySavedPct sets page.grow from the change-log pct the config saved, so
// the page opens (and re-opens on resize) at the remembered split. Called on
// WindowSizeMsg, which fires at start and on every resize.
func (m *model) applySavedPct() {
	pct := m.page.cfg.ChangesPct()
	if pct <= 0 {
		return
	}
	avail := m.maxChanges()
	if avail <= 0 {
		return
	}
	m.page.grow = avail*pct/100 - m.defaultChanges()
}

// The page from top to bottom: the header, the sections, the change log in
// its rail, the footer.
const (
	changesFrame = 2 // the rail above and below the change log
	minBody      = 3 // the sections keep at least 3 lines
	minChanges   = 2 // and the change log 2
)

// maxChanges is the most lines the change log can take.
func (m *model) maxChanges() int {
	return m.height - m.headerLines() - changesFrame - footerLines - minBody
}

func (m *model) layout() layout {
	l := layout{changes: max(minChanges, min(m.maxChanges(), m.defaultChanges()+m.page.grow))}
	l.body = max(1, m.height-m.headerLines()-changesFrame-l.changes-footerLines)
	return l
}

// statusLine is the header frame's top border: the kfs badge, context,
// namespace and filter, and at the right whether all sections are watching.
func (m *model) statusLine() string {
	filter := "filter: " + dim.Render("none")
	if m.info.Selector != "" {
		filter = "filter: " + cyan.Render(m.info.Selector)
	}
	left := []string{badge.Render("kfs"), "ctx: " + cyan.Render(m.info.Context), "ns: " + cyan.Render(m.info.Namespace), filter}
	ready := true
	for _, k := range m.sections() {
		ready = ready && k.Ready()
	}
	right := yellow.Render("○ syncing")
	if ready {
		right = m.pulse() + " " + cyan.Render("watching")
	}
	return frameBar(frameStyle("kfs"), left, []string{right}, m.width)
}

// sectionStatus is what a section's frame shows at the bottom right: the
// items the config picks (its own indicators, and "hidden": rows hidden by
// state), those with something to say.
func (m *model) sectionStatus(k resources.Resource) []string {
	byName := map[string]resources.Indicator{}
	for _, ind := range k.Indicators() {
		byName[ind.Name] = ind
	}
	var items []string
	for _, name := range m.page.cfg.StatusItems(k.Name()) {
		var cs []resources.Cell
		switch ind, ok := byName[name]; {
		case ok:
			cs = ind.Cells()
		case name == "hidden":
			if n := m.hiddenRows(k); n > 0 {
				cs = []resources.Cell{{Text: fmt.Sprintf("(+%d hidden)", n), Tone: resources.Dim}}
			}
		}
		if len(cs) > 0 {
			items = append(items, cells(cs))
		}
	}
	return items
}

func (m *model) pageView() string {
	l := m.layout()
	entries := m.entries()
	if (!m.page.moved || !slices.Contains(entries, m.page.selected)) && len(entries) > 0 {
		m.page.selected = entries[0]
	}
	selName, selKey, _ := strings.Cut(m.page.selected, "/")

	// What the selected row links to, in any section.
	var selRef resources.Ref
	for _, k := range m.sections() {
		if k.Name() == selName {
			for _, r := range m.rows(k) {
				if r.Key == selKey {
					selRef = r.Ref
				}
			}
		}
	}
	links := linked(selRef, m.pageLinks())

	// Every shown section in its frame, remembering where the selected row
	// is. The frame's left border points at the selected row and lights a
	// bulb by every row linked to it (not by sub-rows: their row has it).
	inner := m.width - 2
	bulb := m.pulse()
	var lines []string
	selLine, selTop := 0, 0 // the selected row's line, and its section's top border
	for _, k := range m.sections() {
		rows := m.rows(k)
		c := frameStyle(k.Name())
		top := len(lines)
		var labels []label
		var body []string
		if len(rows) == 0 {
			empty := k.Empty()
			if n := m.hiddenRows(k); n > 0 {
				empty = fmt.Sprintf("all %d hidden by state (3 to change)", n)
			}
			body = []string{dim.Render(" " + empty)}
		} else {
			sel := ""
			if k.Name() == selName {
				sel, selTop = selKey, top
				if i := slices.Index(keys(rows), selKey); i >= 0 {
					selLine = top + 1 + i
				}
			}
			labels, body = table{cols: m.sortCols(k), rows: rows, number: true, linked: links}.framed(sel, inner)
		}
		lines = append(lines, frameTop(c, k.Name(), labels, m.width))
		for i, line := range body {
			left := c.Render("│")
			if i < len(rows) {
				switch r := rows[i]; {
				case k.Name() == selName && r.Key == selKey:
					left = railCursorStyle.Render(railCursor)
				case r.Sub == "" && links[r.Ref]:
					left = bulb
				}
			}
			lines = append(lines, frameRow(c, left, line, m.width))
		}
		var cmds []string
		if k.Name() == selName && len(rows) > 0 {
			cmds = m.commandItems()
		}
		lines = append(lines, frameBottom(c, cmds, m.sectionStatus(k), m.width))
	}
	if len(lines) == 0 {
		last := keyLabel(sectionKey(min(len(m.kinds), 12) - 1))
		lines = append(lines, dim.Render("  all resources hidden: press F1–"+last+" to show them"))
	}
	m.page.offset = scrollOffset(selLine, m.page.offset, l.body, len(lines))
	// Keep the selected section's top border (its name and column headers)
	// in view while it fits with the selected row.
	if selLine-selTop < l.body {
		m.page.offset = min(m.page.offset, selTop)
	}

	var b strings.Builder
	b.WriteString(m.headerView() + "\n")
	for i := range l.body {
		if idx := m.page.offset + i; idx < len(lines) {
			b.WriteString(lines[idx])
		}
		b.WriteByte('\n')
	}

	// The change log, framed by the rail: it starts right after the title.
	title, entries := m.changes.body(l.changes, max(1, m.width-2))
	title = header.Render(title)
	rf := m.changes.rail.frame(m.width, l.changes, lipgloss.Width(title), time.Now())
	b.WriteString(rf.topLeft + title + rf.top + "\n")
	for i, e := range entries {
		e = clip(e, max(0, m.width-2))
		b.WriteString(rf.left[i] + e + strings.Repeat(" ", max(0, m.width-2-lipgloss.Width(e))) + rf.right[i] + "\n")
	}
	b.WriteString(rf.bottom + "\n")

	b.WriteString(m.footer(hints(tuiHints())))
	return b.String()
}

// pulse is a dot that breathes once per metrics refresh: brightest the moment
// fresh metrics arrive, darkest halfway to the next refresh.
// It is also the bulb on the rail by every row linked to the selected one.
func (m *model) pulse() string {
	period := m.info.Refresh.Seconds()
	since := time.Since(m.pulseAt).Seconds()
	if m.pulseAt.IsZero() {
		since = float64(time.Now().UnixMilli()) / 1000
	}
	phase := math.Mod(since, period) / period
	k := (1 + math.Cos(2*math.Pi*phase)) / 2 // 1 at a refresh → 0 → 1
	return lipgloss.NewStyle().Foreground(shade(string(pulseColor), 0.35+0.65*k)).Render("●")
}
