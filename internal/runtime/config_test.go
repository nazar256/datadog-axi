package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nazar256/datadog-axi/internal/timeutil"
)

func TestResolveConfig(t *testing.T) {
	// Create a temporary .env file
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	err := os.WriteFile(envFile, []byte("DATADOG_SITE=eu\nDATADOG_API_KEY=env_api\nDATADOG_APP_KEY=env_app\n"), 0644)
	if err != nil {
		t.Fatalf("failed to write temp env file: %v", err)
	}

	tests := []struct {
		name       string
		flags      FlagValues
		envVars    map[string]string
		wantSite   string
		wantAPIKey string
		wantAppKey string
		wantErr    bool
	}{
		{
			name: "defaults",
			flags: FlagValues{
				NoEnvFile: true,
			},
			wantSite: DefaultSite,
		},
		{
			name: "flag overrides env var",
			flags: FlagValues{
				Site:      "us3",
				NoEnvFile: true,
			},
			envVars: map[string]string{
				"DATADOG_SITE": "eu",
			},
			wantSite: "us3.datadoghq.com",
		},
		{
			name: "env var overrides .env file",
			flags: FlagValues{
				EnvFile: envFile,
			},
			envVars: map[string]string{
				"DATADOG_SITE":    "us5",
				"DATADOG_API_KEY": "var_api",
			},
			wantSite:   "us5.datadoghq.com",
			wantAPIKey: "var_api",
			wantAppKey: "env_app",
		},
		{
			name: "empty env masks dotenv values",
			flags: FlagValues{
				EnvFile: envFile,
			},
			envVars: map[string]string{
				"DATADOG_SITE":    "",
				"DATADOG_API_KEY": "",
				"DATADOG_APP_KEY": "",
			},
			wantSite: DefaultSite,
		},
		{
			name: "no env file flag ignores .env",
			flags: FlagValues{
				EnvFile:   envFile,
				NoEnvFile: true,
			},
			wantSite: DefaultSite,
			wantErr:  true,
		},
		{
			name: "invalid site",
			flags: FlagValues{
				Site:      "invalid",
				NoEnvFile: true,
			},
			wantErr: true,
		},
		{
			name: "reject unknown dotted host",
			flags: FlagValues{
				Site:      "evil.example.com",
				NoEnvFile: true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			cfg, err := ResolveConfig(tt.flags)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if cfg.Site != tt.wantSite {
				t.Errorf("ResolveConfig() Site = %v, want %v", cfg.Site, tt.wantSite)
			}
			if tt.wantAPIKey != "" && cfg.APIKey != tt.wantAPIKey {
				t.Errorf("ResolveConfig() APIKey = %v, want %v", cfg.APIKey, tt.wantAPIKey)
			}
			if tt.wantAPIKey == "" && cfg.APIKey != "" {
				t.Errorf("ResolveConfig() APIKey = %v, want empty", cfg.APIKey)
			}
			if tt.wantAppKey != "" && cfg.AppKey != tt.wantAppKey {
				t.Errorf("ResolveConfig() AppKey = %v, want %v", cfg.AppKey, tt.wantAppKey)
			}
			if tt.wantAppKey == "" && cfg.AppKey != "" {
				t.Errorf("ResolveConfig() AppKey = %v, want empty", cfg.AppKey)
			}
		})
	}
}

func TestResolveConfigStandardAliases(t *testing.T) {
	t.Setenv("DD_API_KEY", "alias-api")
	t.Setenv("DD_APP_KEY", "alias-app")
	t.Setenv("DD_SITE", "us3")
	cfg, err := ResolveConfig(FlagValues{NoEnvFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "alias-api" || cfg.AppKey != "alias-app" || cfg.Site != "us3.datadoghq.com" {
		t.Fatalf("unexpected alias config: %+v", cfg)
	}
	if cfg.Sources["api_key"] != "process:DD_API_KEY" || cfg.Sources["site"] != "process:DD_SITE" {
		t.Fatalf("unexpected config sources: %+v", cfg.Sources)
	}
}

func TestResolveConfigPrefersStandardAliasesOverLegacy(t *testing.T) {
	t.Setenv("DD_API_KEY", "preferred-api")
	t.Setenv("DATADOG_API_KEY", "legacy-api")
	t.Setenv("DD_APP_KEY", "preferred-app")
	t.Setenv("DATADOG_APP_KEY", "legacy-app")
	t.Setenv("DD_SITE", "us5")
	t.Setenv("DATADOG_SITE", "eu")
	cfg, err := ResolveConfig(FlagValues{NoEnvFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "preferred-api" || cfg.AppKey != "preferred-app" || cfg.Site != "us5.datadoghq.com" {
		t.Fatalf("standard aliases did not win: %+v", cfg)
	}
}

func TestResolveConfigLayeredEnvUsesMostSpecificFile(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "repo", "nested")
	if err := os.MkdirAll(child, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("DD_SITE=eu\nDD_API_KEY=lower-api\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, ".env"), []byte("DATADOG_SITE=us5\nDATADOG_API_KEY=higher-api\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(child)
	for _, key := range []string{"DD_SITE", "DATADOG_SITE"} {
		old, present := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if present {
				_ = os.Setenv(key, old)
			} else {
				_ = os.Unsetenv(key)
			}
		})
	}
	cfg, err := ResolveConfig(FlagValues{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Site != "us5.datadoghq.com" || cfg.APIKey != "higher-api" {
		t.Fatalf("most-specific env file did not win: site=%s files=%v", cfg.Site, cfg.EnvFiles)
	}
}

func TestNormalizeSite(t *testing.T) {
	tests := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{"us1", "datadoghq.com", false},
		{"us3", "us3.datadoghq.com", false},
		{"eu", "datadoghq.eu", false},
		{"us5.datadoghq.com", "us5.datadoghq.com", false},
		{"invalid", "", true},
		{"custom.datadoghq.com", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := normalizeSite(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("normalizeSite() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("normalizeSite() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseRangeRejectsNonPositiveLast(t *testing.T) {
	_, err := timeutil.ParseRange("-15m", "", "", func() time.Time { return time.Unix(0, 0) })
	if err == nil {
		t.Fatal("expected error for negative --last")
	}
	_, err = timeutil.ParseRange("0s", "", "", func() time.Time { return time.Unix(0, 0) })
	if err == nil {
		t.Fatal("expected error for zero --last")
	}
}

func TestResolveConfigRejectsNegativeTimeout(t *testing.T) {
	_, err := ResolveConfig(FlagValues{NoEnvFile: true, Timeout: -1 * time.Second})
	if err == nil {
		t.Fatal("expected negative timeout to fail")
	}
}

func TestResolveConfigRejectsExplicitSymlinkEnvFile(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve test working directory: %v", err)
	}
	root := findProjectTempRoot(workingDirectory)
	if info, err := os.Stat(root); err == nil && !info.IsDir() {
		t.Fatalf("temporary test root is not a directory: %s", root)
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatalf("create temporary test root: %v", err)
	}
	directory, err := os.MkdirTemp(root, "case-")
	if err != nil {
		t.Fatalf("create temporary test directory: %v", err)
	}
	realPath := filepath.Join(directory, "real.env")
	linkPath := filepath.Join(directory, "linked.env")
	if err := os.WriteFile(realPath, []byte("DD_SITE=eu\n"), 0600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Fatalf("create env-file symlink: %v", err)
	}
	_, err = ResolveConfig(FlagValues{EnvFile: linkPath})
	if err == nil || !strings.Contains(err.Error(), "env file must not be a symlink") {
		t.Fatalf("expected explicit env-file symlink rejection, got %v", err)
	}
}

func findProjectTempRoot(start string) string {
	for directory := start; ; directory = filepath.Dir(directory) {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return filepath.Join(directory, ".tmp", "env-safety-tests")
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return filepath.Join(start, ".tmp", "env-safety-tests")
		}
	}
}
