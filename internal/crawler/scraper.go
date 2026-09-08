package crawler

import (
	"fmt"
	"time"

	"github.com/playwright-community/playwright-go"
	"go.uber.org/zap"
)

// Scraper handles browser page creation, separating this concern from the
// Crawler orchestrator.
type Scraper struct{}

// NewScraper creates a stateless Scraper.
func NewScraper() *Scraper {
	return &Scraper{}
}

// CreatePageWithRetry creates a new browser page with retry logic.
func (s *Scraper) CreatePageWithRetry(context playwright.BrowserContext, maxRetries int, log *zap.Logger) (playwright.Page, error) {
	var page playwright.Page
	var err error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		page, err = context.NewPage()
		if err == nil {
			return page, nil
		}
		if containsAny(err.Error(), []string{"target closed", "Target page", "browser has been closed"}) {
			return nil, fmt.Errorf("createPageWithRetry: browser closed: %w", err)
		}
		if attempt < maxRetries {
			time.Sleep(2 * time.Second)
		}
	}
	return nil, fmt.Errorf("createPageWithRetry: failed after %d attempts: %w", maxRetries, err)
}
