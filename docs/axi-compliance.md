# datadog-axi AXI principle mapping

`datadog-axi` is designed according to the [AXI guidance](https://axi.md/) and
uses the official Datadog Go SDK behind domain adapters. This document maps the
current implementation to each AXI principle and calls out boundaries instead of
claiming catalog membership or complete conformance.

`ddog` remains a deprecated compatibility build path and is intentionally absent
from primary examples. The canonical module and repository are
`github.com/nazar256/datadog-axi`.

## Principle 1: token-efficient output

Implemented: output is deterministic and compact by default; normalized JSON is
available with `--json`/`--output json`; `--fields a,b` projects top-level fields
for TOON/JSON. Internal domain adapters remain JSON-shaped and the output boundary
handles rendering.

Boundary: the default renderer is a deterministic **TOON-like subset**, not a
claim of strict TOON conformance. A maintained Go encoder and upstream fixture
suite are not currently part of the repository, so strict conformance is deferred
rather than implied.

## Principle 2: minimal default schemas

Implemented: list commands use compact summary tables, bounded limits, and
domain-specific normalized fields. `--fields` is the explicit expansion/projected
schema escape hatch. Detail and export commands carry the complete fields needed
for inspection or safe review; reduced list models are never reused as update
payloads.

Boundary: some official API responses expose provider-specific attributes, so
normalized detail output may include an `attributes`, `raw`, or payload map. The
CLI preserves those fields for investigation rather than silently discarding them.

## Principle 3: content truncation

Implemented: human tables preview long content and metric points; `--full` disables
those previews where supported. JSON responses retain the normalized data needed
for parsing, and monitor/dashboard export preserves the full SDK specification.

Boundary: truncation metadata is command-specific rather than a universal wrapper;
consult the command's concise help when a detail surface has a domain-specific
escape hatch.

## Principle 4: pre-computed aggregates

Implemented: list/search results expose a returned-item `count`, time range, query,
and cursor where the API supplies one. Metric output includes per-series summaries
and points; event, downtime, SLO, span, audit, and service results include bounded
pagination controls.

Boundary: when an endpoint does not return a global total, `count` means the
returned page, not an invented account-wide count. Span aggregation by service
or resource is available as bounded server-side buckets; the CLI does not claim
a global total across unbounded pages.

## Principle 5: definitive empty states

Implemented: successful zero-result lists are rendered with explicit zero/count and
query or filter context, distinct from unavailable or permission errors. Empty
results are exit-code-zero answers and do not trigger implicit retries or broadening.

## Principle 6: structured errors and exit codes

Implemented: canonical stdout carries structured errors with sanitized dependency
messages and corrective suggestions. Exit code `2` is for usage/validation,
`1` for operational/API failures, and `0` includes successful no-ops. Required
flags and mutually exclusive options are validated before API calls; Cobra rejects
unknown flags/arguments; commands never prompt. Credentials and secret-bearing
payloads are not printed.

Boundary: the deprecated compatibility wrapper may retain legacy presentation
details, but primary documentation and canonical binary behavior follow this
contract.

## Principle 7: ambient context via session integrations

Deferred: no session hook, plugin, or automatic start-up API call is installed.
Ambient context is not required for this milestone. Any future integration must be
explicit opt-in, idempotent, directory-scoped, secret-safe, and bounded in output,
API calls, and latency.

## Principle 8: content first

Implemented: invoking `datadog-axi` without arguments shows a bounded home view
with executable identity, purpose, auth/site readiness, available domains, and
next commands. It is content and orientation, not a large usage manual. Offline
help/docs remain available for deliberate discovery.

## Principle 9: contextual disclosure

Implemented: list/search and mutation responses include a few actionable next
commands, carrying forward returned cursors plus query and time bounds where
known. Empty results explain the successful scope and suggest a bounded follow-up;
detail views omit noise when they are self-contained. `--full`, cursor, export,
get, and service-owner follow-ups are discoverable from relevant output/help.

Boundary: suggestions are command-specific rather than a generic workflow engine;
the CLI does not launch an expensive multi-domain investigation automatically.

## Principle 10: consistent help

Implemented: executable domain commands have concise Cobra help with required
arguments, flags, defaults, and examples; utility topic renderers provide their
guidance through topic output. `docs commands --json` exposes machine-readable taxonomy,
read-only status, pagination, and mutation notes. The home view and docs topics
provide orientation without making a skill or external integration mandatory.

## Safety and API boundaries

- Investigation commands are read-only by default.
- Only existing monitors and dashboards are mutable.
- Updates are non-interactive, dry-run by default, require `--apply` plus a live
  SHA-256 fingerprint, reject stale state by default, and re-fetch to verify after
  one write. A reviewed stale override is possible only with explicit
  `--allow-stale` and the expected fingerprint.
- Unknown SDK fields are retained in exported/update representations; summary
  models are not used as write payloads.
- No create, delete, mute/unmute, downtime cancel/create, event create, or other
  arbitrary Datadog write is exposed.
- Search and detail adapters use documented official SDK endpoints. API-specific
  pagination or schema limitations are surfaced rather than hidden behind private
  requests.

## Deliberate project boundaries

The CLI is not Pup parity, an MCP server, a plugin runtime, OAuth client, or
credential store. The Pup repository was studied for ideas, not cloned. The
default renderer's TOON-like boundary, lack of a global span aggregation endpoint,
and absence of ambient context are explicit limitations. The optional Agent Skill
is deferred because the standalone CLI and repository guides currently cover the
representative workflows without a maintainable synchronization source.

This repository is the published `datadog-axi` fork of the earlier `datadog-cli`
/`ddog` line. Module path, release assets, installer URLs, and docs all use
`github.com/nazar256/datadog-axi`.
