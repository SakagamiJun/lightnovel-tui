package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"lnr-cli/pkg/model"
)

// Storage manages local disk caching of books, catalogs, chapters, and images.
type Storage struct {
	baseDir string
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
