package audit

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
	"github.com/nazar256/datadog-axi/internal/timeutil"
)

type Service interface {
	Search(context.Context, cliruntime.Config, SearchParams) (SearchResult, error)
}

type LiveService struct{}

type SearchParams struct {
	Query    string
	Range    timeutil.Range
	Limit    int32
	SortAsc  bool
	Cursor   string
	Actor    string
	Service  string
	Action   string
	Resource string
	Tags     []string
}

type Entry struct {
	ID                     string                 `json:"id,omitempty"`
	Type                   string                 `json:"type,omitempty"`
	Timestamp              *time.Time             `json:"timestamp,omitempty"`
	Service                string                 `json:"service,omitempty"`
	Actor                  string                 `json:"actor,omitempty"`
	Action                 string                 `json:"action,omitempty"`
	Resource               string                 `json:"resource,omitempty"`
	Message                string                 `json:"message,omitempty"`
	Tags                   []string               `json:"tags,omitempty"`
	Attributes             map[string]interface{} `json:"attributes,omitempty"`
	ChangedFields          []string               `json:"changed_fields,omitempty"`
	MonitorDashboardChange bool                   `json:"monitor_dashboard_change,omitempty"`
}

type SearchResult struct {
	Query      string    `json:"query"`
	From       time.Time `json:"from"`
	To         time.Time `json:"to"`
	Actor      string    `json:"actor,omitempty"`
	Service    string    `json:"service,omitempty"`
	Action     string    `json:"action,omitempty"`
	Resource   string    `json:"resource,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
	Items      []Entry   `json:"items"`
	Count      int       `json:"count"`
	NextCursor string    `json:"next_cursor,omitempty"`
	Truncated  bool      `json:"truncated,omitempty"`
}

func (LiveService) Search(ctx context.Context, cfg cliruntime.Config, params SearchParams) (SearchResult, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return SearchResult{}, err
	}
	return searchWithClient(client, cfg, params)
}

// searchWithClient keeps request construction and response projection together
// while allowing the adapter contract to be exercised against a controlled
// HTTP endpoint in tests.
func searchWithClient(client *cliruntime.Client, cfg cliruntime.Config, params SearchParams) (SearchResult, error) {
	if client == nil {
		return SearchResult{}, fmt.Errorf("audit client must not be nil")
	}

	filter := datadogV2.NewAuditLogsQueryFilter()
	filter.SetFrom(params.Range.From.Format(time.RFC3339Nano))
	filter.SetTo(params.Range.To.Format(time.RFC3339Nano))
	query := ComposeQuery(params)
	if query == "" {
		query = "*"
	}
	filter.SetQuery(query)
	page := datadogV2.NewAuditLogsQueryPageOptions()
	if params.Limit > 0 {
		page.SetLimit(params.Limit)
	}
	if params.Cursor != "" {
		page.SetCursor(params.Cursor)
	}
	body := datadogV2.NewAuditLogsSearchEventsRequest()
	body.SetFilter(*filter)
	body.SetPage(*page)
	body.SetOptions(*datadogV2.NewAuditLogsQueryOptions())
	if params.SortAsc {
		body.SetSort(datadogV2.AUDITLOGSSORT_TIMESTAMP_ASCENDING)
	} else {
		body.SetSort(datadogV2.AUDITLOGSSORT_TIMESTAMP_DESCENDING)
	}

	resp, _, err := datadogV2.NewAuditApi(client.API).SearchAuditLogs(client.Ctx, *datadogV2.NewSearchAuditLogsOptionalParameters().WithBody(*body))
	if err != nil {
		return SearchResult{}, cliruntime.WrapAPIError(err, client.Config)
	}
	items := resp.GetData()
	views := make([]Entry, 0, len(items))
	for _, item := range items {
		views = append(views, mapEvent(item))
	}
	nextCursor := ""
	meta := resp.GetMeta()
	if page := meta.GetPage(); page.HasAfter() {
		nextCursor = page.GetAfter()
	}
	return SearchResult{Query: query, From: params.Range.From, To: params.Range.To, Actor: params.Actor, Service: params.Service, Action: params.Action, Resource: params.Resource, Tags: append([]string(nil), params.Tags...), Items: views, Count: len(views), NextCursor: nextCursor, Truncated: nextCursor != ""}, nil
}

func ComposeQuery(params SearchParams) string {
	parts := []string{strings.TrimSpace(params.Query)}
	for _, pair := range []struct{ key, value string }{{"@usr.email", params.Actor}, {"service", params.Service}, {"@evt.name", params.Action}, {"@resource.name", params.Resource}} {
		if value := strings.TrimSpace(pair.value); value != "" {
			value = quoteAuditValue(value)
			parts = append(parts, fmt.Sprintf("%s:%s", pair.key, value))
		}
	}
	for _, tag := range params.Tags {
		if value := strings.TrimSpace(tag); value != "" {
			parts = append(parts, quoteAuditValue(value))
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func quoteAuditValue(value string) string {
	for _, r := range value {
		if r <= ' ' || strings.ContainsRune(`"\\`, r) {
			return strconv.Quote(value)
		}
	}
	return value
}

func mapEvent(item datadogV2.AuditLogsEvent) Entry {
	attrs := item.GetAttributes()
	view := Entry{
		ID:         item.GetId(),
		Type:       string(item.GetType()),
		Service:    attrs.GetService(),
		Actor:      firstAttribute(attrs.GetAttributes(), "actor", "usr.email", "user.email", "user"),
		Action:     firstAttribute(attrs.GetAttributes(), "action", "evt.name", "event", "operation"),
		Resource:   firstAttribute(attrs.GetAttributes(), "resource", "target.name", "target", "resource_name"),
		Message:    attrs.GetMessage(),
		Tags:       append([]string{}, attrs.GetTags()...),
		Attributes: cloneAttributes(attrs.GetAttributes()),
	}
	if changes, ok := attrs.GetAttributes()["changes"].(map[string]interface{}); ok {
		for field := range changes {
			view.ChangedFields = append(view.ChangedFields, field)
		}
	}
	view.MonitorDashboardChange = strings.Contains(strings.ToLower(view.Service+" "+view.Resource), "monitor") || strings.Contains(strings.ToLower(view.Service+" "+view.Resource), "dashboard")
	if attrs.HasTimestamp() {
		timestamp := attrs.GetTimestamp().UTC()
		view.Timestamp = &timestamp
	}
	return view
}

func firstAttribute(values map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

func cloneAttributes(values map[string]interface{}) map[string]interface{} {
	if values == nil {
		return nil
	}
	cloned := make(map[string]interface{}, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
