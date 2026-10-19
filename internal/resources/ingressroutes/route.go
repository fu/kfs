package ingressroutes

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"

	"github.com/fu/k7s/internal/kube"
	"github.com/fu/k7s/internal/resources"
)

// Row is one IngressRoute, as its row and the change log need it.
type Row struct {
	Namespace, Name string
	Hosts           []string
	EntryPoints     []string
	Routes          int
	Services        []resources.Ref // the services behind its routes
	Created         time.Time
}

func (r Row) Key() string { return r.Namespace + "/" + r.Name }

func (r Row) ref() resources.Ref {
	return resources.Ref{Kind: "IngressRoute", Namespace: r.Namespace, Name: r.Name}
}

// Light: amber while it holds no route, else green.
func (r Row) Light() resources.Tone {
	if r.Routes == 0 {
		return resources.Busy
	}
	return resources.Good
}

func (r Row) state() string {
	if r.Routes == 0 {
		return "empty"
	}
	return "routing"
}

// Traefik serves IngressRoutes under its new group, older ones under the
// containo.us group.
var groups = []schema.GroupVersion{
	{Group: "traefik.io", Version: "v1alpha1"},
	{Group: "traefik.containo.us", Version: "v1alpha1"},
}

// Detect returns the IngressRoute resource the cluster serves, if any.
func Detect(c *kube.Client) (schema.GroupVersionResource, bool) {
	for _, gv := range groups {
		list, err := c.Kube.Discovery().ServerResourcesForGroupVersion(gv.String())
		if err != nil {
			continue
		}
		for _, r := range list.APIResources {
			if r.Name == "ingressroutes" {
				return gv.WithResource("ingressroutes"), true
			}
		}
	}
	return schema.GroupVersionResource{}, false
}

// hostRule finds Host(`a`, `b`) in a route's match rule.
var (
	hostRule = regexp.MustCompile("Host\\(([^)]*)\\)")
	quoted   = regexp.MustCompile("`([^`]*)`")
)

func fromUnstructured(u *unstructured.Unstructured) Row {
	r := Row{Namespace: u.GetNamespace(), Name: u.GetName(), Created: u.GetCreationTimestamp().Time}
	r.EntryPoints, _, _ = unstructured.NestedStringSlice(u.Object, "spec", "entryPoints")
	routes, _, _ := unstructured.NestedSlice(u.Object, "spec", "routes")
	r.Routes = len(routes)
	for _, rt := range routes {
		route, ok := rt.(map[string]any)
		if !ok {
			continue
		}
		match, _ := route["match"].(string)
		for _, h := range hostRule.FindAllStringSubmatch(match, -1) {
			for _, q := range quoted.FindAllStringSubmatch(h[1], -1) {
				if !slices.Contains(r.Hosts, q[1]) {
					r.Hosts = append(r.Hosts, q[1])
				}
			}
		}
		svcs, _ := route["services"].([]any)
		for _, s := range svcs {
			svc, ok := s.(map[string]any)
			if !ok {
				continue
			}
			kind, _ := svc["kind"].(string)
			name, _ := svc["name"].(string)
			if name == "" || (kind != "" && kind != "Service") {
				continue // a TraefikService is not a Service
			}
			ns, _ := svc["namespace"].(string)
			if ns == "" {
				ns = r.Namespace
			}
			ref := resources.Ref{Kind: "Service", Namespace: ns, Name: name}
			if !slices.Contains(r.Services, ref) {
				r.Services = append(r.Services, ref)
			}
		}
	}
	return r
}

func (r Row) hosts() string { return strings.Join(r.Hosts, ",") }

// Handlers receive updates; they are called from background goroutines.
type Handlers struct {
	Row    func(r Row, deleted, initial bool)
	Synced func()
	Error  func(error)
}

// Watch watches the IngressRoutes (gvr) of ns matching selector. It returns
// immediately and stops when ctx is done.
func Watch(ctx context.Context, c *kube.Client, gvr schema.GroupVersionResource, ns, selector string, h Handlers) error {
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(c.Dynamic, 0, ns,
		func(o *metav1.ListOptions) { o.LabelSelector = selector })
	inf := factory.ForResource(gvr).Informer()
	if err := inf.SetWatchErrorHandler(func(_ *cache.Reflector, err error) { h.Error(err) }); err != nil {
		return err
	}
	emit := func(obj any, deleted, initial bool) {
		if t, ok := obj.(cache.DeletedFinalStateUnknown); ok {
			obj = t.Obj
		}
		if u, ok := obj.(*unstructured.Unstructured); ok {
			h.Row(fromUnstructured(u), deleted, initial)
		}
	}
	_, err := inf.AddEventHandler(cache.ResourceEventHandlerDetailedFuncs{
		AddFunc:    func(obj any, initial bool) { emit(obj, false, initial) },
		UpdateFunc: func(_, obj any) { emit(obj, false, false) },
		DeleteFunc: func(obj any) { emit(obj, true, false) },
	})
	if err != nil {
		return err
	}
	factory.Start(ctx.Done())
	go func() {
		if cache.WaitForCacheSync(ctx.Done(), inf.HasSynced) {
			h.Synced()
		}
	}()
	return nil
}
