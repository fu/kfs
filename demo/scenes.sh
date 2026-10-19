#!/bin/sh
# Things to make happen in the shop namespace while kfs records, one scene at
# a time:
#
#   ./demo/scenes.sh <scene>
#
#   up        apply everything (also: reset after any scene)
#   rollout   new image for web: a new ReplicaSet, the change log says
#             "rollout nginx:1.27-alpine -> nginx:1.26-alpine"
#   back      roll web back to the image in web.yaml
#   crash     web's sidecar exits: CrashLoopBackOff, red on the rail,
#             "exit Error(1)" explained
#   oom       web's sidecar eats memory: OOMKilled, "out of memory, hit its
#             memory limit"
#   stuck     web asks for more CPU than any node has: the new pod stays
#             Pending, the rollout turns red after progressDeadlineSeconds
#   scale     web to 3 pods, then back to 1 after 20s
#   config    change a value in web-env: configmaps "data updated"
#   rotate    new db password: secrets "data updated"
#   kill      delete web's pod: DELETED, a new one ADDED
#   probe     break web's readiness: pod not ready (amber), Unhealthy events
#   down      delete the namespace
#
# It only runs against a local cluster (k3d, kind, minikube, Docker Desktop,
# Rancher Desktop, OrbStack); KFS_DEMO_CONTEXT=<context> allows another one.

set -eu

ns=shop
here="$(cd "$(dirname "$0")" && pwd)"

ctx="$(kubectl config current-context)"
case "$ctx" in
  k3d-* | kind-* | minikube | docker-desktop | rancher-desktop | orbstack | "${KFS_DEMO_CONTEXT:-}") ;;
  *)
    echo "scenes.sh: the current context is '$ctx', not a local toy cluster." >&2
    echo "Switch with kubectl config use-context, or set KFS_DEMO_CONTEXT=$ctx to allow it." >&2
    exit 1
    ;;
esac

k() { kubectl -n "$ns" "$@"; }

# sidecar replaces the command of web's sidecar (container 1).
sidecar() {
  k patch deployment web --type=json \
    -p "[{\"op\":\"replace\",\"path\":\"/spec/template/spec/containers/1/command\",\"value\":[\"sh\",\"-c\",\"$1\"]}]"
}

case "${1:-}" in
  up) kubectl apply -k "$here" ;;
  rollout) k set image deployment/web app=nginx:1.26-alpine ;;
  back) k set image deployment/web app=nginx:1.27-alpine ;;
  crash) sidecar 'echo starting; sleep 5; echo boom >&2; exit 1' ;;
  oom) sidecar 'echo filling memory; head -c 200m /dev/zero | tail' ;;
  stuck) k patch deployment web --type=json \
    -p '[{"op":"replace","path":"/spec/template/spec/containers/0/resources/requests/cpu","value":"64"}]' ;;
  scale) k scale deployment/web --replicas=3 && sleep 20 && k scale deployment/web --replicas=1 ;;
  config) k patch configmap web-env --type=merge -p "{\"data\":{\"SHOP_GREETING\":\"hello at $(date +%H:%M:%S)\"}}" ;;
  rotate) k patch secret db-auth --type=merge -p "{\"stringData\":{\"password\":\"demo-$(date +%s)\"}}" ;;
  kill) k delete pod -l app.kubernetes.io/name=web --wait=false ;;
  probe) k patch deployment web --type=json \
    -p '[{"op":"replace","path":"/spec/template/spec/containers/0/readinessProbe/httpGet/path","value":"/missing"}]' ;;
  down) kubectl delete namespace "$ns" ;;
  *)
    sed -n '2,25p' "$0" | sed 's/^# \{0,1\}//'
    exit 2
    ;;
esac
