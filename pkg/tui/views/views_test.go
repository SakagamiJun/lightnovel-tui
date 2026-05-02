package views

import (
	"testing"

	"lnr-core/pkg/model"
)

func TestCatalogWindowing(t *testing.T) {
	v := &CatalogView{
		height:    20, // visibleLines = 16
		flatItems: make([]flatChapterItem, 50),
	}

	for i := 0; i < 50; i++ {
		v.flatItems[i] = flatChapterItem{
			title: "章节",
		}
	}

	visible := v.visibleLines()
	if visible != 16 {
		t.Fatalf("expected visible lines 16, got %d", visible)
	}

	// 1. Initial position
	v.cursor = 0
	v.adjustOffset()
	if v.offset != 0 {
		t.Errorf("expected offset 0 at cursor 0, got %d", v.offset)
	}

	// 2. Cursor moves within first screen
	v.cursor = 10
	v.adjustOffset()
	if v.offset != 0 {
		t.Errorf("expected offset 0 at cursor 10, got %d", v.offset)
	}

	// 3. Cursor moves beyond first screen (e.g. index 20)
	v.cursor = 20
	v.adjustOffset()
	// offset should shift so cursor is visible: cursor - visible + 1 = 20 - 16 + 1 = 5
	expectedOffset := 20 - 16 + 1
	if v.offset != expectedOffset {
		t.Errorf("expected offset %d at cursor 20, got %d", expectedOffset, v.offset)
	}

	// 4. Cursor jumps to bottom (index 49)
	v.cursor = 49
	v.adjustOffset()
	maxOffset := 50 - 16 // 34
	if v.offset != maxOffset {
		t.Errorf("expected offset %d at cursor 49, got %d", maxOffset, v.offset)
	}

	// 5. Cursor jumps back to top
	v.cursor = 0
	v.adjustOffset()
	if v.offset != 0 {
		t.Errorf("expected offset 0 when jumping back to top, got %d", v.offset)
	}
}

func TestSearchWindowing(t *testing.T) {
	v := &SearchView{
		height:  27, // visibleCards = (27-7)/4 = 5
		results: make([]model.BookSummary, 30),
	}

	visible := v.visibleCards()
	if visible != 5 {
		t.Fatalf("expected visible cards 5, got %d", visible)
	}

	// Move cursor to 10
	v.cursor = 10
	v.adjustOffset()
	expectedOffset := 10 - 5 + 1 // 6
	if v.offset != expectedOffset {
		t.Errorf("expected offset %d, got %d", expectedOffset, v.offset)
	}

	// Move cursor backwards
	v.cursor = 4
	v.adjustOffset()
	if v.offset != 4 {
		t.Errorf("expected offset 4, got %d", v.offset)
	}
}
