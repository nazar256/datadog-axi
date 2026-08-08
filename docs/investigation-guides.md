# Investigation guides

These recipes keep each investigation bounded and explicit. They are intended
for humans and agents using only the `datadog-axi` binary; there is no hidden
“investigate everything” command. Start with a narrow time window and page, then
widen or follow a returned cursor deliberately.

## 1. Error log to trace

1. Search a small window and keep the result machine-readable:

   ```bash
datadog-axi log search --query 'service:web status:error' --last 15m --limit 20 --json
   ```

2. Use the returned service, host, trace identifiers, or request attributes to
   narrow APM span search. Prefer dedicated identifiers when the log carries
   them; the adapter preserves `trace_id` and `span_id` from log attributes:

   ```bash
   datadog-axi span list --trace-id '<trace-id>' --last 15m --limit 50 --json
   datadog-axi span list --span-id '<span-id>' --last 15m --limit 50 --json
   ```

   If no identifier is available, combine explicit service/environment filters
   with the original query rather than issuing an unbounded search:

   ```bash
   datadog-axi span list --query 'status:error' --service web --env prod --last 15m --limit 50 --json
   ```

3. If `next_cursor` is present, continue with the same query and cursor. Do not
   silently assume that one page is the complete incident population.
4. Use `--full` only for text previews that were explicitly truncated; JSON
   already carries the normalized fields and span attributes returned by the API.

For explicit log analytics, keep buckets separate from event search and bound
pagination:

```bash
datadog-axi log aggregate --query 'service:web' --last 15m \
  --facet status --compute count --all --max-pages 3 --json
```

For a server-side bounded aggregate, use the explicit analytics command and
keep the grouping and computation visible:

```bash
datadog-axi span aggregate --query 'service:web env:prod' --last 15m \
  --group-by service --compute count --json
```

The result is explicitly bucketed and may include rate-limit warnings; it is
not a client-side aggregate of one page.

## 2. Metric assessment and metadata

1. Query the smallest useful window:

   ```bash
   datadog-axi metric query --query 'avg:system.cpu.user{env:prod}' --last 1h --json
   ```

2. If the metric name is unknown, discover recent or actively reporting names:

   ```bash
   datadog-axi metric search --query 'system.cpu' --limit 20 --json
   datadog-axi metric active --last 1h --tag-filter 'env:prod' --limit 50 --json
   ```

3. Inspect the metric definition before interpreting tags, unit, or monitor
   suitability:

   ```bash
   datadog-axi metric metadata system.cpu.user --json
   ```

4. Compare the returned series summaries and complete `points` arrays. Widen the
   range only when the first window does not answer the question. Use `--fields`
   for a top-level projection when a downstream parser needs less context.

## 3. Monitor type and query

1. Find candidates with a bounded list filter:

   ```bash
   datadog-axi monitor list --name api --limit 20 --json
   ```

2. Inspect the full monitor, including its actual type and query:

   ```bash
   datadog-axi monitor get 123456 --json
   datadog-axi monitor export 123456 --file .tmp/monitor.json
   ```

   For the dedicated monitor-search endpoint, use `monitor search` with its
   query/page/per-page/sort controls.

3. Treat `type`, `query`, thresholds, tags, and evaluation settings as separate
   facts. A list summary is not an update payload.

## 4. Events, deployments, and audit trail

Use a source/service filter and a narrow range for event context:

```bash
datadog-axi event list --last 1h --sources deploy --limit 50 --json
datadog-axi event get 123456789 --json
datadog-axi audit list --service monitors --action update --last 1h --limit 50 --json
datadog-axi audit list --actor alice@example.com --resource monitor-123 --tag env:prod --last 1h --json
```

Audit filters are translated into the documented audit-log query syntax. The
normalized result exposes actor, service, action, resource, changed fields, and
arbitrary attributes when the API provides them; a missing projection is not
proof that the event lacked that field.

Event payloads and audit attributes may contain customer-controlled or sensitive
fields. Store redirected JSON according to the same controls as Datadog data.

## 5. SLO and downtime state

Check service health and whether an active maintenance window explains an alert:

```bash
datadog-axi slo list --query checkout --limit 20 --json
datadog-axi slo search --query checkout --limit 20 --json
datadog-axi slo get slo-123 --json
datadog-axi downtime list --current-only --limit 50 --json
datadog-axi downtime get downtime-123 --json
```

SLO and downtime commands are read-only. A zero-result response is a successful
answer with explicit count/state; it is not an invitation to retry with an
unbounded query.

## 6. Find the owner

Use the service catalog to connect a service name to an owner, lifecycle, tier,
documentation, and dependencies:

```bash
datadog-axi service list --filter checkout --limit 20 --json
datadog-axi service get checkout --json
```

The service-list filter is applied to the returned bounded page. Use `--offset`
to inspect later pages; do not interpret a filtered page as a server-side total.

## 7. Export, validate, dry-run, apply, verify

The only mutable resource types are existing monitors and dashboards.

1. Export the complete SDK specification (unknown fields are retained):

   ```bash
   datadog-axi monitor export 123456 --file .tmp/monitor.json
   datadog-axi dashboard export abc-def-ghi --file .tmp/dashboard.json
   ```

2. Edit a copy, then validate its local shape without contacting Datadog:

   ```bash
   datadog-axi monitor validate --file .tmp/monitor.json
   datadog-axi dashboard validate --file .tmp/dashboard.json
   ```

3. Fetch the live resource and review a semantic dry-run. Dry-run is the default;
   `--dry-run` makes intent explicit:

   ```bash
   datadog-axi monitor update 123456 --file .tmp/monitor.json --dry-run --json
   datadog-axi dashboard update abc-def-ghi --file .tmp/dashboard.json --dry-run --json
   ```

   Review `live_fingerprint`, every diff path, and any redaction marker. The
   fingerprint is computed from the live canonical JSON, not from the local file.

4. Apply only after recording the reviewed fingerprint:

   ```bash
   datadog-axi monitor update 123456 --file .tmp/monitor.json --apply --fingerprint '<reviewed-live-fingerprint>' --json
   ```

   Apply is non-interactive, refuses a stale fingerprint by default, performs one
   write, and re-fetches the resource to verify the requested state. If a reviewer
   has explicitly accepted the race, `--allow-stale` can be added alongside the
   expected fingerprint; do not use it as a convenience retry. A no-op exits
   successfully without writing. There is no create, delete, mute, downtime-cancel,
   or arbitrary API write command.

## Output and failure discipline

- Prefer `--json` for parsing; default output is deterministic TOON-like text.
- Use `--fields a,b` only with TOON/JSON; it projects top-level fields.
- Use `--full` when a text preview or metric point display says it is truncated.
- Structured errors are on stdout with exit code 2 for usage and 1 for operational
  failures. They include a corrective command where one is known.
- No-argument output, list results, and mutations include contextual next commands
  when another action is useful. Detail views that fully answer a request omit
  unnecessary suggestions.
