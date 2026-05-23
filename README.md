# ddog

`ddog` is a single-binary Datadog CLI for AI agents, automation, and terminal-first engineers.

It is designed for the cases where you want Datadog access from a shell, script, CI job, or coding agent runtime where MCP is unavailable, inconvenient, or unnecessary.

`ddog` uses the official Datadog Go SDK, authenticates with environment variables, stays self-discoverable through `--help`, and supports stable JSON output for automation.

## Why this exists

- **MCP fallback for agents**: useful when an agent can run terminal commands but cannot use a Datadog MCP server
- **Fast terminal workflows**: inspect monitors, dashboards, hosts, metrics, and logs without leaving the shell
- **Automation-friendly output**: use concise text for humans or `--output json` for scripts and agents
- **Help-driven discovery**: the command tree is meant to be explored directly from the binary

Current scope is intentionally **read-only**.

## Install

### Recommended: latest release installer

```bash
curl -fsSL https://github.com/nazar256/datadog-cli/releases/latest/download/install.sh | sh
```

This installer selects the correct release archive for your platform and verifies its SHA256 checksum before installing.

### Build from source

```bash
go build -o ddog ./cmd/ddog
./ddog --help
```

### Go install

```bash
go install github.com/nazar256/datadog-cli/cmd/ddog@latest
```

`go install` is useful for source-based workflows, but release binaries are the primary install path and include embedded version metadata.

More install details: [docs/install.md](docs/install.md)

## Authentication

`ddog` reads Datadog credentials from:

- `DATADOG_API_KEY`
- `DATADOG_APP_KEY`
- optional `DATADOG_SITE`

You can also point to a local env file with `--env-file`. By default, `ddog` reads `.env` from the current working directory only.

```bash
export DATADOG_API_KEY=YOUR_DATADOG_API_KEY
export DATADOG_APP_KEY=YOUR_DATADOG_APP_KEY
export DATADOG_SITE=datadoghq.com
ddog doctor
```

Secrets are never accepted as CLI flags.

## Discover commands

Start with built-in help and docs:

```bash
ddog --help
ddog docs summary
ddog docs commands --output json
ddog doctor --help
ddog log search --help
ddog completion --help
```

## Real examples

```bash
# verify auth, site, and output mode
ddog doctor --output json

# inspect monitor coverage for a service
ddog monitor list --name api --limit 20 --output json

# fetch dashboards in concise terminal output
ddog dashboard list --count 20

# query a recent metric window; use JSON when parsing
ddog metric query --query 'avg:system.load.1{*}' --last 1h --output json

# search recent logs for a narrow incident query
ddog log search --query 'service:web status:error' --last 15m --limit 20 --output json
```

## Use with AI agents

`ddog` works well for agents that need Datadog access through ordinary shell commands.

Recommended agent flow:

1. Run `ddog --help` for the command tree, and `ddog docs commands --output json` for high-level command taxonomy guidance.
2. Run `ddog doctor --output json` before live Datadog calls. `ddog config doctor` remains available and resolves to the same configuration check.
3. Prefer `--output json` whenever the result will be parsed.
4. Keep queries narrow and explicit, especially for logs and metrics.
5. For `metric query`, parse the JSON `series` summaries such as `point_count`, `last_point_ts`, and `last_value` instead of assuming raw pointlists or text tables.
6. For `log search`, start with a focused Datadog query and a short `--last` window, then widen only if needed.

Examples:

```bash
ddog version --output json
ddog doctor --output json
ddog monitor list --limit 10 --output json
ddog metric query --query 'avg:system.cpu.user{env:prod}' --last 1h --output json
ddog log search --query 'service:web status:error' --last 15m --limit 20 --output json
```

More: [docs/for-ai-agents.md](docs/for-ai-agents.md)

## Output modes

- default: concise terminal text
- `--output json`: stable machine-readable output

Useful JSON entry points:

```bash
ddog version --output json
ddog docs commands --output json
ddog doctor --output json
ddog monitor list --output json
ddog metric query --query 'avg:system.load.1{*}' --last 1h --output json
```

## Supported v1 command areas

- `doctor`
- `config doctor`
- `completion`
- `docs`
- `version`
- `monitor list|get`
- `dashboard list|get`
- `host list|get`
- `metric query`
- `log search`

## Releases

Release archives are intended for:

- Linux amd64
- Linux arm64
- macOS amd64
- macOS arm64

Linux is the main release target today.

Download binaries and checksums from:

- <https://github.com/nazar256/datadog-cli/releases>

## Troubleshooting auth and API calls

- Run `ddog doctor --output json` first. It reports whether credentials are present without printing secret values.
- Confirm `DATADOG_API_KEY`, `DATADOG_APP_KEY`, and `DATADOG_SITE` match the Datadog account/site you are querying.
- Use `--no-env-file` if you want to ignore a local `.env` file and rely only on exported environment variables.
- If a live command fails, retry with a narrow query and include the command, site, and non-secret error text in bug reports.

## Documentation

- [docs/install.md](docs/install.md)
- [docs/usage.md](docs/usage.md)
- [docs/for-ai-agents.md](docs/for-ai-agents.md)
- [docs/publish-checklist.md](docs/publish-checklist.md)

## Development

```bash
go test ./...
go build ./cmd/ddog
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for lightweight contribution guidance.

## License

[MIT](LICENSE)
