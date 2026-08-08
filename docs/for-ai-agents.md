# For AI agents

`datadog-axi` is self-describing and does not require an Agent Skill, session hook, plugin, or platform-specific integration.

The command's built-in help and home view are the primary discovery surface. The
optional repository guide at [investigation-guides.md](investigation-guides.md)
adds bounded procedures without becoming a runtime dependency or duplicating every
flag.

Start with the bounded home view and configuration check:

```bash
datadog-axi
datadog-axi doctor --json
```

Use `--json` whenever output will be parsed. Default TOON-like output is compact and deterministic; errors are structured on stdout and use exit code 2 for usage errors and 1 for operational errors.

## Narrow investigation loop

```bash
datadog-axi log search --query 'service:web status:error' --last 15m --limit 20 --json
datadog-axi metric query --query 'avg:system.cpu.user{env:prod}' --last 1h --json
datadog-axi monitor list --name api --limit 20 --json
```

Keep time windows and limits explicit. Treat empty, partial, unavailable, and permission-limited responses as distinct states. Follow each command's concrete help suggestions rather than guessing a broad command.

Use `metric metadata` before treating a metric name as monitor-ready. Use
`monitor export` or `dashboard export` before reviewing an existing resource
specification.

## Safe edits

Export or prepare a complete existing monitor/dashboard specification, validate it
with `validate --file <path>`,
then run a targeted update with the resource ID and specification file for a live
semantic dry-run. Apply only after
reviewing the returned live fingerprint and diff, with `--apply --fingerprint`.
The command re-fetches after writing and refuses stale fingerprints. Never pass
credentials on the command line. The CLI never creates or deletes resources and
does not prompt.

## More discovery

```bash
datadog-axi --help
datadog-axi docs commands --json
datadog-axi monitor list --help
datadog-axi log search --help
```

For the complete logs-to-traces, metric metadata, ownership, SLO/downtime, event,
and guarded update recipes, see [investigation-guides.md](investigation-guides.md).
