# kumo-cli

`kumo` is the command-line interface for the [Kumo](https://kumo.run) platform —
a thin wrapper over the official Go SDK
[`github.com/kumobase/kumo-go`](https://github.com/kumobase/kumo-go).

## What you can do

| Group | What it manages |
|---|---|
| `kumo auth` | Sign in with an API key, view active identity, sign out |
| `kumo apps` | Deploy and manage applications (create, update, start/stop, domains, operations history) |
| `kumo secret` | Registry credentials, env-var groups, and file-mount secrets |
| `kumo apikey` | Inspect personal and registry-scoped API keys (read-only) |
| `kumo registry` | Container registry: organizations, repositories, images, and `docker login` |
| `kumo volume` | Persistent volumes: create, attach/detach, online resize |
| `kumo vps` | Virtual servers: rent, list plans/regions, lifecycle (start/stop/reboot/reinstall), SSH shortcut |

Run `kumo <group> --help` for the full subcommand list and `kumo <group> <cmd> --help` for flags.

## Install

One-line install (Linux & macOS, x86_64 & arm64):

```bash
curl -sSfL https://raw.githubusercontent.com/kumobase/kumo-cli/main/install.sh | sh
```

Pin a specific version:

```bash
curl -sSfL https://raw.githubusercontent.com/kumobase/kumo-cli/main/install.sh | KUMO_VERSION=v1.2.3 sh
```

The installer downloads the matching archive from GitHub Releases, verifies
its SHA-256 checksum, and drops `kumo` into `~/.local/bin` (override with
`KUMO_INSTALL_DIR`). Windows users grab the `.zip` from the
[releases page](https://github.com/kumobase/kumo-cli/releases).

With Go:

```bash
go install github.com/kumobase/kumo-cli@latest
```

Or build from source:

```bash
make build      # produces ./kumo
```

## Authenticate

`kumo` authenticates with a Kumo API key (`kumo_sk_…`) created in the dashboard.

```bash
kumo auth login                       # prompts for the key (hidden input)
kumo auth login --api-key kumo_sk_…   # non-interactive
echo "$KUMO_KEY" | kumo auth login    # from stdin / pipe
kumo auth whoami                      # show the active identity
kumo auth logout                      # remove stored credentials
```

The key is validated against the API before being saved.

## Configuration

Settings live under `~/.kumo/` (override with `KUMO_HOME`), separating non-secret
config from secrets in the AWS/gcloud style:

| File | Mode | Contents |
|---|---|---|
| `~/.kumo/config.yaml` | `0644` | `base_url`, `output`, current profile |
| `~/.kumo/credentials.yaml` | `0600` | API keys, per profile |

Multiple **profiles** are supported via `--profile` / `KUMO_PROFILE` (default
`default`). Values resolve in this order (highest first):

1. command-line flag (`--base-url`, `--output`, `--profile`)
2. environment (`KUMO_API_KEY`, `KUMO_BASE_URL`, `KUMO_OUTPUT`, `KUMO_PROFILE`)
3. selected profile in the config files
4. built-in default (base URL `https://api.kumo.run`, output `table`)

Every command prints a human-readable table by default. Pass `-o json` /
`--output json` for machine-readable output.

## Quick tour

Resources are addressed by **name** (per-user unique). If a name matches more
than one resource the CLI reports `AMBIGUOUS_NAME` and asks you to rename one.

```bash
# Apps — declarative deploys from a manifest, or one-shot flags
kumo apps create -f app.yaml
kumo apps update web --replicas 3
kumo apps stop web && kumo apps start web

# Secrets — registry creds, env-var groups, file mounts
kumo secret create --name dockerhub --type registry \
  --registry-username alice --registry-password '***'
kumo secret list

# Container registry — push images via docker, browse via the CLI
kumo registry login                    # shells `docker login` using your stored key
kumo registry repo create myapp
docker push registry.kumo.run/<org>/myapp:1
kumo registry image list myapp

# Volumes — create, attach to an app, resize online
kumo volume create --name data --tier ssd --size 5 --app web --mount /data
kumo volume resize data --size 20      # waits until ready

# VPS — rent a server, manage its lifecycle, jump in over SSH
kumo vps regions
kumo vps plans --region sg-singapore
kumo vps rent --name box1 --provider zeabur --region sg-singapore --plan <id>
kumo vps password box1                 # reveal the initial password
kumo vps ssh box1                      # exec `ssh root@<ip> -p <port>`
kumo vps stop box1 && kumo vps start box1
```

## Manifest example

For full app specs (env vars, health checks, autoscaling), use a manifest with
`-f`. Flags override manifest values when both are given. A complete example
lives at [`testdata/app.yaml`](./testdata/app.yaml).

```yaml
name: my-demo-app
image: nginx:1.27
port: 80
isExposed: true
replicas: 2
pricingSlug: app-small
environmentVariables:
  - key: LOG_LEVEL
    value: info
healthCheck:
  type: http
  path: /healthz
  port: 80
autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 5
  cpuTargetPercentage: 70
```

## Async operations

Mutations on apps, volumes, and VPS instances are asynchronous. Commands wait
for completion by default; pass `--wait=false` to return as soon as the
operation is queued, and `--timeout` to bound the wait.

## Development

```bash
make build      # build ./kumo
make test       # go test -race ./...
make vet        # go vet ./...
make lint       # golangci-lint run
```

## License

Apache-2.0. See [LICENSE](./LICENSE).
