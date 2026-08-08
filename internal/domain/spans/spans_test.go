package spans

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
	"github.com/nazar256/datadog-axi/internal/timeutil"
)

func TestMapSpan(t *testing.T) {
	start := time.Date(2026, 8, 4, 12, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	end := start.Add(250 * time.Millisecond)
	attrs := datadogV2.NewSpansAttributes()
	attrs.SetStartTimestamp(start)
	attrs.SetEndTimestamp(end)
	attrs.SetService("api")
	attrs.SetResourceName("GET /hotels")
	attrs.SetType("web")
	attrs.SetEnv("prod")
	attrs.SetHost("web-01")
	attrs.SetTraceId("trace-1")
	attrs.SetSpanId("span-1")
	attrs.SetParentId("parent-1")
	attrs.SetTags([]string{"env:prod"})
	span := datadogV2.NewSpan()
	span.SetId("abc")
	span.SetAttributes(*attrs)

	view := mapSpan(*span)
	if view.ID != "abc" || view.Service != "api" || view.Resource != "GET /hotels" || view.Type != "web" {
		t.Fatalf("unexpected span mapping: %+v", view)
	}
	if view.Start == nil || !view.Start.Equal(start.UTC()) || view.End == nil || !view.End.Equal(end.UTC()) {
		t.Fatalf("unexpected span timestamps: %+v", view)
	}
	if view.TraceID != "trace-1" || view.SpanID != "span-1" || view.ParentID != "parent-1" {
		t.Fatalf("unexpected span IDs: %+v", view)
	}
}

func TestMapSpanDerivesContextAndPreservesRawAttributes(t *testing.T) {
	start := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	end := start.Add(1250 * time.Millisecond)
	attrs := datadogV2.NewSpansAttributes()
	attrs.SetStartTimestamp(start)
	attrs.SetEndTimestamp(end)
	attrs.SetCustom(map[string]interface{}{"operation_name": "GET /hotels", "version": "2026.08", "error": true, "future": map[string]interface{}{"enabled": true}})
	span := datadogV2.NewSpan()
	span.SetId("span-raw")
	span.SetAttributes(*attrs)
	view := mapSpan(*span)
	if view.DurationMS == nil || *view.DurationMS != 1250 {
		t.Fatalf("unexpected derived duration: %+v", view.DurationMS)
	}
	if view.Operation != "GET /hotels" || view.Version != "2026.08" || view.Status != "error" {
		t.Fatalf("unexpected derived context: %+v", view)
	}
	if len(view.Derived) != 4 || view.Raw["attributes"] == nil {
		t.Fatalf("expected derived metadata and raw payload: %+v", view)
	}
	if view.Attributes["future"].(map[string]interface{})["enabled"] != true {
		t.Fatalf("custom attribute was dropped: %+v", view.Attributes)
	}
}

func TestComposeQueryEscapesDedicatedFilters(t *testing.T) {
	duration := 250 * time.Millisecond
	query := ComposeQuery(SearchParams{Query: "service:api", Env: "prod west", Operation: "GET /hotels", DurationMin: &duration, Tags: []string{"team:platform"}, TraceID: "trace-1"})
	want := `service:api env:"prod west" operation_name:"GET /hotels" duration:>250ms team:platform trace_id:trace-1`
	if query != want {
		t.Fatalf("unexpected composed query: %q (want %q)", query, want)
	}
}

func TestAggregateRequestRejectsUnsupportedBounds(t *testing.T) {
	params := AggregateParams{GroupBy: []string{"host"}, Compute: []string{"avg"}}
	if _, _, _, err := aggregateRequest(params); err == nil {
		t.Fatal("expected unsupported group-by validation error")
	}
	params.GroupBy = []string{"service"}
	if _, _, _, err := aggregateRequest(params); err == nil {
		t.Fatal("expected unsupported compute validation error")
	}
}

func TestSpanSearchUsesOfficialEndpointAndRequestFilters(t *testing.T) {
	seen := make(chan capturedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen <- capturedRequest{Method: r.Method, Path: r.URL.Path, Body: body}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[],"meta":{"page":{"after":"next"}}}`)
	}))
	defer server.Close()
	client, api := testClient(server)
	params := SearchParams{Query: "service:web", Range: timeutil.Range{From: time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC), To: time.Date(2026, 8, 4, 11, 0, 0, 0, time.UTC)}, Limit: 25, Cursor: "cursor-1", SortAsc: true, Service: "web", Env: "prod"}
	response, _, err := api.ListSpans(client.Ctx, searchRequest(params))
	if err != nil {
		t.Fatalf("list spans: %v", err)
	}
	meta := response.GetMeta()
	page := meta.GetPage()
	if !page.HasAfter() {
		t.Fatal("expected pagination metadata")
	}
	req := <-seen
	if req.Method != http.MethodPost || req.Path != "/api/v2/spans/events/search" {
		t.Fatalf("unexpected request: %s %s", req.Method, req.Path)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	data := body["data"].(map[string]interface{})
	attrs := data["attributes"].(map[string]interface{})
	filter := attrs["filter"].(map[string]interface{})
	if filter["query"] != "service:web service:web env:prod" {
		t.Fatalf("unexpected query: %#v", filter["query"])
	}
	if attrs["sort"] != "timestamp" {
		t.Fatalf("unexpected sort: %#v", attrs["sort"])
	}
	if attrs["page"].(map[string]interface{})["cursor"] != "cursor-1" {
		t.Fatalf("unexpected cursor: %#v", attrs["page"])
	}
}

func TestSpanAggregateUsesOfficialEndpointAndPreservesWarnings(t *testing.T) {
	seen := make(chan capturedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen <- capturedRequest{Method: r.Method, Path: r.URL.Path, Body: body}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Limit", "300")
		w.Header().Set("X-RateLimit-Remaining", "299")
		_, _ = io.WriteString(w, `{"data":[{"id":"service:web","type":"bucket","attributes":{"by":{"service":"web"},"computes":{"count":3}}}],"meta":{"status":"done","warnings":[{"code":"partial","title":"Partial","detail":"sampled"}]}}`)
	}))
	defer server.Close()
	client, api := testClient(server)
	params := AggregateParams{Query: "env:prod", Range: timeutil.Range{From: time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC), To: time.Date(2026, 8, 4, 11, 0, 0, 0, time.UTC)}, GroupBy: []string{"service", "env"}, Compute: []string{"count"}, BucketLimit: 25}
	response, httpResponse, err := api.AggregateSpans(client.Ctx, mustAggregateRequest(t, params))
	if err != nil {
		t.Fatalf("aggregate spans: %v", err)
	}
	req := <-seen
	if req.Method != http.MethodPost || req.Path != "/api/v2/spans/analytics/aggregate" {
		t.Fatalf("unexpected request: %s %s", req.Method, req.Path)
	}
	if httpResponse.Header.Get("X-RateLimit-Limit") != "300" {
		t.Fatal("expected rate limit response header")
	}
	if len(response.GetData()) != 1 {
		t.Fatalf("unexpected aggregate response: %+v", response.GetData())
	}
	aggregateAttrs := response.GetData()[0].GetAttributes()
	if aggregateAttrs.GetBy()["service"] != "web" {
		t.Fatalf("unexpected aggregate response: %+v", response.GetData())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	attrs := body["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	if len(attrs["group_by"].([]interface{})) != 2 || attrs["compute"].([]interface{})[0].(map[string]interface{})["aggregation"] != "count" {
		t.Fatalf("unexpected aggregate request: %#v", attrs)
	}
}

type capturedRequest struct {
	Method string
	Path   string
	Body   []byte
}

func mustAggregateRequest(t *testing.T, params AggregateParams) datadogV2.SpansAggregateRequest {
	t.Helper()
	body, _, _, err := aggregateRequest(params)
	if err != nil {
		t.Fatalf("aggregate request: %v", err)
	}
	return body
}

func testClient(server *httptest.Server) (*cliruntime.Client, *datadogV2.SpansApi) {
	configuration := datadog.NewConfiguration()
	target, err := url.Parse(server.URL)
	if err != nil {
		panic(err)
	}
	configuration.Servers[0].URL = target.String()
	configuration.HTTPClient = server.Client()
	apiClient := datadog.NewAPIClient(configuration)
	ctx := context.WithValue(context.Background(), datadog.ContextAPIKeys, map[string]datadog.APIKey{
		"apiKeyAuth": {Key: "key"},
		"appKeyAuth": {Key: "app"},
	})
	return &cliruntime.Client{API: apiClient, Ctx: ctx}, datadogV2.NewSpansApi(apiClient)
}
