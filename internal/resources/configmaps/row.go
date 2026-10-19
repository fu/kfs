package configmaps

import (
	"fmt"

	"github.com/fu/k7s/internal/kube"
	"github.com/fu/k7s/internal/resources"
)

// What a configmap row shows (never its values).
var columns = []resources.Column{
	{Title: "NAME", Middle: true, Shrink: true},
	{Title: "DATA", Right: true},
	{Title: "AGE"},
}

func (r *Resource) Columns() []resources.Column { return columns }

func (r *Resource) Rows() []resources.Row {
	out := make([]resources.Row, len(r.order))
	for i, k := range r.order {
		cm := r.rows[k]
		out[i] = resources.Row{Key: k, Ref: resources.Ref{Kind: "ConfigMap", Namespace: cm.Namespace, Name: cm.Name}, Cells: []resources.Cell{
			{Text: cm.Name},
			{Text: fmt.Sprint(cm.Entries)},
			{Text: kube.Age(cm.Created), Tone: resources.Dim},
		}}
	}
	return out
}
