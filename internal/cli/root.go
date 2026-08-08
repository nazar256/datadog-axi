package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nazar256/datadog-axi/internal/output"
	"github.com/nazar256/datadog-axi/internal/runtime"
	"github.com/spf13/cobra"
)

type GlobalOptions struct {
	runtime.FlagValues
	BuildInfo BuildInfo
	Services  serviceSet
}

func NewRootCmd(buildInfo BuildInfo) *cobra.Command {
	if buildInfo.Product == "" {
		buildInfo.Product = "datadog-axi"
	}
	return newRootCmdWithOptions(&GlobalOptions{
		BuildInfo: buildInfo,
	})
}

func newRootCmdWithOptions(opts *GlobalOptions) *cobra.Command {
	opts.ensureServices()
	if opts.BuildInfo.Version == "" {
		opts.BuildInfo.Version = "dev"
	}
	if opts.BuildInfo.Product == "" {
		opts.BuildInfo.Product = "datadog-axi"
	}
	if opts.BuildInfo.Product == "datadog-axi" && opts.DefaultOutput == "" {
		opts.DefaultOutput = "toon"
	}

	var showVersion bool
	cmd := &cobra.Command{
		Use:   opts.BuildInfo.Product,
		Short: "Investigate Datadog observability domains with guarded monitor/dashboard updates",
		Long: strings.TrimSpace(fmt.Sprintf(`%s is a Datadog CLI for humans, coding agents, and automation.

Use '%s <command> --help' to explore the command tree. Offline-safe commands such as
'version', 'docs', 'doctor', 'config doctor', and 'completion' work without Datadog credentials. Live Datadog
	commands use DD_API_KEY and DD_APP_KEY (legacy DATADOG_* aliases remain supported) from the environment or layered env files.`, opts.BuildInfo.Product, opts.BuildInfo.Product)),
		Example: strings.TrimSpace(fmt.Sprintf(`%s

%s doctor

%s docs commands --json
`, opts.BuildInfo.Product, opts.BuildInfo.Product, opts.BuildInfo.Product)),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if showVersion {
				return newVersionCmd(opts).RunE(cmd, nil)
			}
			if len(args) == 0 {
				return writeHome(cmd, opts)
			}
			return nil
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			formatName := strings.TrimSpace(opts.Output)
			if formatName == "" {
				formatName = strings.TrimSpace(opts.DefaultOutput)
			}
			if formatName == "" {
				formatName = string(output.Text)
			}
			if strings.TrimSpace(opts.Fields) != "" && !opts.JSON && strings.EqualFold(formatName, string(output.Text)) {
				return usagef("--fields requires --output toon or --output json")
			}
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&opts.Site, "site", "", "Datadog site (e.g., datadoghq.com, us3, eu)")
	cmd.PersistentFlags().StringVar(&opts.EnvFile, "env-file", "", "Load exactly this env file (overrides layered discovery)")
	cmd.PersistentFlags().BoolVar(&opts.NoEnvFile, "no-env-file", false, "Disable user, repository, and cwd env-file discovery")
	cmd.PersistentFlags().DurationVar(&opts.Timeout, "timeout", 0, "API timeout (default 30s)")
	cmd.PersistentFlags().StringVarP(&opts.Output, "output", "o", "", "Output format (toon, json, text)")
	cmd.PersistentFlags().BoolVar(&opts.JSON, "json", false, "Stable JSON output (same as --output json)")
	cmd.PersistentFlags().BoolVar(&opts.Full, "full", false, "Expand previews and retain complete structured result content")
	cmd.PersistentFlags().StringVar(&opts.Fields, "fields", "", "Comma-separated top-level fields for structured output")
	cmd.PersistentFlags().BoolVar(&showVersion, "version", false, "Print version and exit")

	// Command groups
	cmd.AddGroup(&cobra.Group{
		ID:    "core",
		Title: "Core Commands:",
	})
	cmd.AddGroup(&cobra.Group{
		ID:    "utility",
		Title: "Utility Commands:",
	})

	cmd.AddCommand(newVersionCmd(opts))
	cmd.AddCommand(newDocsCmd(opts))
	cmd.AddCommand(newDoctorCmd(opts))
	cmd.AddCommand(newConfigCmd(opts))
	addCoreCommands(cmd, opts)
	cmd.SetCompletionCommandGroupID("utility")
	cmd.InitDefaultCompletionCmd()
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return usageError(fmt.Errorf("%w\n\nSee '%s --help' for usage.", err, cmd.CommandPath()))
	})

	return cmd
}

func writeOutput(cmd *cobra.Command, opts *GlobalOptions, cfg runtime.Config, value any, textRenderer func(io.Writer) error) error {
	return writeValuePath(cmd.OutOrStdout(), opts, cfg, value, textRenderer, cmd.CommandPath())
}

func writeValue(w io.Writer, opts *GlobalOptions, cfg runtime.Config, value any, textRenderer func(io.Writer) error) error {
	return writeValuePath(w, opts, cfg, value, textRenderer, "")
}

func writeValuePath(w io.Writer, opts *GlobalOptions, cfg runtime.Config, value any, textRenderer func(io.Writer) error, path string) error {
	if cfg.Output != output.Text {
		value = redactStructuredOutput(value, cfg)
	}
	suggestions := nextSuggestions(opts.BuildInfo.Product, path, value)
	if len(suggestions) > 0 {
		value = addSuggestions(value, suggestions)
		originalRenderer := textRenderer
		textRenderer = func(out io.Writer) error {
			if err := originalRenderer(out); err != nil {
				return err
			}
			_, err := fmt.Fprintf(out, "\nNext commands: %s\n", strings.Join(suggestions, "; "))
			return err
		}
	}
	if strings.TrimSpace(opts.Fields) != "" {
		if cfg.Output == output.Text {
			return usagef("--fields requires --output toon or --output json")
		}
		projected, err := output.Project(value, opts.Fields)
		if err != nil {
			return usageError(err)
		}
		value = projected
	}
	return output.Write(w, cfg.Output, value, textRenderer)
}

func redactStructuredOutput(value any, cfg runtime.Config) any {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return value
	}
	lower := strings.ToLower(string(data))
	needsRedaction := false
	for _, marker := range []string{"api_key", "app_key", "apikey", "application_key", "authorization", "access_key", "private_key", "password", "secret", "token=", "bearer "} {
		if strings.Contains(lower, marker) {
			needsRedaction = true
			break
		}
	}
	if !needsRedaction {
		return value
	}
	return redactOutputValue(decoded, cfg)
}

func redactOutputValue(value any, cfg runtime.Config) any {
	switch item := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(item))
		for key, nested := range item {
			if redactedKey(key) {
				if _, presenceValue := nested.(bool); presenceValue {
					result[key] = nested
					continue
				}
				if isPresenceMetadata(key, nested) {
					result[key] = nested
					continue
				}
				result[key] = "[REDACTED]"
				continue
			}
			result[key] = redactOutputValue(nested, cfg)
		}
		return result
	case []any:
		result := make([]any, len(item))
		for i, nested := range item {
			result[i] = redactOutputValue(nested, cfg)
		}
		return result
	case string:
		if containsSensitiveString(item) {
			return runtime.SanitizeError(item, cfg)
		}
		return item
	default:
		return value
	}
}

func isPresenceMetadata(key string, value any) bool {
	if key != "api_key" && key != "app_key" {
		return false
	}
	text, ok := value.(string)
	if !ok {
		return false
	}
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "present" || text == "missing" || text == "unset" || text == "none" {
		return true
	}
	for _, prefix := range []string{"process:", "file:", "user:", "repository:", "cwd:"} {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

func addSuggestions(value any, suggestions []string) any {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil || object == nil {
		return value
	}
	object["next"] = suggestions
	return object
}

func nextSuggestions(product, path string, value any) []string {
	if path == "" {
		return nil
	}
	path = strings.TrimSpace(strings.TrimPrefix(path, product+" "))
	if strings.Contains(path, " get") || strings.HasSuffix(path, " metadata") {
		return nil
	}
	if product == "" {
		product = "datadog-axi"
	}
	var suggestions []string
	switch {
	case strings.HasPrefix(path, "monitor list"):
		suggestions = []string{"datadog-axi monitor get <monitor-id> --json", "datadog-axi monitor list --name <text> --json"}
	case strings.HasPrefix(path, "monitor search"):
		suggestions = []string{"datadog-axi monitor search --query <text> --json", "datadog-axi monitor get <monitor-id> --json"}
	case strings.HasPrefix(path, "monitor update"):
		suggestions = []string{"datadog-axi monitor get <monitor-id> --json", "datadog-axi monitor export <monitor-id> --file .tmp/monitor.json"}
	case strings.HasPrefix(path, "dashboard list"):
		suggestions = []string{"datadog-axi dashboard get <dashboard-id> --json", "datadog-axi dashboard list --count 20 --json"}
	case strings.HasPrefix(path, "dashboard update"):
		suggestions = []string{"datadog-axi dashboard get <dashboard-id> --json", "datadog-axi dashboard export <dashboard-id> --file .tmp/dashboard.json"}
	case strings.HasPrefix(path, "host list"):
		suggestions = []string{"datadog-axi host get <hostname> --json", "datadog-axi host list --filter <text> --json"}
	case strings.HasPrefix(path, "metric query"):
		suggestions = []string{"datadog-axi metric metadata <metric-name> --json", "datadog-axi metric query --query <query> --last 1h --json"}
	case strings.HasPrefix(path, "metric search"):
		suggestions = []string{"datadog-axi metric metadata <metric-name> --json", "datadog-axi metric active --last 1h --limit 50 --json"}
	case strings.HasPrefix(path, "metric active"):
		suggestions = []string{"datadog-axi metric metadata <metric-name> --json", "datadog-axi metric query --query <query> --last 1h --json"}
	case strings.HasPrefix(path, "log search"):
		suggestions = []string{"datadog-axi log search --query <query> --last 15m --json"}
		if hasNextCursor(value) {
			suggestions = append(suggestions, cursorSuggestion(product, "log search", value))
		}
	case strings.HasPrefix(path, "log aggregate"):
		suggestions = []string{"datadog-axi log aggregate --query <query> --facet status --compute count --json", "datadog-axi log search --query <query> --last 15m --json"}
	case strings.HasPrefix(path, "event list"):
		suggestions = []string{"datadog-axi event get <event-id> --json", "datadog-axi event list --last 1h --json"}
	case strings.HasPrefix(path, "downtime list"):
		suggestions = []string{"datadog-axi downtime get <downtime-id> --json", "datadog-axi downtime list --current-only --json"}
	case strings.HasPrefix(path, "slo list"):
		suggestions = []string{"datadog-axi slo list --query <text> --json", "datadog-axi slo list --offset <n> --json"}
	case strings.HasPrefix(path, "slo search"):
		suggestions = []string{"datadog-axi slo search --query <text> --json", "datadog-axi slo get <slo-id> --json"}
	case strings.HasPrefix(path, "span list"):
		suggestions = []string{"datadog-axi span list --last 1h --json"}
		if hasNextCursor(value) {
			suggestions = append([]string{cursorSuggestion(product, "span list", value)}, suggestions...)
		}
	case strings.HasPrefix(path, "span aggregate"):
		suggestions = []string{"datadog-axi span aggregate --query <query> --group-by service --compute count --json", "datadog-axi span list --query <query> --last 15m --json"}
	case strings.HasPrefix(path, "span services"):
		suggestions = []string{"datadog-axi span services --last 15m --json", "datadog-axi span aggregate --group-by service --compute count --json"}
	case strings.HasPrefix(path, "span resources"):
		suggestions = []string{"datadog-axi span resources --last 15m --json", "datadog-axi span aggregate --group-by resource --compute count --json"}
	case strings.HasPrefix(path, "span operations"):
		suggestions = []string{"datadog-axi span operations --last 15m --json", "datadog-axi span aggregate --group-by operation --compute count --json"}
	case strings.HasPrefix(path, "audit list"):
		suggestions = []string{"datadog-axi audit list --last 1h --json"}
		if hasNextCursor(value) {
			suggestions = append([]string{cursorSuggestion(product, "audit list", value)}, suggestions...)
		}
	case strings.HasPrefix(path, "service list"):
		suggestions = []string{"datadog-axi service get <service-name> --json", "datadog-axi service list --offset <n> --json"}
	}
	if product != "datadog-axi" {
		for i := range suggestions {
			suggestions[i] = strings.ReplaceAll(suggestions[i], "datadog-axi", product)
		}
	}
	return suggestions
}

func cursorSuggestion(product, command string, value any) string {
	data, _ := json.Marshal(value)
	var object map[string]any
	_ = json.Unmarshal(data, &object)
	query, _ := object["query"].(string)
	from, _ := object["from"].(string)
	to, _ := object["to"].(string)
	parts := []string{product, command, "--cursor", "<next_cursor>"}
	if query != "" {
		parts = append(parts, "--query", quoteSuggestion(query))
	}
	if from != "" {
		parts = append(parts, "--from", quoteSuggestion(from))
	}
	if to != "" {
		parts = append(parts, "--to", quoteSuggestion(to))
	}
	parts = append(parts, "--json")
	return strings.Join(parts, " ")
}

func quoteSuggestion(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func hasNextCursor(value any) bool {
	data, err := json.Marshal(value)
	if err != nil {
		return false
	}
	var object map[string]any
	if json.Unmarshal(data, &object) != nil {
		return false
	}
	for _, key := range []string{"next_cursor", "nextCursor", "cursor"} {
		if cursor, ok := object[key].(string); ok && strings.TrimSpace(cursor) != "" {
			return true
		}
	}
	return false
}

func writeHome(cmd *cobra.Command, opts *GlobalOptions) error {
	cfg, err := runtime.ResolveConfig(opts.FlagValues)
	if err != nil {
		return err
	}
	exe, _ := os.Executable()
	if home, e := os.UserHomeDir(); e == nil {
		if rel, e := filepath.Rel(home, exe); e == nil && !strings.HasPrefix(rel, "..") {
			exe = "~/" + rel
		}
	}
	available := []string{"monitors", "dashboards", "hosts", "metrics", "metric search/active/metadata", "logs", "events", "slos", "downtimes", "APM spans", "audit logs", "service catalog", "monitor/dashboard export and guarded update"}
	deferred := []string{"native AXI/TOON conformance and broader write operations"}
	product := opts.BuildInfo.Product
	help := []string{product + " doctor", product + " monitor list --limit 20", product + " docs commands"}
	description := "Datadog observability CLI for agents and humans with guarded existing-resource updates"
	view := map[string]any{"bin": exe, "product": product, "description": description, "site": cfg.Site, "auth": map[string]any{"api_key": cfg.APIKey != "", "app_key": cfg.AppKey != ""}, "available": available, "deferred": deferred, "next": help}
	return writeOutput(cmd, opts, cfg, view, func(w io.Writer) error {
		return output.KeyValue(w, [][2]string{{"Binary", product}, {"Executable", exe}, {"Description", description}, {"Site", cfg.Site}, {"API Key", presence(cfg.APIKey)}, {"App Key", presence(cfg.AppKey)}, {"Available", strings.Join(available, ", ")}, {"Deferred", strings.Join(deferred, ", ")}, {"Next", strings.Join(help, "; ")}})
	})
}
