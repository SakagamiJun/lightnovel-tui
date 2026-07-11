package reader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"lnr-core/pkg/model"
	"lnr-core/pkg/storage"
	"lnr-core/pkg/text"
)

// ReadingProgress records the reading location of a book.
type ReadingProgress struct {
	BookID     string `json:"book_id"`
	VolumeID   string `json:"volume_id"`
	ChapterID  string `json:"chapter_id"`
	LineIndex  int    `json:"line_index"`
	LastReadAt int64  `json:"last_read_at"`
}

// Reader provides high-level reading navigation across cached chapters,
// preparing text lines/images for consumption by TUI (Bubble Tea) or GUI (Fyne/Wails).
type Reader struct {
	store         *storage.Storage
	bookID        string
	catalog       *model.BookCatalog
	currVolume    *model.Volume
	currChapter   *model.ChapterContent
	lines         []string
	illustrations []string
	traditional   bool
	rules         []text.FormattingRule
	sessionStart  time.Time
}

// NewReader initializes a Reader engine for a cached book.
func NewReader(store *storage.Storage, bookID string) (*Reader, error) {
	catalog, err := store.LoadCatalog(bookID)
	if err != nil {
		return nil, fmt.Errorf("failed to load catalog for reader: %w", err)
	}

	var rules []text.FormattingRule
	if store != nil {
		rules, _ = store.LoadRules()
	}

	return &Reader{
		store:         store,
		bookID:        bookID,
		catalog:       catalog,
		illustrations: make([]string, 0),
		rules:         rules,
		sessionStart:  time.Now(),
	}, nil
}

// LoadChapter loads a chapter into reader memory, pre-splitting paragraphs into readable lines.
func (r *Reader) LoadChapter(chapterID string) (*model.ChapterContent, error) {
	ch, err := r.store.LoadChapter(r.bookID, chapterID)
	if err != nil {
		return nil, fmt.Errorf("chapter %s not found in cache: %w", chapterID, err)
	}

	r.currChapter = ch
	r.sessionStart = time.Now()
	r.lines, r.illustrations = text.FormatNovelLinesWithRules(ch.Elements, r.traditional, r.bookID, r.rules)
	return ch, nil
}

// SetRules updates formatting rules and refreshes current chapter lines.
func (r *Reader) SetRules(rules []text.FormattingRule) {
	r.rules = rules
	if r.currChapter != nil {
		r.lines, r.illustrations = text.FormatNovelLinesWithRules(r.currChapter.Elements, r.traditional, r.bookID, r.rules)
	}
}

// SetTraditional sets whether to format text in Traditional Chinese.
func (r *Reader) SetTraditional(traditional bool) {
	if r.traditional == traditional {
		return
	}
	r.traditional = traditional
	if r.currChapter != nil {
		r.lines, r.illustrations = text.FormatNovelLinesWithRules(r.currChapter.Elements, r.traditional, r.bookID, r.rules)
	}
}

// ToggleTraditional toggles between Simplified and Traditional Chinese.
func (r *Reader) ToggleTraditional() {
	r.SetTraditional(!r.traditional)
}

// IsTraditional returns whether Traditional Chinese conversion is active.
func (r *Reader) IsTraditional() bool {
	return r.traditional
}

// Lines returns the split text lines for the current chapter.
func (r *Reader) Lines() []string {
	return r.lines
}

// Illustrations returns the list of illustration image URLs in the current chapter.
func (r *Reader) Illustrations() []string {
	return r.illustrations
}

// IllustrationPath returns the local cached path for a given illustration image URL.
func (r *Reader) IllustrationPath(imgURL string) string {
	return r.store.IllustrationPath(r.bookID, imgURL)
}

// BookID returns the current book ID.
func (r *Reader) BookID() string {
	return r.bookID
}

// CurrentChapter returns the active chapter content.
func (r *Reader) CurrentChapter() *model.ChapterContent {
	return r.currChapter
}

// Catalog returns the book catalog.
func (r *Reader) Catalog() *model.BookCatalog {
	return r.catalog
}

// SaveProgress records the reading bookmark to disk and updates reading statistics.
func (r *Reader) SaveProgress(progress *ReadingProgress) error {
	dir := r.store.BookDir(r.bookID)
	f, err := os.Create(filepath.Join(dir, "progress.json"))
	if err != nil {
		return err
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(progress); err != nil {
		return err
	}

	// Update stats if storage is available
	if r.store != nil {
		elapsedSeconds := int(time.Since(r.sessionStart).Seconds())
		r.sessionStart = time.Now()

		bookTitle := ""
		if detail, err := r.store.LoadBookDetail(r.bookID); err == nil && detail != nil {
			bookTitle = detail.Title
		}
		chapTitle := ""
		if r.currChapter != nil {
			chapTitle = r.currChapter.Title
		}
		words := progress.LineIndex * 35
		if words < 0 {
			words = 0
		}
		_ = r.store.RecordReading(r.bookID, bookTitle, progress.ChapterID, chapTitle, elapsedSeconds, words, progress.LineIndex, len(r.lines))
	}

	return nil
}

// LoadProgress loads the saved bookmark.
func (r *Reader) LoadProgress() (*ReadingProgress, error) {
	f, err := os.Open(filepath.Join(r.store.BookDir(r.bookID), "progress.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var p ReadingProgress
	if err := json.NewDecoder(f).Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

// ChapterPosition returns (currentChapterIndex, totalChaptersInVolume, volumeTitle).
// Index is 1-based.
func (r *Reader) ChapterPosition() (int, int, string) {
	if r.catalog == nil || r.currChapter == nil {
		return 1, 1, ""
	}
	for _, vol := range r.catalog.Volumes {
		for i, ch := range vol.Chapters {
			if ch.ID == r.currChapter.ID {
				return i + 1, len(vol.Chapters), vol.Title
			}
		}
	}
	return 1, 1, ""
}

// NextChapter returns the next chapter ID and title if available.
func (r *Reader) NextChapter() (string, string, bool) {
	if r.catalog == nil || r.currChapter == nil {
		return "", "", false
	}
	found := false
	for _, vol := range r.catalog.Volumes {
		for _, ch := range vol.Chapters {
			if found {
				return ch.ID, ch.Title, true
			}
			if ch.ID == r.currChapter.ID {
				found = true
			}
		}
	}
	return "", "", false
}

// PrevChapter returns the previous chapter ID and title if available.
func (r *Reader) PrevChapter() (string, string, bool) {
	if r.catalog == nil || r.currChapter == nil {
		return "", "", false
	}
	var prevID, prevTitle string
	for _, vol := range r.catalog.Volumes {
		for _, ch := range vol.Chapters {
			if ch.ID == r.currChapter.ID {
				if prevID != "" {
					return prevID, prevTitle, true
				}
				return "", "", false
			}
			prevID = ch.ID
			prevTitle = ch.Title
		}
	}
	return "", "", false
}
