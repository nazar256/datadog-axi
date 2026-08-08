package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
	"github.com/nazar256/datadog-axi/internal/timeutil"
)

const (
	defaultMaxPages = 10
	maxAllowedPages = 100
)
const maximumMaxPages = 100

type Service interface {
	Search(context.Context, cliruntime.Config, SearchParams) (SearchResult, error)
}

type AggregateService interface {
	Aggregate(context.Context, cliruntime.Config, AggregateParams) (AggregateResult, error)
}

type LiveService struct{}

// SearchParams controls the POST log search. AllPages is deliberately opt-in;
// MaxPages bounds the number of requests when it is enabled.
type SearchParams struct {
	Query       string
	Range       timeutil.Range
	Limit       int32
	Indexes     []string
	StorageTier string
	SortAsc     bool
	Cursor      string
	AllPages    bool
	MaxPages    int
}

type Entry struct {
	ID         string                 `json:"id,omitempty"`
	Type       string                 `json:"type,omitempty"`
	Timestamp  *time.Time             `json:"timestamp,omitempty"`
	Service    string                 `json:"service,omitempty"`
	Status     string                 `json:"status,omitempty"`
	Host       string                 `json:"host,omitempty"`
	Message    string                 `json:"message,omitempty"`
	Tags       []string               `json:"tags,omitempty"`
	TraceID    string                 `json:"trace_id,omitempty"`
	SpanID     string                 `json:"span_id,omitempty"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
	Raw        map[string]interface{} `json:"raw,omitempty"`
}

type SearchResult struct {
	Query        string    `json:"query"`
	From         time.Time `json:"from"`
	To           time.Time `json:"to"`
	Indexes      []string  `json:"indexes,omitempty"`
	StorageTier  string    `json:"storage_tier,omitempty"`
	SortAsc      bool      `json:"sort_ascending"`
	Items        []Entry   `json:"items"`
	Count        int       `json:"count"`
	NextCursor   string    `json:"next_cursor,omitempty"`
	PagesFetched int       `json:"pages_fetched"`
	Truncated    bool      `json:"truncated"`
}

type FacetSpec struct {
	Facet string `json:"facet"`
	Limit int64  `json:"limit,omitempty"`
}

type ComputeSpec struct {
	Aggregation string `json:"aggregation"`
	Metric      string `json:"metric,omitempty"`
	Type        string `json:"type,omitempty"`
	Interval    string `json:"interval,omitempty"`
}

type AggregateParams struct {
	Query       string
	Range       timeutil.Range
	Indexes     []string
	StorageTier string
	Cursor      string
	Facets      []FacetSpec
	Computes    []ComputeSpec
	AllPages    bool
	MaxPages    int
}

type Bucket struct {
	By       map[string]interface{} `json:"by,omitempty"`
	Computes map[string]interface{} `json:"computes,omitempty"`
	Raw      map[string]interface{} `json:"raw,omitempty"`
}

type Warning struct {
	Code   string `json:"code,omitempty"`
	Title  string `json:"title,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type AggregateResult struct {
	Query        string        `json:"query"`
	From         time.Time     `json:"from"`
	To           time.Time     `json:"to"`
	Facets       []FacetSpec   `json:"facets,omitempty"`
	Computes     []ComputeSpec `json:"computes,omitempty"`
	Buckets      []Bucket      `json:"buckets"`
	Count        int           `json:"count"`
	NextCursor   string        `json:"next_cursor,omitempty"`
	PagesFetched int           `json:"pages_fetched"`
	Truncated    bool          `json:"truncated"`
	Warnings     []Warning     `json:"warnings,omitempty"`
}

func (s LiveService) Search(ctx context.Context, cfg cliruntime.Config, params SearchParams) (SearchResult, error) {
	if err := validateSearchParams(params); err != nil {
		return SearchResult{}, err
	}
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return SearchResult{}, err
	}
	api := datadogV2.NewLogsApi(client.API)
	result := SearchResult{Query: params.Query, From: params.Range.From, To: params.Range.To, Indexes: append([]string(nil), params.Indexes...), StorageTier: params.StorageTier, SortAsc: params.SortAsc}
	cursor := params.Cursor
	maxPages := pageLimit(params.AllPages, params.MaxPages)
	for page := 0; page < maxPages; page++ {
		response, postErr := listPOST(client, api, params, cursor)
		if postErr != nil {
			if !isPOSTCompatibilityError(postErr) {
				return SearchResult{}, cliruntime.WrapAPIError(postErr, cfg)
			}
			response, postErr = listGET(client, api, params, cursor)
			if postErr != nil {
				return SearchResult{}, cliruntime.WrapAPIError(postErr, cfg)
			}
		}
		items := response.GetData()
		for _, item := range items {
			result.Items = append(result.Items, mapEntry(item))
		}
		result.PagesFetched++
		cursor = responseCursor(response)
		if cursor == "" || !params.AllPages {
			break
		}
	}
	result.Count = len(result.Items)
	result.NextCursor = cursor
	result.Truncated = cursor != ""
	return result, nil
}

func (s LiveService) Aggregate(ctx context.Context, cfg cliruntime.Config, params AggregateParams) (AggregateResult, error) {
	if err := validateAggregateParams(params); err != nil {
		return AggregateResult{}, err
	}
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return AggregateResult{}, err
	}
	api := datadogV2.NewLogsApi(client.API)
	result := AggregateResult{Query: params.Query, From: params.Range.From, To: params.Range.To, Facets: params.Facets, Computes: params.Computes}
	cursor := params.Cursor
	maxPages := pageLimit(params.AllPages, params.MaxPages)
	for page := 0; page < maxPages; page++ {
		request := aggregateRequest(params, cursor)
		response, _, callErr := api.AggregateLogs(client.Ctx, request)
		if callErr != nil {
			return AggregateResult{}, cliruntime.WrapAPIError(callErr, cfg)
		}
		if data, ok := response.GetDataOk(); ok {
			for _, item := range data.GetBuckets() {
				result.Buckets = append(result.Buckets, mapBucket(item))
			}
		}
		if meta, ok := response.GetMetaOk(); ok {
			for _, warning := range meta.GetWarnings() {
				result.Warnings = append(result.Warnings, Warning{Code: warning.GetCode(), Title: warning.GetTitle(), Detail: warning.GetDetail()})
			}
			pageInfo := meta.GetPage()
			cursor = pageInfo.GetAfter()
		}
		result.PagesFetched++
		if cursor == "" || !params.AllPages {
			break
		}
	}
	result.Count = len(result.Buckets)
	result.NextCursor = cursor
	result.Truncated = cursor != ""
	return result, nil
}

func validateSearchParams(params SearchParams) error {
	if strings.TrimSpace(params.Query) == "" {
		return fmt.Errorf("query cannot be empty")
	}
	if params.Limit < 0 || params.Limit > 1000 {
		return fmt.Errorf("limit must be between 0 and 1000")
	}
	if params.MaxPages < 0 || params.MaxPages > maxAllowedPages {
		return fmt.Errorf("max pages must be between 0 and %d", maxAllowedPages)
	}
	if params.StorageTier != "" {
		if _, err := datadogV2.NewLogsStorageTierFromValue(params.StorageTier); err != nil {
			return fmt.Errorf("invalid storage tier %q", params.StorageTier)
		}
	}
	return nil
}

func listPOST(client *cliruntime.Client, api *datadogV2.LogsApi, params SearchParams, cursor string) (datadogV2.LogsListResponse, error) {
	filter := queryFilter(params.Query, params.Range, params.Indexes, params.StorageTier)
	page := datadogV2.NewLogsListRequestPageWithDefaults()
	if params.Limit > 0 {
		page.SetLimit(params.Limit)
	}
	if cursor != "" {
		page.SetCursor(cursor)
	}
	sort := datadogV2.LOGSSORT_TIMESTAMP_DESCENDING
	if params.SortAsc {
		sort = datadogV2.LOGSSORT_TIMESTAMP_ASCENDING
	}
	request := datadogV2.NewLogsListRequestWithDefaults()
	request.SetFilter(filter)
	request.SetPage(*page)
	request.SetSort(sort)
	response, _, err := api.ListLogs(client.Ctx, *datadogV2.NewListLogsOptionalParameters().WithBody(*request))
	return response, err
}

func listGET(client *cliruntime.Client, api *datadogV2.LogsApi, params SearchParams, cursor string) (datadogV2.LogsListResponse, error) {
	opt := datadogV2.NewListLogsGetOptionalParameters().WithFilterQuery(params.Query).WithFilterFrom(params.Range.From).WithFilterTo(params.Range.To)
	if params.Limit > 0 {
		opt.WithPageLimit(params.Limit)
	}
	if cursor != "" {
		opt.WithPageCursor(cursor)
	}
	if len(params.Indexes) > 0 {
		opt.WithFilterIndexes(params.Indexes)
	}
	if params.StorageTier != "" {
		tier, _ := datadogV2.NewLogsStorageTierFromValue(params.StorageTier)
		if tier != nil {
			opt.WithFilterStorageTier(*tier)
		}
	}
	if params.SortAsc {
		opt.WithSort(datadogV2.LOGSSORT_TIMESTAMP_ASCENDING)
	} else {
		opt.WithSort(datadogV2.LOGSSORT_TIMESTAMP_DESCENDING)
	}
	response, _, err := api.ListLogsGet(client.Ctx, *opt)
	return response, err
}

func aggregateRequest(params AggregateParams, cursor string) datadogV2.LogsAggregateRequest {
	filter := queryFilter(params.Query, params.Range, params.Indexes, params.StorageTier)
	request := datadogV2.NewLogsAggregateRequestWithDefaults()
	request.SetFilter(filter)
	groups := make([]datadogV2.LogsGroupBy, 0, len(params.Facets))
	for _, facet := range params.Facets {
		group := datadogV2.NewLogsGroupBy(facet.Facet)
		if facet.Limit > 0 {
			group.SetLimit(facet.Limit)
		}
		groups = append(groups, *group)
	}
	request.SetGroupBy(groups)
	computes := make([]datadogV2.LogsCompute, 0, len(params.Computes))
	for _, compute := range params.Computes {
		aggregation, _ := datadogV2.NewLogsAggregationFunctionFromValue(compute.Aggregation)
		item := datadogV2.NewLogsCompute(*aggregation)
		if compute.Metric != "" {
			item.SetMetric(compute.Metric)
		}
		if compute.Type != "" {
			typeValue, _ := datadogV2.NewLogsComputeTypeFromValue(compute.Type)
			item.SetType(*typeValue)
		}
		if compute.Interval != "" {
			item.SetInterval(compute.Interval)
		}
		computes = append(computes, *item)
	}
	request.SetCompute(computes)
	if cursor != "" {
		page := datadogV2.NewLogsAggregateRequestPage()
		page.SetCursor(cursor)
		request.SetPage(*page)
	}
	return *request
}

func queryFilter(query string, r timeutil.Range, indexes []string, tier string) datadogV2.LogsQueryFilter {
	filter := datadogV2.NewLogsQueryFilterWithDefaults()
	filter.SetQuery(query)
	filter.SetFrom(r.From.UTC().Format(time.RFC3339Nano))
	filter.SetTo(r.To.UTC().Format(time.RFC3339Nano))
	if len(indexes) > 0 {
		filter.SetIndexes(append([]string(nil), indexes...))
	}
	if tier != "" {
		storageTier, _ := datadogV2.NewLogsStorageTierFromValue(tier)
		if storageTier != nil {
			filter.SetStorageTier(*storageTier)
		}
	}
	return *filter
}

func responseCursor(response datadogV2.LogsListResponse) string {
	meta := response.GetMeta()
	page := meta.GetPage()
	return page.GetAfter()
}

func mapEntry(item datadogV2.Log) Entry {
	attrs := item.GetAttributes()
	view := Entry{
		ID:         item.GetId(),
		Type:       string(item.GetType()),
		Service:    attrs.GetService(),
		Status:     attrs.GetStatus(),
		Host:       attrs.GetHost(),
		Message:    attrs.GetMessage(),
		Tags:       append([]string{}, attrs.GetTags()...),
		Attributes: cloneAttributes(attrs.GetAttributes()),
		Raw:        redactMap(rawObject(item)),
	}
	if attrs.HasTimestamp() {
		timestamp := attrs.GetTimestamp().UTC()
		view.Timestamp = &timestamp
	}
	if values := attrs.GetAttributes(); values != nil {
		view.TraceID = stringAttribute(values, "trace_id")
		view.SpanID = stringAttribute(values, "span_id")
	}
	return view
}

func mapBucket(bucket datadogV2.LogsAggregateBucket) Bucket {
	raw := redactMap(rawObject(bucket))
	result := Bucket{Raw: raw}
	if values, ok := raw["by"].(map[string]interface{}); ok {
		result.By = values
	}
	if values, ok := raw["computes"].(map[string]interface{}); ok {
		result.Computes = values
	}
	return result
}

func rawObject(value interface{}) map[string]interface{} {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var decoded map[string]interface{}
	if json.Unmarshal(encoded, &decoded) != nil {
		return nil
	}
	return decoded
}

func cloneAttributes(values map[string]interface{}) map[string]interface{} {
	if values == nil {
		return nil
	}
	result := make(map[string]interface{}, len(values))
	for key, value := range values {
		result[key] = redactValue(key, value)
	}
	return result
}

func redactMap(values map[string]interface{}) map[string]interface{} {
	if values == nil {
		return nil
	}
	result := make(map[string]interface{}, len(values))
	for key, value := range values {
		result[key] = redactValue(key, value)
	}
	return result
}

func redactValue(key string, value interface{}) interface{} {
	if sensitiveKey(key) {
		return "[REDACTED]"
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		return redactMap(typed)
	case []interface{}:
		result := make([]interface{}, len(typed))
		for index, item := range typed {
			result[index] = redactValue("", item)
		}
		return result
	default:
		return value
	}
}

func sensitiveKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return false
	}
	normalized := strings.NewReplacer("_", "", "-", "", ".", "").Replace(key)
	for _, marker := range []string{"password", "passphrase", "authorization", "credential", "apikey", "appkey", "privatekey", "clientsecret", "refreshtoken", "accesstoken"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return key == "token" || strings.HasSuffix(key, "_token") || strings.HasSuffix(key, "_secret")
}

func stringAttribute(values map[string]interface{}, key string) string {
	value, ok := values[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return text
}

func pageLimit(allPages bool, maxPages int) int {
	if !allPages {
		return 1
	}
	if maxPages <= 0 {
		return defaultMaxPages
	}
	if maxPages > maximumMaxPages {
		return maximumMaxPages
	}
	return maxPages
}

func isPOSTCompatibilityError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	for _, marker := range []string{"404", "405", "415", "501", "not implemented", "method not allowed"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func ParseFacetSpecs(raw []string) ([]FacetSpec, error) {
	var result []FacetSpec
	for _, group := range raw {
		for _, part := range strings.Split(group, ",") {
			value := strings.TrimSpace(part)
			if value == "" {
				continue
			}
			facet := FacetSpec{Facet: value}
			if index := strings.LastIndex(value, ":"); index > 0 && index < len(value)-1 {
				limit, err := strconv.ParseInt(value[index+1:], 10, 64)
				if err != nil {
					return nil, fmt.Errorf("facet %q has invalid limit", value)
				}
				facet.Facet = strings.TrimSpace(value[:index])
				facet.Limit = limit
				if facet.Limit == 0 {
					return nil, fmt.Errorf("facet %q limit must be between 1 and 10000", facet.Facet)
				}
			}
			if facet.Facet == "" {
				return nil, fmt.Errorf("facet cannot be empty")
			}
			if facet.Limit < 0 || facet.Limit > 10000 {
				return nil, fmt.Errorf("facet %q limit must be between 1 and 10000", facet.Facet)
			}
			result = append(result, facet)
		}
	}
	return result, nil
}

func ParseComputeSpecs(raw []string) ([]ComputeSpec, error) {
	var result []ComputeSpec
	for _, group := range raw {
		for _, part := range strings.Split(group, ",") {
			value := strings.TrimSpace(part)
			if value == "" {
				continue
			}
			spec := ComputeSpec{Type: "total"}
			if strings.HasPrefix(value, "timeseries(") && strings.HasSuffix(value, ")") {
				spec.Type = "timeseries"
				value = strings.TrimSuffix(strings.TrimPrefix(value, "timeseries("), ")")
			}
			if open := strings.IndexByte(value, '('); open > 0 && strings.HasSuffix(value, ")") {
				spec.Aggregation = strings.TrimSpace(value[:open])
				spec.Metric = strings.TrimSpace(value[open+1 : len(value)-1])
			} else if index := strings.Index(value, ":"); index > 0 {
				spec.Aggregation = strings.TrimSpace(value[:index])
				spec.Metric = strings.TrimSpace(value[index+1:])
			} else {
				spec.Aggregation = value
			}
			if spec.Aggregation == "" {
				return nil, fmt.Errorf("compute aggregation cannot be empty")
			}
			if _, err := datadogV2.NewLogsAggregationFunctionFromValue(spec.Aggregation); err != nil {
				return nil, fmt.Errorf("unsupported compute %q: %w", spec.Aggregation, err)
			}
			if spec.Aggregation == "count" && spec.Metric != "" {
				return nil, fmt.Errorf("count does not accept a metric")
			}
			if spec.Aggregation != "count" && strings.TrimSpace(spec.Metric) == "" {
				return nil, fmt.Errorf("compute %s requires a metric, such as %s(@duration)", spec.Aggregation, spec.Aggregation)
			}
			result = append(result, spec)
		}
	}
	return result, nil
}

func validateAggregateParams(params AggregateParams) error {
	if strings.TrimSpace(params.Query) == "" {
		return fmt.Errorf("query cannot be empty")
	}
	if params.StorageTier != "" {
		if _, err := datadogV2.NewLogsStorageTierFromValue(params.StorageTier); err != nil {
			return fmt.Errorf("invalid storage tier %q", params.StorageTier)
		}
	}
	if params.MaxPages < 0 || params.MaxPages > maxAllowedPages {
		return fmt.Errorf("max pages must be between 0 and %d", maxAllowedPages)
	}
	for _, facet := range params.Facets {
		if strings.TrimSpace(facet.Facet) == "" || facet.Limit < 0 || facet.Limit > 10000 {
			return fmt.Errorf("invalid facet specification")
		}
	}
	for _, compute := range params.Computes {
		if _, err := datadogV2.NewLogsAggregationFunctionFromValue(compute.Aggregation); err != nil {
			return fmt.Errorf("unsupported compute %q", compute.Aggregation)
		}
		if compute.Type != "" && compute.Type != "total" && compute.Type != "timeseries" {
			return fmt.Errorf("unsupported compute type %q", compute.Type)
		}
		if compute.Type == "total" && compute.Interval != "" {
			return fmt.Errorf("total compute does not accept an interval")
		}
		if compute.Aggregation == "count" && compute.Metric != "" {
			return fmt.Errorf("count does not accept a metric")
		}
		if compute.Aggregation != "count" && strings.TrimSpace(compute.Metric) == "" {
			return fmt.Errorf("compute %s requires a metric", compute.Aggregation)
		}
	}
	return nil
}
