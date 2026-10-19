package services

import (
	"time"

	"github.com/fu/k7s/internal/kube"
	"github.com/fu/k7s/internal/resources"
)

// What a service row shows.
var columns = []resources.Column{
	{Title: "NAME", Middle: true, Shrink: true},
	{Title: "TYPE"},
	{Title: "CLUSTER-IP", Drop: 2},
	{Title: "EXTERNAL-IP", Shrink: true},
	{Title: "PORTS", Shrink: true},
	{Title: "STATUS"},
	{Title: "AGE"},
}

func (r *Resource) Columns() []resources.Column { return columns }

func (r *Resource) Rows() []resources.Row {
	now := time.Now()
	out := make([]resources.Row, len(r.order))
	for i, k := range r.order {
		rs := r.rows[k]
		s := rs.row
		ext := resources.Cell{Text: s.External}
		if ext.Text == "" {
			ext = resources.Cell{Text: "-", Tone: resources.Dim}
			if s.Light() == resources.Busy {
				ext = resources.Cell{Text: "<pending>", Tone: resources.Busy}
			}
		}
		cells := map[string]resources.Cell{
			"NAME":        {Text: s.Name},
			"TYPE":        {Text: s.Type},
			"CLUSTER-IP":  {Text: s.ClusterIP, Tone: resources.Dim},
			"EXTERNAL-IP": ext,
			"PORTS":       {Text: s.Ports},
			"STATUS":      {Text: "●", Tone: s.Light()},
			"AGE":         {Text: kube.Age(s.Created), Tone: resources.Dim},
		}
		if !rs.addedAt.IsZero() && now.Sub(rs.addedAt) < resources.FlashFor {
			cells["NAME"] = resources.Cell{Text: s.Name, Tone: resources.Fresh}
		}
		for col, at := range rs.changed {
			if now.Sub(at) < resources.FlashFor {
				c := cells[col]
				c.Tone = resources.Flash
				cells[col] = c
			}
		}
		row := resources.Row{Key: k, Ref: s.ref(), State: s.state(), Struck: !rs.deletedAt.IsZero()}
		for _, c := range columns {
			row.Cells = append(row.Cells, cells[c.Title])
		}
		out[i] = row
	}
	return out
}
