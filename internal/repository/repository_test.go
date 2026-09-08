package repository

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadKeywordsFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keywords.json")

	data, err := json.Marshal([]string{"kw1", "kw2", "kw3"})
	if err != nil {
		t.Fatalf("failed to marshal fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	keywords, err := LoadKeywordsFromFile(path, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keywords) != 3 {
		t.Fatalf("expected 3 keywords, got %d", len(keywords))
	}
	for i, kw := range keywords {
		if kw.OrgID != 7 {
			t.Errorf("keyword[%d].OrgID = %d, want 7", i, kw.OrgID)
		}
		if !kw.Active {
			t.Errorf("keyword[%d].Active = false, want true", i)
		}
	}
	if keywords[0].Keyword != "kw1" || keywords[2].Keyword != "kw3" {
		t.Errorf("unexpected keyword values: %+v", keywords)
	}
}

func TestLoadKeywordsFromFile_MissingFile(t *testing.T) {
	_, err := LoadKeywordsFromFile(filepath.Join(t.TempDir(), "missing.json"), 0)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadKeywordsFromFile_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keywords.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	_, err := LoadKeywordsFromFile(path, 0)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestKeywordModel(t *testing.T) {
	k := Keyword{
		ID:      42,
		Keyword: "test-keyword",
		OrgID:   42,
		Active:  true,
	}
	if k.Keyword != "test-keyword" {
		t.Errorf("Keyword = %q, want %q", k.Keyword, "test-keyword")
	}
	if k.OrgID != 42 {
		t.Errorf("OrgID = %d, want 42", k.OrgID)
	}
	if !k.Active {
		t.Error("expected Active=true")
	}
}

func TestBotConfigModel(t *testing.T) {
	now := time.Now()
	bc := BotConfig{
		ID:        1,
		BotName:   "testbot",
		BotType:   "video",
		OrgIDs:    []string{"org1", "org2"},
		Sleep:     30,
		GPMAPI:    "https://gpm.example.com",
		ProfileID: "profile-001",
		Active:    true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if bc.BotName != "testbot" {
		t.Errorf("BotName = %q, want %q", bc.BotName, "testbot")
	}
	if bc.BotType != "video" {
		t.Errorf("BotType = %q, want %q", bc.BotType, "video")
	}
	if len(bc.OrgIDs) != 2 {
		t.Errorf("OrgIDs len = %d, want 2", len(bc.OrgIDs))
	}
	if bc.Sleep != 30 {
		t.Errorf("Sleep = %d, want 30", bc.Sleep)
	}
}

func TestVideoDocumentModel(t *testing.T) {
	vd := VideoDocument{
		Keyword:     "kw",
		VideoID:     "vid-1",
		Description: "desc",
		PubTime:     1700000000,
		UniqueID:    "user1",
		AuthID:      "auth1",
		AuthName:    "User",
		Comments:    10,
		Shares:      5,
		Reactions:   3,
		Favors:      1,
		Views:       100,
	}
	if vd.Keyword != "kw" {
		t.Errorf("Keyword = %q, want %q", vd.Keyword, "kw")
	}
	if vd.VideoID != "vid-1" {
		t.Errorf("VideoID = %q, want %q", vd.VideoID, "vid-1")
	}
	if vd.Comments != 10 {
		t.Errorf("Comments = %d, want 10", vd.Comments)
	}
}
