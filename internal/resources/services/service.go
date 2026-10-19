package services

import (
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"

	"github.com/fu/kfs/internal/resources"
)

// Row is one service, as its row and the change log need it.
type Row struct {
	Namespace, Name string
	Type            string // ClusterIP, NodePort, LoadBalancer, ExternalName
	ClusterIP       string
	External        string // load balancer addresses, external IPs or the ExternalName
	Ports           string // e.g. "80/TCP,443:30443/TCP"
	Created         time.Time
}

func (s Row) Key() string { return s.Namespace + "/" + s.Name }

func (s Row) ref() resources.Ref {
	return resources.Ref{Kind: "Service", Namespace: s.Namespace, Name: s.Name}
}

func fromService(o *corev1.Service) Row {
	s := Row{Namespace: o.Namespace, Name: o.Name, Type: string(o.Spec.Type), ClusterIP: o.Spec.ClusterIP,
		Created: o.CreationTimestamp.Time}
	if s.Type == "" {
		s.Type = string(corev1.ServiceTypeClusterIP)
	}
	var ext []string
	for _, in := range o.Status.LoadBalancer.Ingress {
		if in.IP != "" {
			ext = append(ext, in.IP)
		} else if in.Hostname != "" {
			ext = append(ext, in.Hostname)
		}
	}
	ext = append(ext, o.Spec.ExternalIPs...)
	if o.Spec.Type == corev1.ServiceTypeExternalName {
		ext = append(ext, o.Spec.ExternalName)
	}
	s.External = strings.Join(ext, ",")
	var ports []string
	for _, p := range o.Spec.Ports {
		port := fmt.Sprint(p.Port)
		if p.NodePort != 0 {
			port += fmt.Sprintf(":%d", p.NodePort)
		}
		ports = append(ports, port+"/"+string(p.Protocol))
	}
	s.Ports = strings.Join(ports, ",")
	return s
}

// Light: amber while a load balancer has no address yet, red for an
// ExternalName without a target, else green.
func (s Row) Light() resources.Tone {
	switch {
	case s.Type == string(corev1.ServiceTypeLoadBalancer) && s.External == "":
		return resources.Busy
	case s.Type == string(corev1.ServiceTypeExternalName) && s.External == "":
		return resources.Alarm
	}
	return resources.Good
}

// state names the light for hiding rows.
func (s Row) state() string {
	switch s.Light() {
	case resources.Busy:
		return "pending"
	case resources.Alarm:
		return "broken"
	}
	return "ready"
}

// endpoints is what one EndpointSlice says: which service, which pods.
type endpoints struct {
	service string
	pods    []string
}

func fromSlice(o *discoveryv1.EndpointSlice) endpoints {
	e := endpoints{service: o.Labels[discoveryv1.LabelServiceName]}
	for _, ep := range o.Endpoints {
		if ep.TargetRef != nil && ep.TargetRef.Kind == "Pod" {
			e.pods = append(e.pods, ep.TargetRef.Name)
		}
	}
	return e
}
