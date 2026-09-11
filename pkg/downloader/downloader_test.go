package downloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/SakagamiJun/lnovel_tui/pkg/model"
	"github.com/SakagamiJun/lnovel_tui/pkg/source"
	"github.com/SakagamiJun/lnovel_tui/pkg/storage"
)

type mockDataSource struct{}

func (m *mockDataSource) Name() string { return "mock" }

func (m *mockDataSource) Search(ctx context.Context, searchType source.SearchType, keyword string, page int) ([]model.BookSummary, int, error) {
	return nil, 0, nil
}

func (m *mockDataSource) GetBookDetail(ctx context.Context, bookID string) (*model.BookDetail, error) {
	return nil, nil
}

func (m *mockDataSource) GetCatalog(ctx context.Context, bookID string) (*model.BookCatalog, error) {
	return nil, nil
}

func (m *mockDataSource) GetChapterContent(ctx context.Context, bookID, chapterID string) (*model.ChapterContent, error) {
	if chapterID == "177911" {
		return &model.ChapterContent{
			ID:       "177911",
			BookID:   bookID,
			Title:    "序章",
			Elements: []model.ContentElement{{Type: model.ContentTypeText, Text: "这是序章测试正文内容。"}},
		}, nil
	}
	return &model.ChapterContent{
		ID:       chapterID,
		BookID:   bookID,
		Title:    "插图",
		Elements: []model.ContentElement{{Type: model.ContentTypeText, Text: "这是插图说明。"}},
	}, nil
}

func (m *mockDataSource) GetToplist(ctx context.Context, tType source.ToplistType, page int) ([]model.BookSummary, int, error) {
	return nil, 0, nil
}

func (m *mockDataSource) GetTags() []string { return nil }

func (m *mockDataSource) GetTagBooks(ctx context.Context, tag string, page int) ([]model.BookSummary, int, error) {
	return nil, 0, nil
}

func (m *mockDataSource) GetPublishers() []source.PublisherInfo { return nil }

func (m *mockDataSource) GetPublisherBooks(ctx context.Context, classID int, page int) ([]model.BookSummary, int, error) {
	return nil, 0, nil
}

func (m *mockDataSource) EnrichDescriptions(ctx context.Context, books []model.BookSummary, maxWorkers int) []model.BookSummary {
	return books
}

func TestDownloadVolume(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lnr-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	src := &mockDataSource{}
	dl, err := NewDownloader(src, store)
	if err != nil {
		t.Fatalf("failed to init downloader: %v", err)
	}

	testVolume := &model.Volume{
		ID:    "test_vol",
		Title: "测试卷",
		Chapters: []model.Chapter{
			{ID: "177911", Title: "序章"},
			{ID: "177919", Title: "插图"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	progressCalls := 0
	err = dl.DownloadVolume(ctx, "4340", testVolume, func(ev ProgressEvent) {
		progressCalls++
		t.Logf("[%d/%d] (%.1f%%) Downloading: %s", ev.Current, ev.Total, ev.Percentage, ev.ItemTitle)
	})
	if err != nil {
		t.Fatalf("DownloadVolume failed: %v", err)
	}

	if progressCalls != 2 {
		t.Errorf("expected 2 progress notifications, got %d", progressCalls)
	}

	// Check that chapter 177911 is cached
	ch, err := store.LoadChapter("4340", "177911")
	if err != nil {
		t.Fatalf("failed to load cached chapter 177911: %v", err)
	}
	if ch.Title != "序章" {
		t.Errorf("expected title '序章', got %q", ch.Title)
	}

	// Local test HTTP server for cover download
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("fake-cover-image-bytes"))
	}))
	defer server.Close()

	coverPath, err := dl.DownloadCover(ctx, "4340", server.URL+"/cover.jpg")
	if err != nil {
		t.Fatalf("failed to download cover: %v", err)
	}
	if _, err := os.Stat(coverPath); err != nil {
		t.Errorf("cover file does not exist: %v", err)
	}
	t.Logf("Downloaded cover successfully at: %s", coverPath)
}
