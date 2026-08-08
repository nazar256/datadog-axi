package cli

import (
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func newDocsCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "docs",
		Short:   "Read built-in CLI guidance",
		GroupID: "utility",
		Args:    cobra.NoArgs,
		Long: strings.TrimSpace(`Read concise offline documentation about authentication, output, and the CLI
command taxonomy. This is intended to be self-discoverable for both humans and AI agents.`),
		RunE: func(cmd *cobra.Command, args []string) error {
			return renderDocs(cmd.OutOrStdout(), opts, "summary")
		},
		Example: strings.TrimSpace(`datadog-axi docs summary

datadog-axi docs auth

datadog-axi docs commands --json`),
	}
	cmd.AddCommand(newDocsTopicCmd(opts, "summary", docsSummary))
	cmd.AddCommand(newDocsTopicCmd(opts, "auth", docsAuth))
	cmd.AddCommand(newDocsTopicCmd(opts, "sites", docsSites))
	cmd.AddCommand(newDocsTopicCmd(opts, "output", docsOutput))
	cmd.AddCommand(newDocsTopicCmd(opts, "commands", docsCommands))
	return cmd
}

type docTopic struct {
	Name        string       `json:"name"`
	Summary     string       `json:"summary"`
	KeyPoints   []string     `json:"key_points,omitempty"`
	Examples    []string     `json:"examples,omitempty"`
	RelatedDocs []string     `json:"related_docs,omitempty"`
	Commands    []commandDoc `json:"commands,omitempty"`
}

type commandDoc struct {
	Path        string   `json:"path"`
	ReadOnly    bool     `json:"read_only"`
	Arguments   []string `json:"required_arguments,omitempty"`
	Flags       []string `json:"important_flags,omitempty"`
	OutputModes []string `json:"output_modes,omitempty"`
	Pagination  []string `json:"pagination,omitempty"`
	Apply       string   `json:"apply,omitempty"`
	Notes       []string `json:"notes,omitempty"`
}

var docsSummary = docTopic{
	Name:    "summary",
	Summary: "datadog-axi provides an AXI-oriented Datadog investigation CLI with compact output and normalized JSON.",
	KeyPoints: []string{
		"Use 'datadog-axi <command> --help' to learn each command.",
		"Offline-safe commands work without credentials.",
		"Live Datadog commands read DD_API_KEY and DD_APP_KEY (or legacy DATADOG_* aliases) from env files or the process.",
	},
	Examples:    []string{"datadog-axi doctor", "datadog-axi docs commands --json", "datadog-axi monitor list --help"},
	RelatedDocs: []string{"auth", "commands", "output", "sites"},
}

var docsAuth = docTopic{
	Name:    "auth",
	Summary: "Authentication is env-first. Secrets are never accepted as CLI flags.",
	KeyPoints: []string{
		"Preferred secret variables: DD_API_KEY and DD_APP_KEY; legacy DATADOG_* aliases remain supported.",
		"Optional DD_SITE chooses the Datadog site; use --site to override.",
		"Default env files are layered from user config through repository root to the current directory.",
		"Process environment variables override merged env-file values.",
	},
	Examples: []string{
		"DD_API_KEY=*** DD_APP_KEY=*** datadog-axi doctor --no-env-file",
		"datadog-axi --env-file .env config doctor",
	},
	RelatedDocs: []string{"sites", "output"},
}

var docsSites = docTopic{
	Name:    "sites",
	Summary: "Use DD_SITE or --site to choose the Datadog site/region.",
	KeyPoints: []string{
		"Common aliases: us1, us3, us5, eu, ap1, ap2, us1-fed.",
		"Aliases are normalized to full hostnames such as datadoghq.com or datadoghq.eu.",
		"Only supported Datadog site hostnames are accepted directly.",
	},
	Examples:    []string{"datadog-axi --site eu doctor", "DD_SITE=us3 datadog-axi config doctor"},
	RelatedDocs: []string{"auth"},
}

var docsOutput = docTopic{
	Name:    "output",
	Summary: "Default output is deterministic TOON-like text. Use --json for stable machine-readable data.",
	KeyPoints: []string{
		"TOON-like output is optimized for agent token budgets; --output text remains available for humans.",
		"JSON output preserves the CLI's normalized response model and is optimized for agents and scripts.",
		"Use --fields with TOON or JSON for top-level projection and --full to expand previews where supported.",
		"Metric JSON returns per-series summaries plus complete point arrays for inspection.",
		"Log search JSON returns top-level items and count for stable parsing.",
		"Canonical binary errors are structured on stdout with exit code 1 or 2.",
	},
	Examples:    []string{"datadog-axi doctor --json", "datadog-axi metric query --query 'avg:system.load.1{*}' --last 1h --json", "datadog-axi log search --query 'service:web status:error' --last 15m --json"},
	RelatedDocs: []string{"commands"},
}

var docsCommands = docTopic{
	Name:    "commands",
	Summary: "The CLI uses top-level Datadog domains and predictable verbs so it scales cleanly.",
	KeyPoints: []string{
		"Offline utility commands: --version, version, docs, doctor, config doctor, completion.",
		"Available live domains: monitor, dashboard, host, metric, log, event, slo, downtime, span, audit, and service; metric search/active listing, metadata, and full monitor/dashboard export are also available.",
		"Monitor and dashboard updates are existing-resource only, dry-run by default, and fingerprint-gated for apply.",
		"Typical verbs are list, get, query, and search.",
		"Prefer 'datadog-axi doctor' for quick config checks; 'datadog-axi config doctor' remains available for explicit discovery.",
	},
	Examples: []string{
		"datadog-axi completion --help",
		"datadog-axi monitor list --help",
		"datadog-axi dashboard get abc-def-ghi --help",
		"datadog-axi metric query --help",
	},
	RelatedDocs: []string{"summary", "output"},
	Commands: []commandDoc{
		{Path: "monitor list|search|get|export|validate", ReadOnly: true, Flags: []string{"--name", "--tags", "--query", "--page", "--per-page", "--file", "--remote"}, OutputModes: []string{"toon", "json", "text"}, Pagination: []string{"offset", "limit", "page", "per-page"}, Notes: []string{"search uses the dedicated monitor-search endpoint; validate supports local and optional remote checks"}},
		{Path: "monitor validate", ReadOnly: true, Arguments: []string{"[file]"}, Flags: []string{"--file", "--remote", "--existing-id"}, OutputModes: []string{"toon", "json", "text"}, Notes: []string{"local shape validation by default; remote existing-resource validation is explicit"}},
		{Path: "monitor update", ReadOnly: false, Arguments: []string{"<id> [file]"}, Flags: []string{"--file", "--dry-run", "--apply", "--fingerprint", "--allow-stale"}, OutputModes: []string{"toon", "json", "text"}, Apply: "--apply is explicit and requires a live fingerprint", Notes: []string{"dry-run by default; updates existing monitors only"}},
		{Path: "dashboard list|get|export|validate", ReadOnly: true, Flags: []string{"--filter", "--count", "--start", "--file"}, OutputModes: []string{"toon", "json", "text"}, Pagination: []string{"count", "start"}, Notes: []string{"--filter is applied only to the returned page"}},
		{Path: "dashboard validate", ReadOnly: true, Arguments: []string{"[file]"}, Flags: []string{"--file"}, OutputModes: []string{"toon", "json", "text"}, Notes: []string{"local shape validation only"}},
		{Path: "dashboard update", ReadOnly: false, Arguments: []string{"<id> [file]"}, Flags: []string{"--file", "--dry-run", "--apply", "--fingerprint", "--allow-stale"}, OutputModes: []string{"toon", "json", "text"}, Apply: "--apply is explicit and requires a live fingerprint", Notes: []string{"dry-run by default; updates existing dashboards only"}},
		{Path: "host list|get", ReadOnly: true, Flags: []string{"--filter", "--limit", "--offset"}, OutputModes: []string{"toon", "json", "text"}, Pagination: []string{"limit", "offset"}},
		{Path: "metric query|search|active|metadata", ReadOnly: true, Flags: []string{"--query", "--last", "--host", "--tag-filter", "--limit"}, OutputModes: []string{"toon", "json", "text"}, Pagination: []string{"limit"}, Notes: []string{"query defaults to the last 1h when no range is supplied; search uses the recent metric index and active lists names reported since --last; discovery totals are the full response count before the local --limit bound"}},
		{Path: "log search", ReadOnly: true, Flags: []string{"--query", "--last", "--limit", "--cursor", "--all", "--max-pages"}, OutputModes: []string{"toon", "json", "text"}, Pagination: []string{"limit", "cursor"}, Notes: []string{"POST search; --all is bounded by --max-pages"}},
		{Path: "log aggregate", ReadOnly: true, Flags: []string{"--query", "--facet", "--compute", "--all", "--max-pages"}, OutputModes: []string{"toon", "json", "text"}, Pagination: []string{"cursor", "max-pages"}, Notes: []string{"explicit facet buckets and computations; --all is bounded"}},
		{Path: "event list|get", ReadOnly: true, Flags: []string{"--query", "--sources", "--tags", "--page", "--limit"}, OutputModes: []string{"toon", "json", "text"}, Pagination: []string{"page", "limit"}},
		{Path: "downtime list|get", ReadOnly: true, Flags: []string{"--current-only", "--include", "--offset", "--limit"}, OutputModes: []string{"toon", "json", "text"}, Pagination: []string{"offset", "limit"}},
		{Path: "slo list|search|get", ReadOnly: true, Flags: []string{"--query", "--limit", "--last", "--from", "--to"}, OutputModes: []string{"toon", "json", "text"}, Pagination: []string{"offset", "limit", "page"}, Notes: []string{"search includes status/error-budget context; get can request bounded history"}},
		{Path: "span list", ReadOnly: true, Flags: []string{"--query", "--service", "--env", "--operation", "--resource", "--last", "--limit", "--cursor"}, OutputModes: []string{"toon", "json", "text"}, Pagination: []string{"limit", "cursor"}},
		{Path: "span aggregate|services|resources|operations", ReadOnly: true, Flags: []string{"--query", "--group-by", "--compute", "--limit"}, OutputModes: []string{"toon", "json", "text"}, Pagination: []string{"limit"}, Notes: []string{"server-side bounded analytics buckets; discovery commands return one bounded aggregate page"}},
		{Path: "audit list", ReadOnly: true, Flags: []string{"--query", "--actor", "--service", "--action", "--resource", "--tag", "--limit", "--cursor"}, OutputModes: []string{"toon", "json", "text"}, Pagination: []string{"limit", "cursor"}},
		{Path: "service list|get", ReadOnly: true, Flags: []string{"--filter", "--limit", "--offset"}, OutputModes: []string{"toon", "json", "text"}, Pagination: []string{"limit", "offset"}},
	},
}

func newDocsTopicCmd(opts *GlobalOptions, name string, topic docTopic) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: topic.Summary,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "commands" {
				topic.Commands = generatedCommandDocs(cmd.Root())
			}
			return renderTopic(cmd.OutOrStdout(), opts, topic)
		},
	}
}

func generatedCommandDocs(root *cobra.Command) []commandDoc {
	var docs []commandDoc
	var visit func(*cobra.Command)
	visit = func(parent *cobra.Command) {
		for _, child := range parent.Commands() {
			if child.Hidden {
				continue
			}
			if len(child.Commands()) == 0 && child.Name() != "help" {
				path := strings.TrimSpace(strings.TrimPrefix(child.CommandPath(), root.CommandPath()))
				path = strings.TrimSpace(path)
				useParts := strings.Fields(child.Use)
				arguments := []string(nil)
				if len(useParts) > 1 {
					arguments = []string{strings.Join(useParts[1:], " ")}
				}
				flags := make([]string, 0)
				child.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) { flags = append(flags, "--"+flag.Name) })
				sort.Strings(flags)
				docs = append(docs, commandDoc{Path: path, ReadOnly: !strings.Contains(path, " update"), Arguments: arguments, Flags: flags, OutputModes: []string{"toon", "json", "text"}, Notes: []string{"generated from the registered Cobra command"}})
			}
			visit(child)
		}
	}
	visit(root)
	sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	return docs
}

func renderDocs(w io.Writer, opts *GlobalOptions, name string) error {
	topics := []docTopic{docsSummary, docsAuth, docsSites, docsOutput, docsCommands}
	return renderTopic(w, opts, docsSummaryWithIndex(topics, name))
}

func docsSummaryWithIndex(topics []docTopic, name string) docTopic {
	if name != "summary" {
		for _, topic := range topics {
			if topic.Name == name {
				return topic
			}
		}
	}
	summary := docsSummary
	summary.KeyPoints = append([]string{}, summary.KeyPoints...)
	summary.KeyPoints = append(summary.KeyPoints, "Available topics: summary, auth, sites, output, commands.")
	return summary
}

func renderTopic(w io.Writer, opts *GlobalOptions, topic docTopic) error {
	cfg, err := runtimeConfigForOffline(opts)
	if err != nil {
		return err
	}
	return writeValue(w, opts, cfg, topic, func(w io.Writer) error {
		if _, err := io.WriteString(w, topic.Name+"\n"); err != nil {
			return err
		}
		if _, err := io.WriteString(w, strings.Repeat("-", len(topic.Name))+"\n"); err != nil {
			return err
		}
		if _, err := io.WriteString(w, topic.Summary+"\n"); err != nil {
			return err
		}
		if len(topic.KeyPoints) > 0 {
			if _, err := io.WriteString(w, "\nKey points:\n"); err != nil {
				return err
			}
			for _, item := range topic.KeyPoints {
				if _, err := io.WriteString(w, "- "+item+"\n"); err != nil {
					return err
				}
			}
		}
		if len(topic.Examples) > 0 {
			if _, err := io.WriteString(w, "\nExamples:\n"); err != nil {
				return err
			}
			for _, item := range topic.Examples {
				if _, err := io.WriteString(w, "- "+item+"\n"); err != nil {
					return err
				}
			}
		}
		if len(topic.RelatedDocs) > 0 {
			_, err := io.WriteString(w, "\nRelated docs: "+strings.Join(topic.RelatedDocs, ", ")+"\n")
			return err
		}
		return nil
	})
}
