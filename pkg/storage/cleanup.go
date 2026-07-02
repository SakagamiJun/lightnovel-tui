package storage

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BookStorageItem holds the storage usage details for a single book.
type BookStorageItem struct {
	BookID     string `json:"book_id"`
	Title      string `json:"title"`
	TextBytes  int64  `json:"text_bytes"`
	ImageBytes int64  `json:"image_bytes"`
	TotalBytes int64  `json:"total_bytes"`
}

// StorageBreakdown provides a granular breakdown of local disk consumption.
type StorageBreakdown struct {
	TotalBytes int64             `json:"total_bytes"`
	TextBytes  int64             `json:"text_bytes"`
	ImageBytes int64             `json:"image_bytes"`
	EpubBytes  int64             `json:"epub_bytes"`
	BookItems  []BookStorageItem `json:"book_items"`
}

// GetStorageBreakdown analyzes local cache and export directories to provide
// itemized disk consumption for text, images, and EPUB files.
func (s *Storage) GetStorageBreakdown(exportDir string) (*StorageBreakdown, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	breakdown := &StorageBreakdown{
		BookItems: make([]BookStorageItem, 0),
	}

	booksRoot := filepath.Join(s.baseDir, "books")
	entries, err := os.ReadDir(booksRoot)
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			bookID := entry.Name()
			bookDir := filepath.Join(booksRoot, bookID)

			title := bookID
			detail, err := s.LoadBookDetail(bookID)
			if err == nil && detail != nil && detail.Title != "" {
				title = detail.Title
			}

			var bookTextBytes int64
			var bookImageBytes int64

			// Measure root files in book directory (detail.json, catalog.json, progress.json)
			if bookFiles, err := os.ReadDir(bookDir); err == nil {
				for _, bf := range bookFiles {
					if !bf.IsDir() {
						if fi, err := bf.Info(); err == nil {
							bookTextBytes += fi.Size()
						}
					}
				}
			}

			// Measure chapters directory
			chaptersDir := filepath.Join(bookDir, "chapters")
			if chFiles, err := os.ReadDir(chaptersDir); err == nil {
				for _, cf := range chFiles {
					if !cf.IsDir() {
						if fi, err := cf.Info(); err == nil {
							bookTextBytes += fi.Size()
						}
					}
				}
			}

			// Measure images directory
			imagesDir := filepath.Join(bookDir, "images")
			if imgFiles, err := os.ReadDir(imagesDir); err == nil {
				for _, imf := range imgFiles {
					if !imf.IsDir() {
						if fi, err := imf.Info(); err == nil {
							bookImageBytes += fi.Size()
						}
					}
				}
			}

			item := BookStorageItem{
				BookID:     bookID,
				Title:      title,
				TextBytes:  bookTextBytes,
				ImageBytes: bookImageBytes,
				TotalBytes: bookTextBytes + bookImageBytes,
			}
			breakdown.BookItems = append(breakdown.BookItems, item)
			breakdown.TextBytes += bookTextBytes
			breakdown.ImageBytes += bookImageBytes
		}
	}

	// Sort book items by total size descending
	sort.Slice(breakdown.BookItems, func(i, j int) bool {
		return breakdown.BookItems[i].TotalBytes > breakdown.BookItems[j].TotalBytes
	})

	// Measure exported EPUB files
	if exportDir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			exportDir = filepath.Join(home, ".lnr", "exports")
		}
	}
	if exportDir != "" {
		if epubFiles, err := os.ReadDir(exportDir); err == nil {
			for _, ef := range epubFiles {
				if !ef.IsDir() && strings.HasSuffix(strings.ToLower(ef.Name()), ".epub") {
					if fi, err := ef.Info(); err == nil {
						breakdown.EpubBytes += fi.Size()
					}
				}
			}
		}
	}

	breakdown.TotalBytes = breakdown.TextBytes + breakdown.ImageBytes + breakdown.EpubBytes
	return breakdown, nil
}

// CleanImagesOnly deletes illustration images across all cached books to reclaim space.
// If keepCovers is true, cover images (cover.*) are preserved.
// Returns the total number of bytes freed.
func (s *Storage) CleanImagesOnly(keepCovers bool) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var freedBytes int64
	booksRoot := filepath.Join(s.baseDir, "books")
	entries, err := os.ReadDir(booksRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		imagesDir := filepath.Join(booksRoot, entry.Name(), "images")
		imgFiles, err := os.ReadDir(imagesDir)
		if err != nil {
			continue
		}

		for _, img := range imgFiles {
			if img.IsDir() {
				continue
			}
			name := strings.ToLower(img.Name())
			if keepCovers && strings.HasPrefix(name, "cover.") {
				continue
			}
			if fi, err := img.Info(); err == nil {
				freedBytes += fi.Size()
			}
			_ = os.Remove(filepath.Join(imagesDir, img.Name()))
		}
	}

	return freedBytes, nil
}

// CleanEpubsOnly removes all exported .epub files from the export directory.
// Returns the total number of bytes freed.
func (s *Storage) CleanEpubsOnly(exportDir string) (int64, error) {
	if exportDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return 0, err
		}
		exportDir = filepath.Join(home, ".lnr", "exports")
	}

	entries, err := os.ReadDir(exportDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	var freedBytes int64
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".epub") {
			filePath := filepath.Join(exportDir, entry.Name())
			if fi, err := entry.Info(); err == nil {
				freedBytes += fi.Size()
			}
			_ = os.Remove(filePath)
		}
	}

	return freedBytes, nil
}
