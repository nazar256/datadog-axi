# datadog-axi

`datadog-axi` is the canonical single-binary Datadog CLI for Datadog investigations by AI agents, automation, and terminal-first engineers. `ddog` is retained only as an explicit deprecated compatibility build path.

It is designed for the cases where you want Datadog access from a shell, script, CI job, or coding agent runtime where MCP is unavailable, inconvenient, or unnecessary.

`datadog-axi` calls documented official Datadog APIs through the official Datadog
Go SDK, follows the [AXI CLI guidance](https://axi.md/), stays self-discoverable
through `--help`, and emits deterministic TOON-like output by default. Use
`--json` for stable normalized JSON; exports preserve full SDK specifications. The
default renderer is documented as TOON-like rather than a full TOON-conformance
claim; see [the principle mapping](docs/axi-compliance.md).

The design studied Datadog's [Pup](https://github.com/DataDog/pup) for useful
investigation and agent-ergonomics ideas, but does not clone Pup's architecture,
command count, implementation, documentation, or agent system. `datadog-axi` is a
focused investigation CLI with a smaller, deliberately bounded write surface.

## Why this exists

- **MCP fallback for agents**: useful when an agent can run terminal commands but cannot use a Datadog MCP server
- **Fast terminal workflows**: inspect monitors, dashboards, hosts, metrics, and logs without leaving the shell
- **Automation-friendly output**: use compact TOON-like output by default or `--json` for scripts and agents
- **Help-driven discovery**: the command tree is meant to be explored directly from the binary

The interface is shaped around concrete investigation costs: bounded previews and
field projection reduce oversized payloads; explicit cursors, counts, and time
ranges make pagination predictable; definitive empty states avoid re-running a
successful query just to confirm “nothing”; related domains (spans, metrics,
monitors, events, SLOs, downtimes, and service ownership) reduce round trips;
export/validate/dry-run/fingerprint checks keep mutation interfaces narrow; and
contextual next-command suggestions make the next safe step discoverable.

Investigation commands are read-only by default. The only write surface is an
explicit, fingerprint-gated update of an existing monitor or dashboard; no
create, delete, mute, or arbitrary API writes are exposed.

## Install

### Release installer (recommended)

```bash
curl -fsSL https://github.com/nazar256/datadog-axi/releases/latest/download/install.sh | sh
```

### Go install

```bash
go install github.com/nazar256/datadog-axi/cmd/datadog-axi@latest
```

### Build from source

```bash
mkdir -p .tmp/bin
go build -o .tmp/bin/datadog-axi ./cmd/datadog-axi
./.tmp/bin/datadog-axi
```

More install details: [docs/install.md](docs/install.md)

## Authentication

`datadog-axi` reads Datadog credentials from:

- `DD_API_KEY` (legacy `DATADOG_API_KEY`)
- `DD_APP_KEY` (legacy `DATADOG_APP_KEY`)
- optional `DD_SITE` (legacy `DATADOG_SITE`)

You can also point to a local env file with `--env-file`; this is isolated from layered discovery. Without it, user config, the repository-root, and cwd `.env` files are considered. `--no-env-file` disables all file discovery.

```bash
export DD_API_KEY=YOUR_DATADOG_API_KEY
export DD_APP_KEY=YOUR_DATADOG_APP_KEY
export DD_SITE=datadoghq.com
datadog-axi doctor --json
```

Secrets are never accepted as CLI flags.

## Discover commands

Start with built-in help and docs:

```bash
datadog-axi --help
datadog-axi docs summary
datadog-axi docs commands --json
datadog-axi doctor --help
datadog-axi log search --help
datadog-axi completion --help
```

## Real examples

```bash
# verify auth, site, and output mode
datadog-axi doctor --json

# inspect monitor coverage for a service
datadog-axi monitor list --name api --limit 20 --json

# fetch dashboards in concise terminal output
datadog-axi dashboard list --count 20

# query a recent metric window; use JSON when parsing
datadog-axi metric query --query 'avg:system.load.1{*}' --last 1h --json
datadog-axi metric search --query 'system.cpu' --limit 20 --json
datadog-axi metric active --last 1h --limit 50 --json

# search recent logs for a narrow incident query
datadog-axi log search --query 'service:web status:error' --last 15m --limit 20 --json
```

## Use with AI agents

`datadog-axi` works well for agents that need Datadog access through ordinary shell commands.

Recommended agent flow:

1. Run `datadog-axi` for the bounded home view, then `datadog-axi --help` or `datadog-axi docs commands --json` for taxonomy guidance.
2. Run `datadog-axi doctor --json` before live Datadog calls. `datadog-axi config doctor` remains available for explicit command-tree discovery.
3. Prefer `--json` whenever the result will be parsed.
4. Keep queries narrow and explicit, especially for logs and metrics.
5. For `metric query`, parse the JSON `series` summaries and complete `points` arrays instead of assuming text tables.
6. For `log search`, start with a focused Datadog query and a short `--last` window, then widen only if needed.

Examples:

```bash
datadog-axi --version --json
datadog-axi doctor --json
datadog-axi monitor list --limit 10 --json
datadog-axi metric query --query 'avg:system.cpu.user{env:prod}' --last 1h --json
datadog-axi log search --query 'service:web status:error' --last 15m --limit 20 --json
```

More: [docs/for-ai-agents.md](docs/for-ai-agents.md)

Bounded, command-by-command investigation recipes are in
[docs/investigation-guides.md](docs/investigation-guides.md).

## Output modes

- default: deterministic TOON-like AXI output
- `--json`: stable normalized machine-readable output (`--output json` remains a compatibility spelling)
- `--output text`: legacy human table output
- `--fields id,name`: project structured output to selected top-level fields
- `--full`: disable table previews and retain complete metric points where applicable

Useful JSON entry points:

```bash
datadog-axi --version --json
datadog-axi docs commands --json
datadog-axi doctor --json
datadog-axi monitor list --json
datadog-axi metric query --query 'avg:system.load.1{*}' --last 1h --json
```

## Supported command areas

- `doctor`
- `config doctor`
- `completion`
- `docs`
- `version`
- `monitor list|search|get`
- `dashboard list|get` (with page-local `--filter`)
- `host list|get`
- `metric query`
- `metric search`
- `metric active`
- `metric metadata`
- `log search|aggregate`
- `event list|get`
- `slo list|search|get` (optional bounded history)
- `downtime list|get`
- `monitor export|validate|update` and `dashboard export|validate|update` preserve full SDK specifications and guard existing-resource edits.
- `span list|aggregate|services|resources|operations`, `audit list`, and `service list|get` use official Datadog SDK adapters with bounded ranges, pagination, and normalized fields.

## Releases

Release archives are intended for:

- Linux amd64
- Linux arm64
- macOS amd64
- macOS arm64

Linux is the main release target today.

Download binaries and checksums from:

- <https://github.com/nazar256/datadog-axi/releases>

## Troubleshooting auth and API calls

- Run `datadog-axi doctor --json` first. It reports whether credentials are present without printing secret values.
- Confirm `DD_API_KEY`, `DD_APP_KEY`, and `DD_SITE` (or legacy `DATADOG_*` aliases) match the Datadog account/site you are querying.
- Use `--no-env-file` if you want to ignore a local `.env` file and rely only on exported environment variables.
- If a live command fails, retry with a narrow query and include the command, site, and non-secret error text in bug reports.

## Documentation

- [docs/install.md](docs/install.md)
- [docs/usage.md](docs/usage.md)
- [docs/for-ai-agents.md](docs/for-ai-agents.md)
- [docs/investigation-guides.md](docs/investigation-guides.md)
- [docs/axi-compliance.md](docs/axi-compliance.md)
- [docs/documentation-inventory.md](docs/documentation-inventory.md)
- [docs/publish-checklist.md](docs/publish-checklist.md)

## Development

```bash
go test ./...
go build ./cmd/datadog-axi
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for lightweight contribution guidance.

## License

[MIT](LICENSE)
