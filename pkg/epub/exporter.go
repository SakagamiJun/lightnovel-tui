package epub

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"lnr-core/pkg/downloader"
	"lnr-core/pkg/storage"
)

// Exporter manages creating EPUB files from local cache or downloading missing content on demand.
type Exporter struct {
	store *storage.Storage
	dl    *downloader.Downloader
}

// NewExporter creates a novel EPUB exporter.
func NewExporter(store *storage.Storage, dl *downloader.Downloader) *Exporter {
	return &Exporter{
		store: store,
		dl:    dl,
	}
}

// ExportVolume exports a specific volume into a standalone EPUB file.
func (e *Exporter) ExportVolume(ctx context.Context, bookID string, volumeIndex int, outputPath string) (string, error) {
	detail, err := e.store.LoadBookDetail(bookID)
	if err != nil {
		return "", fmt.Errorf("book detail not cached for ID %s: %w", bookID, err)
	}

	catalog, err := e.store.LoadCatalog(bookID)
	if err != nil {
		return "", fmt.Errorf("catalog not cached for ID %s: %w", bookID, err)
	}

	if volumeIndex < 1 || volumeIndex > len(catalog.Volumes) {
		return "", fmt.Errorf("volume index out of range (1..%d): %d", len(catalog.Volumes), volumeIndex)
	}

	vol := catalog.Volumes[volumeIndex-1]
	title := fmt.Sprintf("%s - %s", detail.Title, vol.Title)
	builder := NewBuilder(bookID+"_"+vol.ID, title, detail.Author, detail.Publisher, detail.Description)

	// Resolve local cover
	coverPath := e.resolveCover(ctx, bookID, detail.CoverURL)
	builder.SetCover(coverPath)

	imageResolver := func(imgURL string) string {
		imgFileName := downloader.ImageNameToFileName(imgURL)
		path := e.store.ImagePath(bookID, imgFileName)
		if _, err := os.Stat(path); err == nil {
			return path
		}
		return ""
	}

	// Add chapters
	for _, ch := range vol.Chapters {
		chContent, err := e.store.LoadChapter(bookID, ch.ID)
		if err != nil {
			return "", fmt.Errorf("chapter %s (%s) not found in cache. Please run download first: %w", ch.Title, ch.ID, err)
		}
		builder.AddChapter(ch.ID, ch.Title, chContent.Elements, imageResolver)
	}

	if outputPath == "" {
		outputPath = fmt.Sprintf("%s_%s.epub", sanitizeFileName(detail.Title), sanitizeFileName(vol.Title))
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to create output file %s: %w", outputPath, err)
	}
	defer f.Close()

	if err := builder.WriteTo(f, imageResolver); err != nil {
		return "", fmt.Errorf("failed to write EPUB: %w", err)
	}

	return outputPath, nil
}

// ExportFullBook exports all volumes into a single consolidated EPUB file.
func (e *Exporter) ExportFullBook(ctx context.Context, bookID string, outputPath string) (string, error) {
	detail, err := e.store.LoadBookDetail(bookID)
	if err != nil {
		return "", fmt.Errorf("book detail not cached for ID %s: %w", bookID, err)
	}

	catalog, err := e.store.LoadCatalog(bookID)
	if err != nil {
		return "", fmt.Errorf("catalog not cached for ID %s: %w", bookID, err)
	}

	builder := NewBuilder(bookID, detail.Title, detail.Author, detail.Publisher, detail.Description)

	coverPath := e.resolveCover(ctx, bookID, detail.CoverURL)
	builder.SetCover(coverPath)

	imageResolver := func(imgURL string) string {
		imgFileName := downloader.ImageNameToFileName(imgURL)
		path := e.store.ImagePath(bookID, imgFileName)
		if _, err := os.Stat(path); err == nil {
			return path
		}
		return ""
	}

	for _, vol := range catalog.Volumes {
		for _, ch := range vol.Chapters {
			chContent, err := e.store.LoadChapter(bookID, ch.ID)
			if err != nil {
				return "", fmt.Errorf("chapter %s (%s) not cached. Please download first: %w", ch.Title, ch.ID, err)
			}
			chTitle := fmt.Sprintf("%s · %s", vol.Title, ch.Title)
			builder.AddChapter(ch.ID, chTitle, chContent.Elements, imageResolver)
		}
	}

	if outputPath == "" {
		outputPath = fmt.Sprintf("%s.epub", sanitizeFileName(detail.Title))
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to create output file %s: %w", outputPath, err)
	}
	defer f.Close()

	if err := builder.WriteTo(f, imageResolver); err != nil {
		return "", fmt.Errorf("failed to write EPUB: %w", err)
	}

	return outputPath, nil
}

func (e *Exporter) resolveCover(ctx context.Context, bookID, coverURL string) string {
	ext := filepath.Ext(coverURL)
	if ext == "" {
		ext = ".jpg"
	}
	candidate := filepath.Join(e.store.BookDir(bookID), "images", "cover"+ext)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	if e.dl != nil && coverURL != "" {
		path, _ := e.dl.DownloadCover(ctx, bookID, coverURL)
		return path
	}
	return ""
}

func sanitizeFileName(name string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		" ", "_",
	)
	return replacer.Replace(name)
}
