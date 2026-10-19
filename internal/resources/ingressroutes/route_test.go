package ingressroutes

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/fu/k7s/internal/resources"
)

func TestFromUnstructured(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "web", "namespace": "shop"},
		"spec": map[string]any{
			"entryPoints": []any{"websecure"},
			"routes": []any{
				map[string]any{"match": "Host(`a.example.org`, `b.example.org`) && PathPrefix(`/api`)",
					"services": []any{map[string]any{"name": "api", "port": int64(80)}}},
				map[string]any{"match": "Host(`a.example.org`)",
					"services": []any{
						map[string]any{"name": "front", "namespace": "edge"},
						map[string]any{"name": "weighted", "kind": "TraefikService"},
					}},
			},
		},
	}}
	r := fromUnstructured(u)
	if r.hosts() != "a.example.org,b.example.org" || r.Routes != 2 || r.EntryPoints[0] != "websecure" {
		t.Errorf("unexpected row: %+v", r)
	}
	want := []resources.Ref{
		{Kind: "Service", Namespace: "shop", Name: "api"},
		{Kind: "Service", Namespace: "edge", Name: "front"},
	}
	if len(r.Services) != 2 || r.Services[0] != want[0] || r.Services[1] != want[1] {
		t.Errorf("services: %+v", r.Services)
	}
	if r.Light() != resources.Good {
		t.Errorf("a route with routes is green")
	}
}
