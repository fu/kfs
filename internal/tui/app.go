// Package tui draws kfs: one page for a namespace (and filter) with a
// section per kind and a change log underneath, the navigation bar, and the
// screens every kind's steps open (pickers, viewers, interactive programs).
//
// It knows nothing about specific kinds beyond the registry below: they say
// what to show (resources.Row, resources.Step), the tui says how.
package tui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/fu/kfs/internal/config"
	"github.com/fu/kfs/internal/kube"
	"github.com/fu/kfs/internal/resources"
	"github.com/fu/kfs/internal/resources/configmaps"
	"github.com/fu/kfs/internal/resources/deployments"
	"github.com/fu/kfs/internal/resources/gitops"
	"github.com/fu/kfs/internal/resources/ingressroutes"
	"github.com/fu/kfs/internal/resources/pods"
	podcommands "github.com/fu/kfs/internal/resources/pods/commands"
	"github.com/fu/kfs/internal/resources/pvcs"
	"github.com/fu/kfs/internal/resources/pvs"
	"github.com/fu/kfs/internal/resources/replicasets"
	"github.com/fu/kfs/internal/resources/secrets"
	secretcommands "github.com/fu/kfs/internal/resources/secrets/commands"
	"github.com/fu/kfs/internal/resources/services"
)

// registry lists the kinds in display order. Each name's first letter (or
// the key it names) toggles its section, so it must not clash with another
// key of the page. A kind with commands of its own gets them here; the
// header's commands line shows them after those every row has.
func registry() []resources.Resource {
	return []resources.Resource{
		gitops.New(),
		deployments.New(),
		replicasets.New(),
		pods.New(podcommands.All()...),
		services.New(),
		ingressroutes.New(),
		configmaps.New(),
		pvcs.New(),
		pvs.New(),
		secrets.New(secretcommands.All()...),
	}
}

// ordered are the kinds in the config's resource_order (top to bottom);
// kinds it does not list keep their registry order, after the others.
func ordered(cfg *config.Config, kinds []resources.Resource) []resources.Resource {
	byName := map[string]resources.Resource{}
	names := make([]string, len(kinds))
	for i, k := range kinds {
		byName[k.Name()], names[i] = k, k.Name()
	}
	out := make([]resources.Resource, 0, len(kinds))
	for _, n := range cfg.Order(names) {
		out = append(out, byName[n])
	}
	return out
}

// Info describes what is being watched.
type Info struct {
	Context   string
	Server    string
	Namespace string
	Selector  string
	Refresh   time.Duration
	Notice    string // shown in the navigation bar at start
	// Config has the colors and the state to come back to (hidden
	// sections, last namespaces); changes are saved to it.
	Config *config.Config
}

// screen is what opens on top of the page: a picker or a viewer.
type screen interface {
	// update handles keys and the screen's own messages; done closes it.
	update(msg tea.Msg) (cmd tea.Cmd, done bool)
	// view renders a title line, an empty line and the body in width × height.
	view(width, height int) string
	hints() string
}

type model struct {
	ctx    context.Context
	flags  *genericclioptions.ConfigFlags // to build clients for other contexts
	client *kube.Client                   // the context in use
	send   func(tea.Msg)
	info   Info

	kinds   []resources.Resource
	started map[string]bool // which kinds are currently watching (lazy-loaded)
	screens []screen        // the top one is shown
	page    page
	changes changeLog

	width, height int
	notice        string
	noticeAt      time.Time
	pulseAt       time.Time // when fresh metrics last arrived

	listed   []resources.Ref
	listedAt time.Time
}

// Run shows the page and blocks until the user quits.
func Run(ctx context.Context, flags *genericclioptions.ConfigFlags, client *kube.Client, info Info) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// kfs picks its own colors: don't ask the terminal for its background
	// (a terminal that never answers would stall the start for seconds).
	lipgloss.SetHasDarkBackground(true)
	if info.Config == nil {
		info.Config = &config.Config{} // nothing to restore, nothing saved
	}
	setPalette(info.Config.Colors)

	m := &model{ctx: ctx, flags: flags, client: client, info: info, kinds: ordered(info.Config, registry()), started: map[string]bool{}}
	m.page.cfg = info.Config
	m.changes.rail.window = info.Config.RailWindow()
	// Mouse reporting for the wheel / trackpad (select text with Shift, or
	// Option in macOS Terminal and iTerm2).
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithContext(ctx))
	m.send = p.Send
	if info.Notice != "" {
		m.Notice(info.Notice)
		m.noticeAt = time.Now().Add(4 * time.Second) // a little longer at start
	}
	if _, err := p.Run(); err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return err
	}
	return nil
}

// pulseMsg redraws the "watching" dot often enough for a smooth fade.
type pulseMsg struct{}

func pulseTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return pulseMsg{} })
}

func (m *model) Init() tea.Cmd { return tea.Batch(pulseTick(), m.start()) }

// start (re)starts every visible kind and the event watch for the current
// scope; hidden kinds are left idle until toggled on.
func (m *model) start() tea.Cmd {
	m.changes.reset()
	m.page.reset()
	m.listedAt = time.Time{}
	m.started = map[string]bool{}
	m.watchEvents()
	var cmds []tea.Cmd
	for _, k := range m.kinds {
		if m.page.shown(k) {
			cmds = append(cmds, k.Start(kindHost{m, k}))
			m.started[k.Name()] = true
		}
	}
	return tea.Batch(cmds...)
}

// kindHost is the Host a kind gets: the model, tagging what the kind logs
// with its two-letter tag (unless the change has one already).
type kindHost struct {
	*model
	kind resources.Resource
}

func (h kindHost) Log(c resources.Change) {
	if c.Tag == "" {
		c.Tag = h.kind.Tag()
	}
	h.model.Log(c)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.applySavedPct()
		return m, nil
	case pulseMsg:
		return m, pulseTick()
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// The top screen gets every key; the page's keys work on the page.
		if top := m.top(); top != nil {
			cmd, done := top.update(msg)
			if done {
				m.close(top)
			}
			return m, cmd
		}
		return m, m.pageKey(msg)
	case tea.MouseMsg:
		return m, m.wheel(msg)
	case execDoneMsg:
		if msg.err != nil {
			m.Notice(msg.err.Error())
		}
		return m, nil
	case eventMsg:
		m.addEvent(msg)
		return m, nil
	}

	// Everything else belongs to a kind or a screen; each ignores what is
	// not its own.
	var cmds []tea.Cmd
	for _, k := range m.kinds {
		cmds = append(cmds, k.Update(msg))
	}
	for _, s := range slices.Clone(m.screens) {
		cmd, done := s.update(msg)
		if done {
			m.close(s)
		}
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

// follow shows what a step leads to. opened reports whether a screen (or
// program) came up, so a picker that led to it stays underneath.
func (m *model) follow(step resources.Step) (cmd tea.Cmd, opened bool) {
	switch s := step.(type) {
	case *resources.Picker:
		return m.openPicker(s), true
	case *resources.Viewer:
		return m.openViewer(s), true
	case *resources.Exec:
		return runExec(s), true
	case resources.Cmd:
		return tea.Cmd(s), false
	}
	return nil, false
}

func (m *model) open(s screen) { m.screens = append(m.screens, s) }

func (m *model) top() screen {
	if len(m.screens) == 0 {
		return nil
	}
	return m.screens[len(m.screens)-1]
}

// close removes screen s (which may be covered by one it opened).
func (m *model) close(s screen) {
	if i := slices.Index(m.screens, s); i >= 0 {
		m.screens = slices.Delete(m.screens, i, i+1)
	}
}

// closeAll leaves every screen the way esc would, so streams stop.
func (m *model) closeAll() {
	for len(m.screens) > 0 {
		top := m.top()
		top.update(tea.KeyMsg{Type: tea.KeyEsc})
		m.close(top)
	}
}

// scopeChanged restarts everything for the new context, namespace or filter,
// and remembers the namespace for the context.
func (m *model) scopeChanged() tea.Cmd {
	m.closeAll()
	if c := m.info.Config; c != nil {
		c.SetNamespace(m.info.Context, m.info.Namespace)
		if err := c.Save(); err != nil {
			m.Notice("config: " + err.Error())
		}
	}
	return m.start()
}

func (m *model) View() string {
	if m.width == 0 {
		return "starting…"
	}
	if top := m.top(); top != nil {
		h := max(1, m.height-headerLines-footerLines)
		return m.headerView() + "\n" + fitLines(top.view(m.width, h), h) + "\n" + m.footer(top.hints())
	}
	return m.pageView()
}

// fitLines pads or cuts s to exactly n lines.
func fitLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	for len(lines) < n {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// ---- resources.Host -------------------------------------------------------------

var _ resources.Host = (*model)(nil)

func (m *model) Context() context.Context { return m.ctx }
func (m *model) Client() *kube.Client     { return m.client }
func (m *model) Namespace() string        { return m.info.Namespace }
func (m *model) Selector() string         { return m.info.Selector }
func (m *model) Refresh() time.Duration   { return m.info.Refresh }
func (m *model) Send(msg tea.Msg)         { m.send(msg) }
func (m *model) Log(c resources.Change)   { m.changes.add(c) }
func (m *model) Notice(s string)          { m.notice, m.noticeAt = s, time.Now() }
func (m *model) Pulse(at time.Time)       { m.pulseAt = at }

// Listed collects what all sections list. It is asked often while drawing
// (relevance of log entries), so it is cached briefly.
func (m *model) Listed() []resources.Ref {
	if time.Since(m.listedAt) > 200*time.Millisecond {
		m.listed = m.listed[:0]
		for _, k := range m.sections() {
			m.listed = append(m.listed, k.Listed()...)
		}
		m.listedAt = time.Now()
	}
	return m.listed
}

// toggleKey is the key that shows or hides a kind's section: the first
// letter of its name, unless it names another one.
func toggleKey(k resources.Resource) string {
	if kk, ok := k.(resources.Keyed); ok {
		return kk.Key()
	}
	return k.Name()[:1]
}
