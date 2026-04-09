package downloader

// ProgressEvent represents progress updates from the downloader.
// Designed to be consumed uniformly by CLI stdout, TUI progress bars, or GUI state.
type ProgressEvent struct {
	BookID      string  `json:"book_id"`
	VolumeID    string  `json:"volume_id"`
	VolumeTitle string  `json:"volume_title"`
	Current     int     `json:"current"`
	Total       int     `json:"total"`
	ItemTitle   string  `json:"item_title"`
	Percentage  float64 `json:"percentage"`
	Message     string  `json:"message,omitempty"`
}

// ProgressHandler is a callback receiving ProgressEvent.
type ProgressHandler func(event ProgressEvent)
