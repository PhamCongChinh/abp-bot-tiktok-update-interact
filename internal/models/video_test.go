package models

import (
	"testing"
)

func TestVideoItem_Fields(t *testing.T) {
	v := VideoItem{
		Keyword:     "test-kw",
		OrgID:       42,
		VideoID:     "vid-001",
		Description: "A test description",
		PubTime:     1700000000,
		UniqueID:    "user1",
		AuthID:      "auth-001",
		AuthName:    "Test User",
		Comments:    100,
		Shares:      200,
		Reactions:   300,
		Favors:      50,
		Views:       10000,
	}

	if v.Keyword != "test-kw" {
		t.Errorf("Keyword = %q, want 'test-kw'", v.Keyword)
	}
	if v.OrgID != 42 {
		t.Errorf("OrgID = %d, want 42", v.OrgID)
	}
	if v.VideoID != "vid-001" {
		t.Errorf("VideoID = %q, want 'vid-001'", v.VideoID)
	}
	if v.Description != "A test description" {
		t.Errorf("Description = %q, want 'A test description'", v.Description)
	}
	if v.PubTime != 1700000000 {
		t.Errorf("PubTime = %d, want 1700000000", v.PubTime)
	}
	if v.UniqueID != "user1" {
		t.Errorf("UniqueID = %q, want 'user1'", v.UniqueID)
	}
	if v.AuthID != "auth-001" {
		t.Errorf("AuthID = %q, want 'auth-001'", v.AuthID)
	}
	if v.AuthName != "Test User" {
		t.Errorf("AuthName = %q, want 'Test User'", v.AuthName)
	}
	if v.Comments != 100 {
		t.Errorf("Comments = %d, want 100", v.Comments)
	}
	if v.Shares != 200 {
		t.Errorf("Shares = %d, want 200", v.Shares)
	}
	if v.Reactions != 300 {
		t.Errorf("Reactions = %d, want 300", v.Reactions)
	}
	if v.Favors != 50 {
		t.Errorf("Favors = %d, want 50", v.Favors)
	}
	if v.Views != 10000 {
		t.Errorf("Views = %d, want 10000", v.Views)
	}
}

func TestVideoItem_ZeroValues(t *testing.T) {
	v := VideoItem{}
	if v.Keyword != "" {
		t.Errorf("Keyword zero value = %q, want empty", v.Keyword)
	}
	if v.OrgID != 0 {
		t.Errorf("OrgID zero value = %d, want 0", v.OrgID)
	}
	if v.Comments != 0 {
		t.Errorf("Comments zero value = %d, want 0", v.Comments)
	}
}
