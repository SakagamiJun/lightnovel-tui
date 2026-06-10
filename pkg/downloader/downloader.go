package downloader

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"lnr-core/internal/client"
	"lnr-core/pkg/model"
	"lnr-core/pkg/source"
	"lnr-core/pkg/storage"
)

// bufferPool provides reusable 32KB buffers to avoid allocation churn and keep memory minimal.
var bufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, 32*1024)
		return &buf
	},
}

type Downloader struct {
	src        source.DataSource
	store      *storage.Storage
	client     *client.HttpClient
	maxWorkers int
}

// NewDownloader creates a memory-conscious novel downloader.
func NewDownloader(src source.DataSource, store *storage.Storage) (*Downloader, error) {
	c, err := client.NewHttpClient()
	if err != nil {
		return nil, err
	}
	return &Downloader{
		src:        src,
		store:      store,
		client:     c,
		maxWorkers: 3, // Keep concurrent downloads restrained to prevent memory spikes & bans
	}, nil
}

// DownloadVolume downloads all chapters and images within a specific volume.
func (d *Downloader) DownloadVolume(ctx context.Context, bookID string, volume *model.Volume, handler ProgressHandler) error {
	total := len(volume.Chapters)
	if total == 0 {
		return nil
	}

	if err := d.store.EnsureImageDir(bookID); err != nil {
		return err
	}

	for i, chInfo := range volume.Chapters {
		if handler != nil {
			handler(ProgressEvent{
				BookID:      bookID,
				VolumeID:    volume.ID,
				VolumeTitle: volume.Title,
				Current:     i + 1,
				Total:       total,
				ItemTitle:   chInfo.Title,
				Percentage:  float64(i+1) / float64(total) * 100,
			})
		}

		// Check if chapter already cached
		chContent, err := d.store.LoadChapter(bookID, chInfo.ID)
		if err != nil {
			// Fetch from network
			chContent, err = d.src.GetChapterContent(ctx, bookID, chInfo.ID)
			if err != nil {
				return fmt.Errorf("failed to fetch chapter %s (%s): %w", chInfo.Title, chInfo.ID, err)
			}
			if err := d.store.SaveChapter(chContent); err != nil {
				return fmt.Errorf("failed to cache chapter: %w", err)
			}
			// Small delay to be polite to the host
			time.Sleep(300 * time.Millisecond)
		}

		// Download illustrations within this chapter
		for _, el := range chContent.Elements {
			if el.Type == model.ContentTypeImage && el.URL != "" {
				_ = d.downloadImage(ctx, bookID, el.URL)
			}
		}
	}

	return nil
}

// DownloadCover downloads and caches the book cover.
func (d *Downloader) DownloadCover(ctx context.Context, bookID, coverURL string) (string, error) {
	if coverURL == "" {
		return "", nil
	}
	if err := d.store.EnsureImageDir(bookID); err != nil {
		return "", err
	}
	ext := filepath.Ext(coverURL)
	if ext == "" {
		ext = ".jpg"
	}
	destPath := filepath.Join(d.store.BookDir(bookID), "images", "cover"+ext)
	if _, err := os.Stat(destPath); err == nil {
		return destPath, nil
	}

	stream, err := d.client.GetRawStream(ctx, coverURL)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	defer out.Close()

	bufPtr := bufferPool.Get().(*[]byte)
	defer bufferPool.Put(bufPtr)

	_, err = io.CopyBuffer(out, stream, *bufPtr)
	if err != nil {
		return "", err
	}
	return destPath, nil
}

// downloadImage downloads an illustration image using streaming and buffer reuse.
func (d *Downloader) downloadImage(ctx context.Context, bookID, imgURL string) error {
	imgName := ImageNameToFileName(imgURL)
	destPath := d.store.ImagePath(bookID, imgName)

	if _, err := os.Stat(destPath); err == nil {
		return nil // already downloaded
	}

	stream, err := d.client.GetRawStream(ctx, imgURL)
	if err != nil {
		return err
	}
	defer stream.Close()

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	bufPtr := bufferPool.Get().(*[]byte)
	defer bufferPool.Put(bufPtr)

	_, err = io.CopyBuffer(out, stream, *bufPtr)
	return err
}

// ImageNameToFileName hashes URL to produce safe local file name.
func ImageNameToFileName(imgURL string) string {
	return storage.ImageNameToFileName(imgURL)
}
