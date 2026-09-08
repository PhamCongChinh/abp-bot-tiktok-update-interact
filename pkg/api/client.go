package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"abp-bot-tiktok/internal/parser"

	"go.uber.org/zap"
)

// APIClient abstracts the backend API for pushing crawled posts.
// Define at provider side so consumers (Crawler, Publisher) can accept this
// interface for testability.
type APIClient interface {
	PostUnclassified(ctx context.Context, posts []parser.TiktokPost) error
	PostUnclassifiedBatch(ctx context.Context, posts []parser.TiktokPost) error
	PostClassified(ctx context.Context, posts []parser.TiktokPost) error
}

type Client struct {
	baseURL    string
	httpClient *http.Client
	log        *zap.Logger
}

type postRequest struct {
	Index  string              `json:"index"`
	Data   []parser.TiktokPost `json:"data"`
	Upsert bool                `json:"upsert"`
}

func NewClient(baseURL string, timeout time.Duration, log *zap.Logger) *Client {
	// Normalize baseURL - strip trailing slash, add http:// if missing
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	// Default timeout if zero or negative
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		log: log,
	}
}

func (c *Client) PostUnclassified(ctx context.Context, posts []parser.TiktokPost) error {
	return c.post(ctx, "/api/v1/posts/insert-unclassified-org-posts", "not_classify_org_posts", posts)
}

func (c *Client) PostUnclassifiedBatch(ctx context.Context, posts []parser.TiktokPost) error {
	return c.post(ctx, "/api/v1/posts/insert-unclassified-org-posts", "not_classify_org_posts", posts)
}

func (c *Client) PostClassified(ctx context.Context, posts []parser.TiktokPost) error {
	return c.post(ctx, "/api/v1/posts/insert-posts", "classify_org_posts", posts)
}

func (c *Client) post(ctx context.Context, path, index string, posts []parser.TiktokPost) error {
	if len(posts) == 0 {
		return nil
	}

	payload := postRequest{
		Index:  index,
		Data:   posts,
		Upsert: true,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s%s", c.baseURL, path)
	c.log.Info("api: pushing posts",
		zap.String("url", url),
		zap.String("index", index),
		zap.Int("count", len(posts)),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Drain response body to allow connection reuse.
	// Detect context cancellation during body reads so callers can distinguish
	// between real IO errors and a cancelled/timed-out context.
	_, drainErr := io.Copy(io.Discard, resp.Body)
	if drainErr != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("context cancelled while reading response: %w", ctx.Err())
		}
		return fmt.Errorf("failed to read response body: %w", drainErr)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	return nil
}
