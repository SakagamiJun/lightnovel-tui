package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"lnr-core/pkg/model"
	"lnr-core/pkg/source"
)

type mockUpdateSource struct {
	remoteCatalog *model.BookCatalog
}

func (m *mockUpdateSource) Name() string { return "mock" }
func (m *mockUpdateSource) Search(ctx context.Context, sType source.SearchType, keyword string, page int) ([]model.BookSummary, int, error) {
	return nil, 0, nil
}
func (m *mockUpdateSource) GetBookDetail(ctx context.Context, bookID string) (*model.BookDetail, error) {
	return nil, nil
}
func (m *mockUpdateSource) GetCatalog(ctx context.Context, bookID string) (*model.BookCatalog, error) {
	return m.remoteCatalog, nil
}
func (m *mockUpdateSource) GetChapterContent(ctx context.Context, bookID, chapterID string) (*model.ChapterContent, error) {
	return nil, nil
}
func (m *mockUpdateSource) GetToplist(ctx context.Context, tType source.ToplistType, page int) ([]model.BookSummary, int, error) {
	return nil, 0, nil
}
func (m *mockUpdateSource) GetTags() []string { return nil }
func (m *mockUpdateSource) GetTagBooks(ctx context.Context, tag string, page int) ([]model.BookSummary, int, error) {
	return nil, 0, nil
}

func TestSortBooks(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStorage(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	books := []model.BookDetail{
		{
			BookSummary: model.BookSummary{ID: "book_c", Title: "刀剑神域", LastUpdated: "2023-05-01"},
		},
		{
			BookSummary: model.BookSummary{ID: "book_a", Title: "加速世界", LastUpdated: "2024-01-01"},
		},
		{
			BookSummary: model.BookSummary{ID: "book_b", Title: "关于我转生变成史莱姆这档事", LastUpdated: "2023-12-01"},
		},
	}

	// 1. Sort by title
	store.SortBooks(books, SortByTitle)
	if books[0].ID != "book_b" || books[1].ID != "book_c" || books[2].ID != "book_a" {
		t.Errorf("SortByTitle unexpected order: %s, %s, %s", books[0].ID, books[1].ID, books[2].ID)
	}

	// 2. Sort by last updated (newest first)
	store.SortBooks(books, SortByLastUpdated)
	if books[0].ID != "book_a" || books[1].ID != "book_b" || books[2].ID != "book_c" {
		t.Errorf("SortByLastUpdated unexpected order: %s, %s, %s", books[0].ID, books[1].ID, books[2].ID)
	}

	// 3. Pin book_c (it should always jump to the very top!)
	_, _ = store.TogglePinBook("book_c")
	store.SortBooks(books, SortByLastUpdated)
	if books[0].ID != "book_c" {
		t.Errorf("expected pinned book_c to be at index 0, got %s", books[0].ID)
	}

	// 4. Sort by last read
	// Write progress for book_b
	pDir := store.BookDir("book_b")
	_ = os.MkdirAll(pDir, 0755)
	pData, _ := json.Marshal(map[string]any{"last_read_at": 1700000000})
	_ = os.WriteFile(filepath.Join(pDir, "progress.json"), pData, 0644)

	// Unpin book_c
	_, _ = store.TogglePinBook("book_c")
	store.SortBooks(books, SortByLastRead)
	if books[0].ID != "book_b" {
		t.Errorf("expected recently read book_b to be at index 0, got %s", books[0].ID)
	}
}

func TestCheckBookUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewStorage(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	bookID := "1001"
	localCat := &model.BookCatalog{
		BookID: bookID,
		Volumes: []model.Volume{
			{
				ID: "v1", Title: "第一卷",
				Chapters: []model.Chapter{
					{ID: "c1", Title: "第一章"},
					{ID: "c2", Title: "第二章"},
				},
			},
		},
	}
	_ = store.SaveCatalog(localCat)

	remoteCat := &model.BookCatalog{
		BookID: bookID,
		Volumes: []model.Volume{
			{
				ID: "v1", Title: "第一卷",
				Chapters: []model.Chapter{
					{ID: "c1", Title: "第一章"},
					{ID: "c2", Title: "第二章"},
					{ID: "c3", Title: "第三章 (新)"},
				},
			},
		},
	}

	mockSrc := &mockUpdateSource{remoteCatalog: remoteCat}
	info, err := store.CheckBookUpdate(context.Background(), mockSrc, bookID)
	if err != nil {
		t.Fatalf("CheckBookUpdate failed: %v", err)
	}

	if !info.HasUpdate {
		t.Errorf("expected HasUpdate to be true")
	}
	if info.NewChapterCount != 1 {
		t.Errorf("expected 1 new chapter, got %d", info.NewChapterCount)
	}
	if info.LocalChapters != 2 || info.RemoteChapters != 3 {
		t.Errorf("expected 2 local, 3 remote chapters, got %d local, %d remote", info.LocalChapters, info.RemoteChapters)
	}
}
