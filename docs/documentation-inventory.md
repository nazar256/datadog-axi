# Documentation inventory and disposition

This inventory is the documentation source of truth for `datadog-axi`.
The canonical product, module, and GitHub repository are
`github.com/nazar256/datadog-axi`. The earlier `datadog-cli` / `ddog` line remains
historical compatibility context only.

## Inventory

| Surface | Disposition | Current purpose or follow-up |
| --- | --- | --- |
| `README.md` | Current | Product identity, AXI link, read-only default, guarded writes, installation, auth precedence, output, investigation workflows, limitations, and development checks. |
| `CONTRIBUTING.md` | Current | Current binary name, env aliases, docs/example synchronization, tests, and secret-handling expectations. |
| `SECURITY.md` | Retained | Generic vulnerability-reporting policy remains accurate; it does not request credentials or authorize production changes. |
| `LICENSE` | Retained | MIT license; no product or installation claims are embedded in it. |
| `go.mod` / `go.sum` | Current | Module identity is `github.com/nazar256/datadog-axi`. |
| `.gitignore` | Current | Keeps env files, `.tmp/` artifacts, and local binary build outputs out of normal changes. |
| `.env.example` | Current | Preferred `DD_*` names, legacy aliases, and precedence/safety comments. It contains placeholders only. |
| `docs/README.md` | Current | Indexes public guidance, this inventory, and historical planning records. |
| `docs/documentation-inventory.md` | Current | This inventory records every documentation surface and its disposition. |
| `docs/usage.md` | Current | Quick start, environment precedence, output contract, supported domains, safe edits, and links to investigation guides. |
| `docs/for-ai-agents.md` | Current | Compact agent workflow, pagination/time-window discipline, contextual disclosure, and safe mutation sequence. |
| `docs/investigation-guides.md` | Current | Explicit, bounded workflows for logs-to-traces, metric discovery/metadata, monitors, events, SLOs, downtimes, ownership, exports, and guarded updates. |
| `docs/axi-compliance.md` | Current | Principle-by-principle implementation mapping, truthful TOON boundary, and deliberate project limits. |
| `docs/axi-ecosystem-study.md` | Current | Source study for AXI catalog/community examples and Pup comparison; external links are evidence, not runtime dependencies. |
| `docs/install.md` | Current | Release installer, Go install, source build, and maintainer release checklist. |
| `docs/github-metadata.md` | Current | Suggested repository metadata and release notes content. GitHub UI topics/homepage may still need a one-time manual confirm. |
| `docs/publish-checklist.md` | Current | Separates checked-in readiness from release publication steps. |
| `docs/publish-plan.md` | Historical | Planning note remains useful provenance and points to the current README/AXI contract. |
| `docs/plan.md` | Historical | Original pre-AXI design; its narrower scope and read-only non-goal are explicitly historical, not a capability claim. |
| `docs/progress.md` | Historical | Chronological implementation notes; current behavior is delegated to `docs/axi-compliance.md`. |
| `docs/tasks.md` | Historical | Pre-AXI task ledger; not the current feature matrix. |
| `docs/adr/ADR-0001-domain-first-cli.md` | Retained | Architecture decision remains consistent with the domain adapters; later domains extend rather than invalidate it. |
| `docs/adr/ADR-0002-config-and-auth.md` | Current | Preferred `DD_*` variables and layered env-file behavior. |
| `install.sh` | Current | Checksum-verifying installer for `nazar256/datadog-axi` release assets. |
| `.github/workflows/ci.yml` | Current | Tests, canonical build, help/docs smoke checks, and archive-shape checks. |
| `.github/workflows/release.yml` | Current | Cross-platform archives, checksums, installer asset, and draft-release guard. |
| Built-in command help and metadata | Current | `--help` and `docs commands --json` are executable guidance. |
| Command examples | Current | Examples use `datadog-axi`; compatibility `ddog` is intentionally absent from primary examples. |
| Optional Agent Skill | Deferred | Home view, help, structured errors, and repository docs already cover discovery. |
| Ambient session context | Deferred | No hooks or plugins are installed. |

## Stale-name classification

| Occurrence class | Examples | Disposition |
| --- | --- | --- |
| Historical provenance | Mentions of the earlier `datadog-cli` repository in historical docs | Keep clearly labelled as historical. |
| Compatibility wrapper | `cmd/ddog` deprecated build path | Keep; not part of release assets or primary docs. |
| Domain site alias | Datadog gov site hostnames containing `ddog-gov.com` | Keep; these are Datadog site values, not the old binary name. |
| Preferred product identity | `datadog-axi`, `github.com/nazar256/datadog-axi`, installer/release URLs | Required current spelling. |

The standard `DD_API_KEY`, `DD_APP_KEY`, and `DD_SITE` names are preferred.
The `DATADOG_*` aliases remain a compatibility input, not the preferred
documentation spelling.

## Optional skill and ambient-context decision

An Agent Skill and ambient session context remain deferred for this milestone:
current help, home view, structured errors, and repository guides already cover
the representative workflows without a maintainable skill synchronization source.

## Verification scope

Documentation examples that do not require credentials are checked with the
canonical binary's help/docs smoke paths and by searching for stale command names.
Live Datadog workflows remain credential-dependent and are verified separately.
