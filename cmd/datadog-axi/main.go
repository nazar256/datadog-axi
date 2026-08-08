package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nazar256/datadog-axi/internal/cli"
	"github.com/nazar256/datadog-axi/internal/output"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	cmd := cli.NewRootCmd(cli.BuildInfo{Product: "datadog-axi", Version: version, Commit: commit, Date: date})
	executed, err := cmd.ExecuteC()
	if err != nil {
		if executed == nil {
			executed = cmd
		}
		code := 1
		if isUsageError(err) {
			code = 2
		}
		help := "run datadog-axi --help"
		if path := strings.TrimSpace(executed.CommandPath()); path != "" && path != "datadog-axi" {
			help = "run " + path + " --help"
		} else if marker := "See '"; strings.Contains(err.Error(), marker) {
			message := err.Error()
			start := strings.Index(message, marker) + len(marker)
			if end := strings.Index(message[start:], " --help'"); end >= 0 {
				help = "run " + message[start:start+end] + " --help"
			}
		}
		errorView := map[string]any{"error": cliruntime.SanitizeError(err.Error()), "code": code, "help": help}
		if requestedJSON(executed) {
			_ = output.Write(executed.OutOrStdout(), output.JSON, errorView, nil)
		} else {
			fmt.Fprint(executed.OutOrStdout(), output.EncodeTOON(errorView))
		}
		os.Exit(code)
	}
}

func requestedJSON(cmd *cobra.Command) bool {
	if flag := cmd.Flags().Lookup("json"); flag != nil && flag.Value.String() == "true" {
		return true
	}
	if flag := cmd.Flags().Lookup("output"); flag != nil && strings.EqualFold(flag.Value.String(), "json") {
		return true
	}
	return false
}

func isUsageError(err error) bool {
	var usage *cli.UsageError
	if errors.As(err, &usage) {
		return true
	}
	s := err.Error()
	for _, marker := range []string{"unknown flag", "unknown command", "required flag", "unsupported output format", "accepts ", "arg(s)", "requires at least", "requires between"} {
		for i := 0; i+len(marker) <= len(s); i++ {
			if s[i:i+len(marker)] == marker {
				return true
			}
		}
	}
	return false
}
