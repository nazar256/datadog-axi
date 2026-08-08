package slo

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
)

type Service interface {
	List(context.Context, cliruntime.Config, ListParams) (ListResult, error)
	Get(context.Context, cliruntime.Config, string) (Detail, error)
}

// Searcher exposes the status-aware SLO search endpoint. It is kept separate
// from Service so existing callers that only need the legacy list/get surface
// do not have to implement the richer search contract.
type Searcher interface {
	Search(context.Context, cliruntime.Config, SearchParams) (SearchResult, error)
}

// HistoryGetter retrieves an SLO and, when requested, one bounded history
// response. History is deliberately optional because it is more expensive
// than the ordinary detail request.
type HistoryGetter interface {
	GetWithHistory(context.Context, cliruntime.Config, string, HistoryParams) (Detail, error)
}

type LiveService struct{}

var _ Service = LiveService{}
var _ Searcher = LiveService{}
var _ HistoryGetter = LiveService{}

type ListParams struct {
	IDs          string
	Query        string
	TagsQuery    string
	MetricsQuery string
	Limit        int64
	Offset       int64
}

type SearchParams struct {
	Query      string
	PageSize   int64
	PageNumber int64
}

type HistoryParams struct {
	Requested bool
	From      time.Time
	To        time.Time
}

const MaxHistoryWindow = 30 * 24 * time.Hour

type Summary struct {
	ID                   string          `json:"id"`
	Name                 string          `json:"name"`
	Type                 string          `json:"type"`
	Timeframe            string          `json:"timeframe,omitempty"`
	TargetThreshold      *float64        `json:"target_threshold,omitempty"`
	Tags                 []string        `json:"tags,omitempty"`
	CreatedAt            *time.Time      `json:"created_at,omitempty"`
	ModifiedAt           *time.Time      `json:"modified_at,omitempty"`
	State                string          `json:"state,omitempty"`
	SLI                  *float64        `json:"sli,omitempty"`
	ErrorBudgetRemaining *float64        `json:"error_budget_remaining,omitempty"`
	RawErrorBudget       any             `json:"raw_error_budget,omitempty"`
	CalculationError     string          `json:"calculation_error,omitempty"`
	CalculationErrors    []string        `json:"calculation_errors,omitempty"`
	IndexedAt            *time.Time      `json:"indexed_at,omitempty"`
	StatusAvailability   string          `json:"status_availability"`
	MonitorIDs           []int64         `json:"monitor_ids,omitempty"`
	Overall              []OverallStatus `json:"overall,omitempty"`
}

type OverallStatus struct {
	Timeframe            string     `json:"timeframe,omitempty"`
	State                string     `json:"state,omitempty"`
	SLI                  *float64   `json:"sli,omitempty"`
	Status               *float64   `json:"status,omitempty"`
	Target               *float64   `json:"target,omitempty"`
	ErrorBudgetRemaining *float64   `json:"error_budget_remaining,omitempty"`
	RawErrorBudget       any        `json:"raw_error_budget,omitempty"`
	CalculationError     string     `json:"calculation_error,omitempty"`
	IndexedAt            *time.Time `json:"indexed_at,omitempty"`
}

type Detail struct {
	Summary
	Description          string         `json:"description,omitempty"`
	MonitorIDs           []int64        `json:"monitor_ids,omitempty"`
	MonitorTags          []string       `json:"monitor_tags,omitempty"`
	Groups               []string       `json:"groups,omitempty"`
	WarningThreshold     *float64       `json:"warning_threshold,omitempty"`
	Thresholds           any            `json:"thresholds,omitempty"`
	ConfiguredAlertIDs   []int64        `json:"configured_alert_ids,omitempty"`
	State                string         `json:"state,omitempty"`
	SLI                  *float64       `json:"sli,omitempty"`
	ErrorBudgetRemaining *float64       `json:"error_budget_remaining,omitempty"`
	RawErrorBudget       any            `json:"raw_error_budget,omitempty"`
	CalculationErrors    []string       `json:"calculation_errors,omitempty"`
	IndexedAt            *time.Time     `json:"indexed_at,omitempty"`
	History              *History       `json:"history,omitempty"`
	Raw                  map[string]any `json:"raw,omitempty"`
}

type History struct {
	From              *time.Time       `json:"from,omitempty"`
	To                *time.Time       `json:"to,omitempty"`
	Type              string           `json:"type,omitempty"`
	Overall           *HistoryOverall  `json:"overall,omitempty"`
	Monitors          []map[string]any `json:"monitors,omitempty"`
	Groups            []map[string]any `json:"groups,omitempty"`
	Series            any              `json:"series,omitempty"`
	Thresholds        any              `json:"thresholds,omitempty"`
	Errors            []string         `json:"errors,omitempty"`
	CalculationErrors []string         `json:"calculation_errors,omitempty"`
	BurnRate          DerivedValue     `json:"burn_rate"`
	Raw               map[string]any   `json:"raw,omitempty"`
}

type HistoryOverall struct {
	SLI                  *float64           `json:"sli,omitempty"`
	ErrorBudgetRemaining map[string]float64 `json:"error_budget_remaining,omitempty"`
	History              [][]float64        `json:"history,omitempty"`
	CalculationErrors    []string           `json:"calculation_errors,omitempty"`
}

type DerivedValue struct {
	Value  *float64 `json:"value,omitempty"`
	Status string   `json:"status"`
	Source string   `json:"source"`
	Reason string   `json:"reason,omitempty"`
}

type BudgetObservation struct {
	At        time.Time
	Remaining float64
}

type ListResult struct {
	Items  []Summary `json:"items"`
	Count  int       `json:"count"`
	Errors []string  `json:"errors,omitempty"`
}

type SearchResult struct {
	Items        []Summary `json:"items"`
	Count        int       `json:"count"`
	PageNumber   *int64    `json:"page_number,omitempty"`
	PageSize     *int64    `json:"page_size,omitempty"`
	Total        *int64    `json:"total,omitempty"`
	NextPage     *int64    `json:"next_page,omitempty"`
	PreviousPage *int64    `json:"previous_page,omitempty"`
	Facets       any       `json:"facets,omitempty"`
	Errors       []string  `json:"errors,omitempty"`
}

func (LiveService) List(ctx context.Context, cfg cliruntime.Config, params ListParams) (ListResult, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return ListResult{}, err
	}
	api := datadogV1.NewServiceLevelObjectivesApi(client.API)
	opt := datadogV1.NewListSLOsOptionalParameters()
	if params.IDs != "" {
		opt.WithIds(params.IDs)
	}
	if params.Query != "" {
		opt.WithQuery(params.Query)
	}
	if params.TagsQuery != "" {
		opt.WithTagsQuery(params.TagsQuery)
	}
	if params.MetricsQuery != "" {
		opt.WithMetricsQuery(params.MetricsQuery)
	}
	if params.Limit > 0 {
		opt.WithLimit(params.Limit)
	}
	if params.Offset > 0 {
		opt.WithOffset(params.Offset)
	}
	resp, _, err := api.ListSLOs(client.Ctx, *opt)
	if err != nil {
		return ListResult{}, cliruntime.WrapAPIError(err, cfg)
	}
	items := resp.GetData()
	views := make([]Summary, 0, len(items))
	for _, item := range items {
		views = append(views, mapSummary(item))
	}
	return ListResult{Items: views, Count: len(views), Errors: append([]string{}, resp.GetErrors()...)}, nil
}

func (LiveService) Search(ctx context.Context, cfg cliruntime.Config, params SearchParams) (SearchResult, error) {
	if params.PageSize < 0 || params.PageSize > 1000 {
		return SearchResult{}, fmt.Errorf("SLO page size must be between 0 and 1000")
	}
	if params.PageNumber < 0 {
		return SearchResult{}, fmt.Errorf("SLO page number cannot be negative")
	}
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return SearchResult{}, err
	}
	api := datadogV1.NewServiceLevelObjectivesApi(client.API)
	opt := datadogV1.NewSearchSLOOptionalParameters().WithIncludeFacets(true)
	if strings.TrimSpace(params.Query) != "" {
		opt.WithQuery(strings.TrimSpace(params.Query))
	}
	if params.PageSize > 0 {
		opt.WithPageSize(params.PageSize)
	}
	if params.PageNumber > 0 {
		opt.WithPageNumber(params.PageNumber)
	}
	resp, _, err := api.SearchSLO(client.Ctx, *opt)
	if err != nil {
		return SearchResult{}, cliruntime.WrapAPIError(err, cfg)
	}
	data := resp.GetData()
	attrs := data.GetAttributes()
	items := make([]Summary, 0, len(attrs.GetSlos()))
	for _, item := range attrs.GetSlos() {
		items = append(items, mapSearchSummary(item))
	}
	result := SearchResult{Items: items, Count: len(items)}
	if resp.HasMeta() {
		meta := resp.GetMeta()
		if page, ok := meta.GetPaginationOk(); ok {
			if value, exists := page.GetNumberOk(); exists {
				result.PageNumber = optionalInt64(value, true)
			}
			if value, exists := page.GetSizeOk(); exists {
				result.PageSize = optionalInt64(value, true)
			}
			if value, exists := page.GetTotalOk(); exists {
				result.Total = optionalInt64(value, true)
			}
			if value, exists := page.GetNextNumberOk(); exists {
				result.NextPage = optionalInt64(value, true)
			}
			if value, exists := page.GetPrevNumberOk(); exists {
				result.PreviousPage = optionalInt64(value, true)
			}
		}
	}
	if attrs.HasFacets() {
		result.Facets = rawValue(attrs.GetFacets())
	}
	return result, nil
}

func (LiveService) Get(ctx context.Context, cfg cliruntime.Config, id string) (Detail, error) {
	return LiveService{}.GetWithHistory(ctx, cfg, id, HistoryParams{})
}

func (LiveService) GetWithHistory(ctx context.Context, cfg cliruntime.Config, id string, params HistoryParams) (Detail, error) {
	if strings.TrimSpace(id) == "" {
		return Detail{}, fmt.Errorf("SLO id must not be empty")
	}
	requested := params.Requested || !params.From.IsZero() || !params.To.IsZero()
	if requested {
		if params.From.IsZero() || params.To.IsZero() || !params.From.Before(params.To) {
			return Detail{}, fmt.Errorf("SLO history range must have a start before its end")
		}
		if params.To.Sub(params.From) > MaxHistoryWindow {
			return Detail{}, fmt.Errorf("SLO history range cannot exceed %s", MaxHistoryWindow)
		}
	}
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return Detail{}, err
	}
	opt := datadogV1.NewGetSLOOptionalParameters().WithWithConfiguredAlertIds(true)
	resp, _, err := datadogV1.NewServiceLevelObjectivesApi(client.API).GetSLO(client.Ctx, id, *opt)
	if err != nil {
		return Detail{}, cliruntime.WrapAPIError(err, cfg)
	}
	data := resp.GetData()
	timeframe := data.GetTimeframe()
	summary := mapSummary(datadogV1.ServiceLevelObjective{Id: data.Id, Name: data.GetName(), Type: data.GetType(), Timeframe: &timeframe, TargetThreshold: data.TargetThreshold, Tags: data.Tags, CreatedAt: data.CreatedAt, ModifiedAt: data.ModifiedAt})
	rawBytes, _ := json.Marshal(data)
	var raw map[string]any
	_ = json.Unmarshal(rawBytes, &raw)
	detail := Detail{Summary: summary, Description: data.GetDescription(), MonitorIDs: append([]int64{}, data.MonitorIds...), MonitorTags: append([]string{}, data.MonitorTags...), Groups: append([]string{}, data.Groups...), WarningThreshold: data.WarningThreshold, Thresholds: data.Thresholds, ConfiguredAlertIDs: append([]int64{}, data.ConfiguredAlertIds...), Raw: raw}
	if requested {
		history, _, historyErr := datadogV1.NewServiceLevelObjectivesApi(client.API).GetSLOHistory(client.Ctx, id, params.From.Unix(), params.To.Unix())
		if historyErr != nil {
			return Detail{}, cliruntime.WrapAPIError(historyErr, cfg)
		}
		detail.History = mapHistory(history, params.From, params.To)
		if detail.History != nil {
			detail.CalculationErrors = appendUnique(detail.CalculationErrors, detail.History.CalculationErrors...)
			if detail.History.Overall != nil && historyOverallHasMeasurement(detail.History.Overall) {
				detail.StatusAvailability = "available"
				detail.ErrorBudgetRemaining = budgetForTimeframe(detail.History.Overall.ErrorBudgetRemaining, detail.Timeframe)
			}
		}
	}
	return detail, nil
}

func historyOverallHasMeasurement(value *HistoryOverall) bool {
	return value != nil && (value.SLI != nil || len(value.ErrorBudgetRemaining) > 0 || len(value.History) > 0)
}

func mapSummary(item datadogV1.ServiceLevelObjective) Summary {
	view := Summary{
		ID:                 item.GetId(),
		Name:               item.GetName(),
		Type:               string(item.GetType()),
		Timeframe:          string(item.GetTimeframe()),
		Tags:               append([]string{}, item.GetTags()...),
		StatusAvailability: "unavailable",
	}
	if item.HasTargetThreshold() {
		threshold := item.GetTargetThreshold()
		view.TargetThreshold = &threshold
	}
	if item.HasCreatedAt() {
		created := time.Unix(item.GetCreatedAt(), 0).UTC()
		view.CreatedAt = &created
	}
	if item.HasModifiedAt() {
		modified := time.Unix(item.GetModifiedAt(), 0).UTC()
		view.ModifiedAt = &modified
	}
	return view
}

func mapSearchSummary(item datadogV1.SearchServiceLevelObjective) Summary {
	data := item.GetData()
	attrs := data.GetAttributes()
	typeValue := string(attrs.GetSloType())
	if typeValue == "" {
		typeValue = data.GetType()
	}
	view := Summary{ID: data.GetId(), Name: attrs.GetName(), Type: typeValue, Tags: append([]string{}, attrs.GetAllTags()...), MonitorIDs: append([]int64{}, attrs.GetMonitorIds()...), StatusAvailability: "unavailable"}
	if attrs.HasCreatedAt() {
		created := time.Unix(attrs.GetCreatedAt(), 0).UTC()
		view.CreatedAt = &created
	}
	if attrs.HasModifiedAt() {
		modified := time.Unix(attrs.GetModifiedAt(), 0).UTC()
		view.ModifiedAt = &modified
	}
	if thresholds := attrs.GetThresholds(); len(thresholds) > 0 {
		threshold := thresholds[0]
		view.Timeframe = string(threshold.GetTimeframe())
		value := threshold.GetTarget()
		view.TargetThreshold = &value
	}
	if attrs.HasStatus() {
		status := attrs.GetStatus()
		view.StatusAvailability = "available"
		applyStatus(&view, status)
	}
	for _, overall := range attrs.GetOverallStatus() {
		mapped := mapOverallStatus(overall)
		view.Overall = append(view.Overall, mapped)
		view.CalculationErrors = appendUnique(view.CalculationErrors, mapped.CalculationError)
	}
	return view
}

func applyStatus(view *Summary, status datadogV1.SLOStatus) {
	if state, ok := status.GetStateOk(); ok {
		view.State = string(*state)
	}
	if value, ok := status.GetSliOk(); ok {
		view.SLI = value
	}
	if value, ok := status.GetErrorBudgetRemainingOk(); ok {
		view.ErrorBudgetRemaining = value
	}
	if status.HasRawErrorBudgetRemaining() {
		view.RawErrorBudget = rawValue(status.GetRawErrorBudgetRemaining())
	}
	if value, ok := status.GetCalculationErrorOk(); ok && value != nil && strings.TrimSpace(*value) != "" {
		view.CalculationError = *value
		view.CalculationErrors = appendUnique(view.CalculationErrors, *value)
	}
	if value, ok := status.GetIndexedAtOk(); ok {
		indexed := time.Unix(*value, 0).UTC()
		view.IndexedAt = &indexed
	}
}

func mapOverallStatus(value datadogV1.SLOOverallStatuses) OverallStatus {
	view := OverallStatus{Timeframe: string(value.GetTimeframe())}
	if state, ok := value.GetStateOk(); ok {
		view.State = string(*state)
	}
	if current, ok := value.GetStatusOk(); ok {
		view.Status = current
		view.SLI = current
	}
	if target, ok := value.GetTargetOk(); ok {
		view.Target = target
	}
	if budget, ok := value.GetErrorBudgetRemainingOk(); ok {
		view.ErrorBudgetRemaining = budget
	}
	if value.HasRawErrorBudgetRemaining() {
		view.RawErrorBudget = rawValue(value.GetRawErrorBudgetRemaining())
	}
	if calculation, ok := value.GetErrorOk(); ok && calculation != nil && strings.TrimSpace(*calculation) != "" {
		view.CalculationError = *calculation
	}
	if indexed, ok := value.GetIndexedAtOk(); ok {
		at := time.Unix(*indexed, 0).UTC()
		view.IndexedAt = &at
	}
	return view
}

func mapHistory(response datadogV1.SLOHistoryResponse, from, to time.Time) *History {
	data := response.GetData()
	raw, _ := rawValue(response).(map[string]any)
	history := &History{From: &from, To: &to, Type: string(data.GetType()), Raw: raw}
	if value, ok := data.GetFromTsOk(); ok {
		at := time.Unix(*value, 0).UTC()
		history.From = &at
	}
	if value, ok := data.GetToTsOk(); ok {
		at := time.Unix(*value, 0).UTC()
		history.To = &at
	}
	if history.Type == "" {
		if rawData, ok := raw["data"].(map[string]any); ok {
			history.Type, _ = rawData["type"].(string)
		}
	}
	history.Errors = historyResponseErrors(response.GetErrors())
	if data.HasOverall() {
		overall := data.GetOverall()
		history.Overall = &HistoryOverall{ErrorBudgetRemaining: cloneFloatMap(overall.GetErrorBudgetRemaining()), History: cloneHistoryObservations(overall.GetHistory()), CalculationErrors: historyResponseTypedErrors(overall.GetErrors())}
		if value, ok := overall.GetSliValueOk(); ok {
			history.Overall.SLI = value
		}
		history.CalculationErrors = appendUnique(history.CalculationErrors, history.Errors...)
	} else {
		history.CalculationErrors = appendUnique(history.CalculationErrors, history.Errors...)
	}
	if rawData, ok := raw["data"].(map[string]any); ok {
		if history.Overall == nil {
			history.Overall = mapRawHistoryOverall(rawData["overall"])
		}
		if overallRaw, ok := rawData["overall"].(map[string]any); ok {
			history.CalculationErrors = appendUnique(history.CalculationErrors, rawCalculationErrors(overallRaw["errors"])...)
		}
		if monitors, ok := rawData["monitors"].([]any); ok {
			history.Monitors = cloneMaps(monitors)
			for _, monitor := range monitors {
				if object, ok := monitor.(map[string]any); ok {
					history.CalculationErrors = appendUnique(history.CalculationErrors, rawCalculationErrors(object["errors"])...)
				}
			}
		}
		if groups, ok := rawData["groups"].([]any); ok {
			history.Groups = cloneMaps(groups)
			for _, group := range groups {
				if object, ok := group.(map[string]any); ok {
					history.CalculationErrors = appendUnique(history.CalculationErrors, rawCalculationErrors(object["errors"])...)
				}
			}
		}
		history.Series = rawData["series"]
		history.Thresholds = rawData["thresholds"]
	}
	history.BurnRate = deriveBurnRate(nil)
	return history
}

func mapRawHistoryOverall(value any) *HistoryOverall {
	fields, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	overall := &HistoryOverall{}
	if value, ok := fields["sli_value"].(float64); ok {
		overall.SLI = &value
	}
	if budgets, ok := fields["error_budget_remaining"].(map[string]any); ok {
		overall.ErrorBudgetRemaining = make(map[string]float64, len(budgets))
		for key, value := range budgets {
			if number, ok := value.(float64); ok {
				overall.ErrorBudgetRemaining[key] = number
			}
		}
	}
	if observations, ok := fields["history"].([]any); ok {
		overall.History = make([][]float64, 0, len(observations))
		for _, observation := range observations {
			values, ok := observation.([]any)
			if !ok {
				continue
			}
			row := make([]float64, 0, len(values))
			for _, value := range values {
				if number, ok := value.(float64); ok {
					row = append(row, number)
				}
			}
			overall.History = append(overall.History, row)
		}
	}
	overall.CalculationErrors = rawCalculationErrors(fields["errors"])
	if overall.SLI == nil && len(overall.ErrorBudgetRemaining) == 0 && len(overall.History) == 0 && len(overall.CalculationErrors) == 0 {
		return nil
	}
	return overall
}

func rawCalculationErrors(value any) []string {
	entries, ok := value.([]any)
	if !ok {
		return nil
	}
	result := []string{}
	for _, entry := range entries {
		fields, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := fields["error_type"].(string)
		message, _ := fields["error_message"].(string)
		if kind == "" {
			result = appendUnique(result, message)
		} else if message == "" {
			result = appendUnique(result, kind)
		} else {
			result = appendUnique(result, kind+": "+message)
		}
	}
	return result
}

func cloneMaps(values []any) []map[string]any {
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if object, ok := value.(map[string]any); ok {
			result = append(result, object)
		}
	}
	return result
}

func deriveBurnRate(observations []BudgetObservation) DerivedValue {
	if len(observations) < 2 {
		return DerivedValue{Status: "unavailable", Source: "derived", Reason: "at least two valid error-budget observations are required"}
	}
	first, last := observations[0], observations[len(observations)-1]
	if first.At.IsZero() || last.At.IsZero() || !first.At.Before(last.At) || math.IsNaN(first.Remaining) || math.IsNaN(last.Remaining) {
		return DerivedValue{Status: "unavailable", Source: "derived", Reason: "known elapsed window and valid observations are required"}
	}
	if first.Remaining <= 0 || last.Remaining < 0 {
		return DerivedValue{Status: "unavailable", Source: "derived", Reason: "error-budget observations must be positive"}
	}
	value := (first.Remaining - last.Remaining) / first.Remaining / last.At.Sub(first.At).Hours()
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return DerivedValue{Status: "unavailable", Source: "derived", Reason: "burn rate could not be calculated"}
	}
	return DerivedValue{Value: &value, Status: "available", Source: "derived"}
}

func rawValue(value any) any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var result any
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil
	}
	return result
}

func optionalInt64(value *int64, ok bool) *int64 {
	if !ok {
		return nil
	}
	copy := *value
	return &copy
}

func budgetForTimeframe(values map[string]float64, timeframe string) *float64 {
	if value, ok := values[timeframe]; ok {
		copy := value
		return &copy
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		copy := values[key]
		return &copy
	}
	return nil
}

func cloneFloatMap(values map[string]float64) map[string]float64 {
	if values == nil {
		return nil
	}
	result := make(map[string]float64, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func cloneHistoryObservations(values [][]float64) [][]float64 {
	if values == nil {
		return nil
	}
	result := make([][]float64, len(values))
	for index, value := range values {
		result[index] = append([]float64{}, value...)
	}
	return result
}

func historyResponseErrors(values []datadogV1.SLOHistoryResponseError) []string {
	result := []string{}
	for _, value := range values {
		if text, ok := value.GetErrorOk(); ok && text != nil && strings.TrimSpace(*text) != "" {
			result = appendUnique(result, *text)
		}
	}
	return result
}

func historyResponseTypedErrors(values []datadogV1.SLOHistoryResponseErrorWithType) []string {
	result := []string{}
	for _, value := range values {
		message := strings.TrimSpace(value.GetErrorMessage())
		kind := strings.TrimSpace(value.GetErrorType())
		if message == "" && kind == "" {
			continue
		}
		if kind == "" {
			result = appendUnique(result, message)
		} else if message == "" {
			result = appendUnique(result, kind)
		} else {
			result = appendUnique(result, kind+": "+message)
		}
	}
	return result
}

func appendUnique(values []string, additions ...string) []string {
	for _, addition := range additions {
		if strings.TrimSpace(addition) == "" {
			continue
		}
		seen := false
		for _, existing := range values {
			if existing == addition {
				seen = true
				break
			}
		}
		if !seen {
			values = append(values, addition)
		}
	}
	return values
}
