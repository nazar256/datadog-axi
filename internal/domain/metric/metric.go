package metric

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
	"github.com/nazar256/datadog-axi/internal/timeutil"
)

type Service interface {
	Query(context.Context, cliruntime.Config, QueryParams) (QueryResult, error)
}

type MetadataService interface {
	Metadata(context.Context, cliruntime.Config, string) (Metadata, error)
}

type SearchService interface {
	Search(context.Context, cliruntime.Config, SearchParams) (SearchResult, error)
}

type ActiveService interface {
	Active(context.Context, cliruntime.Config, ActiveParams) (ActiveResult, error)
}

type LiveService struct{}

type QueryParams struct {
	Query string
	Range timeutil.Range
}

type Series struct {
	Metric      string       `json:"metric,omitempty"`
	Expression  string       `json:"expression,omitempty"`
	Scope       string       `json:"scope,omitempty"`
	Aggregator  string       `json:"aggregator,omitempty"`
	IntervalMS  int64        `json:"interval_ms,omitempty"`
	PointCount  int          `json:"point_count"`
	Start       *time.Time   `json:"start,omitempty"`
	End         *time.Time   `json:"end,omitempty"`
	LastPointTS *time.Time   `json:"last_point_ts,omitempty"`
	LastValue   *float64     `json:"last_value,omitempty"`
	Points      [][]*float64 `json:"points,omitempty"`
}

type QueryResult struct {
	Query  string    `json:"query"`
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
	Status string    `json:"status,omitempty"`
	Series []Series  `json:"series"`
	Count  int       `json:"count"`
}

type Metadata struct {
	Metric         string         `json:"metric"`
	Type           string         `json:"type,omitempty"`
	Unit           string         `json:"unit,omitempty"`
	PerUnit        string         `json:"per_unit,omitempty"`
	Description    string         `json:"description,omitempty"`
	ShortName      string         `json:"short_name,omitempty"`
	Integration    string         `json:"integration,omitempty"`
	StatsdInterval int64          `json:"statsd_interval,omitempty"`
	Raw            map[string]any `json:"raw,omitempty"`
}

type SearchParams struct {
	Query string
	Limit int
}

type SearchResult struct {
	Query     string   `json:"query"`
	Metrics   []string `json:"metrics"`
	Count     int      `json:"count"`
	Total     int      `json:"total"`
	Limit     int      `json:"limit,omitempty"`
	Truncated bool     `json:"truncated,omitempty"`
}

type ActiveParams struct {
	From      time.Time
	Host      string
	TagFilter string
	Limit     int
}

type ActiveResult struct {
	From      time.Time `json:"from"`
	Host      string    `json:"host,omitempty"`
	TagFilter string    `json:"tag_filter,omitempty"`
	Metrics   []string  `json:"metrics"`
	Count     int       `json:"count"`
	Total     int       `json:"total"`
	Limit     int       `json:"limit,omitempty"`
	Truncated bool      `json:"truncated,omitempty"`
}

func (LiveService) Query(ctx context.Context, cfg cliruntime.Config, params QueryParams) (QueryResult, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return QueryResult{}, err
	}
	return queryWithClient(client, params)
}

func queryWithClient(client *cliruntime.Client, params QueryParams) (QueryResult, error) {
	if client == nil {
		return QueryResult{}, fmt.Errorf("metric client must not be nil")
	}
	api := datadogV1.NewMetricsApi(client.API)
	resp, _, err := api.QueryMetrics(client.Ctx, params.Range.From.Unix(), params.Range.To.Unix(), params.Query)
	if err != nil {
		return QueryResult{}, cliruntime.WrapAPIError(err, client.Config)
	}
	if status := strings.TrimSpace(resp.GetStatus()); status != "" && !strings.EqualFold(status, "ok") {
		detail := strings.TrimSpace(resp.GetError())
		if detail == "" {
			detail = strings.TrimSpace(resp.GetMessage())
		}
		if detail == "" {
			detail = "status " + status
		}
		return QueryResult{}, fmt.Errorf("metric query failed: %s", cliruntime.SanitizeError(detail, client.Config))
	}
	series := make([]Series, 0, len(resp.GetSeries()))
	for _, item := range resp.GetSeries() {
		series = append(series, mapSeries(item))
	}
	return QueryResult{
		Query:  params.Query,
		From:   params.Range.From,
		To:     params.Range.To,
		Status: resp.GetStatus(),
		Series: series,
		Count:  len(series),
	}, nil
}

func (LiveService) Metadata(ctx context.Context, cfg cliruntime.Config, name string) (Metadata, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return Metadata{}, err
	}
	return metadataWithClient(client, name)
}

func (LiveService) Search(ctx context.Context, cfg cliruntime.Config, params SearchParams) (SearchResult, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return SearchResult{}, err
	}
	return searchWithClient(client, params)
}

func searchWithClient(client *cliruntime.Client, params SearchParams) (SearchResult, error) {
	if client == nil {
		return SearchResult{}, fmt.Errorf("metric client must not be nil")
	}
	resp, _, err := datadogV1.NewMetricsApi(client.API).ListMetrics(client.Ctx, params.Query)
	if err != nil {
		return SearchResult{}, cliruntime.WrapAPIError(err, client.Config)
	}
	results := resp.GetResults()
	metrics := append([]string(nil), results.GetMetrics()...)
	total := len(metrics)
	truncated := params.Limit > 0 && len(metrics) > params.Limit
	if truncated {
		metrics = metrics[:params.Limit]
	}
	return SearchResult{Query: params.Query, Metrics: metrics, Count: len(metrics), Total: total, Limit: params.Limit, Truncated: truncated}, nil
}

func (LiveService) Active(ctx context.Context, cfg cliruntime.Config, params ActiveParams) (ActiveResult, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return ActiveResult{}, err
	}
	return activeWithClient(client, params)
}

func activeWithClient(client *cliruntime.Client, params ActiveParams) (ActiveResult, error) {
	if client == nil {
		return ActiveResult{}, fmt.Errorf("metric client must not be nil")
	}
	optional := datadogV1.NewListActiveMetricsOptionalParameters()
	if params.Host != "" {
		optional.WithHost(params.Host)
	}
	if params.TagFilter != "" {
		optional.WithTagFilter(params.TagFilter)
	}
	resp, _, err := datadogV1.NewMetricsApi(client.API).ListActiveMetrics(client.Ctx, params.From.Unix(), *optional)
	if err != nil {
		return ActiveResult{}, cliruntime.WrapAPIError(err, client.Config)
	}
	metrics := append([]string(nil), resp.GetMetrics()...)
	total := len(metrics)
	truncated := params.Limit > 0 && len(metrics) > params.Limit
	if truncated {
		metrics = metrics[:params.Limit]
	}
	return ActiveResult{From: params.From, Host: params.Host, TagFilter: params.TagFilter, Metrics: metrics, Count: len(metrics), Total: total, Limit: params.Limit, Truncated: truncated}, nil
}

func metadataWithClient(client *cliruntime.Client, name string) (Metadata, error) {
	if client == nil {
		return Metadata{}, fmt.Errorf("metric client must not be nil")
	}
	item, _, err := datadogV1.NewMetricsApi(client.API).GetMetricMetadata(client.Ctx, name)
	if err != nil {
		return Metadata{}, cliruntime.WrapAPIError(err, client.Config)
	}
	return Metadata{Metric: name, Type: item.GetType(), Unit: item.GetUnit(), PerUnit: item.GetPerUnit(), Description: item.GetDescription(), ShortName: item.GetShortName(), Integration: item.GetIntegration(), StatsdInterval: item.GetStatsdInterval(), Raw: cloneMap(item.AdditionalProperties)}, nil
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

func mapSeries(item datadogV1.MetricsQueryMetadata) Series {
	view := Series{
		Metric:     item.GetMetric(),
		Expression: item.GetExpression(),
		Scope:      item.GetScope(),
		Aggregator: item.GetAggr(),
		IntervalMS: item.GetInterval(),
		PointCount: countPoints(item.Pointlist),
		Points:     item.Pointlist,
	}
	if item.HasStart() {
		start := time.UnixMilli(item.GetStart()).UTC()
		view.Start = &start
	}
	if item.HasEnd() {
		end := time.UnixMilli(item.GetEnd()).UTC()
		view.End = &end
	}
	if ts, value, ok := lastPoint(item.Pointlist); ok {
		view.LastPointTS = &ts
		view.LastValue = &value
	}
	return view
}

func countPoints(points [][]*float64) int {
	count := 0
	for _, point := range points {
		if len(point) >= 2 && point[1] != nil && !math.IsNaN(*point[1]) {
			count++
		}
	}
	return count
}

func lastPoint(points [][]*float64) (time.Time, float64, bool) {
	for i := len(points) - 1; i >= 0; i-- {
		point := points[i]
		if len(point) < 2 || point[0] == nil || point[1] == nil || math.IsNaN(*point[1]) {
			continue
		}
		return time.UnixMilli(int64(*point[0])).UTC(), *point[1], true
	}
	return time.Time{}, 0, false
}
