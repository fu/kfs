// Package ingressroutes is the Traefik IngressRoutes kind: hosts, entry
// points, routes, and the services behind them. It only shows on clusters
// that serve the IngressRoute CRD.
//
//	route.go     detection, the watch, a route as a row needs it
//	resource.go  state, change log, links
//	row.go       what a row shows
//	status.go    what it can show in the status line
package ingressroutes

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/fu/k7s/internal/kube"
	"github.com/fu/k7s/internal/resources"
)

// Resource is the ingressroutes kind. It is not Labeled: the filter still
// applies, but selector suggestions come from the kinds every cluster has.
type Resource struct {
	host   resources.Host
	client *kube.Client // the cluster detection ran on
	gen    int
	cancel context.CancelFunc

	detected bool
	gvr      schema.GroupVersionResource
	served   bool
	rows     map[string]*rowState
	order    []string
	synced   bool
	err      error
	errAt    time.Time
}

func New() *Resource { return &Resource{} }

const keepDeleted = 8 * time.Second

type (
	detectedMsg struct {
		gen    int
		gvr    schema.GroupVersionResource
		served bool
	}
	objMsg struct {
		gen              int
		row              Row
		deleted, initial bool
	}
	syncedMsg struct{ gen int }
	errMsg    struct {
		gen int
		err error
	}
	tickMsg struct{ gen int }
)

type rowState struct {
	row       Row
	changed   map[string]time.Time // column title → when it last changed
	addedAt   time.Time
	deletedAt time.Time
}

func (r *Resource) Name() string      { return "ingressroutes" }
func (r *Resource) Key() string       { return "u" } // i is services
func (r *Resource) Tag() string       { return "ir" }
func (r *Resource) Objects() []string { return []string{"IngressRoute"} }
func (r *Resource) Available() bool   { return r.detected && r.served }
func (r *Resource) Ready() bool       { return r.detected && (!r.served || r.synced) }

// States an IngressRoute row can be in, for hiding rows.
func (r *Resource) States() []string { return []string{"routing", "empty"} }

// Start detects the CRD on a new cluster, then watches the namespace.
func (r *Resource) Start(h resources.Host) tea.Cmd {
	r.Stop()
	r.host = h
	r.gen++
	gen := r.gen
	r.rows, r.order, r.synced, r.err = map[string]*rowState{}, nil, false, nil
	if h.Client() == r.client && r.detected {
		return r.watch()
	}
	r.client, r.detected, r.served = h.Client(), false, false
	client := r.client
	return func() tea.Msg {
		gvr, ok := Detect(client)
		return detectedMsg{gen, gvr, ok}
	}
}

func (r *Resource) watch() tea.Cmd {
	if !r.served {
		return nil
	}
	gen, h := r.gen, r.host
	ctx, cancel := context.WithCancel(h.Context())
	r.cancel = cancel
	err := Watch(ctx, h.Client(), r.gvr, h.Namespace(), h.Selector(), Handlers{
		Row:    func(row Row, deleted, initial bool) { h.Send(objMsg{gen, row, deleted, initial}) },
		Synced: func() { h.Send(syncedMsg{gen}) },
		Error:  func(err error) { h.Send(errMsg{gen, err}) },
	})
	if err != nil {
		r.err, r.errAt = err, time.Now()
	}
	return tick(gen)
}

func (r *Resource) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
}

func tick(gen int) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{gen} })
}

func (r *Resource) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case detectedMsg:
		if msg.gen == r.gen {
			r.detected, r.gvr, r.served = true, msg.gvr, msg.served
			return r.watch()
		}
	case objMsg:
		if msg.gen == r.gen {
			r.apply(msg.row, msg.deleted, msg.initial)
		}
	case syncedMsg:
		if msg.gen == r.gen && !r.synced {
			r.synced = true
			r.logInitialProblems()
		}
	case errMsg:
		if msg.gen == r.gen {
			r.err, r.errAt = msg.err, time.Now()
		}
	case tickMsg:
		if msg.gen == r.gen {
			r.purgeDeleted()
			return tick(r.gen)
		}
	}
	return nil
}

func (r *Resource) Listed() []resources.Ref {
	var out []resources.Ref
	for _, k := range r.order {
		out = append(out, r.rows[k].row.ref())
	}
	return out
}

// Links: an IngressRoute leads to the services behind its routes.
func (r *Resource) Links() []resources.Link {
	var out []resources.Link
	for _, k := range r.order {
		ir := r.rows[k].row
		for _, s := range ir.Services {
			out = append(out, resources.Link{From: ir.ref(), To: s})
		}
	}
	return out
}

func (r *Resource) Empty() string {
	if !r.synced {
		return "waiting for ingressroute list…"
	}
	return "no ingressroutes"
}

// ---- state and change log ------------------------------------------------------

func (r *Resource) log(kind resources.ChangeKind, ir Row, detail string) {
	r.host.Log(resources.Change{At: time.Now(), Kind: kind, Object: ir.Name, Detail: detail,
		State: []resources.Cell{{Text: "●", Tone: ir.Light()}}})
}

func (r *Resource) apply(ir Row, deleted, initial bool) {
	now := time.Now()
	key := ir.Key()
	rs, exists := r.rows[key]
	switch {
	case deleted:
		if exists {
			rs.row, rs.deletedAt = ir, now
		}
		r.log(resources.Deleted, ir, "")
	case !exists || !rs.deletedAt.IsZero():
		rs = &rowState{row: ir, changed: map[string]time.Time{}}
		if !initial {
			rs.addedAt = now
			r.log(resources.Added, ir, describe(ir))
		}
		r.rows[key] = rs
	default:
		old := rs.row
		var details []string
		mark := func(col, what, from, to string) {
			if from != to {
				rs.changed[col] = now
				details = append(details, fmt.Sprintf("%s %s → %s", what, none(from), none(to)))
			}
		}
		mark("HOSTS", "hosts", old.hosts(), ir.hosts())
		mark("ENTRYPOINTS", "entrypoints", strings.Join(old.EntryPoints, ","), strings.Join(ir.EntryPoints, ","))
		mark("ROUTES", "routes", fmt.Sprint(old.Routes), fmt.Sprint(ir.Routes))
		if !slices.Equal(old.Services, ir.Services) {
			details = append(details, "services "+names(old.Services)+" → "+names(ir.Services))
		}
		rs.row = ir
		if len(details) > 0 {
			r.log(resources.Modified, ir, strings.Join(details, ", "))
		}
	}
	r.resort()
}

func describe(ir Row) string {
	return fmt.Sprintf("%s · %d routes → %s", none(ir.hosts()), ir.Routes, names(ir.Services))
}

func names(refs []resources.Ref) string {
	var out []string
	for _, s := range refs {
		out = append(out, s.Name)
	}
	return none(strings.Join(out, ","))
}

func none(s string) string {
	if s == "" {
		return "<none>"
	}
	return s
}

// logInitialProblems names every IngressRoute without a route at start.
func (r *Resource) logInitialProblems() {
	for _, k := range r.order {
		if ir := r.rows[k].row; ir.Light() != resources.Good {
			r.log(resources.State, ir, "holds no route")
		}
	}
}

func (r *Resource) purgeDeleted() {
	removed := false
	for k, rs := range r.rows {
		if !rs.deletedAt.IsZero() && time.Since(rs.deletedAt) > keepDeleted {
			delete(r.rows, k)
			removed = true
		}
	}
	if removed {
		r.resort()
	}
}

func (r *Resource) resort() {
	r.order = r.order[:0]
	for k := range r.rows {
		r.order = append(r.order, k)
	}
	sort.Strings(r.order)
}
