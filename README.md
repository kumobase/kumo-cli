# kumo-cli

`kumo` is the command-line interface for the [Kumo](https://kumo.run) platform —
a thin wrapper over the official Go SDK
[`github.com/kumobase/kumo-go`](https://github.com/kumobase/kumo-go).

## What you can do

| Group | What it manages |
|---|---|
| `kumo auth` | Sign in with an API key, view active identity, sign out |
| `kumo apps` | Deploy and manage applications (create, update, start/stop, domains, operations history) |
| `kumo jobs` | One-off and scheduled jobs, and their execution history |
| `kumo secret` | Registry credentials, env-var groups, and file-mount secrets |
| `kumo apikey` | Inspect personal and registry-scoped API keys (read-only) |
| `kumo registry` | Container registry: organizations, repositories, images, and `docker login` |
| `kumo volume` | Persistent volumes: create, attach/detach, online resize |
| `kumo vps` | Virtual servers: rent, list plans/regions, lifecycle (start/stop/reboot/reinstall), SSH shortcut |
| `kumo billing` | Account balance, usage charges, and cost breakdown |
| `kumo source` | Inspect git source connections (GitHub/GitLab) used for git-build apps |
| `kumo runners` | Inspect VM-backed CI runner jobs |

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

### Global flags

These apply to every command:

| Flag | Purpose |
|---|---|
| `-o, --output table\|json` | output format (default `table`) |
| `--profile`, `--base-url` | select profile / override the API base URL |
| `-y, --yes` | skip confirmation prompts on destructive commands |
| `-q, --quiet` | suppress progress/success chatter (stdout stays machine-clean) |
| `--idempotency-key <key>` | make a write safely retryable (see below) |

## Output & scripting

Every command prints a human-readable **table** by default. Pass `-o json` for
machine-readable output, designed to be driven by scripts and AI agents:

- **Success** prints the **bare** result on **stdout** — an object for
  `get`/detail and mutations, an array for `list` — following the
  `aws`/`gh`/`kubectl` convention:

  ```bash
  kumo apps get web -o json        # {"id":42,"name":"web","status":"running",…}
  kumo apps list -o json           # [ {…}, {…} ]
  kumo apps stop web -o json       # {"resource":"app","id":42,"action":"stop","status":"done"}
  ```

- **Errors** print a structured object on **stderr** (stdout stays empty), so a
  stable `code` can be branched on without parsing prose:

  ```json
  {"error":{"code":"APP_NOT_FOUND","message":"no app named \"web\"","http_status":404}}
  ```

- **Exit codes** classify the failure so callers can react without reading text:

  | Code | Meaning | | Code | Meaning |
  |---|---|---|---|---|
  | `0` | success | | `5` | conflict (incl. ambiguous name, in-use) |
  | `2` | usage / bad flags | | `6` | validation |
  | `3` | authentication | | `7` | etag mismatch (concurrent modification) |
  | `4` | not found | | `1` | other |

- **`kumo introspect`** emits the full command/flag tree as JSON, so an agent can
  discover the surface without scraping `--help`.

- **Idempotency:** a retried write can double-provision or double-bill. Pass a
  stable `--idempotency-key <key>` and the server collapses retries of the *same*
  request to a single effect.

## Quick tour

Resources are addressed by **name** (per-user unique). A bare **numeric id** is
treated as an id, which is the recovery when a name is ambiguous: if a name
matches more than one resource the CLI reports `AMBIGUOUS_NAME`, and you can pass
the id from `kumo <group> list` instead.

```bash
# Apps — declarative deploys from a manifest, or one-shot flags
kumo apps create -f app.yaml
kumo apps update web --replicas 3
kumo apps stop web && kumo apps start web

# Secrets — keep the password out of shell history via stdin
printf '%s' "$TOKEN" | kumo secret create --name dockerhub --type registry \
  --registry-username alice --registry-password-stdin
kumo secret list

# Jobs — one-off or scheduled, and follow a run to completion
kumo jobs create --name migrate --image myrepo/tools:v1 --command "./migrate"
kumo jobs run migrate --wait           # exits non-zero if the execution fails
kumo jobs list --kind scheduled

# Container registry — push images via docker, browse via the CLI
kumo registry login                    # shells `docker login` using your stored key
kumo registry repo create myapp
docker push registry.kumo.run/<org>/myapp:1
kumo registry image list myapp

# Volumes — create, attach to an app, resize online
kumo volume create --name data --tier ssd --size 5 --app web --mount /data
kumo volume resize data --size 20      # waits until ready

# VPS — rent a server (optionally wait until running), manage its lifecycle
kumo vps plans --region sg-singapore
kumo vps rent --name box1 --provider zeabur --region sg-singapore --plan <id> --wait
kumo vps password box1                 # reveal the initial password
kumo vps ssh box1                      # exec `ssh root@<ip> -p <port>`
kumo vps stop box1 && kumo vps start box1

# Billing — check your prepaid balance and usage
kumo billing balance
kumo billing summary
```

## Manifest example

For full app specs (env vars, health checks, autoscaling), use a manifest with
`-f`. Flags override manifest values when both are given. A complete example
lives at [`testdata/app.yaml`](./testdata/app.yaml).

The same fields are also available as `create`/`update` flags —
`--autoscale`/`--min-replicas`/`--max-replicas`/`--cpu-target`/`--mem-target` and
`--health-check-type`/`--health-check-path`/`--health-check-port` — so you can
enable them without a manifest. When updating with a partial manifest, only the
fields present in the file are changed; omitted fields keep their current value.

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
