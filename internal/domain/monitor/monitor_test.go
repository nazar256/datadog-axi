package monitor

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

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/nazar256/datadog-axi/internal/runtime"
)

var _ Updater = LiveService{}
var _ RawExporter = LiveService{}
var _ RawUpdater = LiveService{}
var _ Validator = LiveService{}

func TestMapMonitor(t *testing.T) {
	created := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	modified := created.Add(time.Hour)
	priority := int64(2)
	state := datadogV1.MONITOROVERALLSTATES_ALERT
	monitorType := datadogV1.MONITORTYPE_QUERY_ALERT
	item := datadogV1.Monitor{Query: "avg:test{*} > 1", Type: monitorType, Tags: []string{"env:prod"}}
	item.SetId(123)
	item.SetName("CPU high")
	item.SetMessage("Investigate")
	item.SetCreated(created)
	item.SetModified(modified)
	item.SetOverallState(state)
	item.Priority.Set(&priority)

	view := mapMonitor(item)
	if view.ID != 123 || view.Name != "CPU high" || view.State != "Alert" {
		t.Fatalf("unexpected view: %+v", view)
	}
	if view.Priority == nil || *view.Priority != 2 {
		t.Fatalf("unexpected priority: %+v", view.Priority)
	}
	if view.CreatedAt == nil || view.ModifiedAt == nil {
		t.Fatalf("expected timestamps to be set: %+v", view)
	}
}

func TestMonitorUpdateRequestPreservesUnknownFields(t *testing.T) {
	var request datadogV1.MonitorUpdateRequest
	if err := json.Unmarshal([]byte(`{"id":123,"name":"CPU high","query":"avg:test{*} > 1","type":"query alert","options":{"thresholds":{"critical":1}},"future_field":{"enabled":true}}`), &request); err != nil {
		t.Fatalf("unmarshal update request: %v", err)
	}

	request.SetMessage("Investigate")
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal update request: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode update request: %v", err)
	}
	if got, ok := payload["future_field"].(map[string]any); !ok || got["enabled"] != true {
		t.Fatalf("unknown field was not preserved: %s", encoded)
	}
	if payload["message"] != "Investigate" {
		t.Fatalf("known update field was not sent: %s", encoded)
	}
}

type monitorRedirectTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t monitorRedirectTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL = cloneURL(request.URL)
	clone.URL.Scheme = t.target.Scheme
	clone.URL.Host = t.target.Host
	clone.Host = t.target.Host
	return t.base.RoundTrip(clone)
}

func cloneURL(value *url.URL) *url.URL {
	clone := *value
	return &clone
}

func TestLiveServiceRawExportAndUpdateUseOfficialMonitorEndpoints(t *testing.T) {
	var updatePayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/monitor/123" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":123,"name":"CPU high","type":"query alert","query":"avg:test{*} > 1","future_field":{"enabled":true}}`))
		case http.MethodPut:
			body, err := io.ReadAll(request.Body)
			if err != nil {
				http.Error(w, "read body", http.StatusBadRequest)
				return
			}
			if err := json.Unmarshal(body, &updatePayload); err != nil {
				http.Error(w, "invalid JSON", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"id":123,"name":"CPU high","type":"query alert","query":"avg:test{*} > 1","message":"Investigate","future_field":{"enabled":true}}`))
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultTransport
	http.DefaultTransport = monitorRedirectTransport{target: target, base: previous}
	t.Cleanup(func() { http.DefaultTransport = previous })

	cfg := runtime.Config{Site: runtime.DefaultSite, Timeout: 5 * time.Second, APIKey: "api", AppKey: "app"}
	service := LiveService{}
	exported, err := service.ExportRaw(context.Background(), cfg, 123)
	if err != nil {
		t.Fatalf("export raw: %v", err)
	}
	if future, ok := exported["future_field"].(map[string]any); !ok || future["enabled"] != true {
		t.Fatalf("export did not preserve unknown field: %#v", exported)
	}
	updated, err := service.UpdateRaw(context.Background(), cfg, 123, map[string]any{
		"id":           float64(123),
		"name":         "CPU high",
		"type":         "query alert",
		"query":        "avg:test{*} > 1",
		"message":      "Investigate",
		"future_field": map[string]any{"enabled": true},
	})
	if err != nil {
		t.Fatalf("update raw: %v", err)
	}
	if updatePayload["message"] != "Investigate" {
		t.Fatalf("update payload omitted known field: %#v", updatePayload)
	}
	if future, ok := updatePayload["future_field"].(map[string]any); !ok || future["enabled"] != true {
		t.Fatalf("update payload omitted unknown field: %#v", updatePayload)
	}
	if future, ok := updated["future_field"].(map[string]any); !ok || future["enabled"] != true {
		t.Fatalf("update response did not preserve unknown field: %#v", updated)
	}
}

func TestLiveServiceSearchUsesOfficialMonitorSearchEndpointAndMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/v1/monitor/search" {
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
			return
		}
		query := request.URL.Query()
		for key, want := range map[string]string{"query": "service:web", "page": "2", "per_page": "25", "sort": "name"} {
			if query.Get(key) != want {
				http.Error(w, "missing query parameter", http.StatusBadRequest)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"monitors":[{"id":123,"name":"CPU high","query":"avg:test{*} > 1","type":"query alert","status":"Alert","tags":["env:prod"],"classification":"custom","metrics":["test.metric"],"quality_issues":["missing_owner"],"scopes":["*"],"last_triggered_ts":1700000000}],"metadata":{"page":2,"page_count":4,"per_page":25,"total_count":88}}`))
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultTransport
	http.DefaultTransport = monitorRedirectTransport{target: target, base: previous}
	t.Cleanup(func() { http.DefaultTransport = previous })

	cfg := runtime.Config{Site: runtime.DefaultSite, Timeout: 5 * time.Second, APIKey: "api", AppKey: "app"}
	result, err := (LiveService{}).Search(context.Background(), cfg, SearchParams{Query: "service:web", Page: 2, PerPage: 25, Sort: "name"})
	if err != nil {
		t.Fatalf("monitor search: %v", err)
	}
	if result.Count != 1 || result.Page != 2 || result.PageCount != 4 || result.PerPage != 25 || result.TotalCount != 88 {
		t.Fatalf("unexpected search metadata: %+v", result)
	}
	if result.Endpoint != "GET /api/v1/monitor/search" || result.Items[0].ID != 123 || result.Items[0].Classification != "custom" {
		t.Fatalf("unexpected search result: %+v", result)
	}
}

func TestLiveServiceValidateExistingUsesOfficialMonitorEndpointAndPayload(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/monitor/123/validate" {
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
			return
		}
		body, err := io.ReadAll(request.Body)
		if err != nil || json.Unmarshal(body, &payload) != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"valid":true,"warnings":[]}`))
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultTransport
	http.DefaultTransport = monitorRedirectTransport{target: target, base: previous}
	t.Cleanup(func() { http.DefaultTransport = previous })

	cfg := runtime.Config{Site: runtime.DefaultSite, Timeout: 5 * time.Second, APIKey: "api", AppKey: "app"}
	response, err := (LiveService{}).ValidateExisting(context.Background(), cfg, 123, map[string]any{
		"id":           float64(123),
		"name":         "CPU high",
		"type":         "query alert",
		"query":        "avg:test{*} > 1",
		"future_field": map[string]any{"enabled": true},
	})
	if err != nil {
		t.Fatalf("validate existing: %v", err)
	}
	if payload["name"] != "CPU high" {
		t.Fatalf("validation payload omitted known field: %#v", payload)
	}
	if future, ok := payload["future_field"].(map[string]any); !ok || future["enabled"] != true {
		t.Fatalf("validation payload omitted unknown field: %#v", payload)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("encode validation response: %v", err)
	}
	if !strings.Contains(string(encoded), `"valid":true`) {
		t.Fatalf("unexpected validation response: %s", encoded)
	}
}

func TestLiveServiceValidateUsesOfficialMonitorEndpointAndPayload(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/monitor/validate" {
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
			return
		}
		body, err := io.ReadAll(request.Body)
		if err != nil || json.Unmarshal(body, &payload) != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"valid":true,"warnings":[]}`))
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultTransport
	http.DefaultTransport = monitorRedirectTransport{target: target, base: previous}
	t.Cleanup(func() { http.DefaultTransport = previous })

	cfg := runtime.Config{Site: runtime.DefaultSite, Timeout: 5 * time.Second, APIKey: "api", AppKey: "app"}
	response, err := (LiveService{}).Validate(context.Background(), cfg, map[string]any{
		"id":           float64(123),
		"name":         "CPU high",
		"type":         "query alert",
		"query":        "avg:test{*} > 1",
		"future_field": map[string]any{"enabled": true},
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if payload["name"] != "CPU high" {
		t.Fatalf("validation payload omitted known field: %#v", payload)
	}
	if future, ok := payload["future_field"].(map[string]any); !ok || future["enabled"] != true {
		t.Fatalf("validation payload omitted unknown field: %#v", payload)
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("encode validation response: %v", err)
	}
	if !strings.Contains(string(encoded), `"valid":true`) {
		t.Fatalf("unexpected validation response: %s", encoded)
	}
}

func TestMonitorURLMapsAPISitesToApplicationHosts(t *testing.T) {
	for _, test := range []struct {
		site string
		want string
	}{
		{"datadoghq.com", "https://app.datadoghq.com/monitors/123"},
		{"datadoghq.eu", "https://app.datadoghq.eu/monitors/123"},
		{"us3.datadoghq.com", "https://us3.datadoghq.com/monitors/123"},
		{"us5.datadoghq.com", "https://us5.datadoghq.com/monitors/123"},
		{"ap1.datadoghq.com", "https://ap1.datadoghq.com/monitors/123"},
		{"ap2.datadoghq.com", "https://ap2.datadoghq.com/monitors/123"},
		{"ddog-gov.com", "https://app.ddog-gov.com/monitors/123"},
	} {
		if got := monitorURL(test.site, 123); got != test.want {
			t.Errorf("site %q: got %q, want %q", test.site, got, test.want)
		}
	}
}
