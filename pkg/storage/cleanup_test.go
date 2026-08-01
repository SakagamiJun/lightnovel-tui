package storage

import (
	"os"
	"path/filepath"
	"testing"

	"lnr-core/pkg/model"
)

func TestStorageBreakdownAndCleanup(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cleanup-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cacheDir := filepath.Join(tmpDir, "cache")
	exportDir := filepath.Join(tmpDir, "exports")
	_ = os.MkdirAll(exportDir, 0755)

	store, err := NewStorage(cacheDir)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}

	bookID := "8888"
	// Save book detail and chapter
	detail := &model.BookDetail{
		BookSummary: model.BookSummary{
			ID:    bookID,
			Title: "测试轻小说",
		},
	}
	if err := store.SaveBookDetail(detail); err != nil {
		t.Fatalf("SaveBookDetail failed: %v", err)
	}

	ch := &model.ChapterContent{
		ID:     "c1",
		BookID: bookID,
		Title:  "第一章",
		Elements: []model.ContentElement{
			{Type: model.ContentTypeText, Text: "正文内容正文内容"},
		},
	}
	if err := store.SaveChapter(ch); err != nil {
		t.Fatalf("SaveChapter failed: %v", err)
	}

	// Create cover and illustration
	_ = store.EnsureImageDir(bookID)
	coverPath := filepath.Join(store.BookDir(bookID), "images", "cover.jpg")
	_ = os.WriteFile(coverPath, []byte("cover-image-data-123"), 0644)

	illustPath := store.IllustrationPath(bookID, "http://example.com/illust1.jpg")
	_ = os.WriteFile(illustPath, []byte("illustration-image-data-4567890"), 0644)

	// Create dummy exported EPUB
	epubPath := filepath.Join(exportDir, "test.epub")
	_ = os.WriteFile(epubPath, []byte("dummy-epub-archive-bytes"), 0644)

	// Test GetStorageBreakdown
	breakdown, err := store.GetStorageBreakdown(exportDir)
	if err != nil {
		t.Fatalf("GetStorageBreakdown failed: %v", err)
	}

	if breakdown.TextBytes <= 0 {
		t.Errorf("expected TextBytes > 0, got %d", breakdown.TextBytes)
	}
	if breakdown.ImageBytes <= 0 {
		t.Errorf("expected ImageBytes > 0, got %d", breakdown.ImageBytes)
	}
	if breakdown.EpubBytes <= 0 {
		t.Errorf("expected EpubBytes > 0, got %d", breakdown.EpubBytes)
	}
	if len(breakdown.BookItems) != 1 || breakdown.BookItems[0].Title != "测试轻小说" {
		t.Errorf("unexpected book items: %+v", breakdown.BookItems)
	}

	// Test CleanImagesOnly with keepCovers = true
	freedImages, err := store.CleanImagesOnly(true)
	if err != nil {
		t.Fatalf("CleanImagesOnly failed: %v", err)
	}
	if freedImages == 0 {
		t.Errorf("expected freed bytes > 0")
	}

	// Cover should still exist
	if _, err := os.Stat(coverPath); os.IsNotExist(err) {
		t.Errorf("cover should be preserved when keepCovers is true")
	}
	// Illustration should be deleted
	if _, err := os.Stat(illustPath); err == nil {
		t.Errorf("illustration should have been deleted")
	}

	// Test CleanEpubsOnly
	freedEpubs, err := store.CleanEpubsOnly(exportDir)
	if err != nil {
		t.Fatalf("CleanEpubsOnly failed: %v", err)
	}
	if freedEpubs == 0 {
		t.Errorf("expected freed EPUB bytes > 0")
	}
	if _, err := os.Stat(epubPath); err == nil {
		t.Errorf("epub should have been deleted")
	}
}
