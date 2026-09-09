// Package config provides application configuration loaded from environment variables.
// It validates required fields at startup and applies defaults for optional fields.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all application configuration values.
type Config struct {
	// Logging
	LogLevel      string
	LogMaxSizeMB  int
	LogMaxAgeDays int
	LogMaxBackups int

	// General
	OutputDir string
	Debug     bool
	BotName   string

	// PostgreSQL
	PostgresDSN         string
	PostgresMaxPoolSize int
	PostgresMinPoolSize int

	// Post lookup (tbl_posts query run once at startup)
	OrgID        int
	PubTimeAfter int64

	// GPM (GoLogin Profile Manager)
	GPMAPI     string
	ProfileIDs []string
	UseGPM     bool

	// PostURLs is the list of TikTok video URLs to visit, fetched from
	// tbl_posts at startup (see ORG_ID/PUB_TIME above). Not env-driven — set
	// programmatically by main() before the crawler runs.
	PostURLs []string

	// Crawl timing (seconds): pacing between URL visits and between batches.
	SleepMinKeyword int
	SleepMaxKeyword int
	RestMinSession  int
	RestMaxSession  int

	// Batch limits: URLs visited per browser session before reconnecting.
	BatchMin int
	BatchMax int

	// Pagination guards
	MaxPagesPerSession int
}

// Load reads configuration from environment variables (with .env file support),
// validates required fields, applies defaults for optional fields, and returns
// the parsed Config along with any validation errors.
//
// Required fields (load fails if any are missing or empty):
//   - POSTGRES_DSN
//   - BOT_NAME
//   - GPM_API
//   - ORG_ID
//   - PUB_TIME
//
// Optional fields have sensible defaults when not set.
func Load() (Config, error) {
	_ = godotenv.Load()

	var errs []string
	cfg := Config{}

	loadGeneralSettings(&cfg, &errs)
	loadLogSettings(&cfg, &errs)
	loadPostgresSettings(&cfg, &errs)
	loadPostLookupSettings(&cfg, &errs)
	loadGpmSettings(&cfg, &errs)
	loadCrawlSettings(&cfg, &errs)
	validateBounds(&cfg, &errs)

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("config validation errors:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return cfg, nil
}

// --- Environment variable helpers -----------------------------------------------

// requireStr reads a required string from the environment. If the key is missing
// or the value is empty, it appends an error to errs and returns "".
func requireStr(errs *[]string, key string) string {
	v, ok := os.LookupEnv(key)
	if !ok {
		*errs = append(*errs, fmt.Sprintf("%s is required", key))
		return ""
	}
	v = strings.TrimSpace(v)
	if v == "" {
		*errs = append(*errs, fmt.Sprintf("%s is required (must not be empty)", key))
		return ""
	}
	return v
}

// requireInt reads a required integer from the environment. If the key is
// missing, empty, or not a valid integer, it appends an error to errs and
// returns 0.
func requireInt(errs *[]string, key string) int {
	v := requireStr(errs, key)
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		*errs = append(*errs, fmt.Sprintf("%s: invalid integer value %q", key, v))
		return 0
	}
	return n
}

// requireInt64 reads a required 64-bit integer from the environment. If the
// key is missing, empty, or not a valid integer, it appends an error to errs
// and returns 0.
func requireInt64(errs *[]string, key string) int64 {
	v := requireStr(errs, key)
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		*errs = append(*errs, fmt.Sprintf("%s: invalid integer value %q", key, v))
		return 0
	}
	return n
}

// optStr reads an optional string from the environment, returning fallback when
// the key is missing or the value is empty.
func optStr(errs *[]string, key, fallback string) string {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}

// optBool reads an optional boolean from the environment. On parse failure it
// appends an error and returns fallback.
func optBool(errs *[]string, key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		*errs = append(*errs, fmt.Sprintf("%s: invalid boolean value %q", key, v))
		return fallback
	}
	return b
}

// optInt reads an optional integer from the environment. On parse failure it
// appends an error and returns fallback.
func optInt(errs *[]string, key string, fallback int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		*errs = append(*errs, fmt.Sprintf("%s: invalid integer value %q", key, v))
		return fallback
	}
	return n
}

// --- Settings loaders -----------------------------------------------------------

// loadGeneralSettings populates general configuration fields.
func loadGeneralSettings(cfg *Config, errs *[]string) {
	cfg.BotName = requireStr(errs, "BOT_NAME")
	cfg.OutputDir = optStr(errs, "OUTPUT_DIR", "./data")
	cfg.Debug = optBool(errs, "DEBUG", false)
}

// loadLogSettings populates logging configuration fields.
func loadLogSettings(cfg *Config, errs *[]string) {
	cfg.LogLevel = optStr(errs, "LOG_LEVEL", "info")
	cfg.LogMaxSizeMB = optInt(errs, "LOG_MAX_SIZE_MB", 100)
	cfg.LogMaxAgeDays = optInt(errs, "LOG_MAX_AGE_DAYS", 7)
	cfg.LogMaxBackups = optInt(errs, "LOG_MAX_BACKUPS", 7)
}

// loadPostgresSettings populates PostgreSQL configuration fields.
func loadPostgresSettings(cfg *Config, errs *[]string) {
	cfg.PostgresDSN = requireStr(errs, "POSTGRES_DSN")
	cfg.PostgresMaxPoolSize = optInt(errs, "POSTGRES_MAX_POOL_SIZE", 100)
	cfg.PostgresMinPoolSize = optInt(errs, "POSTGRES_MIN_POOL_SIZE", 10)
}

// loadPostLookupSettings populates the org/pub_time filter used for the
// startup tbl_posts lookup query.
func loadPostLookupSettings(cfg *Config, errs *[]string) {
	cfg.OrgID = requireInt(errs, "ORG_ID")
	cfg.PubTimeAfter = requireInt64(errs, "PUB_TIME")
}

// loadGpmSettings populates GPM (GoLogin Profile Manager) configuration fields.
func loadGpmSettings(cfg *Config, errs *[]string) {
	cfg.GPMAPI = requireStr(errs, "GPM_API")

	profileIDsStr := optStr(errs, "PROFILE_IDS", "")
	if profileIDsStr != "" {
		cfg.ProfileIDs = splitComma(profileIDsStr)
	}

	cfg.UseGPM = cfg.GPMAPI != "" && len(cfg.ProfileIDs) > 0
}

// loadCrawlSettings populates crawl timing and batch limits.
func loadCrawlSettings(cfg *Config, errs *[]string) {
	cfg.SleepMinKeyword = optInt(errs, "SLEEP_MIN_KEYWORD", 60)
	cfg.SleepMaxKeyword = optInt(errs, "SLEEP_MAX_KEYWORD", 180)
	cfg.RestMinSession = optInt(errs, "REST_MIN_SESSION", 180)
	cfg.RestMaxSession = optInt(errs, "REST_MAX_SESSION", 300)

	cfg.BatchMin = optInt(errs, "BATCH_MIN", 3)
	cfg.BatchMax = optInt(errs, "BATCH_MAX", 5)

	cfg.MaxPagesPerSession = optInt(errs, "MAX_PAGES_PER_SESSION", 20)
}

// validateBounds checks that paired min/max settings are consistent.
func validateBounds(cfg *Config, errs *[]string) {
	if cfg.PostgresMinPoolSize > cfg.PostgresMaxPoolSize {
		*errs = append(*errs, "POSTGRES_MIN_POOL_SIZE must not exceed POSTGRES_MAX_POOL_SIZE")
	}
	if cfg.BatchMin > cfg.BatchMax {
		*errs = append(*errs, "BATCH_MIN must not exceed BATCH_MAX")
	}
	if cfg.SleepMinKeyword > cfg.SleepMaxKeyword {
		*errs = append(*errs, "SLEEP_MIN_KEYWORD must not exceed SLEEP_MAX_KEYWORD")
	}
	if cfg.RestMinSession > cfg.RestMaxSession {
		*errs = append(*errs, "REST_MIN_SESSION must not exceed REST_MAX_SESSION")
	}
}

// --- Internal helpers ----------------------------------------------------------

// parseIntSliceFromEnv parses a comma-separated list of integers from an
// environment variable. Returns an empty slice and no error when the variable
// is not set. Returns an error when the variable is set but contains invalid
// values.
func parseIntSliceFromEnv(key string) ([]int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return nil, nil
	}

	parts := strings.Split(v, ",")
	var result []int
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid integer at position %d: %q", key, i+1, strings.TrimSpace(parts[i]))
		}
		if n <= 0 {
			return nil, fmt.Errorf("%s: value must be positive at position %d: %d", key, i+1, n)
		}
		result = append(result, n)
	}

	return result, nil
}

// splitComma splits a comma-separated string, trimming whitespace and filtering
// out empty parts.
func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
