package wenku8

import (
	"context"
	"testing"
	"time"

	"lnr-core/pkg/source"
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

	// Test Search
	results, _, err := src.Search(ctx, source.SearchTypeTitle, "史莱姆", 1)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatalf("expected search results for '史莱姆', got 0")
	}

	first := results[0]
	t.Logf("Found book: ID=%s, Title=%s, Author=%s", first.ID, first.Title, first.Author)

	// Test Detail
	detail, err := src.GetBookDetail(ctx, first.ID)
	if err != nil {
		t.Fatalf("get detail failed: %v", err)
	}
	t.Logf("Book Detail: Title=%s, Publisher=%s, Words=%d, StatusComplete=%v",
		detail.Title, detail.Publisher, detail.WordCount, detail.IsComplete)

	// Test Catalog
	catalog, err := src.GetCatalog(ctx, first.ID)
	if err != nil {
		t.Fatalf("get catalog failed: %v", err)
	}
	t.Logf("Catalog volumes count: %d", len(catalog.Volumes))
	if len(catalog.Volumes) > 0 {
		t.Logf("First volume: %s with %d chapters", catalog.Volumes[0].Title, len(catalog.Volumes[0].Chapters))
	}
}
