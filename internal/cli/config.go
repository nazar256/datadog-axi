package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/nazar256/datadog-axi/internal/output"
	"github.com/nazar256/datadog-axi/internal/runtime"
	"github.com/spf13/cobra"
)

func newConfigCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config",
		Short:   "Inspect CLI configuration",
		Long:    "Inspect non-secret CLI configuration. Use 'datadog-axi doctor' or 'datadog-axi config doctor' to verify auth and resolved settings.",
		GroupID: "utility",
	}

	cmd.AddCommand(newConfigDoctorCmd(opts))

	return cmd
}

func newDoctorCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "doctor",
		Short:   "Check configuration and authentication status",
		Long:    "Resolve configuration exactly as the CLI sees it and report non-secret settings plus whether authentication values are present. This is the top-level alias for 'datadog-axi config doctor'.",
		Args:    cobra.NoArgs,
		Example: "datadog-axi doctor\n  datadog-axi doctor --json\n  datadog-axi --site eu doctor",
		GroupID: "utility",
		RunE:    runDoctor(opts),
	}
	return cmd
}

func newConfigDoctorCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check configuration and authentication status",
		Args:  cobra.NoArgs,
		Long: strings.TrimSpace(`Resolve configuration exactly as the CLI sees it and report non-secret settings plus whether authentication values are present.

Prefer 'datadog-axi doctor' when you want the shortest path. 'datadog-axi config doctor' remains available for explicit command-tree discovery.`),
		Example: "datadog-axi config doctor\n  datadog-axi doctor --json\n  datadog-axi --env-file .env config doctor",
		RunE:    runDoctor(opts),
	}
	return cmd
}

func runDoctor(opts *GlobalOptions) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cfg, err := runtime.ResolveConfig(opts.FlagValues)
		if err != nil {
			return fmt.Errorf("failed to resolve config: %w", err)
		}

		doctor := configDoctorView{
			Site:        cfg.Site,
			Timeout:     cfg.Timeout.String(),
			Output:      string(cfg.Output),
			EnvFile:     emptyFallback(cfg.EnvFileUsed, "(none)"),
			EnvFiles:    cfg.EnvFiles,
			Diagnostics: cfg.Diagnostics,
			Sources:     cfg.Sources,
			APIKey:      presence(cfg.APIKey),
			AppKey:      presence(cfg.AppKey),
			AuthStatus:  doctorStatus(cfg),
		}

		return writeOutput(cmd, opts, cfg, doctor, func(w io.Writer) error {
			_, err := fmt.Fprintln(w, "Configuration Doctor")
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(w, "--------------------")
			if err != nil {
				return err
			}
			if err := output.KeyValue(w, [][2]string{
				{"Site", doctor.Site},
				{"Timeout", doctor.Timeout},
				{"Output", doctor.Output},
				{"Env File", doctor.EnvFile},
				{"API Key", doctor.APIKey},
				{"App Key", doctor.AppKey},
				{"Status", doctor.AuthStatus},
			}); err != nil {
				return err
			}
			if len(doctor.EnvFiles) > 0 {
				if _, err := fmt.Fprintln(w, "Env Files\t"+strings.Join(doctor.EnvFiles, "; ")); err != nil {
					return err
				}
			}
			if len(doctor.Diagnostics) > 0 {
				if _, err := fmt.Fprintln(w, "Diagnostics\t"+strings.Join(doctor.Diagnostics, "; ")); err != nil {
					return err
				}
			}
			if len(doctor.Sources) > 0 {
				if _, err := fmt.Fprintln(w, "Sources\t"+formatSources(doctor.Sources)); err != nil {
					return err
				}
			}
			_, err = fmt.Fprintln(w, "\nSecrets are never printed. Use DD_API_KEY and DD_APP_KEY (legacy DATADOG_* aliases are supported).")
			return err
		})
	}
}

type configDoctorView struct {
	Site        string            `json:"site"`
	Timeout     string            `json:"timeout"`
	Output      string            `json:"output"`
	EnvFile     string            `json:"env_file"`
	EnvFiles    []string          `json:"env_files,omitempty"`
	Diagnostics []string          `json:"diagnostics,omitempty"`
	Sources     map[string]string `json:"sources,omitempty"`
	APIKey      string            `json:"api_key"`
	AppKey      string            `json:"app_key"`
	AuthStatus  string            `json:"auth_status"`
}

func formatSources(sources map[string]string) string {
	keys := []string{"site", "api_key", "app_key"}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if source := sources[key]; source != "" {
			parts = append(parts, key+"="+source)
		}
	}
	return strings.Join(parts, "; ")
}

func runtimeConfigForOffline(opts *GlobalOptions) (runtime.Config, error) {
	if opts.NoEnvFile && strings.TrimSpace(opts.EnvFile) != "" {
		return runtime.Config{}, runtime.UsageErrorf("--env-file and --no-env-file cannot be used together")
	}
	if opts.Timeout < 0 {
		return runtime.Config{}, runtime.UsageErrorf("timeout must be greater than or equal to 0")
	}
	formatName := opts.Output
	if opts.JSON {
		if strings.TrimSpace(formatName) != "" && !strings.EqualFold(formatName, string(output.JSON)) {
			return runtime.Config{}, runtime.UsageErrorf("--json cannot be combined with --output %s", formatName)
		}
		formatName = string(output.JSON)
	}
	if formatName == "" {
		formatName = opts.DefaultOutput
	}
	format, err := output.ParseFormat(formatName)
	if err != nil {
		return runtime.Config{}, runtime.WrapUsageError(err)
	}
	return runtime.Config{Output: format}, nil
}

func presence(value string) string {
	if value == "" {
		return "missing"
	}
	return "present"
}

func doctorStatus(cfg runtime.Config) string {
	if cfg.HasAuth() {
		return "ready"
	}
	return "missing_credentials"
}

func emptyFallback(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
