package views

import (
	tea "github.com/charmbracelet/bubbletea"

	termimage "github.com/SakagamiJun/lightnovel-tui/pkg/image"
)

const (
	// catalogCoverMinWidth is the minimum terminal width for reserving a
	// right-side cover column on the catalog page.
	catalogCoverMinWidth = 80
	// catalogCoverPaneWidth includes the border of the cover box.
	catalogCoverPaneWidth = 24
	// catalogCoverImageWidth is the placeholder grid budget inside the border.
	catalogCoverImageWidth = catalogCoverPaneWidth - 2
	// catalogCoverMinListWidth keeps the chapter list readable when the cover
	// column is visible.
	catalogCoverMinListWidth = 40
)

// SetTerminalOutput enables the catalog cover preview when the terminal
// supports Kitty Unicode placeholders.
func (v *CatalogView) SetTerminalOutput(out *termimage.LockedWriter) {
	v.cover.SetTerminalOutput(out)
}

// coverPaneWidth returns the width reserved for the cover column, or zero when
// the current book, terminal or layout cannot show a cover.
func (v *CatalogView) coverPaneWidth() int {
	c := &v.cover
	if !c.capable || c.out == nil || v.store == nil || v.detail == nil || c.err != nil {
		return 0
	}
	if !c.HasCandidate() {
		return 0
	}
	if v.width < catalogCoverMinWidth || v.height < 8 {
		return 0
	}
	fullWidth := v.width - 2
	if fullWidth-catalogCoverPaneWidth-1 < catalogCoverMinListWidth {
		return 0
	}
	return catalogCoverPaneWidth
}

// coverArea returns the maximum placeholder grid that fits in the right
// column. The cover box border and the one-line catalog header are excluded.
func (v *CatalogView) coverArea() (maxCols, maxRows int) {
	maxCols = catalogCoverImageWidth
	maxRows = v.height - 4
	if maxRows < 4 {
		maxRows = 4
	}
	if maxRows > 40 {
		maxRows = 40
	}
	return maxCols, maxRows
}

func (v *CatalogView) resetCoverState() tea.Cmd {
	return v.cover.Reset()
}

func (v *CatalogView) startCoverLoadIfNeeded() tea.Cmd {
	if v.coverPaneWidth() == 0 {
		return nil
	}
	maxCols, maxRows := v.coverArea()
	return v.cover.StartLoadIfNeeded(maxCols, maxRows)
}

func (v *CatalogView) updateCover(msg CoverLoadedMsg) tea.Cmd {
	maxCols, maxRows := 0, 0
	if v.coverPaneWidth() > 0 {
		maxCols, maxRows = v.coverArea()
	}
	return v.cover.ApplyLoaded(msg, maxCols, maxRows)
}

// handleCoverResize adapts the pane after a terminal resize. When the pane is
// not eligible the already-transmitted image is kept hidden in terminal memory
// so shrinking and re-expanding the window does not require a reload.
func (v *CatalogView) handleCoverResize() tea.Cmd {
	if v.coverPaneWidth() == 0 {
		return nil
	}
	if v.cover.imageID == 0 {
		return v.startCoverLoadIfNeeded()
	}
	maxCols, maxRows := v.coverArea()
	return v.cover.Reflow(maxCols, maxRows)
}

func (v *CatalogView) renderCoverPane() string {
	return v.cover.Render(catalogCoverImageWidth)
}

// HandleCoverLoaded applies a cover result only when the catalog view owns it.
// AppModel calls this even when another tab is active.
func (v *CatalogView) HandleCoverLoaded(msg CoverLoadedMsg) tea.Cmd {
	if msg.Owner() != CoverOwnerCatalog {
		return nil
	}
	return v.updateCover(msg)
}
