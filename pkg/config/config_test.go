package config

import (
	"os"
	"strings"
	"testing"
)

// setEnv sets an environment variable and returns a cleanup function to restore
// the original state.
func setEnv(t *testing.T, key, value string) func() {
	t.Helper()
	prev, existed := os.LookupEnv(key)
	_ = os.Setenv(key, value)
	return func() {
		if existed {
			_ = os.Setenv(key, prev)
		} else {
			_ = os.Unsetenv(key)
		}
	}
}

// unsetEnv unsets an environment variable and returns a cleanup function.
func unsetEnv(t *testing.T, key string) func() {
	t.Helper()
	prev, existed := os.LookupEnv(key)
	_ = os.Unsetenv(key)
	return func() {
		if existed {
			_ = os.Setenv(key, prev)
		}
	}
}

func TestLoad_AllRequiredFields(t *testing.T) {
	cleanups := []func(){
		setEnv(t, "POSTGRES_DSN", "postgres://localhost:5432/testdb?sslmode=disable"),
		setEnv(t, "BOT_NAME", "testbot"),
		setEnv(t, "GPM_API", "https://gpm.example.com"),
		setEnv(t, "ORG_ID", "987700"),
		setEnv(t, "PUB_TIME", "1785517201"),
	}
	defer func() {
		for _, fn := range cleanups {
			fn()
		}
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.PostgresDSN != "postgres://localhost:5432/testdb?sslmode=disable" {
		t.Errorf("POSTGRES_DSN = %q, want %q", cfg.PostgresDSN, "postgres://localhost:5432/testdb?sslmode=disable")
	}
	if cfg.BotName != "testbot" {
		t.Errorf("BOT_NAME = %q, want %q", cfg.BotName, "testbot")
	}
	if cfg.GPMAPI != "https://gpm.example.com" {
		t.Errorf("GPM_API = %q, want %q", cfg.GPMAPI, "https://gpm.example.com")
	}
	if cfg.OrgID != 987700 {
		t.Errorf("ORG_ID = %d, want %d", cfg.OrgID, 987700)
	}
	if cfg.PubTimeAfter != 1785517201 {
		t.Errorf("PUB_TIME = %d, want %d", cfg.PubTimeAfter, 1785517201)
	}
}

func TestLoad_MissingRequiredField(t *testing.T) {
	tests := []struct {
		name    string
		missing string
		wantMsg string
	}{
		{"missing POSTGRES_DSN", "POSTGRES_DSN", "POSTGRES_DSN is required"},
		{"missing BOT_NAME", "BOT_NAME", "BOT_NAME is required"},
		{"missing GPM_API", "GPM_API", "GPM_API is required"},
		{"missing ORG_ID", "ORG_ID", "ORG_ID is required"},
		{"missing PUB_TIME", "PUB_TIME", "PUB_TIME is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set all required fields
			cleanups := []func(){
				setEnv(t, "POSTGRES_DSN", "postgres://localhost:5432/testdb?sslmode=disable"),
				setEnv(t, "BOT_NAME", "testbot"),
				setEnv(t, "GPM_API", "https://gpm.example.com"),
				setEnv(t, "ORG_ID", "987700"),
				setEnv(t, "PUB_TIME", "1785517201"),
			}
			defer func() {
				for _, fn := range cleanups {
					fn()
				}
			}()

			// Unset the one we want to test
			restoreMissing := unsetEnv(t, tt.missing)
			defer restoreMissing()

			_, err := Load()
			if err == nil {
				t.Fatalf("expected error for missing %s, got nil", tt.missing)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestLoad_RequiredFieldEmpty(t *testing.T) {
	cleanups := []func(){
		setEnv(t, "POSTGRES_DSN", "postgres://localhost:5432/testdb?sslmode=disable"),
		setEnv(t, "BOT_NAME", ""),
		setEnv(t, "GPM_API", "https://gpm.example.com"),
	}
	defer func() {
		for _, fn := range cleanups {
			fn()
		}
	}()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for empty BOT_NAME")
	}
	if !strings.Contains(err.Error(), "BOT_NAME is required (must not be empty)") {
		t.Errorf("error %q does not contain expected message", err.Error())
	}
}

func TestLoad_Defaults(t *testing.T) {
	cleanups := []func(){
		setEnv(t, "POSTGRES_DSN", "postgres://localhost:5432/testdb?sslmode=disable"),
		setEnv(t, "BOT_NAME", "testbot"),
		setEnv(t, "GPM_API", "https://gpm.example.com"),
		setEnv(t, "ORG_ID", "987700"),
		setEnv(t, "PUB_TIME", "1785517201"),
	}
	defer func() {
		for _, fn := range cleanups {
			fn()
		}
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		field string
		got   any
		want  any
	}{
		{"LogLevel", cfg.LogLevel, "info"},
		{"LogMaxSizeMB", cfg.LogMaxSizeMB, 100},
		{"LogMaxAgeDays", cfg.LogMaxAgeDays, 7},
		{"LogMaxBackups", cfg.LogMaxBackups, 7},
		{"OutputDir", cfg.OutputDir, "./data"},
		{"Debug", cfg.Debug, false},
		{"PostgresMaxPoolSize", cfg.PostgresMaxPoolSize, 100},
		{"PostgresMinPoolSize", cfg.PostgresMinPoolSize, 10},
		{"HTTPTimeoutSeconds", cfg.HTTPTimeoutSeconds, 30},
		{"SleepMinKeyword", cfg.SleepMinKeyword, 60},
		{"SleepMaxKeyword", cfg.SleepMaxKeyword, 180},
		{"RestMinSession", cfg.RestMinSession, 180},
		{"RestMaxSession", cfg.RestMaxSession, 300},
		{"BatchMin", cfg.BatchMin, 3},
		{"BatchMax", cfg.BatchMax, 5},
		{"MaxPagesPerSession", cfg.MaxPagesPerSession, 20},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s = %v, want %v", tt.field, tt.got, tt.want)
			}
		})
	}

	if len(cfg.PostURLs) != 0 {
		t.Errorf("PostURLs default = %v, want empty slice", cfg.PostURLs)
	}
	if cfg.UseGPM {
		t.Errorf("UseGPM default = true, want false (no PROFILE_IDS set)")
	}
}

func TestLoad_CustomValues(t *testing.T) {
	cleanups := []func(){
		setEnv(t, "POSTGRES_DSN", "postgres://custom:5432/customdb?sslmode=disable"),
		setEnv(t, "BOT_NAME", "custombot"),
		setEnv(t, "GPM_API", "https://custom.gpm.example.com"),
		setEnv(t, "ORG_ID", "123456"),
		setEnv(t, "PUB_TIME", "1700000000"),
		setEnv(t, "LOG_LEVEL", "debug"),
		setEnv(t, "DEBUG", "true"),
		setEnv(t, "OUTPUT_DIR", "/custom/output"),
		setEnv(t, "API_URL", "https://api.example.com"),
		setEnv(t, "PROFILE_IDS", "profile1,profile2"),
		setEnv(t, "POSTGRES_MAX_POOL_SIZE", "50"),
		setEnv(t, "POSTGRES_MIN_POOL_SIZE", "5"),
		setEnv(t, "HTTP_TIMEOUT_SECONDS", "45"),
		setEnv(t, "SLEEP_MIN_KEYWORD", "500"),
		setEnv(t, "SLEEP_MAX_KEYWORD", "2000"),
		setEnv(t, "REST_MIN_SESSION", "10000"),
		setEnv(t, "REST_MAX_SESSION", "20000"),
		setEnv(t, "BATCH_MIN", "2"),
		setEnv(t, "BATCH_MAX", "8"),
		setEnv(t, "MAX_PAGES_PER_SESSION", "50"),
		setEnv(t, "LOG_MAX_SIZE_MB", "200"),
		setEnv(t, "LOG_MAX_AGE_DAYS", "14"),
		setEnv(t, "LOG_MAX_BACKUPS", "10"),
	}
	defer func() {
		for _, fn := range cleanups {
			fn()
		}
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	checks := []struct {
		field string
		got   any
		want  any
	}{
		{"PostgresDSN", cfg.PostgresDSN, "postgres://custom:5432/customdb?sslmode=disable"},
		{"BotName", cfg.BotName, "custombot"},
		{"GPMAPI", cfg.GPMAPI, "https://custom.gpm.example.com"},
		{"OrgID", cfg.OrgID, 123456},
		{"PubTimeAfter", cfg.PubTimeAfter, int64(1700000000)},
		{"LogLevel", cfg.LogLevel, "debug"},
		{"Debug", cfg.Debug, true},
		{"OutputDir", cfg.OutputDir, "/custom/output"},
		{"APIURL", cfg.APIURL, "https://api.example.com"},
		{"PostgresMaxPoolSize", cfg.PostgresMaxPoolSize, 50},
		{"PostgresMinPoolSize", cfg.PostgresMinPoolSize, 5},
		{"HTTPTimeoutSeconds", cfg.HTTPTimeoutSeconds, 45},
		{"SleepMinKeyword", cfg.SleepMinKeyword, 500},
		{"SleepMaxKeyword", cfg.SleepMaxKeyword, 2000},
		{"RestMinSession", cfg.RestMinSession, 10000},
		{"RestMaxSession", cfg.RestMaxSession, 20000},
		{"BatchMin", cfg.BatchMin, 2},
		{"BatchMax", cfg.BatchMax, 8},
		{"MaxPagesPerSession", cfg.MaxPagesPerSession, 50},
		{"LogMaxSizeMB", cfg.LogMaxSizeMB, 200},
		{"LogMaxAgeDays", cfg.LogMaxAgeDays, 14},
		{"LogMaxBackups", cfg.LogMaxBackups, 10},
	}

	for _, c := range checks {
		t.Run(c.field, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("%s = %v, want %v", c.field, c.got, c.want)
			}
		})
	}

	if len(cfg.ProfileIDs) != 2 || cfg.ProfileIDs[0] != "profile1" || cfg.ProfileIDs[1] != "profile2" {
		t.Errorf("ProfileIDs = %v, want [profile1 profile2]", cfg.ProfileIDs)
	}
	if !cfg.UseGPM {
		t.Error("UseGPM should be true when GPM_API and PROFILE_IDS are set")
	}
}

func TestLoad_InvalidBool(t *testing.T) {
	cleanups := []func(){
		setEnv(t, "POSTGRES_DSN", "postgres://localhost:5432/testdb?sslmode=disable"),
		setEnv(t, "BOT_NAME", "testbot"),
		setEnv(t, "GPM_API", "https://gpm.example.com"),
		setEnv(t, "ORG_ID", "987700"),
		setEnv(t, "PUB_TIME", "1785517201"),
		setEnv(t, "DEBUG", "invalid"),
	}
	defer func() {
		for _, fn := range cleanups {
			fn()
		}
	}()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid DEBUG value")
	}
	if !strings.Contains(err.Error(), "DEBUG") {
		t.Errorf("error %q should mention DEBUG", err.Error())
	}
}

func TestLoad_InvalidRequiredInt(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantMsg string
	}{
		{"invalid ORG_ID", "ORG_ID", "not-an-int", "ORG_ID"},
		{"invalid PUB_TIME", "PUB_TIME", "not-an-int", "PUB_TIME"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanups := []func(){
				setEnv(t, "POSTGRES_DSN", "postgres://localhost:5432/testdb?sslmode=disable"),
				setEnv(t, "BOT_NAME", "testbot"),
				setEnv(t, "GPM_API", "https://gpm.example.com"),
				setEnv(t, "ORG_ID", "987700"),
				setEnv(t, "PUB_TIME", "1785517201"),
				setEnv(t, tt.key, tt.value),
			}
			defer func() {
				for _, fn := range cleanups {
					fn()
				}
			}()

			_, err := Load()
			if err == nil {
				t.Fatalf("expected error for invalid %s", tt.key)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q should mention %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestLoad_InvalidInt(t *testing.T) {
	cleanups := []func(){
		setEnv(t, "POSTGRES_DSN", "postgres://localhost:5432/testdb?sslmode=disable"),
		setEnv(t, "BOT_NAME", "testbot"),
		setEnv(t, "GPM_API", "https://gpm.example.com"),
		setEnv(t, "ORG_ID", "987700"),
		setEnv(t, "PUB_TIME", "1785517201"),
		setEnv(t, "POSTGRES_MAX_POOL_SIZE", "not-an-int"),
	}
	defer func() {
		for _, fn := range cleanups {
			fn()
		}
	}()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid POSTGRES_MAX_POOL_SIZE")
	}
	if !strings.Contains(err.Error(), "POSTGRES_MAX_POOL_SIZE") {
		t.Errorf("error %q should mention POSTGRES_MAX_POOL_SIZE", err.Error())
	}
}

func TestLoad_BoundsCheckMinGreaterThanMax(t *testing.T) {
	tests := []struct {
		name    string
		minKey  string
		maxKey  string
		minVal  string
		maxVal  string
		wantMsg string
	}{
		{
			name:    "postgres pool min > max",
			minKey:  "POSTGRES_MIN_POOL_SIZE",
			maxKey:  "POSTGRES_MAX_POOL_SIZE",
			minVal:  "200",
			maxVal:  "100",
			wantMsg: "POSTGRES_MIN_POOL_SIZE must not exceed POSTGRES_MAX_POOL_SIZE",
		},
		{
			name:    "batch min > max",
			minKey:  "BATCH_MIN",
			maxKey:  "BATCH_MAX",
			minVal:  "10",
			maxVal:  "5",
			wantMsg: "BATCH_MIN must not exceed BATCH_MAX",
		},
		{
			name:    "sleep min > max",
			minKey:  "SLEEP_MIN_KEYWORD",
			maxKey:  "SLEEP_MAX_KEYWORD",
			minVal:  "10000",
			maxVal:  "5000",
			wantMsg: "SLEEP_MIN_KEYWORD must not exceed SLEEP_MAX_KEYWORD",
		},
		{
			name:    "rest min > max",
			minKey:  "REST_MIN_SESSION",
			maxKey:  "REST_MAX_SESSION",
			minVal:  "120000",
			maxVal:  "60000",
			wantMsg: "REST_MIN_SESSION must not exceed REST_MAX_SESSION",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanups := []func(){
				setEnv(t, "POSTGRES_DSN", "postgres://localhost:5432/testdb?sslmode=disable"),
				setEnv(t, "BOT_NAME", "testbot"),
				setEnv(t, "GPM_API", "https://gpm.example.com"),
				setEnv(t, "ORG_ID", "987700"),
				setEnv(t, "PUB_TIME", "1785517201"),
				setEnv(t, tt.minKey, tt.minVal),
				setEnv(t, tt.maxKey, tt.maxVal),
			}
			defer func() {
				for _, fn := range cleanups {
					fn()
				}
			}()

			_, err := Load()
			if err == nil {
				t.Fatal("expected bounds error")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

func TestLoad_EmptyOptionalStringUsesDefault(t *testing.T) {
	cleanups := []func(){
		setEnv(t, "POSTGRES_DSN", "postgres://localhost:5432/testdb?sslmode=disable"),
		setEnv(t, "BOT_NAME", "testbot"),
		setEnv(t, "GPM_API", "https://gpm.example.com"),
		setEnv(t, "ORG_ID", "987700"),
		setEnv(t, "PUB_TIME", "1785517201"),
		setEnv(t, "LOG_LEVEL", ""),
	}
	defer func() {
		for _, fn := range cleanups {
			fn()
		}
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want default %q", cfg.LogLevel, "info")
	}
}

func TestLoad_UseGPMNoProfileIDs(t *testing.T) {
	cleanups := []func(){
		setEnv(t, "POSTGRES_DSN", "postgres://localhost:5432/testdb?sslmode=disable"),
		setEnv(t, "BOT_NAME", "testbot"),
		setEnv(t, "GPM_API", "https://gpm.example.com"),
		setEnv(t, "ORG_ID", "987700"),
		setEnv(t, "PUB_TIME", "1785517201"),
		// No PROFILE_IDS
	}
	defer func() {
		for _, fn := range cleanups {
			fn()
		}
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.UseGPM {
		t.Error("UseGPM should be false when PROFILE_IDS is not set")
	}
}

func TestLoad_MultipleErrors(t *testing.T) {
	// Unset all required fields — should get multiple errors
	cleanups := []func(){
		unsetEnv(t, "POSTGRES_DSN"),
		unsetEnv(t, "BOT_NAME"),
		unsetEnv(t, "GPM_API"),
		unsetEnv(t, "ORG_ID"),
		unsetEnv(t, "PUB_TIME"),
	}
	defer func() {
		for _, fn := range cleanups {
			fn()
		}
	}()

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for missing required fields")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "POSTGRES_DSN") {
		t.Error("error should mention POSTGRES_DSN")
	}
	if !strings.Contains(errMsg, "BOT_NAME") {
		t.Error("error should mention BOT_NAME")
	}
	if !strings.Contains(errMsg, "GPM_API") {
		t.Error("error should mention GPM_API")
	}
	if !strings.Contains(errMsg, "ORG_ID") {
		t.Error("error should mention ORG_ID")
	}
	if !strings.Contains(errMsg, "PUB_TIME") {
		t.Error("error should mention PUB_TIME")
	}
}

func TestSplitComma(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{"a, ,c", []string{"a", "c"}},
		{"  a  ,  b  ", []string{"a", "b"}},
		{",", nil},
		{",,", nil},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitComma(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("splitComma(%q) len = %d, want %d", tt.input, len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitComma(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseIntSliceFromEnv(t *testing.T) {
	t.Run("not set", func(t *testing.T) {
		_ = os.Unsetenv("TEST_INT_SLICE")
		got, err := parseIntSliceFromEnv("TEST_INT_SLICE")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected empty slice, got %v", got)
		}
	})

	t.Run("empty string", func(t *testing.T) {
		_ = os.Setenv("TEST_INT_SLICE", "")
		defer func() { _ = os.Unsetenv("TEST_INT_SLICE") }()
		got, err := parseIntSliceFromEnv("TEST_INT_SLICE")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected empty slice, got %v", got)
		}
	})

	t.Run("valid", func(t *testing.T) {
		_ = os.Setenv("TEST_INT_SLICE", "1,2,3")
		defer func() { _ = os.Unsetenv("TEST_INT_SLICE") }()
		got, err := parseIntSliceFromEnv("TEST_INT_SLICE")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
			t.Errorf("got %v, want [1 2 3]", got)
		}
	})

	t.Run("with spaces", func(t *testing.T) {
		_ = os.Setenv("TEST_INT_SLICE", " 10 , 20 , 30 ")
		defer func() { _ = os.Unsetenv("TEST_INT_SLICE") }()
		got, err := parseIntSliceFromEnv("TEST_INT_SLICE")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(got) != 3 || got[0] != 10 || got[1] != 20 || got[2] != 30 {
			t.Errorf("got %v, want [10 20 30]", got)
		}
	})

	t.Run("invalid character", func(t *testing.T) {
		_ = os.Setenv("TEST_INT_SLICE", "1,abc,3")
		defer func() { _ = os.Unsetenv("TEST_INT_SLICE") }()
		_, err := parseIntSliceFromEnv("TEST_INT_SLICE")
		if err == nil {
			t.Fatal("expected error for invalid int")
		}
	})

	t.Run("negative", func(t *testing.T) {
		_ = os.Setenv("TEST_INT_SLICE", "1,-1,3")
		defer func() { _ = os.Unsetenv("TEST_INT_SLICE") }()
		_, err := parseIntSliceFromEnv("TEST_INT_SLICE")
		if err == nil {
			t.Fatal("expected error for negative value")
		}
	})

	t.Run("zero value", func(t *testing.T) {
		_ = os.Setenv("TEST_INT_SLICE", "0")
		defer func() { _ = os.Unsetenv("TEST_INT_SLICE") }()
		_, err := parseIntSliceFromEnv("TEST_INT_SLICE")
		if err == nil {
			t.Fatal("expected error for zero value")
		}
	})
}
