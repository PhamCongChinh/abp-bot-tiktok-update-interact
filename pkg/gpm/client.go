package gpm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// GPMClient abstracts the GoLogin Profile Manager operations.
// Define at provider side so consumers (Crawler) can accept this interface
// for testability.
type GPMClient interface {
	StartProfile(ctx context.Context, profileID string) (string, error)
	StopProfile(ctx context.Context, profileID string) error
}

type Client struct {
	apiURL string
	log    *zap.Logger
	client *http.Client
}

type StartProfileResponse struct {
	Data struct {
		RemoteDebuggingAddress string `json:"remote_debugging_address"`
		WSEndpoint             string `json:"ws_endpoint"`
	} `json:"data"`
}

func NewClient(apiURL string, log *zap.Logger) *Client {
	return &Client{
		apiURL: apiURL,
		log:    log,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) StartProfile(ctx context.Context, profileID string) (string, error) {
	url := fmt.Sprintf("%s/profiles/start/%s", c.apiURL, profileID)

	// Retry start profile up to 5 times - GPM sometimes needs time to launch browser
	var debugAddr string
	maxRetries := 5
	for attempt := 1; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", fmt.Errorf("failed to create request: %w", err)
		}
		resp, err := c.client.Do(req)
		if err != nil {
			if attempt == maxRetries {
				return "", fmt.Errorf("failed to start profile: %w", err)
			}
			select {
			case <-time.After(3 * time.Second):
			case <-ctx.Done():
				return "", ctx.Err()
			}
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("GPM API returned status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var raw map[string]any
		if err := json.Unmarshal(body, &raw); err != nil {
			return "", fmt.Errorf("failed to decode response: %w", err)
		}

		// Check success flag - if false, no point retrying
		if success, ok := raw["success"].(bool); ok && !success {
			msg, _ := raw["message"].(string)
			return "", fmt.Errorf("GPM error: %s", msg)
		}

		var result StartProfileResponse
		if err := json.Unmarshal(body, &result); err != nil {
			return "", fmt.Errorf("failed to decode response: %w", err)
		}

		// Try ws_endpoint first (direct WS URL), fallback to remote_debugging_address
		if result.Data.WSEndpoint != "" {
			c.log.Sugar().Infof("GPM profile %s started", profileID[:8])
			return result.Data.WSEndpoint, nil
		}

		debugAddr = result.Data.RemoteDebuggingAddress
		if debugAddr != "" {
			break
		}

		// Both empty - browser still starting, wait and retry
		c.log.Sugar().Warnf("GPM profile %s: browser not ready (attempt %d/%d), waiting...", profileID[:8], attempt, maxRetries)
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	if debugAddr == "" {
		return "", fmt.Errorf("empty remote_debugging_address after %d attempts", maxRetries)
	}

	// Get WebSocket endpoint from CDP
	wsURL, err := c.getWebSocketURL(ctx, debugAddr)
	if err != nil {
		return "", fmt.Errorf("failed to get WebSocket URL: %w", err)
	}

	c.log.Sugar().Infof("GPM profile %s started", profileID[:8])
	return wsURL, nil
}

func (c *Client) getWebSocketURL(ctx context.Context, debugAddr string) (string, error) {
	// Query CDP /json/version to get webSocketDebuggerUrl
	url := fmt.Sprintf("http://%s/json/version", debugAddr)

	// Retry up to 5 times with 2 second delay (total ~10 seconds)
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		} else {
			// First attempt: wait 2 seconds for browser to start
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}

		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if reqErr != nil {
			return "", fmt.Errorf("failed to create CDP request: %w", reqErr)
		}
		resp, err := c.client.Do(req)
		if err != nil {
			if i == maxRetries-1 {
				return "", fmt.Errorf("failed to query CDP after %d attempts: %w", maxRetries, err)
			}
			continue
		}

		var result map[string]any
		decodeErr := json.NewDecoder(resp.Body).Decode(&result)
		_ = resp.Body.Close()
		if decodeErr != nil {
			return "", fmt.Errorf("failed to decode CDP response: %w", decodeErr)
		}

		wsURL, ok := result["webSocketDebuggerUrl"].(string)
		if !ok || wsURL == "" {
			return "", fmt.Errorf("webSocketDebuggerUrl not found or empty in CDP response")
		}

		// Validate WebSocket URL format.
		if !strings.HasPrefix(wsURL, "ws://") && !strings.HasPrefix(wsURL, "wss://") {
			return "", fmt.Errorf("invalid WebSocket URL format: %s", wsURL)
		}

		return wsURL, nil
	}

	return "", fmt.Errorf("failed to get WebSocket URL after %d attempts", maxRetries)
}

func (c *Client) StopProfile(ctx context.Context, profileID string) error {
	url := fmt.Sprintf("%s/profiles/close/%s", c.apiURL, profileID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create stop request: %w", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		c.log.Sugar().Warnf("Failed to stop profile %s: %v", profileID[:8], err)
		return fmt.Errorf("failed to stop profile: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		c.log.Sugar().Warnf("GPM stop profile %s returned %d: %s", profileID[:8], resp.StatusCode, string(body))
		return fmt.Errorf("GPM stop returned status %d", resp.StatusCode)
	}

	c.log.Sugar().Infof("GPM profile %s stopped", profileID[:8])
	return nil
}
