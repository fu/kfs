// Package configmaps is the configmaps kind: name, number of entries, age.
// Like secrets, the values are never kept in the list; describe and yaml
// fetch them on request.
//
//	watch.go     the configmap watch (values dropped as they arrive)
//	resource.go  state, change log
//	row.go       what a row shows
//	status.go    what it can show in the status line
package configmaps

import (
	"context"
	"fmt"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/fu/k7s/internal/resources"
)

type Resource struct {
	host   resources.Host
	gen    int
	cancel context.CancelFunc

	rows   map[string]Row
	order  []string
	synced bool
	err    error
	errAt  time.Time
}

func New() *Resource { return &Resource{} }

type (
	rowMsg struct {
		gen     int
		row     Row
		deleted bool
	}
	syncedMsg struct{ gen int }
	errMsg    struct {
		gen int
		err error
	}
)

func (r *Resource) Name() string      { return "configmaps" }
func (r *Resource) Key() string       { return "o" } // c is contexts
func (r *Resource) Tag() string       { return "cm" }
func (r *Resource) Objects() []string { return []string{"ConfigMap"} }
func (r *Resource) GVR() schema.GroupVersionResource {
	return corev1.SchemeGroupVersion.WithResource("configmaps")
}
func (r *Resource) Available() bool { return true }
func (r *Resource) Ready() bool     { return r.synced }

func (r *Resource) Start(h resources.Host) tea.Cmd {
	r.Stop()
	r.host = h
	r.gen++
	gen := r.gen
	ctx, cancel := context.WithCancel(h.Context())
	r.cancel = cancel
	r.rows, r.order, r.synced, r.err = map[string]Row{}, nil, false, nil
	err := Watch(ctx, h.Client(), h.Namespace(), h.Selector(), Handlers{
		Row:    func(row Row, deleted bool) { h.Send(rowMsg{gen, row, deleted}) },
		Synced: func() { h.Send(syncedMsg{gen}) },
		Error:  func(err error) { h.Send(errMsg{gen, err}) },
	})
	if err != nil {
		r.err, r.errAt = err, time.Now()
	}
	return nil
}

func (r *Resource) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
}

func (r *Resource) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case rowMsg:
		if msg.gen != r.gen {
			return nil
		}
		key := msg.row.Key()
		old, existed := r.rows[key]
		if msg.deleted {
			delete(r.rows, key)
		} else {
			r.rows[key] = msg.row
		}
		r.resort()
		if r.synced {
			r.logChange(old, existed, msg.row, msg.deleted)
		}
	case syncedMsg:
		if msg.gen == r.gen {
			r.synced = true
		}
	case errMsg:
		if msg.gen == r.gen {
			r.err, r.errAt = msg.err, time.Now()
		}
	}
	return nil
}

// logChange records added, deleted and updated configmaps (an update is a
// new resourceVersion; the values are not looked at).
func (r *Resource) logChange(old Row, existed bool, row Row, deleted bool) {
	c := resources.Change{At: time.Now(), Object: row.Name}
	switch {
	case deleted:
		c.Kind = resources.Deleted
	case !existed:
		c.Kind, c.Detail = resources.Added, fmt.Sprintf("%d entries", row.Entries)
	case old.Version != row.Version:
		c.Kind, c.Detail = resources.Modified, "data updated"
		if old.Entries != row.Entries {
			c.Detail = fmt.Sprintf("entries %d → %d", old.Entries, row.Entries)
		}
	default:
		return
	}
	r.host.Log(c)
}

func (r *Resource) resort() {
	r.order = r.order[:0]
	for k := range r.rows {
		r.order = append(r.order, k)
	}
	sort.Strings(r.order)
}

func (r *Resource) Listed() []resources.Ref {
	var out []resources.Ref
	for _, k := range r.order {
		row := r.rows[k]
		out = append(out, resources.Ref{Kind: "ConfigMap", Namespace: row.Namespace, Name: row.Name})
	}
	return out
}

func (r *Resource) Empty() string {
	if !r.synced {
		return "waiting for configmap list…"
	}
	return "no configmaps"
}
