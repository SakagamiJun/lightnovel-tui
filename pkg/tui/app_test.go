package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"lnr-core/pkg/model"
	"lnr-core/pkg/storage"
	"lnr-core/pkg/tui/common"
)

func TestAppModelViewLineBudget(t *testing.T) {
	app := NewAppModel(nil, nil)

	testSizes := []struct {
		width  int
		height int
	}{
		{80, 24},
		{100, 30},
		{80, 15},
		{120, 50},
	}

	for _, sz := range testSizes {
		app.Update(tea.WindowSizeMsg{Width: sz.width, Height: sz.height})

		// Test Bookshelf
		view := app.View()
		lines := strings.Split(view, "\n")
		if len(lines) != sz.height {
			t.Errorf("expected %d lines for size %dx%d in bookshelf, got %d", sz.height, sz.width, sz.height, len(lines))
		}

		// Switch to Search
		app.Update(common.SwitchViewMsg{Target: common.ViewSearch})
		view = app.View()
		lines = strings.Split(view, "\n")
		if len(lines) != sz.height {
			t.Errorf("expected %d lines for size %dx%d in search, got %d", sz.height, sz.width, sz.height, len(lines))
		}

		// Verify line 0 contains header
		if !strings.Contains(lines[0], "LNR") && !strings.Contains(lines[0], "轻小说文库") {
			t.Errorf("expected line 0 to contain header, got: %s", lines[0])
		}

		// Verify bottom line contains status bar
		lastLine := lines[len(lines)-1]
		if !strings.Contains(lastLine, "Tab") && !strings.Contains(lastLine, "Enter") {
			t.Errorf("expected bottom line to be status bar, got: %s", lastLine)
		}
	}
}

func TestAppModelSearchViewWithManyResults(t *testing.T) {
	app := NewAppModel(nil, nil)
	height := 24
	width := 80

	app.Update(tea.WindowSizeMsg{Width: width, Height: height})
	app.Update(common.SwitchViewMsg{Target: common.ViewSearch})

	// Inject 50 results into searchView
	results := make([]model.BookSummary, 50)
	for i := 0; i < 50; i++ {
		results[i] = model.BookSummary{
			ID:          "1001",
			Title:       "刀剑神域 Progressive",
			Author:      "川原砾",
			Publisher:   "电击文库",
			WordCount:   2800000,
			Description: "艾恩葛朗特第一层攻略...",
		}
	}

	// Set search results directly
	app.searchView.SetResults(results)

	view := app.View()
	lines := strings.Split(view, "\n")

	if len(lines) != height {
		t.Fatalf("expected exactly %d lines on height %d with 50 results, got %d", height, height, len(lines))
	}

	// Line 0: Header tabs
	if !strings.Contains(lines[0], "LNR") && !strings.Contains(lines[0], "轻小说文库") {
		t.Errorf("expected line 0 to have header, got: %s", lines[0])
	}
	// Line 1: Header border
	// Line 2: Search Title
	if !strings.Contains(lines[2], "轻小说检索") {
		t.Errorf("expected line 2 to have search title, got: %s", lines[2])
	}
	// Line 3: Input box
	// Line 5: Stats ("50 本小说" or "50")
	foundStats := false
	for _, l := range lines[:8] {
		if strings.Contains(l, "50 本小说") || strings.Contains(l, "50") {
			foundStats = true
			break
		}
	}
	if !foundStats {
		t.Errorf("expected to find search stats in top 8 lines, got:\n%s", strings.Join(lines[:8], "\n"))
	}

	// Last line: status bar
	lastLine := lines[len(lines)-1]
	if !strings.Contains(lastLine, "Tab") {
		t.Errorf("expected last line to be status bar, got: %s", lastLine)
	}
}

func TestAppModelBookshelfViewWithManyResults(t *testing.T) {
	app := NewAppModel(nil, nil)
	height := 24
	width := 80

	app.Update(tea.WindowSizeMsg{Width: width, Height: height})
	app.Update(common.SwitchViewMsg{Target: common.ViewBookshelf})

	// Inject 20 cached books
	books := make([]model.BookDetail, 20)
	for i := 0; i < 20; i++ {
		books[i] = model.BookDetail{
			BookSummary: model.BookSummary{
				ID:          "1001",
				Title:       "刀剑神域",
				Author:      "川原砾",
				Publisher:   "电击文库",
				WordCount:   2800000,
				Description: "艾恩葛朗特攻略故事...",
			},
		}
	}
	app.bookshelfView.SetBooks(books)

	view := app.View()
	lines := strings.Split(view, "\n")

	if len(lines) != height {
		t.Fatalf("expected exactly %d lines on height %d with 20 books, got %d", height, height, len(lines))
	}

	// Line 0: Header tabs
	if !strings.Contains(lines[0], "LNR") && !strings.Contains(lines[0], "轻小说文库") {
		t.Errorf("expected line 0 to have header, got: %s", lines[0])
	}
	// Line 2: Bookshelf Title
	if !strings.Contains(lines[2], "本地藏书库") {
		t.Errorf("expected line 2 to have bookshelf title, got: %s", lines[2])
	}
	// Last line: status bar
	lastLine := lines[len(lines)-1]
	if !strings.Contains(lastLine, "Tab") {
		t.Errorf("expected last line to be status bar, got: %s", lastLine)
	}
}

func TestSettingsViewAndTabCycling(t *testing.T) {
	app := NewAppModel(nil, nil)
	height := 24
	width := 80
	app.Update(tea.WindowSizeMsg{Width: width, Height: height})

	// Initial view is Bookshelf
	if app.currentView != common.ViewBookshelf {
		t.Fatalf("expected initial view to be Bookshelf, got %v", app.currentView)
	}

	// 1. Tab to Search
	app.Update(tea.KeyMsg{Type: tea.KeyTab})
	if app.currentView != common.ViewSearch {
		t.Fatalf("expected view Search after Tab, got %v", app.currentView)
	}

	// 2. Tab to Settings
	app.Update(tea.KeyMsg{Type: tea.KeyTab})
	if app.currentView != common.ViewSettings {
		t.Fatalf("expected view Settings after Tab, got %v", app.currentView)
	}

	// Verify Settings line budget and content
	view := app.View()
	lines := strings.Split(view, "\n")
	if len(lines) != height {
		t.Fatalf("expected exactly %d lines for settings, got %d", height, len(lines))
	}
	if !strings.Contains(lines[0], "系统设置") {
		t.Errorf("expected tab header to contain '系统设置', got: %s", lines[0])
	}
	if !strings.Contains(view, "本地缓存目录") || !strings.Contains(view, "沉浸阅读步长") {
		t.Errorf("expected settings view to contain core setting items")
	}

	// Test settings interactions
	// Move cursor down to item 2 (scrollStep)
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if app.settingsView.Cursor() != 2 {
		t.Errorf("expected cursor at 2, got %d", app.settingsView.Cursor())
	}
	initialStep := app.settingsView.ScrollStep()
	// Press Enter to cycle step
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if app.settingsView.ScrollStep() == initialStep {
		t.Errorf("expected scrollStep to cycle from %d", initialStep)
	}

	// 3. Tab back to Bookshelf
	app.Update(tea.KeyMsg{Type: tea.KeyTab})
	if app.currentView != common.ViewBookshelf {
		t.Fatalf("expected view Bookshelf after 3rd Tab, got %v", app.currentView)
	}

	// 4. Shift+Tab backwards to Settings
	app.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if app.currentView != common.ViewSettings {
		t.Fatalf("expected view Settings after Shift+Tab, got %v", app.currentView)
	}
}

func TestSanitizeFileName(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"刀剑神域: 进击篇", "刀剑神域__进击篇"},
		{"Re:从零开始/异世界?生活*1", "Re_从零开始_异世界_生活_1"},
	}
	for _, tc := range cases {
		out := sanitizeFileName(tc.input)
		if out != tc.expected {
			t.Errorf("expected %q, got %q", tc.expected, out)
		}
	}
}

func TestProgressUpdateAndExportTask(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "app-ops-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	bookID := "test_export_book"
	detail := &model.BookDetail{
		BookSummary: model.BookSummary{
			ID:     bookID,
			Title:  "测试导出小说",
			Author: "测试作者",
		},
	}
	_ = store.SaveBookDetail(detail)

	catalog := &model.BookCatalog{
		BookID: bookID,
		Volumes: []model.Volume{
			{
				ID:    "v1",
				Title: "第一卷",
				Chapters: []model.Chapter{
					{ID: "c1", Title: "第一章"},
				},
			},
		},
	}
	_ = store.SaveCatalog(catalog)

	ch := &model.ChapterContent{
		BookID: bookID,
		ID:     "c1",
		Title:  "第一章",
		Elements: []model.ContentElement{
			{Type: model.ContentTypeText, Text: "正文第一段内容。"},
		},
	}
	_ = store.SaveChapter(ch)

	app := NewAppModel(store, nil)
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Test progressUpdateMsg handling
	app.Update(progressUpdateMsg{text: "[下载中] 测试下载进行中..."})
	if app.statusText != "[下载中] 测试下载进行中..." {
		t.Errorf("expected statusText to update with progress, got: %s", app.statusText)
	}

	// Test export task with pre-cached book
	cmd := startExportTask(nil, store, bookID, 0, "")
	if cmd == nil {
		t.Fatalf("expected non-nil cmd from startExportTask")
	}

	// Read messages from cmd until done
	timeout := time.After(5 * time.Second)
	completed := false
	for {
		select {
		case <-timeout:
			t.Fatalf("timed out waiting for export task to finish")
		default:
			msg := cmd()
			if msg == nil {
				completed = true
				break
			}
			if pMsg, ok := msg.(progressUpdateMsg); ok {
				app.Update(pMsg)
				if strings.Contains(pMsg.text, "[完成]") {
					completed = true
					break
				}
				cmd = listenProgress(pMsg.sub)
			}
		}
		if completed {
			break
		}
	}

	if !strings.Contains(app.statusText, "[完成]") {
		t.Errorf("expected statusText to indicate [完成], got: %s", app.statusText)
	}

	// Test separate volume export task
	customExportDir := filepath.Join(tmpDir, "custom_exports")
	cmdSplit := startExportTask(nil, store, bookID, common.ExportModeAllVolumesSeparate, customExportDir)
	if cmdSplit == nil {
		t.Fatalf("expected non-nil cmd from startExportTask with split mode")
	}

	timeoutSplit := time.After(5 * time.Second)
	completedSplit := false
	for {
		select {
		case <-timeoutSplit:
			t.Fatalf("timed out waiting for split export task to finish")
		default:
			msg := cmdSplit()
			if msg == nil {
				completedSplit = true
				break
			}
			if pMsg, ok := msg.(progressUpdateMsg); ok {
				app.Update(pMsg)
				if strings.Contains(pMsg.text, "[完成]") {
					completedSplit = true
					break
				}
				cmdSplit = listenProgress(pMsg.sub)
			}
		}
		if completedSplit {
			break
		}
	}

	if !strings.Contains(app.statusText, "[完成]") || !strings.Contains(app.statusText, "全部分卷") {
		t.Errorf("expected statusText to indicate split volume completion, got: %s", app.statusText)
	}
}

func TestSettingsEditPathAndClearConfirmation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "settings-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	app := NewAppModel(store, nil)
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	app.Update(common.SwitchViewMsg{Target: common.ViewSettings})

	// 1. Cursor is at 0 (Cache Dir). Press Enter to edit
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	viewStr := app.View()
	if !strings.Contains(viewStr, "编辑路径") || !strings.Contains(viewStr, "新路径:") {
		t.Errorf("expected view to indicate editing mode, got: %s", viewStr)
	}

	// Clear existing value and type new cache path
	app.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	newCacheDir := filepath.Join(tmpDir, "new_cache_dir")
	for _, ch := range newCacheDir {
		app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	// Press Enter to save
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if store.BaseDir() != newCacheDir {
		t.Errorf("expected store baseDir to be %s, got %s", newCacheDir, store.BaseDir())
	}
	if app.settingsView.CacheDir() != newCacheDir {
		t.Errorf("expected settingsView cacheDir to be %s, got %s", newCacheDir, app.settingsView.CacheDir())
	}

	// 2. Cursor down to 1 (Export Dir). Press Enter to edit
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	newExportDir := filepath.Join(tmpDir, "new_export_dir")
	for _, ch := range newExportDir {
		app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if app.settingsView.ExportDir() != newExportDir {
		t.Errorf("expected settingsView exportDir to be %s, got %s", newExportDir, app.settingsView.ExportDir())
	}

	// 3. Move down to 5 (Clear All Cache)
	for i := 0; i < 4; i++ {
		app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	}
	if app.settingsView.Cursor() != 5 {
		t.Fatalf("expected cursor at 5, got %d", app.settingsView.Cursor())
	}

	// Press Enter to trigger clear confirmation
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	confirmView := app.View()
	if !strings.Contains(confirmView, "确认清空") {
		t.Errorf("expected confirm view to contain '确认清空'")
	}
	if !strings.Contains(confirmView, "[y]") || !strings.Contains(confirmView, "[n/Esc]") {
		t.Errorf("expected explicit [y] and [n/Esc] instructions in clear confirmation")
	}

	// Press 'n' to cancel
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	afterCancel := app.View()
	if strings.Contains(afterCancel, "[清空确认]") {
		t.Errorf("expected confirmation banner to be dismissed after 'n'")
	}

	// Press Enter again and press 'y' to confirm
	app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	afterClear := app.View()
	if strings.Contains(afterClear, "[清空确认]") {
		t.Errorf("expected confirmation banner to be dismissed after 'y'")
	}
}
