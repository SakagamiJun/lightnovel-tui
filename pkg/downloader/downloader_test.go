package downloader

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/SakagamiJun/lnovel_tui/pkg/model"
	"github.com/SakagamiJun/lnovel_tui/pkg/source/wenku8"
	"github.com/SakagamiJun/lnovel_tui/pkg/storage"
)

func TestDownloadVolume(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	tmpDir, err := os.MkdirTemp("", "lnr-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	src, err := wenku8.NewWenku8Source()
	if err != nil {
		t.Fatalf("failed to init wenku8: %v", err)
	}

	dl, err := NewDownloader(src, store)
	if err != nil {
		t.Fatalf("failed to init downloader: %v", err)
	}

	// Minimal test volume with 2 chapters (1 text, 1 illustration)
	testVolume := &model.Volume{
		ID:    "test_vol",
		Title: "测试卷",
		Chapters: []model.Chapter{
			{ID: "177911", Title: "序章"},
			{ID: "177919", Title: "插图"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	if ch.Title == "" {
		t.Errorf("expected title in cached chapter, got empty")
	}

	// Check that cover download works
	coverPath, err := dl.DownloadCover(ctx, "4340", "http://img.wenku8.com/image/4/4340/4340s.jpg")
	if err != nil {
		t.Fatalf("failed to download cover: %v", err)
	}
	if _, err := os.Stat(coverPath); err != nil {
		t.Errorf("cover file does not exist: %v", err)
	}
	t.Logf("Downloaded cover successfully at: %s", coverPath)
}
