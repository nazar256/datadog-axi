package servicecatalog

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
)

// Service is the read-only Service Catalog surface exposed by the CLI.
type Service interface {
	List(context.Context, cliruntime.Config, ListParams) (ListResult, error)
	Get(context.Context, cliruntime.Config, string) (Detail, error)
}

type LiveService struct{}

// ListParams uses an item offset. The Datadog endpoint itself is page based;
// LiveService translates the offset to the corresponding page and trims the
// first page when the offset is not aligned to the requested limit.
type ListParams struct {
	Limit         int64
	Offset        int64
	Filter        string
	SchemaVersion string
}

type Link struct {
	Name     string         `json:"name,omitempty"`
	Type     string         `json:"type,omitempty"`
	URL      string         `json:"url,omitempty"`
	Provider string         `json:"provider,omitempty"`
	Raw      map[string]any `json:"raw,omitempty"`
}

type Contact struct {
	Name    string `json:"name,omitempty"`
	Type    string `json:"type,omitempty"`
	Contact string `json:"contact,omitempty"`
}

// Summary is a stable, SDK-independent representation of a service
// definition. Fields absent from an older schema remain empty.
type Summary struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Type          string     `json:"type,omitempty"`
	Owner         string     `json:"owner,omitempty"`
	Application   string     `json:"application,omitempty"`
	Description   string     `json:"description,omitempty"`
	Lifecycle     string     `json:"lifecycle,omitempty"`
	Tier          string     `json:"tier,omitempty"`
	SchemaVersion string     `json:"schema_version,omitempty"`
	Tags          []string   `json:"tags,omitempty"`
	Repositories  []Link     `json:"repositories,omitempty"`
	Documentation []Link     `json:"documentation,omitempty"`
	Links         []Link     `json:"links,omitempty"`
	Dependencies  []string   `json:"dependencies,omitempty"`
	Contacts      []Contact  `json:"contacts,omitempty"`
	Origin        string     `json:"origin,omitempty"`
	OriginDetail  string     `json:"origin_detail,omitempty"`
	ModifiedAt    *time.Time `json:"modified_at,omitempty"`
}

type Detail struct {
	Summary
	Raw map[string]any `json:"raw,omitempty"`
}

type ListResult struct {
	Items []Summary `json:"items"`
	Count int       `json:"count"`
}

func (LiveService) List(ctx context.Context, cfg cliruntime.Config, params ListParams) (ListResult, error) {
	if err := validateListParams(params); err != nil {
		return ListResult{}, err
	}
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return ListResult{}, err
	}
	return listWithClient(client, params)
}

func listWithClient(client *cliruntime.Client, params ListParams) (ListResult, error) {
	if client == nil {
		return ListResult{}, fmt.Errorf("service catalog client must not be nil")
	}
	opt := datadogV2.NewListServiceDefinitionsOptionalParameters()
	limit, page, trim := pageRequest(params)
	if limit > 0 {
		opt.WithPageSize(limit)
	}
	if page > 0 || params.Offset > 0 {
		opt.WithPageNumber(page)
	}
	if params.SchemaVersion != "" {
		version, err := schemaVersion(params.SchemaVersion)
		if err != nil {
			return ListResult{}, err
		}
		opt.WithSchemaVersion(version)
	}
	resp, _, err := datadogV2.NewServiceDefinitionApi(client.API).ListServiceDefinitions(client.Ctx, *opt)
	if err != nil {
		return ListResult{}, cliruntime.WrapAPIError(err, client.Config)
	}
	items := make([]Summary, 0, len(resp.GetData()))
	for _, item := range resp.GetData() {
		view := mapDefinition(item)
		if matchesFilter(view, params.Filter) {
			items = append(items, view)
		}
	}
	if trim > int64(len(items)) {
		items = nil
	} else if trim > 0 {
		items = items[int(trim):]
	}
	if params.Limit > 0 && len(items) > int(params.Limit) {
		items = items[:params.Limit]
	}
	return ListResult{Items: items, Count: len(items)}, nil
}

func (LiveService) Get(ctx context.Context, cfg cliruntime.Config, name string) (Detail, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Detail{}, fmt.Errorf("service name must not be empty")
	}
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return Detail{}, err
	}
	return getWithClient(client, name)
}

func getWithClient(client *cliruntime.Client, name string) (Detail, error) {
	if client == nil {
		return Detail{}, fmt.Errorf("service catalog client must not be nil")
	}
	resp, _, err := datadogV2.NewServiceDefinitionApi(client.API).GetServiceDefinition(client.Ctx, name)
	if err != nil {
		return Detail{}, cliruntime.WrapAPIError(err, client.Config)
	}
	item, ok := resp.GetDataOk()
	if !ok {
		return Detail{}, fmt.Errorf("service %q returned no definition", name)
	}
	return mapDetail(*item), nil
}

func validateListParams(params ListParams) error {
	if params.Limit < 0 || params.Limit > 1000 {
		return fmt.Errorf("service catalog limit must be between 0 and 1000")
	}
	if params.Offset < 0 {
		return fmt.Errorf("service catalog offset must not be negative")
	}
	if params.SchemaVersion != "" {
		_, err := schemaVersion(params.SchemaVersion)
		return err
	}
	return nil
}

func schemaVersion(value string) (datadogV2.ServiceDefinitionSchemaVersions, error) {
	version, err := datadogV2.NewServiceDefinitionSchemaVersionsFromValue(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("invalid service catalog schema version: %w", err)
	}
	return *version, nil
}

func pageRequest(params ListParams) (limit, page, trim int64) {
	limit = params.Limit
	if limit == 0 {
		limit = 100
	}
	page = params.Offset / limit
	trim = params.Offset % limit
	requestLimit := limit + trim
	if requestLimit > 1000 {
		requestLimit = 1000
	}
	return requestLimit, page, trim
}

func matchesFilter(item Summary, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}
	filter = strings.ToLower(filter)
	for _, value := range []string{item.ID, item.Name, item.Owner, item.Application, item.Description, item.Lifecycle, item.Tier} {
		if strings.Contains(strings.ToLower(value), filter) {
			return true
		}
	}
	return false
}

func mapDetail(item datadogV2.ServiceDefinitionData) Detail {
	return Detail{Summary: mapDefinition(item), Raw: rawData(item)}
}

func mapDefinition(item datadogV2.ServiceDefinitionData) Summary {
	view := Summary{ID: item.GetId(), Type: item.GetType()}
	if item.Attributes == nil || item.Attributes.Schema == nil {
		return view
	}
	if item.Attributes.Meta != nil {
		meta := item.Attributes.Meta
		view.Origin = meta.GetOrigin()
		view.OriginDetail = meta.GetOriginDetail()
		if value := meta.GetLastModifiedTime(); value != "" {
			if parsed, err := time.Parse(time.RFC3339, value); err == nil {
				parsed = parsed.UTC()
				view.ModifiedAt = &parsed
			}
		}
	}
	schema := item.Attributes.Schema
	switch {
	case schema.ServiceDefinitionV1 != nil:
		mapV1(&view, schema.ServiceDefinitionV1)
	case schema.ServiceDefinitionV2 != nil:
		mapV2(&view, schema.ServiceDefinitionV2)
	case schema.ServiceDefinitionV2Dot1 != nil:
		mapV21(&view, schema.ServiceDefinitionV2Dot1)
	case schema.ServiceDefinitionV2Dot2 != nil:
		mapV22(&view, schema.ServiceDefinitionV2Dot2)
	}
	if view.Name == "" {
		view.Name = view.ID
	}
	return view
}

func mapV1(view *Summary, item *datadogV2.ServiceDefinitionV1) {
	view.Name = item.Info.GetDdService()
	view.Type = "service"
	view.Description = item.Info.GetDescription()
	view.Tier = item.Info.GetServiceTier()
	view.SchemaVersion = string(item.SchemaVersion)
	view.Tags = cloneStrings(item.Tags)
	if item.Contact != nil {
		if email := item.Contact.GetEmail(); email != "" {
			view.Contacts = append(view.Contacts, Contact{Type: "email", Contact: email})
		}
		if slack := item.Contact.GetSlack(); slack != "" {
			view.Contacts = append(view.Contacts, Contact{Type: "slack", Contact: slack})
		}
	}
	for _, resource := range item.ExternalResources {
		view.Links = append(view.Links, Link{Name: resource.GetName(), Type: string(resource.GetType()), URL: resource.GetUrl(), Raw: cloneMap(resource.AdditionalProperties)})
	}
	view.Dependencies = dependencies(item.Extensions)
}

func mapV2(view *Summary, item *datadogV2.ServiceDefinitionV2) {
	view.Name = item.GetDdService()
	view.Owner = firstNonEmpty(item.GetTeam(), item.GetDdTeam())
	view.SchemaVersion = string(item.SchemaVersion)
	view.Tags = cloneStrings(item.Tags)
	view.Repositories = mapV2Repos(item.Repos)
	view.Documentation = mapV2Docs(item.Docs)
	view.Links = mapV2Links(item.Links)
	view.Contacts = mapV2Contacts(item.Contacts)
	view.Dependencies = dependencies(item.Extensions)
}

func mapV21(view *Summary, item *datadogV2.ServiceDefinitionV2Dot1) {
	view.Name = item.GetDdService()
	view.Owner = item.GetTeam()
	view.Application = item.GetApplication()
	view.Description = item.GetDescription()
	view.Lifecycle = item.GetLifecycle()
	view.Tier = item.GetTier()
	view.SchemaVersion = string(item.SchemaVersion)
	view.Tags = cloneStrings(item.Tags)
	view.Links = mapV21Links(item.Links)
	view.Contacts = mapV21Contacts(item.Contacts)
	view.Dependencies = dependencies(item.Extensions)
}

func mapV22(view *Summary, item *datadogV2.ServiceDefinitionV2Dot2) {
	view.Name = item.GetDdService()
	view.Type = item.GetType()
	view.Application = item.GetApplication()
	view.Description = item.GetDescription()
	view.Owner = item.GetTeam()
	view.Lifecycle = item.GetLifecycle()
	view.Tier = item.GetTier()
	view.SchemaVersion = string(item.SchemaVersion)
	view.Tags = cloneStrings(item.Tags)
	view.Links = mapV22Links(item.Links)
	view.Contacts = mapV22Contacts(item.Contacts)
	view.Dependencies = dependencies(item.Extensions)
}

func mapV2Repos(items []datadogV2.ServiceDefinitionV2Repo) []Link {
	links := make([]Link, 0, len(items))
	for _, item := range items {
		links = append(links, Link{Name: item.GetName(), URL: item.GetUrl(), Raw: cloneMap(item.AdditionalProperties)})
	}
	return links
}

func mapV2Docs(items []datadogV2.ServiceDefinitionV2Doc) []Link {
	links := make([]Link, 0, len(items))
	for _, item := range items {
		links = append(links, Link{Name: item.GetName(), URL: item.GetUrl(), Raw: cloneMap(item.AdditionalProperties)})
	}
	return links
}

func mapV2Links(items []datadogV2.ServiceDefinitionV2Link) []Link {
	links := make([]Link, 0, len(items))
	for _, item := range items {
		links = append(links, Link{Name: item.GetName(), URL: item.GetUrl(), Raw: cloneMap(item.AdditionalProperties)})
	}
	return links
}

func mapV21Links(items []datadogV2.ServiceDefinitionV2Dot1Link) []Link {
	links := make([]Link, 0, len(items))
	for _, item := range items {
		links = append(links, Link{Name: item.GetName(), Type: string(item.GetType()), URL: item.GetUrl(), Provider: item.GetProvider(), Raw: cloneMap(item.AdditionalProperties)})
	}
	return links
}

func mapV22Links(items []datadogV2.ServiceDefinitionV2Dot2Link) []Link {
	links := make([]Link, 0, len(items))
	for _, item := range items {
		links = append(links, Link{Name: item.GetName(), Type: item.GetType(), URL: item.GetUrl(), Provider: item.GetProvider(), Raw: cloneMap(item.AdditionalProperties)})
	}
	return links
}

func mapV2Contacts(items []datadogV2.ServiceDefinitionV2Contact) []Contact {
	contacts := make([]Contact, 0, len(items))
	for _, item := range items {
		switch {
		case item.ServiceDefinitionV2Email != nil:
			contacts = append(contacts, Contact{Name: item.ServiceDefinitionV2Email.GetName(), Type: string(item.ServiceDefinitionV2Email.GetType()), Contact: item.ServiceDefinitionV2Email.GetContact()})
		case item.ServiceDefinitionV2Slack != nil:
			contacts = append(contacts, Contact{Name: item.ServiceDefinitionV2Slack.GetName(), Type: string(item.ServiceDefinitionV2Slack.GetType()), Contact: item.ServiceDefinitionV2Slack.GetContact()})
		case item.ServiceDefinitionV2MSTeams != nil:
			contacts = append(contacts, Contact{Name: item.ServiceDefinitionV2MSTeams.GetName(), Type: string(item.ServiceDefinitionV2MSTeams.GetType()), Contact: item.ServiceDefinitionV2MSTeams.GetContact()})
		}
	}
	return contacts
}

func mapV21Contacts(items []datadogV2.ServiceDefinitionV2Dot1Contact) []Contact {
	contacts := make([]Contact, 0, len(items))
	for _, item := range items {
		switch {
		case item.ServiceDefinitionV2Dot1Email != nil:
			contacts = append(contacts, Contact{Name: item.ServiceDefinitionV2Dot1Email.GetName(), Type: string(item.ServiceDefinitionV2Dot1Email.GetType()), Contact: item.ServiceDefinitionV2Dot1Email.GetContact()})
		case item.ServiceDefinitionV2Dot1Slack != nil:
			contacts = append(contacts, Contact{Name: item.ServiceDefinitionV2Dot1Slack.GetName(), Type: string(item.ServiceDefinitionV2Dot1Slack.GetType()), Contact: item.ServiceDefinitionV2Dot1Slack.GetContact()})
		case item.ServiceDefinitionV2Dot1MSTeams != nil:
			contacts = append(contacts, Contact{Name: item.ServiceDefinitionV2Dot1MSTeams.GetName(), Type: string(item.ServiceDefinitionV2Dot1MSTeams.GetType()), Contact: item.ServiceDefinitionV2Dot1MSTeams.GetContact()})
		}
	}
	return contacts
}

func mapV22Contacts(items []datadogV2.ServiceDefinitionV2Dot2Contact) []Contact {
	contacts := make([]Contact, 0, len(items))
	for _, item := range items {
		contacts = append(contacts, Contact{Name: item.GetName(), Type: item.GetType(), Contact: item.GetContact()})
	}
	return contacts
}

func dependencies(extensions map[string]interface{}) []string {
	value, ok := extensions["dependencies"]
	if !ok {
		return nil
	}
	switch values := value.(type) {
	case []string:
		return cloneStrings(values)
	case []interface{}:
		result := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok && text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func rawData(item datadogV2.ServiceDefinitionData) map[string]any {
	raw := cloneMap(item.AdditionalProperties)
	if item.Attributes != nil {
		if len(item.Attributes.AdditionalProperties) > 0 && raw == nil {
			raw = make(map[string]any, len(item.Attributes.AdditionalProperties))
		}
		for key, value := range item.Attributes.AdditionalProperties {
			raw[key] = value
		}
	}
	return raw
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string{}, values...)
}

func cloneMap(values map[string]interface{}) map[string]any {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]any, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
