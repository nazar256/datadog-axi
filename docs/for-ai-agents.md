# For AI agents

`ddog` is useful when an agent needs Datadog access from a terminal, script, or sandbox where MCP is unavailable or impractical.

## Authenticate

Use environment variables or an explicit `.env` file:

```bash
export DATADOG_API_KEY=...
export DATADOG_APP_KEY=...
export DATADOG_SITE=datadoghq.com
```

Or:

```bash
ddog --env-file .env doctor
```

## Discover the command tree

Start with built-in help and docs:

```bash
ddog --help
ddog docs summary
ddog docs commands --output json
ddog doctor --help
ddog log search --help
ddog completion --help
```

Use `--help` for the actual command tree. `ddog docs commands --output json` is a compact machine-readable summary of the command taxonomy, not a full command listing.

## Prefer machine-readable output when parsing results

```bash
ddog version --output json
ddog doctor --output json
ddog monitor list --limit 10 --output json
ddog log search --query 'service:web status:error' --last 15m --output json
```

## Good agent workflows

Check auth and site before live calls:

```bash
ddog doctor --output json
```

`ddog config doctor` is still supported and returns the same data, but `ddog doctor` is the shortest discoverable path.

Discover monitors related to a service:

```bash
ddog monitor list --name api --limit 20 --output json
```

Pull recent logs for a narrow incident query:

```bash
ddog log search --query 'service:web status:error' --last 15m --limit 20 --output json
```

When exploring logs, start with a focused query and short `--last` window, then add terms such as `env:prod`, `host:web-01`, or `@http.status_code:[500 TO 599]` before widening the range.

Inspect a metric window:

```bash
ddog metric query --query 'avg:system.cpu.user{env:prod}' --last 1h --output json
```

For `metric query`, parse the JSON `series` array. Each series is summarized with fields such as:

- `point_count`: number of non-empty points returned for the series
- `last_point_ts`: timestamp of the last non-empty point
- `last_value`: numeric value of that last point

Do not assume raw pointlists are present in CLI output; if you need stable parsing, always request `--output json`.

## Completion

The CLI exposes the built-in Cobra completion command:

```bash
ddog completion bash
ddog completion zsh
ddog completion fish
ddog completion powershell
```

Current scope is intentionally read-oriented: monitors, dashboards, hosts, metrics, and logs.
