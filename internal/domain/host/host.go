package host

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
)

type Service interface {
	List(context.Context, cliruntime.Config, ListParams) (ListResult, error)
	Get(context.Context, cliruntime.Config, string) (Detail, error)
}

type LiveService struct{}

type ListParams struct {
	Filter string
	Count  int64
	Start  int64
}

type Summary struct {
	Name           string     `json:"name"`
	HostName       string     `json:"host_name,omitempty"`
	Aliases        []string   `json:"aliases,omitempty"`
	Apps           []string   `json:"apps,omitempty"`
	Muted          bool       `json:"muted"`
	Up             bool       `json:"up"`
	LastReportedAt *time.Time `json:"last_reported_at,omitempty"`
	Platform       string     `json:"platform,omitempty"`
	AgentVersion   string     `json:"agent_version,omitempty"`
	ID             int64      `json:"id,omitempty"`
}

type Detail struct {
	Summary
	AWSName      string              `json:"aws_name,omitempty"`
	TagsBySource map[string][]string `json:"tags_by_source,omitempty"`
}

type ListResult struct {
	Items         []Summary `json:"items"`
	Count         int       `json:"count"`
	TotalMatching int64     `json:"total_matching,omitempty"`
	TotalReturned int64     `json:"total_returned,omitempty"`
}

func (LiveService) List(ctx context.Context, cfg cliruntime.Config, params ListParams) (ListResult, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return ListResult{}, err
	}
	return listWithClient(client, params)
}

func listWithClient(client *cliruntime.Client, params ListParams) (ListResult, error) {
	if client == nil {
		return ListResult{}, fmt.Errorf("host client must not be nil")
	}
	api := datadogV1.NewHostsApi(client.API)
	opt := datadogV1.NewListHostsOptionalParameters().WithIncludeHostsMetadata(true).WithIncludeMutedHostsData(true)
	if params.Filter != "" {
		opt.WithFilter(params.Filter)
	}
	if params.Count > 0 {
		opt.WithCount(params.Count)
	}
	if params.Start > 0 {
		opt.WithStart(params.Start)
	}
	resp, _, err := api.ListHosts(client.Ctx, *opt)
	if err != nil {
		return ListResult{}, cliruntime.WrapAPIError(err, client.Config)
	}
	items := resp.GetHostList()
	views := make([]Summary, 0, len(items))
	for _, item := range items {
		views = append(views, mapHostSummary(item))
	}
	return ListResult{Items: views, Count: len(views), TotalMatching: resp.GetTotalMatching(), TotalReturned: resp.GetTotalReturned()}, nil
}

func (LiveService) Get(ctx context.Context, cfg cliruntime.Config, name string) (Detail, error) {
	client, err := cliruntime.NewClient(ctx, cfg)
	if err != nil {
		return Detail{}, err
	}
	return getWithClient(client, name)
}

func getWithClient(client *cliruntime.Client, name string) (Detail, error) {
	if client == nil {
		return Detail{}, fmt.Errorf("host client must not be nil")
	}
	api := datadogV1.NewHostsApi(client.API)
	const pageSize int64 = 1000
	const maxPages = 100
	start := int64(0)
	for page := 0; page < maxPages; page++ {
		resp, _, err := api.ListHosts(client.Ctx, *datadogV1.NewListHostsOptionalParameters().WithFilter(name).WithIncludeHostsMetadata(true).WithIncludeMutedHostsData(true).WithCount(pageSize).WithStart(start))
		if err != nil {
			return Detail{}, cliruntime.WrapAPIError(err, client.Config)
		}
		items := resp.GetHostList()
		for _, item := range items {
			if matchesHost(item, name) {
				return mapHostDetail(item), nil
			}
		}
		if len(items) < int(pageSize) {
			break
		}
		start += pageSize
	}
	return Detail{}, fmt.Errorf("host %q not found within bounded inventory search (%d pages)", name, maxPages)
}

func matchesHost(item datadogV1.Host, name string) bool {
	target := strings.ToLower(strings.TrimSpace(name))
	if target == "" {
		return false
	}
	candidates := []string{item.GetName(), item.GetHostName(), item.GetAwsName()}
	for _, candidate := range append(candidates, item.Aliases...) {
		if strings.ToLower(candidate) == target {
			return true
		}
	}
	return false
}

func mapHostSummary(item datadogV1.Host) Summary {
	view := Summary{
		Name:     item.GetName(),
		HostName: item.GetHostName(),
		Aliases:  append([]string{}, item.Aliases...),
		Apps:     append([]string{}, item.Apps...),
		Muted:    item.GetIsMuted(),
		Up:       item.GetUp(),
		ID:       item.GetId(),
	}
	if item.HasLastReportedTime() {
		lastReported := time.Unix(item.GetLastReportedTime(), 0).UTC()
		view.LastReportedAt = &lastReported
	}
	if item.HasMeta() {
		meta := item.GetMeta()
		view.Platform = (&meta).GetPlatform()
		view.AgentVersion = (&meta).GetAgentVersion()
	}
	return view
}

func mapHostDetail(item datadogV1.Host) Detail {
	summary := mapHostSummary(item)
	return Detail{
		Summary:      summary,
		AWSName:      item.GetAwsName(),
		TagsBySource: item.TagsBySource,
	}
}
