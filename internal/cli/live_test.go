package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nazar256/datadog-axi/internal/domain/dashboard"
	"github.com/nazar256/datadog-axi/internal/domain/host"
	"github.com/nazar256/datadog-axi/internal/domain/logs"
	"github.com/nazar256/datadog-axi/internal/domain/metric"
	"github.com/nazar256/datadog-axi/internal/domain/monitor"
	"github.com/nazar256/datadog-axi/internal/domain/slo"
	"github.com/nazar256/datadog-axi/internal/runtime"
)

type fakeMonitorService struct{}

func (fakeMonitorService) List(context.Context, runtime.Config, monitor.ListParams) (monitor.ListResult, error) {
	return monitor.ListResult{Items: []monitor.Summary{{ID: 123, Name: "CPU high", State: "Alert", Type: "query alert", Query: "avg:test{*} > 1"}}, Count: 1}, nil
}
func (fakeMonitorService) Get(context.Context, runtime.Config, int64) (monitor.Detail, error) {
	return monitor.Detail{ID: 123, Name: "CPU high", State: "Alert", Type: "query alert", Query: "avg:test{*} > 1"}, nil
}

type fakeDashboardService struct{}

func (fakeDashboardService) List(context.Context, runtime.Config, dashboard.ListParams) (dashboard.ListResult, error) {
	return dashboard.ListResult{}, nil
}
func (fakeDashboardService) Get(context.Context, runtime.Config, string) (dashboard.Detail, error) {
	return dashboard.Detail{}, nil
}

type fakeHostService struct{}

func (fakeHostService) List(context.Context, runtime.Config, host.ListParams) (host.ListResult, error) {
	return host.ListResult{}, nil
}
func (fakeHostService) Get(context.Context, runtime.Config, string) (host.Detail, error) {
	return host.Detail{}, nil
}

type fakeMetricService struct{}

func (fakeMetricService) Query(context.Context, runtime.Config, metric.QueryParams) (metric.QueryResult, error) {
	v := 1.5
	lastPoint := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	return metric.QueryResult{Query: "avg:test{*}", Count: 1, Series: []metric.Series{{Metric: "test.metric", Scope: "*", Aggregator: "avg", PointCount: 7, LastValue: &v, LastPointTS: &lastPoint}}}, nil
}
func (fakeMetricService) Search(context.Context, runtime.Config, metric.SearchParams) (metric.SearchResult, error) {
	return metric.SearchResult{Query: "system.cpu", Metrics: []string{"system.cpu.user", "system.cpu.system"}, Count: 2}, nil
}
func (fakeMetricService) Active(context.Context, runtime.Config, metric.ActiveParams) (metric.ActiveResult, error) {
	return metric.ActiveResult{From: time.Date(2026, 3, 21, 9, 0, 0, 0, time.UTC), Metrics: []string{"system.cpu.user"}, Count: 1}, nil
}

type fakeLogsService struct{}

func (fakeLogsService) Search(context.Context, runtime.Config, logs.SearchParams) (logs.SearchResult, error) {
	timestamp := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	return logs.SearchResult{Query: "service:web", Count: 1, Items: []logs.Entry{{Timestamp: &timestamp, Service: "web", Status: "error", Host: "web-01", Message: "boom"}}}, nil
}
func (fakeLogsService) Aggregate(context.Context, runtime.Config, logs.AggregateParams) (logs.AggregateResult, error) {
	return logs.AggregateResult{}, nil
}

type fakeSLOService struct{}

func (fakeSLOService) List(context.Context, runtime.Config, slo.ListParams) (slo.ListResult, error) {
	modified := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	target := 99.9
	return slo.ListResult{Items: []slo.Summary{{ID: "slo-123", Name: "API availability", Type: "monitor", Timeframe: "7d", TargetThreshold: &target, ModifiedAt: &modified}}, Count: 1}, nil
}
func (fakeSLOService) Get(context.Context, runtime.Config, string) (slo.Detail, error) {
	return slo.Detail{Summary: slo.Summary{ID: "slo-123", Name: "API availability", Type: "monitor", Timeframe: "7d"}}, nil
}
func (fakeSLOService) Search(context.Context, runtime.Config, slo.SearchParams) (slo.SearchResult, error) {
	status := "ok"
	sli := 99.95
	budget := 0.55
	return slo.SearchResult{Items: []slo.Summary{{ID: "slo-123", Name: "API availability", Type: "monitor", Timeframe: "7d", State: status, SLI: &sli, ErrorBudgetRemaining: &budget, MonitorIDs: []int64{42}}}, Count: 1, Total: int64Ptr(1)}, nil
}
func (fakeSLOService) GetWithHistory(context.Context, runtime.Config, string, slo.HistoryParams) (slo.Detail, error) {
	sli := 99.95
	return slo.Detail{Summary: slo.Summary{ID: "slo-123", Name: "API availability", Type: "monitor", Timeframe: "7d"}, State: "ok", SLI: &sli, History: &slo.History{BurnRate: slo.DerivedValue{Status: "unavailable", Source: "derived", Reason: "insufficient observations"}}}, nil
}

func int64Ptr(value int64) *int64 { return &value }

func TestMonitorListJSON(t *testing.T) {
	cmd := newRootCmdWithOptions(&GlobalOptions{Services: serviceSet{Monitor: fakeMonitorService{}, Dashboard: fakeDashboardService{}, Host: fakeHostService{}, Metric: fakeMetricService{}, Logs: fakeLogsService{}}, FlagValues: runtime.FlagValues{NoEnvFile: true, Output: "json"}})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"monitor", "list", "--output", "json"})
	t.Setenv("DATADOG_API_KEY", "x")
	t.Setenv("DATADOG_APP_KEY", "y")
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "\"items\"") || !strings.Contains(buf.String(), "CPU high") || !strings.Contains(buf.String(), "\"next\"") {
		t.Fatalf("unexpected output: %s", buf.String())
	}
}

func TestSLOListJSON(t *testing.T) {
	cmd := newRootCmdWithOptions(&GlobalOptions{Services: serviceSet{Monitor: fakeMonitorService{}, Dashboard: fakeDashboardService{}, Host: fakeHostService{}, Metric: fakeMetricService{}, Logs: fakeLogsService{}, SLO: fakeSLOService{}}, FlagValues: runtime.FlagValues{NoEnvFile: true, Output: "json"}})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"slo", "list", "--query", "availability", "--output", "json"})
	t.Setenv("DATADOG_API_KEY", "x")
	t.Setenv("DATADOG_APP_KEY", "y")
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "slo-123") || !strings.Contains(buf.String(), "API availability") {
		t.Fatalf("unexpected output: %s", buf.String())
	}
}

func TestSLOSearchJSONIncludesStatusContext(t *testing.T) {
	cmd := newRootCmdWithOptions(&GlobalOptions{Services: serviceSet{Monitor: fakeMonitorService{}, Dashboard: fakeDashboardService{}, Host: fakeHostService{}, Metric: fakeMetricService{}, Logs: fakeLogsService{}, SLO: fakeSLOService{}}, FlagValues: runtime.FlagValues{NoEnvFile: true, Output: "json"}})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"slo", "search", "--query", "availability", "--limit", "20", "--page", "2", "--output", "json"})
	t.Setenv("DATADOG_API_KEY", "x")
	t.Setenv("DATADOG_APP_KEY", "y")
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, expected := range []string{"\"state\":\"ok\"", "\"sli\":99.95", "\"error_budget_remaining\":0.55", "\"monitor_ids\":[42]"} {
		if !strings.Contains(buf.String(), expected) {
			t.Fatalf("expected %s in output: %s", expected, buf.String())
		}
	}
}

func TestSLOGetHistoryFlagIncludesHistory(t *testing.T) {
	cmd := newRootCmdWithOptions(&GlobalOptions{Services: serviceSet{Monitor: fakeMonitorService{}, Dashboard: fakeDashboardService{}, Host: fakeHostService{}, Metric: fakeMetricService{}, Logs: fakeLogsService{}, SLO: fakeSLOService{}}, FlagValues: runtime.FlagValues{NoEnvFile: true, Output: "json"}})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"slo", "get", "slo-123", "--last", "7d", "--output", "json"})
	t.Setenv("DATADOG_API_KEY", "x")
	t.Setenv("DATADOG_APP_KEY", "y")
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "\"history\"") || !strings.Contains(buf.String(), "\"burn_rate\"") {
		t.Fatalf("expected history context: %s", buf.String())
	}
}

func TestMetricQueryRequiresQuery(t *testing.T) {
	cmd := newRootCmdWithOptions(&GlobalOptions{Services: serviceSet{Monitor: fakeMonitorService{}, Dashboard: fakeDashboardService{}, Host: fakeHostService{}, Metric: fakeMetricService{}, Logs: fakeLogsService{}}, FlagValues: runtime.FlagValues{NoEnvFile: true}})
	cmd.SetArgs([]string{"metric", "query"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--query is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMetricQueryAllowsAbsoluteRange(t *testing.T) {
	cmd := newRootCmdWithOptions(&GlobalOptions{Services: serviceSet{Monitor: fakeMonitorService{}, Dashboard: fakeDashboardService{}, Host: fakeHostService{}, Metric: fakeMetricService{}, Logs: fakeLogsService{}}, FlagValues: runtime.FlagValues{NoEnvFile: true}})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	t.Setenv("DATADOG_API_KEY", "x")
	t.Setenv("DATADOG_APP_KEY", "y")
	cmd.SetArgs([]string{"metric", "query", "--query", "avg:test{*}", "--from", "2026-03-21T09:00:00Z", "--to", "2026-03-21T10:00:00Z", "--output", "text"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "test.metric") {
		t.Fatalf("unexpected output: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "POINTS") || !strings.Contains(buf.String(), "7") {
		t.Fatalf("expected metric text output to include point summary, got: %s", buf.String())
	}
}

func TestMetricDiscoveryCommands(t *testing.T) {
	service := serviceSet{Monitor: fakeMonitorService{}, Dashboard: fakeDashboardService{}, Host: fakeHostService{}, Metric: fakeMetricService{}, Logs: fakeLogsService{}}
	for _, args := range [][]string{
		{"metric", "search", "--query", "system.cpu", "--output", "json"},
		{"metric", "active", "--last", "1h", "--output", "json"},
	} {
		cmd := newRootCmdWithOptions(&GlobalOptions{Services: service, FlagValues: runtime.FlagValues{NoEnvFile: true}})
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		t.Setenv("DATADOG_API_KEY", "x")
		t.Setenv("DATADOG_APP_KEY", "y")
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute %v: %v", args, err)
		}
		if !strings.Contains(buf.String(), "system.cpu") {
			t.Fatalf("expected metric discovery output for %v: %s", args, buf.String())
		}
	}
}

func TestMetricQueryHelpGuidesJSONParsing(t *testing.T) {
	cmd := newRootCmdWithOptions(&GlobalOptions{FlagValues: runtime.FlagValues{NoEnvFile: true}})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"metric", "query", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	for _, snippet := range []string{"--output json", "point_count", "last_point_ts", "last_value"} {
		if !strings.Contains(output, snippet) {
			t.Fatalf("expected metric help to contain %q, got:\n%s", snippet, output)
		}
	}
}

func TestLogSearchText(t *testing.T) {
	cmd := newRootCmdWithOptions(&GlobalOptions{Services: serviceSet{Monitor: fakeMonitorService{}, Dashboard: fakeDashboardService{}, Host: fakeHostService{}, Metric: fakeMetricService{}, Logs: fakeLogsService{}}, FlagValues: runtime.FlagValues{NoEnvFile: true}})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	t.Setenv("DATADOG_API_KEY", "x")
	t.Setenv("DATADOG_APP_KEY", "y")
	cmd.SetArgs([]string{"log", "search", "--query", "service:web", "--last", "15m"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "web-01") || !strings.Contains(buf.String(), "boom") {
		t.Fatalf("unexpected output: %s", buf.String())
	}
}

func TestLogSearchRejectsInvalidSort(t *testing.T) {
	cmd := newRootCmdWithOptions(&GlobalOptions{Services: serviceSet{Monitor: fakeMonitorService{}, Dashboard: fakeDashboardService{}, Host: fakeHostService{}, Metric: fakeMetricService{}, Logs: fakeLogsService{}}, FlagValues: runtime.FlagValues{NoEnvFile: true}})
	t.Setenv("DATADOG_API_KEY", "x")
	t.Setenv("DATADOG_APP_KEY", "y")
	cmd.SetArgs([]string{"log", "search", "--query", "service:web", "--sort", "sideways"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--sort must be 'asc' or 'desc'") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLogSearchHelpGuidesExploration(t *testing.T) {
	cmd := newRootCmdWithOptions(&GlobalOptions{FlagValues: runtime.FlagValues{NoEnvFile: true}})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"log", "search", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	for _, snippet := range []string{"focused query", "--last", "--index", "--output json", "@http.status_code:[500 TO 599]"} {
		if !strings.Contains(output, snippet) {
			t.Fatalf("expected log help to contain %q, got:\n%s", snippet, output)
		}
	}
}

func TestDocsIgnoreInvalidSite(t *testing.T) {
	cmd := newRootCmdWithOptions(&GlobalOptions{FlagValues: runtime.FlagValues{NoEnvFile: true}})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	t.Setenv("DATADOG_SITE", "evil.example.com")
	cmd.SetArgs([]string{"docs", "summary"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "datadog-axi provides an AXI-oriented Datadog investigation CLI") {
		t.Fatalf("unexpected output: %s", buf.String())
	}
}
