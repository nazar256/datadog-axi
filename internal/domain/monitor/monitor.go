package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
	"github.com/samber/lo"
)

type Service interface {
	List(context.Context, cliruntime.Config, ListParams) (ListResult, error)
	Get(context.Context, cliruntime.Config, int64) (Detail, error)
}

// Searcher is the advanced monitor-search surface. It is deliberately kept
// separate from Service so callers that only provide list/get fakes remain
// source-compatible while the live adapter can expose the documented search
// endpoint.
type Searcher interface {
	Search(context.Context, cliruntime.Config, SearchParams) (SearchResult, error)
}

// Updater is the write surface for updating an existing monitor. The SDK
// request model is used directly so its AdditionalProperties map can carry
// fields this CLI does not interpret.
type Updater interface {
	UpdateMonitor(context.Context, cliruntime.Config, int64, datadogV1.MonitorUpdateRequest) (datadogV1.Monitor, error)
}

type Exporter interface {
	Export(context.Context, cliruntime.Config, int64) (datadogV1.Monitor, error)
}

type RawExporter interface {
	ExportRaw(context.Context, cliruntime.Config, int64) (map[string]any, error)
}

type RawUpdater interface {
	UpdateRaw(context.Context, cliruntime.Config, int64, map[string]any) (map[string]any, error)
}

type Validator interface {
	Validate(context.Context, cliruntime.Config, map[string]any) (any, error)
}

// ExistingValidator validates a proposed monitor against an existing monitor
// id. The endpoint is read-only and preserves the API's type-specific rules.
type ExistingValidator interface {
	ValidateExisting(context.Context, cliruntime.Config, int64, map[string]any) (any, error)
}

type LiveService struct{}

type ListParams struct {
	Name        string
	Tags        string
	MonitorTags string
	Offset      int64
	Limit       int32
}

type SearchParams struct {
	Query   string
	Page    int64
	PerPage int64
	Sort    string
}

type Summary struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Type        string     `json:"type"`
	State       string     `json:"state,omitempty"`
	Query       string     `json:"query"`
	Tags        []string   `json:"tags,omitempty"`
	Priority    *int64     `json:"priority,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	ModifiedAt  *time.Time `json:"modified_at,omitempty"`
	Message     string     `json:"message,omitempty"`
	URL         string     `json:"url,omitempty"`
	MultiAlert  bool       `json:"multi_alert"`
	DraftStatus string     `json:"draft_status,omitempty"`
}

// Detail is the bounded human-facing monitor projection. Raw contains the
// complete SDK JSON when callers use the domain service directly; the CLI's
// JSON/full paths use RawExporter to return that representation without
// reducing it to this projection.
type Detail struct {
	ID                int64                        `json:"id"`
	Name              string                       `json:"name"`
	Type              string                       `json:"type"`
	State             string                       `json:"state,omitempty"`
	Query             string                       `json:"query"`
	Tags              []string                     `json:"tags,omitempty"`
	Priority          *int64                       `json:"priority,omitempty"`
	CreatedAt         *time.Time                   `json:"created_at,omitempty"`
	ModifiedAt        *time.Time                   `json:"modified_at,omitempty"`
	DeletedAt         *time.Time                   `json:"deleted_at,omitempty"`
	Message           string                       `json:"message,omitempty"`
	MultiAlert        bool                         `json:"multi_alert"`
	DraftStatus       string                       `json:"draft_status,omitempty"`
	Creator           *datadogV1.Creator           `json:"creator,omitempty"`
	Options           *datadogV1.MonitorOptions    `json:"options,omitempty"`
	RestrictedRoles   []string                     `json:"restricted_roles,omitempty"`
	MatchingDowntimes []datadogV1.MatchingDowntime `json:"matching_downtimes,omitempty"`
	URL               string                       `json:"url,omitempty"`
	Raw               map[string]any               `json:"raw,omitempty"`
}

type SearchItem struct {
	ID              int64                                       `json:"id"`
	Name            string                                      `json:"name"`
	Query           string                                      `json:"query,omitempty"`
	Type            string                                      `json:"type,omitempty"`
	Status          string                                      `json:"status,omitempty"`
	Tags            []string                                    `json:"tags,omitempty"`
	Scopes          []string                                    `json:"scopes,omitempty"`
	Classification  string                                      `json:"classification,omitempty"`
	Metrics         []string                                    `json:"metrics,omitempty"`
	QualityIssues   []string                                    `json:"quality_issues,omitempty"`
	Notifications   []datadogV1.MonitorSearchResultNotification `json:"notifications,omitempty"`
	Creator         *datadogV1.Creator                          `json:"creator,omitempty"`
	LastTriggeredTs *int64                                      `json:"last_triggered_ts,omitempty"`
	URL             string                                      `json:"url,omitempty"`
	Raw             map[string]any                              `json:"raw,omitempty"`
}

type SearchResult struct {
	Items      []SearchItem   `json:"items"`
	Count      int            `json:"count"`
	Query      string         `json:"query,omitempty"`
	Sort       string         `json:"sort,omitempty"`
	Page       int64          `json:"page,omitempty"`
	PageCount  int64          `json:"page_count,omitempty"`
	PerPage    int64          `json:"per_page,omitempty"`
	TotalCount int64          `json:"total_count,omitempty"`
	Counts     map[string]any `json:"counts,omitempty"`
	Raw        map[string]any `json:"raw,omitempty"`
	Endpoint   string         `json:"endpoint"`
}

type ListResult struct {
	Items    []Summary `json:"items"`
	Count    int       `json:"count"`
	Endpoint string    `json:"endpoint"`
}

func (LiveService) List(ctx context.Context, cfg cliruntime.Config, params ListParams) (ListResult, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return ListResult{}, err
	}
	api := datadogV1.NewMonitorsApi(client.API)
	opt := datadogV1.NewListMonitorsOptionalParameters()
	if params.Name != "" {
		opt.WithName(params.Name)
	}
	if params.Tags != "" {
		opt.WithTags(params.Tags)
	}
	if params.MonitorTags != "" {
		opt.WithMonitorTags(params.MonitorTags)
	}
	if params.Offset > 0 {
		opt.WithIdOffset(params.Offset)
	}
	if params.Limit > 0 {
		opt.WithPageSize(params.Limit)
	}
	items, _, err := api.ListMonitors(client.Ctx, *opt)
	if err != nil {
		return ListResult{}, cliruntime.WrapAPIError(err, cfg)
	}
	views := make([]Summary, 0, len(items))
	for _, item := range items {
		view := mapMonitor(item)
		view.URL = monitorURL(cfg.Site, view.ID)
		views = append(views, view)
	}
	return ListResult{Items: views, Count: len(views), Endpoint: "GET /api/v1/monitor"}, nil
}

func (LiveService) Search(ctx context.Context, cfg cliruntime.Config, params SearchParams) (SearchResult, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return SearchResult{}, err
	}
	api := datadogV1.NewMonitorsApi(client.API)
	opt := datadogV1.NewSearchMonitorsOptionalParameters()
	if params.Query != "" {
		opt.WithQuery(params.Query)
	}
	if params.Page > 0 {
		opt.WithPage(params.Page)
	}
	if params.PerPage > 0 {
		opt.WithPerPage(params.PerPage)
	}
	if params.Sort != "" {
		opt.WithSort(params.Sort)
	}
	response, _, err := api.SearchMonitors(client.Ctx, *opt)
	if err != nil {
		return SearchResult{}, cliruntime.WrapAPIError(err, cfg)
	}
	result := SearchResult{Items: make([]SearchItem, 0, len(response.Monitors)), Query: params.Query, Sort: params.Sort, Endpoint: "GET /api/v1/monitor/search", Raw: rawJSON(response)}
	for _, item := range response.Monitors {
		view := mapSearchItem(item)
		view.URL = monitorURL(cfg.Site, view.ID)
		result.Items = append(result.Items, view)
	}
	result.Count = len(result.Items)
	if response.Metadata != nil {
		result.Page = response.Metadata.GetPage()
		result.PageCount = response.Metadata.GetPageCount()
		result.PerPage = response.Metadata.GetPerPage()
		result.TotalCount = response.Metadata.GetTotalCount()
	}
	if raw := result.Raw["counts"]; raw != nil {
		if counts, ok := raw.(map[string]any); ok {
			result.Counts = counts
		}
	}
	return result, nil
}

func (LiveService) Get(ctx context.Context, cfg cliruntime.Config, id int64) (Detail, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return Detail{}, err
	}
	api := datadogV1.NewMonitorsApi(client.API)
	item, _, err := api.GetMonitor(client.Ctx, id)
	if err != nil {
		return Detail{}, cliruntime.WrapAPIError(err, cfg)
	}
	detail := mapMonitorDetail(item)
	detail.URL = monitorURL(cfg.Site, detail.ID)
	return detail, nil
}

func monitorURL(site string, id int64) string {
	if site == "" || id <= 0 {
		return ""
	}
	return fmt.Sprintf("https://%s/monitors/%d", monitorUIHost(site), id)
}

func monitorUIHost(site string) string {
	switch strings.ToLower(strings.TrimSpace(site)) {
	case "datadoghq.com":
		return "app.datadoghq.com"
	case "datadoghq.eu":
		return "app.datadoghq.eu"
	case "us3.datadoghq.com":
		return "us3.datadoghq.com"
	case "us5.datadoghq.com":
		return "us5.datadoghq.com"
	case "ap1.datadoghq.com":
		return "ap1.datadoghq.com"
	case "ap2.datadoghq.com":
		return "ap2.datadoghq.com"
	case "ddog-gov.com":
		return "app.ddog-gov.com"
	default:
		return site
	}
}

// UpdateMonitor updates an existing monitor using Datadog's official update
// endpoint. The request is intentionally not reduced to the CLI summary model
// so fields unknown to this CLI survive JSON round trips and are sent back to
// Datadog unchanged.
func (LiveService) UpdateMonitor(ctx context.Context, cfg cliruntime.Config, id int64, request datadogV1.MonitorUpdateRequest) (datadogV1.Monitor, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return datadogV1.Monitor{}, err
	}
	item, _, err := datadogV1.NewMonitorsApi(client.API).UpdateMonitor(client.Ctx, id, request)
	if err != nil {
		return datadogV1.Monitor{}, cliruntime.WrapAPIError(err, cfg)
	}
	return item, nil
}

// Export returns the SDK model without reducing it to the CLI summary shape.
// It is used by export, detail, validation, and the guarded edit workflow so
// fields unknown to this CLI remain available for review and round trips.
func (LiveService) Export(ctx context.Context, cfg cliruntime.Config, id int64) (datadogV1.Monitor, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return datadogV1.Monitor{}, err
	}
	item, _, err := datadogV1.NewMonitorsApi(client.API).GetMonitor(client.Ctx, id)
	if err != nil {
		return datadogV1.Monitor{}, cliruntime.WrapAPIError(err, cfg)
	}
	return item, nil
}

func (s LiveService) ExportRaw(ctx context.Context, cfg cliruntime.Config, id int64) (map[string]any, error) {
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

func (s LiveService) UpdateRaw(ctx context.Context, cfg cliruntime.Config, id int64, value map[string]any) (map[string]any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var request datadogV1.MonitorUpdateRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, err
	}
	updated, err := s.UpdateMonitor(ctx, cfg, id, request)
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

func (s LiveService) Validate(ctx context.Context, cfg cliruntime.Config, value map[string]any) (any, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var request datadogV1.Monitor
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, err
	}
	response, _, err := datadogV1.NewMonitorsApi(client.API).ValidateMonitor(client.Ctx, request)
	if err != nil {
		return nil, cliruntime.WrapAPIError(err, cfg)
	}
	return response, nil
}

func (s LiveService) ValidateExisting(ctx context.Context, cfg cliruntime.Config, id int64, value map[string]any) (any, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var request datadogV1.Monitor
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, err
	}
	response, _, err := datadogV1.NewMonitorsApi(client.API).ValidateExistingMonitor(client.Ctx, id, request)
	if err != nil {
		return nil, cliruntime.WrapAPIError(err, cfg)
	}
	return response, nil
}

func mapMonitor(item datadogV1.Monitor) Summary {
	view := Summary{
		ID:         item.GetId(),
		Name:       item.GetName(),
		Type:       string(item.Type),
		State:      string(item.GetOverallState()),
		Query:      item.Query,
		Tags:       item.Tags,
		Message:    item.GetMessage(),
		MultiAlert: item.GetMulti(),
	}
	if item.HasCreated() {
		created := item.GetCreated().UTC()
		view.CreatedAt = &created
	}
	if item.HasModified() {
		modified := item.GetModified().UTC()
		view.ModifiedAt = &modified
	}
	if priority := item.Priority.Get(); priority != nil {
		p := *priority
		view.Priority = &p
	}
	if item.HasDraftStatus() {
		view.DraftStatus = string(item.GetDraftStatus())
	}
	view.Tags = lo.Map(item.Tags, func(tag string, _ int) string { return tag })
	return view
}

func mapMonitorDetail(item datadogV1.Monitor) Detail {
	view := Detail{
		ID:                item.GetId(),
		Name:              item.GetName(),
		Type:              string(item.Type),
		State:             string(item.GetOverallState()),
		Query:             item.Query,
		Tags:              append([]string(nil), item.Tags...),
		Message:           item.GetMessage(),
		MultiAlert:        item.GetMulti(),
		Options:           item.Options,
		MatchingDowntimes: append([]datadogV1.MatchingDowntime(nil), item.MatchingDowntimes...),
	}
	if item.HasCreated() {
		created := item.GetCreated().UTC()
		view.CreatedAt = &created
	}
	if item.HasModified() {
		modified := item.GetModified().UTC()
		view.ModifiedAt = &modified
	}
	if deleted := item.Deleted.Get(); deleted != nil {
		value := deleted.UTC()
		view.DeletedAt = &value
	}
	if priority := item.Priority.Get(); priority != nil {
		value := *priority
		view.Priority = &value
	}
	if item.HasDraftStatus() {
		view.DraftStatus = string(item.GetDraftStatus())
	}
	if item.HasCreator() {
		creator := item.GetCreator()
		view.Creator = &creator
	}
	if roles := item.RestrictedRoles.Get(); roles != nil {
		view.RestrictedRoles = append([]string(nil), (*roles)...)
	}
	view.Raw = rawJSON(item)
	return view
}

func mapSearchItem(item datadogV1.MonitorSearchResult) SearchItem {
	view := SearchItem{
		ID:              item.GetId(),
		Name:            item.GetName(),
		Query:           item.GetQuery(),
		Type:            string(item.GetType()),
		Status:          string(item.GetStatus()),
		Tags:            append([]string(nil), item.Tags...),
		Scopes:          append([]string(nil), item.Scopes...),
		Classification:  item.GetClassification(),
		Metrics:         append([]string(nil), item.Metrics...),
		QualityIssues:   append([]string(nil), item.QualityIssues...),
		Notifications:   append([]datadogV1.MonitorSearchResultNotification(nil), item.Notifications...),
		LastTriggeredTs: nil,
		Raw:             rawJSON(item),
	}
	if item.HasCreator() {
		creator := item.GetCreator()
		view.Creator = &creator
	}
	if value := item.LastTriggeredTs.Get(); value != nil {
		copyValue := *value
		view.LastTriggeredTs = &copyValue
	}
	return view
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
