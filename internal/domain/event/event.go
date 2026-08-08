package event

import (
	"context"
	"fmt"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
	"github.com/nazar256/datadog-axi/internal/timeutil"
)

type Service interface {
	List(context.Context, cliruntime.Config, ListParams) (ListResult, error)
	Get(context.Context, cliruntime.Config, int64) (Detail, error)
}

type LiveService struct{}

type ListParams struct {
	Range            timeutil.Range
	Sources          string
	Tags             string
	Page             int32
	Limit            int
	Unaggregated     bool
	ExcludeAggregate bool
}

type Summary struct {
	ID        int64      `json:"id"`
	IDString  string     `json:"id_string,omitempty"`
	Timestamp *time.Time `json:"timestamp,omitempty"`
	Title     string     `json:"title,omitempty"`
	Text      string     `json:"text,omitempty"`
	Source    string     `json:"source,omitempty"`
	Priority  string     `json:"priority,omitempty"`
	Tags      []string   `json:"tags,omitempty"`
	URL       string     `json:"url,omitempty"`
}

// Detail is the stable read projection returned by event get. It embeds the
// list projection and retains the additional fields exposed by the V1 event
// endpoint without leaking the generated SDK model to callers.
type Detail struct {
	Summary
	Status     string `json:"status,omitempty"`
	AlertType  string `json:"alert_type,omitempty"`
	DeviceName string `json:"device_name,omitempty"`
	Host       string `json:"host,omitempty"`
	Payload    string `json:"payload,omitempty"`
}

type ListResult struct {
	Items             []Summary `json:"items"`
	Count             int       `json:"count"`
	Page              int32     `json:"page,omitempty"`
	Limit             int       `json:"limit,omitempty"`
	PossiblyTruncated bool      `json:"possibly_truncated,omitempty"`
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
		return ListResult{}, fmt.Errorf("event client must not be nil")
	}
	opt := datadogV1.NewListEventsOptionalParameters()
	if params.Sources != "" {
		opt.WithSources(params.Sources)
	}
	if params.Tags != "" {
		opt.WithTags(params.Tags)
	}
	if params.Page > 0 {
		opt.WithPage(params.Page)
	}
	if params.Unaggregated {
		opt.WithUnaggregated(true)
	}
	if params.ExcludeAggregate {
		opt.WithExcludeAggregate(true)
	}
	resp, _, err := datadogV1.NewEventsApi(client.API).ListEvents(client.Ctx, params.Range.From.Unix(), params.Range.To.Unix(), *opt)
	if err != nil {
		return ListResult{}, cliruntime.WrapAPIError(err, client.Config)
	}
	items := resp.GetEvents()
	possiblyTruncated := params.Limit > 0 && len(items) > params.Limit
	if params.Limit > 0 && len(items) > params.Limit {
		items = items[:params.Limit]
	}
	result := ListResult{Items: make([]Summary, 0, len(items)), Count: len(items), Page: params.Page, Limit: params.Limit, PossiblyTruncated: possiblyTruncated}
	for _, item := range items {
		result.Items = append(result.Items, mapEvent(item))
	}
	return result, nil
}

func (LiveService) Get(ctx context.Context, cfg cliruntime.Config, id int64) (Detail, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return Detail{}, err
	}
	return getWithClient(client, cfg, id)
}

func getWithClient(client *cliruntime.Client, cfg cliruntime.Config, id int64) (Detail, error) {
	if client == nil {
		return Detail{}, fmt.Errorf("event client must not be nil")
	}
	resp, _, err := datadogV1.NewEventsApi(client.API).GetEvent(client.Ctx, id)
	if err != nil {
		return Detail{}, cliruntime.WrapAPIError(err, client.Config)
	}
	return mapEventDetail(resp), nil
}

func mapEvent(item datadogV1.Event) Summary {
	view := Summary{ID: item.GetId(), IDString: item.GetIdStr(), Title: item.GetTitle(), Text: item.GetText(), Source: item.GetSourceTypeName(), Priority: string(item.GetPriority()), Tags: append([]string{}, item.GetTags()...), URL: item.GetUrl()}
	if item.HasDateHappened() {
		timestamp := time.Unix(item.GetDateHappened(), 0).UTC()
		view.Timestamp = &timestamp
	}
	return view
}

func mapEventDetail(resp datadogV1.EventResponse) Detail {
	item := resp.GetEvent()
	return Detail{
		Summary:    mapEvent(item),
		Status:     resp.GetStatus(),
		AlertType:  string(item.GetAlertType()),
		DeviceName: item.GetDeviceName(),
		Host:       item.GetHost(),
		Payload:    item.GetPayload(),
	}
}
