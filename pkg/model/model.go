package model

// BookSummary represents lightweight metadata for search results or lists.
type BookSummary struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Subtitle    string   `json:"subtitle,omitempty"`
	Author      string   `json:"author"`
	CoverURL    string   `json:"cover_url"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Publisher   string   `json:"publisher,omitempty"`
	WordCount   int      `json:"word_count,omitempty"`
	LastUpdated string   `json:"last_updated,omitempty"`
	IsComplete  bool     `json:"is_complete"`
}

// BookDetail contains complete book metadata including tags and last update.
type BookDetail struct {
	BookSummary
}

// Chapter represents a single chapter in a volume.
type Chapter struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Volume represents a volume/subdivision containing a list of chapters.
type Volume struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Chapters []Chapter `json:"chapters"`
}

// BookCatalog contains the volume and chapter hierarchy of a book.
type BookCatalog struct {
	BookID  string   `json:"book_id"`
	Volumes []Volume `json:"volumes"`
}

// ContentType defines whether an element is text or an image.
type ContentType string

const (
	ContentTypeText  ContentType = "text"
	ContentTypeImage ContentType = "image"
)

// ContentElement represents a paragraph of text or an image.
type ContentElement struct {
	Type ContentType `json:"type"`
	Text string      `json:"text,omitempty"`
	URL  string      `json:"url,omitempty"`
}

// ChapterContent contains the full parsed text and image URLs of a chapter.
type ChapterContent struct {
	ID          string           `json:"id"`
	BookID      string           `json:"book_id"`
	Title       string           `json:"title"`
	Elements    []ContentElement `json:"elements"`
	PrevChapter string           `json:"prev_chapter,omitempty"`
	NextChapter string           `json:"next_chapter,omitempty"`
}
