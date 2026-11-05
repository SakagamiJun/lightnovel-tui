package views

import (
	"bytes"
	"fmt"
	stdimage "image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/SakagamiJun/lightnovel-tui/pkg/epub"
	termimage "github.com/SakagamiJun/lightnovel-tui/pkg/image"
	"github.com/SakagamiJun/lightnovel-tui/pkg/model"
	"github.com/SakagamiJun/lightnovel-tui/pkg/reader"
	"github.com/SakagamiJun/lightnovel-tui/pkg/storage"
	"github.com/SakagamiJun/lightnovel-tui/pkg/tui/common"
	"github.com/SakagamiJun/lightnovel-tui/pkg/tui/theme"
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

func TestBookshelfCoverPaneAbsentWithoutCover(t *testing.T) {
	t.Setenv("LNR_IMAGE_PROTOCOL", "kitty")
	store, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	bv := NewBookshelfView(store, nil)
	bv.SetSize(100, 30)
	bv.SetTerminalOutput(termimage.NewLockedWriter(&out))
	bv.SetBooks([]model.BookDetail{{
		BookSummary: model.BookSummary{ID: "book_no_cover", Title: "无封面藏书"},
	}})
	if cmd := bv.Activate(); cmd != nil {
		t.Fatalf("expected no cover load for a book without a cover URL or file")
	}
	if got := bv.bookshelfCoverPaneWidth(); got != 0 {
		t.Fatalf("cover pane width = %d, want 0", got)
	}
	view := bv.View()
	if strings.Contains(view, string(rune(0x10EEEE))) || strings.Contains(view, "封面加载中") {
		t.Errorf("view should not contain a cover")
	}
	for i, line := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(line); w > 100 {
			t.Errorf("line %d width = %d, exceeds bookshelf width 100", i, w)
		}
	}
}

func TestBookshelfCoverPaneLoadAndRender(t *testing.T) {
	t.Setenv("LNR_IMAGE_PROTOCOL", "kitty")
	store, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const bookID = "book_shelf_cover"
	writeTestCover(t, store.ImagePath(bookID, "cover.png"), 201, 303)

	var out bytes.Buffer
	bv := NewBookshelfView(store, nil)
	bv.SetSize(100, 30)
	bv.SetTerminalOutput(termimage.NewLockedWriter(&out))
	bv.SetBooks([]model.BookDetail{{
		BookSummary: model.BookSummary{ID: bookID, Title: "书架封面"},
	}})

	cmd := bv.Activate()
	if cmd == nil {
		t.Fatalf("expected a bookshelf cover load command")
	}
	loaded, ok := cmd().(CoverLoadedMsg)
	if !ok {
		t.Fatalf("unexpected cover message type")
	}
	if loaded.err != nil {
		t.Fatalf("cover load failed: %v", loaded.err)
	}
	bv.Update(loaded)

	if got := bv.bookshelfCoverPaneWidth(); got == 0 {
		t.Fatalf("expected a visible cover pane")
	}
	view := bv.View()
	if !strings.Contains(view, "本地藏书库") {
		t.Errorf("expected the bookshelf title")
	}
	if got, want := strings.Count(view, string(rune(0x10EEEE))), bv.cover.cols*bv.cover.rows; got != want {
		t.Errorf("placeholder count = %d, want %d", got, want)
	}
	lines := strings.Split(view, "\n")
	if len(lines) > 30 {
		t.Errorf("bookshelf rendered %d lines, exceeds height 30", len(lines))
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w > 100 {
			t.Errorf("line %d width = %d, exceeds bookshelf width 100", i, w)
		}
	}
	if !strings.Contains(out.String(), "\x1b_Ga=T,f=100") {
		t.Errorf("terminal output missing Kitty cover transmission")
	}
}

func TestBookshelfCoverLineBudget(t *testing.T) {
	t.Setenv("LNR_IMAGE_PROTOCOL", "kitty")
	store, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const bookID = "book_shelf_many"
	writeTestCover(t, store.ImagePath(bookID, "cover.png"), 201, 303)
	books := make([]model.BookDetail, 0, 50)
	for i := 0; i < 50; i++ {
		books = append(books, model.BookDetail{BookSummary: model.BookSummary{
			ID:          fmt.Sprintf("book_%d", i),
			Title:       fmt.Sprintf("藏书 %d", i),
			Author:      "作者",
			Description: "简介内容",
		}})
	}
	books[0].ID = bookID

	var out bytes.Buffer
	bv := NewBookshelfView(store, nil)
	bv.SetSize(80, 21)
	bv.SetTerminalOutput(termimage.NewLockedWriter(&out))
	bv.SetBooks(books)

	cmd := bv.Activate()
	if cmd == nil {
		t.Fatalf("expected a cover load command")
	}
	loaded, ok := cmd().(CoverLoadedMsg)
	if !ok || loaded.err != nil {
		t.Fatalf("cover load failed: %+v", loaded)
	}
	bv.Update(loaded)

	view := bv.View()
	lines := strings.Split(view, "\n")
	if len(lines) > 21 {
		t.Errorf("bookshelf rendered %d lines with cover, exceeds height 21", len(lines))
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w > 80 {
			t.Errorf("line %d width = %d, exceeds bookshelf width 80", i, w)
		}
	}
	if got, want := strings.Count(view, string(rune(0x10EEEE))), bv.cover.cols*bv.cover.rows; got != want {
		t.Errorf("placeholder count = %d, want %d", got, want)
	}
}

func TestBookshelfCoverGrowsOnWideTerminal(t *testing.T) {
	t.Setenv("LNR_IMAGE_PROTOCOL", "kitty")
	store, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const bookID = "book_shelf_wide"
	writeTestCover(t, store.ImagePath(bookID, "cover.png"), 201, 303)

	bv := NewBookshelfView(store, nil)
	bv.SetSize(160, 40)
	bv.SetTerminalOutput(termimage.NewLockedWriter(&bytes.Buffer{}))
	bv.SetBooks([]model.BookDetail{{
		BookSummary: model.BookSummary{ID: bookID, Title: "宽屏封面"},
	}})

	cmd := bv.Activate()
	if cmd == nil {
		t.Fatalf("expected a cover load command")
	}
	loaded, ok := cmd().(CoverLoadedMsg)
	if !ok || loaded.err != nil {
		t.Fatalf("cover load failed: %+v", loaded)
	}
	bv.Update(loaded)

	if got := bv.bookshelfCoverPaneWidth(); got <= bookshelfCoverMinPane {
		t.Errorf("wide terminal pane width = %d, want > %d", got, bookshelfCoverMinPane)
	}
	if bv.cover.cols <= 22 {
		t.Errorf("wide terminal cover grid width = %d, want > 22", bv.cover.cols)
	}
	view := bv.View()
	for i, line := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(line); w > 160 {
			t.Errorf("line %d width = %d, exceeds bookshelf width 160", i, w)
		}
	}
}

func TestCatalogCoverPaneAbsentWithoutCover(t *testing.T) {
	t.Setenv("LNR_IMAGE_PROTOCOL", "kitty")
	store, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const bookID = "book_no_cover"
	detail := &model.BookDetail{BookSummary: model.BookSummary{ID: bookID, Title: "无封面书籍"}}
	catalog := &model.BookCatalog{BookID: bookID, Volumes: []model.Volume{{
		ID: "v1", Title: "第一卷", Chapters: []model.Chapter{{ID: "c1", Title: "第一章"}},
	}}}

	var out bytes.Buffer
	cv := NewCatalogView(store, nil, nil)
	cv.SetSize(100, 30)
	cv.SetTerminalOutput(termimage.NewLockedWriter(&out))
	cv.bookID = bookID
	_, cmd := cv.Update(catalogResultMsg{detail: detail, catalog: catalog})
	if cmd != nil {
		t.Fatalf("expected no cover command for a book without a cover URL or file")
	}
	if got := cv.coverPaneWidth(); got != 0 {
		t.Fatalf("cover pane width = %d, want 0", got)
	}
	view := cv.View()
	if strings.Contains(view, string(rune(0x10EEEE))) {
		t.Errorf("view should not contain Kitty placeholders")
	}
	if strings.Contains(view, "封面加载中") {
		t.Errorf("view should not contain a cover placeholder")
	}
	for i, line := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(line); w > 100 {
			t.Errorf("line %d width = %d, exceeds catalog width 100", i, w)
		}
	}
}

func TestCatalogCoverLoadAndRender(t *testing.T) {
	t.Setenv("LNR_IMAGE_PROTOCOL", "kitty")
	store, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const bookID = "book_with_cover"
	detail := &model.BookDetail{BookSummary: model.BookSummary{
		ID:       bookID,
		Title:    "封面测试",
		CoverURL: "http://example.com/cover.jpg",
	}}
	if err := store.SaveBookDetail(detail); err != nil {
		t.Fatal(err)
	}
	catalog := &model.BookCatalog{BookID: bookID, Volumes: []model.Volume{
		{ID: "v1", Title: "第一卷", Chapters: []model.Chapter{{ID: "c1", Title: "第一章"}}},
		{ID: "v2", Title: "第二卷", Chapters: []model.Chapter{{ID: "c2", Title: "第二章"}}},
	}}
	if err := store.SaveCatalog(catalog); err != nil {
		t.Fatal(err)
	}
	writeTestCover(t, store.ImagePath(bookID, "cover.png"), 120, 180)

	var out bytes.Buffer
	lw := termimage.NewLockedWriter(&out)
	cv := NewCatalogView(store, nil, nil)
	cv.SetSize(100, 30)
	cv.SetTerminalOutput(lw)
	cv.bookID = bookID

	_, cmd := cv.Update(catalogResultMsg{detail: detail, catalog: catalog})
	if cmd == nil {
		t.Fatalf("expected a cover load command")
	}
	msg := cmd()
	loaded, ok := msg.(CoverLoadedMsg)
	if !ok {
		t.Fatalf("unexpected cover message type %T", msg)
	}
	if loaded.err != nil {
		t.Fatalf("cover load failed: %v", loaded.err)
	}
	if !loaded.kitty || loaded.imageID == 0 || loaded.cols <= 0 || loaded.rows <= 0 {
		t.Fatalf("expected a Kitty cover transmission, got %+v", loaded)
	}
	if !strings.Contains(out.String(), "\x1b_Ga=T,f=100") || !strings.Contains(out.String(), "U=1") {
		t.Errorf("terminal output missing Kitty cover transmission: %q", out.String())
	}

	cv.Update(loaded)
	if cv.coverPaneWidth() != catalogCoverPaneWidth {
		t.Fatalf("cover pane width = %d, want %d", cv.coverPaneWidth(), catalogCoverPaneWidth)
	}
	if cv.cover.imageID != loaded.imageID {
		t.Errorf("cover image id = %d, want %d", cv.cover.imageID, loaded.imageID)
	}

	view := cv.View()
	wantPlaceholders := loaded.cols * loaded.rows
	if got := strings.Count(view, string(rune(0x10EEEE))); got != wantPlaceholders {
		t.Errorf("placeholder count = %d, want %d", got, wantPlaceholders)
	}
	lines := strings.Split(view, "\n")
	if len(lines) > 30 {
		t.Errorf("catalog view rendered %d lines, exceeds height 30", len(lines))
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w > 100 {
			t.Errorf("line %d width = %d, exceeds catalog width 100", i, w)
		}
	}
}

func TestCatalogCoverDownloadsMissingLocalCover(t *testing.T) {
	t.Setenv("LNR_IMAGE_PROTOCOL", "kitty")
	coverBytes := func() []byte {
		var buf bytes.Buffer
		img := stdimage.NewRGBA(stdimage.Rect(0, 0, 90, 135))
		for y := 0; y < 135; y++ {
			for x := 0; x < 90; x++ {
				img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 80, A: 255})
			}
		}
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(coverBytes)
	}))
	defer server.Close()

	store, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const bookID = "book_remote_cover"
	detail := &model.BookDetail{BookSummary: model.BookSummary{
		ID:       bookID,
		Title:    "远程封面",
		CoverURL: server.URL + "/cover.png",
	}}
	catalog := &model.BookCatalog{BookID: bookID, Volumes: []model.Volume{
		{ID: "v1", Title: "第一卷", Chapters: []model.Chapter{{ID: "c1", Title: "第一章"}}},
	}}

	var out bytes.Buffer
	cv := NewCatalogView(store, nil, nil)
	cv.SetSize(100, 30)
	cv.SetTerminalOutput(termimage.NewLockedWriter(&out))
	cv.bookID = bookID

	_, cmd := cv.Update(catalogResultMsg{detail: detail, catalog: catalog})
	if cmd == nil {
		t.Fatalf("expected a cover download command")
	}
	loaded, ok := cmd().(CoverLoadedMsg)
	if !ok {
		t.Fatalf("unexpected cover message type")
	}
	if loaded.err != nil {
		t.Fatalf("cover download failed: %v", loaded.err)
	}
	cv.Update(loaded)

	if path := store.FindCoverPath(bookID); path == "" {
		t.Fatalf("expected the downloaded cover to be cached")
	}
	if cv.cover.imageID == 0 || len(cv.cover.grid) == 0 {
		t.Fatalf("expected the downloaded cover to be displayed")
	}
	if !strings.Contains(out.String(), "\x1b_Ga=T,f=100") {
		t.Errorf("terminal output missing Kitty cover transmission")
	}
}

func TestCatalogCoverFailureKeepsFullWidth(t *testing.T) {
	t.Setenv("LNR_IMAGE_PROTOCOL", "kitty")
	store, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const bookID = "book_bad_cover"
	detail := &model.BookDetail{BookSummary: model.BookSummary{
		ID:       bookID,
		Title:    "坏封面",
		CoverURL: "://bad-url",
	}}
	catalog := &model.BookCatalog{BookID: bookID, Volumes: []model.Volume{
		{ID: "v1", Title: "第一卷", Chapters: []model.Chapter{{ID: "c1", Title: "第一章"}}},
	}}

	var out bytes.Buffer
	lw := termimage.NewLockedWriter(&out)
	cv := NewCatalogView(store, nil, nil)
	cv.SetSize(100, 30)
	cv.SetTerminalOutput(lw)
	cv.bookID = bookID

	_, cmd := cv.Update(catalogResultMsg{detail: detail, catalog: catalog})
	if cmd == nil {
		t.Fatalf("expected a cover download command")
	}
	msg := cmd()
	loaded, ok := msg.(CoverLoadedMsg)
	if !ok {
		t.Fatalf("unexpected cover message type %T", msg)
	}
	if loaded.err == nil {
		t.Fatalf("expected the invalid cover URL to fail")
	}
	cv.Update(loaded)

	if got := cv.coverPaneWidth(); got != 0 {
		t.Fatalf("cover pane width = %d, want 0 after failure", got)
	}
	view := cv.View()
	if strings.Contains(view, string(rune(0x10EEEE))) || strings.Contains(view, "封面加载中") {
		t.Errorf("view should fall back to the full-width text layout")
	}
	for i, line := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(line); w > 100 {
			t.Errorf("line %d width = %d, exceeds catalog width 100", i, w)
		}
	}
}

func TestCatalogCoverLineBudget(t *testing.T) {
	t.Setenv("LNR_IMAGE_PROTOCOL", "kitty")
	store, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const bookID = "book_cover_many"
	detail := &model.BookDetail{BookSummary: model.BookSummary{ID: bookID, Title: "长目录封面测试"}}
	chapters := make([]model.Chapter, 0, 60)
	for i := 0; i < 60; i++ {
		chapters = append(chapters, model.Chapter{ID: fmt.Sprintf("c%d", i), Title: fmt.Sprintf("第 %d 章 测试章节标题", i+1)})
	}
	catalog := &model.BookCatalog{BookID: bookID, Volumes: []model.Volume{
		{ID: "v1", Title: "第一卷", Chapters: chapters},
	}}
	writeTestCover(t, store.ImagePath(bookID, "cover.png"), 201, 303)

	var out bytes.Buffer
	cv := NewCatalogView(store, nil, nil)
	cv.SetSize(80, 21)
	cv.SetTerminalOutput(termimage.NewLockedWriter(&out))
	cv.bookID = bookID

	_, cmd := cv.Update(catalogResultMsg{detail: detail, catalog: catalog})
	if cmd == nil {
		t.Fatalf("expected a cover load command")
	}
	loaded, ok := cmd().(CoverLoadedMsg)
	if !ok || loaded.err != nil {
		t.Fatalf("cover load failed: %+v", loaded)
	}
	cv.Update(loaded)

	view := cv.View()
	lines := strings.Split(view, "\n")
	if len(lines) > 21 {
		t.Errorf("catalog rendered %d lines with cover, exceeds height 21", len(lines))
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w > 80 {
			t.Errorf("line %d width = %d, exceeds catalog width 80", i, w)
		}
	}
	if got := strings.Count(view, string(rune(0x10EEEE))); got != cv.cover.cols*cv.cover.rows {
		t.Errorf("placeholder count = %d, want %d", got, cv.cover.cols*cv.cover.rows)
	}
	if cv.cover.cols == 0 || cv.cover.rows == 0 {
		t.Errorf("expected a non-empty cover grid")
	}
}

func TestCatalogCoverResizeReflow(t *testing.T) {
	t.Setenv("LNR_IMAGE_PROTOCOL", "kitty")
	store, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const bookID = "book_cover_resize"
	detail := &model.BookDetail{BookSummary: model.BookSummary{ID: bookID, Title: "封面缩放"}}
	catalog := &model.BookCatalog{BookID: bookID, Volumes: []model.Volume{
		{ID: "v1", Title: "第一卷", Chapters: []model.Chapter{{ID: "c1", Title: "第一章"}}},
	}}
	writeTestCover(t, store.ImagePath(bookID, "cover.png"), 120, 180)

	var out bytes.Buffer
	lw := termimage.NewLockedWriter(&out)
	cv := NewCatalogView(store, nil, nil)
	cv.SetSize(100, 30)
	cv.SetTerminalOutput(lw)
	cv.bookID = bookID

	_, cmd := cv.Update(catalogResultMsg{detail: detail, catalog: catalog})
	if cmd == nil {
		t.Fatalf("expected a cover load command")
	}
	loaded, ok := cmd().(CoverLoadedMsg)
	if !ok || loaded.err != nil {
		t.Fatalf("cover load failed: %+v", loaded)
	}
	cv.Update(loaded)
	oldCols, oldRows := cv.cover.cols, cv.cover.rows

	out.Reset()
	cv.SetSize(100, 18)
	_, cmd = cv.Update(tea.WindowSizeMsg{Width: 100, Height: 18})
	if cmd == nil {
		t.Fatalf("expected a cover reflow command")
	}
	cmd()
	if !strings.Contains(out.String(), "\x1b_Ga=p,U=1") {
		t.Errorf("expected a Kitty placement command, got %q", out.String())
	}
	if cv.cover.cols == oldCols && cv.cover.rows == oldRows {
		t.Errorf("expected cover grid to shrink after resize, still %dx%d", oldCols, oldRows)
	}
	if len(cv.cover.grid) != cv.cover.rows {
		t.Errorf("grid rows = %d, want %d", len(cv.cover.grid), cv.cover.rows)
	}
}

func writeTestCover(t *testing.T, path string, width, height int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	img := stdimage.NewRGBA(stdimage.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 110, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMixedChapterRendersInlineImages(t *testing.T) {
	t.Setenv("LNR_IMAGE_PROTOCOL", "kitty")
	store, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const bookID = "book_mixed"
	const chapterID = "ch_mixed"
	const imgURL = "https://example.com/mixed_illustration.png"
	const imgURL2 = "https://example.com/mixed_illustration_2.png"
	if err := store.SaveCatalog(&model.BookCatalog{
		BookID: bookID,
		Volumes: []model.Volume{{
			ID:       "vol_1",
			Title:    "第一卷",
			Chapters: []model.Chapter{{ID: chapterID, Title: "第一章 图文"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveChapter(&model.ChapterContent{
		ID:     chapterID,
		BookID: bookID,
		Title:  "第一章 图文",
		Elements: []model.ContentElement{
			{Type: model.ContentTypeText, Text: "第一段正文，故事从这里开始。"},
			{Type: model.ContentTypeImage, URL: imgURL},
			{Type: model.ContentTypeText, Text: "两幅插图之间的正文。"},
			{Type: model.ContentTypeImage, URL: imgURL2},
		},
	}); err != nil {
		t.Fatal(err)
	}

	r, err := reader.NewReader(store, bookID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.LoadChapter(chapterID); err != nil {
		t.Fatal(err)
	}
	if r.IsIllustrationOnly() {
		t.Fatalf("mixed chapter should not be illustration-only")
	}
	imgPath := r.IllustrationPath(imgURL)
	writeTestCover(t, imgPath, 640, 960)
	writeTestCover(t, r.IllustrationPath(imgURL2), 960, 540)

	var out bytes.Buffer
	rv := NewReaderView(store, nil)
	rv.SetSize(90, 30)
	rv.SetTerminalOutput(termimage.NewLockedWriter(&out))

	cmd := rv.OpenChapter(bookID, chapterID)
	if cmd == nil {
		t.Fatalf("expected chapter load command")
	}
	msg, ok := cmd().(ChapterContentLoadedMsg)
	if !ok {
		t.Fatalf("unexpected chapter message type")
	}
	if msg.err != nil {
		t.Fatalf("chapter load failed: %v", msg.err)
	}
	if len(msg.inlineImages) != 2 {
		t.Fatalf("inline images = %d, want 2", len(msg.inlineImages))
	}
	for i, img := range msg.inlineImages {
		if img.err != nil || img.id == 0 || len(img.grid) == 0 {
			t.Fatalf("inline image %d was not prepared: %+v", i, img)
		}
		if img.cols <= 20 || img.rows < 6 {
			t.Errorf("inline image %d grid = %dx%d, expected a visible illustration", i, img.cols, img.rows)
		}
	}
	if msg.inlineImages[0].id == msg.inlineImages[1].id {
		t.Errorf("inline images reused the same terminal image id")
	}
	rv.Update(msg)

	view := rv.View()
	if !strings.Contains(view, "第一段正文") {
		t.Errorf("inline view lost preceding prose, got: %s", view)
	}
	if strings.Contains(view, "按 [i]") {
		t.Errorf("inline view should not keep the [i] badge hint, got: %s", view)
	}
	if !strings.Contains(view, "[已内联]") {
		t.Errorf("expected inline illustration hint, got: %s", view)
	}
	if strings.Count(view, string(rune(0x10EEEE))) == 0 {
		t.Errorf("inline view missing first image placeholders")
	}
	rv.viewport.SetYOffset(rv.inlineImages[0].rows)
	midView := rv.View()
	if !strings.Contains(midView, "第 1/2 张插图") {
		t.Errorf("inline view missing first image caption, got: %s", midView)
	}
	if !strings.Contains(midView, "两幅插图之间的正文") {
		t.Errorf("inline view lost interlaced prose, got: %s", midView)
	}
	rv.viewport.GotoBottom()
	bottomView := rv.View()
	if !strings.Contains(bottomView, "第 2/2 张插图") {
		t.Errorf("inline view missing second image caption, got: %s", bottomView)
	}
	if strings.Count(bottomView, string(rune(0x10EEEE))) == 0 {
		t.Errorf("inline view missing second image placeholders")
	}
	if lines := strings.Count(view, "\n") + 1; lines > 30 {
		t.Errorf("inline view rendered %d lines, exceeds height 30", lines)
	}
	if got := strings.Count(out.String(), "\x1b_Ga=T,f=100"); got < 2 {
		t.Errorf("terminal output has %d inline transmissions, want at least 2", got)
	}

	// Pressing [i] must not open the old modal in a Kitty-capable terminal.
	rv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if rv.showImageModal {
		t.Errorf("inline chapter should not open the illustration modal")
	}

	// Traditional conversion changes the badge prefix; inline layout must
	// still recognize it and keep images in place.
	rv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if !strings.Contains(rv.viewport.View(), string(rune(0x10EEEE))) {
		t.Errorf("inlined illustration disappeared after traditional conversion")
	}
	if strings.Contains(rv.viewport.View(), "[插圖:") || strings.Contains(rv.viewport.View(), "[插图:") {
		t.Errorf("traditional conversion exposed an illustration badge instead of inlining")
	}
}

func TestMixedInlineImagesCleanupAndProgress(t *testing.T) {
	t.Setenv("LNR_IMAGE_PROTOCOL", "kitty")
	store, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const bookID = "book_inline_cleanup"
	const chapterID = "ch_inline_cleanup"
	const imgURL = "https://example.com/cleanup.png"
	if err := store.SaveCatalog(&model.BookCatalog{
		BookID: bookID,
		Volumes: []model.Volume{{
			ID:       "vol_1",
			Title:    "第一卷",
			Chapters: []model.Chapter{{ID: chapterID, Title: "图文"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	elements := make([]model.ContentElement, 0, 12)
	for i := 0; i < 10; i++ {
		elements = append(elements, model.ContentElement{Type: model.ContentTypeText, Text: fmt.Sprintf("前置段落 %d。", i+1)})
	}
	elements = append(elements,
		model.ContentElement{Type: model.ContentTypeImage, URL: imgURL},
		model.ContentElement{Type: model.ContentTypeText, Text: "第二段。"},
	)
	if err := store.SaveChapter(&model.ChapterContent{
		ID:       chapterID,
		BookID:   bookID,
		Title:    "图文",
		Elements: elements,
	}); err != nil {
		t.Fatal(err)
	}
	r, err := reader.NewReader(store, bookID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.LoadChapter(chapterID); err != nil {
		t.Fatal(err)
	}
	writeTestCover(t, r.IllustrationPath(imgURL), 640, 960)

	var out bytes.Buffer
	rv := NewReaderView(store, nil)
	rv.SetSize(90, 30)
	rv.SetTerminalOutput(termimage.NewLockedWriter(&out))
	msg := rv.OpenChapter(bookID, chapterID)()
	rv.Update(msg)
	rv.viewport.SetYOffset(3)
	oldImageID := rv.inlineImages[0].id
	if oldImageID == 0 {
		t.Fatalf("expected a prepared inline image")
	}

	out.Reset()
	_, cmd := rv.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatalf("expected reader exit command")
	}
	if _, ok := cmd().(common.SwitchViewMsg); !ok {
		t.Fatalf("expected SwitchViewMsg on reader exit")
	}
	if rv.inlineImages != nil {
		t.Errorf("inline images were not cleared on exit")
	}
	if !strings.Contains(out.String(), "\x1b_Ga=d") {
		t.Errorf("terminal output missing inline image deletion")
	}
	p, err := r.LoadProgress()
	if err != nil {
		t.Fatal(err)
	}
	if p == nil || p.LineIndex != 3 {
		t.Fatalf("unexpected text progress: %+v", p)
	}
}

func TestMixedInlineImageReflow(t *testing.T) {
	t.Setenv("LNR_IMAGE_PROTOCOL", "kitty")
	store, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const bookID = "book_inline_reflow"
	const chapterID = "ch_inline_reflow"
	const imgURL = "https://example.com/reflow.png"
	if err := store.SaveCatalog(&model.BookCatalog{
		BookID: bookID,
		Volumes: []model.Volume{{
			ID:       "vol_1",
			Title:    "第一卷",
			Chapters: []model.Chapter{{ID: chapterID, Title: "图文"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveChapter(&model.ChapterContent{
		ID:     chapterID,
		BookID: bookID,
		Title:  "图文",
		Elements: []model.ContentElement{
			{Type: model.ContentTypeText, Text: "正文之前。"},
			{Type: model.ContentTypeImage, URL: imgURL},
			{Type: model.ContentTypeText, Text: "正文之后。"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	r, err := reader.NewReader(store, bookID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.LoadChapter(chapterID); err != nil {
		t.Fatal(err)
	}
	writeTestCover(t, r.IllustrationPath(imgURL), 640, 960)

	var out bytes.Buffer
	rv := NewReaderView(store, nil)
	rv.SetSize(90, 30)
	rv.SetTerminalOutput(termimage.NewLockedWriter(&out))
	msg := rv.OpenChapter(bookID, chapterID)()
	rv.Update(msg)
	oldCols := rv.inlineImages[0].cols
	oldRows := rv.inlineImages[0].rows

	out.Reset()
	rv.SetSize(60, 21)
	_, cmd := rv.Update(tea.WindowSizeMsg{Width: 60, Height: 21})
	if cmd == nil {
		t.Fatalf("expected inline images to reflow after resize")
	}
	cmd()
	if rv.inlineImages[0].cols >= oldCols {
		t.Errorf("inline image width = %d, expected narrower than %d", rv.inlineImages[0].cols, oldCols)
	}
	if rv.inlineImages[0].rows > oldRows {
		t.Errorf("inline image height = %d, expected <= %d", rv.inlineImages[0].rows, oldRows)
	}
	if !strings.Contains(out.String(), "\x1b_Ga=p") {
		t.Errorf("expected a placement reflow after resize, output missing a=p")
	}
	view := rv.View()
	if got, want := strings.Count(view, string(rune(0x10EEEE))), rv.inlineImages[0].cols*rv.inlineImages[0].rows; got != want {
		t.Errorf("placeholder count after reflow = %d, want %d", got, want)
	}
}

func TestMixedInlineImageFallbackWithoutKitty(t *testing.T) {
	t.Setenv("LNR_IMAGE_PROTOCOL", "off")
	store, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const bookID = "book_inline_fallback"
	const chapterID = "ch_inline_fallback"
	const imgURL = "https://example.com/fallback.png"
	if err := store.SaveCatalog(&model.BookCatalog{
		BookID: bookID,
		Volumes: []model.Volume{{
			ID:       "vol_1",
			Title:    "第一卷",
			Chapters: []model.Chapter{{ID: chapterID, Title: "图文"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveChapter(&model.ChapterContent{
		ID:     chapterID,
		BookID: bookID,
		Title:  "图文",
		Elements: []model.ContentElement{
			{Type: model.ContentTypeText, Text: "正文。"},
			{Type: model.ContentTypeImage, URL: imgURL},
		},
	}); err != nil {
		t.Fatal(err)
	}
	r, err := reader.NewReader(store, bookID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.LoadChapter(chapterID); err != nil {
		t.Fatal(err)
	}
	writeTestCover(t, r.IllustrationPath(imgURL), 320, 480)

	rv := NewReaderView(store, nil)
	rv.SetSize(80, 24)
	rv.SetTerminalOutput(termimage.NewLockedWriter(&bytes.Buffer{}))
	msg := rv.OpenChapter(bookID, chapterID)()
	if msg.(ChapterContentLoadedMsg).inlineImages != nil {
		t.Fatalf("non-Kitty terminal should not prepare inline images")
	}
	rv.Update(msg)
	view := rv.View()
	if !strings.Contains(view, "按 [i]") {
		t.Errorf("non-Kitty fallback should keep the [i] hint, got: %s", view)
	}
	if rv.showImageModal {
		t.Errorf("fallback should not auto-open the modal")
	}

	_, cmd := rv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if cmd == nil {
		t.Fatalf("expected fallback modal load command")
	}
	loaded, ok := cmd().(IllustrationLoadedMsg)
	if !ok || loaded.err != nil {
		t.Fatalf("fallback illustration load failed: %+v", loaded)
	}
	rv.Update(loaded)
	if !rv.showImageModal || !strings.Contains(rv.View(), "插图查看器") {
		t.Errorf("expected metadata illustration modal on non-Kitty terminal")
	}
}

func TestIllustrationOnlyChapterOpensGallery(t *testing.T) {
	t.Setenv("LNR_IMAGE_PROTOCOL", "kitty")
	store, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const bookID = "book_gallery"
	const chapterID = "ch_illustrations"
	urls := []string{
		"https://example.com/gallery_1.png",
		"https://example.com/gallery_2.png",
	}
	if err := store.SaveCatalog(&model.BookCatalog{
		BookID: bookID,
		Volumes: []model.Volume{{
			ID:       "vol_1",
			Title:    "第一卷",
			Chapters: []model.Chapter{{ID: chapterID, Title: "插图"}},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	elements := make([]model.ContentElement, 0, len(urls))
	for _, u := range urls {
		elements = append(elements, model.ContentElement{Type: model.ContentTypeImage, URL: u})
	}
	if err := store.SaveChapter(&model.ChapterContent{
		ID:       chapterID,
		BookID:   bookID,
		Title:    "插图",
		Elements: elements,
	}); err != nil {
		t.Fatal(err)
	}
	r, err := reader.NewReader(store, bookID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.LoadChapter(chapterID); err != nil {
		t.Fatal(err)
	}
	if !r.IsIllustrationOnly() {
		t.Fatalf("expected chapter to be illustration-only")
	}
	for i, u := range urls {
		writeTestCover(t, r.IllustrationPath(u), 320+i*20, 480)
	}

	var out bytes.Buffer
	rv := &ReaderView{
		store:     store,
		reader:    r,
		bookID:    bookID,
		chapterID: chapterID,
		width:     80,
		height:    30,
	}
	rv.SetSize(80, 30)
	rv.SetTerminalOutput(termimage.NewLockedWriter(&out))

	_, cmd := rv.Update(ChapterContentLoadedMsg{engine: r})
	if cmd == nil {
		t.Fatalf("expected automatic gallery image load")
	}
	if !rv.galleryMode || !rv.showImageModal {
		t.Fatalf("expected gallery mode, got gallery=%v modal=%v", rv.galleryMode, rv.showImageModal)
	}
	loaded, ok := cmd().(IllustrationLoadedMsg)
	if !ok || loaded.err != nil {
		t.Fatalf("gallery image load failed: %+v", loaded)
	}
	rv.Update(loaded)

	view := rv.View()
	if !strings.Contains(view, "插图欣赏") || !strings.Contains(view, "[1/2]") {
		t.Errorf("expected gallery header with counter, got: %s", view)
	}
	if !strings.Contains(view, "上一张/下一张") || !strings.Contains(view, "[Esc] 返回目录") {
		t.Errorf("expected gallery hint footer, got: %s", view)
	}
	if got, want := strings.Count(view, string(rune(0x10EEEE))), rv.imageCols*rv.imageRows; got != want {
		t.Errorf("placeholder count = %d, want %d", got, want)
	}
	if lines := strings.Count(view, "\n") + 1; lines != 30 {
		t.Errorf("gallery rendered %d lines, want 30", lines)
	}
	if strings.Contains(view, "第 1/2 行") {
		t.Errorf("gallery should not render the text viewport status")
	}

	// Navigate to the second image and verify progress records the image index.
	_, nextCmd := rv.Update(tea.KeyMsg{Type: tea.KeyRight})
	if nextCmd == nil {
		t.Fatalf("expected next image command")
	}
	next, ok := nextCmd().(IllustrationLoadedMsg)
	if !ok || next.err != nil {
		t.Fatalf("second gallery image load failed: %+v", next)
	}
	rv.Update(next)
	if rv.imageIndex != 1 {
		t.Fatalf("imageIndex = %d, want 1", rv.imageIndex)
	}
	_, backCmd := rv.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if backCmd == nil {
		t.Fatalf("expected back command on Esc")
	}
	if _, ok := backCmd().(common.SwitchViewMsg); !ok {
		t.Fatalf("expected SwitchViewMsg on gallery exit, got %T", backCmd())
	}
	p, err := r.LoadProgress()
	if err != nil {
		t.Fatal(err)
	}
	if p == nil || p.ImageIndex != 1 || p.ChapterID != chapterID {
		t.Fatalf("unexpected gallery progress: %+v", p)
	}

	// Reopening the chapter resumes at the saved gallery page.
	rv2 := NewReaderView(store, nil)
	rv2.SetSize(80, 30)
	rv2.SetTerminalOutput(termimage.NewLockedWriter(&bytes.Buffer{}))
	loadCmd := rv2.OpenChapter(bookID, chapterID)
	if loadCmd == nil {
		t.Fatalf("expected chapter load command")
	}
	loadedMsg, ok := loadCmd().(ChapterContentLoadedMsg)
	if !ok || loadedMsg.err != nil {
		t.Fatalf("reload failed: %+v", loadedMsg)
	}
	rv2.Update(loadedMsg)
	if !rv2.galleryMode || rv2.imageIndex != 1 {
		t.Fatalf("gallery did not resume at image 1: gallery=%v index=%d", rv2.galleryMode, rv2.imageIndex)
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

// newPagingReaderView prepares a two-chapter book whose first chapter is long
// enough to page through, with all content cached locally.
func newPagingReaderView(t *testing.T) *ReaderView {
	t.Helper()
	store, err := storage.NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const bookID = "book_paging"
	const chapterID = "ch_paging"
	const nextChapterID = "ch_paging_next"
	if err := store.SaveCatalog(&model.BookCatalog{
		BookID: bookID,
		Volumes: []model.Volume{{
			Title: "第一卷",
			Chapters: []model.Chapter{
				{ID: chapterID, Title: "长章节"},
				{ID: nextChapterID, Title: "下一章"},
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	elements := make([]model.ContentElement, 0, 80)
	for i := 0; i < 80; i++ {
		elements = append(elements, model.ContentElement{
			Type: model.ContentTypeText,
			Text: fmt.Sprintf("第 %d 段正文内容。", i+1),
		})
	}
	if err := store.SaveChapter(&model.ChapterContent{
		ID:       chapterID,
		BookID:   bookID,
		Title:    "长章节",
		Elements: elements,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveChapter(&model.ChapterContent{
		ID:       nextChapterID,
		BookID:   bookID,
		Title:    "下一章",
		Elements: []model.ContentElement{{Type: model.ContentTypeText, Text: "下一章正文。"}},
	}); err != nil {
		t.Fatal(err)
	}

	rv := NewReaderView(store, nil)
	rv.SetSize(80, 24)
	cmd := rv.OpenChapter(bookID, chapterID)
	if cmd == nil {
		t.Fatal("expected chapter load command")
	}
	loaded, ok := cmd().(ChapterContentLoadedMsg)
	if !ok || loaded.err != nil {
		t.Fatalf("chapter load failed: %+v", loaded)
	}
	rv.Update(loaded)
	return rv
}

func TestReaderPageTurnKeys(t *testing.T) {
	rv := newPagingReaderView(t)

	if got := rv.viewport.Height; got != 22 {
		t.Fatalf("viewport height = %d, want 22", got)
	}
	if got := rv.pageStep(); got != 20 {
		t.Fatalf("page step = %d, want 20", got)
	}

	press := func(key tea.KeyMsg) {
		t.Helper()
		if _, cmd := rv.Update(key); cmd != nil {
			t.Fatalf("page turn emitted an unexpected command")
		}
	}

	// Arrow keys and h/l move one page in both directions.
	press(tea.KeyMsg{Type: tea.KeyRight})
	if got := rv.viewport.YOffset; got != 20 {
		t.Fatalf("after right: y offset = %d, want 20", got)
	}
	press(tea.KeyMsg{Type: tea.KeyRight})
	if got := rv.viewport.YOffset; got != 40 {
		t.Fatalf("after second right: y offset = %d, want 40", got)
	}
	press(tea.KeyMsg{Type: tea.KeyLeft})
	if got := rv.viewport.YOffset; got != 20 {
		t.Fatalf("after left: y offset = %d, want 20", got)
	}
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if got := rv.viewport.YOffset; got != 40 {
		t.Fatalf("after l: y offset = %d, want 40", got)
	}
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if got := rv.viewport.YOffset; got != 20 {
		t.Fatalf("after h: y offset = %d, want 20", got)
	}

	// The legacy pager keys share the same 90% step.
	press(tea.KeyMsg{Type: tea.KeySpace})
	if got := rv.viewport.YOffset; got != 40 {
		t.Fatalf("after space: y offset = %d, want 40", got)
	}
	press(tea.KeyMsg{Type: tea.KeyPgDown})
	if got := rv.viewport.YOffset; got != 60 {
		t.Fatalf("after pgdown: y offset = %d, want 60", got)
	}
	press(tea.KeyMsg{Type: tea.KeyPgUp})
	if got := rv.viewport.YOffset; got != 40 {
		t.Fatalf("after pgup: y offset = %d, want 40", got)
	}
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if got := rv.viewport.YOffset; got != 60 {
		t.Fatalf("after f: y offset = %d, want 60", got)
	}
	press(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	if got := rv.viewport.YOffset; got != 40 {
		t.Fatalf("after b: y offset = %d, want 40", got)
	}

	// Paging never switches chapters; chapter navigation keeps its own keys.
	if rv.chapterID != "ch_paging" {
		t.Fatalf("page turn switched chapter to %s", rv.chapterID)
	}

	// The first and last page stop at the scroll bounds.
	rv.viewport.SetYOffset(0)
	press(tea.KeyMsg{Type: tea.KeyLeft})
	if got := rv.viewport.YOffset; got != 0 {
		t.Fatalf("left at top: y offset = %d, want 0", got)
	}
	rv.viewport.GotoBottom()
	bottom := rv.viewport.YOffset
	press(tea.KeyMsg{Type: tea.KeyRight})
	if got := rv.viewport.YOffset; got != bottom {
		t.Fatalf("right at bottom: y offset = %d, want %d", got, bottom)
	}

	// The step shrinks with the viewport and never drops below one row.
	rv.SetSize(80, 20) // viewport height 18, overlap 2
	if got := rv.pageStep(); got != 16 {
		t.Fatalf("page step at height 18 = %d, want 16", got)
	}
	rv.SetSize(80, 3) // viewport height 1
	if got := rv.pageStep(); got != 1 {
		t.Fatalf("page step at height 1 = %d, want 1", got)
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

func TestSettingsVersionItem(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "lnr-test-settings-version-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	sv := NewSettingsView(store)
	sv.SetSize(100, 30)

	// Move cursor down to item 9 (version item)
	for i := 0; i < 9; i++ {
		sv.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if sv.Cursor() != 9 {
		t.Fatalf("expected cursor at 9, got %d", sv.Cursor())
	}

	// View should render version item label
	viewStr := sv.View()
	if !strings.Contains(viewStr, "程序版本与环境") {
		t.Errorf("expected view to contain '程序版本与环境', got: %s", viewStr)
	}

	// Press Enter on item 9
	_, cmd := sv.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected non-nil cmd on Enter on version item")
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

func TestBookshelfGroupsAndExploreTabsNotTruncated(t *testing.T) {
	// 1. Bookshelf groups at 80 and 100 width: verify all 4 groups are displayed
	bv := &BookshelfView{
		width:   80,
		height:  24,
		shelves: storage.DefaultBookshelves,
		loaded:  true,
	}
	bOut80 := bv.View()
	for _, expectedGroup := range []string{"全部", "在读", "已完结", "精选收藏"} {
		if !strings.Contains(bOut80, expectedGroup) {
			t.Errorf("expected bookshelf at width 80 to contain group %q, got output:\n%s", expectedGroup, bOut80)
		}
	}

	bv.width = 100
	bOut100 := bv.View()
	for _, expectedGroup := range []string{"全部", "在读", "已完结", "精选收藏"} {
		if !strings.Contains(bOut100, expectedGroup) {
			t.Errorf("expected bookshelf at width 100 to contain group %q, got output:\n%s", expectedGroup, bOut100)
		}
	}

	// 2. Explore ranking subtabs at width 80 and 100: verify all 7 categories are displayed
	ev := &ExploreView{
		width:  80,
		height: 24,
		subTab: SubTabHot,
	}
	eOut80 := ev.View()
	for _, expectedTab := range []string{"热门榜", "动画化", "今日更新", "新书榜", "完结全本", "题材分类", "文库分类"} {
		if !strings.Contains(eOut80, expectedTab) {
			t.Errorf("expected explore at width 80 to contain tab %q, got output:\n%s", expectedTab, eOut80)
		}
	}

	ev.width = 100
	eOut100 := ev.View()
	for _, expectedTab := range []string{"热门榜", "动画化", "今日更新", "新书榜", "完结全本", "题材分类", "文库分类"} {
		if !strings.Contains(eOut100, expectedTab) {
			t.Errorf("expected explore at width 100 to contain tab %q, got output:\n%s", expectedTab, eOut100)
		}
	}
}

func TestCJKDescriptionConsistencyAndSanitization(t *testing.T) {
	rawDesc := "\u3000\u3000圣玛格诺利亚共和国日复一日遭受邻国「帝国」的无人机「军团」侵略国土。\n\u3000\u3000是的——表面上确实如此……实际上“毫无伤亡”当然是谎言。"
	cleaned := theme.CleanDescription(rawDesc)
	if strings.Contains(cleaned, "\u3000") || strings.Contains(cleaned, "\n") {
		t.Errorf("CleanDescription failed to remove fullwidth spaces and newlines: %q", cleaned)
	}

	book := model.BookDetail{
		BookSummary: model.BookSummary{
			ID:          "1001",
			Title:       "86-不存在的战区-",
			Author:      "安里アサト",
			Publisher:   "电击文库",
			Description: rawDesc,
		},
	}

	for _, w := range []int{80, 100, 120} {
		bv := &BookshelfView{
			width:   w,
			height:  24,
			shelves: storage.DefaultBookshelves,
			books:   []model.BookDetail{book},
			loaded:  true,
		}
		bOut := bv.View()

		sv := &SearchView{
			width:   w,
			height:  24,
			results: []model.BookSummary{book.BookSummary},
		}
		sv.input.Blur()
		sOut := sv.View()

		ev := &ExploreView{
			width:   w,
			height:  24,
			results: []model.BookSummary{book.BookSummary},
		}
		eOut := ev.View()

		// Verify all 3 views rendered the cleaned description prefix without line wrapping
		expectedPrefix := "圣玛格诺利亚共和国"
		if !strings.Contains(bOut, expectedPrefix) {
			t.Errorf("bookshelf at width %d missing expected prefix %s", w, expectedPrefix)
		}
		if !strings.Contains(sOut, expectedPrefix) {
			t.Errorf("search at width %d missing expected prefix %s", w, expectedPrefix)
		}
		if !strings.Contains(eOut, expectedPrefix) {
			t.Errorf("explore at width %d missing expected prefix %s", w, expectedPrefix)
		}
	}
}

func TestIllustrationModalCenterAndMetadata(t *testing.T) {
	rv := &ReaderView{
		width:          80,
		height:         24,
		showImageModal: true,
		imageIndex:     0,
		imagePath:      "/tmp/mock_illu.jpg",
		imageWidth:     1200,
		imageHeight:    1800,
		imageSize:      256 * 1024,
	}

	out := rv.renderImageModal()
	if !strings.Contains(out, "[插图查看器]") {
		t.Errorf("expected modal to contain '[插图查看器]'")
	}
	if !strings.Contains(out, "1200 × 1800 像素") {
		t.Errorf("expected modal to contain image dimensions, got: %s", out)
	}
	if !strings.Contains(out, "256.0 KB") {
		t.Errorf("expected modal to contain image size, got: %s", out)
	}
	if !strings.Contains(out, "Quick Look") {
		t.Errorf("expected modal to mention Quick Look, got: %s", out)
	}
}

func TestIllustrationModalKittyGrid(t *testing.T) {
	const imageID = 0x123456
	grid := termimage.KittyPlaceholderGrid(imageID, 8, 4)
	if len(grid) != 4 {
		t.Fatalf("expected 4 grid rows, got %d", len(grid))
	}

	rv := &ReaderView{
		width:          80,
		height:         30,
		showImageModal: true,
		imageIndex:     0,
		imagePath:      "/tmp/mock_illu.jpg",
		imageWidth:     1200,
		imageHeight:    1800,
		imageSize:      256 * 1024,
		kittyCapable:   true,
		kittyImageID:   imageID,
		imageGrid:      grid,
		imageCols:      8,
		imageRows:      4,
	}

	out := rv.renderImageModal()
	if got := strings.Count(out, string(rune(0x10EEEE))); got != 32 {
		t.Errorf("expected 32 placeholder cells, got %d", got)
	}
	if !strings.Contains(out, "Kitty 图形协议") {
		t.Errorf("expected modal to mention Kitty graphics, got: %s", out)
	}
	if !strings.Contains(out, "1200 × 1800 像素") {
		t.Errorf("expected modal to contain image dimensions, got: %s", out)
	}
	if !strings.Contains(out, "Quick Look") {
		t.Errorf("expected modal to retain system preview hint, got: %s", out)
	}
	if lines := strings.Count(out, "\n") + 1; lines > 30 {
		t.Errorf("modal rendered %d lines, exceeds view height 30", lines)
	}
}

func TestKittyIllustrationLoadIntegration(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.NewStorage(dir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	const bookID = "book_kitty"
	const chapterID = "ch_1"
	const imgURL = "https://example.com/illustration.png"
	if err := store.SaveCatalog(&model.BookCatalog{
		BookID: bookID,
		Volumes: []model.Volume{{
			ID:    "vol_1",
			Title: "第一卷",
			Chapters: []model.Chapter{{
				ID:    chapterID,
				Title: "第一章",
			}},
		}},
	}); err != nil {
		t.Fatalf("failed to save catalog: %v", err)
	}
	if err := store.SaveChapter(&model.ChapterContent{
		ID:     chapterID,
		BookID: bookID,
		Title:  "第一章",
		Elements: []model.ContentElement{
			{Type: model.ContentTypeImage, URL: imgURL},
			{Type: model.ContentTypeText, Text: "正文内容"},
		},
	}); err != nil {
		t.Fatalf("failed to save chapter: %v", err)
	}

	r, err := reader.NewReader(store, bookID)
	if err != nil {
		t.Fatalf("failed to create reader: %v", err)
	}
	if _, err := r.LoadChapter(chapterID); err != nil {
		t.Fatalf("failed to load chapter: %v", err)
	}
	if got := len(r.Illustrations()); got != 1 {
		t.Fatalf("expected 1 illustration, got %d", got)
	}

	imgPath := r.IllustrationPath(r.Illustrations()[0])
	if err := os.MkdirAll(filepath.Dir(imgPath), 0o755); err != nil {
		t.Fatalf("failed to create image dir: %v", err)
	}
	img := stdimage.NewRGBA(stdimage.Rect(0, 0, 160, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 160; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 90, A: 255})
		}
	}
	f, err := os.Create(imgPath)
	if err != nil {
		t.Fatalf("failed to create image: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatalf("failed to encode image: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("failed to close image: %v", err)
	}

	var out bytes.Buffer
	lw := termimage.NewLockedWriter(&out)
	rv := &ReaderView{
		reader:         r,
		width:          80,
		height:         30,
		showImageModal: true,
		kittyCapable:   true,
		terminalOut:    lw,
	}
	rv.SetSize(80, 30)

	cmd := rv.startImageLoad(0)
	if cmd == nil {
		t.Fatalf("expected an illustration load command")
	}
	msg, ok := cmd().(IllustrationLoadedMsg)
	if !ok {
		t.Fatalf("unexpected message type %T", msg)
	}
	if msg.err != nil {
		t.Fatalf("illustration load failed: %v", msg.err)
	}
	if !msg.kitty || msg.kittyID == 0 || msg.cols <= 0 || msg.rows <= 0 {
		t.Fatalf("expected a Kitty transmission, got %+v", msg)
	}
	if !strings.Contains(out.String(), "\x1b_Ga=T,f=100") {
		t.Errorf("terminal output missing Kitty transmit header: %q", out.String())
	}
	if !strings.Contains(out.String(), "U=1") || !strings.Contains(out.String(), "c=") {
		t.Errorf("terminal output missing Unicode placeholder placement: %q", out.String())
	}

	rv.Update(msg)
	if rv.imageLoading {
		t.Errorf("expected image loading to finish")
	}
	if len(rv.imageGrid) == 0 {
		t.Fatalf("expected placeholder grid to be installed")
	}
	if rv.kittyImageID != msg.kittyID {
		t.Errorf("kitty image id = %d, want %d", rv.kittyImageID, msg.kittyID)
	}

	view := rv.renderImageModal()
	if got := strings.Count(view, string(rune(0x10EEEE))); got != msg.cols*msg.rows {
		t.Errorf("placeholder count = %d, want %d", got, msg.cols*msg.rows)
	}
	if !strings.Contains(view, "Kitty 图形协议") {
		t.Errorf("expected the modal to label Kitty rendering")
	}
	for i, line := range strings.Split(view, "\n") {
		if w := ansi.StringWidth(line); w > rv.width {
			t.Errorf("line %d width = %d, exceeds reader width %d", i, w, rv.width)
		}
	}
}

func TestStaleIllustrationMessageDoesNotReplaceCurrentImage(t *testing.T) {
	rv := &ReaderView{
		width:          80,
		height:         24,
		showImageModal: true,
		imageIndex:     0,
		imageLoading:   true,
	}
	rv.imageGen.Store(7)

	rv.Update(IllustrationLoadedMsg{
		index:   0,
		gen:     6,
		kitty:   true,
		kittyID: 0x010203,
		cols:    4,
		rows:    2,
	})

	if rv.imageLoading != true {
		t.Errorf("stale message should not finish the active load")
	}
	if rv.imageGrid != nil {
		t.Errorf("stale message should not install a placeholder grid")
	}
	if rv.kittyImageID != 0 {
		t.Errorf("stale message should not commit an image id")
	}
}

func TestIllustrationModalCloseDeletesKittyImage(t *testing.T) {
	var out bytes.Buffer
	lw := termimage.NewLockedWriter(&out)
	rv := &ReaderView{
		width:          80,
		height:         24,
		showImageModal: true,
		kittyCapable:   true,
		terminalOut:    lw,
		kittyImageID:   11,
		imageGrid:      termimage.KittyPlaceholderGrid(11, 4, 2),
		imageCols:      4,
		imageRows:      2,
		imageWidth:     400,
		imageHeight:    200,
	}

	_, cmd := rv.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if rv.showImageModal {
		t.Fatalf("expected modal to close on Esc")
	}
	if rv.kittyImageID != 0 {
		t.Errorf("expected kitty image id to be cleared, got %d", rv.kittyImageID)
	}
	if cmd == nil {
		t.Fatalf("expected a deletion command")
	}
	cmd()
	if got := out.String(); !strings.Contains(got, "\x1b_Ga=d,d=I,i=11,q=2\x1b\\") {
		t.Errorf("expected Kitty deletion sequence, got %q", got)
	}
}

func TestReaderResizeReflowsKittyPlacement(t *testing.T) {
	var out bytes.Buffer
	lw := termimage.NewLockedWriter(&out)
	const imageID = 11
	rv := &ReaderView{
		width:          80,
		height:         30,
		showImageModal: true,
		kittyCapable:   true,
		terminalOut:    lw,
		kittyImageID:   imageID,
		imageGrid:      termimage.KittyPlaceholderGrid(imageID, 20, 10),
		imageCols:      20,
		imageRows:      10,
		imageWidth:     1200,
		imageHeight:    1800,
	}
	rv.SetSize(80, 30)

	rv.SetSize(60, 20)
	_, cmd := rv.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	if cmd == nil {
		t.Fatalf("expected a placement command after resize")
	}
	cmd()
	got := out.String()
	if !strings.Contains(got, "\x1b_Ga=p,U=1,q=2,i=11,p=1,C=1,c=") {
		t.Errorf("expected Kitty placement sequence, got %q", got)
	}
	if rv.imageCols == 20 || rv.imageRows == 10 {
		t.Errorf("expected placeholder grid to be reflowed, got %dx%d", rv.imageCols, rv.imageRows)
	}
	if len(rv.imageGrid) != rv.imageRows {
		t.Errorf("grid rows = %d, want %d", len(rv.imageGrid), rv.imageRows)
	}
}

func TestImageAreaFitsModal(t *testing.T) {
	rv := &ReaderView{width: 80, height: 24}
	cols, rows := rv.imageArea()
	if cols != 70 || rows != 16 {
		t.Errorf("imageArea() = %dx%d, want 70x16", cols, rows)
	}

	modalW, innerW := rv.modalWidths()
	if modalW != 74 || innerW != 72 {
		t.Errorf("modalWidths() = %dx%d, want 74x72", modalW, innerW)
	}
}

func TestExploreAndSearchDescriptionEnrichment(t *testing.T) {
	shortDesc := "真正的实力、平等究竟为何？几乎百分之百实现升学、就业目标的全国首屈一指的名校─…"
	fullDesc := "真正的实力、平等究竟为何？几乎百分之百实现升学、就业目标的全国首屈一指的名校──高度育成高中。这所梦想般的学校，却根据实力将学生分为A到D班。"

	book := model.BookSummary{
		ID:          "1973",
		Title:       "欢迎来到实力至上主义的教室",
		Author:      "衣笠彰梧",
		Publisher:   "MF文库J",
		Description: shortDesc,
	}

	// 1. Test ExploreView enrichment
	ev := &ExploreView{
		width:   120,
		height:  24,
		subTab:  SubTabHot,
		page:    1,
		results: []model.BookSummary{book},
	}

	// Before enrichment: should contain shortDesc prefix and not fullDesc continuation
	outBefore := ev.View()
	if strings.Contains(outBefore, "高度育成高中") {
		t.Errorf("expected output before enrichment not to contain full continuation")
	}

	// Dispatch enrichment msg
	enrichedMsg := exploreDescEnrichedMsg{
		subTab: SubTabHot,
		page:   1,
		items:  map[string]string{"1973": fullDesc},
	}
	ev, _ = ev.Update(enrichedMsg)

	if ev.results[0].Description != fullDesc {
		t.Errorf("expected explore results[0].Description to be updated, got %q", ev.results[0].Description)
	}

	outAfter := ev.View()
	if !strings.Contains(outAfter, "高度育成高中") {
		t.Errorf("expected explore view at width 120 to contain '高度育成高中', got output:\n%s", outAfter)
	}

	// 2. Test SearchView enrichment
	sv := &SearchView{
		width:   120,
		height:  24,
		page:    1,
		results: []model.BookSummary{book},
	}
	sv.input.SetValue("实力至上")
	sv.input.Blur()

	// Dispatch search enrichment msg
	searchEnrichMsg := searchDescEnrichedMsg{
		query: "实力至上",
		page:  1,
		items: map[string]string{"1973": fullDesc},
	}
	sv, _ = sv.Update(searchEnrichMsg)

	if sv.results[0].Description != fullDesc {
		t.Errorf("expected search results[0].Description to be updated, got %q", sv.results[0].Description)
	}

	sOutAfter := sv.View()
	if !strings.Contains(sOutAfter, "高度育成高中") {
		t.Errorf("expected search view at width 120 to contain '高度育成高中', got output:\n%s", sOutAfter)
	}

	// 3. Test quickFillFromStore
	tmpDir := t.TempDir()
	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	_ = store.SaveBookDetail(&model.BookDetail{
		BookSummary: model.BookSummary{
			ID:          "1973",
			Title:       "欢迎来到实力至上主义的教室",
			Description: fullDesc,
		},
	})

	svWithStore := &SearchView{
		store:   store,
		results: []model.BookSummary{book},
	}
	svWithStore.quickFillFromStore()
	if svWithStore.results[0].Description != fullDesc {
		t.Errorf("expected quickFillFromStore to populate full description from store")
	}

	evWithStore := &ExploreView{
		store:   store,
		results: []model.BookSummary{book},
	}
	evWithStore.quickFillFromStore()
	if evWithStore.results[0].Description != fullDesc {
		t.Errorf("expected quickFillFromStore to populate full description in explore from store")
	}
}

func TestBlockedBadgeRendering(t *testing.T) {
	blockedBook := model.BookSummary{
		ID:          "102",
		Title:       "凉宫春日物语",
		Author:      "谷川流",
		Publisher:   "角川文库",
		Description: "这是测试简介",
		IsComplete:  true,
		IsBlocked:   true,
	}

	// 1. Test SearchView rendering
	sv := &SearchView{
		width:   120,
		height:  24,
		results: []model.BookSummary{blockedBook},
	}
	svOutput := sv.View()
	if !strings.Contains(svOutput, "版权受限") {
		t.Errorf("expected SearchView to render '版权受限', got:\n%s", svOutput)
	}

	// 2. Test ExploreView rendering
	ev := &ExploreView{
		width:   120,
		height:  24,
		results: []model.BookSummary{blockedBook},
	}
	evOutput := ev.View()
	if !strings.Contains(evOutput, "版权受限") {
		t.Errorf("expected ExploreView to render '版权受限', got:\n%s", evOutput)
	}

	// 3. Test BookshelfView rendering
	bv := &BookshelfView{
		width:      120,
		height:     24,
		books:      []model.BookDetail{{BookSummary: blockedBook}},
		updatesMap: make(map[string]int),
	}
	bvOutput := bv.View()
	if !strings.Contains(bvOutput, "版权受限") {
		t.Errorf("expected BookshelfView to render '版权受限', got:\n%s", bvOutput)
	}
}
