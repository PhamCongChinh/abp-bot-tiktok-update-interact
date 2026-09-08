package parser

import (
	"testing"
	"time"

	"abp-bot-tiktok/internal/models"
)

func TestFromVideoItem_Valid(t *testing.T) {
	before := time.Now().Unix()
	v := models.VideoItem{
		Keyword:     "test keyword",
		OrgID:       42,
		VideoID:     "abc123",
		Description: "A test video description",
		PubTime:     1700000000,
		UniqueID:    "testuser",
		AuthID:      "auth-001",
		AuthName:    "Test User",
		Comments:    100,
		Shares:      200,
		Reactions:   300,
		Favors:      50,
		Views:       10000,
	}
	post := FromVideoItem(v)
	after := time.Now().Unix()

	if post.DocType != 1 {
		t.Errorf("DocType = %d, want 1", post.DocType)
	}
	if post.CrawlSource != 2 {
		t.Errorf("CrawlSource = %d, want 2", post.CrawlSource)
	}
	if post.CrawlSourceCode != "tt" {
		t.Errorf("CrawlSourceCode = %q, want %q", post.CrawlSourceCode, "tt")
	}
	if post.OrgID != 42 {
		t.Errorf("OrgID = %d, want 42", post.OrgID)
	}
	if post.PubTime != 1700000000 {
		t.Errorf("PubTime = %d, want 1700000000", post.PubTime)
	}
	if post.CrawlTime < before || post.CrawlTime > after {
		t.Errorf("CrawlTime = %d, want between %d and %d", post.CrawlTime, before, after)
	}
	if post.SubjectID != "abc123" {
		t.Errorf("SubjectID = %q, want %q", post.SubjectID, "abc123")
	}
	if post.Description != "A test video description" {
		t.Errorf("Description = %q, want %q", post.Description, "A test video description")
	}
	if post.Content != "A test video description" {
		t.Errorf("Content = %q, want %q", post.Content, "A test video description")
	}
	wantURL := "https://www.tiktok.com/@testuser/video/abc123"
	if post.URL != wantURL {
		t.Errorf("URL = %q, want %q", post.URL, wantURL)
	}
	if post.MediaURLs != "[]" {
		t.Errorf("MediaURLs = %q, want %q", post.MediaURLs, "[]")
	}
	if post.Comments != 100 {
		t.Errorf("Comments = %d, want 100", post.Comments)
	}
	if post.Shares != 200 {
		t.Errorf("Shares = %d, want 200", post.Shares)
	}
	if post.Reactions != 300 {
		t.Errorf("Reactions = %d, want 300", post.Reactions)
	}
	if post.Favors != 50 {
		t.Errorf("Favors = %d, want 50", post.Favors)
	}
	if post.Views != 10000 {
		t.Errorf("Views = %d, want 10000", post.Views)
	}
	if post.WebTags != "[]" {
		t.Errorf("WebTags = %q, want %q", post.WebTags, "[]")
	}
	if post.WebKeywords != "[]" {
		t.Errorf("WebKeywords = %q, want %q", post.WebKeywords, "[]")
	}
	if post.AuthID != "auth-001" {
		t.Errorf("AuthID = %q, want %q", post.AuthID, "auth-001")
	}
	if post.AuthName != "Test User" {
		t.Errorf("AuthName = %q, want %q", post.AuthName, "Test User")
	}
	if post.AuthType != 1 {
		t.Errorf("AuthType = %d, want 1", post.AuthType)
	}
	wantAuthURL := "https://www.tiktok.com/@testuser"
	if post.AuthURL != wantAuthURL {
		t.Errorf("AuthURL = %q, want %q", post.AuthURL, wantAuthURL)
	}
	if post.SourceID != "auth-001" {
		t.Errorf("SourceID = %q, want %q", post.SourceID, "auth-001")
	}
	if post.SourceType != 5 {
		t.Errorf("SourceType = %d, want 5", post.SourceType)
	}
	if post.SourceName != "Test User" {
		t.Errorf("SourceName = %q, want %q", post.SourceName, "Test User")
	}
	if post.SourceURL != wantURL {
		t.Errorf("SourceURL = %q, want %q", post.SourceURL, wantURL)
	}
	if post.Sentiment != 0 {
		t.Errorf("Sentiment = %d, want 0", post.Sentiment)
	}
	if post.IsPriority {
		t.Error("IsPriority = true, want false")
	}
	if post.CrawlBot != "tiktok-1" {
		t.Errorf("CrawlBot = %q, want %q", post.CrawlBot, "tiktok-1")
	}
	if post.SourceOwnership != "nature" {
		t.Errorf("SourceOwnership = %q, want %q", post.SourceOwnership, "nature")
	}
	if post.Title != nil {
		t.Errorf("Title = %v, want nil", post.Title)
	}
	if post.ReplyTo != nil {
		t.Errorf("ReplyTo = %v, want nil", post.ReplyTo)
	}
	if post.Level != nil {
		t.Errorf("Level = %v, want nil", post.Level)
	}
}

func TestFromVideoItem_EmptyDescription(t *testing.T) {
	v := models.VideoItem{
		Keyword:     "test",
		OrgID:       1,
		VideoID:     "v1",
		Description: "",
		PubTime:     1700000000,
		UniqueID:    "u1",
		AuthID:      "a1",
		AuthName:    "User",
	}
	post := FromVideoItem(v)

	if post.Description != "" {
		t.Errorf("Description = %q, want empty", post.Description)
	}
	if post.Content != "" {
		t.Errorf("Content = %q, want empty", post.Content)
	}
}

func TestFromVideoItem_ZeroViews(t *testing.T) {
	v := models.VideoItem{
		Keyword:     "test",
		OrgID:       1,
		VideoID:     "v1",
		Description: "desc",
		PubTime:     1700000000,
		UniqueID:    "u1",
		AuthID:      "a1",
		AuthName:    "User",
		Views:       0,
		Comments:    0,
		Shares:      0,
		Reactions:   0,
		Favors:      0,
	}
	post := FromVideoItem(v)

	if post.Views != 0 {
		t.Errorf("Views = %d, want 0", post.Views)
	}
	if post.Comments != 0 {
		t.Errorf("Comments = %d, want 0", post.Comments)
	}
}

func TestFromVideoItem_UnicodeDescription(t *testing.T) {
	v := models.VideoItem{
		Keyword:     "тест",
		OrgID:       1,
		VideoID:     "v1",
		Description: "🎵 Bài hát hay nhất 中文 한국어 🌟",
		PubTime:     1700000000,
		UniqueID:    "😎cool_user",
		AuthID:      "a1",
		AuthName:    "Người Dùng 测试",
	}
	post := FromVideoItem(v)

	if post.Description != "🎵 Bài hát hay nhất 中文 한국어 🌟" {
		t.Errorf("Description = %q, want unicode preserved", post.Description)
	}
	if post.AuthName != "Người Dùng 测试" {
		t.Errorf("AuthName = %q, want unicode preserved", post.AuthName)
	}
	if post.URL != "https://www.tiktok.com/@😎cool_user/video/v1" {
		t.Errorf("URL = %q, want unicode preserved", post.URL)
	}
}

func TestFromVideoItem_EmptyVideoID(t *testing.T) {
	v := models.VideoItem{
		Keyword:     "test",
		OrgID:       1,
		VideoID:     "",
		Description: "desc",
		PubTime:     1700000000,
		UniqueID:    "user",
		AuthID:      "a1",
		AuthName:    "User",
	}
	post := FromVideoItem(v)

	if post.URL != "" {
		t.Errorf("URL = %q, want empty (empty video ID)", post.URL)
	}
	if post.SourceURL != "" {
		t.Errorf("SourceURL = %q, want empty", post.SourceURL)
	}
	if post.SubjectID != "" {
		t.Errorf("SubjectID = %q, want empty", post.SubjectID)
	}
}

func TestBuildVideoURL(t *testing.T) {
	tests := []struct {
		name     string
		uniqueID string
		postID   string
		want     string
	}{
		{"normal", "user1", "123", "https://www.tiktok.com/@user1/video/123"},
		{"empty postID", "user1", "", ""},
		{"empty uniqueID", "", "123", "https://www.tiktok.com/@/video/123"},
		{"both empty", "", "", ""},
		{"special chars uniqueID", "user.name_123", "456", "https://www.tiktok.com/@user.name_123/video/456"},
		{"long IDs", "verylonguserid12345", "verylongvideoid67890", "https://www.tiktok.com/@verylonguserid12345/video/verylongvideoid67890"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildVideoURL(tt.uniqueID, tt.postID)
			if got != tt.want {
				t.Errorf("buildVideoURL(%q, %q) = %q, want %q", tt.uniqueID, tt.postID, got, tt.want)
			}
		})
	}
}

func TestBuildAuthorURL(t *testing.T) {
	tests := []struct {
		name     string
		uniqueID string
		want     string
	}{
		{"normal", "user1", "https://www.tiktok.com/@user1"},
		{"empty", "", "https://www.tiktok.com/@"},
		{"special chars", "user.name-123", "https://www.tiktok.com/@user.name-123"},
		{"unicode", "người_dùng", "https://www.tiktok.com/@người_dùng"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildAuthorURL(tt.uniqueID)
			if got != tt.want {
				t.Errorf("buildAuthorURL(%q) = %q, want %q", tt.uniqueID, got, tt.want)
			}
		})
	}
}

func TestTiktokPost_AllFieldsSet(t *testing.T) {
	v := models.VideoItem{
		Keyword:     "kw",
		OrgID:       99,
		VideoID:     "vid-001",
		Description: "test desc",
		PubTime:     1712345678,
		UniqueID:    "creator01",
		AuthID:      "auth-xyz",
		AuthName:    "CreatorName",
		Comments:    10,
		Shares:      20,
		Reactions:   30,
		Favors:      5,
		Views:       500,
	}
	post := FromVideoItem(v)

	// Verify all struct fields are populated (not zero-value where data exists).
	if post.DocType == 0 {
		t.Error("DocType is zero, should be set")
	}
	if post.CrawlSource == 0 {
		t.Error("CrawlSource is zero, should be set")
	}
	if post.AuthType == 0 {
		t.Error("AuthType is zero, should be set")
	}
	if post.SourceType == 0 {
		t.Error("SourceType is zero, should be set")
	}
	if post.CrawlBot == "" {
		t.Error("CrawlBot is empty, should be set")
	}
	if post.SourceOwnership == "" {
		t.Error("SourceOwnership is empty, should be set")
	}
}
