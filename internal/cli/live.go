package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/nazar256/datadog-cli/internal/domain/dashboard"
	"github.com/nazar256/datadog-cli/internal/domain/host"
	"github.com/nazar256/datadog-cli/internal/domain/logs"
	"github.com/nazar256/datadog-cli/internal/domain/metric"
	"github.com/nazar256/datadog-cli/internal/domain/monitor"
	"github.com/nazar256/datadog-cli/internal/output"
	"github.com/nazar256/datadog-cli/internal/runtime"
	"github.com/nazar256/datadog-cli/internal/timeutil"
	"github.com/spf13/cobra"
)

type serviceSet struct {
	Monitor   monitor.Service
	Dashboard dashboard.Service
	Host      host.Service
	Metric    metric.Service
	Logs      logs.Service
}

func defaultServices() serviceSet {
	return serviceSet{
		Monitor:   monitor.LiveService{},
		Dashboard: dashboard.LiveService{},
		Host:      host.LiveService{},
		Metric:    metric.LiveService{},
		Logs:      logs.LiveService{},
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

func formatCount(count int) string {
	return strconv.Itoa(count)
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

func addCoreCommands(root *cobra.Command, opts *GlobalOptions) {
	root.AddCommand(newMonitorCmd(opts))
	root.AddCommand(newDashboardCmd(opts))
	root.AddCommand(newHostCmd(opts))
	root.AddCommand(newMetricCmd(opts))
	root.AddCommand(newLogCmd(opts))
}

func newMonitorCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "monitor", Short: "Inspect Datadog monitors", GroupID: "core"}
	cmd.AddCommand(newMonitorListCmd(opts), newMonitorGetCmd(opts))
	return cmd
}

func newMonitorListCmd(opts *GlobalOptions) *cobra.Command {
	var params monitor.ListParams
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List monitors",
		Args:    cobra.NoArgs,
		Long:    "List Datadog monitors with optional name and tag filters. API pagination can be controlled with --offset and --limit.",
		Example: "ddog monitor list\n  ddog monitor list --name api --limit 20\n  ddog monitor list --tags env:prod --offset 20 --limit 20 --output json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if params.Limit < 0 {
				return fmt.Errorf("--limit cannot be negative")
			}
			if params.Offset < 0 {
				return fmt.Errorf("--offset cannot be negative")
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			result, err := opts.Services.Monitor.List(cmd.Context(), cfg, params)
			if err != nil {
				return err
			}
			return output.Write(cmd.OutOrStdout(), cfg.Output, result, func(w io.Writer) error {
				rows := make([][]string, 0, len(result.Items))
				for _, item := range result.Items {
					rows = append(rows, []string{strconv.FormatInt(item.ID, 10), truncateForTable(item.Name, 32), item.State, item.Type, truncateForTable(item.Query, 48)})
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

func newMonitorGetCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <monitor-id>",
		Short:   "Get monitor details",
		Args:    cobra.ExactArgs(1),
		Example: "ddog monitor get 123456\n  ddog monitor get 123456 --output json",
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid monitor id %q", args[0])
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			item, err := opts.Services.Monitor.Get(cmd.Context(), cfg, id)
			if err != nil {
				return err
			}
			return output.Write(cmd.OutOrStdout(), cfg.Output, item, func(w io.Writer) error {
				return output.KeyValue(w, [][2]string{{"ID", strconv.FormatInt(item.ID, 10)}, {"Name", item.Name}, {"State", item.State}, {"Type", item.Type}, {"Priority", formatInt64Pointer(item.Priority)}, {"Query", item.Query}, {"Tags", formatStringSlice(item.Tags)}, {"Created", formatOptionalTime(item.CreatedAt)}, {"Modified", formatOptionalTime(item.ModifiedAt)}, {"Message", item.Message}})
			})
		},
	}
	return cmd
}

func newDashboardCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "dashboard", Short: "Inspect Datadog dashboards", GroupID: "core"}
	cmd.AddCommand(newDashboardListCmd(opts), newDashboardGetCmd(opts))
	return cmd
}

func newDashboardListCmd(opts *GlobalOptions) *cobra.Command {
	var params dashboard.ListParams
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List dashboards",
		Args:    cobra.NoArgs,
		Example: "ddog dashboard list\n  ddog dashboard list --count 20\n  ddog dashboard list --output json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if params.Count < 0 {
				return fmt.Errorf("--count cannot be negative")
			}
			if params.Start < 0 {
				return fmt.Errorf("--start cannot be negative")
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			result, err := opts.Services.Dashboard.List(cmd.Context(), cfg, params)
			if err != nil {
				return err
			}
			return output.Write(cmd.OutOrStdout(), cfg.Output, result, func(w io.Writer) error {
				rows := make([][]string, 0, len(result.Items))
				for _, item := range result.Items {
					rows = append(rows, []string{item.ID, truncateForTable(item.Title, 36), item.LayoutType, truncateForTable(item.Author, 24), formatOptionalTime(item.ModifiedAt)})
				}
				return output.Table(w, []string{"ID", "TITLE", "LAYOUT", "AUTHOR", "MODIFIED"}, rows)
			})
		},
	}
	cmd.Flags().Int64Var(&params.Count, "count", 0, "Maximum dashboards to return")
	cmd.Flags().Int64Var(&params.Start, "start", 0, "Pagination offset")
	cmd.Flags().BoolVar(&params.IncludeShared, "shared", false, "Include shared dashboards")
	cmd.Flags().BoolVar(&params.IncludeDeleted, "deleted", false, "Include deleted dashboards")
	return cmd
}

func newDashboardGetCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "get <dashboard-id>",
		Short:   "Get dashboard details",
		Args:    cobra.ExactArgs(1),
		Example: "ddog dashboard get abc-def-ghi\n  ddog dashboard get abc-def-ghi --output json",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			item, err := opts.Services.Dashboard.Get(cmd.Context(), cfg, args[0])
			if err != nil {
				return err
			}
			return output.Write(cmd.OutOrStdout(), cfg.Output, item, func(w io.Writer) error {
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
		Example: "ddog host list\n  ddog host list --filter web\n  ddog host list --count 50 --output json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if params.Count < 0 {
				return fmt.Errorf("--count cannot be negative")
			}
			if params.Start < 0 {
				return fmt.Errorf("--start cannot be negative")
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			result, err := opts.Services.Host.List(cmd.Context(), cfg, params)
			if err != nil {
				return err
			}
			return output.Write(cmd.OutOrStdout(), cfg.Output, result, func(w io.Writer) error {
				rows := make([][]string, 0, len(result.Items))
				for _, item := range result.Items {
					rows = append(rows, []string{truncateForTable(item.Name, 28), item.Platform, item.AgentVersion, formatBool(item.Up), formatBool(item.Muted), formatOptionalTime(item.LastReportedAt)})
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
		Example: "ddog host get web-01\n  ddog host get web-01 --output json",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			item, err := opts.Services.Host.Get(cmd.Context(), cfg, args[0])
			if err != nil {
				return err
			}
			return output.Write(cmd.OutOrStdout(), cfg.Output, item, func(w io.Writer) error {
				return output.KeyValue(w, [][2]string{{"Name", item.Name}, {"Host Name", item.HostName}, {"AWS Name", item.AWSName}, {"Aliases", formatStringSlice(item.Aliases)}, {"Apps", formatStringSlice(item.Apps)}, {"Platform", item.Platform}, {"Agent", item.AgentVersion}, {"Up", formatBool(item.Up)}, {"Muted", formatBool(item.Muted)}, {"Last Reported", formatOptionalTime(item.LastReportedAt)}})
			})
		},
	}
	return cmd
}

func newMetricCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "metric", Short: "Query Datadog metrics", GroupID: "core"}
	cmd.AddCommand(newMetricQueryCmd(opts))
	return cmd
}

func newMetricQueryCmd(opts *GlobalOptions) *cobra.Command {
	var query string
	var last, from, to string
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query metric timeseries",
		Long: strings.TrimSpace(`Query Datadog metric timeseries for a relative or absolute time window.

Prefer '--output json' when an agent or script will parse the result. JSON returns a top-level 'series' array with summary fields instead of raw pointlists: 'point_count' is the number of non-empty points returned, 'last_point_ts' is the timestamp of the last non-empty point, and 'last_value' is that point's numeric value.`),
		Args:    cobra.NoArgs,
		Example: "ddog metric query --query 'avg:system.load.1{*}' --last 1h\n  ddog metric query --query 'avg:system.cpu.user{env:prod}' --from 2026-03-21T09:00:00Z --to 2026-03-21T10:00:00Z --output json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(query) == "" {
				return fmt.Errorf("--query is required")
			}
			rangeValue, err := timeutil.ParseRangeWithDefault(last, from, to, time.Hour, time.Now)
			if err != nil {
				return err
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			result, err := opts.Services.Metric.Query(cmd.Context(), cfg, metric.QueryParams{Query: query, Range: rangeValue})
			if err != nil {
				return err
			}
			return output.Write(cmd.OutOrStdout(), cfg.Output, result, func(w io.Writer) error {
				rows := make([][]string, 0, len(result.Series))
				for _, item := range result.Series {
					lastValue := ""
					if item.LastValue != nil {
						lastValue = strconv.FormatFloat(*item.LastValue, 'f', -1, 64)
					}
					rows = append(rows, []string{truncateForTable(firstNonEmpty(item.Metric, item.Expression), 36), truncateForTable(item.Scope, 24), item.Aggregator, formatCount(item.PointCount), lastValue, formatOptionalTime(item.LastPointTS)})
				}
				if err := output.Table(w, []string{"SERIES", "SCOPE", "AGGR", "POINTS", "LAST", "LAST POINT"}, rows); err != nil {
					return err
				}
				_, err := fmt.Fprintf(w, "\nReturned %s series for range %s to %s\n", formatCount(result.Count), formatTime(result.From), formatTime(result.To))
				return err
			})
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "Datadog metric query, for example 'avg:system.cpu.user{env:prod}'")
	cmd.Flags().StringVar(&last, "last", "", "Relative lookback duration, such as 15m or 1h")
	cmd.Flags().StringVar(&from, "from", "", "Range start in RFC3339")
	cmd.Flags().StringVar(&to, "to", "", "Range end in RFC3339 or 'now'")
	return cmd
}

func newLogCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "log", Short: "Search Datadog logs", GroupID: "core"}
	cmd.AddCommand(newLogSearchCmd(opts))
	return cmd
}

func newLogSearchCmd(opts *GlobalOptions) *cobra.Command {
	var query string
	var last, from, to string
	var limit int32
	var indexes []string
	var sort string
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search logs",
		Long: strings.TrimSpace(`Search Datadog logs with a Datadog log query and a narrow time window.

Start with a focused query such as 'service:web status:error' or 'env:prod @http.status_code:[500 TO 599]'. Use '--last' for quick exploration, add '--index' to target specific log indexes, and prefer '--output json' when an agent or script needs stable fields like 'items', 'count', and per-entry 'message', 'service', 'status', 'host', and 'timestamp'.`),
		Args:    cobra.NoArgs,
		Example: "ddog log search --query 'service:web status:error' --last 15m\n  ddog log search --query 'env:prod @http.status_code:[500 TO 599]' --last 30m --index main --limit 20 --output json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(query) == "" {
				return fmt.Errorf("--query is required")
			}
			rangeValue, err := timeutil.ParseRangeWithDefault(last, from, to, 15*time.Minute, time.Now)
			if err != nil {
				return err
			}
			if sort != "asc" && sort != "desc" {
				return fmt.Errorf("--sort must be 'asc' or 'desc'")
			}
			if limit < 0 {
				return fmt.Errorf("--limit cannot be negative")
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			result, err := opts.Services.Logs.Search(cmd.Context(), cfg, logs.SearchParams{Query: query, Range: rangeValue, Limit: limit, Indexes: indexes, SortAsc: sort == "asc"})
			if err != nil {
				return err
			}
			return output.Write(cmd.OutOrStdout(), cfg.Output, result, func(w io.Writer) error {
				rows := make([][]string, 0, len(result.Items))
				for _, item := range result.Items {
					rows = append(rows, []string{formatOptionalTime(item.Timestamp), truncateForTable(item.Service, 18), truncateForTable(item.Status, 10), truncateForTable(item.Host, 18), truncateForTable(item.Message, 72)})
				}
				if err := output.Table(w, []string{"TIMESTAMP", "SERVICE", "STATUS", "HOST", "MESSAGE"}, rows); err != nil {
					return err
				}
				_, err := fmt.Fprintf(w, "\nReturned %s logs for range %s to %s\n", formatCount(result.Count), formatTime(result.From), formatTime(result.To))
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
	cmd.Flags().StringVar(&sort, "sort", "desc", "Sort order: asc or desc")
	return cmd
}
