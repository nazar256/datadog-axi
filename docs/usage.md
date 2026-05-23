# Datadog CLI usage

## Quick start

Build the CLI:

```bash
go build -o ddog ./cmd/ddog
```

Check local configuration:

```bash
./ddog doctor
./ddog --env-file .env doctor --output json
./ddog config doctor
```

## Authentication

`ddog` reads credentials from:

- `DATADOG_API_KEY`
- `DATADOG_APP_KEY`
- optional `DATADOG_SITE`

You can also use a local `.env` file in the current directory, or pass an explicit file with `--env-file`.

Secrets are never accepted as CLI flags.

## Command discovery

Use help output as the primary interface:

```bash
ddog --help
ddog docs summary
ddog docs commands --output json
ddog doctor --help
ddog log search --help
ddog completion --help
```

Shell completion is available from the installed binary:

```bash
ddog completion bash
ddog completion zsh
ddog completion fish
ddog completion powershell
```

## Output modes

- default: concise text for terminals
- `--output json`: stable machine-readable output

Examples:

```bash
ddog doctor --output json
ddog monitor list --output json
ddog docs commands --output json
```

## Read-only v1 commands

### Monitors

```bash
ddog monitor list
ddog monitor list --name api --limit 20
ddog monitor get 123456
```

### Dashboards

```bash
ddog dashboard list --count 20
ddog dashboard get abc-def-ghi
```

### Hosts

```bash
ddog host list --filter web
ddog host get web-01
```

### Metrics

Use JSON whenever an agent or script will parse results. Metric JSON returns series summaries, not raw pointlists: `point_count` is the number of non-empty points returned, `last_point_ts` is the timestamp of the last non-empty point, and `last_value` is that point's numeric value.

```bash
ddog metric query --query 'avg:system.load.1{*}' --last 1h --output json
ddog metric query --query 'avg:system.cpu.user{env:prod}' --from 2026-03-21T09:00:00Z --to 2026-03-21T10:00:00Z --output json
```

### Logs

Start with a focused Datadog query and a short time window, then widen only if needed. Useful patterns include service/status filters, env filters, and attribute searches such as `@http.status_code:[500 TO 599]`.

```bash
ddog log search --query 'service:web status:error' --last 15m
ddog log search --query 'env:prod @http.status_code:[500 TO 599]' --last 30m --index main --limit 20 --output json
```

## Notes

- All shipped v1 Datadog commands are read-only.
- `ddog doctor` is the shortest path for config checks; `ddog config doctor` remains supported for explicit command-tree discovery.
- Empty results are valid outcomes.
- Supported sites are: `us1`, `us3`, `us5`, `eu`, `ap1`, `ap2`, `us1-fed`, and their canonical hostnames.
- Install and release details live in [install.md](install.md).
- AI-agent-specific guidance lives in [for-ai-agents.md](for-ai-agents.md).
