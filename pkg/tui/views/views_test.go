package views

import (
	"strings"
	"testing"

	"lnr-core/pkg/model"
)

func TestCatalogWindowing(t *testing.T) {
	v := &CatalogView{
		height:    20, // visibleLines = 20 - 3 = 17
		flatItems: make([]flatChapterItem, 50),
	}

	for i := 0; i < 50; i++ {
		v.flatItems[i] = flatChapterItem{
			title: "章节",
		}
	}

	visible := v.visibleLines()
	if visible != 17 {
		t.Fatalf("expected visible lines 17, got %d", visible)
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
	// offset should shift so cursor is visible: cursor - visible + 1 = 20 - 17 + 1 = 4
	expectedOffset := 20 - 17 + 1
	if v.offset != expectedOffset {
		t.Errorf("expected offset %d at cursor 20, got %d", expectedOffset, v.offset)
	}

	// 4. Cursor jumps to bottom (index 49)
	v.cursor = 49
	v.adjustOffset()
	maxOffset := 50 - 17 // 33
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
		height:  27, // visibleCards = (27 - 6) / 2 = 10
		results: make([]model.BookSummary, 30),
	}

	visible := v.visibleCards()
	if visible != 10 {
		t.Fatalf("expected visible cards 10, got %d", visible)
	}

	// Move cursor to 15
	v.cursor = 15
	v.adjustOffset()
	expectedOffset := 15 - 10 + 1 // 6
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

func TestSearchViewLineBudget(t *testing.T) {
	// Test at terminal height = 21 (which corresponds to 24-line terminal: 24 - 3 = 21)
	v := &SearchView{
		width:   80,
		height:  21,
		results: make([]model.BookSummary, 30),
	}
	for i := 0; i < 30; i++ {
		v.results[i] = model.BookSummary{
			ID:          "1001",
			Title:       "刀剑神域",
			Author:      "川原砾",
			Publisher:   "电击文库",
			WordCount:   2800000,
			Description: "无法完全攻略就无法离开游戏，Game Over也等于宣告玩家的“死亡”。",
		}
	}

	// Render output
	out := v.View()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) > v.height {
		t.Fatalf("SearchView.View() produced %d lines, strictly exceeding max height %d", len(lines), v.height)
	}

	// Verify top elements are always at the top
	if !strings.Contains(lines[0], "在线小说检索") {
		t.Errorf("expected line 0 to contain search title, got: %s", lines[0])
	}
}

func TestBookshelfViewLineBudget(t *testing.T) {
	v := &BookshelfView{
		width:  80,
		height: 21,
		loaded: true,
		books:  make([]model.BookDetail, 30),
	}
	for i := 0; i < 30; i++ {
		v.books[i] = model.BookDetail{
			BookSummary: model.BookSummary{
				ID:          "1001",
				Title:       "刀剑神域",
				Author:      "川原砾",
				Publisher:   "电击文库",
				WordCount:   2800000,
				Description: "无法完全攻略就无法离开游戏，Game Over也等于宣告玩家的“死亡”。",
			},
		}
	}

	out := v.View()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) > v.height {
		t.Fatalf("BookshelfView.View() produced %d lines, strictly exceeding max height %d", len(lines), v.height)
	}

	if !strings.Contains(lines[0], "本地藏书") {
		t.Errorf("expected line 0 to contain bookshelf title, got: %s", lines[0])
	}
}

func TestCatalogViewLineBudget(t *testing.T) {
	v := &CatalogView{
		width:  80,
		height: 21,
		detail: &model.BookDetail{
			BookSummary: model.BookSummary{
				Title: "刀剑神域",
			},
		},
		flatItems: make([]flatChapterItem, 50),
	}
	for i := 0; i < 50; i++ {
		v.flatItems[i] = flatChapterItem{
			title: "第 1 卷 艾恩葛朗特 第一章",
		}
	}

	out := v.View()
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) > v.height {
		t.Fatalf("CatalogView.View() produced %d lines, strictly exceeding max height %d", len(lines), v.height)
	}

	if !strings.Contains(lines[0], "刀剑神域") {
		t.Errorf("expected line 0 to contain book title, got: %s", lines[0])
	}
}
