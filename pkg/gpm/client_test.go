package gpm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewClient(t *testing.T) {
	log := zap.NewNop()
	client := NewClient("http://localhost:8080", log)
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	if client.apiURL != "http://localhost:8080" {
		t.Errorf("apiURL = %q, want %q", client.apiURL, "http://localhost:8080")
	}
	if client.client.Timeout != 30*time.Second {
		t.Errorf("timeout = %v, want %v", client.client.Timeout, 30*time.Second)
	}
}

func TestGetWebSocketURL_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"webSocketDebuggerUrl": "ws://127.0.0.1:9222/devtools/browser/abc123"}`))
	}))
	defer server.Close()

	// Extract host:port from test server
	debugAddr := server.Listener.Addr().String()

	client := &Client{
		apiURL: "http://localhost:8080",
		log:    zap.NewNop(),
		client: &http.Client{Timeout: 10 * time.Second},
	}

	ctx := context.Background()
	got, err := client.getWebSocketURL(ctx, debugAddr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "ws://127.0.0.1:9222/devtools/browser/abc123"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestGetWebSocketURL_WSS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"webSocketDebuggerUrl": "wss://secure.example.com/devtools/browser/xyz"}`))
	}))
	defer server.Close()

	debugAddr := server.Listener.Addr().String()

	client := &Client{
		apiURL: "http://localhost:8080",
		log:    zap.NewNop(),
		client: &http.Client{Timeout: 10 * time.Second},
	}

	ctx := context.Background()
	got, err := client.getWebSocketURL(ctx, debugAddr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "wss://secure.example.com/devtools/browser/xyz" {
		t.Errorf("got %q, want wss URL", got)
	}
}

func TestGetWebSocketURL_EmptyURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"webSocketDebuggerUrl": ""}`))
	}))
	defer server.Close()

	debugAddr := server.Listener.Addr().String()

	client := &Client{
		apiURL: "http://localhost:8080",
		log:    zap.NewNop(),
		client: &http.Client{Timeout: 10 * time.Second},
	}

	ctx := context.Background()
	_, err := client.getWebSocketURL(ctx, debugAddr)
	if err == nil {
		t.Fatal("expected error for empty URL, got nil")
	}
}

func TestGetWebSocketURL_MissingField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"otherField": "value"}`))
	}))
	defer server.Close()

	debugAddr := server.Listener.Addr().String()

	client := &Client{
		apiURL: "http://localhost:8080",
		log:    zap.NewNop(),
		client: &http.Client{Timeout: 10 * time.Second},
	}

	ctx := context.Background()
	_, err := client.getWebSocketURL(ctx, debugAddr)
	if err == nil {
		t.Fatal("expected error for missing field, got nil")
	}
}

func TestGetWebSocketURL_InvalidFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"webSocketDebuggerUrl": "http://invalid.example.com"}`))
	}))
	defer server.Close()

	debugAddr := server.Listener.Addr().String()

	client := &Client{
		apiURL: "http://localhost:8080",
		log:    zap.NewNop(),
		client: &http.Client{Timeout: 10 * time.Second},
	}

	ctx := context.Background()
	_, err := client.getWebSocketURL(ctx, debugAddr)
	if err == nil {
		t.Fatal("expected error for invalid URL format, got nil")
	}
}

func TestGetWebSocketURL_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return empty so it retries
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	debugAddr := server.Listener.Addr().String()

	client := &Client{
		apiURL: "http://localhost:8080",
		log:    zap.NewNop(),
		client: &http.Client{Timeout: 10 * time.Second},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.getWebSocketURL(ctx, debugAddr)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestStartProfile_Success_WSEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"ws_endpoint":"ws://127.0.0.1:9222/devtools/browser/abc"}}`))
	}))
	defer server.Close()

	// Extract profile ID from URL path
	client := NewClient(server.URL, zap.NewNop())
	ctx := context.Background()

	got, err := client.StartProfile(ctx, "test-profile-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ws://127.0.0.1:9222/devtools/browser/abc" {
		t.Errorf("got %q, want ws://127.0.0.1:9222/devtools/browser/abc", got)
	}
}

func TestStartProfile_FailureFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":false,"message":"profile not found"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, zap.NewNop())
	ctx := context.Background()

	_, err := client.StartProfile(ctx, "bad-profile")
	if err == nil {
		t.Fatal("expected error for failure flag, got nil")
	}
	if !strings.Contains(err.Error(), "profile not found") {
		t.Errorf("error should contain 'profile not found', got: %v", err)
	}
}

func TestStartProfile_Non200Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()

	client := NewClient(server.URL, zap.NewNop())
	ctx := context.Background()

	_, err := client.StartProfile(ctx, "test-profile")
	if err == nil {
		t.Fatal("expected error for 500 status, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention 500 status, got: %v", err)
	}
}

func TestStartProfile_RemoteDebugFallback(t *testing.T) {
	// First response: no ws_endpoint, has remote_debugging_address pointing to test server.
	// The CDP endpoint /json/version is served on the same test server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		if strings.Contains(path, "/profiles/start/") {
			// Return remote_debugging_address pointing back to the test server
			addr := r.Host
			resp := fmt.Sprintf(`{"success":true,"data":{"remote_debugging_address":"%s"}}`, addr)
			_, _ = w.Write([]byte(resp))
		} else if strings.Contains(path, "/json/version") {
			_, _ = w.Write([]byte(`{"webSocketDebuggerUrl":"ws://127.0.0.1:9222/devtools/browser/xyz"}`))
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, zap.NewNop())
	ctx := context.Background()

	got, err := client.StartProfile(ctx, "test-profile")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ws://127.0.0.1:9222/devtools/browser/xyz" {
		t.Errorf("got %q, want ws://127.0.0.1:9222/devtools/browser/xyz", got)
	}
}

func TestStartProfile_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return empty so GPM retries
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.StartProfile(ctx, "test-profile")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestStopProfile_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/profiles/close/") {
			t.Errorf("expected /profiles/close/... path, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, zap.NewNop())
	ctx := context.Background()

	err := client.StopProfile(ctx, "test-profile-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStopProfile_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, zap.NewNop())
	ctx := context.Background()

	err := client.StopProfile(ctx, "test-profile")
	if err == nil {
		t.Fatal("expected error for 500 status on stop, got nil")
	}
}

func TestStopProfile_ContextCancelled(t *testing.T) {
	client := NewClient("http://localhost:9999", zap.NewNop())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.StopProfile(ctx, "test-profile")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}
