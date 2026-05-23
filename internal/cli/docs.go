package cli

import (
	"io"
	"strings"

	"github.com/nazar256/datadog-cli/internal/output"
	"github.com/spf13/cobra"
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
		Example: strings.TrimSpace(`ddog docs summary

ddog docs auth

ddog docs commands --output json`),
	}
	cmd.AddCommand(newDocsTopicCmd(opts, "summary", docsSummary))
	cmd.AddCommand(newDocsTopicCmd(opts, "auth", docsAuth))
	cmd.AddCommand(newDocsTopicCmd(opts, "sites", docsSites))
	cmd.AddCommand(newDocsTopicCmd(opts, "output", docsOutput))
	cmd.AddCommand(newDocsTopicCmd(opts, "commands", docsCommands))
	return cmd
}

type docTopic struct {
	Name        string   `json:"name"`
	Summary     string   `json:"summary"`
	KeyPoints   []string `json:"key_points,omitempty"`
	Examples    []string `json:"examples,omitempty"`
	RelatedDocs []string `json:"related_docs,omitempty"`
}

var docsSummary = docTopic{
	Name:    "summary",
	Summary: "ddog provides a read-oriented Datadog CLI with concise terminal output and JSON for automation.",
	KeyPoints: []string{
		"Use 'ddog <command> --help' to learn each command.",
		"Offline-safe commands work without credentials.",
		"Live Datadog commands read DATADOG_API_KEY and DATADOG_APP_KEY from env or .env.",
	},
	Examples:    []string{"ddog doctor", "ddog docs commands", "ddog monitor list --help"},
	RelatedDocs: []string{"auth", "commands", "output", "sites"},
}

var docsAuth = docTopic{
	Name:    "auth",
	Summary: "Authentication is env-first. Secrets are never accepted as CLI flags.",
	KeyPoints: []string{
		"Required secret variables: DATADOG_API_KEY and DATADOG_APP_KEY.",
		"Optional DATADOG_SITE chooses the Datadog site; use --site to override.",
		"A local .env file is read only from the current working directory or an explicit --env-file path.",
		"Process environment variables override .env values.",
	},
	Examples: []string{
		"DATADOG_API_KEY=*** DATADOG_APP_KEY=*** ddog doctor --no-env-file",
		"ddog --env-file .env config doctor",
	},
	RelatedDocs: []string{"sites", "output"},
}

var docsSites = docTopic{
	Name:    "sites",
	Summary: "Use DATADOG_SITE or --site to choose the Datadog site/region.",
	KeyPoints: []string{
		"Common aliases: us1, us3, us5, eu, ap1, ap2, us1-fed.",
		"Aliases are normalized to full hostnames such as datadoghq.com or datadoghq.eu.",
		"Only supported Datadog site hostnames are accepted directly.",
	},
	Examples:    []string{"ddog --site eu doctor", "DATADOG_SITE=us3 ddog config doctor"},
	RelatedDocs: []string{"auth"},
}

var docsOutput = docTopic{
	Name:    "output",
	Summary: "Default output is concise text. Use --output json for stable machine-readable data.",
	KeyPoints: []string{
		"Text output is optimized for terminal reading.",
		"JSON output is optimized for agents and scripts.",
		"Metric JSON returns per-series summaries such as point_count, last_point_ts, and last_value instead of raw pointlists.",
		"Log search JSON returns top-level items and count for stable parsing.",
		"Errors are returned on stderr by Cobra command execution.",
	},
	Examples:    []string{"ddog doctor --output json", "ddog metric query --query 'avg:system.load.1{*}' --last 1h --output json", "ddog log search --query 'service:web status:error' --last 15m --output json"},
	RelatedDocs: []string{"commands"},
}

var docsCommands = docTopic{
	Name:    "commands",
	Summary: "The CLI uses top-level Datadog domains and predictable verbs so it scales cleanly.",
	KeyPoints: []string{
		"Offline utility commands: version, docs, doctor, config doctor, completion.",
		"Live v1 commands are organized by domain: monitor, dashboard, host, metric, log.",
		"Typical verbs are list, get, query, and search.",
		"Prefer 'ddog doctor' for quick config checks; 'ddog config doctor' remains available for explicit discovery.",
	},
	Examples: []string{
		"ddog completion --help",
		"ddog monitor list --help",
		"ddog dashboard get abc-def-ghi --help",
		"ddog metric query --help",
	},
	RelatedDocs: []string{"summary", "output"},
}

func newDocsTopicCmd(opts *GlobalOptions, name string, topic docTopic) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: topic.Summary,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return renderTopic(cmd.OutOrStdout(), opts, topic)
		},
	}
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
	return output.Write(w, cfg.Output, topic, func(w io.Writer) error {
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
