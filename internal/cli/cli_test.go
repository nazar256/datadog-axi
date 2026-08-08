package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nazar256/datadog-axi/internal/runtime"
)

func TestRootCmdHelp(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{
		Version: "1.0.0",
		Commit:  "abcdef",
		Date:    "2023-10-27",
	})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "Core Commands:") {
		t.Errorf("expected help to contain 'Core Commands:', got:\n%s", output)
	}
	if !strings.Contains(output, "Utility Commands:") {
		t.Errorf("expected help to contain 'Utility Commands:', got:\n%s", output)
	}
	if !strings.Contains(output, "datadog-axi is a Datadog CLI for humans, coding agents, and automation.") {
		t.Errorf("expected help to contain updated product description")
	}
	if !strings.Contains(output, "version") {
		t.Errorf("expected help to contain 'version' command")
	}
	if !strings.Contains(output, "monitor") {
		t.Errorf("expected help to contain 'monitor' command")
	}
	if !strings.Contains(output, "docs") {
		t.Errorf("expected help to contain 'docs' command")
	}
	if !strings.Contains(output, "config") {
		t.Errorf("expected help to contain 'config' command")
	}
	if !strings.Contains(output, "doctor") {
		t.Errorf("expected help to contain 'doctor' command")
	}
	utilityIndex := strings.Index(output, "Utility Commands:")
	if utilityIndex == -1 {
		t.Fatalf("expected help to contain Utility Commands section, got:\n%s", output)
	}
	utilitySection := output[utilityIndex:]
	if additionalIndex := strings.Index(utilitySection, "Additional Commands:"); additionalIndex != -1 {
		utilitySection = utilitySection[:additionalIndex]
	}
	if !strings.Contains(utilitySection, "completion") {
		t.Errorf("expected Utility Commands to contain 'completion' command, got:\n%s", output)
	}
	if !strings.Contains(output, "--site") {
		t.Errorf("expected help to contain '--site' flag")
	}
	if !strings.Contains(output, "--env-file") {
		t.Errorf("expected help to contain '--env-file' flag")
	}
}

func TestHomeViewUsesAXIIdentityKeys(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{Product: "datadog-axi"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--no-env-file", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected home-view error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, `"bin":`) || !strings.Contains(output, `"description":`) || !strings.Contains(output, `"auth":`) || !strings.Contains(output, `"api_key":false`) || !strings.Contains(output, `"app_key":false`) {
		t.Fatalf("home view lacks AXI identity/readiness keys: %s", output)
	}
}

func TestRedactStructuredOutputRemovesSecretKeysAndConfiguredValues(t *testing.T) {
	view := redactStructuredOutput(map[string]any{"raw": map[string]any{"api_key": "secret", "message": "token=secret"}}, runtime.Config{APIKey: "secret"})
	data, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret") || !strings.Contains(string(data), "[REDACTED]") {
		t.Fatalf("structured output leaked secret: %s", data)
	}
}

func TestGeneratedCommandDocsFollowRegisteredLeaves(t *testing.T) {
	docs := generatedCommandDocs(NewRootCmd(BuildInfo{Product: "datadog-axi"}))
	if len(docs) < 20 {
		t.Fatalf("generated command docs unexpectedly small: %d", len(docs))
	}
	seen := make(map[string]bool, len(docs))
	for _, doc := range docs {
		if doc.Path == "" || seen[doc.Path] {
			t.Fatalf("invalid or duplicate generated command path: %#v", doc)
		}
		seen[doc.Path] = true
	}
}

func TestVersionCmd(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{
		Version: "1.0.0",
		Commit:  "abcdef",
		Date:    "2023-10-27",
	})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version", "--output", "text"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	expected := "datadog-axi version 1.0.0 (commit: abcdef, date: 2023-10-27)\n"
	if output != expected {
		t.Errorf("expected %q, got %q", expected, output)
	}
}

func TestVersionCmdJSON(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{
		Version: "1.0.0",
		Commit:  "abcdef",
		Date:    "2023-10-27",
	})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"version", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"version":"1.0.0"`) {
		t.Fatalf("unexpected output: %s", output)
	}
}

func TestConfigDoctorCmd(t *testing.T) {
	t.Setenv("DATADOG_API_KEY", "super-secret-api")
	t.Setenv("DATADOG_APP_KEY", "super-secret-app")

	cmd := NewRootCmd(BuildInfo{})

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"config", "doctor", "--no-env-file", "--output", "text"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Site      datadoghq.com") {
		t.Errorf("expected output to contain default site, got:\n%s", output)
	}
	if !strings.Contains(output, "API Key   present") {
		t.Errorf("expected output to report present API key, got:\n%s", output)
	}
	if !strings.Contains(output, "App Key   present") {
		t.Errorf("expected output to report present App key, got:\n%s", output)
	}
	if !strings.Contains(output, "Status    ready") {
		t.Errorf("expected output to report ready status, got:\n%s", output)
	}
	if strings.Contains(output, "super-secret-api") || strings.Contains(output, "super-secret-app") {
		t.Errorf("output should not leak secret keys")
	}
}

func TestConfigDoctorJSONPreservesPresenceAndSourceMetadata(t *testing.T) {
	t.Setenv("DD_API_KEY", "super-secret-api")
	t.Setenv("DD_APP_KEY", "super-secret-app")
	cmd := NewRootCmd(BuildInfo{})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"config", "doctor", "--no-env-file", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := buf.String()
	for _, expected := range []string{`"api_key":"present"`, `"app_key":"present"`, `"api_key":"process:DD_API_KEY"`, `"app_key":"process:DD_APP_KEY"`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("doctor JSON lost safe metadata %q: %s", expected, output)
		}
	}
	if strings.Contains(output, "super-secret") {
		t.Fatalf("doctor JSON leaked credentials: %s", output)
	}
}

func TestDoctorAliasMatchesConfigDoctor(t *testing.T) {
	t.Setenv("DATADOG_API_KEY", "super-secret-api")
	t.Setenv("DATADOG_APP_KEY", "super-secret-app")

	run := func(args ...string) string {
		cmd := NewRootCmd(BuildInfo{})
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected error for %v: %v", args, err)
		}
		return buf.String()
	}

	aliasOutput := run("doctor", "--no-env-file")
	nestedOutput := run("config", "doctor", "--no-env-file")
	if aliasOutput != nestedOutput {
		t.Fatalf("expected doctor alias to match config doctor\nalias:\n%s\nconfig doctor:\n%s", aliasOutput, nestedOutput)
	}
}

func TestCompletionHelpIncludesSupportedShells(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"completion", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		if !strings.Contains(output, shell) {
			t.Fatalf("expected completion help to mention %s, got:\n%s", shell, output)
		}
	}
}

func TestDocsCommandsJSONMentionsDoctorAndCompletion(t *testing.T) {
	cmd := NewRootCmd(BuildInfo{})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"docs", "commands", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "doctor") {
		t.Fatalf("expected docs commands output to mention doctor, got: %s", output)
	}
	if !strings.Contains(output, "completion") {
		t.Fatalf("expected docs commands output to mention completion, got: %s", output)
	}
	if !strings.Contains(output, "pagination") && !strings.Contains(output, "cursor") {
		t.Fatalf("expected docs commands output to include machine-readable command metadata, got: %s", output)
	}
}
