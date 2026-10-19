package tui

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fu/k7s/internal/config"
	"github.com/fu/k7s/internal/resources"
	"github.com/fu/k7s/internal/resources/commands"
)

// The page: a status line, a section per shown kind (title and table), the
// change log and the navigation bar. One row is selected; the commands the
// config binds for its section are its keys.

type page struct {
	cfg      *config.Config // which sections and row states are hidden
	selected string         // "<kind>/<row key>"
	moved    bool           // the user moved the selection; until then it stays on the top row
	offset   int            // first visible body line
	jumpBuf  string
	jumpAt   time.Time
	grow     int                  // lines the change log got (Q) or gave (A) over its default height
	sorts    map[string]sortState // per section: the column Shift+key sorted it to
	rail     railFlow             // the flowing link rail down the left edge
}

func (p *page) reset() {
	p.selected, p.moved, p.offset, p.jumpBuf = "", false, 0, ""
	p.sorts = nil
	p.rail.reset()
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
	hint string // empty: bound but not shown (a second key, e.g. enter)
	do   func() resources.Step
}

// actions are the selected row's commands the config offers, with their
// keys: the kind's own commands, then those for every kind (only on rows
// that show an object).
func (m *model) actions() []action {
	k, key := m.selection()
	if k == nil {
		return nil
	}
	var ref resources.Ref
	var sub string
	for _, r := range m.rows(k) {
		if r.Key == key {
			ref = r.Ref
			sub = r.Sub
		}
	}
	cmds := kindCommands(k.Name())
	if ref.Kind != "" {
		cmds = append(cmds, commands.All()...)
	}
	target := resources.Target{Host: kindHost{m, k}, Kind: k, Key: key, Ref: ref, Sub: sub}
	var out []action
	for _, c := range cmds {
		if c.Enabled != nil && !c.Enabled(target) {
			continue
		}
		for i, keyName := range m.page.cfg.CommandKeys(k.Name(), c.Name) {
			a := action{key: keyName, do: func() resources.Step { return c.Run(target) }}
			if i == 0 {
				a.hint = commandHint(c.Name, keyName)
			}
			out = append(out, a)
		}
	}
	return out
}

// commandHint shows a command with its key: the key's letter in the name
// ("s[h]ell"), else before it ("[x] shell", "[enter] open").
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

func (m *model) pageKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()
	switch key {
	case "c":
		return m.openContexts()
	case "n":
		return m.openNamespaces()
	case "f":
		return m.openFilter()
	}
	if isDigit(key) {
		// Jump to row #n of the section the cursor is in.
		if k, _ := m.selection(); k != nil {
			ks := keys(m.rows(k))
			if n := typeNumber(&m.page.jumpBuf, &m.page.jumpAt, key); n >= 1 && n <= len(ks) {
				m.page.selected, m.page.moved = k.Name()+"/"+ks[n-1], true
			}
		}
		return nil
	}
	// Shift+<section key> (the key's capital) cycles that section's column sort.
	if len(key) == 1 && 'A' <= key[0] && key[0] <= 'Z' {
		lower := strings.ToLower(key)
		for _, k := range m.kinds {
			if m.page.shown(k) && k.Available() && toggleKey(k) == lower {
				m.page.cycle(k.Name(), k.Columns())
				return nil
			}
		}
	}
	for _, k := range m.kinds {
		if key == toggleKey(k) {
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
	}
	for _, a := range m.actions() {
		if a.key == key {
			cmd, _ := m.follow(a.do())
			return cmd
		}
	}
	page := m.layout().body
	switch key {
	case "esc":
		return tea.Quit
	case "up", "k":
		m.moveSelection(-1)
	case "down", "j":
		m.moveSelection(1)
	case "pgup":
		m.moveSelection(-page)
	case "pgdown":
		m.moveSelection(page)
	case "home":
		m.moveSelection(-1 << 30)
	case "end":
		m.moveSelection(1 << 30)
	case "q":
		m.changes.back++ // the pane clamps it
	case "a":
		m.changes.back = max(0, m.changes.back-1)
	case "Q": // move the divider up: a taller change log
		m.page.grow = m.layout().changes - m.defaultChanges() + 1
		m.saveChangesPct()
	case "A": // and down
		m.page.grow = m.layout().changes - m.defaultChanges() - 1
		m.saveChangesPct()
	}
	return nil
}

// saveChangesPct folds the divider's current position into the config so the
// next start opens with the same split. The in-session offset (page.grow) is
// left alone: defaultChanges() is fixed within a session, so grow keeps
// meaning "lines off from default".
func (m *model) saveChangesPct() {
	if m.page.cfg == nil {
		return
	}
	avail := m.height - 2 - 1 - 2 - 3
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
	avail := m.height - 2 - 1 - 2 - 3
	if avail <= 0 {
		return
	}
	m.page.grow = avail*pct/100 - m.defaultChanges()
}

func (m *model) layout() layout {
	// status line + blank + body + changes rule + changes + two nav bar lines;
	// the change log keeps at least 2 lines, the sections at least 3.
	l := layout{changes: max(2, min(m.height-2-1-2-3, m.defaultChanges()+m.page.grow))}
	l.body = max(1, m.height-2-1-l.changes-2)
	return l
}

func (m *model) statusLine() string {
	filter := "filter " + dim.Render("none")
	if m.info.Selector != "" {
		filter = "filter " + cyan.Render(m.info.Selector)
	}
	parts := []string{badge.Render("k7s"), "ctx " + cyan.Render(m.info.Context), "ns " + cyan.Render(m.info.Namespace), filter}
	for _, k := range m.sections() {
		if s := m.sectionStatus(k); s != "" {
			parts = append(parts, s)
		}
	}
	ready := true
	for _, k := range m.sections() {
		ready = ready && k.Ready()
	}
	right := yellow.Render("○ syncing")
	if ready {
		right = m.pulse() + " " + cyan.Render("watching")
	}
	return alignLR(joinParts(parts), right, m.width)
}

// sectionStatus is a section's part of the status line: its name and the
// items the config picks (its own indicators, and "hidden": rows hidden by
// state), those with something to say; empty if none has.
func (m *model) sectionStatus(k resources.Resource) string {
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
	if len(items) == 0 {
		return ""
	}
	return k.Name() + " " + strings.Join(items, " ")
}

func (m *model) pageView() string {
	l := m.layout()
	var b strings.Builder
	b.WriteString(m.statusLine())
	b.WriteString("\n\n")

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

	// Every shown section's lines, remembering where the selected row is. The
	// panes are drawn one column narrower to leave room for the link rail, and
	// each line carries a rail role (spine / linked / selected) alongside it.
	pw := m.width - railWidth // pane width, inside the rail
	var lines []string
	var role []int
	selLine := 0
	// add appends a pane line and its rail role together.
	add := func(r int, s string) {
		role = append(role, r)
		lines = append(lines, s)
	}
	for _, k := range m.sections() {
		rows := m.rows(k)
		if len(lines) > 0 {
			add(railRoleRun, "")
		}
		add(railRoleRun, header.Render(k.Name()))
		if len(rows) == 0 {
			empty := k.Empty()
			if n := m.hiddenRows(k); n > 0 {
				empty = fmt.Sprintf("all %d hidden by state (f to change)", n)
			}
			add(railRoleRun, dim.Render("  "+empty))
			continue
		}
		sel := ""
		if k.Name() == selName {
			sel = selKey
			if i := slices.Index(keys(rows), selKey); i >= 0 {
				selLine = len(lines) + 1 + i
			}
		}
		hdr, body := table{cols: m.sortCols(k), rows: rows, number: true, linked: links}.lines(sel, pw)
		add(railRoleRun, hdr)
		for i, r := range rows {
			switch {
			case k.Name() == selName && r.Key == selKey:
				add(railRoleSel, body[i])
			case links[r.Ref]:
				add(railRoleLink, body[i])
			default:
				add(railRoleRun, body[i])
			}
		}
	}
	if len(lines) == 0 {
		var letters []string
		for _, k := range m.kinds {
			letters = append(letters, toggleKey(k))
		}
		add(railRoleRun, dim.Render("  all sections hidden: press "+strings.Join(letters, ", ")+" to show them"))
	}

	m.page.offset = scrollOffset(selLine, m.page.offset, l.body, len(lines))
	now := time.Now()
	m.page.rail.advance(l.body, now, m.changes.lastWarn)
	for i := 0; i < l.body; i++ {
		idx := m.page.offset + i
		b.WriteString(railDraw(roleAt(role, idx), m.page.rail.cell(i)))
		if idx < len(lines) {
			b.WriteString(lines[idx])
		}
		b.WriteByte('\n')
	}

	b.WriteString(m.changes.view(l.changes, m.width))

	var cmds []string
	for _, a := range m.actions() {
		if a.hint != "" {
			cmds = append(cmds, a.hint)
		}
	}
	line := strings.Join(cmds, "  ")
	if m.page.jumpBuf != "" && time.Since(m.page.jumpAt) < jumpWindow {
		line = "go to #[" + m.page.jumpBuf + "]"
	}
	tui := hints("[shift+section] sort  [q/a] scroll changes  [Q/A] resize TUI  [esc] quit")
	b.WriteString(m.navBarCommands(hints(line), tui))
	b.WriteByte('\n')
	b.WriteString(m.navBarResources())
	return b.String()
}

// pulse is a dot that breathes once per metrics refresh: brightest the moment
// fresh metrics arrive, darkest halfway to the next refresh.
func (m *model) pulse() string {
	period := m.info.Refresh.Seconds()
	since := time.Since(m.pulseAt).Seconds()
	if m.pulseAt.IsZero() {
		since = float64(time.Now().UnixMilli()) / 1000
	}
	phase := math.Mod(since, period) / period
	k := (1 + math.Cos(2*math.Pi*phase)) / 2 // 1 at a refresh → 0 → 1
	mix := func(from, to float64) int { return int(from + (to-from)*k) }
	color := fmt.Sprintf("#%02x%02x%02x", mix(0x0b, 0x00), mix(0x4f, 0xe5), mix(0x5c, 0xff))
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render("●")
}

// ---- jump to #n ------------------------------------------------------------------

const jumpWindow = 1500 * time.Millisecond

// typeNumber adds a typed digit to buf and returns the number typed so far;
// digits typed within jumpWindow of each other form one number.
func typeNumber(buf *string, at *time.Time, digit string) int {
	if time.Since(*at) > jumpWindow {
		*buf = ""
	}
	if *buf == "" && digit == "0" {
		return 0
	}
	*buf += digit
	*at = time.Now()
	var n int
	fmt.Sscan(*buf, &n)
	return n
}

func isDigit(key string) bool { return len(key) == 1 && key[0] >= '0' && key[0] <= '9' }
