package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/nazar256/datadog-axi/internal/domain/dashboard"
	"github.com/nazar256/datadog-axi/internal/domain/monitor"
	"github.com/nazar256/datadog-axi/internal/runtime"
)

func TestSemanticDiffRedactsSecretsAndSortsPaths(t *testing.T) {
	expected := map[string]any{"name": "new", "options": map[string]any{"threshold": 2, "api_key": "do-not-compare"}, "query": "avg:new"}
	actual := map[string]any{"name": "old", "options": map[string]any{"threshold": 1, "api_key": "different"}, "query": "avg:old"}
	got := semanticDiff(expected, actual, "")
	want := []string{"name", "options.api_key", "options.threshold", "query"}
	if len(got) != len(want) {
		t.Fatalf("diff = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i].Path != want[i] {
			t.Fatalf("diff = %#v, want %#v", got, want)
		}
	}
	for _, change := range got {
		if change.Path == "options.threshold" && (change.Before != 1 || change.After != 2) {
			t.Fatalf("unexpected threshold values: %#v", change)
		}
	}
	if got := semanticDiff(map[string]any{"api_key": "one"}, map[string]any{"api_key": "two"}, ""); got[0].Before != "[REDACTED]" || got[0].After != "[REDACTED]" {
		t.Fatalf("secret values leaked: %#v", got)
	}
	nested := semanticDiff(map[string]any{"secret": map[string]any{"value": "new"}, "headers": []any{map[string]any{"authorization": "new"}}}, map[string]any{"secret": map[string]any{"value": "old"}, "headers": []any{map[string]any{"authorization": "old"}}}, "")
	for _, change := range nested {
		if change.Before != "[REDACTED]" || change.After != "[REDACTED]" {
			t.Fatalf("nested secret values leaked: %#v", nested)
		}
	}
	if got := semanticDiff(map[string]any{"message": "callback token=secret-value"}, map[string]any{"message": "callback token=old-value"}, ""); got[0].Before != "[REDACTED]" || got[0].After != "[REDACTED]" {
		t.Fatalf("token-bearing string leaked: %#v", got)
	}
}

func TestUpdateRejectsConflictingApplyAndDryRun(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{})
	cmd.SetIn(strings.NewReader(`{"id":123,"name":"CPU","type":"query alert","query":"avg:test{*} > 1"}`))
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"monitor", "update", "123", "-", "--apply", "--dry-run"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("expected conflicting update flags error, got %v", err)
	}
}

func TestFingerprintJSONIsCanonical(t *testing.T) {
	a := fingerprintJSON(map[string]any{"b": 2, "a": 1})
	b := fingerprintJSON(map[string]any{"a": 1, "b": 2})
	if a != b || len(a) != 64 || strings.Trim(a, "0123456789abcdef") != "" {
		t.Fatalf("unexpected fingerprints: %q %q", a, b)
	}
}

type guardedMonitorService struct {
	live           map[string]any
	persist        bool
	updateErr      error
	invalidRefetch bool
	exports        int
	updates        int
}

type validatingMonitorService struct {
	guardedMonitorService
	validated int
}

func (s *guardedMonitorService) List(context.Context, runtime.Config, monitor.ListParams) (monitor.ListResult, error) {
	return monitor.ListResult{}, nil
}

func (s *guardedMonitorService) Get(context.Context, runtime.Config, int64) (monitor.Detail, error) {
	return monitor.Detail{}, nil
}

func (s *guardedMonitorService) ExportRaw(context.Context, runtime.Config, int64) (map[string]any, error) {
	s.exports++
	if s.invalidRefetch && s.exports > 1 {
		return nil, nil
	}
	return cloneJSONMap(s.live), nil
}

func (s *guardedMonitorService) UpdateRaw(_ context.Context, _ runtime.Config, _ int64, value map[string]any) (map[string]any, error) {
	s.updates++
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	if s.persist {
		for key, item := range value {
			s.live[key] = item
		}
	}
	return cloneJSONMap(s.live), nil
}

func (s *validatingMonitorService) Validate(context.Context, runtime.Config, map[string]any) (any, error) {
	s.validated++
	return map[string]any{"valid": true, "warnings": []any{}}, nil
}

type guardedDashboardService struct {
	live    map[string]any
	persist bool
	exports int
	updates int
}

type fakeMonitorSearchService struct {
	fakeMonitorService
	params monitor.SearchParams
}

func (s *fakeMonitorSearchService) Search(_ context.Context, _ runtime.Config, params monitor.SearchParams) (monitor.SearchResult, error) {
	s.params = params
	return monitor.SearchResult{
		Items:    []monitor.SearchItem{{ID: 123, Name: "CPU high", Status: "Alert", Type: "query alert", Query: "avg:test{*} > 1", Classification: "custom"}},
		Count:    1,
		Query:    params.Query,
		Sort:     params.Sort,
		Page:     params.Page,
		PerPage:  params.PerPage,
		Endpoint: "GET /api/v1/monitor/search",
	}, nil
}

type fakeDashboardFilterService struct {
	params dashboard.ListParams
}

func (s *fakeDashboardFilterService) List(_ context.Context, _ runtime.Config, params dashboard.ListParams) (dashboard.ListResult, error) {
	s.params = params
	return dashboard.ListResult{Items: []dashboard.Summary{
		{ID: "dash-1", Title: "Ops overview", Author: "ops@example.com"},
		{ID: "dash-2", Title: "Finance overview", Author: "finance@example.com"},
	}}, nil
}

func (s *fakeDashboardFilterService) Get(context.Context, runtime.Config, string) (dashboard.Detail, error) {
	return dashboard.Detail{}, nil
}

func (s *guardedDashboardService) List(context.Context, runtime.Config, dashboard.ListParams) (dashboard.ListResult, error) {
	return dashboard.ListResult{}, nil
}

func (s *guardedDashboardService) Get(context.Context, runtime.Config, string) (dashboard.Detail, error) {
	return dashboard.Detail{}, nil
}

func (s *guardedDashboardService) ExportRaw(context.Context, runtime.Config, string) (map[string]any, error) {
	s.exports++
	return cloneJSONMap(s.live), nil
}

func (s *guardedDashboardService) UpdateRaw(_ context.Context, _ runtime.Config, _ string, value map[string]any) (map[string]any, error) {
	s.updates++
	if s.persist {
		for key, item := range value {
			s.live[key] = item
		}
	}
	return cloneJSONMap(s.live), nil
}

func cloneJSONMap(value map[string]any) map[string]any {
	data, _ := json.Marshal(value)
	var result map[string]any
	_ = json.Unmarshal(data, &result)
	return result
}

func executeGuardedCommand(t *testing.T, services serviceSet, input string, args ...string) (string, error) {
	t.Helper()
	t.Setenv("DD_API_KEY", "test-api-key")
	t.Setenv("DD_APP_KEY", "test-app-key")
	opts := &GlobalOptions{Services: services, FlagValues: runtime.FlagValues{NoEnvFile: true}}
	cmd := newRootCmdWithOptions(opts)
	output := new(bytes.Buffer)
	cmd.SetIn(strings.NewReader(input))
	cmd.SetOut(output)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return output.String(), err
}

func decodeJSONOutput(t *testing.T, value string) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		t.Fatalf("decode JSON output %q: %v", value, err)
	}
	return result
}

func monitorSpecJSON(message string) string {
	return `{"id":123,"name":"CPU","type":"query alert","query":"avg:test{*} > 1","message":"` + message + `"}`
}

func dashboardSpecJSON(title string) string {
	return `{"id":"dash-1","title":"` + title + `","layout_type":"ordered","widgets":[]}`
}

func TestGuardedMonitorDryRunFetchesLiveStateWithoutWriting(t *testing.T) {
	service := &guardedMonitorService{live: map[string]any{
		"id":      float64(123),
		"name":    "CPU",
		"type":    "query alert",
		"query":   "avg:test{*} > 1",
		"message": "old",
	}}
	output, err := executeGuardedCommand(t, serviceSet{Monitor: service}, monitorSpecJSON("new"), "monitor", "update", "123", "-", "--output", "json")
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	view := decodeJSONOutput(t, output)
	if view["dry_run"] != true || view["changed"] != true {
		t.Fatalf("unexpected dry-run result: %#v", view)
	}
	if service.exports != 1 || service.updates != 0 {
		t.Fatalf("unexpected service calls: exports=%d updates=%d", service.exports, service.updates)
	}
	if !strings.Contains(output, "message") || !strings.Contains(output, "monitor get") {
		t.Fatalf("dry-run output omitted diff or next command: %s", output)
	}
}

func TestGuardedMonitorApplyRejectsStaleFingerprintBeforeWriting(t *testing.T) {
	service := &guardedMonitorService{live: map[string]any{
		"id":      float64(123),
		"name":    "CPU",
		"type":    "query alert",
		"query":   "avg:test{*} > 1",
		"message": "old",
	}}
	_, err := executeGuardedCommand(t, serviceSet{Monitor: service}, monitorSpecJSON("new"), "monitor", "update", "123", "-", "--apply", "--fingerprint", strings.Repeat("b", 64), "--output", "json")
	if err == nil || !strings.Contains(err.Error(), "stale fingerprint") {
		t.Fatalf("expected stale fingerprint error, got %v", err)
	}
	if service.exports != 1 || service.updates != 0 {
		t.Fatalf("stale update made unexpected service calls: exports=%d updates=%d", service.exports, service.updates)
	}
}

func TestGuardedMonitorUpdateRejectsNonPositiveIDBeforeFetching(t *testing.T) {
	service := &guardedMonitorService{live: map[string]any{"id": float64(123)}}
	_, err := executeGuardedCommand(t, serviceSet{Monitor: service}, `{"id":0,"name":"CPU","type":"query alert","query":"avg:test{*} > 1","message":"new"}`, "monitor", "update", "0", "-", "--output", "json")
	if err == nil || !strings.Contains(err.Error(), `invalid monitor id "0"`) {
		t.Fatalf("expected invalid monitor id error, got %v", err)
	}
	if service.exports != 0 || service.updates != 0 {
		t.Fatalf("invalid monitor id contacted service: exports=%d updates=%d", service.exports, service.updates)
	}
}

func TestGuardedMonitorApplyAllowsExplicitReviewedStaleOverride(t *testing.T) {
	service := &guardedMonitorService{persist: true, live: map[string]any{
		"id": float64(123), "name": "CPU", "type": "query alert", "query": "avg:test{*} > 1", "message": "old",
	}}
	output, err := executeGuardedCommand(t, serviceSet{Monitor: service}, monitorSpecJSON("new"), "monitor", "update", "123", "-", "--apply", "--fingerprint", strings.Repeat("a", 64), "--allow-stale", "--output", "json")
	if err != nil {
		t.Fatalf("stale override: %v", err)
	}
	view := decodeJSONOutput(t, output)
	if view["updated"] != true || view["stale_override"] != true || service.updates != 1 {
		t.Fatalf("unexpected stale override result: %#v calls=%d", view, service.updates)
	}
	if service.live["query"] != "avg:test{*} > 1" {
		t.Fatalf("partial specification dropped unchanged query: %#v", service.live)
	}
}

func TestGuardedMonitorApplyRejectsMalformedFingerprint(t *testing.T) {
	service := &guardedMonitorService{live: map[string]any{"id": float64(123), "name": "CPU", "type": "query alert", "query": "avg:test{*} > 1"}}
	_, err := executeGuardedCommand(t, serviceSet{Monitor: service}, monitorSpecJSON("new"), "monitor", "update", "123", "-", "--apply", "--fingerprint", "not-a-digest", "--allow-stale", "--output", "json")
	if err == nil || !strings.Contains(err.Error(), "64-character SHA-256") {
		t.Fatalf("expected malformed fingerprint error, got %v", err)
	}
}

func TestGuardedMonitorApplyNoOpDoesNotWrite(t *testing.T) {
	service := &guardedMonitorService{live: map[string]any{
		"id":      float64(123),
		"name":    "CPU",
		"type":    "query alert",
		"query":   "avg:test{*} > 1",
		"message": "same",
	}}
	fingerprint := fingerprintJSON(service.live)
	output, err := executeGuardedCommand(t, serviceSet{Monitor: service}, monitorSpecJSON("same"), "monitor", "update", "123", "-", "--apply", "--fingerprint", fingerprint, "--output", "json")
	if err != nil {
		t.Fatalf("no-op apply: %v", err)
	}
	view := decodeJSONOutput(t, output)
	if view["no_op"] != true || view["updated"] != false {
		t.Fatalf("unexpected no-op result: %#v", view)
	}
	if service.exports != 1 || service.updates != 0 {
		t.Fatalf("no-op made unexpected service calls: exports=%d updates=%d", service.exports, service.updates)
	}
}

func TestGuardedMonitorApplyRefetchesAndVerifies(t *testing.T) {
	service := &guardedMonitorService{persist: true, live: map[string]any{
		"id":      float64(123),
		"name":    "CPU",
		"type":    "query alert",
		"query":   "avg:test{*} > 1",
		"message": "old",
	}}
	fingerprint := fingerprintJSON(service.live)
	output, err := executeGuardedCommand(t, serviceSet{Monitor: service}, monitorSpecJSON("new"), "monitor", "update", "123", "-", "--apply", "--fingerprint", fingerprint, "--output", "json")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	view := decodeJSONOutput(t, output)
	if view["updated"] != true {
		t.Fatalf("unexpected apply result: %#v", view)
	}
	if service.exports != 2 || service.updates != 1 {
		t.Fatalf("apply did not fetch, write, and refetch exactly once: exports=%d updates=%d", service.exports, service.updates)
	}
}

func TestGuardedMonitorApplyPreservesOmittedMutableAndUnknownFields(t *testing.T) {
	service := &guardedMonitorService{persist: true, live: map[string]any{
		"id":      float64(123),
		"name":    "CPU",
		"type":    "query alert",
		"query":   "avg:test{*} > 1",
		"message": "keep this message",
		"options": map[string]any{
			"thresholds":    map[string]any{"critical": float64(1)},
			"future_option": map[string]any{"enabled": true},
		},
		"future_field": map[string]any{"enabled": true},
	}}
	fingerprint := fingerprintJSON(service.live)
	input := `{"id":123,"name":"CPU renamed","type":"query alert","query":"avg:test{*} > 1"}`
	_, err := executeGuardedCommand(t, serviceSet{Monitor: service}, input, "monitor", "update", "123", "-", "--apply", "--fingerprint", fingerprint, "--output", "json")
	if err != nil {
		t.Fatalf("partial monitor apply: %v", err)
	}
	if service.live["message"] != "keep this message" {
		t.Fatalf("omitted mutable field was dropped: %#v", service.live)
	}
	options, ok := service.live["options"].(map[string]any)
	if !ok {
		t.Fatalf("options field was dropped: %#v", service.live)
	}
	futureOption, ok := options["future_option"].(map[string]any)
	if !ok || futureOption["enabled"] != true {
		t.Fatalf("unknown nested field was dropped: %#v", options)
	}
	futureField, ok := service.live["future_field"].(map[string]any)
	if !ok || futureField["enabled"] != true {
		t.Fatalf("unknown top-level field was dropped: %#v", service.live)
	}
}

func TestGuardedMonitorApplyReportsVerificationFailureWithRecoveryPath(t *testing.T) {
	service := &guardedMonitorService{live: map[string]any{
		"id": float64(123), "name": "CPU", "type": "query alert", "query": "avg:test{*} > 1", "message": "old",
	}}
	fingerprint := fingerprintJSON(service.live)
	_, err := executeGuardedCommand(t, serviceSet{Monitor: service}, monitorSpecJSON("new"), "monitor", "update", "123", "-", "--apply", "--fingerprint", fingerprint, "--output", "json")
	if err == nil || !strings.Contains(err.Error(), "post-update verification failed") || !strings.Contains(err.Error(), "re-export the resource") {
		t.Fatalf("expected verification failure with recovery path, got %v", err)
	}
	if service.exports != 2 || service.updates != 1 {
		t.Fatalf("verification failure made unexpected service calls: exports=%d updates=%d", service.exports, service.updates)
	}
}

func TestGuardedMonitorApplyReportsMalformedRefetchWithRecoveryPath(t *testing.T) {
	service := &guardedMonitorService{persist: true, invalidRefetch: true, live: map[string]any{
		"id": float64(123), "name": "CPU", "type": "query alert", "query": "avg:test{*} > 1", "message": "old",
	}}
	fingerprint := fingerprintJSON(service.live)
	_, err := executeGuardedCommand(t, serviceSet{Monitor: service}, monitorSpecJSON("new"), "monitor", "update", "123", "-", "--apply", "--fingerprint", fingerprint, "--output", "json")
	if err == nil || !strings.Contains(err.Error(), "post-update response was unusable") || !strings.Contains(err.Error(), "re-export resource 123") {
		t.Fatalf("expected malformed-refetch recovery path, got %v", err)
	}
	if service.exports != 2 || service.updates != 1 {
		t.Fatalf("malformed refetch made unexpected service calls: exports=%d updates=%d", service.exports, service.updates)
	}
}

func TestGuardedMonitorApplyReportsUncertainWriteWithRecoveryPath(t *testing.T) {
	service := &guardedMonitorService{
		updateErr: fmt.Errorf("upstream connection reset"),
		live: map[string]any{
			"id": float64(123), "name": "CPU", "type": "query alert", "query": "avg:test{*} > 1", "message": "old",
		},
	}
	fingerprint := fingerprintJSON(service.live)
	_, err := executeGuardedCommand(t, serviceSet{Monitor: service}, monitorSpecJSON("new"), "monitor", "update", "123", "-", "--apply", "--fingerprint", fingerprint, "--output", "json")
	if err == nil || !strings.Contains(err.Error(), "may have partially succeeded") || !strings.Contains(err.Error(), "re-export resource 123") {
		t.Fatalf("expected uncertain-write recovery path, got %v", err)
	}
	if service.exports != 1 || service.updates != 1 {
		t.Fatalf("uncertain write made unexpected service calls: exports=%d updates=%d", service.exports, service.updates)
	}
}

func TestMonitorRemoteValidationUsesConfiguredValidator(t *testing.T) {
	service := &validatingMonitorService{}
	output, err := executeGuardedCommand(t, serviceSet{Monitor: service}, monitorSpecJSON("new"), "monitor", "validate", "-", "--remote", "--output", "json")
	if err != nil {
		t.Fatalf("remote validation: %v", err)
	}
	view := decodeJSONOutput(t, output)
	if view["remote_validated"] != true || service.validated != 1 {
		t.Fatalf("remote validator was not called: view=%#v calls=%d", view, service.validated)
	}
	remoteResult, ok := view["remote_result"].(map[string]any)
	if !ok || remoteResult["valid"] != true {
		t.Fatalf("remote validation result missing: %#v", view)
	}
}

func TestGuardedDashboardApplyRefetchesAndVerifies(t *testing.T) {
	service := &guardedDashboardService{persist: true, live: map[string]any{
		"id":          "dash-1",
		"title":       "Old dashboard",
		"layout_type": "ordered",
		"widgets":     []any{},
	}}
	fingerprint := fingerprintJSON(service.live)
	output, err := executeGuardedCommand(t, serviceSet{Dashboard: service}, dashboardSpecJSON("New dashboard"), "dashboard", "update", "dash-1", "-", "--apply", "--fingerprint", fingerprint, "--output", "json")
	if err != nil {
		t.Fatalf("dashboard apply: %v", err)
	}
	view := decodeJSONOutput(t, output)
	if view["updated"] != true {
		t.Fatalf("unexpected dashboard apply result: %#v", view)
	}
	if service.exports != 2 || service.updates != 1 {
		t.Fatalf("dashboard apply did not fetch, write, and refetch exactly once: exports=%d updates=%d", service.exports, service.updates)
	}
}

func TestStructuredOutputProjectionRemovesUnselectedContext(t *testing.T) {
	output, err := executeGuardedCommand(t, serviceSet{Monitor: fakeMonitorService{}}, "", "monitor", "list", "--output", "json", "--fields", "items")
	if err != nil {
		t.Fatalf("projected list: %v", err)
	}
	view := decodeJSONOutput(t, output)
	if _, ok := view["items"]; !ok {
		t.Fatalf("projected output omitted selected field: %#v", view)
	}
	if _, ok := view["next"]; ok {
		t.Fatalf("projected output retained unselected contextual field: %#v", view)
	}
}

func TestTextOutputStatesEmptyResultsExplicitly(t *testing.T) {
	service := &guardedMonitorService{}
	output, err := executeGuardedCommand(t, serviceSet{Monitor: service}, "", "monitor", "list", "--output", "text")
	if err != nil {
		t.Fatalf("empty list: %v", err)
	}
	if !strings.Contains(output, "(no results)") {
		t.Fatalf("empty text output did not state no results: %q", output)
	}
	if !strings.Contains(output, "Next commands:") {
		t.Fatalf("empty text output omitted contextual next commands: %q", output)
	}
}

func TestMonitorSearchCLIForwardsSearchControlsAndEndpoint(t *testing.T) {
	service := &fakeMonitorSearchService{}
	output, err := executeGuardedCommand(t, serviceSet{Monitor: service}, "", "monitor", "search", "--query", "service:web", "--page", "2", "--per-page", "25", "--sort", "name", "--output", "json")
	if err != nil {
		t.Fatalf("monitor search: %v", err)
	}
	if service.params.Query != "service:web" || service.params.Page != 2 || service.params.PerPage != 25 || service.params.Sort != "name" {
		t.Fatalf("search controls were not forwarded: %+v", service.params)
	}
	view := decodeJSONOutput(t, output)
	if view["endpoint"] != "GET /api/v1/monitor/search" || !strings.Contains(output, "CPU high") {
		t.Fatalf("unexpected monitor search output: %s", output)
	}
}

func TestDashboardListCLIAppliesPageLocalFilterMetadata(t *testing.T) {
	service := &fakeDashboardFilterService{}
	output, err := executeGuardedCommand(t, serviceSet{Dashboard: service}, "", "dashboard", "list", "--filter", "ops", "--output", "json")
	if err != nil {
		t.Fatalf("dashboard list filter: %v", err)
	}
	if service.params.Filter != "ops" {
		t.Fatalf("dashboard filter was not forwarded: %+v", service.params)
	}
	view := decodeJSONOutput(t, output)
	if view["filter"] != "ops" || view["filter_scope"] != "page" || view["possibly_incomplete"] != true {
		t.Fatalf("missing page-local filter metadata: %#v", view)
	}
	items, ok := view["items"].([]any)
	if !ok || len(items) != 1 || !strings.Contains(output, "dash-1") || strings.Contains(output, "dash-2") {
		t.Fatalf("unexpected filtered dashboard output: %s", output)
	}
}
