package runtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
)

type Client struct {
	API    *datadog.APIClient
	Ctx    context.Context
	Config Config
}

func NewClient(parent context.Context, cfg Config) (*Client, error) {
	if err := cfg.RequireAuth(); err != nil {
		return nil, err
	}
	if parent == nil {
		parent = context.Background()
	}

	configuration := datadog.NewConfiguration()
	configuration.HTTPClient = &http.Client{Timeout: cfg.Timeout, Transport: retryTransport{base: http.DefaultTransport, attempts: 3}}
	version := cfg.Version
	if version == "" {
		version = "dev"
	}
	configuration.UserAgent = "datadog-axi/" + version + " (" + cfg.Site + ")"

	apiClient := datadog.NewAPIClient(configuration)
	ctx := context.WithValue(parent, datadog.ContextAPIKeys, map[string]datadog.APIKey{
		"apiKeyAuth": {Key: cfg.APIKey},
		"appKeyAuth": {Key: cfg.AppKey},
	})
	ctx = context.WithValue(ctx, datadog.ContextServerVariables, map[string]string{"site": cfg.Site})

	return &Client{API: apiClient, Ctx: ctx, Config: cfg}, nil
}

type retryTransport struct {
	base     http.RoundTripper
	attempts int
}

func (t retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	attempts := t.attempts
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		resp, err := base.RoundTrip(req)
		if !shouldRetry(req, resp, err) || attempt == attempts {
			return resp, err
		}
		wait, ok := retryDelay(resp, attempt, req.Context())
		if !ok {
			return resp, err
		}
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if req.Body != nil {
			if req.GetBody == nil {
				return resp, err
			}
			body, bodyErr := req.GetBody()
			if bodyErr != nil {
				return resp, err
			}
			req.Body = body
		}
		timer := time.NewTimer(wait)
		select {
		case <-req.Context().Done():
			timer.Stop()
			return nil, req.Context().Err()
		case <-timer.C:
		}
	}
	return nil, nil
}

func shouldRetry(req *http.Request, resp *http.Response, err error) bool {
	if req == nil || !retryableRead(req) {
		return false
	}
	if err != nil {
		return true
	}
	return resp != nil && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500)
}

func retryableRead(req *http.Request) bool {
	if strings.EqualFold(req.Method, http.MethodGet) {
		return true
	}
	if !strings.EqualFold(req.Method, http.MethodPost) {
		return false
	}
	path := strings.ToLower(req.URL.Path)
	return strings.HasSuffix(path, "/spans/events/search") ||
		strings.HasSuffix(path, "/audit/events/search") ||
		strings.HasSuffix(path, "/logs/events/search") ||
		strings.HasSuffix(path, "/logs/analytics/aggregate")
}

func retryDelay(resp *http.Response, attempt int, ctx context.Context) (time.Duration, bool) {
	if resp != nil {
		if raw := strings.TrimSpace(resp.Header.Get("Retry-After")); raw != "" {
			if seconds, err := strconv.Atoi(raw); err == nil && seconds >= 0 {
				return retryWait(time.Duration(seconds)*time.Second, ctx)
			}
			if when, err := http.ParseTime(raw); err == nil {
				return retryWait(time.Until(when), ctx)
			}
			return 0, false
		}
	}
	return retryWait(minDuration(time.Duration(attempt)*100*time.Millisecond, 2*time.Second), ctx)
}

func retryWait(wait time.Duration, ctx context.Context) (time.Duration, bool) {
	if wait < 0 || wait > 60*time.Second {
		return 0, false
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= wait {
		return 0, false
	}
	return wait, true
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// WrapAPIError turns SDK failures into stable, non-sensitive CLI errors. The
// optional config values let adapters redact credentials loaded from env files
// as well as credentials inherited from the process environment.
func WrapAPIError(err error, configs ...Config) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("datadog API request canceled")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("datadog API request timed out")
	}
	var generic datadog.GenericOpenAPIError
	if errors.As(err, &generic) && generic.ErrorMessage != "" {
		message := generic.ErrorMessage
		switch model := generic.ErrorModel.(type) {
		case *datadogV1.APIErrorResponse:
			if model != nil && len(model.GetErrors()) > 0 {
				message += ": " + strings.Join(model.GetErrors(), "; ")
			}
		case datadogV1.APIErrorResponse:
			if len(model.GetErrors()) > 0 {
				message += ": " + strings.Join(model.GetErrors(), "; ")
			}
		}
		return fmt.Errorf("datadog API request failed: %s", SanitizeError(message, configs...))
	}
	return fmt.Errorf("datadog API request failed: transport error")
}

// SanitizeError removes configured Datadog credentials from a human-facing
// error. Values are sorted longest-first so a shorter credential cannot leave
// a suffix of a longer value visible after replacement.
func SanitizeError(message string, configs ...Config) string {
	values := make(map[string]struct{}, 4+len(configs)*2)
	for _, name := range []string{"DD_API_KEY", "DD_APP_KEY", "DATADOG_API_KEY", "DATADOG_APP_KEY"} {
		if value, ok := os.LookupEnv(name); ok && value != "" {
			values[value] = struct{}{}
		}
	}
	for _, cfg := range configs {
		if cfg.APIKey != "" {
			values[cfg.APIKey] = struct{}{}
		}
		if cfg.AppKey != "" {
			values[cfg.AppKey] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(values))
	for value := range values {
		ordered = append(ordered, value)
	}
	sort.SliceStable(ordered, func(i, j int) bool { return len(ordered[i]) > len(ordered[j]) })
	for _, value := range ordered {
		message = strings.ReplaceAll(message, value, "[REDACTED]")
	}
	return message
}
