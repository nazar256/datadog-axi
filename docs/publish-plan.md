# Repository publishability plan

This planning note predates the current `datadog-axi` implementation; use the
README and [AXI compliance contract](axi-compliance.md) as the current source of
truth for command and mutation claims.

Goal: make the repository easier to discover, trust, install, and understand as a public open-source repository.

## Focus areas

1. Strengthen the README first screen:
   - what it is
   - who it is for
   - why it exists
   - how to install and authenticate
   - how to discover commands
   - a few real examples
2. Add compact public docs for install, AI-agent usage, GitHub metadata, and publication follow-up.
3. Add lightweight trust signals:
   - license
   - contributing guidance
   - security reporting guidance
   - CI workflow
4. Keep all claims/examples aligned with the current read-only-by-default CLI
   behavior and its narrowly scoped fingerprint-gated updates.

## Non-goals

- New Datadog feature surfaces
- Package-manager submission work
- External promotion or publishing actions
- GitHub UI changes performed from this task
