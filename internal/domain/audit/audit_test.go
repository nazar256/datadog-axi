package audit

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
	"github.com/nazar256/datadog-axi/internal/timeutil"
)

func TestComposeQueryUsesAuditFacetsAndQuotesValues(t *testing.T) {
	query := ComposeQuery(SearchParams{Query: "status:error", Actor: "alice@example.com", Service: "monitors", Action: "monitor.update", Resource: "CPU high", Tags: []string{"env:prod"}})
	for _, expected := range []string{"status:error", "@usr.email:alice@example.com", "service:monitors", "@evt.name:monitor.update", `@resource.name:"CPU high"`, "env:prod"} {
		if !strings.Contains(query, expected) {
			t.Fatalf("query %q missing %q", query, expected)
		}
	}
}

func TestComposeQueryQuotesSpecialFacetValues(t *testing.T) {
	query := ComposeQuery(SearchParams{Actor: `alice" OR *`})
	if query != `@usr.email:"alice\" OR *"` {
		t.Fatalf("unexpected escaped audit query: %q", query)
	}
}

func TestComposeQueryQuotesTagControlCharacters(t *testing.T) {
	query := ComposeQuery(SearchParams{Tags: []string{"env:prod\nstatus:error"}})
	if strings.ContainsAny(query, "\r\n") {
		t.Fatalf("tag value injected control characters: %q", query)
	}
	if query != `"env:prod\nstatus:error"` {
		t.Fatalf("unexpected escaped tag query: %q", query)
	}
}

func TestMapEvent(t *testing.T) {
	timestamp := time.Date(2026, 8, 4, 12, 0, 0, 0, time.FixedZone("CEST", 2*60*60))
	attrs := datadogV2.NewAuditLogsEventAttributes()
	attrs.SetTimestamp(timestamp)
	attrs.SetService("audit-service")
	attrs.SetMessage("Updated monitor")
	attrs.SetTags([]string{"env:prod"})
	attrs.SetAttributes(map[string]interface{}{"@evt.name": "monitor.update", "monitor_id": "123"})
	event := datadogV2.NewAuditLogsEvent()
	event.SetId("abc")
	event.SetAttributes(*attrs)

	view := mapEvent(*event)
	if view.ID != "abc" || view.Type != "audit" || view.Service != "audit-service" || view.Message != "Updated monitor" {
		t.Fatalf("unexpected audit mapping: %+v", view)
	}
	if view.Timestamp == nil || !view.Timestamp.Equal(timestamp.UTC()) {
		t.Fatalf("unexpected audit timestamp: %+v", view)
	}
	if view.Attributes["monitor_id"] != "123" {
		t.Fatalf("unexpected audit attributes: %+v", view.Attributes)
	}
}

func TestSearchUsesAuditEventsPostContract(t *testing.T) {
	from := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	seen := make(chan struct {
		method string
		path   string
		body   []byte
	}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		seen <- struct {
			method string
			path   string
			body   []byte
		}{method: r.Method, path: r.URL.Path, body: body}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"audit-1","type":"audit","attributes":{"timestamp":"2026-08-04T10:30:00Z","service":"monitors","message":"monitor updated","tags":["env:prod"],"attributes":{"actor":"alice@example.com","action":"monitor.update","resource":"CPU high","changes":{"query":"old"}}}}],"meta":{"page":{"after":"cursor-next"}}}`)
	}))
	defer server.Close()

	client := auditTestClient(t, server)
	result, err := searchWithClient(client, cliruntime.Config{}, SearchParams{
		Query:    "status:error",
		Range:    timeutil.Range{From: from, To: to},
		Limit:    25,
		SortAsc:  true,
		Cursor:   "cursor-prev",
		Actor:    "alice@example.com",
		Service:  "monitors",
		Action:   "monitor.update",
		Resource: "CPU high",
		Tags:     []string{"env:prod"},
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	req := <-seen
	if req.method != http.MethodPost || req.path != "/api/v2/audit/events/search" {
		t.Fatalf("unexpected audit request: %s %s", req.method, req.path)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(req.body, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	filter, ok := body["filter"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing filter in request: %#v", body)
	}
	if filter["query"] != `status:error @usr.email:alice@example.com service:monitors @evt.name:monitor.update @resource.name:"CPU high" env:prod` {
		t.Fatalf("unexpected query: %#v", filter["query"])
	}
	if filter["from"] != from.Format(time.RFC3339Nano) || filter["to"] != to.Format(time.RFC3339Nano) {
		t.Fatalf("unexpected time range: %#v", filter)
	}
	page, ok := body["page"].(map[string]interface{})
	if !ok || page["limit"] != float64(25) || page["cursor"] != "cursor-prev" {
		t.Fatalf("unexpected page: %#v", body["page"])
	}
	if body["sort"] != "timestamp" {
		t.Fatalf("unexpected sort: %#v", body["sort"])
	}
	if result.Count != 1 || result.NextCursor != "cursor-next" || !result.Truncated || result.Items[0].Actor != "alice@example.com" || result.Items[0].Action != "monitor.update" || !result.Items[0].MonitorDashboardChange {
		t.Fatalf("unexpected projected result: %+v", result)
	}
	if len(result.Items[0].ChangedFields) != 1 || result.Items[0].ChangedFields[0] != "query" {
		t.Fatalf("expected changed field projection: %+v", result.Items[0])
	}
}

func TestSearchSanitizesHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"errors":[{"detail":"secret diagnostic"}]}`)
	}))
	defer server.Close()

	client := auditTestClient(t, server)
	_, err := searchWithClient(client, cliruntime.Config{}, SearchParams{Range: timeutil.Range{From: time.Unix(1, 0), To: time.Unix(2, 0)}})
	if err == nil || !strings.Contains(err.Error(), "datadog API request failed") || strings.Contains(err.Error(), "secret diagnostic") {
		t.Fatalf("unexpected sanitized error: %v", err)
	}
}

func auditTestClient(t *testing.T, server *httptest.Server) *cliruntime.Client {
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
