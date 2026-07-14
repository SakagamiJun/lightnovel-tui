package storage

import (
	"os"
	"testing"
)

func TestBookshelfStorage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lnr-test-shelves-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := NewStorage(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initial load should return DefaultBookshelves
	shelves, err := store.LoadShelves()
	if err != nil {
		t.Fatalf("failed to load shelves: %v", err)
	}
	if len(shelves) != len(DefaultBookshelves) {
		t.Errorf("expected %d default shelves, got %d", len(DefaultBookshelves), len(shelves))
	}

	// 2. Move book to "reading"
	if err := store.MoveBookToShelf("book_100", "reading"); err != nil {
		t.Fatalf("failed to move book to reading: %v", err)
	}

	shelves, _ = store.LoadShelves()
	found := false
	for _, sh := range shelves {
		if sh.ID == "reading" {
			for _, id := range sh.BookIDs {
				if id == "book_100" {
					found = true
					break
				}
			}
		}
	}
	if !found {
		t.Fatal("expected book_100 to be in reading shelf")
	}

	// 3. Move book to "completed" - should remove from "reading"
	if err := store.MoveBookToShelf("book_100", "completed"); err != nil {
		t.Fatalf("failed to move book to completed: %v", err)
	}

	shelves, _ = store.LoadShelves()
	for _, sh := range shelves {
		if sh.ID == "reading" {
			for _, id := range sh.BookIDs {
				if id == "book_100" {
					t.Errorf("book_100 should have been removed from reading shelf")
				}
			}
		}
		if sh.ID == "completed" {
			inComp := false
			for _, id := range sh.BookIDs {
				if id == "book_100" {
					inComp = true
				}
			}
			if !inComp {
				t.Errorf("expected book_100 to be in completed shelf")
			}
		}
	}

	// 4. Add custom shelf
	customShelf, err := store.AddCustomShelf("神作必看")
	if err != nil {
		t.Fatalf("failed to add custom shelf: %v", err)
	}
	if customShelf.Name != "神作必看" {
		t.Errorf("expected custom shelf name '神作必看', got %q", customShelf.Name)
	}
}
