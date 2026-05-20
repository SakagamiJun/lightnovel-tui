package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"lnr-core/pkg/model"
)

// Storage manages local disk caching of books, catalogs, chapters, and images.
type Storage struct {
	baseDir string
	mu      sync.RWMutex
}

// NewStorage initializes local storage directory.
func NewStorage(baseDir string) (*Storage, error) {
	if baseDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			baseDir = "./cache"
		} else {
			baseDir = filepath.Join(home, ".lnr", "cache")
		}
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache dir: %w", err)
	}
	return &Storage{baseDir: baseDir}, nil
}

// BaseDir returns the root cache directory.
func (s *Storage) BaseDir() string {
	return s.baseDir
}

// BookDir returns the path for a given book.
func (s *Storage) BookDir(bookID string) string {
	return filepath.Join(s.baseDir, "books", bookID)
}

// SaveBookDetail persists book details as JSON.
func (s *Storage) SaveBookDetail(detail *model.BookDetail) error {
	dir := s.BookDir(detail.ID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "detail.json"))
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(detail)
}

// LoadBookDetail loads cached book details.
func (s *Storage) LoadBookDetail(bookID string) (*model.BookDetail, error) {
	f, err := os.Open(filepath.Join(s.BookDir(bookID), "detail.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var detail model.BookDetail
	if err := json.NewDecoder(f).Decode(&detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

// SaveCatalog persists the catalog structure.
func (s *Storage) SaveCatalog(catalog *model.BookCatalog) error {
	dir := s.BookDir(catalog.BookID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "catalog.json"))
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(catalog)
}

// LoadCatalog loads cached catalog.
func (s *Storage) LoadCatalog(bookID string) (*model.BookCatalog, error) {
	f, err := os.Open(filepath.Join(s.BookDir(bookID), "catalog.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var catalog model.BookCatalog
	if err := json.NewDecoder(f).Decode(&catalog); err != nil {
		return nil, err
	}
	return &catalog, nil
}

// ChapterPath returns file path for a chapter JSON.
func (s *Storage) ChapterPath(bookID, chapterID string) string {
	return filepath.Join(s.BookDir(bookID), "chapters", chapterID+".json")
}

// SaveChapter saves parsed chapter content.
func (s *Storage) SaveChapter(chapter *model.ChapterContent) error {
	dir := filepath.Join(s.BookDir(chapter.BookID), "chapters")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, chapter.ID+".json"))
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(chapter)
}

// LoadChapter loads cached chapter content.
func (s *Storage) LoadChapter(bookID, chapterID string) (*model.ChapterContent, error) {
	f, err := os.Open(s.ChapterPath(bookID, chapterID))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var ch model.ChapterContent
	if err := json.NewDecoder(f).Decode(&ch); err != nil {
		return nil, err
	}
	return &ch, nil
}

// ImagePath returns the storage path for an image file.
func (s *Storage) ImagePath(bookID, imgFileName string) string {
	return filepath.Join(s.BookDir(bookID), "images", imgFileName)
}

// EnsureImageDir creates images folder if needed.
func (s *Storage) EnsureImageDir(bookID string) error {
	return os.MkdirAll(filepath.Join(s.BookDir(bookID), "images"), 0755)
}

// ListCachedBooks returns all book details that have been cached on disk.
func (s *Storage) ListCachedBooks() ([]model.BookDetail, error) {
	booksRoot := filepath.Join(s.baseDir, "books")
	entries, err := os.ReadDir(booksRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var books []model.BookDetail
	for _, entry := range entries {
		if entry.IsDir() {
			detail, err := s.LoadBookDetail(entry.Name())
			if err == nil && detail != nil {
				books = append(books, *detail)
			}
		}
	}
	return books, nil
}

func (s *Storage) pinnedFilePath() string {
	return filepath.Join(s.baseDir, "pinned.json")
}

// GetPinnedBookIDs returns the list of pinned book IDs.
func (s *Storage) GetPinnedBookIDs() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.pinnedFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	if err := json.Unmarshal(data, &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *Storage) savePinnedBookIDs(ids []string) error {
	data, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return os.WriteFile(s.pinnedFilePath(), data, 0644)
}

// IsBookPinned checks if a book ID is pinned.
func (s *Storage) IsBookPinned(bookID string) bool {
	ids, err := s.GetPinnedBookIDs()
	if err != nil {
		return false
	}
	for _, id := range ids {
		if id == bookID {
			return true
		}
	}
	return false
}

// TogglePinBook toggles pinned status of a book, returning the new status.
func (s *Storage) TogglePinBook(bookID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var ids []string
	data, err := os.ReadFile(s.pinnedFilePath())
	if err == nil {
		_ = json.Unmarshal(data, &ids)
	}

	found := false
	newIDs := make([]string, 0, len(ids)+1)
	for _, id := range ids {
		if id == bookID {
			found = true
		} else {
			newIDs = append(newIDs, id)
		}
	}

	pinnedNow := false
	if !found {
		newIDs = append([]string{bookID}, newIDs...)
		pinnedNow = true
	}

	if err := s.savePinnedBookIDs(newIDs); err != nil {
		return false, err
	}
	return pinnedNow, nil
}

// DeleteBook removes cached book folder and removes book from pinned list.
func (s *Storage) DeleteBook(bookID string) error {
	s.mu.Lock()
	var ids []string
	data, err := os.ReadFile(s.pinnedFilePath())
	if err == nil {
		_ = json.Unmarshal(data, &ids)
		newIDs := make([]string, 0, len(ids))
		for _, id := range ids {
			if id != bookID {
				newIDs = append(newIDs, id)
			}
		}
		_ = s.savePinnedBookIDs(newIDs)
	}
	s.mu.Unlock()

	return os.RemoveAll(s.BookDir(bookID))
}

// CalculateCacheSize sums the total size of all files in the cache.
func (s *Storage) CalculateCacheSize() (int64, error) {
	var total int64
	err := filepath.Walk(s.baseDir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// ClearCache removes all cached books and pinned list.
func (s *Storage) ClearCache() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_ = os.Remove(s.pinnedFilePath())
	booksRoot := filepath.Join(s.baseDir, "books")
	return os.RemoveAll(booksRoot)
}
