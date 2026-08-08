# datadog-axi usage

`datadog-axi` follows the [AXI CLI guidance](https://axi.md/). It uses official Datadog APIs and the pinned Go SDK, is read-only by default, and only exposes guarded update workflows for existing monitors and dashboards.

## Quick start

```bash
mkdir -p .tmp/bin
go build -o .tmp/bin/datadog-axi ./cmd/datadog-axi
./.tmp/bin/datadog-axi
./.tmp/bin/datadog-axi doctor --json
```

## Authentication and environment precedence

Preferred variables are `DD_API_KEY`, `DD_APP_KEY`, and `DD_SITE`. The legacy `DATADOG_API_KEY`, `DATADOG_APP_KEY`, and `DATADOG_SITE` aliases remain supported. Flags override process variables; process variables override merged env files; more-specific files override less-specific files. Default files are user config, the repository root, then the current directory. `--env-file` isolates loading to one file and `--no-env-file` disables files; explicit env paths must be regular files, not symlinks.

Configured Datadog API/app key values are never accepted as flags or emitted in
diagnostics. Raw API and customer-controlled fields can still contain sensitive
values; treat those fields as data rather than assuming the CLI can identify every
secret-shaped value.

Event payloads, audit attributes, downtime relationships, span attributes,
service definitions, and monitor/dashboard specifications can contain
customer-controlled or sensitive fields. Treat JSON exports and redirected
output as sensitive data and apply your normal storage and access controls.

## Output and discovery

No arguments show a bounded home view with executable identity, site/auth readiness,
available and deferred domains, and next commands. Default output is deterministic
TOON-like AXI output. Use `--json` (or legacy `--output json`) for stable normalized
machine-readable output and `--output text` for legacy tables. `--help` is concise
and command-specific.

Use `--fields field1,field2` with TOON or JSON to project structured results;
use `--full` when a text preview or metric point summary should be expanded.

```bash
datadog-axi --help
datadog-axi docs commands --json
datadog-axi completion bash
```

## Investigation commands

```bash
datadog-axi monitor list --name api --limit 20 --json
datadog-axi dashboard list --count 20 --json
datadog-axi host list --filter web --json
datadog-axi metric query --query 'avg:system.cpu.user{env:prod}' --last 1h --json
datadog-axi metric search --query 'system.cpu' --limit 20 --json
datadog-axi metric active --last 1h --tag-filter 'env:prod' --limit 50 --json
datadog-axi metric metadata system.cpu.user --json
datadog-axi slo list --query checkout --limit 20 --json
datadog-axi slo get slo-123 --json
datadog-axi event list --last 1h --sources deploy --json
datadog-axi downtime list --current-only --json
datadog-axi log search --query 'service:web status:error' --last 15m --limit 20 --json
# continue a paginated log search with the returned cursor
datadog-axi log search --query 'service:web status:error' --last 15m --cursor '<next-cursor>' --json
datadog-axi span list --query 'service:web env:prod' --last 15m --limit 20 --json
datadog-axi audit list --query 'service:monitors' --last 1h --limit 20 --json
datadog-axi service list --filter checkout --limit 20 --json
```

APM spans, audit logs, and service catalog use the official Datadog SDK v2
search/definition endpoints with bounded ranges, filters, pagination, and stable
normalized fields. They do not silently call private endpoints.

## Existing-resource safety

```bash
datadog-axi monitor validate --file monitor.json
datadog-axi monitor export 123456 --file .tmp/monitor.json
datadog-axi monitor update 123456 --file monitor.json --dry-run
datadog-axi monitor update 123456 --file monitor.json --apply --fingerprint '<reviewed-live-fingerprint>'
datadog-axi monitor update 123456 --file monitor.json --apply --fingerprint '<old-fingerprint>' --allow-stale
datadog-axi dashboard update abc-def-ghi --file dashboard.json --dry-run
```

Validation and export do not mutate Datadog. Update is dry-run by default; apply
is explicit, non-interactive, fingerprint-gated, and re-fetches the resource to
verify the result. A stale fingerprint is refused unless a reviewer explicitly
adds `--allow-stale` with the expected fingerprint. No resource creation,
deletion, muting, or arbitrary writes are available.

## Notes

- Empty and unavailable results are reported explicitly.
- `metric search` uses Datadog's recent metric index and `metric active` lists
  names reported since a bounded lookback; inspect discovered names with
  `metric metadata` rather than inferring type or tags from a name.
- Time ranges accept `--last` or `--from/--to`, never both.
- Keep log and metric windows narrow, then widen deliberately.
- [install.md](install.md) covers release installation.
- [for-ai-agents.md](for-ai-agents.md) covers agent workflows.
- [investigation-guides.md](investigation-guides.md) covers bounded cross-domain
  recipes, pagination, ownership, and the export/validate/update safety sequence.
- [axi-compliance.md](axi-compliance.md) records implemented AXI behaviors and deliberate boundaries.
- [documentation-inventory.md](documentation-inventory.md) records what is
  current, historical, or deferred.
