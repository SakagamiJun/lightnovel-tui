package storage

import (
	"os"
	"testing"

	"github.com/SakagamiJun/lightnovel-tui/pkg/model"
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

func TestPinAndUnpinBook(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-pin-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	if store.IsBookPinned("1001") {
		t.Errorf("expected book 1001 not pinned initially")
	}

	// Toggle pin -> should be true
	pinned, err := store.TogglePinBook("1001")
	if err != nil || !pinned {
		t.Fatalf("expected pinned=true, got %v, err=%v", pinned, err)
	}
	if !store.IsBookPinned("1001") {
		t.Errorf("expected IsBookPinned to return true")
	}

	// Pin another book
	pinned, err = store.TogglePinBook("1002")
	if err != nil || !pinned {
		t.Fatalf("expected pinned=true for 1002, got %v, err=%v", pinned, err)
	}

	ids, _ := store.GetPinnedBookIDs()
	if len(ids) != 2 || ids[0] != "1002" || ids[1] != "1001" {
		t.Errorf("unexpected pinned ids order: %v", ids)
	}

	// Toggle unpin 1001
	pinned, err = store.TogglePinBook("1001")
	if err != nil || pinned {
		t.Fatalf("expected pinned=false for 1001, got %v, err=%v", pinned, err)
	}
	if store.IsBookPinned("1001") {
		t.Errorf("expected 1001 to be unpinned")
	}
}

func TestDeleteAndClearCache(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "storage-del-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	b1 := &model.BookDetail{BookSummary: model.BookSummary{ID: "1001", Title: "书1"}}
	_ = store.SaveBookDetail(b1)
	_, _ = store.TogglePinBook("1001")

	sz, err := store.CalculateCacheSize()
	if err != nil || sz <= 0 {
		t.Errorf("expected positive cache size, got %d, err: %v", sz, err)
	}

	// Delete book
	if err := store.DeleteBook("1001"); err != nil {
		t.Fatalf("failed to delete book: %v", err)
	}
	if store.IsBookPinned("1001") {
		t.Errorf("expected deleted book to be unpinned")
	}
	books, _ := store.ListCachedBooks()
	if len(books) != 0 {
		t.Errorf("expected 0 books after delete, got %d", len(books))
	}

	// Save again and test ClearCache
	_ = store.SaveBookDetail(b1)
	if err := store.ClearCache(); err != nil {
		t.Fatalf("ClearCache failed: %v", err)
	}
	books, _ = store.ListCachedBooks()
	if len(books) != 0 {
		t.Errorf("expected 0 books after clear, got %d", len(books))
	}
}
