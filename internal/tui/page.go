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
	cfg      *config.Config // which sections and row states are hidden
	selected string         // "<kind>/<row key>"
	moved    bool           // the user moved the selection; until then it stays on the top row
	offset   int            // first visible body line
	jumpBuf  string
	jumpAt   time.Time
	grow     int                  // lines the change log got (Q) or gave (A) over its default height
	sorts    map[string]sortState // per section: the column Shift+key sorted it to
}

func (p *page) reset() {
	p.selected, p.moved, p.offset, p.jumpBuf = "", false, 0, ""
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
	hint string // empty: bound but not shown (a second key, e.g. enter)
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

// wheel scrolls with the mouse wheel or trackpad: over the change log it
// scrolls the log, elsewhere on the page it moves the selection; on a screen
// it is the arrow keys.
func (m *model) wheel(msg tea.MouseMsg) tea.Cmd {
	up := msg.Button == tea.MouseButtonWheelUp
	if !up && msg.Button != tea.MouseButtonWheelDown {
		return nil
	}
	key := tea.KeyMsg{Type: tea.KeyDown}
	if up {
		key = tea.KeyMsg{Type: tea.KeyUp}
	}
	if top := m.top(); top != nil {
		cmd, done := top.update(key)
		if done {
			m.close(top)
		}
		return cmd
	}
	// the header, the rail's first slot, the sections, the divider, the log
	if l := m.layout(); msg.Y > headerLines+1+l.body {
		if up {
			m.changes.back++ // the pane clamps it
		} else {
			m.changes.back = max(0, m.changes.back-1)
		}
		return nil
	}
	if up {
		m.moveSelection(-1)
	} else {
		m.moveSelection(1)
	}
	return nil
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
	for _, k := range m.kinds {
		switch key {
		case toggleKey(k):
			return m.toggleSection(k)
		case strings.ToUpper(toggleKey(k)): // Shift+<section key>
			if m.page.shown(k) && k.Available() {
				m.page.cycle(k.Name(), k.Columns())
			}
			return nil
		}
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

// jumpTo moves to row #n of the section the cursor is in, digit by digit.
func (m *model) jumpTo(digit string) {
	if k, _ := m.selection(); k != nil {
		ks := keys(m.rows(k))
		if n := typeNumber(&m.page.jumpBuf, &m.page.jumpAt, digit); n >= 1 && n <= len(ks) {
			m.page.selected, m.page.moved = k.Name()+"/"+ks[n-1], true
		}
	}
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

// The page from top to bottom: the header, the rail's first slot, the
// sections, the change log's divider, the change log, the footer.
const (
	railSlotLines = 1
	dividerLines  = 1
	minBody       = 3 // the sections keep at least 3 lines
	minChanges    = 2 // and the change log 2
)

// maxChanges is the most lines the change log can take.
func (m *model) maxChanges() int {
	return m.height - headerLines - railSlotLines - dividerLines - footerLines - minBody
}

func (m *model) layout() layout {
	l := layout{changes: max(minChanges, min(m.maxChanges(), m.defaultChanges()+m.page.grow))}
	l.body = max(1, m.height-headerLines-railSlotLines-dividerLines-l.changes-footerLines)
	return l
}

func (m *model) statusLine() string {
	filter := "filter " + dim.Render("none")
	if m.info.Selector != "" {
		filter = "filter " + cyan.Render(m.info.Selector)
	}
	parts := []string{"ctx " + cyan.Render(m.info.Context), "ns " + cyan.Render(m.info.Namespace), filter}
	ready := true
	for _, k := range m.sections() {
		ready = ready && k.Ready()
	}
	right := yellow.Render("○ syncing")
	if ready {
		right = m.pulse() + " " + cyan.Render("watching")
	}
	return alignLR(logoCell(0)+joinParts(parts), right, m.width)
}

// sectionTitle is a section's heading: its name and the items the config
// picks (its own indicators, and "hidden": rows hidden by state), those with
// something to say.
func (m *model) sectionTitle(k resources.Resource) string {
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
	return strings.Join(append([]string{header.Render(k.Name())}, items...), " ")
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

	// Every shown section's lines, remembering where the selected row is. The
	// sections are drawn one column narrower: the rail runs left of them.
	pw := m.width - railWidth
	var lines []string
	marks := map[int]string{} // line → what the rail shows there instead of time
	selLine := 0
	for _, k := range m.sections() {
		rows := m.rows(k)
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, clip(m.sectionTitle(k), pw))
		if len(rows) == 0 {
			empty := k.Empty()
			if n := m.hiddenRows(k); n > 0 {
				empty = fmt.Sprintf("all %d hidden by state (f to change)", n)
			}
			lines = append(lines, dim.Render("  "+empty))
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
		lines = append(lines, hdr)
		// The rail points at the selected row and lights a bulb on every row
		// linked to it (not on sub-rows: their row has the bulb).
		for i, r := range rows {
			switch {
			case k.Name() == selName && r.Key == selKey:
				marks[len(lines)+i] = railCursor
			case r.Sub == "" && links[r.Ref]:
				marks[len(lines)+i] = railLinked
			}
		}
		lines = append(lines, body...)
	}
	if len(lines) == 0 {
		var letters []string
		for _, k := range m.kinds {
			letters = append(letters, toggleKey(k))
		}
		lines = append(lines, dim.Render("  all sections hidden: press "+strings.Join(letters, ", ")+" to show them"))
	}
	m.page.offset = scrollOffset(selLine, m.page.offset, l.body, len(lines))

	// The change log draws its divider; the rail is cut there, once the
	// divider's title (and so how much of it the rail runs along) is known:
	// one slot under the badge, one per section line, the corner, the run.
	var rv railView
	changes := m.changes.view(l.changes, m.width, func(title string) string {
		rv = m.changes.rail.view(1+l.body, max(0, m.width-railWidth-lipgloss.Width(title)), time.Now())
		return rv.corner + dim.Render(title) + rv.along
	})

	var b strings.Builder
	b.WriteString(m.headerView())
	b.WriteString("\n" + rv.down[0] + "\n")
	bulb := m.pulse()
	for i := 0; i < l.body; i++ {
		idx := m.page.offset + i
		switch marks[idx] {
		case railCursor:
			b.WriteString(railCursorStyle.Render(railCursor))
		case railLinked:
			b.WriteString(bulb)
		default:
			b.WriteString(rv.down[1+i])
		}
		if idx < len(lines) {
			b.WriteString(lines[idx])
		}
		b.WriteByte('\n')
	}
	b.WriteString(changes)

	keys := tuiHints()
	if m.page.jumpBuf != "" && time.Since(m.page.jumpAt) < jumpWindow {
		keys = "go to #[" + m.page.jumpBuf + "]  " + keys
	}
	b.WriteString(m.footer(hints(keys)))
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
