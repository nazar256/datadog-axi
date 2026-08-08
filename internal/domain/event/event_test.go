package event

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
	"github.com/nazar256/datadog-axi/internal/timeutil"
)

func TestMapEvent(t *testing.T) {
	id := int64(42)
	title := "Deployment completed"
	text := "deployed"
	source := "deploy"
	item := datadogV1.Event{Id: &id, Title: &title, Text: &text, SourceTypeName: &source, Tags: []string{"env:prod"}}
	item.SetDateHappened(time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC).Unix())
	got := mapEvent(item)
	if got.ID != 42 || got.Title != title || got.Source != "deploy" || got.Timestamp == nil || got.Timestamp.Year() != 2026 {
		t.Fatalf("unexpected event mapping: %+v", got)
	}
}

func TestMapEventDetailPreservesV1Fields(t *testing.T) {
	id := int64(42)
	status := "ok"
	alertType := datadogV1.EVENTALERTTYPE_SUCCESS
	device := "laptop"
	host := "web-01"
	payload := `{"release":"2026.08.04"}`
	event := datadogV1.Event{Id: &id, AlertType: &alertType, DeviceName: &device, Host: &host, Payload: &payload}
	response := datadogV1.EventResponse{Event: &event, Status: &status}

	got := mapEventDetail(response)
	if got.ID != id || got.Status != status || got.AlertType != string(alertType) || got.DeviceName != device || got.Host != host || got.Payload != payload {
		t.Fatalf("unexpected event detail mapping: %+v", got)
	}
}

func TestListUsesEventsQueryContractAndLimit(t *testing.T) {
	from := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	seen := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"ok","events":[{"id":1,"title":"first","text":"one","source_type_name":"deploy","date_happened":1785837600},{"id":2,"title":"second","text":"two","source_type_name":"deploy","date_happened":1785837660}]}`)
	}))
	defer server.Close()

	client := eventTestClient(t, server)
	result, err := listWithClient(client, cliruntime.Config{}, ListParams{Range: timeutil.Range{From: from, To: to}, Sources: "deploy", Tags: "env:prod", Page: 2, Limit: 1, Unaggregated: true, ExcludeAggregate: true})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	req := <-seen
	if req.Method != http.MethodGet || req.URL.Path != "/api/v1/events" {
		t.Fatalf("unexpected events request: %s %s", req.Method, req.URL.Path)
	}
	query := req.URL.Query()
	if query.Get("start") != "1785837600" || query.Get("end") != "1785841200" || query.Get("sources") != "deploy" || query.Get("tags") != "env:prod" || query.Get("page") != "2" || query.Get("unaggregated") != "true" || query.Get("exclude_aggregate") != "true" {
		t.Fatalf("unexpected event query: %v", query)
	}
	if result.Count != 1 || result.Items[0].ID != 1 {
		t.Fatalf("expected adapter limit to trim response: %+v", result)
	}
}

func TestGetUsesEventDetailEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/events/42" {
			t.Fatalf("unexpected event detail request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"ok","event":{"id":42,"title":"deploy","text":"done","source_type_name":"deploy","alert_type":"success","device_name":"laptop","host":"web-01","payload":"{\"release\":\"2026.08.04\"}","date_happened":1785840000}}`)
	}))
	defer server.Close()

	client := eventTestClient(t, server)
	detail, err := getWithClient(client, cliruntime.Config{}, 42)
	if err != nil {
		t.Fatalf("get event: %v", err)
	}
	if detail.ID != 42 || detail.Status != "ok" || detail.Host != "web-01" || detail.Payload == "" {
		t.Fatalf("unexpected event detail: %+v", detail)
	}
}

func eventTestClient(t *testing.T, server *httptest.Server) *cliruntime.Client {
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
