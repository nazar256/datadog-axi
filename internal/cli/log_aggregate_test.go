package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/nazar256/datadog-axi/internal/domain/logs"
	"github.com/nazar256/datadog-axi/internal/runtime"
)

type aggregateOnlyLogsService struct{}

func (aggregateOnlyLogsService) Search(context.Context, runtime.Config, logs.SearchParams) (logs.SearchResult, error) {
	return logs.SearchResult{}, nil
}

func (aggregateOnlyLogsService) Aggregate(context.Context, runtime.Config, logs.AggregateParams) (logs.AggregateResult, error) {
	return logs.AggregateResult{
		Query:        "service:web",
		Count:        1,
		PagesFetched: 1,
		Buckets: []logs.Bucket{{
			By:       map[string]interface{}{"status": "error"},
			Computes: map[string]interface{}{"count": float64(3)},
		}},
	}, nil
}

func TestLogAggregateJSONKeepsBucketsSeparate(t *testing.T) {
	cmd := newRootCmdWithOptions(&GlobalOptions{Services: serviceSet{Logs: aggregateOnlyLogsService{}}, FlagValues: runtime.FlagValues{NoEnvFile: true, Output: "json"}})
	cmd.SetArgs([]string{"log", "aggregate", "--query", "service:web", "--facet", "status", "--compute", "count", "--output", "json"})
	t.Setenv("DATADOG_API_KEY", "test-key")
	t.Setenv("DATADOG_APP_KEY", "test-app")
	output := new(strings.Builder)
	cmd.SetOut(output)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result := output.String()
	if !strings.Contains(result, `"buckets"`) || strings.Contains(result, `"items"`) {
		t.Fatalf("expected aggregate buckets without event items: %s", result)
	}
}
