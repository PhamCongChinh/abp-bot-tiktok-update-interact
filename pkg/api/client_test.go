package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"abp-bot-tiktok/internal/parser"

	"go.uber.org/zap"
)

func testLogger() *zap.Logger {
	return zap.NewNop()
}

func makeTestPost(orgID int, subjectID string) parser.TiktokPost {
	return parser.TiktokPost{
		OrgID:     orgID,
		SubjectID: subjectID,
		URL:       "https://example.com/video",
	}
}

func TestNewClient_ValidTimeout(t *testing.T) {
	cl := NewClient("http://example.com", 10*time.Second, testLogger())
	if cl.httpClient.Timeout != 10*time.Second {
		t.Fatalf("expected timeout 10s, got %v", cl.httpClient.Timeout)
	}
	if cl.baseURL != "http://example.com" {
		t.Fatalf("expected baseURL 'http://example.com', got %q", cl.baseURL)
	}
}

func TestNewClient_ZeroTimeoutDefaultsTo30s(t *testing.T) {
	cl := NewClient("http://example.com", 0, testLogger())
	if cl.httpClient.Timeout != 30*time.Second {
		t.Fatalf("expected default timeout 30s, got %v", cl.httpClient.Timeout)
	}
}

func TestNewClient_NegativeTimeoutDefaultsTo30s(t *testing.T) {
	cl := NewClient("http://example.com", -5*time.Second, testLogger())
	if cl.httpClient.Timeout != 30*time.Second {
		t.Fatalf("expected default timeout 30s, got %v", cl.httpClient.Timeout)
	}
}

func TestNewClient_AutoAddHTTPScheme(t *testing.T) {
	cl := NewClient("example.com", 30*time.Second, testLogger())
	if cl.baseURL != "http://example.com" {
		t.Fatalf("expected 'http://example.com', got %q", cl.baseURL)
	}
}

func TestNewClient_KeepHTTPSScheme(t *testing.T) {
	cl := NewClient("https://example.com", 30*time.Second, testLogger())
	if cl.baseURL != "https://example.com" {
		t.Fatalf("expected 'https://example.com', got %q", cl.baseURL)
	}
}

func TestNewClient_TrimTrailingSlash(t *testing.T) {
	cl := NewClient("http://example.com/", 30*time.Second, testLogger())
	if cl.baseURL != "http://example.com" {
		t.Fatalf("expected 'http://example.com', got %q", cl.baseURL)
	}
}

func TestPostUnclassified_EmptyPosts(t *testing.T) {
	cl := NewClient("http://example.com", 30*time.Second, testLogger())
	err := cl.PostUnclassified(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error for empty posts, got %v", err)
	}
}

func TestPostUnclassified_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cl := NewClient(srv.URL, 10*time.Second, testLogger())
	posts := []parser.TiktokPost{makeTestPost(1, "v1")}
	err := cl.PostUnclassified(context.Background(), posts)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestPostUnclassified_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cl := NewClient(srv.URL, 10*time.Second, testLogger())
	posts := []parser.TiktokPost{makeTestPost(1, "v1")}
	err := cl.PostUnclassified(context.Background(), posts)
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "server returned status 500") {
		t.Fatalf("expected status 500 error, got %v", err)
	}
}

func TestPostUnclassified_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	cl := NewClient(srv.URL, 10*time.Second, testLogger())
	posts := []parser.TiktokPost{makeTestPost(1, "v1")}
	err := cl.PostUnclassified(ctx, posts)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	if !strings.Contains(err.Error(), "context") && !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("expected context-related error, got %v", err)
	}
}

func TestPostUnclassified_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	cl := NewClient(srv.URL, 10*time.Second, testLogger())
	posts := []parser.TiktokPost{makeTestPost(1, "v1")}
	err := cl.PostUnclassified(ctx, posts)
	if err == nil {
		t.Fatal("expected error for timed-out context, got nil")
	}
}

func TestPostUnclassifiedBatch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %q", ct)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cl := NewClient(srv.URL, 10*time.Second, testLogger())
	posts := []parser.TiktokPost{makeTestPost(1, "v1"), makeTestPost(2, "v2")}
	err := cl.PostUnclassifiedBatch(context.Background(), posts)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestPostUnclassifiedBatch_EmptyPosts(t *testing.T) {
	cl := NewClient("http://example.com", 30*time.Second, testLogger())
	err := cl.PostUnclassifiedBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error for empty posts, got %v", err)
	}
}

func TestPostClassified_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/posts/insert-posts" {
			t.Errorf("expected /api/v1/posts/insert-posts, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cl := NewClient(srv.URL, 10*time.Second, testLogger())
	posts := []parser.TiktokPost{makeTestPost(2, "v2")}
	err := cl.PostClassified(context.Background(), posts)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestPostClassified_EmptyPosts(t *testing.T) {
	cl := NewClient("http://example.com", 30*time.Second, testLogger())
	err := cl.PostClassified(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error for empty posts, got %v", err)
	}
}

func TestPost_RequestBodyContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cl := NewClient(srv.URL, 10*time.Second, testLogger())
	posts := []parser.TiktokPost{makeTestPost(3, "v3")}
	err := cl.PostUnclassified(context.Background(), posts)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
