package wenku8

import (
	"context"
	"testing"
	"time"

	"github.com/SakagamiJun/lnovel_tui/pkg/model"
	"github.com/SakagamiJun/lnovel_tui/pkg/source"
)

func TestWenku8SearchAndDetail(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	src, err := NewWenku8Source()
	if err != nil {
		t.Fatalf("failed to init wenku8 source: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	// Test Search (requires valid login session cookie)
	results, _, err := src.Search(ctx, source.SearchTypeTitle, "史莱姆", 1)
	bookID := "4340"
	if err != nil {
		t.Logf("search skipped: %v", err)
	} else if len(results) > 0 {
		bookID = results[0].ID
		t.Logf("Found book: ID=%s, Title=%s, Author=%s", results[0].ID, results[0].Title, results[0].Author)
	}

	// Test Detail
	detail, err := src.GetBookDetail(ctx, bookID)
	if err != nil {
		t.Fatalf("get detail failed: %v", err)
	}
	t.Logf("Book Detail: Title=%s, Publisher=%s, Words=%d, StatusComplete=%v",
		detail.Title, detail.Publisher, detail.WordCount, detail.IsComplete)

	// Test Catalog
	catalog, err := src.GetCatalog(ctx, bookID)
	if err != nil {
		t.Fatalf("get catalog failed: %v", err)
	}
	t.Logf("Catalog volumes count: %d", len(catalog.Volumes))
	if len(catalog.Volumes) > 0 {
		t.Logf("First volume: %s with %d chapters", catalog.Volumes[0].Title, len(catalog.Volumes[0].Chapters))
	}
}

func TestWenku8DescriptionCachingAndEnrichment(t *testing.T) {
	src := &Wenku8Source{}

	// Test caching
	src.SetCachedDescription("9999", "这是完整的测试小说简介，描述丰富，情节生动。")
	desc, ok := src.GetCachedDescription("9999")
	if !ok || desc != "这是完整的测试小说简介，描述丰富，情节生动。" {
		t.Fatalf("expected cached description, got %q, ok=%v", desc, ok)
	}

	// Test enriching from cache
	summaries := []model.BookSummary{
		{ID: "9999", Title: "测试书籍", Description: "这是简略..."},
	}
	enriched := src.EnrichDescriptions(context.Background(), summaries, 2)
	if enriched[0].Description != "这是完整的测试小说简介，描述丰富，情节生动。" {
		t.Errorf("expected enriched full description, got %q", enriched[0].Description)
	}
}
