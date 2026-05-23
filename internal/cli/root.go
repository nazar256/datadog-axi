package cli

import (
	"fmt"
	"strings"

	"github.com/nazar256/datadog-cli/internal/runtime"
	"github.com/spf13/cobra"
)

type GlobalOptions struct {
	runtime.FlagValues
	BuildInfo BuildInfo
	Services  serviceSet
}

func NewRootCmd(buildInfo BuildInfo) *cobra.Command {
	return newRootCmdWithOptions(&GlobalOptions{
		BuildInfo: buildInfo,
	})
}

func newRootCmdWithOptions(opts *GlobalOptions) *cobra.Command {
	opts.ensureServices()
	if opts.BuildInfo.Version == "" {
		opts.BuildInfo.Version = "dev"
	}

	cmd := &cobra.Command{
		Use:   "ddog",
		Short: "Read Datadog monitors, dashboards, hosts, metrics, and logs",
		Long: strings.TrimSpace(`ddog is a Datadog CLI for humans, coding agents, and automation.

Use 'ddog <command> --help' to explore the command tree. Offline-safe commands such as
'version', 'docs', 'doctor', 'config doctor', and 'completion' work without Datadog credentials. Live Datadog
commands use DATADOG_API_KEY and DATADOG_APP_KEY from the environment or a local .env file.`),
		Example: strings.TrimSpace(`ddog doctor

ddog docs commands --output json

ddog completion --help

ddog version`),
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().StringVar(&opts.Site, "site", "", "Datadog site (e.g., datadoghq.com, us3, eu)")
	cmd.PersistentFlags().StringVar(&opts.EnvFile, "env-file", "", "Path to .env file (default .env)")
	cmd.PersistentFlags().BoolVar(&opts.NoEnvFile, "no-env-file", false, "Do not load .env file")
	cmd.PersistentFlags().DurationVar(&opts.Timeout, "timeout", 0, "API timeout (default 30s)")
	cmd.PersistentFlags().StringVarP(&opts.Output, "output", "o", "", "Output format (text, json)")

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
		return fmt.Errorf("%w\n\nSee '%s --help' for usage.", err, cmd.CommandPath())
	})

	return cmd
}
