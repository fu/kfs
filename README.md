# kfs

One namespace, everything in it, live: like `kubectl get -w` on steroids - all linked up.

kfs shows a namespace on one page that updates in place: the GitOps apps that
deploy it, its deployments, replicasets, pods (with CPU/MEM) and secrets — and
below them a change log
of everything that happens there, including what happened before you started.

Built on the same libraries as kubectl (`client-go`, `cli-runtime`, `cobra`),
so `-n`, `--context`, `--kubeconfig`, `--as`, exec auth plugins (kubelogin) …
behave exactly like in kubectl. TUI: Bubble Tea + Lip Gloss.

## Install

```bash
curl -fsSL https://getkfs.org/install.sh | sh    # macOS / Linux
brew install fu/tap/kfs                          # or with Homebrew
go install github.com/fu/kfs@latest              # or with Go
```

The install script ([`website/install.sh`](website/install.sh)) picks the
build for your OS and CPU, checks it against the release's `checksums.txt`
and puts `kfs` into `/usr/local/bin` if that is writable, else
`~/.local/bin`. `KFS_VERSION=v0.5.2` installs another release,
`KFS_INSTALL_DIR=…` another place.

Or download a binary from the [releases](https://github.com/fu/kfs/releases).

kfs was called k7s up to v0.4.3. Coming from there: `brew uninstall k7s`,
then install kfs as above. The config now lives in `~/.config/kfs/` and
starts again at format version 1; kfs does not read `~/.config/k7s/`, so copy
over what you changed there by hand.

## Usage

```bash
kfs                         # the current namespace
kfs -n shop                 # namespace shop
kfs -n shop -l app=web      # only one app in it
kfs -n shop --once          # print the sections once and exit (also when piped)
kfs --refresh 2s            # metrics poll interval (default 5s)
```

There are no sub commands: you pick a namespace (and optionally a filter),
and everything in it is shown.

## The page

```
 kfs │ ctx dev │ ns shop │ filter app=web │ gitops ● 1 apps │ deployments 1 ●1 ●0 ●0 │ pods 3 ●3 ●0 ●0 │ secrets 2 │ ● watching

┆gitops
│#  SYNC  HEALTH  NAME  DEST  PARENT      SOURCE              REV      SYNCED
▶1  ●     ●       web   shop  ● main-app  deploy/apps/web     f91dfdc  2h
⋮
│deployments
┆#  NAME  READY  UP-TO-DATE  AVAILABLE  STATUS  IMAGES    AGE
●1  web   3/3             3          3  ●       web:1.4.2  40d
.
 replicasets
┆#  NAME             REV  DESIRED  CURRENT  READY  STATUS  AGE
●1  web-7d4b9c6f5      4        3        3      3  ●       3h
│2  web-5f6c8d9b7      3        0        0      0  ●       2d
⋮
┆pods
│#  NAME                   READY  STATUS  RESTARTS  CPU  %CPU/R  %CPU/L  MEM  %MEM/R  %MEM/L  AGE
●1  web-7d4b9c6f5-x2k8q    1/1    ●       0         3m  1%      -       248Mi 48%    8%      3h
┆…
⋮
│services
┆#  NAME  TYPE        CLUSTER-IP   EXTERNAL-IP  PORTS    STATUS  AGE
│1  web   LoadBalancer 10.96.3.11  1.2.3.4      80/TCP   ●       40d
.
┆ingressroutes
⋮#  NAME  HOSTS            ENTRYPOINTS  ROUTES  STATUS  AGE
┆1  web   app.example.com  websecure         2  ●       40d
│
 configmaps
│#  NAME     DATA  AGE
┆1  settings    4  40d
⋮
│secrets
┆#  NAME     TYPE    KEYS  AGE
⋮…

╰ changes (42) ━━━    ┄┄   ·     ━━   ┈┈    ───    ┄    ···    ━━
13:02:10 MODIFIED deployment/web  2/3  rollout web:1.4.1 → web:1.4.2, up-to-date 3 → 0
13:02:11 EVENT    deployment/web  ScalingReplicaSet: Scaled up replica set web-7d4b9c6f5 to 3
13:02:15 MODIFIED web-7d4b9c6f5-x2k8q  Running 1/1  status ContainerCreating → Running
 contexts  namespaces  filter │ gitops  deployments  replicasets  pods  services  ingressroutes  configmaps  secrets   logs  shell  forward  [q/a] changes  [esc] quit
```

**Sections** (in this order; each shows the same rows wherever it appears):

1. **gitops** — only the Argo CD / Flux apps responsible for what is listed:
   those deploying a listed secret or the workload (Deployment,
   StatefulSet, …) of a listed pod, read from Argo CD's managed resources /
   Flux's inventory; without a filter, every app deploying into the
   namespace. SYNC and HEALTH lights; PARENT is the app that deploys this
   one (app of apps), colored by the worst light up that chain.
2. **deployments** — READY, UP-TO-DATE, AVAILABLE, a status light (green
   all ready and up to date, amber rolling out or scaling, red stuck:
   progress deadline exceeded or replica failure) and the images.
3. **replicasets** — one per rollout revision (REV) with desired, current
   and ready replicas; old revisions scaled to zero are dimmed.
4. **pods** — `#`, READY, a status light (green running, amber starting,
   red failing/terminating), RESTARTS ("x ago"), CPU, MEM and their % of the
   pod's request (`/R`) and limit (`/L`, `-` = none). CPU/MEM show ↑/↓ versus
   the previous poll, changed cells flash, new pods are highlighted, deleted
   ones stay struck through for 8s, port-forwarded pods show `⇄:<port>`.
5. **services** — type, cluster and external address, the port list and a
   status light (amber while a LoadBalancer is still getting its address, red
   for an ExternalName without a target). It links to the pods its selector
   selects. Toggle with `i` (`s` is secrets).
6. **ingressroutes** — Traefik IngressRoutes (only shown on clusters that have
   the CRDs): the hosts each routes match, its entrypoints, how many routes it
   holds and a light (amber while it holds no route). It links
   to the services its routes send traffic to. Toggle with `u`.
7. **configmaps** — name, number of entries, age. Like secrets, the values are
   never kept in the list; describe and yaml fetch them on request. Toggle
   with `o` (`c` is contexts).
8. **claims** — the PersistentVolumeClaims: a status light (green bound,
   amber pending or resizing, red lost), the volume, capacity, access modes
   (RWO, RWX, …) and storage class. Toggle with `m` (`c` is contexts).
9. **volumes** — the PersistentVolumes bound to claims of the namespace
   (with a filter: to the listed claims): status (yellow released: the
   claim is gone, the data may remain), claim, capacity, reclaim policy.
10. **secrets** — name, type, number of keys, age. Enter opens one: its keys
    with the values masked; `r` reveals them. Values are only fetched for the
    opened secret, never kept in the list.

**Links**: the selected row lights up (dark teal) everything linked to it,
across sections, following app → Deployment → ReplicaSet → pods → secrets,
configmaps and claims → volumes. A service reaches the pods its selector
picks out and whatever runs them; an IngressRoute reaches the services behind
its routes — so a service lights up its pods and their Deployment, an ingress
route its services and everything behind them, and a Deployment (or a
configmap it uses) the services and ingress routes in front of it.
Select an app to see its deployments, their replicasets, pods and the
secrets they use. Select a pod to see its replicaset, deployment, app and
secrets. Select a secret to see who uses it, up to the app. Secrets are
used through volumes, `env`/`envFrom` and image pull secrets; jobs belong
to their CronJob / KEDA ScaledJob by name.

The **rail** is a time line of the change log. It starts under the kfs
badge, runs down the left edge of the sections (which sit one column to its
right), turns at the divider (`╰`) and runs along it to the right edge:
top-left is now, the far right end is `rail.minutes` ago (config, default
10). Every frame the rail is cut into as many slots as the window has room
for, each slot covering an equal share of that time — so what happened walks
one slot further every minutes/slots, and a bigger window shows the same time
finer: an alert that fills one slot on a small screen may fill two on a large
one. A slot where something changed is red when something failed (a state
in red: a crashing pod, a stuck rollout), amber for a warning (a Warning
event, something pending or progressing), and cyan when all was fine; with
several changes in one slot the worst wins. It fades as it travels: solid
near now, then thinner, then dotted (`┃ │ : .` down, `━ ─ ┄ ·` along); a busy
slot stays a step brighter. At start the rail is filled from the events
Kubernetes still keeps (about the last hour), squeezed into its window so
the oldest sit at the far end; they walk off as time goes on. Where
nothing happened the rail is grey packets that swell and fade with gaps
between them (`. : │ │ : .   . : .`), flowing on a steady beat of their own:
a new piece appears right under the badge every 300 ms and the rest move on. It counts what the change log
shows, so the filter applies to it too. The rail also follows the selection:
an arrow (`▶`) where it passes the selected row, and a bulb (`●`) breathing
with the "watching" dot by every row linked to it.

The status line has one **gitops light**: the worst of the responsible apps
and their parents, so green pods under a broken deployment chain do not
look fine. The cyan "watching" dot breathes once per metrics refresh.

**Change log** (`q`/`a` scroll it, line by line) — everything about what is
shown, in time order. Each line starts with the time and a tag (up to three
letters) for the kind it is about: kubectl's short name where it has a short
one (`po` pods, `rs` replicasets, `pvc` claims, `pv` volumes, `svc` services,
`cm` configmaps, `ks`/`hr`
Flux), else kfs' own (`dp` deployments, `se` secrets, `ir` ingress routes, `ap` Argo CD apps). Kubernetes events get the
tag of the object they are about; events about other kinds (Jobs, …) are
`ev`.

```
16:59:03 po  MODIFIED web-7d4-x  Running 2/2  status PodInitializing → Running
16:59:04 rs  ADDED    web-7d4  revision 4, 3 replicas
16:59:05 pvc MODIFIED data-db-0  Bound  resized 10Gi → 20Gi
16:59:06 ev  EVENT    job/agent-x  Completed: Job completed
```

It records:

- deployment rollouts (image changes), scaling, readiness and why a rollout
  is stuck; new replicaset revisions and their scaling
- pod adds/deletes and status, ready and restart changes; container exits are
  explained (`exit OOMKilled(137) — out of memory, hit its memory limit`)
- secrets and configmaps added, deleted and updated
- services added and deleted, type, address and port changes; IngressRoutes
  added and deleted, host, entry point, route and service changes
- sync / health / revision changes of the responsible apps, and why an
  app is not green, one line each: the resources Argo CD / Flux list, plus
  what kfs finds itself in what the app manages (Argo CD 3 no longer keeps
  resource health in the Application). kfs looks for child apps that are
  not green, Deployments, StatefulSets, DaemonSets and Jobs that are not
  available, with their failing pods and why (CrashLoopBackOff, a missing
  secret, …), and claims that are not bound.
- all Kubernetes events of the namespace — warnings (failed probes,
  scheduling, mounts…) and normal ones (scaling, rollouts, pulls, kills)
- at start: the events Kubernetes still keeps (about the last hour), so you
  see what happened before — including pods that are already gone — and a
  `STATE` line for everything already unhealthy

With a filter, only events about listed objects (and their Deployments /
ReplicaSets) are shown.

## Keys

The navigation bar shows the menus with their key letter in cyan:

| key | |
|-----|---|
| `c` / `n` / `f` | pick the **c**ontext / **n**amespace / **f**ilter; enter switches and returns |
| `g` `e` `r` `p` `i` `u` `o` `m` `v` `s` | show or hide the **g**itops / d**e**ployments / **r**eplicasets / **p**ods / serv**i**ces / ingressro**u**tes / c**o**nfigmaps / clai**m**s / **v**olumes / **s**ecrets section |
| `Shift` + section key | sort that section by its columns: a small arrow marks the column in order (it starts at NAME ↑); each press steps that column ↓, then the next column ↑, then ↓, cycling back to NAME ↑ |
| `↑↓`, `0-9` | move; jump to row `#` of the current section |
| `tab` / `shift+tab` | jump to the first row of the next / previous section, round and round |
| mouse wheel / trackpad | over the sections: move the selection; over the change log: scroll it; on a screen (logs, describe, pickers): like `↑↓`. kfs takes the mouse for this, so select text with `Shift` (`Option` in macOS Terminal and iTerm2) |
| `d` | on any row: **d**escribe it, the same text as `kubectl describe` (from kubectl's own describe code) |
| `l` / `h` / `w` | on a pod: **l**ogs, s**h**ell, for**w**ard a port (again: stop) |
| `enter` | on a pod: logs; on a secret: open it |

The keys on the selected row are **commands**. The config decides which
commands each section offers and on which keys (see [Config](#config)). The
ones above are the defaults. More exist than are bound by default:

| command | sections | |
|---|---|---|
| `describe` | every | `kubectl describe` |
| `yaml` | every | `kubectl get -o yaml`, without managedFields; secret values masked |
| `logs` | pods | `kubectl logs -f --tail=500` of a container |
| `previous-logs` | pods | `kubectl logs -p`: the run before the last restart |
| `shell` | pods | `kubectl exec -it … sh` |
| `forward` | pods | `kubectl port-forward` (again: stop) |
| `open` | secrets | the keys, values masked until `r` |
| `q` / `a` | scroll the change log up / down |
| `Q` / `A` | move the divider up / down: a taller / shorter change log |
| `esc` | back; on the main page: quit |

The **filter** pane suggests selectors from the app labels in the namespace
(`app.kubernetes.io/instance`, `app.kubernetes.io/name`, `app`, `k8s-app`,
`release` …) with how many deployments, replicasets, pods and secrets each
matches, plus "everything" (`enter` applies). Below them are the row
**states** of each kind (pods: running, pending, failing, completed;
deployments: ready, progressing, stuck, scaled-down; replicasets: active,
old). `enter` or `space` shows ● or hides ○ a state. That's saved in the
config and applies to every namespace. By default completed pods and old
replicasets are hidden. The status line counts what is hidden
(`pods 8 … (+5 hidden)`). Hidden rows stay in the change log, and links
still pass through them. To exclude by label, use the selector syntax,
e.g. `-l '!job-name'` (no pods created by Jobs).
The **namespace** pane shows pods running/pending/failing and CPU/MEM per
namespace.

Logs follow the last 500 lines like `kubectl logs -f --tail=500`; the shell
uses client-go's exec (like `kubectl exec -it`, `bash` else `sh`), type
`exit` to come back; port-forwarding works like `kubectl port-forward` (same
local port when free). Pods with several containers or ports get a picker.

## Config

`~/.config/kfs/config.yaml` (or `$XDG_CONFIG_HOME/kfs/config.yaml`, or
`--config <file>`) keeps the colors, the order of the sections
(`resource_order`, top to bottom, e.g. `[pods, gitops]` for pods on top;
sections not listed follow in the default order), the commands and their keys,
and the state kfs comes back to: hidden sections, hidden row states, the last
namespace per context (used without `-n`).

The defaults are [`internal/config/default.yaml`](internal/config/default.yaml),
compiled into kfs. Change a default there, not in Go. On the first start kfs
writes that file, comments and all, as your config. After that kfs rewrites
the file whenever you toggle a section or state or switch namespace, so
comments do not survive.

The `commands` part binds commands to keys, per section; `"*"` is for every
section, and several keys go in one string:

```yaml
commands:
  "*":
    describe: d
    yaml: y               # not bound by default
  pods:
    logs: l enter
    previous-logs: o      # not bound by default
    shell: h
    forward: w
  secrets:
    open: enter
```

The `status` part picks what each section shows in the status line, after
its name. Every kind declares its items in its `status.go`. A section
toggled off shows nothing there.

```yaml
status:
  "*": [errors, hidden]           # every section: watch errors, rows hidden by state
  gitops: [light]                 # the worst light of the apps and their parents
  deployments: [lights]           # "2 ●2 ●0 ●0": how many, green, amber, red
  pods: [lights, forwards, metrics]
  claims: [lights]
  secrets: [count]
```

`rail.minutes` is how much time the rail spans, from now
(top-left) to its far right end:

```yaml
rail:
  minutes: 10
```

A command a section does not list is not offered (`pods: {}` offers none).
A section missing from `commands` gets its defaults. Page keys (`c n f`, the
section keys, `q a Q A`, `j k G`, digits, `esc`) win over commands.

Keys missing from your file take their defaults. `version` is the file format:
older files are migrated, and a file from a newer kfs is used as far as
understood but never overwritten. The version is bumped whenever a key is
added, because an older kfs would otherwise drop the new key when saving.

## Code layout

```
cmd/                       CLI: kfs [-n] [-l] [--once] [--config]
internal/config/           the YAML config: section order, commands, colors, hidden
  default.yaml             sections and states, last namespaces; the defaults (compiled in)
internal/kube/             cluster access: clients, contexts, a generic watch,
                           exec, port-forward, events, value formatting
internal/resources/        the contract every kind implements
  resource.go              Resource (a section), Indicator, Keyed, Stateful, Linker/Link, Host, Change
  row.go                   Column, Row, Cell + Tone — what to show, not how
  command.go               Command (what a row can do) and its Target
  steps.go                 what a command leads to: Picker, Viewer, Exec
  commands/                commands for every kind: describe, yaml
  gitops/ deployments/ replicasets/ pods/ services/ ingressroutes/ configmaps/ pvcs/ pvs/ secrets/
                           one folder per kind, each with
                             resource.go  watch, state, change log, links
                             row.go       its columns and cells
                             status.go    what it can show in the status line
                             commands/    what its rows can do, one file each
                                          (pods: logs, shell, forward; secrets: open)
  restest/                 a recording Host for testing kinds
internal/tui/              everything drawn, the same way for every kind
  app.go                   the registry of kinds and their commands, screens, navigation bar
  page.go                  the page: sections, selection, # jump, commands bound by the config
  links.go                 the link view: what the selected row links to
  sort.go                  per-section column sorting (Shift+section key)
  table.go                 row layout: widths, shrinking, truncation
  picker.go  viewer.go     the generic screens of Picker / Viewer steps
  terminal.go              Exec steps: terminal hand-over (+ size queue)
  changes.go               the change log and Kubernetes events
  rail.go                  the rail: the change log as a time line along the page edge
  contexts.go namespaces.go filter.go   the scope menus (pickers)
  once.go                  --once: the same rows, printed
```

Adding a kind (deployments and replicasets were added this way) means a
folder under `internal/resources` with `resource.go` and `row.go` and one
line in the registry in `internal/tui/app.go`; its watch uses `kube.Watch`.
Adding a command means a file in the kind's `commands/` folder (listed in its
`All()`), and a key for it in `default.yaml` if it should be bound by
default. Its steps (Picker, Viewer, Exec) are drawn by the tui.

## Build

```bash
go build -o bin/kfs .
```

Behind a TLS-inspecting corporate proxy, Go must trust the proxy's root CA to
download modules. Note that on macOS Go uses *only* `SSL_CERT_FILE` when it is
set (instead of the Keychain), so a bundle without the corporate root fails
with `x509: certificate signed by unknown authority`. Either unset it, or
point it at a bundle that includes the corporate root:

```bash
cat /etc/ssl/cert.pem corporate-root.pem > ~/.certs/ca-bundle.pem
SSL_CERT_FILE=~/.certs/ca-bundle.pem go build -o bin/kfs .
```

## Release

Push a version tag; the `release` workflow tests, builds macOS/Linux/Windows
binaries with [GoReleaser](https://goreleaser.com), publishes a GitHub release
and updates the Homebrew cask in `fu/homebrew-tap`:

```bash
git tag v0.1.0 && git push origin v0.1.0
```

The Homebrew step needs the repository secret `HOMEBREW_TAP_GITHUB_TOKEN` (a
token allowed to push to `fu/homebrew-tap`); without it, it is skipped. Check
the config locally with
`go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean`.

## License

[Apache License 2.0](LICENSE), the same as kubectl and client-go.
