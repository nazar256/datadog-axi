package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/nazar256/datadog-axi/internal/output"
)

const (
	DefaultSite    = "datadoghq.com"
	DefaultTimeout = 30 * time.Second
)

type FlagValues struct {
	Site          string
	EnvFile       string
	NoEnvFile     bool
	Timeout       time.Duration
	Output        string
	DefaultOutput string
	JSON          bool
	Full          bool
	Fields        string
}

type Config struct {
	Site        string
	Timeout     time.Duration
	Output      output.Format
	APIKey      string
	AppKey      string
	EnvFileUsed string
	EnvFiles    []string          `json:"env_files,omitempty"`
	Diagnostics []string          `json:"diagnostics,omitempty"`
	Sources     map[string]string `json:"sources,omitempty"`
	Version     string
}

func ResolveConfig(flags FlagValues) (Config, error) {
	dotenv, envFiles, diagnostics, err := readDotEnv(flags)
	if err != nil {
		return Config{}, WrapUsageError(err)
	}

	lookup := func(key string) string {
		for _, alias := range aliasesFor(key) {
			if value, ok := os.LookupEnv(alias); ok {
				return strings.TrimSpace(value)
			}
		}
		if value, ok := dotenv[key]; ok {
			return strings.TrimSpace(value)
		}
		for _, alias := range aliasesFor(key) {
			if value, ok := dotenv[alias]; ok {
				return strings.TrimSpace(value)
			}
		}
		return ""
	}
	source := func(key string) string {
		for _, alias := range aliasesFor(key) {
			if _, ok := os.LookupEnv(alias); ok {
				return "process:" + alias
			}
		}
		if _, ok := dotenv[key]; ok {
			return "env_file:" + key
		}
		for _, alias := range aliasesFor(key) {
			if _, ok := dotenv[alias]; ok {
				return "env_file:" + alias
			}
		}
		return "default"
	}

	siteValue := firstNonEmpty(flags.Site, lookup("DATADOG_SITE"), DefaultSite)
	site, err := normalizeSite(siteValue)
	if err != nil {
		return Config{}, WrapUsageError(err)
	}

	formatName := flags.Output
	if flags.JSON {
		if strings.TrimSpace(formatName) != "" && !strings.EqualFold(formatName, string(output.JSON)) {
			return Config{}, UsageErrorf("--json cannot be combined with --output %s", formatName)
		}
		formatName = string(output.JSON)
	}
	if strings.TrimSpace(formatName) == "" {
		formatName = flags.DefaultOutput
	}
	format, err := output.ParseFormat(formatName)
	if err != nil {
		return Config{}, WrapUsageError(err)
	}

	timeout := flags.Timeout
	if timeout < 0 {
		return Config{}, UsageErrorf("timeout must be greater than or equal to 0")
	}
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	return Config{
		Site:        site,
		Timeout:     timeout,
		Output:      format,
		APIKey:      lookup("DATADOG_API_KEY"),
		AppKey:      lookup("DATADOG_APP_KEY"),
		EnvFileUsed: firstNonEmpty(envFiles...),
		EnvFiles:    envFiles,
		Diagnostics: diagnostics,
		Sources: map[string]string{
			"site":    sourceForFlag(flags.Site, source("DATADOG_SITE")),
			"api_key": source("DATADOG_API_KEY"),
			"app_key": source("DATADOG_APP_KEY"),
		},
	}, nil
}

func sourceForFlag(flag, fallback string) string {
	if strings.TrimSpace(flag) != "" {
		return "flag"
	}
	return fallback
}

func (c Config) HasAuth() bool {
	return c.APIKey != "" && c.AppKey != ""
}

func (c Config) RequireAuth() error {
	if c.APIKey == "" {
		return fmt.Errorf("missing DATADOG_API_KEY (or DD_API_KEY)")
	}
	if c.AppKey == "" {
		return fmt.Errorf("missing DATADOG_APP_KEY (or DD_APP_KEY)")
	}
	return nil
}

func readDotEnv(flags FlagValues) (map[string]string, []string, []string, error) {
	if flags.NoEnvFile && strings.TrimSpace(flags.EnvFile) != "" {
		return nil, nil, nil, UsageErrorf("--env-file and --no-env-file cannot be used together")
	}
	if flags.NoEnvFile {
		return map[string]string{}, nil, []string{"env files disabled by --no-env-file"}, nil
	}

	path := strings.TrimSpace(flags.EnvFile)
	if path != "" {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("resolve env file path: %w", err)
		}
		values, err := readEnvPath(absPath)
		if err != nil {
			return nil, nil, nil, err
		}
		return canonicalizeEnv(values), []string{absPath}, nil, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve working directory: %w", err)
	}
	paths := []string{}
	if configDir, e := os.UserConfigDir(); e == nil {
		paths = append(paths, filepath.Join(configDir, "datadog-axi", ".env"))
	}
	if repoRoot := findRepositoryRoot(wd); repoRoot != "" && repoRoot != wd {
		paths = append(paths, filepath.Join(repoRoot, ".env"))
	}
	paths = append(paths, filepath.Join(wd, ".env"))
	values := map[string]string{}
	used := []string{}
	diagnostics := []string{}
	seen := map[string]struct{}{}
	for _, candidate := range paths {
		abs, e := filepath.Abs(candidate)
		if e != nil {
			continue
		}
		resolved := abs
		if real, e := filepath.EvalSymlinks(abs); e == nil {
			resolved = real
		}
		if _, ok := seen[resolved]; ok {
			diagnostics = append(diagnostics, "skipped duplicate env path "+abs)
			continue
		}
		seen[resolved] = struct{}{}
		if _, e := os.Stat(abs); e != nil {
			if os.IsNotExist(e) {
				continue
			}
			return nil, nil, nil, fmt.Errorf("read env file: %w", e)
		}
		if info, e := os.Lstat(abs); e == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return nil, nil, nil, fmt.Errorf("default env file must not be a symlink: %s", abs)
			}
			if !info.Mode().IsRegular() {
				return nil, nil, nil, fmt.Errorf("default env file must be a regular file: %s", abs)
			}
		}
		fileValues, e := godotenv.Read(abs)
		if e != nil {
			return nil, nil, nil, fmt.Errorf("parse env file: %w", e)
		}
		for k, v := range canonicalizeEnv(fileValues) {
			values[k] = v
		}
		used = append(used, abs)
	}
	return values, used, diagnostics, nil
}

func findRepositoryRoot(start string) string {
	for dir := start; ; dir = filepath.Dir(dir) {
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && (info.IsDir() || info.Mode().IsRegular()) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
	}
}

func readEnvPath(path string) (map[string]string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("read env file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("env file must not be a symlink: %s", path)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("env file must be a regular file: %s", path)
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("read env file: %w", err)
	}
	values, err := godotenv.Read(path)
	if err != nil {
		return nil, fmt.Errorf("parse env file: %w", err)
	}
	return canonicalizeEnv(values), nil
}

func canonicalizeEnv(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	for _, logical := range []string{"DATADOG_API_KEY", "DATADOG_APP_KEY", "DATADOG_SITE"} {
		aliases := aliasesFor(logical)
		if preferred, ok := values[aliases[0]]; ok {
			result[logical] = preferred
		} else if legacy, ok := values[aliases[1]]; ok {
			result[logical] = legacy
		}
		delete(result, aliases[0])
		if aliases[1] != logical {
			delete(result, aliases[1])
		}
	}
	return result
}

func aliasesFor(key string) []string {
	switch key {
	case "DATADOG_API_KEY":
		return []string{"DD_API_KEY", "DATADOG_API_KEY"}
	case "DATADOG_APP_KEY":
		return []string{"DD_APP_KEY", "DATADOG_APP_KEY"}
	case "DATADOG_SITE":
		return []string{"DD_SITE", "DATADOG_SITE"}
	}
	return []string{key}
}

func normalizeSite(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	aliases := map[string]string{
		"us1":     "datadoghq.com",
		"us3":     "us3.datadoghq.com",
		"us5":     "us5.datadoghq.com",
		"eu":      "datadoghq.eu",
		"ap1":     "ap1.datadoghq.com",
		"ap2":     "ap2.datadoghq.com",
		"us1-fed": "ddog-gov.com",
	}
	if value == "" {
		return "", fmt.Errorf("site cannot be empty")
	}
	if normalized, ok := aliases[value]; ok {
		return normalized, nil
	}
	allowed := map[string]struct{}{
		"datadoghq.com":     {},
		"us3.datadoghq.com": {},
		"us5.datadoghq.com": {},
		"datadoghq.eu":      {},
		"ap1.datadoghq.com": {},
		"ap2.datadoghq.com": {},
		"ddog-gov.com":      {},
	}
	if _, ok := allowed[value]; ok {
		return value, nil
	}
	return "", fmt.Errorf("unsupported Datadog site %q", raw)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
