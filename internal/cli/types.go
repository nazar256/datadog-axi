package cli

import (
	"fmt"

	"github.com/nazar256/datadog-axi/internal/runtime"
)

type BuildInfo struct {
	Product string
	Version string
	Commit  string
	Date    string
}

type UsageError = runtime.UsageError

func usageError(err error) error {
	return runtime.WrapUsageError(err)
}
func usagef(format string, args ...any) error { return usageError(fmt.Errorf(format, args...)) }
