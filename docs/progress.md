# Progress Log

> Historical implementation log. Current behavior and deferred surfaces are
> tracked in [docs/axi-compliance.md](axi-compliance.md).

## 2026-03-21

- Initialized Datadog CLI v1 plan for a near-empty repository.
- Chosen architecture: domain-first curated CLI with reusable runtime/output layers and future room for a raw API namespace.
- Chosen v1 scope: offline support commands plus read-only monitor, dashboard, host, metric, and log workflows.
- Key open implementation risks to validate early:
  - Datadog Go SDK method shapes and pagination differ by domain.
  - Site handling must be explicit and testable.
  - Metrics/logs commands need compact but still useful default output for humans and agents.
- Scaffolded root CLI, global config handling, `version`, `docs`, and `config doctor`.
- Started adding live read-only domain commands and domain adapters for monitors, dashboards, hosts, metrics, and logs.
- Current pause point: the newest domain-command patch is incomplete and has compile issues to resolve before tests can pass. Waiting for implementer provider reconfiguration before continuing the implementation/review loop.
- Resumed and completed the live command implementation.
- Added read-only commands for monitors, dashboards, hosts, metrics, and logs with domain-shaped JSON/text output and command-level tests.
- Fixed reviewer-reported blockers:
  - root binary now prints errors to stderr before exiting
  - metric/log absolute time ranges now work without conflicting default `--last` values
  - site validation now only permits supported Datadog sites and aliases
  - live command context is now propagated through to the SDK client
  - `host get` now paginates across host inventory pages
- Added user docs: `README.md`, `docs/usage.md`, and `.env.example`.
- Continued review-driven hardening beyond the first reviewer findings:
  - sanitized terminal text output to avoid control-sequence injection
  - aligned `version` with global JSON output mode
  - switched monitor pagination flags to explicit Datadog-style `--offset` and `--limit`
  - made dashboard/host/log optional timestamps pointer-based for cleaner JSON
  - made metric/log text output include returned-count and range summaries
  - updated docs to match strict supported-site validation
- Final config/runtime hardening:
  - process env now masks `.env` even when explicitly set to empty values
  - negative timeout values now fail fast
  - verified empty-env and negative-timeout behavior from the built CLI
