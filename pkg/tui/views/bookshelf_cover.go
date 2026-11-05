package views

import (
	tea "github.com/charmbracelet/bubbletea"

	termimage "github.com/SakagamiJun/lightnovel-tui/pkg/image"
	"github.com/SakagamiJun/lightnovel-tui/pkg/model"
)

const (
	// bookshelfCoverMinWidth is the minimum terminal width for reserving a
	// right-side cover column on the bookshelf.
	bookshelfCoverMinWidth = 80
	// bookshelfCoverMinPane and bookshelfCoverMaxPane bound the cover column
	// width. The pane grows with wide terminals but stays near the source
	// cover's native resolution.
	bookshelfCoverMinPane = 24
	bookshelfCoverMaxPane = 36
	// bookshelfCoverMinListWidth keeps the three-line book cards readable.
	bookshelfCoverMinListWidth = 40
)

// SetTerminalOutput enables the bookshelf cover preview when the terminal
// supports Kitty Unicode placeholders.
func (v *BookshelfView) SetTerminalOutput(out *termimage.LockedWriter) {
	v.cover.SetTerminalOutput(out)
}

// Activate refreshes the selected book's cover after the bookshelf becomes the
// active tab.
func (v *BookshelfView) Activate() tea.Cmd {
	return v.syncSelectedCover()
}

// bookshelfCoverPaneWidth returns the width reserved for the cover column, or
// zero when the current selection, terminal or layout cannot show a cover.
func (v *BookshelfView) bookshelfCoverPaneWidth() int {
	c := &v.cover
	if !c.capable || c.out == nil || v.store == nil || c.err != nil || !c.HasCandidate() {
		return 0
	}
	if v.width < bookshelfCoverMinWidth || v.height < 8 {
		return 0
	}
	fullWidth := v.width - 2
	target := v.width * 22 / 100
	if target < bookshelfCoverMinPane {
		target = bookshelfCoverMinPane
	}
	if target > bookshelfCoverMaxPane {
		target = bookshelfCoverMaxPane
	}

	// Keep a 2:3 portrait cover inside the body height.
	maxByHeight := (v.height - 4) * 4 / 3
	if maxByHeight < bookshelfCoverMinPane-2 {
		maxByHeight = bookshelfCoverMinPane - 2
	}
	if target > maxByHeight+2 {
		target = maxByHeight + 2
	}
	if fullWidth-target-1 < bookshelfCoverMinListWidth {
		return 0
	}
	return target
}

func (v *BookshelfView) bookshelfCoverArea() (maxCols, maxRows int) {
	paneWidth := v.bookshelfCoverPaneWidth()
	if paneWidth <= 0 {
		return 0, 0
	}
	maxCols = paneWidth - 2
	maxRows = v.height - 4
	if maxRows < 4 {
		maxRows = 4
	}
	if maxRows > 40 {
		maxRows = 40
	}
	return maxCols, maxRows
}

func (v *BookshelfView) selectedBook() *model.BookDetail {
	if v.cursor < 0 || v.cursor >= len(v.books) {
		return nil
	}
	return &v.books[v.cursor]
}

func (v *BookshelfView) startBookshelfCoverLoad() tea.Cmd {
	if v.bookshelfCoverPaneWidth() == 0 {
		return nil
	}
	maxCols, maxRows := v.bookshelfCoverArea()
	return v.cover.StartLoadIfNeeded(maxCols, maxRows)
}

func (v *BookshelfView) updateBookshelfCover(msg CoverLoadedMsg) tea.Cmd {
	maxCols, maxRows := 0, 0
	if v.bookshelfCoverPaneWidth() > 0 {
		maxCols, maxRows = v.bookshelfCoverArea()
	}
	return v.cover.ApplyLoaded(msg, maxCols, maxRows)
}

// HandleCoverLoaded applies a cover result only when the bookshelf owns it.
// AppModel calls this even when another tab is active.
func (v *BookshelfView) HandleCoverLoaded(msg CoverLoadedMsg) tea.Cmd {
	if msg.Owner() != CoverOwnerBookshelf {
		return nil
	}
	return v.updateBookshelfCover(msg)
}

// syncSelectedCover keeps the preview in step with the selected book and the
// current terminal geometry. It is cheap when nothing changed.
func (v *BookshelfView) syncSelectedCover() tea.Cmd {
	c := &v.cover
	if v.showMoveModal || v.showStatsModal || v.confirmDelete {
		return nil
	}

	book := v.selectedBook()
	if book == nil || v.store == nil {
		if c.bookID == "" && !c.HasCandidate() && c.imageID == 0 && !c.loading {
			return nil
		}
		c.bookID = ""
		return c.Reset()
	}

	if c.bookID != book.ID {
		cleanup := c.Reset()
		path := v.store.FindCoverPath(book.ID)
		c.Configure(v.store, v.src, book.ID, book.CoverURL, path, CoverOwnerBookshelf)
		load := v.startBookshelfCoverLoad()
		switch {
		case cleanup == nil:
			return load
		case load == nil:
			return cleanup
		default:
			return tea.Batch(cleanup, load)
		}
	}

	// The selected book did not change; refresh metadata that may have arrived
	// with a later reload.
	c.src = v.src
	c.store = v.store
	if c.url == "" {
		c.url = book.CoverURL
	}
	if c.path == "" {
		if path := v.store.FindCoverPath(book.ID); path != "" {
			c.path = path
		}
	}

	if v.bookshelfCoverPaneWidth() == 0 {
		return nil
	}
	if c.imageID == 0 {
		return v.startBookshelfCoverLoad()
	}
	maxCols, maxRows := v.bookshelfCoverArea()
	return c.Reflow(maxCols, maxRows)
}

func (v *BookshelfView) renderBookshelfCoverPane(paneWidth int) string {
	if paneWidth <= 2 {
		return ""
	}
	return v.cover.Render(paneWidth - 2)
}
