package dashboard

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
)

type Service interface {
	List(context.Context, cliruntime.Config, ListParams) (ListResult, error)
	Get(context.Context, cliruntime.Config, string) (Detail, error)
}

// Updater is the write surface for updating an existing dashboard. The full
// SDK model is used directly so fields this CLI does not interpret remain
// available through AdditionalProperties and are sent back unchanged.
type Updater interface {
	UpdateDashboard(context.Context, cliruntime.Config, string, datadogV1.Dashboard) (datadogV1.Dashboard, error)
}

type Exporter interface {
	Export(context.Context, cliruntime.Config, string) (datadogV1.Dashboard, error)
}

type RawExporter interface {
	ExportRaw(context.Context, cliruntime.Config, string) (map[string]any, error)
}

type RawUpdater interface {
	UpdateRaw(context.Context, cliruntime.Config, string, map[string]any) (map[string]any, error)
}

type LiveService struct{}

type ListParams struct {
	Count          int64
	Start          int64
	IncludeShared  bool
	IncludeDeleted bool
	Filter         string
}

type Summary struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	LayoutType string     `json:"layout_type,omitempty"`
	Author     string     `json:"author,omitempty"`
	URL        string     `json:"url,omitempty"`
	Tags       []string   `json:"tags,omitempty"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
	ModifiedAt *time.Time `json:"modified_at,omitempty"`
}

type Detail struct {
	ID                      string                                      `json:"id"`
	Title                   string                                      `json:"title"`
	Description             string                                      `json:"description,omitempty"`
	LayoutType              string                                      `json:"layout_type,omitempty"`
	Author                  string                                      `json:"author,omitempty"`
	AuthorName              string                                      `json:"author_name,omitempty"`
	URL                     string                                      `json:"url,omitempty"`
	CreatedAt               *time.Time                                  `json:"created_at,omitempty"`
	ModifiedAt              *time.Time                                  `json:"modified_at,omitempty"`
	Tags                    []string                                    `json:"tags,omitempty"`
	WidgetCount             int                                         `json:"widget_count"`
	NotifyList              []string                                    `json:"notify_list,omitempty"`
	RestrictedRoles         []string                                    `json:"restricted_roles,omitempty"`
	ReflowType              string                                      `json:"reflow_type,omitempty"`
	Widgets                 []datadogV1.Widget                          `json:"widgets,omitempty"`
	TemplateVariables       []datadogV1.DashboardTemplateVariable       `json:"template_variables,omitempty"`
	TemplateVariablePresets []datadogV1.DashboardTemplateVariablePreset `json:"template_variable_presets,omitempty"`
	Raw                     map[string]any                              `json:"raw,omitempty"`
}

type ListResult struct {
	Items              []Summary `json:"items"`
	Count              int       `json:"count"`
	Endpoint           string    `json:"endpoint"`
	Filter             string    `json:"filter,omitempty"`
	FilterScope        string    `json:"filter_scope,omitempty"`
	PossiblyIncomplete bool      `json:"possibly_incomplete,omitempty"`
}

func (LiveService) List(ctx context.Context, cfg cliruntime.Config, params ListParams) (ListResult, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return ListResult{}, err
	}
	api := datadogV1.NewDashboardsApi(client.API)
	opt := datadogV1.NewListDashboardsOptionalParameters()
	if params.Count > 0 {
		opt.WithCount(params.Count)
	}
	if params.Start > 0 {
		opt.WithStart(params.Start)
	}
	if params.IncludeShared {
		opt.WithFilterShared(true)
	}
	if params.IncludeDeleted {
		opt.WithFilterDeleted(true)
	}
	resp, _, err := api.ListDashboards(client.Ctx, *opt)
	if err != nil {
		return ListResult{}, cliruntime.WrapAPIError(err, cfg)
	}
	items := resp.GetDashboards()
	views := make([]Summary, 0, len(items))
	for _, item := range items {
		views = append(views, mapDashboardSummary(item))
	}
	filter := strings.TrimSpace(params.Filter)
	if filter != "" {
		filtered := views[:0]
		for _, item := range views {
			if matchesFilter(item, filter) {
				filtered = append(filtered, item)
			}
		}
		views = filtered
	}
	result := ListResult{Items: views, Count: len(views), Endpoint: "GET /api/v1/dashboard"}
	if filter != "" {
		result.Filter = filter
		result.FilterScope = "page"
		result.PossiblyIncomplete = true
	}
	return result, nil
}

func (LiveService) Get(ctx context.Context, cfg cliruntime.Config, id string) (Detail, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return Detail{}, err
	}
	api := datadogV1.NewDashboardsApi(client.API)
	item, _, err := api.GetDashboard(client.Ctx, id)
	if err != nil {
		return Detail{}, cliruntime.WrapAPIError(err, cfg)
	}
	return mapDashboardDetail(item), nil
}

// UpdateDashboard updates an existing dashboard using Datadog's official
// update endpoint. The full SDK model is passed through without mapping to the
// reduced CLI detail view, preserving fields unknown to this CLI.
func (LiveService) UpdateDashboard(ctx context.Context, cfg cliruntime.Config, id string, dashboard datadogV1.Dashboard) (datadogV1.Dashboard, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return datadogV1.Dashboard{}, err
	}
	item, _, err := datadogV1.NewDashboardsApi(client.API).UpdateDashboard(client.Ctx, id, dashboard)
	if err != nil {
		return datadogV1.Dashboard{}, cliruntime.WrapAPIError(err, cfg)
	}
	return item, nil
}

// Export returns the SDK model without reducing it to the CLI summary shape.
func (LiveService) Export(ctx context.Context, cfg cliruntime.Config, id string) (datadogV1.Dashboard, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return datadogV1.Dashboard{}, err
	}
	item, _, err := datadogV1.NewDashboardsApi(client.API).GetDashboard(client.Ctx, id)
	if err != nil {
		return datadogV1.Dashboard{}, cliruntime.WrapAPIError(err, cfg)
	}
	return item, nil
}

func (s LiveService) ExportRaw(ctx context.Context, cfg cliruntime.Config, id string) (map[string]any, error) {
	value, err := s.Export(ctx, cfg, id)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s LiveService) UpdateRaw(ctx context.Context, cfg cliruntime.Config, id string, value map[string]any) (map[string]any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var request datadogV1.Dashboard
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, err
	}
	updated, err := s.UpdateDashboard(ctx, cfg, id, request)
	if err != nil {
		return nil, err
	}
	data, err = json.Marshal(updated)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func mapDashboardSummary(item datadogV1.DashboardSummaryDefinition) Summary {
	view := Summary{
		ID:         item.GetId(),
		Title:      item.GetTitle(),
		LayoutType: string(item.GetLayoutType()),
		Author:     item.GetAuthorHandle(),
		URL:        item.GetUrl(),
	}
	if rawTags, ok := item.AdditionalProperties["tags"]; ok {
		switch tags := rawTags.(type) {
		case []string:
			view.Tags = append(view.Tags, tags...)
		case []any:
			for _, tag := range tags {
				if value, ok := tag.(string); ok {
					view.Tags = append(view.Tags, value)
				}
			}
		}
	}
	if item.HasCreatedAt() {
		created := item.GetCreatedAt().UTC()
		view.CreatedAt = &created
	}
	if item.HasModifiedAt() {
		modified := item.GetModifiedAt().UTC()
		view.ModifiedAt = &modified
	}
	return view
}

func mapDashboardDetail(item datadogV1.Dashboard) Detail {
	view := Detail{
		ID:                      item.GetId(),
		Title:                   item.Title,
		Description:             item.GetDescription(),
		LayoutType:              string(item.LayoutType),
		Author:                  item.GetAuthorHandle(),
		AuthorName:              item.GetAuthorName(),
		URL:                     item.GetUrl(),
		WidgetCount:             len(item.Widgets),
		RestrictedRoles:         append([]string(nil), item.RestrictedRoles...),
		ReflowType:              string(item.GetReflowType()),
		Widgets:                 append([]datadogV1.Widget(nil), item.Widgets...),
		TemplateVariables:       append([]datadogV1.DashboardTemplateVariable(nil), item.TemplateVariables...),
		TemplateVariablePresets: append([]datadogV1.DashboardTemplateVariablePreset(nil), item.TemplateVariablePresets...),
	}
	if item.HasCreatedAt() {
		created := item.GetCreatedAt().UTC()
		view.CreatedAt = &created
	}
	if item.HasModifiedAt() {
		modified := item.GetModifiedAt().UTC()
		view.ModifiedAt = &modified
	}
	if tags := item.Tags.Get(); tags != nil {
		view.Tags = append([]string{}, (*tags)...)
	}
	if notifyList := item.NotifyList.Get(); notifyList != nil {
		view.NotifyList = append([]string{}, (*notifyList)...)
	}
	if tags := item.Tags.Get(); tags != nil {
		view.Tags = append([]string{}, (*tags)...)
	}
	view.Raw = rawJSON(item)
	return view
}

func matchesFilter(item Summary, filter string) bool {
	needle := strings.ToLower(strings.TrimSpace(filter))
	if needle == "" {
		return true
	}
	for _, value := range append([]string{item.ID, item.Title, item.Author}, item.Tags...) {
		if strings.Contains(strings.ToLower(value), needle) {
			return true
		}
	}
	return false
}

func rawJSON(value any) map[string]any {
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}
