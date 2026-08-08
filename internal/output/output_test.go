package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestSanitizeText(t *testing.T) {
	got := sanitizeText("hello\nworld\x1b[31m\tend")
	if strings.Contains(got, "\x1b") || strings.Contains(got, "\nworld\n") {
		t.Fatalf("unexpected unsafe output: %q", got)
	}
	if !strings.Contains(got, `\n`) || !strings.Contains(got, `\t`) || !strings.Contains(got, `\x1B`) {
		t.Fatalf("expected escaped control characters, got %q", got)
	}
}

func TestTableSanitizesCells(t *testing.T) {
	buf := new(bytes.Buffer)
	err := Table(buf, []string{"NAME"}, [][]string{{"line1\nline2\x1b[31m"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "\x1b") || strings.Contains(out, "line1\nline2") {
		t.Fatalf("unexpected unsafe table output: %q", out)
	}
}

func TestTableStatesEmptyResults(t *testing.T) {
	buf := new(bytes.Buffer)
	if err := Table(buf, []string{"ID"}, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "(no results)") {
		t.Fatalf("expected explicit empty result state, got %q", buf.String())
	}
}

func TestEncodeTOONDeterministicTable(t *testing.T) {
	got := EncodeTOON(map[string]any{"items": []map[string]any{{"z": "two", "id": 1}, {"z": "three", "id": 2}}})
	want := "items[2]{id,z}:\n  1,two\n  2,three\n"
	if got != want {
		t.Fatalf("unexpected TOON output: %q", got)
	}
}

func TestEncodeTOONListAndNestedValues(t *testing.T) {
	got := EncodeTOON(map[string]any{"items": []any{map[string]any{"id": 1, "points": []any{[]any{float64(1), float64(2)}}}}})
	if !strings.Contains(got, "items[1]:\n  - id: 1\n    points:\n      [1]:\n        - [2]: 1,2\n") {
		t.Fatalf("unexpected nested TOON output: %q", got)
	}
	if got := EncodeTOON(map[string]any{"value": "42", "control": "\x1b"}); !strings.Contains(got, `value: "42"`) || !strings.Contains(got, `control: "\u001b"`) {
		t.Fatalf("unsafe or ambiguous scalar encoding: %q", got)
	}
}

func TestEncodeTOONPreservesHeterogeneousAndEmptyShapes(t *testing.T) {
	got := EncodeTOON(map[string]any{
		"items":        []any{map[string]any{"a": 1, "b": 2}, map[string]any{"a": 3, "c": 4}},
		"empty_object": map[string]any{},
		"empty_array":  []any{},
	})
	if strings.Contains(got, "items[2]{a,b}") || !strings.Contains(got, "c: 4") || !strings.Contains(got, "empty_array: []") || !strings.Contains(got, "empty_object:") {
		t.Fatalf("heterogeneous or empty shapes were not preserved: %q", got)
	}
}

func TestProjectSelectsFieldsForObjectsAndRows(t *testing.T) {
	value, err := Project(map[string]any{"id": "m-1", "name": "CPU", "query": "avg:test"}, "id,name")
	if err != nil {
		t.Fatal(err)
	}
	if got := EncodeTOON(value); got != "id: m-1\nname: CPU\n" {
		t.Fatalf("unexpected object projection: %q", got)
	}
	rows, err := Project([]map[string]any{{"id": "m-1", "name": "CPU", "query": "avg:test"}}, "id")
	if err != nil {
		t.Fatal(err)
	}
	if got := EncodeTOON(rows); got != "[1]{id}:\n  m-1\n" {
		t.Fatalf("unexpected row projection: %q", got)
	}
}
