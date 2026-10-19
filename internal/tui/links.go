package tui

import "github.com/fu/kfs/internal/resources"

// The link view: everything linked to the selected row is highlighted,
// across sections. Links are followed both ways — down to what it leads to
// (app → Deployment → pods → secrets) and up to what leads to it (secret ←
// pods ← Deployment ← app) — but not sideways: selecting a pod does not
// light up its sibling pods.

// linked returns the objects linked to sel (sel itself not included).
func linked(sel resources.Ref, links []resources.Link) map[resources.Ref]bool {
	out := map[resources.Ref]bool{}
	if sel == (resources.Ref{}) {
		return out
	}
	down, up := map[resources.Ref][]resources.Ref{}, map[resources.Ref][]resources.Ref{}
	for _, l := range links {
		down[l.From] = append(down[l.From], l.To)
		up[l.To] = append(up[l.To], l.From)
	}
	for _, next := range []map[resources.Ref][]resources.Ref{down, up} {
		seen := map[resources.Ref]bool{sel: true}
		queue := []resources.Ref{sel}
		for len(queue) > 0 {
			r := queue[0]
			queue = queue[1:]
			for _, n := range next[r] {
				if !seen[n] {
					seen[n], out[n] = true, true
					queue = append(queue, n)
				}
			}
		}
	}
	return out
}

// pageLinks collects the links of every started kind.
func (m *model) pageLinks() []resources.Link {
	var out []resources.Link
	for _, k := range m.kinds {
		if !m.started[k.Name()] {
			continue
		}
		if l, ok := k.(resources.Linker); ok {
			out = append(out, l.Links()...)
		}
	}
	return serveWorkloads(out)
}

// serveWorkloads adds a link from a Service to whatever runs the pods it
// selects (their ReplicaSet, Deployment, StatefulSet …). Following links
// never turns round, so without these a service would reach its pods but not
// their Deployment, and a Deployment (or its app) never its Service and the
// IngressRoute in front of it.
func serveWorkloads(links []resources.Link) []resources.Link {
	runs := map[resources.Ref][]resources.Ref{} // pod → what runs it
	for _, l := range links {
		if l.To.Kind == "Pod" && l.From.Kind != "Service" {
			runs[l.To] = append(runs[l.To], l.From)
		}
	}
	seen := map[resources.Link]bool{}
	out := links
	for _, l := range links {
		if l.From.Kind != "Service" || l.To.Kind != "Pod" {
			continue
		}
		for _, w := range runs[l.To] {
			if s := (resources.Link{From: l.From, To: w}); !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}
