package logger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestNew_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := Config{
		Level:      "debug",
		FilePath:   logPath,
		MaxSizeMB:  1,
		MaxAgeDays: 1,
		MaxBackups: 1,
	}

	log, err := New(cfg)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	defer func() { _ = log.Sync() }()

	// Verify logger writes a message
	log.Info("test message", zap.String("key", "value"))

	// Verify file was created and contains JSON
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("log file is empty")
	}

	// Verify JSON structure
	var entry map[string]interface{}
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("log file contains invalid JSON: %v\ncontent: %s", err, string(data))
	}

	if entry["msg"] != "test message" {
		t.Errorf("expected msg='test message', got %v", entry["msg"])
	}
	if entry["key"] != "value" {
		t.Errorf("expected key='value', got %v", entry["key"])
	}
	if _, ok := entry["ts"]; !ok {
		t.Error("log entry missing 'ts' timestamp field")
	}
}

func TestNew_InvalidLevel_FallsBackToInfo(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := Config{
		Level:      "INVALID",
		FilePath:   logPath,
		MaxSizeMB:  1,
		MaxAgeDays: 1,
		MaxBackups: 1,
	}

	log, err := New(cfg)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	defer func() { _ = log.Sync() }()

	// Debug message at info level should not appear, info should
	log.Debug("should not appear")
	log.Info("should appear")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	content := string(data)
	if strings.Contains(content, "should not appear") {
		t.Error("debug message appeared despite info-level fallback")
	}
	if !strings.Contains(content, "should appear") {
		t.Error("info message missing despite info-level fallback")
	}
}

func TestNew_DebugLevel_LogsDebugMessages(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := Config{
		Level:      "debug",
		FilePath:   logPath,
		MaxSizeMB:  1,
		MaxAgeDays: 1,
		MaxBackups: 1,
	}

	log, err := New(cfg)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	defer func() { _ = log.Sync() }()

	log.Debug("debug msg")
	log.Info("info msg")
	log.Warn("warn msg")
	log.Error("error msg")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	content := string(data)
	for _, expected := range []string{"debug msg", "info msg", "warn msg", "error msg"} {
		if !strings.Contains(content, expected) {
			t.Errorf("expected log to contain %q", expected)
		}
	}
}

func TestNew_ErrorLevel_SuppressesLowerLevels(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := Config{
		Level:      "error",
		FilePath:   logPath,
		MaxSizeMB:  1,
		MaxAgeDays: 1,
		MaxBackups: 1,
	}

	log, err := New(cfg)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	defer func() { _ = log.Sync() }()

	log.Debug("debug msg")
	log.Info("info msg")
	log.Warn("warn msg")
	log.Error("error msg")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	content := string(data)
	if strings.Contains(content, "debug msg") {
		t.Error("debug message should be suppressed at error level")
	}
	if strings.Contains(content, "info msg") {
		t.Error("info message should be suppressed at error level")
	}
	if strings.Contains(content, "warn msg") {
		t.Error("warn message should be suppressed at error level")
	}
	if !strings.Contains(content, "error msg") {
		t.Error("error message should appear at error level")
	}
}

func TestNew_CreatesLogDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "deeply", "nested", "dirs", "test.log")

	cfg := Config{
		Level:      "info",
		FilePath:   nestedPath,
		MaxSizeMB:  1,
		MaxAgeDays: 1,
		MaxBackups: 1,
	}

	log, err := New(cfg)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	defer func() { _ = log.Sync() }()

	log.Info("hello")

	if _, err := os.Stat(nestedPath); os.IsNotExist(err) {
		t.Fatal("log file was not created in nested directory")
	}
}

func TestNew_FilePathEmpty_NoFileOutput(t *testing.T) {
	cfg := Config{
		Level:      "info",
		FilePath:   "",
		MaxSizeMB:  1,
		MaxAgeDays: 1,
		MaxBackups: 1,
	}

	log, err := New(cfg)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	defer func() { _ = log.Sync() }()

	// Should not panic when writing
	log.Info("console only")
}

func TestNewSessionID_IsValidUUID(t *testing.T) {
	id := NewSessionID()
	if id == "" {
		t.Fatal("NewSessionID() returned empty string")
	}

	// UUID v4 format: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		t.Errorf("expected 5 dash-separated parts, got %d in %q", len(parts), id)
	}
	if len(parts[0]) != 8 {
		t.Errorf("expected first part length 8, got %d", len(parts[0]))
	}
}

func TestNewSessionID_IsUnique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := NewSessionID()
		if seen[id] {
			t.Fatalf("duplicate session ID generated: %s", id)
		}
		seen[id] = true
	}
}

func TestWithSession_AddsSessionID(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := Config{
		Level:      "info",
		FilePath:   logPath,
		MaxSizeMB:  1,
		MaxAgeDays: 1,
		MaxBackups: 1,
	}

	log, err := New(cfg)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	defer func() { _ = log.Sync() }()

	sessionID := "abc-123-def"
	sessionLog := WithSession(log, sessionID)
	sessionLog.Info("session message")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var entry map[string]interface{}
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if entry["session_id"] != sessionID {
		t.Errorf("expected session_id=%q, got %v", sessionID, entry["session_id"])
	}
	if entry["msg"] != "session message" {
		t.Errorf("expected msg='session message', got %v", entry["msg"])
	}
}

func TestWithSession_PreservesCaller(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := Config{
		Level:      "info",
		FilePath:   logPath,
		MaxSizeMB:  1,
		MaxAgeDays: 1,
		MaxBackups: 1,
	}

	log, err := New(cfg)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	defer func() { _ = log.Sync() }()

	sessionLog := WithSession(log, "test-sid")
	sessionLog.Info("with caller")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var entry map[string]interface{}
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if _, ok := entry["caller"]; !ok {
		t.Error("log entry missing 'caller' field")
	}
}

func TestNew_LevelCaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := Config{
		Level:      "DEBUG",
		FilePath:   logPath,
		MaxSizeMB:  1,
		MaxAgeDays: 1,
		MaxBackups: 1,
	}

	log, err := New(cfg)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	defer func() { _ = log.Sync() }()

	log.Debug("uppercase debug")
	log.Info("uppercase info")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "uppercase debug") {
		t.Error("DEBUG level should allow debug messages")
	}
}

func TestNew_StacktraceOnError(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	cfg := Config{
		Level:      "debug",
		FilePath:   logPath,
		MaxSizeMB:  1,
		MaxAgeDays: 1,
		MaxBackups: 1,
	}

	log, err := New(cfg)
	if err != nil {
		t.Fatalf("New() unexpected error: %v", err)
	}
	defer func() { _ = log.Sync() }()

	log.Error("error with stack")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "stacktrace") {
		t.Error("error level should include stacktrace")
	}
}

func TestNew_LevelEdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		level         string
		wantLvl       zapcore.Level
		expectInfo    bool
		expectWarning bool
	}{
		{"empty falls back to info", "", zapcore.InfoLevel, true, true},
		{"warn suppresses info", "warn", zapcore.WarnLevel, false, true},
		{"info level", "info", zapcore.InfoLevel, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			logPath := filepath.Join(tmpDir, "test.log")

			cfg := Config{
				Level:      tt.level,
				FilePath:   logPath,
				MaxSizeMB:  1,
				MaxAgeDays: 1,
				MaxBackups: 1,
			}

			log, err := New(cfg)
			if err != nil {
				t.Fatalf("New() unexpected error: %v", err)
			}
			defer func() { _ = log.Sync() }()

			log.Info("info message")
			log.Warn("warn message")

			data, _ := os.ReadFile(logPath)
			content := string(data)

			if tt.expectInfo && !strings.Contains(content, "info message") {
				t.Error("info message should appear")
			}
			if !tt.expectInfo && strings.Contains(content, "info message") {
				t.Error("info message should NOT appear at this level")
			}
			if !strings.Contains(content, "warn message") {
				t.Error("warn message should appear")
			}
		})
	}
}
