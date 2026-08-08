package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

type versionView struct {
	Product string `json:"product"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

func newVersionCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "version",
		Short:   "Print the CLI version",
		Args:    cobra.NoArgs,
		GroupID: "utility",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := runtimeConfigForOffline(opts)
			if err != nil {
				return err
			}
			view := versionView{Product: opts.BuildInfo.Product, Version: opts.BuildInfo.Version, Commit: opts.BuildInfo.Commit, Date: opts.BuildInfo.Date}
			return writeOutput(cmd, opts, cfg, view, func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "%s version %s (commit: %s, date: %s)\n", opts.BuildInfo.Product, opts.BuildInfo.Version, opts.BuildInfo.Commit, opts.BuildInfo.Date)
				return err
			})
		},
	}
	return cmd
}
