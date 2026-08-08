package downtime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
)

type Service interface {
	List(context.Context, cliruntime.Config, ListParams) (ListResult, error)
	Get(context.Context, cliruntime.Config, string) (Detail, error)
}
type LiveService struct{}

type ListParams struct {
	CurrentOnly bool
	Include     string
	Offset      int64
	Limit       int64
}

type Summary struct {
	ID       string     `json:"id"`
	Status   string     `json:"status,omitempty"`
	Scope    string     `json:"scope,omitempty"`
	Message  string     `json:"message,omitempty"`
	Created  *time.Time `json:"created,omitempty"`
	Modified *time.Time `json:"modified,omitempty"`
}

// Detail is the stable read projection returned by downtime get. Schedule,
// relationship, and included resources remain JSON-compatible values because
// the SDK models are unions whose shape varies by downtime configuration.
type Detail struct {
	Summary
	Canceled                      *time.Time       `json:"canceled,omitempty"`
	DisplayTimezone               string           `json:"display_timezone,omitempty"`
	MonitorID                     *int64           `json:"monitor_id,omitempty"`
	MonitorTags                   []string         `json:"monitor_tags,omitempty"`
	MuteFirstRecoveryNotification bool             `json:"mute_first_recovery_notification"`
	NotifyEndStates               []string         `json:"notify_end_states,omitempty"`
	NotifyEndTypes                []string         `json:"notify_end_types,omitempty"`
	Schedule                      map[string]any   `json:"schedule,omitempty"`
	Relationships                 map[string]any   `json:"relationships,omitempty"`
	Included                      []map[string]any `json:"included,omitempty"`
}
type ListResult struct {
	Items             []Summary        `json:"items"`
	Count             int              `json:"count"`
	TotalFiltered     int64            `json:"total_filtered,omitempty"`
	PossiblyTruncated bool             `json:"possibly_truncated,omitempty"`
	Included          []map[string]any `json:"included,omitempty"`
}

func (LiveService) List(ctx context.Context, cfg cliruntime.Config, params ListParams) (ListResult, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return ListResult{}, err
	}
	return listWithClient(client, cfg, params)
}

func listWithClient(client *cliruntime.Client, cfg cliruntime.Config, params ListParams) (ListResult, error) {
	if client == nil {
		return ListResult{}, fmt.Errorf("downtime client must not be nil")
	}
	opt := datadogV2.NewListDowntimesOptionalParameters().WithCurrentOnly(params.CurrentOnly)
	if params.Include != "" {
		opt.WithInclude(params.Include)
	}
	if params.Offset > 0 {
		opt.WithPageOffset(params.Offset)
	}
	if params.Limit > 0 {
		opt.WithPageLimit(params.Limit)
	}
	resp, _, err := datadogV2.NewDowntimesApi(client.API).ListDowntimes(client.Ctx, *opt)
	if err != nil {
		return ListResult{}, cliruntime.WrapAPIError(err, client.Config)
	}
	items := resp.GetData()
	result := ListResult{Items: make([]Summary, 0, len(items)), Count: len(items), Included: mapIncluded(resp.GetIncluded())}
	if meta, ok := resp.GetMetaOk(); ok {
		if page, ok := meta.GetPageOk(); ok {
			result.TotalFiltered = page.GetTotalFilteredCount()
			result.PossiblyTruncated = result.TotalFiltered > int64(result.Count)
		}
	}
	for _, item := range items {
		result.Items = append(result.Items, mapDowntimeSummary(item))
	}
	return result, nil
}

func (LiveService) Get(ctx context.Context, cfg cliruntime.Config, id string) (Detail, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return Detail{}, err
	}
	return getWithClient(client, cfg, id)
}

func getWithClient(client *cliruntime.Client, cfg cliruntime.Config, id string) (Detail, error) {
	if client == nil {
		return Detail{}, fmt.Errorf("downtime client must not be nil")
	}
	opt := datadogV2.NewGetDowntimeOptionalParameters()
	resp, _, err := datadogV2.NewDowntimesApi(client.API).GetDowntime(client.Ctx, id, *opt)
	if err != nil {
		return Detail{}, cliruntime.WrapAPIError(err, client.Config)
	}
	return mapDowntimeDetail(resp), nil
}

func mapIncluded(items []datadogV2.DowntimeResponseIncludedItem) []map[string]any {
	if len(items) == 0 {
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if value := mapObject(item); value != nil {
			result = append(result, value)
		}
	}
	return result
}

func mapDowntimeDetail(resp datadogV2.DowntimeResponse) Detail {
	item := resp.GetData()
	attrs := item.GetAttributes()
	detail := Detail{
		Summary:                       mapDowntimeSummary(item),
		DisplayTimezone:               attrs.GetDisplayTimezone(),
		MuteFirstRecoveryNotification: attrs.GetMuteFirstRecoveryNotification(),
		NotifyEndStates:               mapDowntimeEndStates(attrs.GetNotifyEndStates()),
		NotifyEndTypes:                mapDowntimeEndTypes(attrs.GetNotifyEndTypes()),
		Included:                      mapIncluded(resp.GetIncluded()),
	}
	if attrs.HasCanceled() {
		if canceled := attrs.GetCanceled(); !canceled.IsZero() {
			value := canceled.UTC()
			detail.Canceled = &value
		}
	}
	if attrs.HasMonitorIdentifier() {
		identifier := attrs.GetMonitorIdentifier()
		switch value := (&identifier).GetActualInstance().(type) {
		case *datadogV2.DowntimeMonitorIdentifierId:
			monitorID := value.GetMonitorId()
			detail.MonitorID = &monitorID
		case *datadogV2.DowntimeMonitorIdentifierTags:
			detail.MonitorTags = append([]string{}, value.GetMonitorTags()...)
		}
	}
	if attrs.HasSchedule() {
		detail.Schedule = mapObject(attrs.GetSchedule())
	}
	if item.HasRelationships() {
		detail.Relationships = mapObject(item.GetRelationships())
	}
	return detail
}

func mapDowntimeSummary(item datadogV2.DowntimeResponseData) Summary {
	attrs := item.GetAttributes()
	summary := Summary{ID: item.GetId(), Status: string(attrs.GetStatus()), Scope: attrs.GetScope(), Message: attrs.GetMessage()}
	if attrs.HasCreated() {
		value := attrs.GetCreated().UTC()
		summary.Created = &value
	}
	if attrs.HasModified() {
		value := attrs.GetModified().UTC()
		summary.Modified = &value
	}
	return summary
}

func mapDowntimeEndStates(values []datadogV2.DowntimeNotifyEndStateTypes) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return result
}

func mapDowntimeEndTypes(values []datadogV2.DowntimeNotifyEndStateActions) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return result
}

func mapObject(value any) map[string]any {
	data, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil || result == nil {
		return nil
	}
	return result
}
