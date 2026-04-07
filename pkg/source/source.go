package source

import (
	"context"

	"lnr-core/pkg/model"
)

// SearchType defines the search criterion (by title, author, etc.)
type SearchType string

const (
	SearchTypeTitle  SearchType = "articlename"
	SearchTypeAuthor SearchType = "author"
)

// DataSource provides an abstract interface to fetch light novel resources.
type DataSource interface {
	// Name returns the identifier of the data source (e.g. "wenku8")
	Name() string

	// Search searches books matching the keyword using the given search type.
	Search(ctx context.Context, searchType SearchType, keyword string, page int) ([]model.BookSummary, int, error)

	// GetBookDetail fetches detailed metadata of a specific book.
	GetBookDetail(ctx context.Context, bookID string) (*model.BookDetail, error)

	// GetCatalog fetches all volumes and chapters of a specific book.
	GetCatalog(ctx context.Context, bookID string) (*model.BookCatalog, error)

	// GetChapterContent fetches the content and illustrations of a specific chapter.
	GetChapterContent(ctx context.Context, bookID, chapterID string) (*model.ChapterContent, error)
}
