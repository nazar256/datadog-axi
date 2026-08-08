package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/nazar256/datadog-axi/internal/domain/dashboard"
	"github.com/nazar256/datadog-axi/internal/domain/monitor"
	cliruntime "github.com/nazar256/datadog-axi/internal/runtime"
	"github.com/spf13/cobra"
)

func addLocalSpecCommands(cmd *cobra.Command, kind string, opts *GlobalOptions) {
	cmd.AddCommand(localValidateCmd(kind, opts), localUpdateCmd(kind, opts))
}

func localValidateCmd(kind string, opts *GlobalOptions) *cobra.Command {
	var fileFlag string
	var remote bool
	var existingID string
	cmd := &cobra.Command{Use: "validate [file]", Short: "Validate a " + kind + " JSON specification locally", Example: "datadog-axi " + kind + " validate .tmp/" + kind + ".json\n  datadog-axi " + kind + " validate --file - --json", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		file := strings.TrimSpace(fileFlag)
		if len(args) == 1 {
			if file != "" {
				return usagef("provide the specification through either a positional file or --file, not both")
			}
			file = args[0]
		}
		if file == "" {
			return usagef("validate requires a file argument or --file")
		}
		data, err := readSpecInput(cmd, file)
		if err != nil {
			return fmt.Errorf("read specification: %w", err)
		}
		var value map[string]any
		if err := json.Unmarshal(data, &value); err != nil {
			return usageError(fmt.Errorf("invalid JSON specification: %w", err))
		}
		if value == nil {
			return usagef("invalid %s specification: expected a JSON object", kind)
		}
		if kind == "monitor" && strings.TrimSpace(existingID) != "" {
			id, parseErr := strconv.ParseInt(strings.TrimSpace(existingID), 10, 64)
			if parseErr != nil || id <= 0 {
				return usagef("invalid --existing-id %q", existingID)
			}
			if _, present := value["id"]; !present {
				value["id"] = float64(id)
			}
		}
		if err := validateSpecificationShape(kind, value); err != nil {
			return err
		}
		if strings.TrimSpace(existingID) != "" {
			remote = true
			id, _ := strconv.ParseInt(strings.TrimSpace(existingID), 10, 64)
			if candidateID, ok := value["id"].(float64); ok && int64(candidateID) != id {
				return usagef("monitor specification id %.0f does not match --existing-id %d", candidateID, id)
			}
		}
		var remoteResult any
		if remote {
			if kind != "monitor" {
				return usagef("remote validation is currently supported only for monitors")
			}
			cfg, err := resolveLiveConfig(opts)
			if err != nil {
				return err
			}
			if strings.TrimSpace(existingID) != "" {
				id, parseErr := strconv.ParseInt(strings.TrimSpace(existingID), 10, 64)
				if parseErr != nil || id <= 0 {
					return usagef("invalid --existing-id %q", existingID)
				}
				validator, ok := opts.Services.Monitor.(monitor.ExistingValidator)
				if !ok {
					return fmt.Errorf("existing monitor validation is unavailable for the configured service")
				}
				remoteResult, err = validator.ValidateExisting(cmd.Context(), cfg, id, value)
			} else {
				validator, ok := opts.Services.Monitor.(monitor.Validator)
				if !ok {
					return fmt.Errorf("monitor remote validation is unavailable for the configured service")
				}
				remoteResult, err = validator.Validate(cmd.Context(), cfg, value)
			}
			if err != nil {
				return err
			}
		}
		cfg, err := runtimeConfigForOffline(opts)
		if err != nil {
			return err
		}
		result := map[string]any{"validated_locally": true, "remote_validation": "available only through Datadog API", "kind": kind, "file": file}
		if remote {
			result["remote_validated"] = true
			result["remote_result"] = remoteResult
		}
		return writeOutput(cmd, opts, cfg, result, func(w io.Writer) error {
			message := fmt.Sprintf("%s specification passed local shape validation", kind)
			if remote {
				message += "; remote validation completed"
			} else {
				message += "; remote validation was not requested"
			}
			_, err := fmt.Fprintln(w, message)
			return err
		})
	}}
	cmd.Flags().StringVar(&fileFlag, "file", "", "Specification JSON file, or - for stdin")
	cmd.Flags().BoolVar(&remote, "remote", false, "Validate against Datadog when a documented validation endpoint exists")
	if kind == "monitor" {
		cmd.Flags().StringVar(&existingID, "existing-id", "", "Validate an existing monitor id using the existing-resource endpoint")
		cmd.Flags().StringVar(&existingID, "id", "", "Alias for --existing-id")
	}
	return cmd
}

func localUpdateCmd(kind string, opts *GlobalOptions) *cobra.Command {
	var apply bool
	var dryRun bool
	var fingerprint string
	var fileFlag string
	var allowStale bool
	dryRun = true
	cmd := &cobra.Command{Use: "update <id> [file]", Short: "Preview or apply a " + kind + " update (dry-run by default)", Example: "datadog-axi " + kind + " update 123 .tmp/" + kind + ".json --json\n  datadog-axi " + kind + " update 123 --file .tmp/" + kind + ".json --apply --fingerprint <sha256>", Args: cobra.RangeArgs(1, 2), RunE: func(cmd *cobra.Command, args []string) error {
		file := args[0]
		id := ""
		if len(args) == 2 {
			id, file = args[0], args[1]
		} else if strings.TrimSpace(fileFlag) != "" {
			id, file = args[0], strings.TrimSpace(fileFlag)
		}
		if len(args) == 2 && strings.TrimSpace(fileFlag) != "" {
			return usagef("provide the specification through either a positional file or --file, not both")
		}
		data, err := readSpecInput(cmd, file)
		if err != nil {
			return fmt.Errorf("read specification: %w", err)
		}
		var value map[string]any
		if err := json.Unmarshal(data, &value); err != nil || value == nil {
			if err == nil {
				err = fmt.Errorf("expected a JSON object")
			}
			return usageError(fmt.Errorf("invalid %s specification: %w", kind, err))
		}
		if err := validateSpecificationShape(kind, value); err != nil {
			return err
		}
		// A one-argument invocation remains an offline preview for compatibility.
		if id == "" {
			if apply {
				return usagef("--apply requires update <id> <file>")
			}
			actualFingerprint := sha256.Sum256(canonicalJSON(value))
			fingerprintValue := hex.EncodeToString(actualFingerprint[:])
			cfg, err := runtimeConfigForOffline(opts)
			if err != nil {
				return err
			}
			return writeOutput(cmd, opts, cfg, map[string]any{"dry_run": true, "semantic_diff": "unavailable", "remote_fetch": "deferred", "kind": kind, "file": file, "fingerprint": fingerprintValue, "apply": "provide <id> <file> for guarded live workflow"}, func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "%s update preview is local-only; provide an id for guarded live workflow\n", kind)
				return err
			})
		}
		if apply && cmd.Flags().Changed("dry-run") && dryRun {
			return usagef("--apply cannot be combined with --dry-run; use --dry-run=false with --apply")
		}
		if !apply && !dryRun {
			return usagef("choose --dry-run or --apply")
		}
		if allowStale && !apply {
			return usagef("--allow-stale requires --apply")
		}
		return runGuardedUpdate(cmd, opts, kind, id, value, apply, fingerprint, allowStale)
	}}
	cmd.Flags().BoolVar(&apply, "apply", false, "Apply the update (requires --fingerprint)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", true, "Show the semantic diff without writing (default)")
	cmd.Flags().StringVar(&fingerprint, "fingerprint", "", "Expected live specification fingerprint (SHA-256 of canonical JSON)")
	cmd.Flags().StringVar(&fileFlag, "file", "", "Specification JSON file, or - for stdin")
	cmd.Flags().BoolVar(&allowStale, "allow-stale", false, "Allow a reviewed fingerprint mismatch (requires --apply and an explicit fingerprint)")
	return cmd
}

func readSpecInput(cmd *cobra.Command, path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(cmd.InOrStdin())
	}
	return os.ReadFile(path)
}

func canonicalJSON(value any) []byte { data, _ := json.Marshal(value); return data }

func fingerprintJSON(value any) string {
	sum := sha256.Sum256(canonicalJSON(value))
	return hex.EncodeToString(sum[:])
}

type semanticChange struct {
	Path   string `json:"path"`
	Before any    `json:"before,omitempty"`
	After  any    `json:"after,omitempty"`
}

func runGuardedUpdate(cmd *cobra.Command, opts *GlobalOptions, kind, id string, candidate map[string]any, apply bool, expectedFingerprint string, allowStale bool) error {
	if candidateID, ok := candidate["id"]; ok {
		if kind == "monitor" {
			if fmt.Sprint(candidateID) != id {
				return usagef("specification id %v does not match target monitor id %s", candidateID, id)
			}
		} else if fmt.Sprint(candidateID) != id {
			return usagef("specification id %v does not match target dashboard id %s", candidateID, id)
		}
	}
	updateCandidate := mutableCandidate(kind, candidate)
	cfg, err := resolveLiveConfig(opts)
	if err != nil {
		return err
	}
	var live any
	var update func() (any, error)
	var refetch func() (any, error)
	switch kind {
	case "monitor":
		monitorID, e := strconv.ParseInt(id, 10, 64)
		if e != nil || monitorID <= 0 {
			return usagef("invalid monitor id %q", id)
		}
		exporter, ok := opts.Services.Monitor.(monitor.RawExporter)
		if !ok {
			return fmt.Errorf("monitor export is unavailable for the configured service")
		}
		updater, ok := opts.Services.Monitor.(monitor.RawUpdater)
		if !ok {
			return fmt.Errorf("monitor update is unavailable for the configured service")
		}
		liveValue, e := exporter.ExportRaw(cmd.Context(), cfg, monitorID)
		if e != nil {
			return e
		}
		live = liveValue
		update = func() (any, error) {
			return updater.UpdateRaw(cmd.Context(), cfg, monitorID, updateCandidate)
		}
		refetch = func() (any, error) { return exporter.ExportRaw(cmd.Context(), cfg, monitorID) }
	case "dashboard":
		exporter, ok := opts.Services.Dashboard.(dashboard.RawExporter)
		if !ok {
			return fmt.Errorf("dashboard export is unavailable for the configured service")
		}
		updater, ok := opts.Services.Dashboard.(dashboard.RawUpdater)
		if !ok {
			return fmt.Errorf("dashboard update is unavailable for the configured service")
		}
		liveValue, e := exporter.ExportRaw(cmd.Context(), cfg, id)
		if e != nil {
			return e
		}
		live = liveValue
		update = func() (any, error) {
			return updater.UpdateRaw(cmd.Context(), cfg, id, updateCandidate)
		}
		refetch = func() (any, error) { return exporter.ExportRaw(cmd.Context(), cfg, id) }
	default:
		return usagef("unsupported update kind %q", kind)
	}
	liveMap, err := asObject(live)
	if err != nil {
		return err
	}
	// Start from the fetched mutable resource and overlay the reviewed file so
	// omitted fields and unknown nested properties are preserved on update.
	updateCandidate = mergeMaps(mutableCandidate(kind, liveMap), updateCandidate)
	diffs := sanitizeDiffs(semanticDiff(updateCandidate, liveMap, ""), cfg)
	liveFingerprint := fingerprintJSON(liveMap)
	if !apply {
		return writeOutput(cmd, opts, cfg, map[string]any{"dry_run": true, "kind": kind, "id": id, "live_fingerprint": liveFingerprint, "changed": len(diffs) > 0, "semantic_diff": diffs}, func(w io.Writer) error {
			_, e := fmt.Fprintf(w, "%s update dry-run: %d semantic differences\n", kind, len(diffs))
			return e
		})
	}
	if strings.TrimSpace(expectedFingerprint) == "" {
		return usagef("--fingerprint is required with --apply to guard against stale updates")
	}
	expectedFingerprint = strings.TrimSpace(expectedFingerprint)
	if len(expectedFingerprint) != sha256.Size*2 {
		return usagef("--fingerprint must be a 64-character SHA-256 hex digest")
	}
	if _, err := hex.DecodeString(expectedFingerprint); err != nil {
		return usagef("--fingerprint must be a 64-character SHA-256 hex digest")
	}
	staleOverride := false
	if !strings.EqualFold(expectedFingerprint, liveFingerprint) {
		if allowStale {
			staleOverride = true
		} else {
			return usagef("stale fingerprint: live=%s expected=%s", liveFingerprint, strings.TrimSpace(expectedFingerprint))
		}
	}
	if len(diffs) == 0 {
		return writeOutput(cmd, opts, cfg, map[string]any{"updated": false, "no_op": true, "kind": kind, "id": id, "stale_override": staleOverride, "live_fingerprint": liveFingerprint}, func(w io.Writer) error {
			_, e := fmt.Fprintln(w, kind+" update is already in the requested state (no-op)")
			return e
		})
	}
	if _, err := update(); err != nil {
		// A transport or API error can arrive after Datadog accepted the
		// request. Tell the operator how to recover without retrying a
		// potentially successful mutation blindly.
		return fmt.Errorf("%s update failed or may have partially succeeded: %w; re-export resource %s before retrying", kind, err, id)
	}
	updated, err := refetch()
	if err != nil {
		return fmt.Errorf("update may have succeeded but post-update refetch failed: %w; re-export resource %s before retrying", err, id)
	}
	updatedMap, err := asObject(updated)
	if err != nil {
		return fmt.Errorf("update may have succeeded but post-update response was unusable: %w; re-export resource %s before retrying", err, id)
	}
	remaining := sanitizeDiffs(semanticDiff(updateCandidate, updatedMap, ""), cfg)
	if len(remaining) > 0 {
		return fmt.Errorf("post-update verification failed: %d semantic differences remain; re-export the resource and review the returned state before retrying", len(remaining))
	}
	return writeOutput(cmd, opts, cfg, map[string]any{"updated": true, "kind": kind, "id": id, "stale_override": staleOverride, "live_fingerprint": fingerprintJSON(updatedMap), "semantic_diff": diffs}, func(w io.Writer) error { _, e := fmt.Fprintln(w, kind+" update applied and verified"); return e })
}

func mutableCandidate(kind string, candidate map[string]any) map[string]any {
	readOnly := map[string]struct{}{"id": {}, "created": {}, "created_at": {}, "modified": {}, "modified_at": {}, "deleted": {}, "creator": {}, "url": {}, "author_handle": {}, "author_name": {}, "is_read_only": {}, "overall_state": {}, "state": {}, "matching_downtimes": {}}
	result := make(map[string]any, len(candidate))
	for key, value := range candidate {
		if kind == "dashboard" {
			if _, ok := readOnly[key]; ok {
				continue
			}
		} else if _, ok := readOnly[key]; ok {
			continue
		}
		result[key] = value
	}
	return result
}

func mergeMaps(base, overlay map[string]any) map[string]any {
	result := make(map[string]any, len(base)+len(overlay))
	for key, value := range base {
		result[key] = value
	}
	for key, value := range overlay {
		baseMap, baseOK := result[key].(map[string]any)
		overlayMap, overlayOK := value.(map[string]any)
		if baseOK && overlayOK {
			result[key] = mergeMaps(baseMap, overlayMap)
			continue
		}
		result[key] = value
	}
	return result
}

func asObject(value any) (map[string]any, error) {
	data, _ := json.Marshal(value)
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil || result == nil {
		return nil, fmt.Errorf("specification is not a JSON object")
	}
	return result, nil
}

func semanticDiff(expected, actual map[string]any, path string) []semanticChange {
	var diffs []semanticChange
	for key, want := range expected {
		current, ok := actual[key]
		next := key
		if path != "" {
			next = path + "." + key
		}
		if !ok {
			diffs = append(diffs, change(next, nil, want, redactedKey(key)))
			continue
		}
		wm, wok := want.(map[string]any)
		cm, cok := current.(map[string]any)
		if wok && cok {
			if redactedKey(key) {
				diffs = append(diffs, change(next, current, want, true))
				continue
			}
			diffs = append(diffs, semanticDiff(wm, cm, next)...)
			continue
		}
		if !jsonEqual(want, current) {
			diffs = append(diffs, change(next, current, want, redactedKey(key)))
		}
	}
	sort.Slice(diffs, func(i, j int) bool { return diffs[i].Path < diffs[j].Path })
	return diffs
}

func change(path string, before, after any, redact bool) semanticChange {
	if redact || containsSensitive(before) || containsSensitive(after) {
		return semanticChange{Path: path, Before: "[REDACTED]", After: "[REDACTED]"}
	}
	return semanticChange{Path: path, Before: before, After: after}
}

func redactedKey(key string) bool {
	key = strings.ToLower(key)
	for _, part := range []string{"secret", "token", "password", "api_key", "apikey", "app_key", "application_key", "authorization", "private_key", "access_key", "credential", "bearer", "cookie", "certificate", "signature"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}

func containsSensitive(value any) bool {
	switch item := value.(type) {
	case string:
		return containsSensitiveString(item)
	case map[string]any:
		for key, nested := range item {
			if redactedKey(key) || containsSensitive(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range item {
			if containsSensitive(nested) {
				return true
			}
		}
	}
	return false
}

func containsSensitiveString(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{"api_key=", "app_key=", "apikey=", "token=", "password=", "authorization:", "bearer "} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func sanitizeDiffs(diffs []semanticChange, cfg cliruntime.Config) []semanticChange {
	for i := range diffs {
		diffs[i].Before = sanitizeDiffValue(diffs[i].Before, cfg)
		diffs[i].After = sanitizeDiffValue(diffs[i].After, cfg)
	}
	return diffs
}

func sanitizeDiffValue(value any, cfg cliruntime.Config) any {
	switch item := value.(type) {
	case string:
		return cliruntime.SanitizeError(item, cfg)
	case []any:
		result := make([]any, len(item))
		for i, nested := range item {
			result[i] = sanitizeDiffValue(nested, cfg)
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(item))
		for key, nested := range item {
			result[key] = sanitizeDiffValue(nested, cfg)
		}
		return result
	default:
		return value
	}
}
func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

func validateSpecificationShape(kind string, value map[string]any) error {
	if _, ok := value["id"]; !ok {
		return usagef("invalid %s specification: id is required; creation is not supported", kind)
	}
	switch kind {
	case "monitor":
		for _, field := range []string{"name", "type", "query"} {
			if _, ok := value[field]; !ok {
				return usagef("invalid monitor specification: %s is required", field)
			}
		}
	case "dashboard":
		for _, field := range []string{"title", "layout_type", "widgets"} {
			if _, ok := value[field]; !ok {
				return usagef("invalid dashboard specification: %s is required", field)
			}
		}
	}
	if kind == "monitor" {
		if _, ok := value["id"].(float64); !ok {
			return usagef("invalid monitor specification: id must be a number")
		}
		for _, field := range []string{"name", "type", "query"} {
			if _, ok := value[field].(string); !ok {
				return usagef("invalid monitor specification: %s must be a string", field)
			}
		}
		monitorType := strings.ToLower(strings.TrimSpace(value["type"].(string)))
		if monitorType == "" {
			return usagef("invalid monitor specification: type cannot be empty")
		}
		if options, present := value["options"]; present {
			optionsMap, ok := options.(map[string]any)
			if !ok {
				return usagef("invalid monitor specification: options must be an object")
			}
			if thresholds, present := optionsMap["thresholds"]; present {
				thresholdMap, ok := thresholds.(map[string]any)
				if !ok {
					return usagef("invalid monitor specification: options.thresholds must be an object")
				}
				for _, key := range []string{"critical", "warning", "critical_recovery", "warning_recovery", "unknown"} {
					if raw, ok := thresholdMap[key]; ok {
						switch raw.(type) {
						case float64, int, int64, json.Number:
						default:
							return usagef("invalid monitor specification: options.thresholds.%s must be numeric", key)
						}
					}
				}
			}
			for _, key := range []string{"scheduling_options"} {
				if raw, ok := optionsMap[key]; ok {
					if _, ok := raw.(map[string]any); !ok {
						return usagef("invalid monitor specification: options.%s must be an object", key)
					}
				}
			}
			if raw, ok := optionsMap["notify_by"]; ok {
				values, ok := raw.([]any)
				if !ok {
					return usagef("invalid monitor specification: options.notify_by must be an array")
				}
				for _, value := range values {
					if _, ok := value.(string); !ok {
						return usagef("invalid monitor specification: options.notify_by values must be strings")
					}
				}
			}
		}
	} else if kind == "dashboard" {
		if _, ok := value["id"].(string); !ok {
			return usagef("invalid dashboard specification: id must be a string")
		}
		for _, field := range []string{"title", "layout_type"} {
			if _, ok := value[field].(string); !ok {
				return usagef("invalid dashboard specification: %s must be a string", field)
			}
		}
		if _, ok := value["widgets"].([]any); !ok {
			return usagef("invalid dashboard specification: widgets must be an array")
		}
		for index, widget := range value["widgets"].([]any) {
			if _, ok := widget.(map[string]any); !ok {
				return usagef("invalid dashboard specification: widgets[%d] must be an object", index)
			}
		}
	}
	return nil
}
