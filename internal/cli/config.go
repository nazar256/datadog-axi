package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/nazar256/datadog-cli/internal/output"
	"github.com/nazar256/datadog-cli/internal/runtime"
	"github.com/spf13/cobra"
)

func newConfigCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config",
		Short:   "Inspect CLI configuration",
		Long:    "Inspect non-secret CLI configuration. Use 'ddog doctor' or 'ddog config doctor' to verify auth and resolved settings.",
		GroupID: "utility",
	}

	cmd.AddCommand(newConfigDoctorCmd(opts))

	return cmd
}

func newDoctorCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "doctor",
		Short:   "Check configuration and authentication status",
		Long:    "Resolve configuration exactly as the CLI sees it and report non-secret settings plus whether authentication values are present. This is the top-level alias for 'ddog config doctor'.",
		Args:    cobra.NoArgs,
		Example: "ddog doctor\n  ddog doctor --output json\n  ddog --site eu doctor",
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

Prefer 'ddog doctor' when you want the shortest path. 'ddog config doctor' remains available for explicit command-tree discovery.`),
		Example: "ddog config doctor\n  ddog doctor --output json\n  ddog --env-file .env config doctor",
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
			Site:       cfg.Site,
			Timeout:    cfg.Timeout.String(),
			Output:     string(cfg.Output),
			EnvFile:    emptyFallback(cfg.EnvFileUsed, "(none)"),
			APIKey:     presence(cfg.APIKey),
			AppKey:     presence(cfg.AppKey),
			AuthStatus: doctorStatus(cfg),
		}

		return output.Write(cmd.OutOrStdout(), cfg.Output, doctor, func(w io.Writer) error {
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
			_, err = fmt.Fprintln(w, "\nSecrets are never printed. Use DATADOG_API_KEY and DATADOG_APP_KEY via env or .env.")
			return err
		})
	}
}

type configDoctorView struct {
	Site       string `json:"site"`
	Timeout    string `json:"timeout"`
	Output     string `json:"output"`
	EnvFile    string `json:"env_file"`
	APIKey     string `json:"api_key"`
	AppKey     string `json:"app_key"`
	AuthStatus string `json:"auth_status"`
}

func runtimeConfigForOffline(opts *GlobalOptions) (runtime.Config, error) {
	format, err := output.ParseFormat(opts.Output)
	if err != nil {
		return runtime.Config{}, err
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
