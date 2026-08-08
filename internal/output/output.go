package output

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode"
)

type Format string

const (
	Text Format = "text"
	JSON Format = "json"
	TOON Format = "toon"
)

func ParseFormat(raw string) (Format, error) {
	switch Format(strings.ToLower(strings.TrimSpace(raw))) {
	case "", Text:
		return Text, nil
	case JSON:
		return JSON, nil
	case TOON:
		return TOON, nil
	default:
		return "", fmt.Errorf("unsupported output format %q", raw)
	}
}

// Project selects comma-separated top-level fields from an object or each
// object in an array. It is intentionally structural and lossless for the
// selected values; nested field expansion can be added without changing the
// command contract.
func Project(value any, fields string) (any, error) {
	requested := make([]string, 0)
	seen := map[string]struct{}{}
	for _, raw := range strings.Split(fields, ",") {
		field := strings.TrimSpace(raw)
		if field == "" {
			return nil, fmt.Errorf("--fields contains an empty field")
		}
		if strings.ContainsAny(field, " \t\r\n") {
			return nil, fmt.Errorf("--fields contains invalid field %q", field)
		}
		if _, ok := seen[field]; !ok {
			seen[field] = struct{}{}
			requested = append(requested, field)
		}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("project output: %w", err)
	}
	var normalized any
	if err := json.Unmarshal(data, &normalized); err != nil {
		return nil, fmt.Errorf("project output: %w", err)
	}
	projectObject := func(item map[string]any) map[string]any {
		result := make(map[string]any, len(requested))
		for _, field := range requested {
			if value, ok := item[field]; ok {
				result[field] = value
			}
		}
		return result
	}
	switch item := normalized.(type) {
	case map[string]any:
		return projectObject(item), nil
	case []any:
		result := make([]any, len(item))
		for i, value := range item {
			object, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("--fields requires an object or array of objects")
			}
			result[i] = projectObject(object)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("--fields requires an object or array of objects")
	}
}

func Write(w io.Writer, format Format, value any, textRenderer func(io.Writer) error) error {
	if format == JSON {
		encoder := json.NewEncoder(w)
		return encoder.Encode(value)
	}
	if format == TOON {
		_, err := io.WriteString(w, EncodeTOON(value))
		return err
	}
	return textRenderer(w)
}

// EncodeTOON renders a deterministic, JSON-compatible TOON-like representation.
// It intentionally keeps all data in memory and sorts object keys so agent output
// remains stable across runs and Go map iteration order.
func EncodeTOON(value any) string {
	var b strings.Builder
	writeTOON(&b, reflect.ValueOf(value), 0, "")
	if b.Len() == 0 || b.String()[b.Len()-1] != '\n' {
		b.WriteByte('\n')
	}
	return b.String()
}

func writeTOON(b *strings.Builder, v reflect.Value, indent int, key string) {
	if !v.IsValid() {
		writeScalar(b, indent, key, nil)
		return
	}
	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			writeScalar(b, indent, key, nil)
			return
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Struct {
		bytes, err := json.Marshal(v.Interface())
		if err != nil {
			writeScalar(b, indent, key, fmt.Sprint(v.Interface()))
			return
		}
		var normalized any
		if err := json.Unmarshal(bytes, &normalized); err != nil {
			writeScalar(b, indent, key, fmt.Sprint(v.Interface()))
			return
		}
		if m, ok := normalized.(map[string]any); ok {
			writeMap(b, m, indent, key)
			return
		}
		writeTOON(b, reflect.ValueOf(normalized), indent, key)
		return
	}
	if v.Kind() == reflect.Map {
		m := map[string]any{}
		for _, k := range v.MapKeys() {
			m[fmt.Sprint(k.Interface())] = v.MapIndex(k).Interface()
		}
		writeMap(b, m, indent, key)
		return
	}
	if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
		items := make([]any, v.Len())
		for i := range items {
			items[i] = v.Index(i).Interface()
		}
		writeSlice(b, items, indent, key)
		return
	}
	writeScalar(b, indent, key, v.Interface())
}

func writeMap(b *strings.Builder, m map[string]any, indent int, key string) {
	if len(m) == 0 {
		if key != "" {
			b.WriteString(strings.Repeat("  ", indent) + key + ":\n")
		}
		return
	}
	if key != "" {
		b.WriteString(strings.Repeat("  ", indent))
		b.WriteString(key)
		b.WriteString(":\n")
		indent++
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		writeTOON(b, reflect.ValueOf(m[k]), indent, k)
	}
}

func writeSlice(b *strings.Builder, items []any, indent int, key string) {
	if len(items) == 0 {
		b.WriteString(strings.Repeat("  ", indent))
		if key == "" {
			b.WriteString("[]\n")
		} else {
			b.WriteString(key + ": []\n")
		}
		return
	}
	allScalar := true
	for _, item := range items {
		if !isScalar(item) {
			allScalar = false
			break
		}
	}
	if allScalar {
		b.WriteString(strings.Repeat("  ", indent))
		if key != "" {
			b.WriteString(key)
		}
		fmt.Fprintf(b, "[%d]: ", len(items))
		values := make([]string, len(items))
		for i, item := range items {
			values[i] = toonScalar(item)
		}
		b.WriteString(strings.Join(values, ","))
		b.WriteByte('\n')
		return
	}
	// Tabular arrays are compact and deterministic when every item is an object
	// with the same scalar keys.
	if headers, ok := tabularHeaders(items); ok {
		b.WriteString(strings.Repeat("  ", indent))
		b.WriteString(key)
		fmt.Fprintf(b, "[%d]{%s}:\n", len(items), strings.Join(headers, ","))
		for _, item := range items {
			m := item.(map[string]any)
			b.WriteString(strings.Repeat("  ", indent+1))
			vals := make([]string, len(headers))
			for i, h := range headers {
				vals[i] = toonScalar(m[h])
			}
			b.WriteString(strings.Join(vals, ","))
			b.WriteByte('\n')
		}
		return
	}
	b.WriteString(strings.Repeat("  ", indent))
	if key != "" {
		b.WriteString(key)
	}
	fmt.Fprintf(b, "[%d]:\n", len(items))
	for _, item := range items {
		writeListItem(b, item, indent+1)
	}
}

func writeListItem(b *strings.Builder, item any, indent int) {
	prefix := strings.Repeat("  ", indent) + "- "
	if isScalar(item) {
		b.WriteString(prefix + toonScalar(item) + "\n")
		return
	}
	if values, ok := item.([]any); ok {
		if allScalarValues(values) {
			b.WriteString(prefix)
			fmt.Fprintf(b, "[%d]: ", len(values))
			parts := make([]string, len(values))
			for i, value := range values {
				parts[i] = toonScalar(value)
			}
			b.WriteString(strings.Join(parts, ",") + "\n")
			return
		}
		b.WriteString(prefix)
		fmt.Fprintf(b, "[%d]:\n", len(values))
		for _, value := range values {
			writeListItem(b, value, indent+1)
		}
		return
	}
	if object, ok := item.(map[string]any); ok {
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			b.WriteString(prefix + "{}\n")
			return
		}
		first := true
		for _, key := range keys {
			value := object[key]
			linePrefix := strings.Repeat("  ", indent)
			if first {
				linePrefix += "- "
				first = false
			} else {
				linePrefix += "  "
			}
			linePrefix += key
			if isScalar(value) {
				b.WriteString(linePrefix + ": " + toonScalar(value) + "\n")
				continue
			}
			b.WriteString(linePrefix + ":\n")
			writeTOON(b, reflect.ValueOf(value), indent+2, "")
		}
		return
	}
	b.WriteString(prefix + toonScalar(item) + "\n")
}

func allScalarValues(items []any) bool {
	for _, item := range items {
		if !isScalar(item) {
			return false
		}
	}
	return true
}

func tabularHeaders(items []any) ([]string, bool) {
	first, ok := items[0].(map[string]any)
	if !ok || len(first) == 0 {
		return nil, false
	}
	keys := make([]string, 0, len(first))
	for k, v := range first {
		if !isScalar(v) {
			return nil, false
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok || len(m) != len(keys) {
			return nil, false
		}
		for _, k := range keys {
			if _, exists := m[k]; !exists {
				return nil, false
			}
			if !isScalar(m[k]) {
				return nil, false
			}
		}
	}
	return keys, true
}

func writeScalar(b *strings.Builder, indent int, key string, value any) {
	b.WriteString(strings.Repeat("  ", indent))
	if key != "" {
		b.WriteString(key)
		b.WriteString(": ")
	}
	b.WriteString(toonScalar(value))
	b.WriteByte('\n')
}
func isScalar(v any) bool {
	if v == nil {
		return true
	}
	k := reflect.TypeOf(v).Kind()
	return k != reflect.Map && k != reflect.Struct && k != reflect.Slice && k != reflect.Array
}
func toonScalar(value any) string {
	if value == nil {
		return "null"
	}
	switch v := value.(type) {
	case string:
		return quote(v)
	case bool:
		return strconv.FormatBool(v)
	case float32, float64:
		return fmt.Sprint(v)
	case int, int8, int16, int32, int64:
		return fmt.Sprint(v)
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprint(v)
	case map[string]any:
		return "{}"
	case []any:
		return "[]"
	}
	return quote(fmt.Sprint(value))
}
func quote(value string) string {
	if value == "" || strings.ContainsAny(value, ",:\n\r\t\"'\\[]{}#") || strings.TrimSpace(value) != value || value == "null" || value == "true" || value == "false" || value == "-" || strings.HasPrefix(value, "-") || containsControl(value) {
		return quoteJSON(value)
	}
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return quoteJSON(value)
	}
	return value
}

func containsControl(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func quoteJSON(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return strconv.Quote(value)
	}
	return string(encoded)
}

func Table(w io.Writer, headers []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, strings.Join(sanitizeSlice(headers), "\t")); err != nil {
		return err
	}
	if len(rows) == 0 {
		if _, err := fmt.Fprintln(tw, "(no results)"); err != nil {
			return err
		}
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(tw, strings.Join(sanitizeSlice(row), "\t")); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func KeyValue(w io.Writer, pairs [][2]string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, pair := range pairs {
		if _, err := fmt.Fprintf(tw, "%s\t%s\n", sanitizeText(pair[0]), sanitizeText(pair[1])); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func sanitizeSlice(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, sanitizeText(item))
	}
	return result
}

func sanitizeText(value string) string {
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r == '\n':
			builder.WriteString(`\n`)
		case r == '\r':
			builder.WriteString(`\r`)
		case r == '\t':
			builder.WriteString(`\t`)
		case unicode.IsControl(r):
			builder.WriteString(`\x`)
			builder.WriteString(strings.ToUpper(strconv.FormatInt(int64(r), 16)))
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
