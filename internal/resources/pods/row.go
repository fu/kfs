package pods

import (
	"fmt"
	"time"

	"github.com/fu/k7s/internal/kube"
	"github.com/fu/k7s/internal/resources"
)

// What a pod row shows. On narrow terminals pod names shrink first, then
// the utilisation percentages go.
var columns = []resources.Column{
	{Title: "NAME", Middle: true, Shrink: true},
	{Title: "READY"},
	{Title: "STATUS"},
	{Title: "RESTARTS"},
	{Title: "CPU", Right: true},
	{Title: "%CPU/R", Right: true, Drop: 1},
	{Title: "%CPU/L", Right: true, Drop: 3},
	{Title: "MEM", Right: true},
	{Title: "%MEM/R", Right: true, Drop: 2},
	{Title: "%MEM/L", Right: true, Drop: 4},
	{Title: "AGE"},
}

func (r *Resource) Columns() []resources.Column { return columns }

func (r *Resource) Rows() []resources.Row {
	now := time.Now()
	var out []resources.Row
	for _, k := range r.order {
		rs := r.rows[k]
		row := r.row(rs, now)
		out = append(out, row)
		// Multi-container pods: list containers as indented sub-rows.
		if len(rs.row.Containers) > 1 {
			for _, c := range rs.row.Containers {
				out = append(out, r.containerRow(rs, c, now))
			}
		}
	}
	return out
}

func (r *Resource) row(rs *rowState, now time.Time) resources.Row {
	p := rs.row
	name := p.Name
	for _, local := range r.forwardsOf(p.Key()) {
		name += fmt.Sprintf(" ⇄:%d", local)
	}
	cells := rowCells(name, p.ReadyString(), light(p), p.RestartsString(),
		rs.usage, rs.hasUsage, rs.cpuTrend, rs.memTrend,
		p.CPUReq, p.CPULim, p.MemReq, p.MemLim, kube.Age(p.Created))

	if !rs.addedAt.IsZero() && now.Sub(rs.addedAt) < resources.FlashFor {
		c := cells["NAME"]
		c.Tone = resources.Fresh
		cells["NAME"] = c
	}
	for col, at := range rs.changed {
		if now.Sub(at) < resources.FlashFor {
			c := cells[col]
			c.Tone = resources.Flash
			cells[col] = c
		}
	}

	row := resources.Row{Key: p.Key(), Ref: p.ref(), State: p.state(), Struck: !rs.deletedAt.IsZero()}
	for _, c := range columns {
		cell, ok := cells[c.Title]
		if !ok {
			cell = resources.Cell{Text: "-", Tone: resources.Dim}
		}
		row.Cells = append(row.Cells, cell)
	}
	return row
}

// rowCells builds the standard pod-table cells. Both pod rows and container
// sub-rows use this; the caller passes the same shape of data, only with
// different sources (pod-level aggregation vs. single container).
func rowCells(name, readyStr string, status resources.Tone, restartStr string,
	usage Usage, hasUsage bool, cpuTrend, memTrend int,
	cpuReq, cpuLim, memReq, memLim int64, age string,
) map[string]resources.Cell {
	restarts := resources.Cell{Text: restartStr}
	if restartStr != "0" {
		restarts.Tone = resources.Warn
	}
	cells := map[string]resources.Cell{
		"NAME":     {Text: name},
		"READY":    {Text: readyStr},
		"STATUS":   {Text: "●", Tone: status},
		"RESTARTS": restarts,
		"AGE":      {Text: age, Tone: resources.Dim},
	}
	if hasUsage {
		cells["CPU"] = resources.Cell{Text: kube.CPU(usage.CPU) + arrow(cpuTrend), Tone: trendTone(cpuTrend)}
		cells["MEM"] = resources.Cell{Text: kube.Mem(usage.Mem) + arrow(memTrend), Tone: trendTone(memTrend)}
		cells["%CPU/R"] = pct(usage.CPU, cpuReq, false)
		cells["%CPU/L"] = pct(usage.CPU, cpuLim, true)
		cells["%MEM/R"] = pct(usage.Mem, memReq, false)
		cells["%MEM/L"] = pct(usage.Mem, memLim, true)
	}
	return cells
}

// containerRow builds an indented sub-row for one container in a
// multi-container pod. It uses the same cell layout as a pod row; only the
// name is indented by two spaces.
func (r *Resource) containerRow(rs *rowState, c Container, now time.Time) resources.Row {
	p := rs.row
	name := "  " + c.Name
	if kind := string(c.Kind); kind != "" {
		name += " (" + kind + ")"
	}
	readyStr := "0/1"
	if c.Ready {
		readyStr = "1/1"
	}
	restartStr := fmt.Sprint(c.Restarts)

	var usage Usage
	var hasUsage bool
	if byPod, ok := r.containerUsage[p.Key()]; ok {
		if u, ok := byPod[c.Name]; ok {
			usage, hasUsage = u, true
		}
	}

	cells := rowCells(name, readyStr, containerLight(c), restartStr,
		usage, hasUsage, 0, 0,
		c.CPUReq, c.CPULim, c.MemReq, c.MemLim, kube.Age(p.Created))

	row := resources.Row{
		Key:    p.Key() + "/" + c.Name,
		Ref:    p.ref(),
		State:  p.state(),
		Sub:    c.Name,
		Struck: !rs.deletedAt.IsZero(),
	}
	for _, col := range columns {
		cell, ok := cells[col.Title]
		if !ok {
			cell = resources.Cell{Text: "-", Tone: resources.Dim}
		}
		row.Cells = append(row.Cells, cell)
	}
	return row
}

// containerLight maps a container state to the pod-table light tone.
func containerLight(c Container) resources.Tone {
	switch c.State {
	case "Running":
		return resources.Good
	case "Completed", "Succeeded":
		return resources.Dim
	case "Waiting", "ContainerCreating", "PodInitializing":
		return resources.Busy
	}
	return resources.Alarm
}

// States a pod row can be in, for hiding rows.
func (r *Resource) States() []string { return []string{"running", "pending", "failing", "completed"} }

func (p Row) state() string {
	switch p.Health() {
	case Healthy:
		return "running"
	case Progressing:
		return "pending"
	case Finished:
		return "completed"
	}
	return "failing"
}

// light is the traffic light for a pod: green running, amber on its way
// (pending, creating, initializing), red terminating or failing.
func light(p Row) resources.Tone {
	if p.Status == "Terminating" {
		return resources.Bad
	}
	switch p.Health() {
	case Healthy:
		return resources.Good
	case Progressing:
		return resources.Busy
	case Finished:
		return resources.Dim
	}
	return resources.Alarm
}

// pct is usage as a percentage of a request or limit, warning when high.
func pct(v, of int64, isLimit bool) resources.Cell {
	p := kube.PctValue(v, of)
	c := resources.Cell{Text: kube.Pct(v, of)}
	switch {
	case p < 0:
		c.Tone = resources.Dim
	case isLimit && p >= 90, !isLimit && p >= 150:
		c.Tone = resources.Alarm
	case isLimit && p >= 70, !isLimit && p >= 100:
		c.Tone = resources.Warn
	}
	return c
}

func arrow(t int) string {
	switch t {
	case 1:
		return "↑"
	case -1:
		return "↓"
	}
	return " "
}

func trendTone(t int) resources.Tone {
	switch t {
	case 1:
		return resources.Bad
	case -1:
		return resources.Good
	}
	return resources.Plain
}
