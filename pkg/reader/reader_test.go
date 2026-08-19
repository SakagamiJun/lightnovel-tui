package reader

import (
	"os"
	"testing"
	"time"

	"github.com/SakagamiJun/lnovel_tui/pkg/model"
	"github.com/SakagamiJun/lnovel_tui/pkg/storage"
)

func TestReaderEngine(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "reader-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	bookID := "9999"
	// Save mock catalog
	catalog := &model.BookCatalog{
		BookID: bookID,
		Volumes: []model.Volume{
			{
				ID:    "v1",
				Title: "第一卷",
				Chapters: []model.Chapter{
					{ID: "c1", Title: "第一章"},
				},
			},
		},
	}
	if err := store.SaveCatalog(catalog); err != nil {
		t.Fatalf("failed to save catalog: %v", err)
	}

	// Save mock chapter
	ch := &model.ChapterContent{
		ID:     "c1",
		BookID: bookID,
		Title:  "第一章 异界之门",
		Elements: []model.ContentElement{
			{Type: model.ContentTypeText, Text: "微风吹拂着山丘。\n少年睁开了双眼。"},
			{Type: model.ContentTypeImage, URL: "http://example.com/cover.jpg"},
		},
	}
	if err := store.SaveChapter(ch); err != nil {
		t.Fatalf("failed to save chapter: %v", err)
	}

	r, err := NewReader(store, bookID)
	if err != nil {
		t.Fatalf("NewReader failed: %v", err)
	}

	loaded, err := r.LoadChapter("c1")
	if err != nil {
		t.Fatalf("LoadChapter failed: %v", err)
	}
	if loaded.Title != "第一章 异界之门" {
		t.Errorf("expected chapter title '第一章 异界之门', got %q", loaded.Title)
	}

	lines := r.Lines()
	if len(lines) != 3 {
		t.Fatalf("expected 3 formatted lines (2 text + 1 illust), got %d", len(lines))
	}
	t.Logf("Formatted Lines: %q", lines)

	// Test Progress Save/Load
	p := &ReadingProgress{
		BookID:     bookID,
		VolumeID:   "v1",
		ChapterID:  "c1",
		LineIndex:  2,
		LastReadAt: time.Now().Unix(),
	}
	if err := r.SaveProgress(p); err != nil {
		t.Fatalf("SaveProgress failed: %v", err)
	}

	loadedP, err := r.LoadProgress()
	if err != nil {
		t.Fatalf("LoadProgress failed: %v", err)
	}
	if loadedP.LineIndex != 2 || loadedP.ChapterID != "c1" {
		t.Errorf("mismatched reading progress: %+v", loadedP)
	}

	// Test Traditional Toggle
	if r.IsTraditional() {
		t.Errorf("expected initially simplified")
	}
	r.ToggleTraditional()
	if !r.IsTraditional() {
		t.Errorf("expected traditional active after toggle")
	}
	tradLines := r.Lines()
	if len(tradLines) != 3 || tradLines[0] != "\u3000\u3000微風吹拂著山丘。" {
		t.Errorf("unexpected traditional line: %q", tradLines[0])
	}
}
