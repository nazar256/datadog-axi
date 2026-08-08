package spans

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
	"github.com/nazar256/datadog-axi/internal/timeutil"
)

type Service interface {
	Search(context.Context, cliruntime.Config, SearchParams) (SearchResult, error)
	Aggregate(context.Context, cliruntime.Config, AggregateParams) (AggregateResult, error)
}

type LiveService struct{}

type SearchParams struct {
	Query       string
	Range       timeutil.Range
	Limit       int32
	SortAsc     bool
	Cursor      string
	Service     string
	Env         string
	Operation   string
	Resource    string
	Status      string
	DurationMin *time.Duration
	DurationMax *time.Duration
	Tags        []string
	TraceID     string
	SpanID      string
}

// AggregateParams controls the bounded spans analytics endpoint. GroupBy uses
// the aliases service, resource, operation, and env. Compute currently
// accepts count; keeping this first surface narrow avoids ambiguous metric
// handling while preserving an obvious extension point.
type AggregateParams struct {
	Query       string
	Range       timeutil.Range
	GroupBy     []string
	Compute     []string
	BucketLimit int64
}

type Entry struct {
	ID             string                 `json:"id,omitempty"`
	Start          *time.Time             `json:"start,omitempty"`
	End            *time.Time             `json:"end,omitempty"`
	StartTimestamp *time.Time             `json:"start_timestamp,omitempty"`
	EndTimestamp   *time.Time             `json:"end_timestamp,omitempty"`
	Service        string                 `json:"service,omitempty"`
	Resource       string                 `json:"resource,omitempty"`
	ResourceName   string                 `json:"resource_name,omitempty"`
	Type           string                 `json:"type,omitempty"`
	Env            string                 `json:"env,omitempty"`
	Host           string                 `json:"host,omitempty"`
	DurationMS     *float64               `json:"duration_ms,omitempty"`
	Operation      string                 `json:"operation,omitempty"`
	Status         string                 `json:"status,omitempty"`
	Version        string                 `json:"version,omitempty"`
	TraceID        string                 `json:"trace_id,omitempty"`
	SpanID         string                 `json:"span_id,omitempty"`
	ParentID       string                 `json:"parent_id,omitempty"`
	Tags           []string               `json:"tags,omitempty"`
	Attributes     map[string]interface{} `json:"attributes,omitempty"`
	Raw            map[string]interface{} `json:"raw,omitempty"`
	Derived        []string               `json:"derived,omitempty"`
}

type SearchResult struct {
	Query      string    `json:"query"`
	From       time.Time `json:"from"`
	To         time.Time `json:"to"`
	Service    string    `json:"service,omitempty"`
	Env        string    `json:"env,omitempty"`
	Operation  string    `json:"operation,omitempty"`
	Resource   string    `json:"resource,omitempty"`
	Status     string    `json:"status,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
	Items      []Entry   `json:"items"`
	Count      int       `json:"count"`
	NextCursor string    `json:"next_cursor,omitempty"`
	Truncated  bool      `json:"truncated,omitempty"`
}

type AggregateBucket struct {
	ID       string                 `json:"id,omitempty"`
	By       map[string]interface{} `json:"by,omitempty"`
	Computes map[string]interface{} `json:"computes,omitempty"`
	Raw      map[string]interface{} `json:"raw,omitempty"`
}

type RateLimitInfo struct {
	Limit     string `json:"limit,omitempty"`
	Remaining string `json:"remaining,omitempty"`
	Reset     string `json:"reset,omitempty"`
}

type AggregateResult struct {
	Query             string            `json:"query"`
	From              time.Time         `json:"from"`
	To                time.Time         `json:"to"`
	GroupBy           []string          `json:"group_by"`
	Compute           []string          `json:"compute"`
	Buckets           []AggregateBucket `json:"buckets"`
	Count             int               `json:"count"`
	Status            string            `json:"status,omitempty"`
	RequestID         string            `json:"request_id,omitempty"`
	ElapsedMS         int64             `json:"elapsed_ms,omitempty"`
	Warnings          []string          `json:"warnings,omitempty"`
	RateLimit         *RateLimitInfo    `json:"rate_limit,omitempty"`
	PossiblyTruncated bool              `json:"possibly_truncated,omitempty"`
}

func (LiveService) Search(ctx context.Context, cfg cliruntime.Config, params SearchParams) (SearchResult, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return SearchResult{}, err
	}

	body := searchRequest(params)

	resp, _, err := datadogV2.NewSpansApi(client.API).ListSpans(client.Ctx, body)
	if err != nil {
		return SearchResult{}, cliruntime.WrapAPIError(err, cfg)
	}
	items := resp.GetData()
	views := make([]Entry, 0, len(items))
	for _, item := range items {
		views = append(views, mapSpan(item))
	}
	nextCursor := ""
	meta := resp.GetMeta()
	if page := meta.GetPage(); page.HasAfter() {
		nextCursor = page.GetAfter()
	}
	return SearchResult{Query: ComposeQuery(params), From: params.Range.From, To: params.Range.To, Service: params.Service, Env: params.Env, Operation: params.Operation, Resource: params.Resource, Status: params.Status, Tags: append([]string(nil), params.Tags...), Items: views, Count: len(views), NextCursor: nextCursor, Truncated: nextCursor != ""}, nil
}

func (LiveService) Aggregate(ctx context.Context, cfg cliruntime.Config, params AggregateParams) (AggregateResult, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return AggregateResult{}, err
	}
	body, groupLabels, computeLabels, err := aggregateRequest(params)
	if err != nil {
		return AggregateResult{}, err
	}
	query := ComposeAggregateQuery(params.Query)
	resp, httpResp, err := datadogV2.NewSpansApi(client.API).AggregateSpans(client.Ctx, body)
	if err != nil {
		return AggregateResult{}, cliruntime.WrapAPIError(err, cfg)
	}
	result := AggregateResult{Query: query, From: params.Range.From, To: params.Range.To, GroupBy: groupLabels, Compute: computeLabels}
	result.Buckets = make([]AggregateBucket, 0, len(resp.GetData()))
	for _, bucket := range resp.GetData() {
		result.Buckets = append(result.Buckets, mapAggregateBucket(bucket))
	}
	result.Count = len(result.Buckets)
	if result.Count > 0 && params.BucketLimit > 0 && int64(result.Count) >= params.BucketLimit {
		result.PossiblyTruncated = true
		result.Warnings = append(result.Warnings, fmt.Sprintf("result reached the --limit bound of %d buckets; widen it explicitly if needed", params.BucketLimit))
	}
	meta := resp.GetMeta()
	if meta.HasStatus() {
		result.Status = string(meta.GetStatus())
	}
	if meta.HasRequestId() {
		result.RequestID = meta.GetRequestId()
	}
	if meta.HasElapsed() {
		result.ElapsedMS = meta.GetElapsed()
	}
	for _, warning := range meta.GetWarnings() {
		message := strings.TrimSpace(warning.GetDetail())
		if message == "" {
			message = strings.TrimSpace(warning.GetTitle())
		}
		if warning.GetCode() != "" && message != "" {
			message = warning.GetCode() + ": " + message
		}
		if message != "" {
			result.Warnings = append(result.Warnings, message)
		}
	}
	result.RateLimit = readRateLimit(httpResp)
	result.Warnings = append(result.Warnings, "AggregateSpans is rate-limited by Datadog to 300 requests per hour")
	return result, nil
}

func searchRequest(params SearchParams) datadogV2.SpansListRequest {
	filter := datadogV2.NewSpansQueryFilter()
	filter.SetFrom(params.Range.From.Format(time.RFC3339Nano))
	filter.SetTo(params.Range.To.Format(time.RFC3339Nano))
	filter.SetQuery(ComposeQuery(params))
	page := datadogV2.NewSpansListRequestPage()
	if params.Limit > 0 {
		page.SetLimit(params.Limit)
	}
	if params.Cursor != "" {
		page.SetCursor(params.Cursor)
	}
	attrs := datadogV2.NewSpansListRequestAttributes()
	attrs.SetFilter(*filter)
	attrs.SetPage(*page)
	if params.SortAsc {
		attrs.SetSort(datadogV2.SPANSSORT_TIMESTAMP_ASCENDING)
	} else {
		attrs.SetSort(datadogV2.SPANSSORT_TIMESTAMP_DESCENDING)
	}
	body := datadogV2.NewSpansListRequest()
	body.SetData(datadogV2.SpansListRequestData{Attributes: attrs, Type: datadogV2.SPANSLISTREQUESTTYPE_SEARCH_REQUEST.Ptr()})
	return *body
}

func aggregateRequest(params AggregateParams) (datadogV2.SpansAggregateRequest, []string, []string, error) {
	groupBy, groupLabels, err := aggregateGroupBy(params.GroupBy, params.BucketLimit)
	if err != nil {
		return datadogV2.SpansAggregateRequest{}, nil, nil, err
	}
	compute, computeLabels, err := aggregateComputes(params.Compute)
	if err != nil {
		return datadogV2.SpansAggregateRequest{}, nil, nil, err
	}
	filter := datadogV2.NewSpansQueryFilter()
	filter.SetFrom(params.Range.From.Format(time.RFC3339Nano))
	filter.SetTo(params.Range.To.Format(time.RFC3339Nano))
	filter.SetQuery(ComposeAggregateQuery(params.Query))
	attrs := datadogV2.NewSpansAggregateRequestAttributes()
	attrs.SetFilter(*filter)
	attrs.SetGroupBy(groupBy)
	attrs.SetCompute(compute)
	data := datadogV2.NewSpansAggregateData()
	data.SetAttributes(*attrs)
	body := datadogV2.NewSpansAggregateRequest()
	body.SetData(*data)
	return *body, groupLabels, computeLabels, nil
}

// mapSpan derives operation in the order operation_name, operation,
// span.operation, span.name, name; version in the order version,
// service_version, service.version, deployment_version, deployment.version;
// and status in the order status, span.status,
// error.status, then a boolean/numeric error flag. The complete SDK object is
// retained under Raw so derived values never replace source attributes.
func mapSpan(item datadogV2.Span) Entry {
	attrs := item.GetAttributes()
	attributes := cloneAttributes(attrs.GetAttributes())
	for key, value := range attrs.GetCustom() {
		if attributes == nil {
			attributes = make(map[string]interface{})
		}
		attributes[key] = value
	}
	resource := attrs.GetResourceName()
	view := Entry{
		ID:           item.GetId(),
		Service:      attrs.GetService(),
		Resource:     resource,
		ResourceName: resource,
		Type:         attrs.GetType(),
		Env:          attrs.GetEnv(),
		Host:         attrs.GetHost(),
		TraceID:      attrs.GetTraceId(),
		SpanID:       attrs.GetSpanId(),
		ParentID:     attrs.GetParentId(),
		Tags:         append([]string{}, attrs.GetTags()...),
		Attributes:   attributes,
		Raw:          rawJSON(item),
	}
	view.Operation = firstString(attributes, "operation_name", "operation", "span.operation", "span.name", "name")
	view.Version = firstString(attributes, "version", "service_version", "service.version", "deployment_version", "deployment.version")
	view.Status = derivedStatus(attributes)
	if view.Operation != "" {
		view.Derived = append(view.Derived, "operation")
	}
	if view.Version != "" {
		view.Derived = append(view.Derived, "version")
	}
	if view.Status != "" {
		view.Derived = append(view.Derived, "status")
	}
	if attrs.HasStartTimestamp() {
		value := attrs.GetStartTimestamp().UTC()
		view.Start = &value
		view.StartTimestamp = &value
	}
	if attrs.HasEndTimestamp() {
		value := attrs.GetEndTimestamp().UTC()
		view.End = &value
		view.EndTimestamp = &value
	}
	if view.Start != nil && view.End != nil {
		milliseconds := float64(view.End.Sub(*view.Start)) / float64(time.Millisecond)
		if milliseconds >= 0 {
			view.DurationMS = &milliseconds
			view.Derived = append(view.Derived, "duration_ms")
		}
	}
	return view
}

// ComposeQuery adds explicit filters using Datadog's span search syntax. The
// caller's query is kept verbatim so advanced expressions remain available;
// values supplied through dedicated flags are escaped as one term.
func ComposeQuery(params SearchParams) string {
	parts := make([]string, 0, 12)
	if query := strings.TrimSpace(params.Query); query != "" {
		parts = append(parts, query)
	}
	appendQueryFilter(&parts, "service", params.Service)
	appendQueryFilter(&parts, "env", params.Env)
	appendQueryFilter(&parts, "operation_name", params.Operation)
	appendQueryFilter(&parts, "resource_name", params.Resource)
	appendQueryFilter(&parts, "status", params.Status)
	if params.DurationMin != nil {
		parts = append(parts, "duration:>"+formatDuration(*params.DurationMin))
	}
	if params.DurationMax != nil {
		parts = append(parts, "duration:<"+formatDuration(*params.DurationMax))
	}
	for _, tag := range params.Tags {
		if strings.TrimSpace(tag) != "" {
			// Span tag queries use the supplied key:value term directly;
			// unlike custom attributes they are not prefixed with @.
			parts = append(parts, escapeQueryValue(tag))
		}
	}
	appendQueryFilter(&parts, "trace_id", params.TraceID)
	appendQueryFilter(&parts, "span_id", params.SpanID)
	if len(parts) == 0 {
		return "*"
	}
	return strings.Join(parts, " ")
}

func ComposeAggregateQuery(query string) string {
	if strings.TrimSpace(query) == "" {
		return "*"
	}
	return strings.TrimSpace(query)
}

func appendQueryFilter(parts *[]string, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	*parts = append(*parts, key+":"+escapeQueryValue(value))
}

func escapeQueryValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return `""`
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("._:/@-*?", r) {
			continue
		}
		value = strings.ReplaceAll(value, `\`, `\\`)
		value = strings.ReplaceAll(value, `"`, `\"`)
		return `"` + value + `"`
	}
	return value
}

func formatDuration(value time.Duration) string {
	if value%time.Second == 0 {
		return strconv.FormatInt(int64(value/time.Second), 10) + "s"
	}
	return value.String()
}

func aggregateGroupBy(values []string, limit int64) ([]datadogV2.SpansGroupBy, []string, error) {
	if len(values) == 0 {
		values = []string{"service"}
	}
	if len(values) > 4 {
		return nil, nil, fmt.Errorf("at most four span aggregate group-bys are supported")
	}
	groups := make([]datadogV2.SpansGroupBy, 0, len(values))
	labels := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		label := strings.ToLower(strings.TrimSpace(value))
		facet := label
		switch label {
		case "service", "env":
		case "resource":
			facet = "resource_name"
		case "operation":
			facet = "operation_name"
		case "environment":
			label, facet = "env", "env"
		default:
			return nil, nil, fmt.Errorf("unsupported span aggregate group-by %q (use service, resource, operation, or env)", value)
		}
		if _, ok := seen[facet]; ok {
			continue
		}
		seen[facet] = struct{}{}
		group := datadogV2.NewSpansGroupBy(facet)
		if limit > 0 {
			group.SetLimit(limit)
		}
		groups = append(groups, *group)
		labels = append(labels, label)
	}
	return groups, labels, nil
}

func aggregateComputes(values []string) ([]datadogV2.SpansCompute, []string, error) {
	if len(values) == 0 {
		values = []string{"count"}
	}
	computes := make([]datadogV2.SpansCompute, 0, len(values))
	labels := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		label := strings.ToLower(strings.TrimSpace(value))
		if label != "count" {
			return nil, nil, fmt.Errorf("unsupported span aggregate compute %q (only count is currently supported)", value)
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		computes = append(computes, *datadogV2.NewSpansCompute(datadogV2.SPANSAGGREGATIONFUNCTION_COUNT))
		labels = append(labels, label)
	}
	return computes, labels, nil
}

func mapAggregateBucket(bucket datadogV2.SpansAggregateBucket) AggregateBucket {
	attrs := bucket.GetAttributes()
	computes := make(map[string]interface{}, len(attrs.GetComputes()))
	for key, value := range attrs.GetComputes() {
		computes[key] = value.GetActualInstance()
	}
	if attrs.HasCompute() && len(computes) == 0 {
		computes["compute"] = attrs.GetCompute()
	}
	return AggregateBucket{ID: bucket.GetId(), By: cloneAttributes(attrs.GetBy()), Computes: computes, Raw: rawJSON(bucket)}
}

func rawJSON(value interface{}) map[string]interface{} {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	result := make(map[string]interface{})
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil
	}
	return result
}

func firstString(values map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			if text := strings.TrimSpace(fmt.Sprint(value)); text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

func derivedStatus(values map[string]interface{}) string {
	if value := firstString(values, "status", "span.status", "error.status"); value != "" {
		return value
	}
	if value, ok := values["error"]; ok {
		switch parsed := value.(type) {
		case bool:
			if parsed {
				return "error"
			}
			return "ok"
		case float64:
			if parsed != 0 {
				return "error"
			}
			return "ok"
		}
	}
	return ""
}

func readRateLimit(response *http.Response) *RateLimitInfo {
	if response == nil {
		return nil
	}
	info := &RateLimitInfo{Limit: response.Header.Get("X-RateLimit-Limit"), Remaining: response.Header.Get("X-RateLimit-Remaining"), Reset: response.Header.Get("X-RateLimit-Reset")}
	if info.Limit == "" && info.Remaining == "" && info.Reset == "" {
		return nil
	}
	return info
}

func cloneAttributes(values map[string]interface{}) map[string]interface{} {
	if values == nil {
		return nil
	}
	result := make(map[string]interface{}, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
