package common

// ViewID identifies which view is currently active in TUI.
type ViewID int

const (
	ViewBookshelf ViewID = iota
	ViewExplore
	ViewSearch
	ViewSettings
	ViewCatalog
	ViewReader
)

// SwitchViewMsg requests switching to another view.
type SwitchViewMsg struct {
	Target ViewID
	BookID string
	ChapID string
}

// ErrorMsg delivers an error to the global status bar.
type ErrorMsg error

// StatusMsg delivers a notification text to the global status bar.
type StatusMsg string

const (
	// ExportModeFullBook exports all volumes into a single consolidated EPUB file.
	ExportModeFullBook = 0
	// ExportModeAllVolumesSeparate exports each volume into an individual EPUB file.
	ExportModeAllVolumesSeparate = -1
)
