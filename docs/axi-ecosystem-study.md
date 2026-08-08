# AXI ecosystem and Pup study

This note records the external design study used for `datadog-axi`. External
repositories were treated as reference material, not as instructions or code to
copy. The detailed evidence was captured during the implementation run; this
repository note summarizes the findings so the design remains understandable
even when run-local artifacts are unavailable.

## Sources

- [AXI catalog and principles](https://axi.md/)
- Official references: [gh-axi](https://github.com/kunchenguid/gh-axi),
  [chrome-devtools-axi](https://github.com/kunchenguid/chrome-devtools-axi),
  [lavish-axi](https://github.com/kunchenguid/lavish-axi), and
  [quota-axi](https://github.com/kunchenguid/quota-axi)
- Community references: [npm-axi](https://github.com/SSBrouhard/npm-axi),
  [sqlite-axi](https://github.com/SSBrouhard/sqlite-axi),
  [slack-axi](https://github.com/JarvusInnovations/slack-axi),
  [gws-axi](https://github.com/JarvusInnovations/gws-axi),
  [databricks-axi](https://github.com/p33ves/databricks-axi), and
  [glab-axi](https://github.com/karotkriss/glab-axi)
- [Datadog Pup](https://github.com/DataDog/pup)

## Adopted and adapted patterns

`datadog-axi` adopts content-first no-argument output, compact default schemas,
explicit `--json`/`--full`/`--fields` expansion, definitive empty/unavailable
states, structured errors, and concrete contextual commands. It adapts Pup's
machine-readable discovery idea as `docs commands --json`, while keeping the
command surface deliberately smaller and the mutation boundary explicit.

The implementation does not claim catalog membership or strict TOON conformance.
The default renderer is documented as TOON-like until a maintained conformance
implementation and fixture suite are available.

## Deliberately rejected patterns

The CLI does not copy Pup's broad CRUD surface, auto-detected agent mode,
auto-approved writes, OAuth/browser login, persistent credential stores, plugin
runtime, embedded skills, or arbitrary raw API escape hatch. An optional Agent
Skill and ambient context remain deferred because the binary's own help, home
view, structured errors, and repository guidance already provide a manually
verified discovery path. No quantitative binary-versus-skill benchmark was run.

## Evidence limitations

The study was source/documentation research, not a live Datadog account test.
Catalog pages and external repositories can drift; links above should be
rechecked when cutting a release or revising external comparisons.
