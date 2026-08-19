package source

import (
	"context"

	"github.com/SakagamiJun/lnovel_tui/pkg/model"
)

// SearchType defines the search criterion (by title, author, etc.)
type SearchType string

const (
	SearchTypeTitle  SearchType = "articlename"
	SearchTypeAuthor SearchType = "author"
)

// ToplistType defines the type of explore leaderboard.
type ToplistType string

const (
	ToplistHot        ToplistType = "allvisit"   // 热门轻小说 (总榜)
	ToplistAnime      ToplistType = "anime"      // 动画化作品
	ToplistLastUpdate ToplistType = "lastupdate" // 今日更新
	ToplistPostDate   ToplistType = "postdate"   // 新书一览
	ToplistCompleted  ToplistType = "completed"  // 完结全本
)

// ToplistNameMap maps ToplistType to a human-readable title.
var ToplistNameMap = map[ToplistType]string{
	ToplistHot:        "热门轻小说",
	ToplistAnime:      "动画化作品",
	ToplistLastUpdate: "今日更新",
	ToplistPostDate:   "新书一览",
	ToplistCompleted:  "完结全本",
}

// PublisherInfo defines publishing house metadata.
type PublisherInfo struct {
	ClassID int    `json:"class_id"`
	Name    string `json:"name"`
}

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

	// GetToplist fetches leaderboard books.
	GetToplist(ctx context.Context, tType ToplistType, page int) ([]model.BookSummary, int, error)

	// GetTags returns the supported categorized novel tags.
	GetTags() []string

	// GetTagBooks searches books tagged with the specified tag.
	GetTagBooks(ctx context.Context, tag string, page int) ([]model.BookSummary, int, error)

	// GetPublishers returns the supported publishing houses / libraries.
	GetPublishers() []PublisherInfo

	// GetPublisherBooks searches novels under a specific publishing house / library.
	GetPublisherBooks(ctx context.Context, classID int, page int) ([]model.BookSummary, int, error)

	// EnrichDescriptions enhances novel summaries with full descriptions if available.
	EnrichDescriptions(ctx context.Context, books []model.BookSummary, maxWorkers int) []model.BookSummary
}
