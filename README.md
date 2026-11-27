# kfs

One namespace, everything in it, live — like `kubectl get -w`, all linked up.

kfs shows a namespace on one page that updates in place: the GitOps apps that
deploy it, its deployments, replicasets, pods (with CPU/MEM), services,
ingress routes, configmaps, volumes and secrets, with a change log of
everything that happens there, including what happened before you started.

Built on kubectl's libraries (`client-go`, `cli-runtime`), so `-n`,
`--context`, `--kubeconfig`, `--as` and exec auth plugins behave as in kubectl.

## Install

```bash
curl -fsSL https://getkfs.org/install.sh | sh    # macOS / Linux
brew install fu/tap/kfs                          # Homebrew
go install github.com/fu/kfs@latest              # Go
```

Or download a binary from the [releases](https://github.com/fu/kfs/releases).
The install script takes `KFS_VERSION=v0.5.2` and `KFS_INSTALL_DIR=…`.

## Usage

```bash
kfs                         # the current namespace
kfs -n shop                 # namespace shop
kfs -n shop -l app=web      # only one app in it
kfs -n shop --once          # print the resources once and exit
kfs --refresh 2s            # metrics poll interval (default 5s)
```

## The page

| resource | shows |
|---|---|
| gitops | Argo CD / Flux apps deploying what is listed: sync, health, parent app |
| deployments | ready, up-to-date, available, status light, images |
| replicasets | one per revision; old ones scaled to zero are dimmed |
| pods | ready, status, restarts, CPU/MEM and % of request/limit, port-forwards |
| services | type, addresses, ports |
| ingressroutes | Traefik IngressRoutes (if the CRDs exist): hosts, entrypoints, routes |
| configmaps | entries, age |
| claims / volumes | PVCs and their PVs: status, capacity, access modes, class |
| secrets | type, keys; `x` shows the decoded values |

Every table starts NAME, READY (where there is one), STATUS: green (fine),
amber (on its way), red (failing). Only AGE is grey. Configmaps and secrets
have no state, so no STATUS. For gitops, CHAIN is the worst light of the
apps above it (PARENT).

- **Links:** selecting a row highlights everything linked to it, across
  sections: app → deployment → replicaset → pods → secrets, configmaps,
  claims → volumes, and service / ingress route → pods.
- **Change log:** rollouts, scaling, status and restart changes (with exit
  reasons such as OOMKilled), why an app is not green, and all Kubernetes
  events of the namespace. Each line is tagged with its kind (`po`, `rs`,
  `svc`, …). At start it shows the events Kubernetes still keeps.
- **Header:** a frame with the context, namespace and filter in its top
  border, the resources by their F-keys inside (dim while hidden), and the
  scopes (`1` contexts, `2` namespaces, `3` filter) in its bottom border.
- **Frames:** each resource sits in a frame in its own color (btop style):
  its name and column headers in the top border, its status (lights,
  counts, hidden rows) at the bottom right. The arrow on the left border marks
  the selected row, the bulbs the rows linked to it. The selected row's
  resource lists what the row can do at the bottom left, with each key lit in
  its word (`logs`, `shell`, `copy`).
- **Rail:** the change log's frame is a time line. Now is right after its
  title, and changes travel clockwise round it to the top-left corner,
  `rail.minutes` ago. Red, amber and cyan mark failures, warnings and changes.

## Keys

| key | |
|---|---|
| `1` / `2` / `3` | pick context / namespace / filter (label selector and hidden row states) |
| `F1` … `F10` | show or hide a resource, in display order (on macOS hold `fn` unless F-keys are standard) |
| `` ` `` | sort the selected row's resource, column by column |
| `↑↓` `j k` | move |
| `tab` / `shift+tab` | next / previous resource |
| `q` / `a` | scroll the change log |
| `+` / `-` | resize the change log |
| `Q`, `esc` | quit (`esc` in a screen: back) |

Commands on the selected row (defaults; rebind them in the config):

| key | resources | command |
|---|---|---|
| `enter` | all | describe (`kubectl describe`) |
| `y` | all | copy the name to the clipboard (OSC 52 over SSH) |
| — | all | yaml (`kubectl get -o yaml`, secrets masked) |
| `l` | pods | logs (`-f --tail=500`) |
| — | pods | previous-logs (`-p`) |
| `s` | pods | shell (`bash`, else `sh`) |
| `f` | pods | port-forward (again: stop) |
| `ctrl+d` | pods | delete |
| `x` | secrets | extract: the decoded values |

Commands every row has (copy, sort) are lit in `key-global`, the resource's
own (logs, shell, …) in `key-command`.

kfs does not capture the mouse, so you can select and copy text with your terminal as usual.

## Config

`~/.config/kfs/config.yaml` (or `$XDG_CONFIG_HOME/kfs/`, or `--config`) is
written on first start from
[`default.yaml`](internal/config/default.yaml), which documents every
option: `resource_order`, `commands` and their keys, `status` items, `hide`
(row states), `rail`. kfs saves hidden resources, row states, the divider and
the last namespace per context back to it, so comments are lost.

```yaml
commands:
  "*":
    yaml: Y
  pods:
    logs: l
    previous-logs: p
```

The colors are in `theme.yaml` next to it, written once from
[`theme.yaml`](internal/config/theme.yaml) and never touched again. It
covers everything kfs draws:

| group | keys |
|---|---|
| header | `badge`, `key-global`, `key-command`, `menu` |
| text | `text-heading`, `text-dim`, `text-on-color`, `text-menu`, `info` |
| rows | `index`, `row-highlighted`, `row-referenced`, `flash` |
| frames | `frame-color-kfs` (the header), `frame-color-<resource>` (e.g. `frame-color-pods`), `frame-color` (any other) |
| status | `status-ok`, `status-busy`, `status-warn`, `status-failed` |
| rail | `rail-ok`, `rail-warn`, `rail-failed`, `rail-idle`, `rail-cursor`, `pulse` |

Colors from an older `config.yaml` move there.

Page keys win over command keys. Older config files are migrated; a file from
a newer kfs is read but never overwritten.

## Development

```bash
go build -o bin/kfs .
go test ./...
```

- `internal/resources/<kind>/`: one folder per kind (`resource.go` watch and
  change log, `row.go` columns, `status.go`, `commands/`)
- `internal/tui/`: rendering, the same for every kind. Register a new kind
  in `registry()` in `app.go`.
- `internal/config/default.yaml`, `theme.yaml`: the defaults. Change them here, not in Go.

Behind a TLS-inspecting proxy on macOS, Go uses only `SSL_CERT_FILE` if it
is set. Make sure that bundle includes the corporate root CA.

**Release:** push a `v*` tag. GoReleaser builds the binaries and publishes the
release and the Homebrew cask (needs `HOMEBREW_TAP_GITHUB_TOKEN`).

## License

[Apache License 2.0](LICENSE)
