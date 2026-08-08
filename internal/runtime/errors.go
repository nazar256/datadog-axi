package runtime

import "fmt"

// UsageError marks input/configuration failures that should use the CLI's
// usage exit code rather than the operational failure code.
type UsageError struct{ Err error }

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

func WrapUsageError(err error) error {
	if err == nil {
		return nil
	}
	return &UsageError{Err: err}
}

func UsageErrorf(format string, args ...any) error {
	return WrapUsageError(fmt.Errorf(format, args...))
}
