package logs

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
	"github.com/nazar256/datadog-axi/internal/timeutil"
)

func TestMapEntry(t *testing.T) {
	ts := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
	attrs := datadogV2.NewLogAttributes()
	attrs.SetTimestamp(ts)
	attrs.SetService("api")
	attrs.SetStatus("error")
	attrs.SetHost("web-01")
	attrs.SetMessage("boom")
	attrs.SetTags([]string{"env:prod"})
	entry := datadogV2.NewLog()
	entry.SetId("abc")
	typeValue := datadogV2.LOGTYPE_LOG
	entry.SetType(typeValue)
	entry.AdditionalProperties = map[string]interface{}{"ingest_source": "custom", "api_key": "should-not-print"}
	attrs.SetAttributes(map[string]interface{}{"nested": map[string]interface{}{"password": "should-not-print"}})
	entry.SetAttributes(*attrs)

	view := mapEntry(*entry)
	if view.ID != "abc" || view.Service != "api" || view.Message != "boom" {
		t.Fatalf("unexpected view: %+v", view)
	}
	if view.Type != "log" || view.Raw["ingest_source"] != "custom" || view.Raw["attributes"] == nil {
		t.Fatalf("expected complete raw event, got %+v", view.Raw)
	}
	if view.Raw["api_key"] != "[REDACTED]" || view.Attributes["nested"].(map[string]interface{})["password"] != "[REDACTED]" {
		t.Fatalf("expected sensitive fields redacted: raw=%+v attrs=%+v", view.Raw, view.Attributes)
	}
}

func TestMapBucketRedactsSensitiveFacetValues(t *testing.T) {
	bucket := datadogV2.NewLogsAggregateBucket()
	bucket.UnparsedObject = map[string]interface{}{
		"by":       map[string]interface{}{"api_key": "should-not-print", "service": "web"},
		"computes": map[string]interface{}{"count": 1},
	}
	view := mapBucket(*bucket)
	if view.Raw["by"].(map[string]interface{})["api_key"] != "[REDACTED]" {
		t.Fatalf("sensitive aggregate facet leaked: %#v", view.Raw)
	}
	if view.By["service"] != "web" || view.Computes["count"] != float64(1) {
		t.Fatalf("aggregate bucket shape was not preserved: %#v", view)
	}
}

func TestParseSpecs(t *testing.T) {
	facets, err := ParseFacetSpecs([]string{"service,status:20", "@http.status_code"})
	if err != nil {
		t.Fatalf("parse facets: %v", err)
	}
	if len(facets) != 3 || facets[1].Facet != "status" || facets[1].Limit != 20 {
		t.Fatalf("unexpected facets: %+v", facets)
	}
	computes, err := ParseComputeSpecs([]string{"count,avg(@duration)", "timeseries(sum:@bytes)"})
	if err != nil {
		t.Fatalf("parse computes: %v", err)
	}
	if len(computes) != 3 || computes[1].Aggregation != "avg" || computes[1].Metric != "@duration" || computes[2].Type != "timeseries" {
		t.Fatalf("unexpected computes: %+v", computes)
	}
}

func TestParseComputeRejectsInvalidCombination(t *testing.T) {
	if _, err := ParseComputeSpecs([]string{"count(@duration)"}); err == nil {
		t.Fatal("expected count metric validation error")
	}
	if _, err := ParseComputeSpecs([]string{"avg"}); err == nil {
		t.Fatal("expected metric validation error")
	}
	if _, err := ParseComputeSpecs([]string{"not-an-aggregation"}); err == nil {
		t.Fatal("expected aggregation validation error")
	}
	if _, err := ParseFacetSpecs([]string{"status:no-limit"}); err == nil {
		t.Fatal("expected facet limit validation error")
	}
}

func TestPageBoundsAreHardCapped(t *testing.T) {
	if err := validateSearchParams(SearchParams{Query: "*", MaxPages: 101}); err == nil {
		t.Fatal("expected search page bound")
	}
	if err := validateAggregateParams(AggregateParams{Query: "*", MaxPages: 101}); err == nil {
		t.Fatal("expected aggregate page bound")
	}
}

func TestListPOSTUsesSearchEndpointAndBody(t *testing.T) {
	seen := make(chan struct {
		req  *http.Request
		body []byte
	}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen <- struct {
			req  *http.Request
			body []byte
		}{req: r, body: body}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[],"meta":{"page":{"after":"next"}}}`)
	}))
	defer server.Close()
	client, api := testClient(server)
	rangeValue := timeutil.Range{From: time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC), To: time.Date(2026, 3, 21, 11, 0, 0, 0, time.UTC)}
	_, err := listPOST(client, api, SearchParams{Query: "service:web", Range: rangeValue, Limit: 25, Indexes: []string{"main"}, StorageTier: "flex"}, "")
	if err != nil {
		t.Fatalf("list post: %v", err)
	}
	captured := <-seen
	req := captured.req
	if req.Method != http.MethodPost || req.URL.Path != "/api/v2/logs/events/search" {
		t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(captured.body, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	filter := body["filter"].(map[string]interface{})
	if filter["query"] != "service:web" || filter["storage_tier"] != "flex" {
		t.Fatalf("unexpected filter: %+v", filter)
	}
	if body["page"].(map[string]interface{})["limit"] != float64(25) {
		t.Fatalf("unexpected page: %+v", body["page"])
	}
}

func TestAggregateUsesAnalyticsEndpointAndPreservesBuckets(t *testing.T) {
	seen := make(chan struct {
		req  *http.Request
		body []byte
	}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen <- struct {
			req  *http.Request
			body []byte
		}{req: r, body: body}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"buckets":[{"by":{"service":"web"},"computes":{"count":3}}]},"meta":{"page":{"after":"next"},"warnings":[{"code":"partial","title":"Partial","detail":"sampled"}]}}`)
	}))
	defer server.Close()
	client, api := testClient(server)
	rangeValue := timeutil.Range{From: time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC), To: time.Date(2026, 3, 21, 11, 0, 0, 0, time.UTC)}
	params := AggregateParams{Query: "service:web", Range: rangeValue, Facets: []FacetSpec{{Facet: "service"}}, Computes: []ComputeSpec{{Aggregation: "count", Type: "total"}}}
	response, _, err := api.AggregateLogs(client.Ctx, aggregateRequest(params, ""))
	if err != nil {
		t.Fatalf("aggregate: %v", err)
	}
	captured := <-seen
	req := captured.req
	if req.Method != http.MethodPost || req.URL.Path != "/api/v2/logs/analytics/aggregate" {
		t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(captured.body, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	if len(body["group_by"].([]interface{})) != 1 || body["group_by"].([]interface{})[0].(map[string]interface{})["facet"] != "service" {
		t.Fatalf("unexpected group_by: %+v", body["group_by"])
	}
	if body["compute"].([]interface{})[0].(map[string]interface{})["aggregation"] != "count" {
		t.Fatalf("unexpected compute: %+v", body["compute"])
	}
	data := response.GetData()
	buckets := data.GetBuckets()
	if len(buckets) != 1 || buckets[0].GetBy()["service"] != "web" {
		t.Fatalf("unexpected response: %+v", buckets)
	}
}

func testClient(server *httptest.Server) (*cliruntime.Client, *datadogV2.LogsApi) {
	configuration := datadog.NewConfiguration()
	configuration.Servers[0].URL = server.URL
	configuration.HTTPClient = server.Client()
	apiClient := datadog.NewAPIClient(configuration)
	ctx := context.WithValue(context.Background(), datadog.ContextAPIKeys, map[string]datadog.APIKey{
		"apiKeyAuth": {Key: "key"},
		"appKeyAuth": {Key: "app"},
	})
	client := &cliruntime.Client{API: apiClient, Ctx: ctx}
	return client, datadogV2.NewLogsApi(apiClient)
}
