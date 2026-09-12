package wenku8

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/SakagamiJun/lightnovel-tui/pkg/model"
)

func TestWenku8ChapterContent(t *testing.T) {
	if testing.Short() || os.Getenv("CI") != "" {
		t.Skip("skipping network test in short/CI mode")
	}

	src, err := NewWenku8Source()
	if err != nil {
		t.Fatalf("failed to init wenku8 source: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Test text chapter (Book 4340, Chapter 177911)
	ch, err := src.GetChapterContent(ctx, "4340", "177911")
	if err != nil {
		if isNetworkBlocked(err) {
			t.Skipf("skipping test due to network error/block: %v", err)
		}
		t.Fatalf("failed to get text chapter: %v", err)
	}

	if len(ch.Elements) == 0 {
		t.Fatalf("expected elements in chapter, got 0")
	}
	t.Logf("Chapter %s title: %s, elements: %d", ch.ID, ch.Title, len(ch.Elements))

	// Test illustration chapter (Book 4340, Chapter 177919)
	imgCh, err := src.GetChapterContent(ctx, "4340", "177919")
	if err != nil {
		t.Fatalf("failed to get image chapter: %v", err)
	}

	imgCount := 0
	for _, el := range imgCh.Elements {
		if el.Type == model.ContentTypeImage {
			imgCount++
		}
	}
	if imgCount == 0 {
		t.Fatalf("expected illustrations in chapter 177919, got 0")
	}
	t.Logf("Illustration chapter parsed %d images successfully", imgCount)
}
