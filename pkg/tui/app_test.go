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
	if !strings.Contains(lines[2], "在线小说检索") {
		t.Errorf("expected line 2 to have search title, got: %s", lines[2])
	}
	// Line 3: Input box
	// Line 5: Stats ("共检索到 50 条结果")
	foundStats := false
	for _, l := range lines[:8] {
		if strings.Contains(l, "共检索到 50 条结果") {
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
