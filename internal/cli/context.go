package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/nazar256/datadog-axi/internal/domain/audit"
	"github.com/nazar256/datadog-axi/internal/domain/servicecatalog"
	"github.com/nazar256/datadog-axi/internal/domain/spans"
	"github.com/nazar256/datadog-axi/internal/output"
	"github.com/nazar256/datadog-axi/internal/timeutil"
	"github.com/spf13/cobra"
)

func newSpanCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "span", Short: "Search APM spans", GroupID: "core"}
	cmd.AddCommand(newSpanListCmd(opts), newSpanAggregateCmd(opts), newSpanServicesCmd(opts), newSpanFacetDiscoveryCmd(opts, "resource"), newSpanFacetDiscoveryCmd(opts, "operation"))
	return cmd
}

func newSpanListCmd(opts *GlobalOptions) *cobra.Command {
	var params spans.SearchParams
	var last, from, to, sortOrder string
	var durationMin, durationMax string
	var allowBroad bool
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "Search APM spans",
		Args:    cobra.NoArgs,
		Example: "datadog-axi span list --query 'service:web env:prod' --last 15m --limit 20 --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if params.Limit < 0 || params.Limit > 1000 {
				return usagef("--limit must be between 0 and 1000")
			}
			if strings.ContainsAny(params.Cursor, "\r\n") {
				return usagef("--cursor contains invalid control characters")
			}
			if durationMin != "" {
				value, err := time.ParseDuration(durationMin)
				if err != nil || value < 0 {
					return usagef("--duration-min must be a non-negative duration such as 250ms")
				}
				params.DurationMin = &value
			}
			if durationMax != "" {
				value, err := time.ParseDuration(durationMax)
				if err != nil || value < 0 {
					return usagef("--duration-max must be a non-negative duration such as 2s")
				}
				params.DurationMax = &value
			}
			if params.DurationMin != nil && params.DurationMax != nil && *params.DurationMin > *params.DurationMax {
				return usagef("--duration-min cannot exceed --duration-max")
			}
			if strings.TrimSpace(params.Query) == "" && !hasSpanFilters(params) && !allowBroad {
				return usagef("a span query or explicit span filter is required; pass --allow-broad for a bounded broad search")
			}
			rangeValue, err := timeutil.ParseRangeWithDefault(last, from, to, 15*time.Minute, time.Now)
			if err != nil {
				return usageError(err)
			}
			params.Range = rangeValue
			switch sortOrder {
			case "asc":
				params.SortAsc = true
			case "desc":
			default:
				return usagef("--sort must be 'asc' or 'desc'")
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			result, err := opts.Services.Spans.Search(cmd.Context(), cfg, params)
			if err != nil {
				return err
			}
			return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
				rows := make([][]string, 0, len(result.Items))
				for _, item := range result.Items {
					rows = append(rows, []string{item.ID, formatOptionalTime(item.Start), preview(opts, item.Service, 18), preview(opts, item.ResourceName, 32), item.Operation, item.Status, formatFloatPointer(item.DurationMS), item.Env, item.TraceID, item.SpanID})
				}
				if err := output.Table(w, []string{"ID", "START", "SERVICE", "RESOURCE", "OPERATION", "STATUS", "DURATION_MS", "ENV", "TRACE", "SPAN"}, rows); err != nil {
					return err
				}
				if result.NextCursor != "" {
					_, err := fmt.Fprintf(w, "\nNext cursor: %s\n", result.NextCursor)
					return err
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&params.Query, "query", "", "Span search query")
	cmd.Flags().StringVar(&params.Service, "service", "", "Filter by service")
	cmd.Flags().StringVar(&params.Env, "env", "", "Filter by environment")
	cmd.Flags().StringVar(&params.Operation, "operation", "", "Filter by operation name")
	cmd.Flags().StringVar(&params.Resource, "resource", "", "Filter by resource name")
	cmd.Flags().StringVar(&params.Status, "status", "", "Filter by span status")
	cmd.Flags().StringArrayVar(&params.Tags, "tag", nil, "Filter by span tag (repeatable)")
	cmd.Flags().StringVar(&params.TraceID, "trace-id", "", "Filter by trace ID")
	cmd.Flags().StringVar(&params.SpanID, "span-id", "", "Filter by span ID")
	cmd.Flags().StringVar(&durationMin, "duration-min", "", "Minimum span duration, such as 250ms")
	cmd.Flags().StringVar(&durationMax, "duration-max", "", "Maximum span duration, such as 2s")
	cmd.Flags().BoolVar(&allowBroad, "allow-broad", false, "Allow a bounded broad search when no query or explicit filters are supplied")
	cmd.Flags().StringVar(&last, "last", "", "Relative lookback duration, such as 15m")
	cmd.Flags().StringVar(&from, "from", "", "Range start in RFC3339")
	cmd.Flags().StringVar(&to, "to", "", "Range end in RFC3339 or 'now'")
	cmd.Flags().Int32Var(&params.Limit, "limit", 100, "Maximum spans to return")
	cmd.Flags().StringVar(&params.Cursor, "cursor", "", "Opaque pagination cursor")
	cmd.Flags().StringVar(&sortOrder, "sort", "desc", "Sort order (asc or desc)")
	return cmd
}

func hasSpanFilters(params spans.SearchParams) bool {
	return strings.TrimSpace(params.Service) != "" || strings.TrimSpace(params.Env) != "" || strings.TrimSpace(params.Operation) != "" || strings.TrimSpace(params.Resource) != "" || strings.TrimSpace(params.Status) != "" || params.DurationMin != nil || params.DurationMax != nil || len(params.Tags) > 0 || strings.TrimSpace(params.TraceID) != "" || strings.TrimSpace(params.SpanID) != ""
}

func newSpanAggregateCmd(opts *GlobalOptions) *cobra.Command {
	var params spans.AggregateParams
	var last, from, to string
	var groupBy, compute []string
	cmd := &cobra.Command{
		Use:     "aggregate",
		Short:   "Aggregate spans into bounded buckets",
		Args:    cobra.NoArgs,
		Example: "datadog-axi span aggregate --query 'service:web' --group-by service,env --compute count --last 15m --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if params.BucketLimit < 1 || params.BucketLimit > 1000 {
				return usagef("--limit must be between 1 and 1000")
			}
			var err error
			params.Range, err = timeutil.ParseRangeWithDefault(last, from, to, 15*time.Minute, time.Now)
			if err != nil {
				return usageError(err)
			}
			params.GroupBy = splitCommaList(groupBy)
			params.Compute = splitCommaList(compute)
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			result, err := opts.Services.Spans.Aggregate(cmd.Context(), cfg, params)
			if err != nil {
				return err
			}
			return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
				rows := make([][]string, 0, len(result.Buckets))
				for _, bucket := range result.Buckets {
					by, _ := json.Marshal(bucket.By)
					computes, _ := json.Marshal(bucket.Computes)
					rows = append(rows, []string{bucket.ID, string(by), string(computes)})
				}
				if err := output.Table(w, []string{"ID", "GROUP", "COMPUTES"}, rows); err != nil {
					return err
				}
				if len(result.Warnings) > 0 {
					_, err := fmt.Fprintf(w, "\nWarnings: %s\n", strings.Join(result.Warnings, "; "))
					return err
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&params.Query, "query", "", "Span aggregate query (defaults to * within the bounded time range)")
	cmd.Flags().StringVar(&last, "last", "", "Relative lookback duration, such as 15m")
	cmd.Flags().StringVar(&from, "from", "", "Range start in RFC3339")
	cmd.Flags().StringVar(&to, "to", "", "Range end in RFC3339 or 'now'")
	cmd.Flags().StringArrayVar(&groupBy, "group-by", []string{"service"}, "Group-by (repeat or comma-separate): service, resource, operation, env")
	cmd.Flags().StringArrayVar(&compute, "compute", []string{"count"}, "Compute (repeat or comma-separate; currently count)")
	cmd.Flags().Int64Var(&params.BucketLimit, "limit", 20, "Maximum buckets per group-by")
	return cmd
}

func newSpanServicesCmd(opts *GlobalOptions) *cobra.Command {
	var query, last, from, to string
	var limit int64
	cmd := &cobra.Command{
		Use:     "services",
		Short:   "Discover active services from span aggregates",
		Args:    cobra.NoArgs,
		Example: "datadog-axi span services --query 'env:prod' --last 1h --limit 50 --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 1 || limit > 1000 {
				return usagef("--limit must be between 1 and 1000")
			}
			rangeValue, err := timeutil.ParseRangeWithDefault(last, from, to, time.Hour, time.Now)
			if err != nil {
				return usageError(err)
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			result, err := opts.Services.Spans.Aggregate(cmd.Context(), cfg, spans.AggregateParams{Query: query, Range: rangeValue, GroupBy: []string{"service"}, Compute: []string{"count"}, BucketLimit: limit})
			if err != nil {
				return err
			}
			return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
				rows := make([][]string, 0, len(result.Buckets))
				for _, bucket := range result.Buckets {
					serviceName := aggregateValueString(bucket.By["service"])
					count := aggregateValueString(bucket.Computes["count"])
					if serviceName == "" {
						by, _ := json.Marshal(bucket.By)
						serviceName = string(by)
					}
					rows = append(rows, []string{serviceName, count})
				}
				if err := output.Table(w, []string{"SERVICE", "COUNT"}, rows); err != nil {
					return err
				}
				_, err := fmt.Fprintln(w, "Dependency relationships are not available from a public Datadog SDK endpoint in v2.46.0.")
				return err
			})
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "Span query (defaults to * within the bounded time range)")
	cmd.Flags().StringVar(&last, "last", "", "Relative lookback duration, such as 1h")
	cmd.Flags().StringVar(&from, "from", "", "Range start in RFC3339")
	cmd.Flags().StringVar(&to, "to", "", "Range end in RFC3339 or 'now'")
	cmd.Flags().Int64Var(&limit, "limit", 50, "Maximum services to return")
	return cmd
}

func newSpanFacetDiscoveryCmd(opts *GlobalOptions, facet string) *cobra.Command {
	var query, last, from, to string
	var limit int64
	plural := facet + "s"
	return &cobra.Command{
		Use:     plural,
		Short:   "Discover " + facet + " values from span aggregates",
		Args:    cobra.NoArgs,
		Example: "datadog-axi span " + plural + " --query 'service:web' --last 1h --limit 50 --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 1 || limit > 1000 {
				return usagef("--limit must be between 1 and 1000")
			}
			rangeValue, err := timeutil.ParseRangeWithDefault(last, from, to, time.Hour, time.Now)
			if err != nil {
				return usageError(err)
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			result, err := opts.Services.Spans.Aggregate(cmd.Context(), cfg, spans.AggregateParams{Query: query, Range: rangeValue, GroupBy: []string{facet}, Compute: []string{"count"}, BucketLimit: limit})
			if err != nil {
				return err
			}
			return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
				rows := make([][]string, 0, len(result.Buckets))
				for _, bucket := range result.Buckets {
					value := aggregateValueString(bucket.By[facet])
					if value == "" {
						alias := facet + "_name"
						value = aggregateValueString(bucket.By[alias])
					}
					rows = append(rows, []string{value, aggregateValueString(bucket.Computes["count"])})
				}
				if err := output.Table(w, []string{strings.ToUpper(facet), "COUNT"}, rows); err != nil {
					return err
				}
				_, err := fmt.Fprintln(w, "Discovery values are derived from the selected time window and query; they are not an authoritative catalog.")
				return err
			})
		},
	}
}

func splitCommaValues(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func splitCommaList(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, splitCommaValues(value)...)
	}
	return result
}

func aggregateValueString(value any) string {
	switch typed := value.(type) {
	case *string:
		if typed != nil {
			return *typed
		}
	case *float64:
		if typed != nil {
			return strconv.FormatFloat(*typed, 'f', -1, 4)
		}
	case *int64:
		if typed != nil {
			return strconv.FormatInt(*typed, 10)
		}
	}
	return stringValue(value)
}

func newAuditCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "audit", Short: "Search audit log events", GroupID: "core"}
	cmd.AddCommand(newAuditListCmd(opts))
	return cmd
}

func newAuditListCmd(opts *GlobalOptions) *cobra.Command {
	var params audit.SearchParams
	var last, from, to, sortOrder string
	cmd := &cobra.Command{Use: "list", Short: "Search audit logs", Args: cobra.NoArgs, Example: "datadog-axi audit list --query 'service:monitors' --last 1h --limit 20 --json", RunE: func(cmd *cobra.Command, args []string) error {
		if params.Limit < 0 || params.Limit > 1000 {
			return usagef("--limit must be between 0 and 1000")
		}
		if strings.ContainsAny(params.Cursor, "\r\n") {
			return usagef("--cursor contains invalid control characters")
		}
		rangeValue, err := timeutil.ParseRangeWithDefault(last, from, to, time.Hour, time.Now)
		if err != nil {
			return usageError(err)
		}
		params.Range = rangeValue
		switch sortOrder {
		case "asc":
			params.SortAsc = true
		case "desc":
		default:
			return usagef("--sort must be 'asc' or 'desc'")
		}
		cfg, err := resolveLiveConfig(opts)
		if err != nil {
			return err
		}
		result, err := opts.Services.Audit.Search(cmd.Context(), cfg, params)
		if err != nil {
			return err
		}
		return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
			rows := make([][]string, 0, len(result.Items))
			for _, item := range result.Items {
				rows = append(rows, []string{item.ID, formatOptionalTime(item.Timestamp), preview(opts, item.Actor, 20), preview(opts, item.Service, 20), preview(opts, item.Action, 20), preview(opts, item.Resource, 24), item.Type, preview(opts, item.Message, 64)})
			}
			if err := output.Table(w, []string{"ID", "TIMESTAMP", "ACTOR", "SERVICE", "ACTION", "RESOURCE", "TYPE", "MESSAGE"}, rows); err != nil {
				return err
			}
			if result.NextCursor != "" {
				_, err := fmt.Fprintf(w, "\nNext cursor: %s\n", result.NextCursor)
				return err
			}
			return nil
		})
	}}
	cmd.Flags().StringVar(&params.Query, "query", "", "Audit log search query")
	cmd.Flags().StringVar(&params.Actor, "actor", "", "Filter by audit actor")
	cmd.Flags().StringVar(&params.Service, "service", "", "Filter by audit service")
	cmd.Flags().StringVar(&params.Action, "action", "", "Filter by audit action")
	cmd.Flags().StringVar(&params.Resource, "resource", "", "Filter by audit resource")
	cmd.Flags().StringArrayVar(&params.Tags, "tag", nil, "Filter by audit tag (repeatable)")
	cmd.Flags().StringVar(&last, "last", "", "Relative lookback duration, such as 1h")
	cmd.Flags().StringVar(&from, "from", "", "Range start in RFC3339")
	cmd.Flags().StringVar(&to, "to", "", "Range end in RFC3339 or 'now'")
	cmd.Flags().Int32Var(&params.Limit, "limit", 100, "Maximum audit events to return")
	cmd.Flags().StringVar(&params.Cursor, "cursor", "", "Opaque pagination cursor")
	cmd.Flags().StringVar(&sortOrder, "sort", "desc", "Sort order (asc or desc)")
	return cmd
}

func newServiceCatalogCmd(opts *GlobalOptions) *cobra.Command {
	cmd := &cobra.Command{Use: "service", Short: "Inspect service ownership definitions", GroupID: "core"}
	cmd.AddCommand(newServiceListCmd(opts), newServiceGetCmd(opts))
	return cmd
}

func newEventGetCmd(opts *GlobalOptions) *cobra.Command {
	return &cobra.Command{Use: "get <event-id>", Short: "Get event details", Args: cobra.ExactArgs(1), Example: "datadog-axi event get 123456 --json", RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return usagef("invalid event id %q", args[0])
		}
		cfg, err := resolveLiveConfig(opts)
		if err != nil {
			return err
		}
		result, err := opts.Services.Event.Get(cmd.Context(), cfg, id)
		if err != nil {
			return err
		}
		return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
			return output.KeyValue(w, [][2]string{{"ID", strconv.FormatInt(result.ID, 10)}, {"TITLE", result.Title}, {"SOURCE", result.Source}, {"STATUS", result.Status}, {"TIMESTAMP", formatOptionalTime(result.Timestamp)}, {"TEXT", result.Text}, {"URL", result.URL}})
		})
	}}
}

func newDowntimeGetCmd(opts *GlobalOptions) *cobra.Command {
	return &cobra.Command{Use: "get <downtime-id>", Short: "Get downtime details", Args: cobra.ExactArgs(1), Example: "datadog-axi downtime get abc-123 --json", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := resolveLiveConfig(opts)
		if err != nil {
			return err
		}
		result, err := opts.Services.Downtime.Get(cmd.Context(), cfg, args[0])
		if err != nil {
			return err
		}
		return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
			return output.KeyValue(w, [][2]string{{"ID", result.ID}, {"STATUS", result.Status}, {"SCOPE", result.Scope}, {"MESSAGE", result.Message}, {"CREATED", formatOptionalTime(result.Created)}, {"MODIFIED", formatOptionalTime(result.Modified)}, {"CANCELED", formatOptionalTime(result.Canceled)}, {"MONITOR ID", formatInt64Pointer(result.MonitorID)}, {"MONITOR TAGS", formatStringSlice(result.MonitorTags)}, {"NOTIFY END STATES", formatStringSlice(result.NotifyEndStates)}})
		})
	}}
}

func newServiceListCmd(opts *GlobalOptions) *cobra.Command {
	var params servicecatalog.ListParams
	cmd := &cobra.Command{Use: "list", Short: "List service definitions", Long: "List one bounded service-catalog page. --filter is applied to that returned page; use --offset to inspect later pages.", Args: cobra.NoArgs, Example: "datadog-axi service list --filter checkout --limit 20 --json", RunE: func(cmd *cobra.Command, args []string) error {
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
		result, err := opts.Services.Services.List(cmd.Context(), cfg, params)
		if err != nil {
			return err
		}
		return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
			rows := make([][]string, 0, len(result.Items))
			for _, item := range result.Items {
				rows = append(rows, []string{item.ID, preview(opts, item.Name, 28), preview(opts, item.Owner, 20), item.Lifecycle, item.Tier, preview(opts, item.Description, 48)})
			}
			return output.Table(w, []string{"ID", "NAME", "OWNER", "LIFECYCLE", "TIER", "DESCRIPTION"}, rows)
		})
	}}
	cmd.Flags().StringVar(&params.Filter, "filter", "", "Filter service name, owner, lifecycle, or description")
	cmd.Flags().Int64Var(&params.Limit, "limit", 100, "Maximum services to return")
	cmd.Flags().Int64Var(&params.Offset, "offset", 0, "Pagination offset")
	cmd.Flags().StringVar(&params.SchemaVersion, "schema-version", "", "Service definition schema version")
	return cmd
}

func newServiceGetCmd(opts *GlobalOptions) *cobra.Command {
	return &cobra.Command{Use: "get <service-name>", Short: "Get a service definition", Args: cobra.ExactArgs(1), Example: "datadog-axi service get checkout --json", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := resolveLiveConfig(opts)
		if err != nil {
			return err
		}
		result, err := opts.Services.Services.Get(cmd.Context(), cfg, args[0])
		if err != nil {
			return err
		}
		return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
			return output.KeyValue(w, [][2]string{{"ID", result.ID}, {"Name", result.Name}, {"Owner", result.Owner}, {"Lifecycle", result.Lifecycle}, {"Tier", result.Tier}, {"Description", result.Description}, {"Repositories", strconv.Itoa(len(result.Repositories))}, {"Documentation", strconv.Itoa(len(result.Documentation))}, {"Dependencies", strconv.Itoa(len(result.Dependencies))}})
		})
	}}
}
