package slo

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

func TestMapSummary(t *testing.T) {
	created := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	modified := created.Add(time.Hour)
	target := 99.9
	id := "slo-123"
	timeframe := datadogV1.SLOTIMEFRAME_SEVEN_DAYS
	item := datadogV1.ServiceLevelObjective{
		Id:              &id,
		Name:            "API availability",
		Type:            datadogV1.SLOTYPE_MONITOR,
		Timeframe:       &timeframe,
		TargetThreshold: &target,
		Tags:            []string{"env:prod"},
	}
	item.SetCreatedAt(created.Unix())
	item.SetModifiedAt(modified.Unix())

	view := mapSummary(item)
	if view.ID != "slo-123" || view.Name != "API availability" || view.Type != "monitor" || view.Timeframe != "7d" {
		t.Fatalf("unexpected view: %+v", view)
	}
	if view.TargetThreshold == nil || *view.TargetThreshold != target {
		t.Fatalf("unexpected target threshold: %+v", view.TargetThreshold)
	}
	if view.CreatedAt == nil || !view.CreatedAt.Equal(created) || view.ModifiedAt == nil || !view.ModifiedAt.Equal(modified) {
		t.Fatalf("unexpected timestamps: %+v", view)
	}
}

type sloRedirectTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (t sloRedirectTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	urlCopy := *request.URL
	clone.URL = &urlCopy
	clone.URL.Scheme = t.target.Scheme
	clone.URL.Host = t.target.Host
	clone.Host = t.target.Host
	return t.base.RoundTrip(clone)
}

func withSLOTestServer(t *testing.T, handler http.Handler) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	previous := http.DefaultTransport
	http.DefaultTransport = sloRedirectTransport{target: target, base: previous}
	t.Cleanup(func() { http.DefaultTransport = previous })
}

func TestSearchMapsStatusErrorBudgetAndPagination(t *testing.T) {
	var query url.Values
	withSLOTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/slo/search" || request.Method != http.MethodGet {
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
			return
		}
		query = request.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"attributes":{"slos":[{"data":{"id":"slo-123","type":"slo","attributes":{"name":"API availability","slo_type":"monitor","monitor_ids":[42],"thresholds":[{"target":99.9,"timeframe":"7d"}],"status":{"state":"ok","sli":99.95,"error_budget_remaining":0.55,"raw_error_budget_remaining":{"unit":"minutes","value":12.5},"calculation_error":null,"indexed_at":1710000000},"overall_status":[{"timeframe":"7d","state":"ok","status":99.95,"target":99.9,"error_budget_remaining":0.55}]}}}]}} ,"meta":{"pagination":{"number":2,"size":1,"total":3,"next_number":3,"prev_number":1}}}`)
	}))

	result, err := (LiveService{}).Search(context.Background(), runtime.Config{Site: runtime.DefaultSite, Timeout: time.Second, APIKey: "api", AppKey: "app"}, SearchParams{Query: "availability", PageSize: 1, PageNumber: 2})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if query.Get("include_facets") != "true" || query.Get("query") != "availability" || query.Get("page[size]") != "1" || query.Get("page[number]") != "2" {
		t.Fatalf("unexpected SearchSLO query: %v", query)
	}
	if result.Count != 1 || result.Total == nil || *result.Total != 3 || result.NextPage == nil || *result.NextPage != 3 {
		t.Fatalf("unexpected pagination: %+v", result)
	}
	item := result.Items[0]
	if item.State != "ok" || item.StatusAvailability != "available" || item.SLI == nil || *item.SLI != 99.95 || item.ErrorBudgetRemaining == nil || *item.ErrorBudgetRemaining != 0.55 || len(item.MonitorIDs) != 1 || item.MonitorIDs[0] != 42 {
		t.Fatalf("status context was not mapped: %+v", item)
	}
	if !strings.Contains(string(mustJSON(item.RawErrorBudget)), "minutes") {
		t.Fatalf("raw error budget was not preserved: %#v", item.RawErrorBudget)
	}
}

func TestSearchMissingStatusRemainsUnavailable(t *testing.T) {
	withSLOTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"attributes":{"slos":[{"data":{"id":"slo-123","type":"slo","attributes":{"name":"No status"}}}]}}}`)
	}))
	result, err := (LiveService{}).Search(context.Background(), runtime.Config{Site: runtime.DefaultSite, Timeout: time.Second, APIKey: "api", AppKey: "app"}, SearchParams{})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].StatusAvailability != "unavailable" || result.Items[0].State != "" || result.Items[0].SLI != nil || result.Items[0].ErrorBudgetRemaining != nil {
		t.Fatalf("missing status should remain unavailable: %+v", result.Items)
	}
}

func TestGetWithHistoryUsesBoundedHistoryQueryAndPreservesErrors(t *testing.T) {
	var historyQuery url.Values
	var historyCalls int
	withSLOTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/api/v1/slo/slo-123":
			_, _ = io.WriteString(w, `{"data":{"id":"slo-123","name":"API availability","type":"monitor","timeframe":"7d","target_threshold":99.9,"monitor_ids":[42],"thresholds":[{"target":99.9,"timeframe":"7d"}]}}`)
		case request.URL.Path == "/api/v1/slo/slo-123/history":
			historyCalls++
			historyQuery = request.URL.Query()
			_, _ = io.WriteString(w, `{"data":{"from_ts":1710000000,"to_ts":1710604800,"type":"monitor","overall":{"sli_value":99.95,"error_budget_remaining":{"7d":0.55},"history":[[1710000000,0],[1710604800,0]],"errors":[{"error_type":"partial","error_message":"one monitor unavailable"}]},"monitors":[{"name":"API monitor","sli_value":99.95,"errors":[{"error_type":"monitor","error_message":"partial data"}]}]},"errors":[{"error":"history partially unavailable"}]}`)
		default:
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
		}
	}))

	from := time.Unix(1710000000, 0).UTC()
	to := time.Unix(1710604800, 0).UTC()
	detail, err := (LiveService{}).GetWithHistory(context.Background(), runtime.Config{Site: runtime.DefaultSite, Timeout: time.Second, APIKey: "api", AppKey: "app"}, "slo-123", HistoryParams{Requested: true, From: from, To: to})
	if err != nil {
		t.Fatalf("get with history: %v", err)
	}
	if historyCalls != 1 || historyQuery.Get("from_ts") != "1710000000" || historyQuery.Get("to_ts") != "1710604800" {
		t.Fatalf("unexpected history request: calls=%d query=%v", historyCalls, historyQuery)
	}
	if detail.History == nil || detail.History.Overall == nil || detail.History.Overall.SLI == nil || *detail.History.Overall.SLI != 99.95 {
		t.Fatalf("history overall was not mapped: %+v", detail.History)
	}
	if len(detail.History.CalculationErrors) < 2 || len(detail.History.Errors) != 1 {
		t.Fatalf("partial/calculation errors were lost: %+v", detail.History)
	}
	if detail.History.BurnRate.Status != "unavailable" || detail.History.BurnRate.Source != "derived" {
		t.Fatalf("burn rate should be explicitly unavailable: %+v", detail.History.BurnRate)
	}
}

func TestGetWithHistoryRejectsUnboundedWindow(t *testing.T) {
	from := time.Unix(1710000000, 0).UTC()
	to := from.Add(MaxHistoryWindow + time.Second)
	_, err := (LiveService{}).GetWithHistory(context.Background(), runtime.Config{}, "slo-123", HistoryParams{Requested: true, From: from, To: to})
	if err == nil || !strings.Contains(err.Error(), "cannot exceed") {
		t.Fatalf("expected bounded history error, got %v", err)
	}
}

func TestGetDoesNotFetchHistoryWithoutRequest(t *testing.T) {
	historyCalls := 0
	withSLOTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/v1/slo/slo-123/history" {
			historyCalls++
			http.Error(w, "history should not be requested", http.StatusInternalServerError)
			return
		}
		if request.URL.Path != "/api/v1/slo/slo-123" {
			http.Error(w, "unexpected endpoint", http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"id":"slo-123","name":"API availability","type":"monitor","timeframe":"7d","target_threshold":99.9}}`)
	}))
	if _, err := (LiveService{}).Get(context.Background(), runtime.Config{Site: runtime.DefaultSite, Timeout: time.Second, APIKey: "api", AppKey: "app"}, "slo-123"); err != nil {
		t.Fatalf("get: %v", err)
	}
	if historyCalls != 0 {
		t.Fatalf("unexpected history calls: %d", historyCalls)
	}
}

func TestMapHistoryRetainsMetricEventSeries(t *testing.T) {
	var response datadogV1.SLOHistoryResponse
	if err := json.Unmarshal([]byte(`{"data":{"type":"metric","series":{"times":[1710000000,1710003600],"interval":3600,"query":"sum:requests"},"overall":{"sli_value":99.9,"error_budget_remaining":{"7d":0.4}}}}`), &response); err != nil {
		t.Fatalf("decode history response: %v", err)
	}
	from := time.Unix(1710000000, 0).UTC()
	to := time.Unix(1710003600, 0).UTC()
	history := mapHistory(response, from, to)
	if history == nil || history.Type != "metric" || history.Series == nil {
		t.Fatalf("metric/event series was not retained: %+v", history)
	}
	if history.Overall == nil || history.Overall.ErrorBudgetRemaining["7d"] != 0.4 {
		t.Fatalf("metric/event overall context was not mapped: %+v", history.Overall)
	}
}

func TestDeriveBurnRateRequiresTwoValidObservations(t *testing.T) {
	unavailable := deriveBurnRate([]BudgetObservation{{At: time.Unix(1, 0), Remaining: 0.5}})
	if unavailable.Status != "unavailable" || unavailable.Value != nil {
		t.Fatalf("expected unavailable burn rate: %+v", unavailable)
	}
	available := deriveBurnRate([]BudgetObservation{{At: time.Unix(1, 0), Remaining: 1}, {At: time.Unix(3601, 0), Remaining: 0.5}})
	if available.Status != "available" || available.Value == nil || *available.Value <= 0 {
		t.Fatalf("expected derived burn rate: %+v", available)
	}
}

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}
