# Contributing

Thanks for your interest in improving `datadog-axi`.

## What this project optimizes for

- small, reviewable changes
- honest docs and examples
- read-oriented Datadog workflows that are useful for terminals, scripts, and AI agents
- stable JSON output for automation

## Local workflow

```bash
go test ./...
go build ./cmd/datadog-axi
go run ./cmd/datadog-axi --help
```

If you are changing docs or examples, make sure they match real CLI behavior.
Keep [docs/documentation-inventory.md](docs/documentation-inventory.md) aligned
when adding, replacing, or deferring a public documentation surface. Built-in
`datadog-axi docs` and `--help` are executable guidance; do not create a second
flag reference that can drift from the command tree.

## Configuration and secrets

- Use `DD_API_KEY` and `DD_APP_KEY` from your environment or a local `.env` file; legacy `DATADOG_*` aliases remain supported.
- Never commit real Datadog credentials.
- Prefer `datadog-axi doctor` when checking auth-related behavior; `datadog-axi config doctor` remains available when you want the explicit subcommand path.
- Use the preferred `DD_*` variable spelling in new examples. Keep
  `DATADOG_*` only when documenting the supported compatibility aliases.

## Pull requests

- Keep scope intentional.
- Include tests when changing behavior.
- Mention any docs updates that were needed to keep the repo accurate.

## Issues

If you report a bug, include:

- the command you ran
- whether you used `--json`
- the Datadog site you targeted
- the relevant error text

Please avoid posting secrets or sensitive Datadog data.
