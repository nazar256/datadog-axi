package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nazar256/datadog-axi/internal/domain/audit"
	"github.com/nazar256/datadog-axi/internal/domain/dashboard"
	"github.com/nazar256/datadog-axi/internal/domain/downtime"
	"github.com/nazar256/datadog-axi/internal/domain/event"
	"github.com/nazar256/datadog-axi/internal/domain/host"
	"github.com/nazar256/datadog-axi/internal/domain/logs"
	"github.com/nazar256/datadog-axi/internal/domain/metric"
	"github.com/nazar256/datadog-axi/internal/domain/monitor"
	"github.com/nazar256/datadog-axi/internal/domain/servicecatalog"
	"github.com/nazar256/datadog-axi/internal/domain/slo"
	"github.com/nazar256/datadog-axi/internal/domain/spans"
	"github.com/nazar256/datadog-axi/internal/output"
	"github.com/nazar256/datadog-axi/internal/runtime"
	"github.com/nazar256/datadog-axi/internal/timeutil"
	"github.com/spf13/cobra"
)

type serviceSet struct {
	Monitor   monitor.Service
	Dashboard dashboard.Service
	Host      host.Service
	Metric    metric.Service
	Logs      logs.Service
	SLO       slo.Service
	Event     event.Service
	Downtime  downtime.Service
	Spans     spans.Service
	Audit     audit.Service
	Services  servicecatalog.Service
}

func defaultServices() serviceSet {
	return serviceSet{
		Monitor:   monitor.LiveService{},
		Dashboard: dashboard.LiveService{},
		Host:      host.LiveService{},
		Metric:    metric.LiveService{},
		Logs:      logs.LiveService{},
		SLO:       slo.LiveService{},
		Event:     event.LiveService{},
		Downtime:  downtime.LiveService{},
		Spans:     spans.LiveService{},
		Audit:     audit.LiveService{},
		Services:  servicecatalog.LiveService{},
	}
}

func (o *GlobalOptions) ensureServices() {
	defaults := defaultServices()
	if o.Services.Monitor == nil {
		o.Services.Monitor = defaults.Monitor
	}
	if o.Services.Dashboard == nil {
		o.Services.Dashboard = defaults.Dashboard
	}
	if o.Services.Host == nil {
		o.Services.Host = defaults.Host
	}
	if o.Services.Metric == nil {
		o.Services.Metric = defaults.Metric
	}
	if o.Services.Logs == nil {
		o.Services.Logs = defaults.Logs
	}
	if o.Services.SLO == nil {
		o.Services.SLO = defaults.SLO
	}
	if o.Services.Event == nil {
		o.Services.Event = defaults.Event
	}
	if o.Services.Downtime == nil {
		o.Services.Downtime = defaults.Downtime
	}
	if o.Services.Spans == nil {
		o.Services.Spans = defaults.Spans
	}
	if o.Services.Audit == nil {
		o.Services.Audit = defaults.Audit
	}
	if o.Services.Services == nil {
		o.Services.Services = defaults.Services
	}
}

func resolveLiveConfig(opts *GlobalOptions) (runtime.Config, error) {
	cfg, err := runtime.ResolveConfig(opts.FlagValues)
	if err != nil {
		return runtime.Config{}, err
	}
	if err := cfg.RequireAuth(); err != nil {
		return runtime.Config{}, err
	}
	cfg.Version = opts.BuildInfo.Version
	return cfg, nil
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return formatTime(*value)
}

func formatStringSlice(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return strings.Join(items, ",")
}

func formatBool(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func formatInt64Pointer(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}

func formatFloatPointer(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', -1, 4)
}

func formatInt64Slice(values []int64) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = strconv.FormatInt(value, 10)
	}
	return strings.Join(parts, ",")
}

func formatCount(count int) string {
	return strconv.Itoa(count)
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func stringSliceValue(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, stringValue(item))
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func truncateForTable(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if limit <= 0 || len(runes) <= limit {
		return value
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}

func preview(opts *GlobalOptions, value string, limit int) string {
	if opts.Full {
		return value
	}
	return truncateForTable(value, limit)
}

func addCoreCommands(root *cobra.Command, opts *GlobalOptions) {
	root.AddCommand(newMonitorCmd(opts))
	root.AddCommand(newDashboardCmd(opts))
	root.AddCommand(newHostCmd(opts))
	root.AddCommand(newMetricCmd(opts))
	root.AddCommand(newLogCmd(opts))
	root.AddCommand(newSLOCmd(opts))
	root.AddCommand(newEventCmd(opts))
	root.AddCommand(newDowntimeCmd(opts))
	root.AddCommand(newSpanCmd(opts), newAuditCmd(opts), newServiceCatalogCmd(opts))
}

func newDowntimeCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "downtime", Short: "Inspect monitor downtimes", GroupID: "core"}
	cmd.AddCommand(newDowntimeListCmd(opts), newDowntimeGetCmd(opts))
	return cmd
}

func newDowntimeListCmd(opts *GlobalOptions) *cobra.Command {
	var params downtime.ListParams
	cmd := &cobra.Command{Use: "list", Short: "List monitor downtimes", Args: cobra.NoArgs, Example: "datadog-axi downtime list --current-only --json", RunE: func(cmd *cobra.Command, args []string) error {
		if params.Limit < 0 || params.Limit > 1000 {
			return usagef("--limit must be between 0 and 1000")
		}
		if params.Offset < 0 {
			return usagef("--offset cannot be negative")
		}
		cfg, err := resolveLiveConfig(opts)
		if err != nil {
			return err
		}
		result, err := opts.Services.Downtime.List(cmd.Context(), cfg, params)
		if err != nil {
			return err
		}
		return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
			rows := make([][]string, 0, len(result.Items))
			for _, item := range result.Items {
				rows = append(rows, []string{item.ID, item.Status, item.Scope, preview(opts, item.Message, 56), formatOptionalTime(item.Modified)})
			}
			return output.Table(w, []string{"ID", "STATUS", "SCOPE", "MESSAGE", "MODIFIED"}, rows)
		})
	}}
	cmd.Flags().BoolVar(&params.CurrentOnly, "current-only", false, "Return only active downtimes")
	cmd.Flags().StringVar(&params.Include, "include", "", "Include related resources (for example monitors)")
	cmd.Flags().Int64Var(&params.Offset, "offset", 0, "Pagination offset")
	cmd.Flags().Int64Var(&params.Limit, "limit", 100, "Maximum downtimes to return")
	return cmd
}

func newEventCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "event", Short: "Inspect Datadog events", GroupID: "core"}
	cmd.AddCommand(newEventListCmd(opts), newEventGetCmd(opts))
	return cmd
}

func newEventListCmd(opts *GlobalOptions) *cobra.Command {
	var params event.ListParams
	var last, from, to string
	cmd := &cobra.Command{Use: "list", Short: "List events", Args: cobra.NoArgs, Example: "datadog-axi event list --last 1h --sources deploy --json", RunE: func(cmd *cobra.Command, args []string) error {
		if params.Limit < 0 || params.Limit > 1000 {
			return usagef("--limit must be between 0 and 1000")
		}
		if params.Page < 0 {
			return usagef("--page cannot be negative")
		}
		if params.Page > 0 && params.Limit > 0 && params.Limit < 1000 {
			return usagef("--page uses Datadog's fixed 1000-event pages; use --limit 1000 or omit --page")
		}
		rangeValue, err := timeutil.ParseRangeWithDefault(last, from, to, time.Hour, time.Now)
		if err != nil {
			return usageError(err)
		}
		params.Range = rangeValue
		cfg, err := resolveLiveConfig(opts)
		if err != nil {
			return err
		}
		result, err := opts.Services.Event.List(cmd.Context(), cfg, params)
		if err != nil {
			return err
		}
		return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
			rows := make([][]string, 0, len(result.Items))
			for _, item := range result.Items {
				rows = append(rows, []string{strconv.FormatInt(item.ID, 10), formatOptionalTime(item.Timestamp), preview(opts, item.Source, 18), preview(opts, item.Title, 36), preview(opts, item.Text, 64)})
			}
			return output.Table(w, []string{"ID", "TIMESTAMP", "SOURCE", "TITLE", "TEXT"}, rows)
		})
	}}
	cmd.Flags().StringVar(&last, "last", "", "Relative lookback duration, such as 1h")
	cmd.Flags().StringVar(&from, "from", "", "Range start in RFC3339")
	cmd.Flags().StringVar(&to, "to", "", "Range end in RFC3339 or 'now'")
	cmd.Flags().StringVar(&params.Sources, "sources", "", "Filter by event sources")
	cmd.Flags().StringVar(&params.Tags, "tags", "", "Filter by event tags")
	cmd.Flags().Int32Var(&params.Page, "page", 0, "Datadog event page number (each page can contain up to 1000 events)")
	cmd.Flags().IntVar(&params.Limit, "limit", 100, "Maximum events to return")
	cmd.Flags().BoolVar(&params.Unaggregated, "unaggregated", false, "Include unaggregated events")
	cmd.Flags().BoolVar(&params.ExcludeAggregate, "exclude-aggregate", false, "Exclude aggregate events")
	return cmd
}

func newSLOCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "slo", Short: "Inspect Datadog service level objectives", GroupID: "core"}
	cmd.AddCommand(newSLOListCmd(opts), newSLOSearchCmd(opts), newSLOGetCmd(opts))
	return cmd
}

func newSLOGetCmd(opts *GlobalOptions) *cobra.Command {
	var last, from, to string
	cmd := &cobra.Command{Use: "get <slo-id>", Short: "Get SLO details and linked monitors", Args: cobra.ExactArgs(1), Example: "datadog-axi slo get slo-123 --last 7d --json", RunE: func(cmd *cobra.Command, args []string) error {
		requested := strings.TrimSpace(last) != "" || strings.TrimSpace(from) != "" || strings.TrimSpace(to) != ""
		var historyRange timeutil.Range
		if requested {
			rangeValue, rangeErr := parseSLOHistoryRange(last, from, to)
			if rangeErr != nil {
				return usageError(rangeErr)
			}
			historyRange = rangeValue
		}
		cfg, err := resolveLiveConfig(opts)
		if err != nil {
			return err
		}
		var item slo.Detail
		if requested {
			getter, ok := opts.Services.SLO.(slo.HistoryGetter)
			if !ok {
				return usagef("SLO history is not supported by the configured service")
			}
			item, err = getter.GetWithHistory(cmd.Context(), cfg, args[0], slo.HistoryParams{Requested: true, From: historyRange.From, To: historyRange.To})
		} else {
			item, err = opts.Services.SLO.Get(cmd.Context(), cfg, args[0])
		}
		if err != nil {
			return err
		}
		return writeOutput(cmd, opts, cfg, item, func(w io.Writer) error {
			rows := [][2]string{{"ID", item.ID}, {"Name", item.Name}, {"Type", item.Type}, {"Timeframe", item.Timeframe}, {"Target", formatFloatPointer(item.TargetThreshold)}, {"Warning", formatFloatPointer(item.WarningThreshold)}, {"State", item.State}, {"Status Availability", item.StatusAvailability}, {"SLI", formatFloatPointer(item.SLI)}, {"Error Budget", formatFloatPointer(item.ErrorBudgetRemaining)}, {"Monitor IDs", formatInt64Slice(item.MonitorIDs)}, {"Description", item.Description}}
			if len(item.CalculationErrors) > 0 {
				rows = append(rows, [2]string{"Calculation Errors", strings.Join(item.CalculationErrors, "; ")})
			}
			return output.KeyValue(w, rows)
		})
	}}
	cmd.Flags().StringVar(&last, "last", "", "Include bounded SLO history for a relative lookback, such as 7d")
	cmd.Flags().StringVar(&from, "from", "", "Include SLO history starting at this RFC3339 timestamp")
	cmd.Flags().StringVar(&to, "to", "", "Include SLO history ending at this RFC3339 timestamp or 'now'")
	return cmd
}

func parseSLOHistoryRange(last, from, to string) (timeutil.Range, error) {
	value := strings.TrimSpace(last)
	if strings.HasSuffix(strings.ToLower(value), "d") {
		number, err := strconv.ParseFloat(strings.TrimSpace(value[:len(value)-1]), 64)
		if err == nil {
			last = strconv.FormatFloat(number*24, 'f', -1, 64) + "h"
		}
	}
	return timeutil.ParseRange(last, from, to, time.Now)
}

func newSLOSearchCmd(opts *GlobalOptions) *cobra.Command {
	var params slo.SearchParams
	cmd := &cobra.Command{
		Use:     "search",
		Short:   "Search SLOs with status and error-budget context",
		Args:    cobra.NoArgs,
		Example: "datadog-axi slo search --query checkout --limit 20\n  datadog-axi slo search --page 2 --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if params.PageSize < 0 || params.PageSize > 1000 {
				return usagef("--limit must be between 0 and 1000")
			}
			if params.PageNumber < 0 {
				return usagef("--page cannot be negative")
			}
			searcher, ok := opts.Services.SLO.(slo.Searcher)
			if !ok {
				return usagef("status-aware SLO search is not supported by the configured service")
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			result, err := searcher.Search(cmd.Context(), cfg, params)
			if err != nil {
				return err
			}
			return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
				rows := make([][]string, 0, len(result.Items))
				for _, item := range result.Items {
					rows = append(rows, []string{item.ID, preview(opts, item.Name, 32), item.Type, firstNonEmpty(item.State, item.StatusAvailability), formatFloatPointer(item.SLI), formatFloatPointer(item.ErrorBudgetRemaining), item.Timeframe, formatInt64Slice(item.MonitorIDs)})
				}
				return output.Table(w, []string{"ID", "NAME", "TYPE", "STATE", "SLI", "ERROR BUDGET", "TIMEFRAME", "MONITOR IDS"}, rows)
			})
		},
	}
	cmd.Flags().StringVar(&params.Query, "query", "", "Search SLO names and descriptions")
	cmd.Flags().Int64Var(&params.PageSize, "limit", 0, "Maximum SLOs to return (maximum 1000)")
	cmd.Flags().Int64Var(&params.PageNumber, "page", 0, "Datadog SLO search page number")
	return cmd
}

func newSLOListCmd(opts *GlobalOptions) *cobra.Command {
	var params slo.ListParams
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List service level objectives",
		Args:    cobra.NoArgs,
		Example: "datadog-axi slo list\n  datadog-axi slo list --query checkout --limit 20\n  datadog-axi slo list --tags-query env:prod --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if params.Limit < 0 {
				return usagef("--limit cannot be negative")
			}
			if params.Offset < 0 {
				return usagef("--offset cannot be negative")
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			result, err := opts.Services.SLO.List(cmd.Context(), cfg, params)
			if err != nil {
				return err
			}
			return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
				rows := make([][]string, 0, len(result.Items))
				for _, item := range result.Items {
					target := ""
					if item.TargetThreshold != nil {
						target = strconv.FormatFloat(*item.TargetThreshold, 'f', -1, 4)
					}
					rows = append(rows, []string{item.ID, preview(opts, item.Name, 32), item.Type, item.Timeframe, target, formatOptionalTime(item.ModifiedAt)})
				}
				if err := output.Table(w, []string{"ID", "NAME", "TYPE", "TIMEFRAME", "TARGET", "MODIFIED"}, rows); err != nil {
					return err
				}
				if len(result.Errors) > 0 {
					_, err := fmt.Fprintf(w, "\nAPI errors: %s\n", strings.Join(result.Errors, "; "))
					return err
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&params.IDs, "ids", "", "Comma-separated SLO IDs")
	cmd.Flags().StringVar(&params.Query, "query", "", "Search SLO names and descriptions")
	cmd.Flags().StringVar(&params.TagsQuery, "tags-query", "", "Filter by SLO tags")
	cmd.Flags().StringVar(&params.MetricsQuery, "metrics-query", "", "Filter by metric query")
	cmd.Flags().Int64Var(&params.Limit, "limit", 0, "Maximum SLOs to return")
	cmd.Flags().Int64Var(&params.Offset, "offset", 0, "Pagination offset")
	return cmd
}

func newMonitorCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "monitor", Short: "Inspect Datadog monitors", GroupID: "core"}
	cmd.AddCommand(newMonitorListCmd(opts), newMonitorSearchCmd(opts), newMonitorGetCmd(opts), newMonitorExportCmd(opts))
	addLocalSpecCommands(cmd, "monitor", opts)
	return cmd
}

func newMonitorExportCmd(opts *GlobalOptions) *cobra.Command {
	var file string
	cmd := &cobra.Command{Use: "export <monitor-id>", Short: "Export the full monitor JSON for review", Args: cobra.ExactArgs(1), Example: "datadog-axi monitor export 123456 --file .tmp/monitor.json", RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return usagef("invalid monitor id %q", args[0])
		}
		if strings.TrimSpace(file) == "" {
			return usagef("--file is required")
		}
		cfg, err := resolveLiveConfig(opts)
		if err != nil {
			return err
		}
		exporter, ok := opts.Services.Monitor.(monitor.Exporter)
		if !ok {
			return fmt.Errorf("monitor export is unavailable for the configured service")
		}
		value, err := exporter.Export(cmd.Context(), cfg, id)
		if err != nil {
			return err
		}
		return writeExportFile(file, value)
	}}
	cmd.Flags().StringVar(&file, "file", "", "Output JSON file")
	return cmd
}

func newMonitorListCmd(opts *GlobalOptions) *cobra.Command {
	var params monitor.ListParams
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List monitors",
		Args:    cobra.NoArgs,
		Long:    "List Datadog monitors with optional name and tag filters. API pagination can be controlled with --offset and --limit.",
		Example: "datadog-axi monitor list\n  datadog-axi monitor list --name api --limit 20\n  datadog-axi monitor list --tags env:prod --offset 20 --limit 20 --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if params.Limit < 0 {
				return usagef("--limit cannot be negative")
			}
			if params.Offset < 0 {
				return usagef("--offset cannot be negative")
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			result, err := opts.Services.Monitor.List(cmd.Context(), cfg, params)
			if err != nil {
				return err
			}
			return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
				rows := make([][]string, 0, len(result.Items))
				for _, item := range result.Items {
					rows = append(rows, []string{strconv.FormatInt(item.ID, 10), preview(opts, item.Name, 32), item.State, item.Type, preview(opts, item.Query, 48)})
				}
				return output.Table(w, []string{"ID", "NAME", "STATE", "TYPE", "QUERY"}, rows)
			})
		},
	}
	cmd.Flags().StringVar(&params.Name, "name", "", "Filter by monitor name")
	cmd.Flags().StringVar(&params.Tags, "tags", "", "Filter by scope tags")
	cmd.Flags().StringVar(&params.MonitorTags, "monitor-tags", "", "Filter by monitor tags")
	cmd.Flags().Int64Var(&params.Offset, "offset", 0, "Datadog monitor offset (id_offset)")
	cmd.Flags().Int32Var(&params.Limit, "limit", 0, "Return at most N monitors")
	return cmd
}

func newMonitorSearchCmd(opts *GlobalOptions) *cobra.Command {
	var params monitor.SearchParams
	cmd := &cobra.Command{
		Use:     "search",
		Short:   "Search monitors with Datadog's monitor-search endpoint",
		Long:    "Search monitors with query, page, per-page, and sort controls. The structured response identifies the exact search endpoint and preserves the API's result context.",
		Args:    cobra.NoArgs,
		Example: "datadog-axi monitor search --query 'type:query_alert status:Alert' --per-page 20 --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if params.Page < 0 || params.PerPage < 0 {
				return usagef("--page and --per-page cannot be negative")
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			searcher, ok := opts.Services.Monitor.(monitor.Searcher)
			if !ok {
				return fmt.Errorf("monitor search is unavailable for the configured service")
			}
			result, err := searcher.Search(cmd.Context(), cfg, params)
			if err != nil {
				return err
			}
			renderResult := result
			if !opts.Full {
				renderResult.Raw = nil
				renderResult.Items = append([]monitor.SearchItem(nil), result.Items...)
				for i := range renderResult.Items {
					renderResult.Items[i].Raw = nil
				}
			}
			return writeOutput(cmd, opts, cfg, renderResult, func(w io.Writer) error {
				rows := make([][]string, 0, len(renderResult.Items))
				for _, item := range renderResult.Items {
					rows = append(rows, []string{strconv.FormatInt(item.ID, 10), preview(opts, item.Name, 32), item.Status, item.Type, preview(opts, item.Query, 48), preview(opts, item.Classification, 24)})
				}
				if err := output.Table(w, []string{"ID", "NAME", "STATUS", "TYPE", "QUERY", "CLASSIFICATION"}, rows); err != nil {
					return err
				}
				_, err := fmt.Fprintf(w, "Endpoint: %s\n", renderResult.Endpoint)
				return err
			})
		},
	}
	cmd.Flags().StringVar(&params.Query, "query", "", "Datadog monitor search query")
	cmd.Flags().Int64Var(&params.Page, "page", 0, "Search result page")
	cmd.Flags().Int64Var(&params.PerPage, "per-page", 0, "Results per page")
	cmd.Flags().StringVar(&params.Sort, "sort", "", "Search sort expression")
	return cmd
}

func newMonitorGetCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <monitor-id>",
		Short:   "Get monitor details",
		Args:    cobra.ExactArgs(1),
		Example: "datadog-axi monitor get 123456\n  datadog-axi monitor get 123456 --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return usagef("invalid monitor id %q", args[0])
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			item, err := opts.Services.Monitor.Get(cmd.Context(), cfg, id)
			item.Raw = nil
			value := item
			if err != nil {
				return err
			}
			return writeOutput(cmd, opts, cfg, value, func(w io.Writer) error {
				return output.KeyValue(w, [][2]string{{"ID", strconv.FormatInt(item.ID, 10)}, {"Name", item.Name}, {"State", item.State}, {"Type", item.Type}, {"Priority", formatInt64Pointer(item.Priority)}, {"Query", item.Query}, {"Tags", formatStringSlice(item.Tags)}, {"Created", formatOptionalTime(item.CreatedAt)}, {"Modified", formatOptionalTime(item.ModifiedAt)}, {"Message", item.Message}})
			})
		},
	}
	return cmd
}

func newDashboardCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "dashboard", Short: "Inspect Datadog dashboards", GroupID: "core"}
	cmd.AddCommand(newDashboardListCmd(opts), newDashboardGetCmd(opts), newDashboardExportCmd(opts))
	addLocalSpecCommands(cmd, "dashboard", opts)
	return cmd
}

func newDashboardExportCmd(opts *GlobalOptions) *cobra.Command {
	var file string
	cmd := &cobra.Command{Use: "export <dashboard-id>", Short: "Export the full dashboard JSON for review", Args: cobra.ExactArgs(1), Example: "datadog-axi dashboard export abc-def-ghi --file .tmp/dashboard.json", RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(file) == "" {
			return usagef("--file is required")
		}
		cfg, err := resolveLiveConfig(opts)
		if err != nil {
			return err
		}
		exporter, ok := opts.Services.Dashboard.(dashboard.Exporter)
		if !ok {
			return fmt.Errorf("dashboard export is unavailable for the configured service")
		}
		value, err := exporter.Export(cmd.Context(), cfg, args[0])
		if err != nil {
			return err
		}
		return writeExportFile(file, value)
	}}
	cmd.Flags().StringVar(&file, "file", "", "Output JSON file")
	return cmd
}

func writeExportFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode export: %w", err)
	}
	data = append(data, '\n')
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to export through symlink: %s", path)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("export destination is not a regular file: %s", path)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect export destination: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".datadog-axi-export-*")
	if err != nil {
		return fmt.Errorf("write export: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("secure export destination: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write export: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync export: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close export: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("publish export: %w", err)
	}
	return nil
}

func newDashboardListCmd(opts *GlobalOptions) *cobra.Command {
	var params dashboard.ListParams
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List dashboards",
		Args:    cobra.NoArgs,
		Example: "datadog-axi dashboard list\n  datadog-axi dashboard list --count 20\n  datadog-axi dashboard list --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if params.Count < 0 {
				return usagef("--count cannot be negative")
			}
			if params.Start < 0 {
				return usagef("--start cannot be negative")
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			result, err := opts.Services.Dashboard.List(cmd.Context(), cfg, params)
			if err != nil {
				return err
			}
			if filter := strings.TrimSpace(params.Filter); filter != "" && result.FilterScope == "" {
				result.Items = filterDashboardPage(result.Items, filter)
				result.Count = len(result.Items)
				result.Filter = filter
				result.FilterScope = "page"
				result.PossiblyIncomplete = true
			}
			return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
				rows := make([][]string, 0, len(result.Items))
				for _, item := range result.Items {
					rows = append(rows, []string{item.ID, preview(opts, item.Title, 36), item.LayoutType, preview(opts, item.Author, 24), formatOptionalTime(item.ModifiedAt)})
				}
				return output.Table(w, []string{"ID", "TITLE", "LAYOUT", "AUTHOR", "MODIFIED"}, rows)
			})
		},
	}
	cmd.Flags().Int64Var(&params.Count, "count", 0, "Maximum dashboards to return")
	cmd.Flags().Int64Var(&params.Start, "start", 0, "Pagination offset")
	cmd.Flags().BoolVar(&params.IncludeShared, "shared", false, "Include shared dashboards")
	cmd.Flags().BoolVar(&params.IncludeDeleted, "deleted", false, "Include deleted dashboards")
	cmd.Flags().StringVar(&params.Filter, "filter", "", "Filter the returned page by dashboard title, id, author, or available tags (page-local)")
	return cmd
}

func filterDashboardPage(items []dashboard.Summary, filter string) []dashboard.Summary {
	needle := strings.ToLower(strings.TrimSpace(filter))
	filtered := make([]dashboard.Summary, 0, len(items))
	for _, item := range items {
		values := append([]string{item.ID, item.Title, item.Author}, item.Tags...)
		matched := false
		for _, value := range values {
			if strings.Contains(strings.ToLower(value), needle) {
				matched = true
				break
			}
		}
		if matched {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func newDashboardGetCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <dashboard-id>",
		Short:   "Get dashboard details",
		Args:    cobra.ExactArgs(1),
		Example: "datadog-axi dashboard get abc-def-ghi\n  datadog-axi dashboard get abc-def-ghi --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			item, err := opts.Services.Dashboard.Get(cmd.Context(), cfg, args[0])
			item.Raw = nil
			value := item
			if err != nil {
				return err
			}
			return writeOutput(cmd, opts, cfg, value, func(w io.Writer) error {
				return output.KeyValue(w, [][2]string{{"ID", item.ID}, {"Title", item.Title}, {"Layout", item.LayoutType}, {"Author", item.Author}, {"URL", item.URL}, {"Created", formatOptionalTime(item.CreatedAt)}, {"Modified", formatOptionalTime(item.ModifiedAt)}, {"Widgets", strconv.Itoa(item.WidgetCount)}, {"Tags", formatStringSlice(item.Tags)}, {"Description", item.Description}})
			})
		},
	}
	return cmd
}

func newHostCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "host", Short: "Inspect Datadog hosts", GroupID: "core"}
	cmd.AddCommand(newHostListCmd(opts), newHostGetCmd(opts))
	return cmd
}

func newHostListCmd(opts *GlobalOptions) *cobra.Command {
	var params host.ListParams
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List hosts",
		Args:    cobra.NoArgs,
		Example: "datadog-axi host list\n  datadog-axi host list --filter web\n  datadog-axi host list --count 50 --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if params.Count < 0 {
				return usagef("--count cannot be negative")
			}
			if params.Start < 0 {
				return usagef("--start cannot be negative")
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			result, err := opts.Services.Host.List(cmd.Context(), cfg, params)
			if err != nil {
				return err
			}
			return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
				rows := make([][]string, 0, len(result.Items))
				for _, item := range result.Items {
					rows = append(rows, []string{preview(opts, item.Name, 28), item.Platform, item.AgentVersion, formatBool(item.Up), formatBool(item.Muted), formatOptionalTime(item.LastReportedAt)})
				}
				return output.Table(w, []string{"NAME", "PLATFORM", "AGENT", "UP", "MUTED", "LAST REPORTED"}, rows)
			})
		},
	}
	cmd.Flags().StringVar(&params.Filter, "filter", "", "Filter by name, alias, or tag")
	cmd.Flags().Int64Var(&params.Count, "count", 0, "Maximum hosts to return")
	cmd.Flags().Int64Var(&params.Start, "start", 0, "Pagination offset")
	return cmd
}

func newHostGetCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <host>",
		Short:   "Get host details",
		Args:    cobra.ExactArgs(1),
		Example: "datadog-axi host get web-01\n  datadog-axi host get web-01 --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			item, err := opts.Services.Host.Get(cmd.Context(), cfg, args[0])
			if err != nil {
				return err
			}
			return writeOutput(cmd, opts, cfg, item, func(w io.Writer) error {
				return output.KeyValue(w, [][2]string{{"Name", item.Name}, {"Host Name", item.HostName}, {"AWS Name", item.AWSName}, {"Aliases", formatStringSlice(item.Aliases)}, {"Apps", formatStringSlice(item.Apps)}, {"Platform", item.Platform}, {"Agent", item.AgentVersion}, {"Up", formatBool(item.Up)}, {"Muted", formatBool(item.Muted)}, {"Last Reported", formatOptionalTime(item.LastReportedAt)}})
			})
		},
	}
	return cmd
}

func newMetricCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "metric", Short: "Query Datadog metrics", GroupID: "core"}
	cmd.AddCommand(newMetricQueryCmd(opts), newMetricMetadataCmd(opts), newMetricSearchCmd(opts), newMetricActiveCmd(opts))
	return cmd
}

func newMetricSearchCmd(opts *GlobalOptions) *cobra.Command {
	var query string
	var limit int
	cmd := &cobra.Command{
		Use:     "search",
		Short:   "Search metric names",
		Long:    "Search metric names reported in Datadog's recent metric index. Use metric metadata after discovery to inspect type, unit, interval, and tags where available.",
		Args:    cobra.NoArgs,
		Example: "datadog-axi metric search --query 'system.cpu' --limit 20 --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(query) == "" {
				return usagef("--query is required")
			}
			if limit < 1 || limit > 1000 {
				return usagef("--limit must be between 1 and 1000")
			}
			service, ok := opts.Services.Metric.(metric.SearchService)
			if !ok {
				return fmt.Errorf("metric search is unavailable for the configured service")
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			result, err := service.Search(cmd.Context(), cfg, metric.SearchParams{Query: query, Limit: limit})
			if err != nil {
				return err
			}
			return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
				rows := make([][]string, 0, len(result.Metrics))
				for _, name := range result.Metrics {
					rows = append(rows, []string{name})
				}
				if err := output.Table(w, []string{"METRIC"}, rows); err != nil {
					return err
				}
				_, err := fmt.Fprintf(w, "\nReturned %d of %d metric name(s) for query %q", result.Count, result.Total, result.Query)
				if result.Truncated {
					_, err = fmt.Fprintf(w, " (truncated at --limit %d)", result.Limit)
				}
				return err
			})
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "Metric-name search text")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum metric names to return")
	return cmd
}

func newMetricActiveCmd(opts *GlobalOptions) *cobra.Command {
	var last, host, tagFilter string
	var limit int
	cmd := &cobra.Command{
		Use:     "active",
		Short:   "List actively reporting metrics",
		Long:    "List metric names reported since a bounded lookback time, optionally filtered by host or tag.",
		Args:    cobra.NoArgs,
		Example: "datadog-axi metric active --last 1h --tag-filter 'env:prod' --limit 50 --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 1 || limit > 1000 {
				return usagef("--limit must be between 1 and 1000")
			}
			rangeValue, err := timeutil.ParseRangeWithDefault(last, "", "", time.Hour, time.Now)
			if err != nil {
				return usageError(err)
			}
			if rangeValue.To.Sub(rangeValue.From) > 30*24*time.Hour {
				return usagef("--last must not exceed 30d for active metric discovery")
			}
			service, ok := opts.Services.Metric.(metric.ActiveService)
			if !ok {
				return fmt.Errorf("active metric listing is unavailable for the configured service")
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			result, err := service.Active(cmd.Context(), cfg, metric.ActiveParams{From: rangeValue.From, Host: host, TagFilter: tagFilter, Limit: limit})
			if err != nil {
				return err
			}
			return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
				rows := make([][]string, 0, len(result.Metrics))
				for _, name := range result.Metrics {
					rows = append(rows, []string{name})
				}
				if err := output.Table(w, []string{"METRIC"}, rows); err != nil {
					return err
				}
				_, err := fmt.Fprintf(w, "\nReturned %d of %d active metric(s) since %s", result.Count, result.Total, formatTime(result.From))
				if result.Truncated {
					_, err = fmt.Fprintf(w, " (truncated at --limit %d)", result.Limit)
				}
				return err
			})
		},
	}
	cmd.Flags().StringVar(&last, "last", "", "Lookback duration, such as 15m or 1h (default 1h)")
	cmd.Flags().StringVar(&host, "host", "", "Filter active metrics by host")
	cmd.Flags().StringVar(&tagFilter, "tag-filter", "", "Filter active metrics by tag")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum metric names to return")
	return cmd
}

func newMetricMetadataCmd(opts *GlobalOptions) *cobra.Command {
	return &cobra.Command{Use: "metadata <metric-name>", Short: "Inspect metric metadata", Args: cobra.ExactArgs(1), Example: "datadog-axi metric metadata system.cpu.user --json", RunE: func(cmd *cobra.Command, args []string) error {
		service, ok := opts.Services.Metric.(metric.MetadataService)
		if !ok {
			return fmt.Errorf("metric metadata is unavailable for the configured service")
		}
		cfg, err := resolveLiveConfig(opts)
		if err != nil {
			return err
		}
		result, err := service.Metadata(cmd.Context(), cfg, args[0])
		if err != nil {
			return err
		}
		return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
			return output.KeyValue(w, [][2]string{{"Metric", result.Metric}, {"Type", result.Type}, {"Unit", result.Unit}, {"Per Unit", result.PerUnit}, {"Description", result.Description}, {"Short Name", result.ShortName}, {"Integration", result.Integration}, {"StatsD Interval", strconv.FormatInt(result.StatsdInterval, 10)}})
		})
	}}
}

func newMetricQueryCmd(opts *GlobalOptions) *cobra.Command {
	var query string
	var last, from, to string
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query metric timeseries",
		Long: strings.TrimSpace(`Query Datadog metric timeseries for a relative or absolute time window.

Prefer '--json' when an agent or script will parse the result. JSON returns a top-level 'series' array with summary fields and complete point arrays: 'point_count' is the number of non-empty points returned, 'last_point_ts' is the timestamp of the last non-empty point, and 'last_value' is that point's numeric value.`),
		Args:    cobra.NoArgs,
		Example: "datadog-axi metric query --query 'avg:system.load.1{*}' --last 1h\n  datadog-axi metric query --query 'avg:system.cpu.user{env:prod}' --from 2026-03-21T09:00:00Z --to 2026-03-21T10:00:00Z --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(query) == "" {
				return usagef("--query is required")
			}
			rangeValue, err := timeutil.ParseRangeWithDefault(last, from, to, time.Hour, time.Now)
			if err != nil {
				return usageError(err)
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			result, err := opts.Services.Metric.Query(cmd.Context(), cfg, metric.QueryParams{Query: query, Range: rangeValue})
			if err != nil {
				return err
			}
			renderResult := result
			if cfg.Output != output.JSON && !opts.Full {
				renderResult.Series = append([]metric.Series(nil), result.Series...)
				for i := range renderResult.Series {
					renderResult.Series[i].Points = nil
				}
			}
			return writeOutput(cmd, opts, cfg, renderResult, func(w io.Writer) error {
				rows := make([][]string, 0, len(renderResult.Series))
				for _, item := range renderResult.Series {
					lastValue := ""
					if item.LastValue != nil {
						lastValue = strconv.FormatFloat(*item.LastValue, 'f', -1, 64)
					}
					rows = append(rows, []string{preview(opts, firstNonEmpty(item.Metric, item.Expression), 36), preview(opts, item.Scope, 24), item.Aggregator, formatCount(item.PointCount), lastValue, formatOptionalTime(item.LastPointTS)})
				}
				if err := output.Table(w, []string{"SERIES", "SCOPE", "AGGR", "POINTS", "LAST", "LAST POINT"}, rows); err != nil {
					return err
				}
				_, err := fmt.Fprintf(w, "\nReturned %s series for range %s to %s\n", formatCount(renderResult.Count), formatTime(renderResult.From), formatTime(renderResult.To))
				return err
			})
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "Datadog metric query, for example 'avg:system.cpu.user{env:prod}'")
	cmd.Flags().StringVar(&last, "last", "", "Relative lookback duration, such as 15m or 1h (default 1h)")
	cmd.Flags().StringVar(&from, "from", "", "Range start in RFC3339")
	cmd.Flags().StringVar(&to, "to", "", "Range end in RFC3339 or 'now'")
	return cmd
}

func newLogCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "log", Short: "Search Datadog logs", GroupID: "core"}
	cmd.AddCommand(newLogSearchCmd(opts), newLogAggregateCmd(opts))
	return cmd
}

func newLogSearchCmd(opts *GlobalOptions) *cobra.Command {
	var query string
	var last, from, to string
	var limit int32
	var indexes []string
	var cursor string
	var sort string
	var storageTier string
	var allPages bool
	var maxPages int
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search logs",
		Long: strings.TrimSpace(`Search Datadog logs with a Datadog log query and a narrow time window.

Start with a focused query such as 'service:web status:error' or 'env:prod @http.status_code:[500 TO 599]'. Use '--last' for quick exploration, add '--index' to target specific log indexes, and prefer '--json' when an agent or script needs stable fields like 'items', 'count', and per-entry 'message', 'service', 'status', 'host', and 'timestamp'.`),
		Args:    cobra.NoArgs,
		Example: "datadog-axi log search --query 'service:web status:error' --last 15m\n  datadog-axi log search --query 'env:prod @http.status_code:[500 TO 599]' --last 30m --index main --limit 20 --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(query) == "" {
				return usagef("--query is required")
			}
			rangeValue, err := timeutil.ParseRangeWithDefault(last, from, to, 15*time.Minute, time.Now)
			if err != nil {
				return usageError(err)
			}
			if sort != "asc" && sort != "desc" {
				return usagef("--sort must be 'asc' or 'desc'")
			}
			if limit < 0 || limit > 1000 {
				return usagef("--limit must be between 0 and 1000")
			}
			if strings.ContainsAny(cursor, "\r\n") {
				return usagef("--cursor contains invalid control characters")
			}
			if maxPages < 0 || maxPages > 100 {
				return usagef("--max-pages must be between 0 and 100")
			}
			if maxPages > 0 && !allPages {
				return usagef("--max-pages requires --all")
			}
			if storageTier != "" {
				if storageTier != "indexes" && storageTier != "online-archives" && storageTier != "flex" {
					return usagef("--storage-tier must be indexes, online-archives, or flex")
				}
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			result, err := opts.Services.Logs.Search(cmd.Context(), cfg, logs.SearchParams{Query: query, Range: rangeValue, Limit: limit, Indexes: indexes, StorageTier: storageTier, SortAsc: sort == "asc", Cursor: cursor, AllPages: allPages, MaxPages: maxPages})
			if err != nil {
				return err
			}
			return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
				rows := make([][]string, 0, len(result.Items))
				for _, item := range result.Items {
					rows = append(rows, []string{formatOptionalTime(item.Timestamp), preview(opts, item.Service, 18), preview(opts, item.Status, 10), preview(opts, item.Host, 18), preview(opts, item.Message, 72)})
				}
				if err := output.Table(w, []string{"TIMESTAMP", "SERVICE", "STATUS", "HOST", "MESSAGE"}, rows); err != nil {
					return err
				}
				if result.NextCursor != "" {
					if _, err := fmt.Fprintf(w, "\nNext cursor: %s\n", result.NextCursor); err != nil {
						return err
					}
				}
				if result.Truncated {
					if _, err := fmt.Fprintf(w, "\nResult truncated after %d page(s); rerun with --all --max-pages %d to widen the bounded search.\n", result.PagesFetched, nextPageBound(result.PagesFetched)); err != nil {
						return err
					}
				}
				_, err := fmt.Fprintf(w, "\nReturned %s logs for range %s to %s (%d page(s))\n", formatCount(result.Count), formatTime(result.From), formatTime(result.To), result.PagesFetched)
				return err
			})
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "Datadog log query, for example 'service:web status:error'")
	cmd.Flags().StringVar(&last, "last", "", "Relative lookback duration, such as 15m or 1h")
	cmd.Flags().StringVar(&from, "from", "", "Range start in RFC3339")
	cmd.Flags().StringVar(&to, "to", "", "Range end in RFC3339 or 'now'")
	cmd.Flags().Int32Var(&limit, "limit", 10, "Maximum logs to return")
	cmd.Flags().StringArrayVar(&indexes, "index", nil, "Limit search to specific log indexes")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Opaque pagination cursor")
	cmd.Flags().StringVar(&sort, "sort", "desc", "Sort order: asc or desc")
	cmd.Flags().StringVar(&storageTier, "storage-tier", "", "Storage tier: indexes, online-archives, or flex")
	cmd.Flags().BoolVar(&allPages, "all", false, "Fetch additional pages up to --max-pages (bounded)")
	cmd.Flags().IntVar(&maxPages, "max-pages", 0, "Maximum pages with --all (0 uses a bounded default of 10; maximum 100)")
	return cmd
}

func newLogAggregateCmd(opts *GlobalOptions) *cobra.Command {
	var query string
	var last, from, to string
	var indexes []string
	var facets []string
	var computes []string
	var cursor string
	var storageTier string
	var allPages bool
	var maxPages int
	cmd := &cobra.Command{
		Use:   "aggregate",
		Short: "Aggregate logs into bounded facet buckets",
		Long: strings.TrimSpace(`Aggregate a focused log query into explicit facet buckets and computed metrics.

Start with a narrow query and one facet, for example '--query service:web --facet status --compute count'.
Repeat '--facet' or use comma-separated values to widen the result deliberately. Use '--all' only with a bounded '--max-pages'.`),
		Args:    cobra.NoArgs,
		Example: "datadog-axi log aggregate --query 'service:web' --last 15m --facet status --compute count --json\n  datadog-axi log aggregate --query 'env:prod' --last 1h --facet service,status --compute count --all --max-pages 3",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(query) == "" {
				return usagef("--query is required")
			}
			if maxPages < 0 || maxPages > 100 {
				return usagef("--max-pages must be between 0 and 100")
			}
			if maxPages > 0 && !allPages {
				return usagef("--max-pages requires --all")
			}
			if strings.ContainsAny(cursor, "\r\n") {
				return usagef("--cursor contains invalid control characters")
			}
			if storageTier != "" && storageTier != "indexes" && storageTier != "online-archives" && storageTier != "flex" {
				return usagef("--storage-tier must be indexes, online-archives, or flex")
			}
			rangeValue, err := timeutil.ParseRangeWithDefault(last, from, to, 15*time.Minute, time.Now)
			if err != nil {
				return usageError(err)
			}
			parsedFacets, err := logs.ParseFacetSpecs(facets)
			if err != nil {
				return usageError(err)
			}
			parsedComputes, err := logs.ParseComputeSpecs(computes)
			if err != nil {
				return usageError(err)
			}
			if len(computes) > 0 && len(parsedComputes) == 0 {
				return usagef("--compute requires at least one specification")
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			aggregator, ok := opts.Services.Logs.(logs.AggregateService)
			if !ok {
				return fmt.Errorf("log aggregation is unavailable for the configured service")
			}
			result, err := aggregator.Aggregate(cmd.Context(), cfg, logs.AggregateParams{Query: query, Range: rangeValue, Indexes: indexes, StorageTier: storageTier, Cursor: cursor, Facets: parsedFacets, Computes: parsedComputes, AllPages: allPages, MaxPages: maxPages})
			if err != nil {
				return err
			}
			return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
				rows := make([][]string, 0, len(result.Buckets))
				for _, bucket := range result.Buckets {
					by, _ := json.Marshal(bucket.By)
					computed, _ := json.Marshal(bucket.Computes)
					rows = append(rows, []string{preview(opts, string(by), 64), preview(opts, string(computed), 72)})
				}
				if err := output.Table(w, []string{"BY", "COMPUTES"}, rows); err != nil {
					return err
				}
				if result.NextCursor != "" {
					if _, err := fmt.Fprintf(w, "\nNext cursor: %s\n", result.NextCursor); err != nil {
						return err
					}
				}
				if result.Truncated {
					if _, err := fmt.Fprintf(w, "\nAggregate truncated after %d page(s); rerun with --all --max-pages %d to widen the bounded search.\n", result.PagesFetched, nextPageBound(result.PagesFetched)); err != nil {
						return err
					}
				}
				_, err := fmt.Fprintf(w, "\nReturned %s aggregate bucket(s) for range %s to %s (%d page(s))\n", formatCount(result.Count), formatTime(result.From), formatTime(result.To), result.PagesFetched)
				return err
			})
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "Datadog log query, for example 'service:web status:error'")
	cmd.Flags().StringVar(&last, "last", "", "Relative lookback duration, such as 15m or 1h")
	cmd.Flags().StringVar(&from, "from", "", "Range start in RFC3339")
	cmd.Flags().StringVar(&to, "to", "", "Range end in RFC3339 or 'now'")
	cmd.Flags().StringArrayVar(&indexes, "index", nil, "Limit search to specific log indexes")
	cmd.Flags().StringArrayVar(&facets, "facet", nil, "Facet name(s), repeat or comma-separate; append :limit to bound a facet")
	cmd.Flags().StringArrayVar(&computes, "compute", []string{"count"}, "Compute specification(s), such as count or avg(@duration)")
	cmd.Flags().StringVar(&cursor, "cursor", "", "Opaque pagination cursor")
	cmd.Flags().StringVar(&storageTier, "storage-tier", "", "Storage tier: indexes, online-archives, or flex")
	cmd.Flags().BoolVar(&allPages, "all", false, "Fetch additional pages up to --max-pages (bounded)")
	cmd.Flags().IntVar(&maxPages, "max-pages", 0, "Maximum pages with --all (0 uses a bounded default of 10; maximum 100)")
	return cmd
}

func nextPageBound(pages int) int {
	if pages >= 1 {
		return pages + 1
	}
	return 2
}
