package dashboard

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/nazar256/datadog-axi/internal/runtime"
)

var _ Updater = LiveService{}
var _ RawExporter = LiveService{}
var _ RawUpdater = LiveService{}

func TestMapDashboardSummaryAndDetail(t *testing.T) {
	created := time.Date(2026, 3, 21, 9, 0, 0, 0, time.UTC)
	modified := created.Add(time.Hour)
	title := "Ops dashboard"
	author := "ops@example.com"
	id := "abc-def"
	url := "/dashboard/abc-def"
	layout := datadogV1.DASHBOARDLAYOUTTYPE_ORDERED
	summary := datadogV1.DashboardSummaryDefinition{}
	summary.SetId(id)
	summary.SetTitle(title)
	summary.SetAuthorHandle(author)
	summary.SetUrl(url)
	summary.SetLayoutType(layout)
	summary.SetCreatedAt(created)
	summary.SetModifiedAt(modified)

	view := mapDashboardSummary(summary)
	if view.ID != id || view.CreatedAt == nil || view.ModifiedAt == nil {
		t.Fatalf("unexpected summary view: %+v", view)
	}

	detail := datadogV1.NewDashboard(layout, title, []datadogV1.Widget{})
	detail.SetId(id)
	detail.SetAuthorHandle(author)
	detail.SetUrl(url)
	detail.SetCreatedAt(created)
	detail.SetModifiedAt(modified)
	full := mapDashboardDetail(*detail)
	if full.ID != id || full.WidgetCount != 0 || full.CreatedAt == nil || full.ModifiedAt == nil {
		t.Fatalf("unexpected detail view: %+v", full)
	}
}

func TestDashboardUpdatePreservesUnknownFields(t *testing.T) {
	var dashboard datadogV1.Dashboard
	if err := json.Unmarshal([]byte(`{"id":"abc-def","title":"Ops dashboard","layout_type":"ordered","widgets":[],"future_field":{"enabled":true}}`), &dashboard); err != nil {
		t.Fatalf("unmarshal dashboard: %v", err)
	}

	dashboard.Title = "Updated dashboard"
	encoded, err := json.Marshal(dashboard)
	if err != nil {
		t.Fatalf("marshal dashboard: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode dashboard: %v", err)
	}
	if got, ok := payload["future_field"].(map[string]any); !ok || got["enabled"] != true {
		t.Fatalf("unknown field was not preserved: %s", encoded)
	}
	if payload["title"] != "Updated dashboard" {
		t.Fatalf("known update field was not sent: %s", encoded)
	}
}

type dashboardRedirectTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t dashboardRedirectTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	urlCopy := *request.URL
	clone.URL = &urlCopy
	clone.URL.Scheme = t.target.Scheme
	clone.URL.Host = t.target.Host
	clone.Host = t.target.Host
	return t.base.RoundTrip(clone)
}

func TestLiveServiceRawExportAndUpdateUseOfficialDashboardEndpoints(t *testing.T) {
	var updatePayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/dashboard/abc-def" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":"abc-def","title":"Ops dashboard","layout_type":"ordered","widgets":[],"future_field":{"enabled":true}}`))
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
			_, _ = w.Write([]byte(`{"id":"abc-def","title":"Updated dashboard","layout_type":"ordered","widgets":[],"future_field":{"enabled":true}}`))
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
	http.DefaultTransport = dashboardRedirectTransport{target: target, base: previous}
	t.Cleanup(func() { http.DefaultTransport = previous })

	cfg := runtime.Config{Site: runtime.DefaultSite, Timeout: 5 * time.Second, APIKey: "api", AppKey: "app"}
	service := LiveService{}
	exported, err := service.ExportRaw(context.Background(), cfg, "abc-def")
	if err != nil {
		t.Fatalf("export raw: %v", err)
	}
	if future, ok := exported["future_field"].(map[string]any); !ok || future["enabled"] != true {
		t.Fatalf("export did not preserve unknown field: %#v", exported)
	}
	updated, err := service.UpdateRaw(context.Background(), cfg, "abc-def", map[string]any{
		"id":           "abc-def",
		"title":        "Updated dashboard",
		"layout_type":  "ordered",
		"widgets":      []any{},
		"future_field": map[string]any{"enabled": true},
	})
	if err != nil {
		t.Fatalf("update raw: %v", err)
	}
	if updatePayload["title"] != "Updated dashboard" {
		t.Fatalf("update payload omitted known field: %#v", updatePayload)
	}
	if future, ok := updatePayload["future_field"].(map[string]any); !ok || future["enabled"] != true {
		t.Fatalf("update payload omitted unknown field: %#v", updatePayload)
	}
	if future, ok := updated["future_field"].(map[string]any); !ok || future["enabled"] != true {
		t.Fatalf("update response did not preserve unknown field: %#v", updated)
	}
}

func TestLiveServiceListFiltersReturnedPageAndMarksIncomplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/v1/dashboard" {
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
			return
		}
		query := request.URL.Query()
		if query.Get("count") != "2" || query.Get("start") != "1" {
			http.Error(w, "unexpected pagination", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"dashboards":[{"id":"dash-1","title":"Ops overview","layout_type":"ordered","author_handle":"ops@example.com"},{"id":"dash-2","title":"Finance overview","layout_type":"free","author_handle":"finance@example.com"}]}`))
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultTransport
	http.DefaultTransport = dashboardRedirectTransport{target: target, base: previous}
	t.Cleanup(func() { http.DefaultTransport = previous })

	cfg := runtime.Config{Site: runtime.DefaultSite, Timeout: 5 * time.Second, APIKey: "api", AppKey: "app"}
	result, err := (LiveService{}).List(context.Background(), cfg, ListParams{Count: 2, Start: 1, Filter: "ops"})
	if err != nil {
		t.Fatalf("dashboard list: %v", err)
	}
	if result.Count != 1 || len(result.Items) != 1 || result.Items[0].ID != "dash-1" {
		t.Fatalf("unexpected filtered dashboards: %+v", result)
	}
	if result.Filter != "ops" || result.FilterScope != "page" || !result.PossiblyIncomplete {
		t.Fatalf("missing filter metadata: %+v", result)
	}
}

func TestLiveServiceListFilterIsPageLocal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/api/v1/dashboard" {
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
			return
		}
		query := request.URL.Query()
		if query.Get("count") != "2" || query.Get("start") != "4" {
			http.Error(w, "unexpected query", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"dashboards":[{"id":"ops-1","title":"Operations","layout_type":"ordered","author_handle":"ops@example.com"},{"id":"sales-1","title":"Sales","layout_type":"free","author_handle":"sales@example.com"}]}`))
	}))
	defer server.Close()
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultTransport
	http.DefaultTransport = dashboardRedirectTransport{target: target, base: previous}
	t.Cleanup(func() { http.DefaultTransport = previous })

	cfg := runtime.Config{Site: runtime.DefaultSite, Timeout: 5 * time.Second, APIKey: "api", AppKey: "app"}
	result, err := (LiveService{}).List(context.Background(), cfg, ListParams{Count: 2, Start: 4, Filter: "ops"})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if result.Filter != "ops" || result.FilterScope != "page" || !result.PossiblyIncomplete || result.Count != 1 || len(result.Items) != 1 || result.Items[0].ID != "ops-1" {
		t.Fatalf("unexpected filtered result: %#v", result)
	}
}

func TestMapDashboardDetailRetainsWidgetsAndRawFields(t *testing.T) {
	detail := datadogV1.NewDashboard(datadogV1.DASHBOARDLAYOUTTYPE_ORDERED, "Ops", []datadogV1.Widget{})
	detail.SetId("ops-1")
	detail.SetAuthorHandle("ops@example.com")
	detail.SetUrl("/dashboard/ops-1")
	detail.SetTags([]string{"team:platform"})
	detail.AdditionalProperties = map[string]interface{}{"future_field": map[string]interface{}{"enabled": true}}
	view := mapDashboardDetail(*detail)
	if len(view.Widgets) != 0 || len(view.Tags) != 1 || view.Raw == nil {
		t.Fatalf("detail did not retain full projection: %#v", view)
	}
	if future, ok := view.Raw["future_field"].(map[string]any); !ok || future["enabled"] != true {
		t.Fatalf("detail raw field was not preserved: %#v", view.Raw)
	}
}
