# Contributing

Thanks for your interest in improving `ddog`.

## What this project optimizes for

- small, reviewable changes
- honest docs and examples
- read-oriented Datadog workflows that are useful for terminals, scripts, and AI agents
- stable JSON output for automation

## Local workflow

```bash
go test ./...
go build ./cmd/ddog
go run ./cmd/ddog --help
```

If you are changing docs or examples, make sure they match real CLI behavior.

## Configuration and secrets

- Use `DATADOG_API_KEY` and `DATADOG_APP_KEY` from your environment or a local `.env` file.
- Never commit real Datadog credentials.
- Prefer `ddog doctor` when checking auth-related behavior; `ddog config doctor` remains available when you want the explicit subcommand path.

## Pull requests

- Keep scope intentional.
- Include tests when changing behavior.
- Mention any docs updates that were needed to keep the repo accurate.

## Issues

If you report a bug, include:

- the command you ran
- whether you used `--output json`
- the Datadog site you targeted
- the relevant error text

Please avoid posting secrets or sensitive Datadog data.
