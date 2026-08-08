# Datadog CLI v1 Plan

> Historical baseline plan. The shipped product is now named `datadog-axi`;
> see [docs/axi-compliance.md](axi-compliance.md) for the current contract.
> Historical design note. The original read-only scope below predates the current
> guarded monitor/dashboard update flow and the added spans, audit, service,
> event, downtime, and SLO domains. For current behavior use
> [docs/axi-compliance.md](axi-compliance.md), [docs/usage.md](usage.md), and
> [docs/documentation-inventory.md](documentation-inventory.md).
>
> The `DATADOG_*` names and “write/mutate workflows” wording below describe the
> pre-AXI baseline only. Current examples use `DD_*`; current writes are limited
> to fingerprint-gated updates of existing monitors and dashboards.

## Objective

Build a production-quality Go CLI for Datadog in this repository using Cobra and the official Datadog Go SDK, optimized for both humans and AI agents.

## Business goal

Provide a single binary that exposes self-discoverable Datadog functionality through CLI help and stable outputs, so users and coding agents can use Datadog without MCP or external docs for basic operation.

## In scope

- Bootstrap a Go CLI project from scratch
- Use the official Datadog Go SDK as the API layer
- Support auth via `DATADOG_API_KEY` and `DATADOG_APP_KEY`
- Support local `.env` loading for development without overriding explicit environment variables
- Support Datadog site selection via `DATADOG_SITE` and `--site`
- Provide AI-friendly help and command discovery
- Establish a scalable domain-first command architecture
- Ship a practical read-only v1 across multiple Datadog surfaces
- Add automated tests and usage docs
- Run read-only real-system verification if credentials are available

## Non-goals

- OAuth flows
- MCP support
- Full Datadog API coverage in v1
- Write/mutate workflows unless later explicitly justified
- Persistent config files beyond env and optional local `.env`

## Constraints

- Go implementation
- Cobra CLI framework
- Official Datadog Go SDK v2
- Secrets only from env or local `.env`; never committed
- Default output concise; machine output stable and compact
- Help output must be good enough for AI self-discovery
- Final repo state must be buildable, tested, and documented

## Proposed command taxonomy

- `version`
- `docs`
- `config doctor`
- `monitor list|get`
- `dashboard list|get`
- `host list|get`
- `metric query`
- `log search`

This uses domain-first top-level nouns and consistent verbs, while reserving room for a future raw `api` namespace if broad full-surface coverage is added later.

## Execution order

1. Create planning and decision artifacts
2. Bootstrap Go module and Cobra entrypoint
3. Implement runtime config, dotenv loading, site handling, auth validation, client factory
4. Implement shared output and help conventions
5. Implement offline commands: `version`, `docs`, `config doctor`
6. Implement read-only Datadog domain commands with tests
7. Finalize docs and examples
8. Run build/tests/help validation and real-system read-only checks

## Testing approach

- Unit tests for config precedence, site normalization, time parsing, and rendering
- Command tests for help and JSON/text output contracts
- Focused API client abstraction tests with fakes
- End-to-end read-only verification against a real Datadog account when `.env` or environment credentials are available
- Real-system checks limited to read-only list/get/query/search flows

## Known early risks

- Exact SDK API shapes vary between Datadog domains and may require adapters
- Metrics/logs time-window UX needs careful normalization and defaults
- Some accounts may return empty results for certain domains; tests and validation must treat empty-but-successful responses as valid
