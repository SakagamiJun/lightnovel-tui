package storage

import (
	"os"
	"testing"

	"lnr-core/pkg/model"
)

func TestListCachedBooks(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	// Initially empty
	books, err := store.ListCachedBooks()
	if err != nil {
		t.Fatalf("ListCachedBooks failed on empty dir: %v", err)
	}
	if len(books) != 0 {
		t.Errorf("expected 0 books, got %d", len(books))
	}

	// Save 2 mock books
	b1 := &model.BookDetail{
		BookSummary: model.BookSummary{
			ID:     "1001",
			Title:  "小说一",
			Author: "作者一",
		},
	}
	b2 := &model.BookDetail{
		BookSummary: model.BookSummary{
			ID:     "1002",
			Title:  "小说二",
			Author: "作者二",
		},
	}
	_ = store.SaveBookDetail(b1)
	_ = store.SaveBookDetail(b2)

	books, err = store.ListCachedBooks()
	if err != nil {
		t.Fatalf("ListCachedBooks failed: %v", err)
	}
	if len(books) != 2 {
		t.Errorf("expected 2 books, got %d", len(books))
	}
	t.Logf("Found %d cached books successfully", len(books))
}
