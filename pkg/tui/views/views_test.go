package views

import (
	"fmt"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"lnr-core/pkg/epub"
	"lnr-core/pkg/model"
	"lnr-core/pkg/storage"
	"lnr-core/pkg/tui/common"
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
		height:  27, // visibleCards = (27 - 6) / 3 = 7
		results: make([]model.BookSummary, 30),
	}

	visible := v.visibleCards()
	if visible != 7 {
		t.Fatalf("expected visible cards 7, got %d", visible)
	}

	// Move cursor to 15
	v.cursor = 15
	v.adjustOffset()
	expectedOffset := 15 - 7 + 1 // 9
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
	if !strings.Contains(lines[0], "轻小说检索") {
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

func TestBookshelfPinningAndDeletion(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bookshelf-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to init storage: %v", err)
	}

	b1 := &model.BookDetail{BookSummary: model.BookSummary{ID: "1001", Title: "小说A", Author: "作者A"}}
	b2 := &model.BookDetail{BookSummary: model.BookSummary{ID: "1002", Title: "小说B", Author: "作者B"}}
	_ = store.SaveBookDetail(b1)
	_ = store.SaveBookDetail(b2)

	v := NewBookshelfView(store, nil)
	v.SetSize(80, 24)
	v.Reload()

	if len(v.books) != 2 {
		t.Fatalf("expected 2 books, got %d", len(v.books))
	}

	// 1. Initially book at cursor 0 is b1 (or b2)
	// Press 'p' to pin current book
	v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	currentBookID := v.books[v.cursor].ID
	if !store.IsBookPinned(currentBookID) {
		t.Errorf("expected book %s to be pinned in storage", currentBookID)
	}

	viewStr := v.View()
	if !strings.Contains(viewStr, "置顶") {
		t.Errorf("expected bookshelf view to render '置顶' badge")
	}

	// Line budget check
	lines := strings.Split(strings.TrimSuffix(viewStr, "\n"), "\n")
	if len(lines) > v.height {
		t.Errorf("expected lines <= %d, got %d", v.height, len(lines))
	}

	// 2. Press 'd' to initiate deletion
	v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if !v.confirmDelete {
		t.Errorf("expected confirmDelete to be true after pressing 'd'")
	}
	viewDel := v.View()
	if !strings.Contains(viewDel, "确认删除") {
		t.Errorf("expected view to show '确认删除' prompt")
	}
	delLines := strings.Split(strings.TrimSuffix(viewDel, "\n"), "\n")
	if len(delLines) > v.height {
		t.Errorf("expected confirm delete lines <= %d, got %d", v.height, len(delLines))
	}

	// 3. Cancel with 'n'
	v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if v.confirmDelete {
		t.Errorf("expected confirmDelete to be false after pressing 'n'")
	}

	// 4. Delete with 'd' then 'y'
	v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if len(v.books) != 1 {
		t.Fatalf("expected 1 book remaining after delete, got %d", len(v.books))
	}
}

func TestViewsDownloadAndExportCallbacks(t *testing.T) {
	// 1. Bookshelf export callback
	bExportID := ""
	bExportVol := -999
	bv := &BookshelfView{
		books: []model.BookDetail{{BookSummary: model.BookSummary{ID: "book_shelf_1"}}},
	}
	bv.SetOnExport(func(bookID string, volumeIndex int) tea.Cmd {
		bExportID = bookID
		bExportVol = volumeIndex
		return nil
	})
	bv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if bExportID != "book_shelf_1" || bExportVol != common.ExportModeFullBook {
		t.Errorf("expected bookshelf full export callback, got %q vol=%d", bExportID, bExportVol)
	}
	bv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	if bExportID != "book_shelf_1" || bExportVol != common.ExportModeAllVolumesSeparate {
		t.Errorf("expected bookshelf split export callback, got %q vol=%d", bExportID, bExportVol)
	}

	// 2. Search download and export callback
	sDownloadID := ""
	sExportID := ""
	sExportVol := -999
	sv := &SearchView{
		results: []model.BookSummary{{ID: "book_search_1"}},
	}
	sv.input.Blur() // ensure results have focus
	sv.SetOnDownload(func(bookID string) tea.Cmd {
		sDownloadID = bookID
		return nil
	})
	sv.SetOnExport(func(bookID string, volumeIndex int) tea.Cmd {
		sExportID = bookID
		sExportVol = volumeIndex
		return nil
	})
	sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if sDownloadID != "book_search_1" {
		t.Errorf("expected search download callback with 'book_search_1', got %q", sDownloadID)
	}
	sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if sExportID != "book_search_1" || sExportVol != common.ExportModeFullBook {
		t.Errorf("expected search full export callback, got %q vol=%d", sExportID, sExportVol)
	}
	sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if sExportID != "book_search_1" || sExportVol != common.ExportModeAllVolumesSeparate {
		t.Errorf("expected search split export callback, got %q vol=%d", sExportID, sExportVol)
	}

	// 3. Catalog volume download and full export callback
	cDownloadVol := -1
	cDownloadBookID := ""
	cExportVol := -999
	cExportBookID := ""
	cv := &CatalogView{
		bookID: "book_cat_1",
		flatItems: []flatChapterItem{
			{isVolume: true, volIndex: 2, volTitle: "第二卷"},
		},
	}
	cv.SetOnDownload(func(bookID string, volumeIndex int) tea.Cmd {
		cDownloadBookID = bookID
		cDownloadVol = volumeIndex
		return nil
	})
	cv.SetOnExport(func(bookID string, volumeIndex int) tea.Cmd {
		cExportBookID = bookID
		cExportVol = volumeIndex
		return nil
	})

	// Press 'd' on volume
	cv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if cDownloadBookID != "book_cat_1" || cDownloadVol != 2 {
		t.Errorf("expected catalog volume download for vol 2, got book=%s vol=%d", cDownloadBookID, cDownloadVol)
	}

	// Press 'e' on volume
	cv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if cExportBookID != "book_cat_1" || cExportVol != 2 {
		t.Errorf("expected catalog volume export for vol 2, got book=%s vol=%d", cExportBookID, cExportVol)
	}

	// Press 's' for split export all volumes
	cv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cExportBookID != "book_cat_1" || cExportVol != common.ExportModeAllVolumesSeparate {
		t.Errorf("expected catalog split export (vol %d), got book=%s vol=%d", common.ExportModeAllVolumesSeparate, cExportBookID, cExportVol)
	}

	// Press 'E' for export modal, then press Enter
	cv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	if !cv.IsExportModalOpen() {
		t.Errorf("expected export modal to be open after pressing E")
	}
	cv.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cExportBookID != "book_cat_1" || cExportVol != common.ExportModeFullBook {
		t.Errorf("expected catalog full book export (vol 0), got book=%s vol=%d", cExportBookID, cExportVol)
	}
}

func TestLongBookTitleDisplayWithoutPrematureTruncation(t *testing.T) {
	longTitle := "普通攻击是全体二连击这样的妈妈你喜欢吗" // 20 Chinese characters = 40 width

	// Bookshelf view on 80-width terminal
	bv := &BookshelfView{
		width:  80,
		height: 24,
		books: []model.BookDetail{
			{BookSummary: model.BookSummary{ID: "1001", Title: longTitle, Author: "井中大吉"}},
		},
		loaded: true,
	}
	bOut := bv.View()
	if !strings.Contains(bOut, longTitle) {
		t.Errorf("expected bookshelf to contain full title without premature truncation: %s", bOut)
	}

	// Search view on 80-width terminal
	sv := &SearchView{
		width:   80,
		height:  24,
		results: []model.BookSummary{{ID: "1001", Title: longTitle, Author: "井中大吉"}},
	}
	sv.input.Blur()
	sOut := sv.View()
	if !strings.Contains(sOut, longTitle) {
		t.Errorf("expected search to contain full title without premature truncation: %s", sOut)
	}
}

func TestReaderIllustrationModal(t *testing.T) {
	rv := &ReaderView{
		width:  80,
		height: 24,
	}
	rv.SetSize(80, 24)

	// In normal view without reader, 'i' should not open modal
	rv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if rv.showImageModal {
		t.Errorf("expected modal not to open when reader is nil")
	}

	// Mock state with modal active
	rv.showImageModal = true
	rv.imageIndex = 0
	rv.imageLoading = false
	rv.imagePath = "/tmp/test.png"

	modalView := rv.View()
	if !strings.Contains(modalView, "[插图查看器]") {
		t.Errorf("expected modal view to contain '[插图查看器]', got: %s", modalView)
	}

	// Press Esc to exit modal
	rv.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if rv.showImageModal {
		t.Errorf("expected modal to close on Esc")
	}
}

func TestExploreViewModeToggle(t *testing.T) {
	ev := NewExploreView(nil, nil, nil)
	ev.SetSize(80, 24)

	if ev.subTab != SubTabHot {
		t.Errorf("expected initial subTab SubTabHot, got %v", ev.subTab)
	}

	// Press 'l' or 'right' to cycle to SubTabAnime
	ev.Update(tea.KeyMsg{Type: tea.KeyRight})
	if ev.subTab != SubTabAnime {
		t.Errorf("expected SubTabAnime after right, got %v", ev.subTab)
	}

	// Press 't' to cycle to SubTabUpdate
	ev.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if ev.subTab != SubTabUpdate {
		t.Errorf("expected SubTabUpdate after 't', got %v", ev.subTab)
	}

	// View output should contain mode titles and badges
	out := ev.View()
	if !strings.Contains(out, "[热门榜]") || !strings.Contains(out, "[动画化]") || !strings.Contains(out, "[文库分类]") {
		t.Errorf("expected view to contain explore badges including [文库分类], got: %s", out)
	}

	// Switch to SubTabPublishers
	ev.subTab = SubTabPublishers
	ev.pubIndex = 0
	outPub := ev.View()
	if !strings.Contains(outPub, "电击文库") {
		t.Errorf("expected view to display 电击文库, got: %s", outPub)
	}

	// Press 't' to cycle to next publisher
	ev.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if ev.pubIndex != 1 {
		t.Errorf("expected pubIndex 1 after 't', got %d", ev.pubIndex)
	}
	outPub2 := ev.View()
	if !strings.Contains(outPub2, "富士见文库") {
		t.Errorf("expected view to display 富士见文库, got: %s", outPub2)
	}
}

func TestBookshelfSortingAndUpdates(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	b1 := &model.BookDetail{BookSummary: model.BookSummary{ID: "b1", Title: "刀剑神域"}}
	b2 := &model.BookDetail{BookSummary: model.BookSummary{ID: "b2", Title: "加速世界"}}
	_ = store.SaveBookDetail(b1)
	_ = store.SaveBookDetail(b2)

	bv := NewBookshelfView(store, nil)
	bv.SetSize(80, 24)
	bv.Reload()

	if bv.sortCriteria != storage.SortByLastRead {
		t.Errorf("expected default sort SortByLastRead, got %v", bv.sortCriteria)
	}

	// Press 'o' to cycle sort criteria
	bv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if bv.sortCriteria != storage.SortByLastUpdated {
		t.Errorf("expected SortByLastUpdated after 'o', got %v", bv.sortCriteria)
	}

	// Set update badges
	bv.updatesMap = map[string]int{"b1": 3}
	out := bv.View()
	if !strings.Contains(out, "更新:+3章") {
		t.Errorf("expected view to contain '更新:+3章', got: %s", out)
	}
}

func TestExploreViewLineBudgetAndWindowing(t *testing.T) {
	ev := NewExploreView(nil, nil, nil)
	ev.SetSize(80, 21) // 21 lines content height

	// Add 30 books
	books := make([]model.BookSummary, 30)
	for i := 0; i < 30; i++ {
		books[i] = model.BookSummary{
			ID:          fmt.Sprintf("b%d", i),
			Title:       fmt.Sprintf("轻小说作品 %d", i),
			Author:      "作者",
			Publisher:   "电击文库",
			WordCount:   500000,
			Description: "小说简介内容...",
		}
	}
	ev.SetResults(books)

	view := ev.View()
	lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")
	// View should produce 1 (Header) + 1 (Badges) + 1 (Detail) + 1 (Stats) + 1 (TopInd) + 5*3 (cards) + 1 (BotInd) = 21 lines
	if len(lines) != 21 {
		t.Fatalf("expected 21 lines for explore view, got %d", len(lines))
	}

	// Verify windowing
	visible := ev.visibleCards()
	if visible != 5 {
		t.Fatalf("expected 5 visible cards, got %d", visible)
	}

	// Move cursor to 12
	ev.cursor = 12
	ev.adjustOffset()
	expectedOffset := 12 - 5 + 1 // 8
	if ev.offset != expectedOffset {
		t.Errorf("expected offset %d, got %d", expectedOffset, ev.offset)
	}
}

func TestBookshelfStatsModalAndResume(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lnr-test-stats-view-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	_ = store.RecordReading("book_1", "魔王学院的不适任者", "chap_2", "第二章", 300, 2500, 15, 100)

	var continuedBookID, continuedChapID string
	bv := NewBookshelfView(store, nil)
	bv.SetOnContinueReading(func(bookID, chapterID string) tea.Cmd {
		continuedBookID = bookID
		continuedChapID = chapterID
		return nil
	})
	bv.SetSize(80, 24)

	// Initially stats modal is closed
	if bv.IsShowingStats() {
		t.Fatal("expected stats modal to be initially closed")
	}

	// Press 's' to open stats modal
	bv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if !bv.IsShowingStats() {
		t.Fatal("expected stats modal to be open after pressing 's'")
	}

	// Render view when modal is open
	viewStr := bv.View()
	if !strings.Contains(viewStr, "个人阅读统计与打卡热力图") {
		t.Errorf("expected view to contain header, got: %s", viewStr)
	}
	if !strings.Contains(viewStr, "最近 12 周打卡记录:") {
		t.Errorf("expected view to contain heatmap title, got: %s", viewStr)
	}
	if !strings.Contains(viewStr, "魔王学院的不适任者") {
		t.Errorf("expected view to contain recent book title, got: %s", viewStr)
	}

	// Press Enter inside stats modal to resume reading
	bv.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if bv.IsShowingStats() {
		t.Fatal("expected stats modal to be closed after Enter")
	}
	if continuedBookID != "book_1" || continuedChapID != "chap_2" {
		t.Errorf("expected resume to open book_1/chap_2, got %q/%q", continuedBookID, continuedChapID)
	}

	// Reopen with 's' and close with 'esc'
	bv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if !bv.IsShowingStats() {
		t.Fatal("expected stats modal to be open")
	}
	bv.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if bv.IsShowingStats() {
		t.Fatal("expected stats modal to be closed after esc")
	}

	// Press 'c' directly on bookshelf to resume reading
	continuedBookID, continuedChapID = "", ""
	bv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if continuedBookID != "book_1" || continuedChapID != "chap_2" {
		t.Errorf("expected direct resume with 'c' to open book_1/chap_2, got %q/%q", continuedBookID, continuedChapID)
	}
}

func TestReaderChapterNavigation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lnr-test-reader-nav-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	cat := &model.BookCatalog{
		BookID: "book_test",
		Volumes: []model.Volume{
			{
				Title: "第一卷",
				Chapters: []model.Chapter{
					{ID: "ch_1", Title: "第一章"},
					{ID: "ch_2", Title: "第二章"},
					{ID: "ch_3", Title: "第三章"},
				},
			},
		},
	}
	_ = store.SaveCatalog(cat)
	_ = store.SaveChapter(&model.ChapterContent{
		BookID:   "book_test",
		ID:       "ch_2",
		Title:    "第二章",
		Elements: []model.ContentElement{{Type: model.ContentTypeText, Text: "内容2"}},
	})
	_ = store.SaveChapter(&model.ChapterContent{
		BookID:   "book_test",
		ID:       "ch_1",
		Title:    "第一章",
		Elements: []model.ContentElement{{Type: model.ContentTypeText, Text: "内容1"}},
	})
	_ = store.SaveChapter(&model.ChapterContent{
		BookID:   "book_test",
		ID:       "ch_3",
		Title:    "第三章",
		Elements: []model.ContentElement{{Type: model.ContentTypeText, Text: "内容3"}},
	})

	rv := NewReaderView(store, nil)
	rv.SetSize(80, 24)
	cmd := rv.OpenChapter("book_test", "ch_2")
	if cmd != nil {
		msg := cmd()
		rv.Update(msg)
	}

	if rv.chapterID != "ch_2" {
		t.Fatalf("expected chapter ch_2, got %s", rv.chapterID)
	}

	// Press '[' to go to prev chapter
	_, nextCmd := rv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	if nextCmd == nil {
		t.Fatal("expected command to load prev chapter")
	}

	// Press ']' to go to next chapter
	_, nextCmd2 := rv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	if nextCmd2 == nil {
		t.Fatal("expected command to load next chapter")
	}
}

func TestSettingsRulesManagement(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lnr-test-settings-rules-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	sv := NewSettingsView(store)
	sv.SetSize(80, 24)

	// Move cursor to Item 8 (Rules management)
	for i := 0; i < 8; i++ {
		sv.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if sv.cursor != 8 {
		t.Fatalf("expected cursor at 8, got %d", sv.cursor)
	}

	// Press Enter to open rules view
	sv.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !sv.IsShowingRules() {
		t.Fatal("expected IsShowingRules to be true after Enter on item 8")
	}

	// Render rules view
	viewStr := sv.View()
	if !strings.Contains(viewStr, "排版规范化与正则清洗规则") {
		t.Errorf("expected view to contain rules header, got: %s", viewStr)
	}
	if !strings.Contains(viewStr, "规范省略号") {
		t.Errorf("expected view to contain default rule, got: %s", viewStr)
	}

	// Press space to toggle rule 0
	origState := sv.rules[0].Enabled
	sv.Update(tea.KeyMsg{Type: tea.KeySpace})
	if sv.rules[0].Enabled == origState {
		t.Errorf("expected rule 0 enabled state to change from %v", origState)
	}

	// Press Esc to exit rules view
	sv.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if sv.IsShowingRules() {
		t.Fatal("expected IsShowingRules to be false after Esc")
	}
}

func TestBookshelfGroupSwitchingAndMoving(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lnr-test-bookshelf-groups-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	book1 := &model.BookDetail{
		BookSummary: model.BookSummary{
			ID:    "b_1",
			Title: "刀剑神域",
		},
	}
	book2 := &model.BookDetail{
		BookSummary: model.BookSummary{
			ID:    "b_2",
			Title: "加速世界",
		},
	}
	_ = store.SaveBookDetail(book1)
	_ = store.SaveBookDetail(book2)

	bv := NewBookshelfView(store, nil)
	bv.SetSize(80, 24)
	bv.Reload()

	// Initially in "全部" (All), should show 2 books
	if len(bv.books) != 2 {
		t.Fatalf("expected 2 books in 'all' shelf, got %d", len(bv.books))
	}

	// Press 'g' to cycle to "在读" (Reading) -> should have 0 books
	bv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if bv.activeShelfIdx != 1 {
		t.Fatalf("expected activeShelfIdx 1, got %d", bv.activeShelfIdx)
	}
	if len(bv.books) != 0 {
		t.Fatalf("expected 0 books in 'reading' shelf initially, got %d", len(bv.books))
	}

	// Render view on empty shelf -> verify prompt is shown
	emptyView := bv.View()
	if !strings.Contains(emptyView, "暂无藏书") {
		t.Errorf("expected view to indicate shelf is empty, got: %s", emptyView)
	}

	// Press 'G' to cycle back to "全部" (All)
	bv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	if bv.activeShelfIdx != 0 {
		t.Fatalf("expected activeShelfIdx 0, got %d", bv.activeShelfIdx)
	}
	if len(bv.books) != 2 {
		t.Fatalf("expected 2 books back in 'all', got %d", len(bv.books))
	}

	// Press 'm' to open move modal
	bv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if !bv.IsMovingBook() {
		t.Fatal("expected IsMovingBook to be true after pressing 'm'")
	}

	// Render move modal
	modalView := bv.View()
	if !strings.Contains(modalView, "移动藏书分组") {
		t.Errorf("expected modal to contain header, got: %s", modalView)
	}
	if !strings.Contains(modalView, "在读") {
		t.Errorf("expected modal to list '在读' shelf, got: %s", modalView)
	}

	// Press '1' to move current book ("刀剑神域") to "在读"
	bv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	if bv.IsMovingBook() {
		t.Fatal("expected move modal to be closed after choosing shelf")
	}

	// Now press 'g' to switch to "在读" -> should now have 1 book ("刀剑神域")!
	bv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	if len(bv.books) != 1 || bv.books[0].ID != "b_1" {
		t.Fatalf("expected 1 book (b_1) in 'reading' shelf, got %d books", len(bv.books))
	}
}

func TestCatalogVolumeSelectionAndExportModal(t *testing.T) {
	cv := NewCatalogView(nil, nil, nil)
	cv.SetSize(80, 24)
	cv.bookID = "book_test_export"
	cv.detail = &model.BookDetail{
		BookSummary: model.BookSummary{
			ID:    "book_test_export",
			Title: "魔法禁书目录",
		},
	}
	cv.catalog = &model.BookCatalog{
		BookID: "book_test_export",
		Volumes: []model.Volume{
			{ID: "v1", Title: "第一卷", Chapters: []model.Chapter{{ID: "c1", Title: "第一章"}}},
			{ID: "v2", Title: "第二卷", Chapters: []model.Chapter{{ID: "c2", Title: "第二章"}}},
		},
	}
	cv.flattenItems()

	var exportedOpt epub.ExportOption
	var exportedBookID string
	cv.SetOnExportWithOptions(func(bookID string, opt epub.ExportOption) tea.Cmd {
		exportedBookID = bookID
		exportedOpt = opt
		return nil
	})

	// Cursor is at 0 (vol 1). Press space to toggle selection on vol 1
	cv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !cv.selectedVols[1] {
		t.Errorf("expected volume 1 to be selected")
	}

	// Move cursor down to vol 2 (index 2 in flatItems) and select it
	cv.cursor = 2
	cv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if !cv.selectedVols[2] {
		t.Errorf("expected volume 2 to be selected")
	}

	// Verify view rendering contains [x] checkmarks
	viewOut := cv.View()
	if !strings.Contains(viewOut, "[x]") {
		t.Errorf("expected view to contain [x] checkmark, got: %s", viewOut)
	}
	if !strings.Contains(viewOut, "已勾选 2 卷") {
		t.Errorf("expected view to indicate 2 volumes selected, got: %s", viewOut)
	}

	// Press 'E' to open export modal
	cv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	if !cv.IsExportModalOpen() {
		t.Fatal("expected export modal to be open")
	}

	// Modal view should show options
	modalView := cv.View()
	if !strings.Contains(modalView, "EPUB 导出选项设置") {
		t.Errorf("expected modal to contain header, got: %s", modalView)
	}
	if !strings.Contains(modalView, "所选分卷 (2卷)") {
		t.Errorf("expected modal to show selected volumes scope, got: %s", modalView)
	}

	// Move focus to row 1 (images) and toggle to pure-text
	cv.Update(tea.KeyMsg{Type: tea.KeyDown})
	cv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	if cv.exportIncludeImages {
		t.Errorf("expected exportIncludeImages to be toggled to false (pure text)")
	}

	// Press Enter to confirm and export
	cv.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cv.IsExportModalOpen() {
		t.Errorf("expected modal to close after Enter")
	}

	if exportedBookID != "book_test_export" {
		t.Errorf("expected exported bookID 'book_test_export', got %s", exportedBookID)
	}
	if len(exportedOpt.VolumeIndexes) != 2 || exportedOpt.VolumeIndexes[0] != 1 || exportedOpt.VolumeIndexes[1] != 2 {
		t.Errorf("expected volume indexes [1, 2], got %v", exportedOpt.VolumeIndexes)
	}
	if !exportedOpt.NoImages {
		t.Errorf("expected NoImages to be true")
	}
}
