package epub

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/SakagamiJun/lnovel_tui/pkg/downloader"
	"github.com/SakagamiJun/lnovel_tui/pkg/model"
	"github.com/SakagamiJun/lnovel_tui/pkg/storage"
)

// ExportOption configures how EPUB files are built and exported.
type ExportOption struct {
	VolumeIndexes []int // 1-indexed volume numbers; empty or nil means all volumes
	NoImages      bool  // true to omit cover and all illustrations for lightweight text-only EPUB
	SplitVolumes  bool  // false: consolidated single EPUB; true: separate EPUB per volume
}

// DefaultExportOption returns default export settings.
func DefaultExportOption() ExportOption {
	return ExportOption{
		VolumeIndexes: nil,
		NoImages:      false,
		SplitVolumes:  false,
	}
}

// ParseVolumeIndexes parses a volume specification string into a slice of 1-indexed volume numbers.
// Formats supported: "1", "1,2,3", "1-3", "1, 3-5", "1 - 3".
// Returns nil if s is empty or "0".
func ParseVolumeIndexes(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return nil, nil
	}

	seen := make(map[int]bool)
	parts := strings.Split(s, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("无效的分卷范围格式: '%s'", part)
			}
			startStr := strings.TrimSpace(rangeParts[0])
			endStr := strings.TrimSpace(rangeParts[1])
			var start, end int
			if _, err := fmt.Sscanf(startStr, "%d", &start); err != nil || start <= 0 {
				return nil, fmt.Errorf("无效的分卷起始序号: '%s'", startStr)
			}
			if _, err := fmt.Sscanf(endStr, "%d", &end); err != nil || end <= 0 {
				return nil, fmt.Errorf("无效的分卷结束序号: '%s'", endStr)
			}
			if start > end {
				return nil, fmt.Errorf("分卷范围起始序号 %d 不能大于结束序号 %d", start, end)
			}
			for i := start; i <= end; i++ {
				seen[i] = true
			}
		} else {
			var vol int
			if _, err := fmt.Sscanf(part, "%d", &vol); err != nil || vol <= 0 {
				return nil, fmt.Errorf("无效的分卷序号: '%s'", part)
			}
			seen[vol] = true
		}
	}

	if len(seen) == 0 {
		return nil, nil
	}

	var res []int
	for vol := range seen {
		res = append(res, vol)
	}
	sort.Ints(res)
	return res, nil
}

// Exporter manages creating EPUB files from local cache or downloading missing content on demand.
type Exporter struct {
	store       *storage.Storage
	dl          *downloader.Downloader
	traditional bool
}

// NewExporter creates a novel EPUB exporter.
func NewExporter(store *storage.Storage, dl *downloader.Downloader) *Exporter {
	return &Exporter{
		store: store,
		dl:    dl,
	}
}

// SetTraditional configures whether exported EPUBs should be in Traditional Chinese.
func (e *Exporter) SetTraditional(traditional bool) {
	e.traditional = traditional
}

// ExportWithOptions exports novel volumes to EPUB using granular configuration.
func (e *Exporter) ExportWithOptions(ctx context.Context, bookID string, opt ExportOption, outputPathOrDir string, onProgress func(current, total int, volTitle, outFile string)) ([]string, error) {
	detail, err := e.store.LoadBookDetail(bookID)
	if err != nil {
		return nil, fmt.Errorf("book detail not cached for ID %s: %w", bookID, err)
	}

	catalog, err := e.store.LoadCatalog(bookID)
	if err != nil {
		return nil, fmt.Errorf("catalog not cached for ID %s: %w", bookID, err)
	}

	if len(catalog.Volumes) == 0 {
		return nil, fmt.Errorf("catalog has no volumes for book ID %s", bookID)
	}

	type volTarget struct {
		index int
		vol   *model.Volume
	}
	var targets []volTarget

	if len(opt.VolumeIndexes) == 0 {
		for i := range catalog.Volumes {
			targets = append(targets, volTarget{index: i + 1, vol: &catalog.Volumes[i]})
		}
	} else {
		for _, idx := range opt.VolumeIndexes {
			if idx < 1 || idx > len(catalog.Volumes) {
				return nil, fmt.Errorf("分卷序号 %d 超出有效范围 (1..%d)", idx, len(catalog.Volumes))
			}
			targets = append(targets, volTarget{index: idx, vol: &catalog.Volumes[idx-1]})
		}
	}

	rules, _ := e.store.LoadRules()
	imageResolver := func(imgURL string) string {
		if opt.NoImages {
			return ""
		}
		imgFileName := downloader.ImageNameToFileName(imgURL)
		path := e.store.ImagePath(bookID, imgFileName)
		if _, err := os.Stat(path); err == nil {
			return path
		}
		return ""
	}

	cleanTitle := sanitizeFileName(detail.Title)

	if opt.SplitVolumes {
		outputDir := outputPathOrDir
		if outputDir == "" {
			outputDir = "."
		} else {
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
			}
		}

		var outputFiles []string
		for i, t := range targets {
			cleanVol := sanitizeFileName(t.vol.Title)
			fileName := fmt.Sprintf("%s - %s.epub", cleanTitle, cleanVol)
			outFile := filepath.Join(outputDir, fileName)

			title := fmt.Sprintf("%s - %s", detail.Title, t.vol.Title)
			builder := NewBuilder(fmt.Sprintf("%s_%s", bookID, t.vol.ID), title, detail.Author, detail.Publisher, detail.Description)
			builder.SetTraditional(e.traditional)
			builder.SetNoImages(opt.NoImages)
			if len(rules) > 0 {
				builder.SetRules(rules)
			}
			if !opt.NoImages {
				builder.SetCover(e.resolveCover(ctx, bookID, detail.CoverURL))
			}

			for _, ch := range t.vol.Chapters {
				chContent, err := e.store.LoadChapter(bookID, ch.ID)
				if err != nil {
					return outputFiles, fmt.Errorf("chapter %s (%s) not found in cache. Please run download first: %w", ch.Title, ch.ID, err)
				}
				builder.AddChapter(ch.ID, ch.Title, chContent.Elements, imageResolver)
			}

			f, err := os.Create(outFile)
			if err != nil {
				return outputFiles, fmt.Errorf("failed to create output file %s: %w", outFile, err)
			}
			writeErr := builder.WriteTo(f, imageResolver)
			f.Close()
			if writeErr != nil {
				return outputFiles, fmt.Errorf("failed to write EPUB %s: %w", outFile, writeErr)
			}

			outputFiles = append(outputFiles, outFile)
			if onProgress != nil {
				onProgress(i+1, len(targets), t.vol.Title, outFile)
			}
		}
		return outputFiles, nil
	}

	// Merged mode
	var outFile string
	if strings.HasSuffix(strings.ToLower(outputPathOrDir), ".epub") {
		outFile = outputPathOrDir
		dir := filepath.Dir(outFile)
		if dir != "" && dir != "." {
			_ = os.MkdirAll(dir, 0755)
		}
	} else {
		var fileName string
		if len(targets) == 1 {
			fileName = fmt.Sprintf("%s_%s.epub", cleanTitle, sanitizeFileName(targets[0].vol.Title))
		} else if len(targets) == len(catalog.Volumes) {
			fileName = fmt.Sprintf("%s.epub", cleanTitle)
		} else {
			var volNums []string
			for _, t := range targets {
				volNums = append(volNums, fmt.Sprintf("%d", t.index))
			}
			fileName = fmt.Sprintf("%s_卷%s.epub", cleanTitle, strings.Join(volNums, ","))
		}

		if outputPathOrDir != "" {
			_ = os.MkdirAll(outputPathOrDir, 0755)
			outFile = filepath.Join(outputPathOrDir, fileName)
		} else {
			outFile = fileName
		}
	}

	bookTitle := detail.Title
	if len(targets) == 1 {
		bookTitle = fmt.Sprintf("%s - %s", detail.Title, targets[0].vol.Title)
	}

	builder := NewBuilder(bookID, bookTitle, detail.Author, detail.Publisher, detail.Description)
	builder.SetTraditional(e.traditional)
	builder.SetNoImages(opt.NoImages)
	if len(rules) > 0 {
		builder.SetRules(rules)
	}
	if !opt.NoImages {
		builder.SetCover(e.resolveCover(ctx, bookID, detail.CoverURL))
	}

	for _, t := range targets {
		for _, ch := range t.vol.Chapters {
			chContent, err := e.store.LoadChapter(bookID, ch.ID)
			if err != nil {
				return nil, fmt.Errorf("chapter %s (%s) not cached. Please download first: %w", ch.Title, ch.ID, err)
			}
			chTitle := ch.Title
			if len(targets) > 1 {
				chTitle = fmt.Sprintf("%s · %s", t.vol.Title, ch.Title)
			}
			builder.AddChapter(ch.ID, chTitle, chContent.Elements, imageResolver)
		}
	}

	f, err := os.Create(outFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create output file %s: %w", outFile, err)
	}
	defer f.Close()

	if err := builder.WriteTo(f, imageResolver); err != nil {
		return nil, fmt.Errorf("failed to write EPUB: %w", err)
	}

	if onProgress != nil {
		onProgress(1, 1, bookTitle, outFile)
	}

	return []string{outFile}, nil
}

// ExportVolume exports a specific volume into a standalone EPUB file.
func (e *Exporter) ExportVolume(ctx context.Context, bookID string, volumeIndex int, outputPath string) (string, error) {
	paths, err := e.ExportWithOptions(ctx, bookID, ExportOption{
		VolumeIndexes: []int{volumeIndex},
		SplitVolumes:  false,
	}, outputPath, nil)
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("no output generated")
	}
	return paths[0], nil
}

// ExportFullBook exports all volumes into a single consolidated EPUB file.
func (e *Exporter) ExportFullBook(ctx context.Context, bookID string, outputPath string) (string, error) {
	paths, err := e.ExportWithOptions(ctx, bookID, ExportOption{
		VolumeIndexes: nil,
		SplitVolumes:  false,
	}, outputPath, nil)
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("no output generated")
	}
	return paths[0], nil
}

// ExportAllVolumes exports each volume in the catalog as an individual standalone EPUB file.
// If outputDir is specified, EPUB files are saved inside it. Otherwise they are saved in the current directory.
// onProgress is invoked after each volume is successfully written.
func (e *Exporter) ExportAllVolumes(ctx context.Context, bookID string, outputDir string, onProgress func(current, total int, volTitle, outFile string)) ([]string, error) {
	return e.ExportWithOptions(ctx, bookID, ExportOption{
		VolumeIndexes: nil,
		SplitVolumes:  true,
	}, outputDir, onProgress)
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
