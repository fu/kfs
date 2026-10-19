package ingressroutes

import (
	"fmt"
	"strings"
	"time"

	"github.com/fu/kfs/internal/kube"
	"github.com/fu/kfs/internal/resources"
)

// What an IngressRoute row shows.
var columns = []resources.Column{
	{Title: "NAME", Middle: true, Shrink: true},
	{Title: "HOSTS", Shrink: true},
	{Title: "ENTRYPOINTS", Drop: 1},
	{Title: "ROUTES", Right: true},
	{Title: "STATUS"},
	{Title: "AGE"},
}

func (r *Resource) Columns() []resources.Column { return columns }

func (r *Resource) Rows() []resources.Row {
	now := time.Now()
	out := make([]resources.Row, len(r.order))
	for i, k := range r.order {
		rs := r.rows[k]
		ir := rs.row
		hosts := resources.Cell{Text: ir.hosts()}
		if hosts.Text == "" {
			hosts = resources.Cell{Text: "*", Tone: resources.Dim}
		}
		cells := map[string]resources.Cell{
			"NAME":        {Text: ir.Name},
			"HOSTS":       hosts,
			"ENTRYPOINTS": {Text: strings.Join(ir.EntryPoints, ","), Tone: resources.Dim},
			"ROUTES":      {Text: fmt.Sprint(ir.Routes)},
			"STATUS":      {Text: "●", Tone: ir.Light()},
			"AGE":         {Text: kube.Age(ir.Created), Tone: resources.Dim},
		}
		if !rs.addedAt.IsZero() && now.Sub(rs.addedAt) < resources.FlashFor {
			cells["NAME"] = resources.Cell{Text: ir.Name, Tone: resources.Fresh}
		}
		for col, at := range rs.changed {
			if now.Sub(at) < resources.FlashFor {
				c := cells[col]
				c.Tone = resources.Flash
				cells[col] = c
			}
		}
		row := resources.Row{Key: k, Ref: ir.ref(), State: ir.state(), Struck: !rs.deletedAt.IsZero()}
		for _, c := range columns {
			row.Cells = append(row.Cells, cells[c.Title])
		}
		out[i] = row
	}
	return out
}
