package commands

import (
	"context"
	"fmt"
	"io"

	"k8s.io/client-go/tools/remotecommand"

	"github.com/fu/kfs/internal/kube"
	"github.com/fu/kfs/internal/resources"
	"github.com/fu/kfs/internal/resources/pods"
)

// Shell runs an interactive shell (bash, else sh) in a running container,
// like "kubectl exec -it". Type exit to come back to kfs.
var Shell = resources.Command{
	Name:    "shell",
	Enabled: func(t resources.Target) bool { return t.Sub != "" || !multiContainer(t) },
	Run: func(t resources.Target) resources.Step {
		return pickContainer(t, true, func(pod pods.Row, c pods.Container) resources.Step {
			if c.State != "Running" {
				t.Host.Notice(fmt.Sprintf("no shell: container %s is %s", c.Name, c.State))
				return nil
			}
			client := t.Host.Client()
			return &resources.Exec{
				Banner: fmt.Sprintf("shell in %s/%s [%s] — type 'exit' to return to kfs", pod.Namespace, pod.Name, c.Name),
				Run: func(stdin io.Reader, stdout io.Writer, sizes remotecommand.TerminalSizeQueue) error {
					return client.Exec(context.Background(), pod.Namespace, pod.Name, c.Name, kube.ShellCommand, stdin, stdout, sizes)
				},
			}
		})
	},
}
