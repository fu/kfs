package gitops

import (
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Aux is one of the supporting Flux kinds beyond Kustomization and
// HelmRelease: a source (GitRepository, OCIRepository, HelmRepository,
// Bucket), an image automation object (ImageRepository, ImagePolicy,
// ImageUpdateAutomation), or a notification object (Alert, Provider,
// Receiver). They share the Ready condition; the helpers below add what each
// kind puts in their Source and Revision cells.

// auxCommon fills what every aux kind shares: the Ready condition light and
// suspension.
func auxCommon(u *unstructured.Unstructured, kind string) (App, map[string]condition) {
	o := u.Object
	a := App{Kind: kind, Namespace: u.GetNamespace(), Name: u.GetName()}
	a.DestNS = a.Namespace
	conds := conditions(o)
	ready, hasReady := conds["Ready"]
	a.SyncedAt = ready.at

	suspended, _, _ := unstructured.NestedBool(o, "spec", "suspend")
	switch {
	case suspended:
		a.Sync, a.SyncL = "Suspended", Amber
		a.why("suspended")
	case hasReady && ready.status == "True":
		a.Sync, a.SyncL = "Ready", Green
	case hasReady && ready.status == "False":
		a.Sync, a.SyncL = "Failed", Red
		a.why(reasonMessage(ready))
	default:
		a.Sync, a.SyncL = "Reconciling", Amber
		if ready.message != "" {
			a.why(ready.message)
		}
	}
	return a, conds
}

// artifact fills Revision and (when set) SyncedAt from status.artifact.
func artifact(a *App, o map[string]any) {
	art, ok, _ := unstructured.NestedMap(o, "status", "artifact")
	if !ok {
		return
	}
	if rev, _ := art["revision"].(string); rev != "" {
		a.Revision = shortRevision(rev)
	} else if dg, _ := art["digest"].(string); dg != "" {
		a.Revision = strings.TrimPrefix(dg, "sha256:")[:min(12, len(strings.TrimPrefix(dg, "sha256:")))]
	}
}

func fromGitRepository(u *unstructured.Unstructured) App {
	a, _ := auxCommon(u, "GitRepository")
	o := u.Object
	url, _, _ := unstructured.NestedString(o, "spec", "url")
	a.Source = describeSource(url, "", "")
	for _, f := range []string{"branch", "tag", "semver"} {
		if ref, _, _ := unstructured.NestedString(o, "spec", "reference", f); ref != "" {
			a.Source += "@" + ref
			break
		}
	}
	artifact(&a, o)
	return a
}

func fromOCIRepository(u *unstructured.Unstructured) App {
	a, _ := auxCommon(u, "OCIRepository")
	o := u.Object
	url, _, _ := unstructured.NestedString(o, "spec", "url")
	a.Source = strings.TrimPrefix(url, "oci://")
	for _, f := range []string{"tag", "semver", "digest"} {
		if ref, _, _ := unstructured.NestedString(o, "spec", "reference", f); ref != "" {
			a.Source += "@" + ref
			break
		}
	}
	artifact(&a, o)
	return a
}

func fromHelmRepository(u *unstructured.Unstructured) App {
	a, _ := auxCommon(u, "HelmRepository")
	o := u.Object
	url, _, _ := unstructured.NestedString(o, "spec", "url")
	a.Source = strings.TrimPrefix(url, "oci://") + " (helm)"
	artifact(&a, o)
	return a
}

func fromBucket(u *unstructured.Unstructured) App {
	a, _ := auxCommon(u, "Bucket")
	o := u.Object
	endpoint, _, _ := unstructured.NestedString(o, "spec", "endpoint")
	name, _, _ := unstructured.NestedString(o, "spec", "bucketName")
	a.Source = strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://") + "/" + name
	artifact(&a, o)
	return a
}

func fromImageRepository(u *unstructured.Unstructured) App {
	a, _ := auxCommon(u, "ImageRepository")
	o := u.Object
	img, _, _ := unstructured.NestedString(o, "spec", "image")
	a.Source = img
	if latest, _, _ := unstructured.NestedString(o, "status", "latestTag"); latest != "" {
		a.Revision = latest
	}
	return a
}

func fromImagePolicy(u *unstructured.Unstructured) App {
	a, _ := auxCommon(u, "ImagePolicy")
	o := u.Object
	ref, _, _ := unstructured.NestedString(o, "spec", "imageRef", "name")
	a.Source = ref
	if latest, _, _ := unstructured.NestedString(o, "status", "latestImage"); latest != "" {
		if i := strings.LastIndex(latest, ":"); i > 0 {
			a.Revision = latest[i+1:]
		}
	}
	return a
}

func fromImageUpdateAutomation(u *unstructured.Unstructured) App {
	a, _ := auxCommon(u, "ImageUpdateAutomation")
	o := u.Object
	src, _, _ := unstructured.NestedString(o, "spec", "sourceRef", "name")
	a.Source = src
	if commit, ok, _ := unstructured.NestedString(o, "status", "lastAutomation", "commit", "digest"); ok {
		a.Revision = shortRevision(commit)
	}
	return a
}

func fromAlert(u *unstructured.Unstructured) App {
	a, _ := auxCommon(u, "Alert")
	o := u.Object
	prov, _, _ := unstructured.NestedString(o, "spec", "providerRef", "name")
	a.Source = "→ " + prov
	return a
}

func fromProvider(u *unstructured.Unstructured) App {
	a, _ := auxCommon(u, "Provider")
	typ, _, _ := unstructured.NestedString(u.Object, "spec", "type")
	a.Source = typ
	return a
}

func fromReceiver(u *unstructured.Unstructured) App {
	a, _ := auxCommon(u, "Receiver")
	typ, _, _ := unstructured.NestedString(u.Object, "spec", "type")
	a.Source = typ
	return a
}

// condition is one entry of a Flux object's status.conditions.
type condition struct {
	status, reason, message string
	at                      time.Time
}

func conditions(o map[string]any) map[string]condition {
	out := map[string]condition{}
	list, _, _ := unstructured.NestedSlice(o, "status", "conditions")
	for _, c := range list {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		s := func(k string) string { v, _ := m[k].(string); return v }
		at, _ := time.Parse(time.RFC3339, s("lastTransitionTime"))
		out[s("type")] = condition{status: s("status"), reason: s("reason"), message: firstLine(s("message")), at: at}
	}
	return out
}

// fluxCommon fills what Kustomizations and HelmReleases share: the Ready
// condition (SYNC light) and suspension.
func fluxCommon(u *unstructured.Unstructured, kind string) (App, map[string]condition) {
	o := u.Object
	a := App{Kind: kind, Namespace: u.GetNamespace(), Name: u.GetName()}
	a.DestNS, _, _ = unstructured.NestedString(o, "spec", "targetNamespace")
	if a.DestNS == "" {
		a.DestNS = a.Namespace
	}
	conds := conditions(o)
	ready, hasReady := conds["Ready"]
	a.SyncedAt = ready.at

	suspended, _, _ := unstructured.NestedBool(o, "spec", "suspend")
	switch {
	case suspended:
		a.Sync, a.SyncL = "Suspended", Amber
		a.why("suspended")
	case hasReady && ready.status == "True":
		a.Sync, a.SyncL = "Ready", Green
	case hasReady && ready.status == "False":
		a.Sync, a.SyncL = "Failed", Red
		a.why(reasonMessage(ready))
	default:
		a.Sync, a.SyncL = "Reconciling", Amber
		if ready.message != "" {
			a.why(ready.message)
		}
	}
	return a, conds
}

func reasonMessage(c condition) string {
	switch {
	case c.reason != "" && c.message != "":
		return c.reason + ": " + c.message
	case c.message != "":
		return c.message
	}
	return c.reason
}

// fromKustomization maps a kustomize.toolkit.fluxcd.io Kustomization.
func fromKustomization(u *unstructured.Unstructured) App {
	a, conds := fluxCommon(u, "Kustomization")
	o := u.Object
	srcName, _, _ := unstructured.NestedString(o, "spec", "sourceRef", "name")
	dir, _, _ := unstructured.NestedString(o, "spec", "path")
	a.Source = describeSource(srcName, dir, "")
	rev, _, _ := unstructured.NestedString(o, "status", "lastAppliedRevision")
	a.Revision = shortRevision(rev)

	// What it applied: status.inventory ids are "<ns>_<name>_<group>_<kind>".
	entries, _, _ := unstructured.NestedSlice(o, "status", "inventory", "entries")
	for _, e := range entries {
		m, _ := e.(map[string]any)
		id, _ := m["id"].(string)
		if p := strings.Split(id, "_"); len(p) == 4 {
			a.Manages = append(a.Manages, Ref{Kind: p[3], Namespace: p[0], Name: p[1]})
		}
	}

	// HEALTH: the Healthy condition (set when health checks / wait are on),
	// otherwise "applied fine" counts as healthy.
	healthy, ok := conds["Healthy"]
	switch {
	case ok && healthy.status == "True":
		a.Health, a.HealthL = "Healthy", Green
	case ok && healthy.status == "False":
		a.Health, a.HealthL = "Unhealthy", Red
		a.why(reasonMessage(healthy))
	case ok:
		a.Health, a.HealthL = "Progressing", Amber
	case a.SyncL == Green:
		a.Health, a.HealthL = "Healthy", Green
	default:
		a.Health = "Unknown"
	}
	return a
}

// fromHelmRelease maps a helm.toolkit.fluxcd.io HelmRelease.
func fromHelmRelease(u *unstructured.Unstructured) App {
	a, conds := fluxCommon(u, "HelmRelease")
	o := u.Object
	chart, _, _ := unstructured.NestedString(o, "spec", "chart", "spec", "chart")
	if chart == "" { // v2 chartRef to an OCIRepository / HelmChart
		chart, _, _ = unstructured.NestedString(o, "spec", "chartRef", "name")
	}
	a.Source = describeSource("", "", chart)
	rev, _, _ := unstructured.NestedString(o, "status", "lastAttemptedRevision")
	if history, ok, _ := unstructured.NestedSlice(o, "status", "history"); ok && len(history) > 0 {
		if h, ok := history[0].(map[string]any); ok {
			if v, ok := h["chartVersion"].(string); ok && v != "" {
				rev = v
			}
		}
	}
	a.Revision = shortRevision(rev)

	// HEALTH: did the release (and its tests, if any) succeed?
	released, hasReleased := conds["Released"]
	tests, hasTests := conds["TestSuccess"]
	switch {
	case hasReleased && released.status == "False":
		a.Health, a.HealthL = "ReleaseFailed", Red
		a.why(reasonMessage(released))
	case hasTests && tests.status == "False":
		a.Health, a.HealthL = "TestFailed", Red
		a.why(reasonMessage(tests))
	case a.SyncL == Green:
		a.Health, a.HealthL = "Healthy", Green
	case a.SyncL == Red:
		a.Health, a.HealthL = "Unhealthy", Red
	default:
		a.Health, a.HealthL = "Progressing", Amber
	}
	return a
}
