package metric

import (
	"context"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
	"github.com/nazar256/datadog-axi/internal/timeutil"
)

func TestMapSeriesUsesLastValidPoint(t *testing.T) {
	metricName := "system.load.1"
	scope := "host:web-01"
	item := datadogV1.MetricsQueryMetadata{Pointlist: [][]*float64{{ptr(1711010000000), nil}, {ptr(1711010060000), ptr(1.5)}, {ptr(1711010120000), ptrNaN()}}}
	item.SetMetric(metricName)
	item.SetScope(scope)
	item.SetInterval(60000)

	view := mapSeries(item)
	if view.LastValue == nil || *view.LastValue != 1.5 {
		t.Fatalf("unexpected last value: %+v", view.LastValue)
	}
	if !view.LastPointTS.Equal(time.UnixMilli(1711010060000).UTC()) {
		t.Fatalf("unexpected last point timestamp: %v", view.LastPointTS)
	}
	if view.PointCount != 1 {
		t.Fatalf("unexpected point count: %d", view.PointCount)
	}
}

func ptr(v float64) *float64 { return &v }
func ptrNaN() *float64 {
	v := math.NaN()
	return &v
}

func TestQueryUsesMetricsQueryContract(t *testing.T) {
	from := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/query" {
			t.Fatalf("unexpected metric query request: %s %s", r.Method, r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("from") != "1785837600" || query.Get("to") != "1785841200" || query.Get("query") != "avg:system.load.1{host:web-01}" {
			t.Fatalf("unexpected metric query: %v", query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"ok","series":[{"metric":"system.load.1","scope":"host:web-01","aggr":"avg","interval":60000,"pointlist":[[1785837600000,1.5]]}]}`)
	}))
	defer server.Close()

	client := metricTestClient(t, server)
	result, err := queryWithClient(client, QueryParams{Query: "avg:system.load.1{host:web-01}", Range: timeutil.Range{From: from, To: to}})
	if err != nil {
		t.Fatalf("query metrics: %v", err)
	}
	if result.Status != "ok" || result.Count != 1 || result.Series[0].LastValue == nil || *result.Series[0].LastValue != 1.5 {
		t.Fatalf("unexpected metric result: %+v", result)
	}
}

func TestQueryRejectsApplicationLevelFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"error","error":"invalid metric query"}`)
	}))
	defer server.Close()
	from := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	_, err := queryWithClient(metricTestClient(t, server), QueryParams{Query: "bad", Range: timeutil.Range{From: from, To: from.Add(time.Hour)}})
	if err == nil || !strings.Contains(err.Error(), "invalid metric query") {
		t.Fatalf("expected application-level query error, got %v", err)
	}
}

func TestMetadataUsesMetricNamePath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/metrics/system.load.1" {
			t.Fatalf("unexpected metadata request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"type":"gauge","unit":"load","per_unit":"host","description":"system load","short_name":"load","integration":"system","statsd_interval":10,"tags":["env:prod"],"cardinality":3}`)
	}))
	defer server.Close()

	client := metricTestClient(t, server)
	metadata, err := metadataWithClient(client, "system.load.1")
	if err != nil {
		t.Fatalf("get metric metadata: %v", err)
	}
	if metadata.Metric != "system.load.1" || metadata.Type != "gauge" || metadata.Unit != "load" || metadata.StatsdInterval != 10 {
		t.Fatalf("unexpected metric metadata: %+v", metadata)
	}
	if metadata.Raw["cardinality"] != float64(3) {
		t.Fatalf("expected unknown metadata preservation: %+v", metadata.Raw)
	}
}

func TestSearchUsesMetricSearchContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/search" || r.URL.Query().Get("q") != "system.cpu" {
			t.Fatalf("unexpected metric search request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":{"metrics":["system.cpu.user","system.cpu.system","system.cpu.idle"]}}`)
	}))
	defer server.Close()

	result, err := searchWithClient(metricTestClient(t, server), SearchParams{Query: "system.cpu", Limit: 2})
	if err != nil {
		t.Fatalf("search metrics: %v", err)
	}
	if result.Count != 2 || result.Total != 3 || !result.Truncated || len(result.Metrics) != 2 || result.Metrics[0] != "system.cpu.user" {
		t.Fatalf("unexpected metric search result: %+v", result)
	}
}

func TestActiveUsesMetricListContract(t *testing.T) {
	from := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/metrics" {
			t.Fatalf("unexpected active metric request: %s %s", r.Method, r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("from") != "1785837600" || query.Get("host") != "web-01" || query.Get("tag_filter") != "env:prod" {
			t.Fatalf("unexpected active metric query: %v", query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"from":"1785837600","metrics":["system.cpu.user","system.cpu.system"]}`)
	}))
	defer server.Close()

	result, err := activeWithClient(metricTestClient(t, server), ActiveParams{From: from, Host: "web-01", TagFilter: "env:prod", Limit: 10})
	if err != nil {
		t.Fatalf("active metrics: %v", err)
	}
	if result.Count != 2 || result.Total != 2 || result.From.IsZero() || result.Metrics[1] != "system.cpu.system" {
		t.Fatalf("unexpected active metric result: %+v", result)
	}
}

func metricTestClient(t *testing.T, server *httptest.Server) *cliruntime.Client {
	t.Helper()
	configuration := datadog.NewConfiguration()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	configuration.Servers[0].URL = target.String()
	configuration.HTTPClient = server.Client()
	apiClient := datadog.NewAPIClient(configuration)
	ctx := context.WithValue(context.Background(), datadog.ContextAPIKeys, map[string]datadog.APIKey{
		"apiKeyAuth": {Key: "test-api-key"},
		"appKeyAuth": {Key: "test-app-key"},
	})
	return &cliruntime.Client{API: apiClient, Ctx: ctx}
}
