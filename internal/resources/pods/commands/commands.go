// Package commands are what a pod row can do: logs, previous-logs, shell,
// forward. Which of them the page offers, and on which keys, is up to the
// config.
//
//	logs.go        logs, previous-logs
//	shell.go       shell
//	forward.go     forward
//
// Multi-container pods show their containers as sub-rows; logs and shell
// work on those (via Target.Sub) and are hidden from the parent row.
package commands

import (
	"github.com/fu/kfs/internal/resources"
	"github.com/fu/kfs/internal/resources/pods"
)

// All are the pods' commands, in hint order.
func All() []resources.Command {
	return []resources.Command{Logs, PreviousLogs, Shell, Forward}
}

// pod is the target's pod (live), if it still exists. For sub-rows (key
// "ns/pod/container"), the pod is looked up by the pod part of the key.
func pod(t resources.Target) (*pods.Resource, pods.Row, bool) {
	r, ok := t.Kind.(*pods.Resource)
	if !ok {
		return nil, pods.Row{}, false
	}
	key := t.Key
	if t.Sub != "" {
		key = t.Key[:len(t.Key)-len(t.Sub)-1] // strip "/container"
	}
	p, ok := r.Pod(key)
	return r, p, ok
}
