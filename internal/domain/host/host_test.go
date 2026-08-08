package host

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
)

func TestMatchesHost(t *testing.T) {
	item := datadogV1.Host{Aliases: []string{"alias-1"}}
	item.SetName("web-01")
	item.SetHostName("web-01.local")
	if !matchesHost(item, "alias-1") || !matchesHost(item, "web-01") || !matchesHost(item, "WEB-01.LOCAL") {
		t.Fatalf("expected host match")
	}
	if matchesHost(item, "db-01") {
		t.Fatalf("did not expect host match")
	}
}

func TestListUsesHostsQueryContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/hosts" {
			t.Fatalf("unexpected hosts request: %s %s", r.Method, r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("filter") != "web" || query.Get("count") != "25" || query.Get("start") != "50" || query.Get("include_hosts_metadata") != "true" || query.Get("include_muted_hosts_data") != "true" {
			t.Fatalf("unexpected hosts query: %v", query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"total_matching":1,"total_returned":1,"host_list":[{"id":7,"name":"web-01","host_name":"web-01.local","aliases":["alias"],"apps":["agent"],"is_muted":false,"up":true,"last_reported_time":1785840000,"tags_by_source":{"Datadog":["env:prod"]}}]}`)
	}))
	defer server.Close()

	client := hostTestClient(t, server)
	result, err := listWithClient(client, ListParams{Filter: "web", Count: 25, Start: 50})
	if err != nil {
		t.Fatalf("list hosts: %v", err)
	}
	if result.Count != 1 || result.TotalMatching != 1 || result.Items[0].Name != "web-01" || result.Items[0].ID != 7 {
		t.Fatalf("unexpected host list: %+v", result)
	}
}

func TestGetUsesBoundedHostInventoryRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/hosts" {
			t.Fatalf("unexpected host detail request: %s %s", r.Method, r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("filter") != "web-01" || query.Get("count") != "1000" || query.Get("start") != "0" || query.Get("include_hosts_metadata") != "true" || query.Get("include_muted_hosts_data") != "true" {
			t.Fatalf("unexpected host detail query: %v", query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"total_matching":1,"total_returned":1,"host_list":[{"id":7,"name":"web-01","aws_name":"ip-10-0-0-1","meta":{"platform":"linux","agent_version":"7.0.0"},"tags_by_source":{"Datadog":["env:prod"]}}]}`)
	}))
	defer server.Close()

	client := hostTestClient(t, server)
	detail, err := getWithClient(client, "web-01")
	if err != nil {
		t.Fatalf("get host: %v", err)
	}
	if detail.Name != "web-01" || detail.AWSName != "ip-10-0-0-1" || detail.Platform != "linux" || detail.AgentVersion != "7.0.0" {
		t.Fatalf("unexpected host detail: %+v", detail)
	}
}

func hostTestClient(t *testing.T, server *httptest.Server) *cliruntime.Client {
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
