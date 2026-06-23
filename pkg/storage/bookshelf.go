package storage

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"lnr-core/pkg/model"
	"lnr-core/pkg/source"
)

// SortCriteria defines the sorting metric for books on the bookshelf.
type SortCriteria int

const (
	SortByLastRead SortCriteria = iota
	SortByLastUpdated
	SortByTitle
	SortBySize
)

// SortCriteriaNames maps SortCriteria to human-readable names.
var SortCriteriaNames = map[SortCriteria]string{
	SortByLastRead:    "最近阅读",
	SortByLastUpdated: "更新时间",
	SortByTitle:       "书名排序",
	SortBySize:        "占用体积",
}

// BookUpdateInfo contains update comparison information for a book.
type BookUpdateInfo struct {
	BookID          string
	Title           string
	HasUpdate       bool
	NewChapterCount int
	LocalChapters   int
	RemoteChapters  int
	RemoteCatalog   *model.BookCatalog
}

// GetBookSize calculates the total byte size of a cached book on disk.
func (s *Storage) GetBookSize(bookID string) int64 {
	dir := s.BookDir(bookID)
	var size int64
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			info, err := d.Info()
			if err == nil {
				size += info.Size()
			}
		}
		return nil
	})
	return size
}

// GetBookLastReadTime returns the Unix timestamp when the book was last read, or 0 if unread.
func (s *Storage) GetBookLastReadTime(bookID string) int64 {
	pPath := filepath.Join(s.BookDir(bookID), "progress.json")
	data, err := os.ReadFile(pPath)
	if err != nil {
		return 0
	}
	type progressRecord struct {
		LastReadAt int64 `json:"last_read_at"`
	}
	var p progressRecord
	if err := json.Unmarshal(data, &p); err != nil {
		return 0
	}
	return p.LastReadAt
}

// SortBooks sorts a slice of books taking into account pinned status and the specified SortCriteria.
func (s *Storage) SortBooks(books []model.BookDetail, criteria SortCriteria) {
	pinnedMap := make(map[string]bool)
	if pinnedIDs, err := s.GetPinnedBookIDs(); err == nil {
		for _, id := range pinnedIDs {
			pinnedMap[id] = true
		}
	}

	// Pre-cache sort metadata to avoid repeatedly hitting disk in O(N log N) comparisons
	lastReadCache := make(map[string]int64, len(books))
	sizeCache := make(map[string]int64, len(books))

	if criteria == SortByLastRead {
		for _, b := range books {
			lastReadCache[b.ID] = s.GetBookLastReadTime(b.ID)
		}
	} else if criteria == SortBySize {
		for _, b := range books {
			sizeCache[b.ID] = s.GetBookSize(b.ID)
		}
	}

	sort.SliceStable(books, func(i, j int) bool {
		bi, bj := books[i], books[j]
		pi, pj := pinnedMap[bi.ID], pinnedMap[bj.ID]
		if pi != pj {
			return pi // pinned books always come first
		}

		switch criteria {
		case SortByLastRead:
			ri, rj := lastReadCache[bi.ID], lastReadCache[bj.ID]
			if ri != rj {
				return ri > rj // most recent first
			}
			return bi.ID < bj.ID

		case SortByLastUpdated:
			if bi.LastUpdated != bj.LastUpdated {
				return bi.LastUpdated > bj.LastUpdated // newest date first
			}
			return bi.ID < bj.ID

		case SortByTitle:
			return strings.Compare(bi.Title, bj.Title) < 0

		case SortBySize:
			si, sj := sizeCache[bi.ID], sizeCache[bj.ID]
			if si != sj {
				return si > sj // largest first
			}
			return bi.ID < bj.ID

		default:
			return bi.ID < bj.ID
		}
	})
}

// CheckBookUpdate compares local cached catalog with the remote catalog to check for new chapters.
func (s *Storage) CheckBookUpdate(ctx context.Context, src source.DataSource, bookID string) (*BookUpdateInfo, error) {
	detail, err := s.LoadBookDetail(bookID)
	title := bookID
	if err == nil && detail != nil {
		title = detail.Title
	}

	localCatalog, err := s.LoadCatalog(bookID)
	if err != nil {
		return nil, err
	}

	localChapIDs := make(map[string]bool)
	localCount := 0
	for _, vol := range localCatalog.Volumes {
		for _, ch := range vol.Chapters {
			localCount++
			localChapIDs[ch.ID] = true
		}
	}

	remoteCatalog, err := src.GetCatalog(ctx, bookID)
	if err != nil {
		return nil, err
	}

	remoteCount := 0
	newCount := 0
	for _, vol := range remoteCatalog.Volumes {
		for _, ch := range vol.Chapters {
			remoteCount++
			if !localChapIDs[ch.ID] {
				newCount++
			}
		}
	}

	return &BookUpdateInfo{
		BookID:          bookID,
		Title:           title,
		HasUpdate:       newCount > 0 || remoteCount > localCount,
		NewChapterCount: newCount,
		LocalChapters:   localCount,
		RemoteChapters:  remoteCount,
		RemoteCatalog:   remoteCatalog,
	}, nil
}
