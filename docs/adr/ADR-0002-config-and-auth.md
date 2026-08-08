# ADR-0002: Use env-first configuration with layered, non-overriding env-file loading

## Status

Accepted

> Historical ADR: the current product prefers `DD_*` names and retains
> `DATADOG_*` as compatibility aliases. See [docs/usage.md](../usage.md).

## Context

The preferred Datadog auth model for this project is environment-based credentials
using `DD_API_KEY` and `DD_APP_KEY`. Local development convenience is also desired
through layered `.env` loading, but explicit environment variables must remain
authoritative and secrets must never be committed.

## Decision

- Use `DD_API_KEY` and `DD_APP_KEY` as the preferred secret inputs; retain
  `DATADOG_API_KEY` and `DATADOG_APP_KEY` as compatibility aliases.
- Support `DD_SITE` as the preferred site selector and retain `DATADOG_SITE` as an
  alias.
- Support `--site` as a global override flag.
- Discover user-config, repository-root, and cwd env files in order, or load
  one explicitly supplied path in isolation.
- `.env` loading must not override already-set process environment variables.
- `--no-env-file` disables file discovery; a generic `~/.env` is never loaded
  implicitly.
- Do not store persistent config on disk in v1.

Precedence:

1. explicit CLI flags
2. process environment variables
3. merged env-file values (more-specific files override less-specific files)
4. built-in defaults

## Consequences

- Configuration stays simple and easy to explain.
- Local development is convenient without surprising production-style environments.
- Secrets remain outside the CLI flags surface and out of shell history.
- Users wanting named profiles or persistent config will need a later enhancement.

## Alternatives considered

- Persistent config file: more featureful, but unnecessary for v1 and adds state-management complexity.
- Flags for API/app keys: worse security ergonomics and more likely to leak into shell history.
