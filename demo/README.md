# kfs demo

A namespace to record kfs in: `shop`, with a bit of everything kfs shows.

| kind | what | shows in kfs |
|---|---|---|
| Deployment `web` | 1 replica, two containers: `app` (nginx) and `sidecar` (polls the app, logs every 3 s) | deployments, its ReplicaSet, a pod with a container sub-row, logs of either container |
| StatefulSet `db` | 1 replica, redis with a 1 Gi volume claim | the second pod, claims and volumes |
| Services `web`, `db` | ClusterIP and headless | services, linked to their pods (and on to the Deployment) |
| IngressRoute `web` | Traefik, `shop.localhost` | ingressroutes, linked to the web service |
| ConfigMaps `web-html`, `web-env`, `db-config` | mounted and read as env | configmaps, linked to the pods using them |
| Secrets `db-auth`, `web-api` | env and envFrom | secrets (masked), linked to the pods using them |

All names are placeholders until there is a theme.

## A toy cluster

[k3d](https://k3d.io) is the easiest: it brings Traefik (for the IngressRoute),
a storage provisioner (for the claim) and metrics-server (for CPU/MEM).

```bash
k3d cluster create kfs-demo -p "8080:80@loadbalancer"
kubectl apply -k demo
kfs -n shop
```

The page is then at http://shop.localhost:8080. With kind or minikube, drop
`web-ingressroute.yaml` from `kustomization.yaml` unless Traefik is
installed, and install metrics-server for CPU/MEM.

Flux: point a Kustomization at this folder (`path: ./demo`), and the gitops
section shows it as the app deploying `shop`.

## Scenes

`demo/scenes.sh` makes the interesting things happen, one at a time
(it refuses to run against anything but a local cluster):

| scene | what kfs shows |
|---|---|
| `rollout`, `back` | a new image: new ReplicaSet, the old one scaled down, "rollout nginx:1.27-alpine → nginx:1.26-alpine" |
| `crash` | the sidecar exits: CrashLoopBackOff, restarts, red on the rail, the exit explained |
| `oom` | the sidecar runs out of memory: "OOMKilled — out of memory, hit its memory limit" |
| `stuck` | a pod that cannot be scheduled: Pending, FailedScheduling, the rollout turns red after a minute |
| `probe` | readiness fails: the pod is not ready, Unhealthy warnings |
| `scale` | 3 pods for 20 s, then 1 |
| `config`, `rotate` | a configmap / a secret changes: "data updated" |
| `kill` | the pod is deleted and replaced |
| `up` | back to the manifests after any scene |
| `down` | delete the namespace |

A recording could go: start kfs, select the Deployment (everything it
touches lights up), `rollout`, `config`, `crash` (watch the rail turn red,
read the change log), `up`, open the sidecar's logs, port-forward the app.
