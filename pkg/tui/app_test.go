package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"lnr-core/pkg/model"
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
		if !strings.Contains(lines[0], "LNR") {
			t.Errorf("expected line 0 to contain LNR header, got: %s", lines[0])
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
	if !strings.Contains(lines[0], "LNR") {
		t.Errorf("expected line 0 to have LNR header, got: %s", lines[0])
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
	if !strings.Contains(lines[0], "LNR") {
		t.Errorf("expected line 0 to have LNR header, got: %s", lines[0])
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
