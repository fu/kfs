package commands

import (
	"github.com/fu/kfs/internal/resources"
	"github.com/fu/kfs/internal/resources/pods"
)

// pickContainer resolves the container for a command. If t.Sub is set (a
// sub-row), it uses that container directly. If the pod has one suitable
// container, it uses that. Otherwise it shows a notice (the user should
// select a container sub-row).
func pickContainer(t resources.Target, longRunning bool, open func(pods.Row, pods.Container) resources.Step) resources.Step {
	_, p, ok := pod(t)
	if !ok {
		return nil
	}
	// Sub-row: the container is already chosen.
	if t.Sub != "" {
		for _, c := range p.Containers {
			if c.Name == t.Sub {
				return open(p, c)
			}
		}
		t.Host.Notice("container " + t.Sub + " not found")
		return nil
	}
	// Filter by longRunning (shell skips init containers).
	var list []pods.Container
	for _, c := range p.Containers {
		if !longRunning || c.Kind != pods.KindInit {
			list = append(list, c)
		}
	}
	switch len(list) {
	case 0:
		t.Host.Notice("no containers to choose from")
		return nil
	case 1:
		return open(p, list[0])
	default:
		t.Host.Notice("this pod has multiple containers — select one below")
		return nil
	}
}

// multiContainer reports whether the target is a pod row (not a container
// sub-row) with more than one suitable container. Used for Enabled.
func multiContainer(t resources.Target) bool {
	_, p, ok := pod(t)
	if !ok {
		return false
	}
	return len(p.Containers) > 1
}
