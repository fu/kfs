// Package services is the services kind: type, addresses, ports, and the
// pods behind each service (from its EndpointSlices).
//
//	service.go   a service as a row needs it; its light
//	resource.go  the watches, state, change log, links
//	row.go       what a row shows
//	status.go    what it can show in the status line
package services

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	"github.com/fu/kfs/internal/kube"
	"github.com/fu/kfs/internal/resources"
)

// Resource is the services kind.
type Resource struct {
	host   resources.Host
	gen    int
	cancel context.CancelFunc

	rows   map[string]*rowState
	order  []string
	slices map[string]endpoints // EndpointSlice name → its service and pods
	synced bool
	err    error
	errAt  time.Time
}

func New() *Resource { return &Resource{} }

const keepDeleted = 8 * time.Second

type (
	objMsg struct {
		gen              int
		row              Row
		deleted, initial bool
	}
	sliceMsg struct {
		gen     int
		name    string
		ep      endpoints
		deleted bool
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

func (r *Resource) Name() string      { return "services" }
func (r *Resource) Key() string       { return "i" } // s is secrets
func (r *Resource) Tag() string       { return "svc" }
func (r *Resource) Objects() []string { return []string{"Service"} }
func (r *Resource) GVR() schema.GroupVersionResource {
	return corev1.SchemeGroupVersion.WithResource("services")
}
func (r *Resource) Available() bool { return true }
func (r *Resource) Ready() bool     { return r.synced }

// States a service row can be in, for hiding rows.
func (r *Resource) States() []string { return []string{"ready", "pending", "broken"} }

func (r *Resource) Start(h resources.Host) tea.Cmd {
	r.Stop()
	r.host = h
	r.gen++
	gen := r.gen
	ctx, cancel := context.WithCancel(h.Context())
	r.cancel = cancel
	r.rows, r.order, r.slices, r.synced, r.err = map[string]*rowState{}, nil, map[string]endpoints{}, false, nil
	err := kube.Watch(ctx, h.Client(), h.Namespace(), h.Selector(),
		func(f informers.SharedInformerFactory) cache.SharedIndexInformer {
			return f.Core().V1().Services().Informer()
		},
		kube.Handlers[*corev1.Service]{
			Object: func(o *corev1.Service, deleted, initial bool) {
				h.Send(objMsg{gen, fromService(o), deleted, initial})
			},
			Synced: func() { h.Send(syncedMsg{gen}) },
			Error:  func(err error) { h.Send(errMsg{gen, err}) },
		})
	if err == nil {
		// The slices carry their service's name as a label, not its labels:
		// watch all of them in the namespace, whatever the filter.
		err = kube.Watch(ctx, h.Client(), h.Namespace(), "",
			func(f informers.SharedInformerFactory) cache.SharedIndexInformer {
				return f.Discovery().V1().EndpointSlices().Informer()
			},
			kube.Handlers[*discoveryv1.EndpointSlice]{
				Object: func(o *discoveryv1.EndpointSlice, deleted, _ bool) {
					h.Send(sliceMsg{gen, o.Name, fromSlice(o), deleted})
				},
			})
	}
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
	case objMsg:
		if msg.gen == r.gen {
			r.apply(msg.row, msg.deleted, msg.initial)
		}
	case sliceMsg:
		if msg.gen == r.gen {
			if msg.deleted {
				delete(r.slices, msg.name)
			} else {
				r.slices[msg.name] = msg.ep
			}
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

// Links: a service leads to the pods its selector selects (as its
// EndpointSlices list them).
func (r *Resource) Links() []resources.Link {
	if r.host == nil {
		return nil
	}
	ns := r.host.Namespace()
	var out []resources.Link
	for _, ep := range r.slices {
		if ep.service == "" {
			continue
		}
		svc := resources.Ref{Kind: "Service", Namespace: ns, Name: ep.service}
		for _, p := range ep.pods {
			out = append(out, resources.Link{From: svc, To: resources.Ref{Kind: "Pod", Namespace: ns, Name: p}})
		}
	}
	return out
}

func (r *Resource) Empty() string {
	if !r.synced {
		return "waiting for service list…"
	}
	return "no services"
}

// ---- state and change log ------------------------------------------------------

func (r *Resource) log(kind resources.ChangeKind, s Row, detail string) {
	r.host.Log(resources.Change{At: time.Now(), Kind: kind, Object: s.Name, Detail: detail,
		State: []resources.Cell{{Text: "●", Tone: s.Light()}}})
}

func (r *Resource) apply(s Row, deleted, initial bool) {
	now := time.Now()
	key := s.Key()
	rs, exists := r.rows[key]
	switch {
	case deleted:
		if exists {
			rs.row, rs.deletedAt = s, now
		}
		r.log(resources.Deleted, s, "")
	case !exists || !rs.deletedAt.IsZero():
		rs = &rowState{row: s, changed: map[string]time.Time{}}
		if !initial {
			rs.addedAt = now
			r.log(resources.Added, s, strings.ToLower(s.Type)+" "+s.Ports)
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
		mark("TYPE", "type", old.Type, s.Type)
		mark("CLUSTER-IP", "cluster ip", old.ClusterIP, s.ClusterIP)
		mark("EXTERNAL-IP", "external", old.External, s.External)
		mark("PORTS", "ports", old.Ports, s.Ports)
		rs.row = s
		if len(details) > 0 {
			r.log(resources.Modified, s, strings.Join(details, ", "))
		}
	}
	r.resort()
}

func none(s string) string {
	if s == "" {
		return "<none>"
	}
	return s
}

// logInitialProblems explains every service not fine at start.
func (r *Resource) logInitialProblems() {
	for _, k := range r.order {
		s := r.rows[k].row
		switch s.Light() {
		case resources.Busy:
			r.log(resources.State, s, "load balancer has no address yet")
		case resources.Alarm:
			r.log(resources.State, s, "ExternalName without a target")
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
