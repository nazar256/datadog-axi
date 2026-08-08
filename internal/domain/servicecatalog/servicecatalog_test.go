package servicecatalog

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
)

func TestMapDefinitionV22(t *testing.T) {
	definition := datadogV2.NewServiceDefinitionV2Dot2("checkout", datadogV2.SERVICEDEFINITIONV2DOT2VERSION_V2_2)
	definition.SetType("web")
	definition.SetTeam("payments")
	definition.SetDescription("Checkout API")
	definition.SetLifecycle("production")
	definition.SetTier("critical")
	definition.SetApplication("storefront")
	definition.SetTags([]string{"env:prod"})
	definition.SetExtensions(map[string]interface{}{"dependencies": []interface{}{"orders", "inventory"}})
	definition.SetLinks([]datadogV2.ServiceDefinitionV2Dot2Link{{Name: "runbook", Type: "runbook", Url: "https://example.test/runbook", Provider: stringPtr("wiki"), AdditionalProperties: map[string]interface{}{"owner": "sre"}}})
	definition.SetContacts([]datadogV2.ServiceDefinitionV2Dot2Contact{{Name: stringPtr("oncall"), Type: "email", Contact: "oncall@example.test"}})

	schema := datadogV2.ServiceDefinitionV2Dot2AsServiceDefinitionSchema(definition)
	attrs := datadogV2.NewServiceDefinitionDataAttributes()
	attrs.SetSchema(schema)
	item := datadogV2.NewServiceDefinitionData()
	item.SetId("service-id")
	item.SetType("service-definition")
	item.SetAttributes(*attrs)

	view := mapDefinition(*item)
	if view.ID != "service-id" || view.Name != "checkout" || view.Type != "web" || view.Owner != "payments" || view.Application != "storefront" {
		t.Fatalf("unexpected service mapping: %+v", view)
	}
	if len(view.Dependencies) != 2 || view.Dependencies[1] != "inventory" || len(view.Links) != 1 || view.Links[0].Raw["owner"] != "sre" {
		t.Fatalf("unexpected links/dependencies: %+v", view)
	}
	if len(view.Contacts) != 1 || view.Contacts[0].Contact != "oncall@example.test" {
		t.Fatalf("unexpected contacts: %+v", view.Contacts)
	}
}

func TestMapDefinitionV2SeparatesRepositoriesAndDocs(t *testing.T) {
	definition := datadogV2.NewServiceDefinitionV2("payments", datadogV2.SERVICEDEFINITIONV2VERSION_V2)
	definition.SetTeam("payments-team")
	definition.SetRepos([]datadogV2.ServiceDefinitionV2Repo{{Name: "api", Url: "https://example.test/repo"}})
	definition.SetDocs([]datadogV2.ServiceDefinitionV2Doc{{Name: "docs", Url: "https://example.test/docs"}})
	schema := datadogV2.ServiceDefinitionV2AsServiceDefinitionSchema(definition)
	attrs := datadogV2.NewServiceDefinitionDataAttributes()
	attrs.SetSchema(schema)
	item := datadogV2.NewServiceDefinitionData()
	item.SetAttributes(*attrs)

	view := mapDefinition(*item)
	if view.Name != "payments" || view.Owner != "payments-team" || len(view.Repositories) != 1 || len(view.Documentation) != 1 {
		t.Fatalf("unexpected v2 mapping: %+v", view)
	}
}

func TestMatchesFilter(t *testing.T) {
	item := Summary{Name: "Checkout API", Owner: "Payments"}
	if !matchesFilter(item, "payments") || !matchesFilter(item, "CHECKOUT") || matchesFilter(item, "unknown") {
		t.Fatal("unexpected service filter result")
	}
}

func stringPtr(value string) *string { return &value }

func TestListUsesServiceCatalogPaginationContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2/services/definitions" {
			t.Fatalf("unexpected service catalog request: %s %s", r.Method, r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("page[size]") != "10" || query.Get("page[number]") != "1" || query.Get("schema_version") != "v2.2" {
			t.Fatalf("unexpected service catalog query: %v", query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"service-id","type":"service-definition","attributes":{"schema":{"dd-service":"checkout","schema-version":"v2.2","team":"payments","type":"web","description":"Checkout API","lifecycle":"production","tier":"critical","application":"storefront","tags":["env:prod"]}}}]}`)
	}))
	defer server.Close()

	client := serviceCatalogTestClient(t, server)
	result, err := listWithClient(client, ListParams{Limit: 10, Offset: 10, Filter: "checkout", SchemaVersion: "v2.2"})
	if err != nil {
		t.Fatalf("list service catalog: %v", err)
	}
	if result.Count != 1 || result.Items[0].ID != "service-id" || result.Items[0].Name != "checkout" || result.Items[0].Owner != "payments" {
		t.Fatalf("unexpected service catalog result: %+v", result)
	}
}

func TestGetUsesServiceDefinitionPathAndPreservesRaw(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2/services/definitions/checkout" {
			t.Fatalf("unexpected service detail request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"service-id","type":"service-definition","attributes":{"future":"preserve-me","meta":{"origin":"api","origin-detail":"catalog","last-modified-time":"2026-08-04T10:00:00Z"},"schema":{"dd-service":"checkout","schema-version":"v2.2","team":"payments","type":"web","extensions":{"future":{"enabled":true}},"links":[{"name":"runbook","type":"runbook","url":"https://example.test/runbook","provider":"wiki"}]}}}}`)
	}))
	defer server.Close()

	client := serviceCatalogTestClient(t, server)
	detail, err := getWithClient(client, "checkout")
	if err != nil {
		t.Fatalf("get service catalog definition: %v", err)
	}
	if detail.ID != "service-id" || detail.Name != "checkout" || detail.Origin != "api" || detail.Raw["future"] != "preserve-me" {
		t.Fatalf("unexpected service detail: %+v", detail)
	}
}

func serviceCatalogTestClient(t *testing.T, server *httptest.Server) *cliruntime.Client {
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
