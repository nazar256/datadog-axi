package main

import (
	"fmt"
	"os"

	"github.com/nazar256/datadog-axi/internal/cli"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	cmd := cli.NewRootCmd(cli.BuildInfo{
		Product: "ddog",
		Version: version,
		Commit:  commit,
		Date:    date,
	})
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, cliruntime.SanitizeError(err.Error()))
		os.Exit(1)
	}
}
