# Suggested GitHub metadata for datadog-axi

These settings are configured in the GitHub UI, not from repository files.

## Repository description

Suggested description:

> Single-binary Datadog CLI for AI agents, automation, and terminal-first engineers when MCP is unavailable.

## Homepage

Suggested homepage until a dedicated docs site exists:

> `https://github.com/nazar256/datadog-axi#readme`

## Topics

Suggested topics:

- `datadog`
- `cli`
- `automation`
- `ai-agents`
- `terminal`
- `observability`
- `logs`
- `metrics`
- `monitors`
- `dashboards`
- `golang`

## Social preview direction

Use a simple terminal screenshot or mockup that shows:

- `datadog-axi --help`
- one JSON example such as `datadog-axi log search --query 'service:web' --last 15m --json`
- a short caption like: `Datadog CLI for AI agents and automation when MCP is unavailable`

## First public release notes should cover

1. what `datadog-axi` is and who it is for
2. supported Datadog surfaces in v1: monitors, dashboards, hosts, metrics, metric metadata, logs, APM spans, events, audit logs, SLOs, downtimes, and service catalog inspection; only existing monitors and dashboards have guarded updates
3. install options: release installer, release assets, source build
4. auth model: `DD_API_KEY`, `DD_APP_KEY`, optional `DD_SITE` (legacy `DATADOG_*` aliases remain supported)
5. AXI discovery and `--json`
6. checksum-verified release assets and supported platforms
