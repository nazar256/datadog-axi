package downtime

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
)

func TestMapIncludedPreservesRelatedResourceShape(t *testing.T) {
	items := mapIncluded([]datadogV2.DowntimeResponseIncludedItem{{
		UnparsedObject: map[string]any{"id": "monitor-1", "type": "monitors"},
	}})
	if len(items) != 1 || items[0]["id"] != "monitor-1" || items[0]["type"] != "monitors" {
		t.Fatalf("unexpected included resources: %#v", items)
	}
}

func TestMapDowntimeDetailPreservesScheduleAndRelationships(t *testing.T) {
	id := "downtime-1"
	scope := "env:prod"
	message := "maintenance"
	status := datadogV2.DOWNTIMESTATUS_ACTIVE
	created := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	canceled := created.Add(time.Hour)
	attrs := datadogV2.DowntimeResponseAttributes{Scope: &scope, Status: &status, Created: &created}
	attrs.SetMessage(message)
	attrs.SetCanceled(canceled)
	attrs.SetDisplayTimezone("Europe/Amsterdam")
	attrs.SetMuteFirstRecoveryNotification(true)
	attrs.SetNotifyEndStates([]datadogV2.DowntimeNotifyEndStateTypes{datadogV2.DOWNTIMENOTIFYENDSTATETYPES_ALERT})
	attrs.SetNotifyEndTypes([]datadogV2.DowntimeNotifyEndStateActions{datadogV2.DOWNTIMENOTIFYENDSTATEACTIONS_EXPIRED})
	monitor := datadogV2.NewDowntimeMonitorIdentifierId(123)
	attrs.SetMonitorIdentifier(datadogV2.DowntimeMonitorIdentifierIdAsDowntimeMonitorIdentifier(monitor))
	attrs.SetSchedule(datadogV2.DowntimeScheduleResponse{UnparsedObject: map[string]any{"start": "immediately"}})
	data := datadogV2.DowntimeResponseData{Id: &id, Attributes: &attrs, Relationships: &datadogV2.DowntimeRelationships{UnparsedObject: map[string]any{"monitor": map[string]any{"data": map[string]any{"id": "123"}}}}}
	response := datadogV2.DowntimeResponse{Data: &data, Included: []datadogV2.DowntimeResponseIncludedItem{{UnparsedObject: map[string]any{"id": "monitor-1", "type": "monitors"}}}}

	got := mapDowntimeDetail(response)
	if got.ID != id || got.Status != string(status) || got.Scope != scope || got.Message != message || got.DisplayTimezone != "Europe/Amsterdam" {
		t.Fatalf("unexpected downtime detail mapping: %+v", got)
	}
	if got.Canceled == nil || !got.Canceled.Equal(canceled) || got.MonitorID == nil || *got.MonitorID != 123 {
		t.Fatalf("expected cancellation and monitor id, got: %+v", got)
	}
	if len(got.NotifyEndStates) != 1 || got.NotifyEndStates[0] != "alert" || len(got.NotifyEndTypes) != 1 || got.NotifyEndTypes[0] != "expired" {
		t.Fatalf("unexpected notification mapping: %+v", got)
	}
	if got.Schedule["start"] != "immediately" || got.Relationships["monitor"] == nil || len(got.Included) != 1 {
		t.Fatalf("unexpected related data mapping: %+v", got)
	}
}

func TestListUsesDowntimeQueryContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2/downtime" {
			t.Fatalf("unexpected downtime request: %s %s", r.Method, r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("current_only") != "true" || query.Get("include") != "monitors" || query.Get("page[offset]") != "25" || query.Get("page[limit]") != "10" {
			t.Fatalf("unexpected downtime query: %v", query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"d-1","type":"downtime","attributes":{"status":"active","scope":"env:prod","message":"maintenance","created":"2026-08-04T10:00:00Z"}}],"meta":{"page":{"total_filtered_count":3}},"included":[{"id":"m-1","type":"monitor"}]}`)
	}))
	defer server.Close()

	client := downtimeTestClient(t, server)
	result, err := listWithClient(client, cliruntime.Config{}, ListParams{CurrentOnly: true, Include: "monitors", Offset: 25, Limit: 10})
	if err != nil {
		t.Fatalf("list downtimes: %v", err)
	}
	if result.Count != 1 || result.Items[0].ID != "d-1" || len(result.Included) != 1 || result.TotalFiltered != 3 || !result.PossiblyTruncated {
		t.Fatalf("unexpected downtime list: %+v", result)
	}
}

func TestGetUsesDowntimeDetailEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2/downtime/d-1" {
			t.Fatalf("unexpected downtime detail request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"d-1","type":"downtime","attributes":{"status":"active","scope":"env:prod","message":"maintenance","display_timezone":"Europe/Amsterdam","mute_first_recovery_notification":true,"notify_end_states":["alert"],"notify_end_types":["expired"]}}}`)
	}))
	defer server.Close()

	client := downtimeTestClient(t, server)
	detail, err := getWithClient(client, cliruntime.Config{}, "d-1")
	if err != nil {
		t.Fatalf("get downtime: %v", err)
	}
	if detail.ID != "d-1" || detail.Scope != "env:prod" || detail.DisplayTimezone != "Europe/Amsterdam" || !detail.MuteFirstRecoveryNotification {
		t.Fatalf("unexpected downtime detail: %+v", detail)
	}
}

func downtimeTestClient(t *testing.T, server *httptest.Server) *cliruntime.Client {
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
