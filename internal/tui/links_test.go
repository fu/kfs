package tui

import (
	"testing"

	"github.com/fu/kfs/internal/resources"
)

func TestLinkedFollowsBothWaysButNotSideways(t *testing.T) {
	ref := func(kind, name string) resources.Ref { return resources.Ref{Kind: kind, Namespace: "shop", Name: name} }
	app, dep := ref("Application", "web"), ref("Deployment", "web")
	pod1, pod2 := ref("Pod", "web-1"), ref("Pod", "web-2")
	tls, db := ref("Secret", "tls"), ref("Secret", "db")
	links := []resources.Link{
		{From: app, To: dep},
		{From: dep, To: pod1}, {From: dep, To: pod2},
		{From: pod1, To: tls}, {From: pod2, To: db},
	}
	for _, tc := range []struct {
		sel  resources.Ref
		want []resources.Ref
	}{
		{app, []resources.Ref{dep, pod1, pod2, tls, db}}, // the app: its pods and their secrets
		{pod1, []resources.Ref{dep, app, tls}},           // a pod: its app and its secrets, not web-2
		{db, []resources.Ref{pod2, dep, app}},            // a secret: who uses it, up to the app
		{ref("Secret", "unused"), nil},                   // nothing linked
	} {
		got := linked(tc.sel, links)
		if len(got) != len(tc.want) {
			t.Errorf("%s: got %v, want %v", tc.sel.Name, got, tc.want)
			continue
		}
		for _, w := range tc.want {
			if !got[w] {
				t.Errorf("%s: %s not linked", tc.sel.Name, w.Name)
			}
		}
	}
}

// An IngressRoute → Service → pods chain reaches the pods' Deployment and
// what it uses; a Deployment reaches the Service and IngressRoute in front.
func TestServicesReachTheirWorkloads(t *testing.T) {
	ref := func(kind, name string) resources.Ref { return resources.Ref{Kind: kind, Namespace: "shop", Name: name} }
	ir, svc := ref("IngressRoute", "web"), ref("Service", "web")
	dep, rs, pod := ref("Deployment", "web"), ref("ReplicaSet", "web-7d4"), ref("Pod", "web-7d4-x")
	cm, other := ref("ConfigMap", "web-config"), ref("Pod", "db-0")
	links := serveWorkloads([]resources.Link{
		{From: ir, To: svc},
		{From: svc, To: pod},
		{From: dep, To: rs}, {From: rs, To: pod}, {From: dep, To: pod},
		{From: pod, To: cm},
		{From: ref("StatefulSet", "db"), To: other},
	})
	for _, tc := range []struct {
		sel  resources.Ref
		want []resources.Ref
	}{
		{ir, []resources.Ref{svc, dep, rs, pod, cm}},
		{svc, []resources.Ref{ir, dep, rs, pod, cm}},
		{dep, []resources.Ref{svc, ir, rs, pod, cm}},
		{cm, []resources.Ref{pod, rs, dep, svc, ir}},
	} {
		got := linked(tc.sel, links)
		for _, w := range tc.want {
			if !got[w] {
				t.Errorf("%s %s: %s %s not linked (got %v)", tc.sel.Kind, tc.sel.Name, w.Kind, w.Name, got)
			}
		}
		if got[other] {
			t.Errorf("%s %s: reached an unrelated pod", tc.sel.Kind, tc.sel.Name)
		}
	}
}
