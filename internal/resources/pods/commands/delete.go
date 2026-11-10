package commands

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/fu/kfs/internal/resources"
)

// Delete removes the pod; k8s will restart it if a workload owns it.
var Delete = resources.Command{
	Name: "delete",
	Run: func(t resources.Target) resources.Step {
		_, p, ok := pod(t)
		if !ok {
			return nil
		}
		err := t.Host.Client().Kube.CoreV1().Pods(p.Namespace).Delete(
			context.Background(), p.Name, metav1.DeleteOptions{})
		if err != nil {
			t.Host.Notice("delete: " + err.Error())
		} else {
			t.Host.Notice(fmt.Sprintf("deleted pod %s/%s", p.Namespace, p.Name))
		}
		return nil
	},
}
