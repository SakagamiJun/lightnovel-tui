package epub

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"lnr-core/pkg/model"
	"lnr-core/pkg/storage"
)

func TestEPUBBuilder(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "epub-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a dummy cover
	coverPath := filepath.Join(tmpDir, "cover.jpg")
	if err := os.WriteFile(coverPath, []byte("fake-jpeg-content"), 0644); err != nil {
		t.Fatalf("failed to write cover: %v", err)
	}

	// Create a dummy image
	imgPath := filepath.Join(tmpDir, "illust.jpg")
	if err := os.WriteFile(imgPath, []byte("fake-illust-jpeg"), 0644); err != nil {
		t.Fatalf("failed to write illust: %v", err)
	}

	builder := NewBuilder("1234", "测试小说", "测试作者", "测试文库", "这是一本测试轻小说")
	builder.SetCover(coverPath)

	imgResolver := func(url string) string {
		if url == "http://example.com/pic.jpg" {
			return imgPath
		}
		return ""
	}

	builder.AddChapter("1", "第一章 开始", []model.ContentElement{
		{Type: model.ContentTypeText, Text: "很久很久以前，有一只史莱姆。"},
		{Type: model.ContentTypeImage, URL: "http://example.com/pic.jpg"},
		{Type: model.ContentTypeText, Text: "它在地下城里生活。"},
	}, imgResolver)

	var buf bytes.Buffer
	err = builder.WriteTo(&buf, imgResolver)
	if err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}

	// Validate zip structure
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("failed to read zip: %v", err)
	}

	// First entry MUST be mimetype and stored without compression
	if len(zr.File) == 0 {
		t.Fatalf("zip is empty")
	}
	if zr.File[0].Name != "mimetype" {
		t.Errorf("expected first file to be mimetype, got %s", zr.File[0].Name)
	}
	if zr.File[0].Method != zip.Store {
		t.Errorf("expected mimetype to be uncompressed (zip.Store), got %d", zr.File[0].Method)
	}

	foundMap := make(map[string]bool)
	for _, f := range zr.File {
		foundMap[f.Name] = true
	}

	expectedFiles := []string{
		"mimetype",
		"META-INF/container.xml",
		"OEBPS/content.opf",
		"OEBPS/toc.ncx",
		"OEBPS/nav.xhtml",
		"OEBPS/chapter_1.xhtml",
		"OEBPS/images/cover.jpg",
	}

	for _, ef := range expectedFiles {
		if !foundMap[ef] {
			t.Errorf("missing expected file in EPUB: %s", ef)
		}
	}

	// Check mimetype content
	rc, err := zr.File[0].Open()
	if err != nil {
		t.Fatalf("failed to open mimetype: %v", err)
	}
	defer rc.Close()
	mimetypeContent, _ := io.ReadAll(rc)
	if string(mimetypeContent) != "application/epub+zip" {
		t.Errorf("invalid mimetype content: %s", string(mimetypeContent))
	}

	t.Logf("EPUB generated successfully with %d files in archive", len(zr.File))
}

func TestExporterAllVolumes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "exporter-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	bookID := "999"
	detail := &model.BookDetail{
		ID:          bookID,
		Title:       "刀剑神域",
		Author:      "川原砾",
		Publisher:   "电击文库",
		Description: "SAO测试轻小说",
	}
	if err := store.SaveBookDetail(detail); err != nil {
		t.Fatalf("SaveBookDetail failed: %v", err)
	}

	catalog := &model.BookCatalog{
		BookID: bookID,
		Volumes: []model.Volume{
			{
				ID:    "v1",
				Title: "第一卷 艾恩葛朗特",
				Chapters: []model.Chapter{
					{ID: "c101", Title: "第一章 序幕"},
				},
			},
			{
				ID:    "v2",
				Title: "第二卷 妖精之舞",
				Chapters: []model.Chapter{
					{ID: "c201", Title: "第二章 降临"},
				},
			},
		},
	}
	if err := store.SaveCatalog(catalog); err != nil {
		t.Fatalf("SaveCatalog failed: %v", err)
	}

	ch1 := &model.ChapterContent{
		BookID: bookID,
		ID:     "c101",
		Title:  "第一章 序幕",
		Elements: []model.ContentElement{
			{Type: model.ContentTypeText, Text: "这是第一卷正文。"},
		},
	}
	if err := store.SaveChapter(ch1); err != nil {
		t.Fatalf("SaveChapter c101 failed: %v", err)
	}

	ch2 := &model.ChapterContent{
		BookID: bookID,
		ID:     "c201",
		Title:  "第二章 降临",
		Elements: []model.ContentElement{
			{Type: model.ContentTypeText, Text: "这是第二卷正文。"},
		},
	}
	if err := store.SaveChapter(ch2); err != nil {
		t.Fatalf("SaveChapter c201 failed: %v", err)
	}

	exporter := NewExporter(store, nil)
	outDir := filepath.Join(tmpDir, "epub_out")

	var progressEvents []int
	outFiles, err := exporter.ExportAllVolumes(context.Background(), bookID, outDir, func(current, total int, volTitle, outFile string) {
		progressEvents = append(progressEvents, current)
		if total != 2 {
			t.Errorf("expected total 2, got %d", total)
		}
	})
	if err != nil {
		t.Fatalf("ExportAllVolumes failed: %v", err)
	}

	if len(outFiles) != 2 {
		t.Fatalf("expected 2 output files, got %d", len(outFiles))
	}
	if len(progressEvents) != 2 {
		t.Errorf("expected 2 progress callbacks, got %d", len(progressEvents))
	}

	for i, fPath := range outFiles {
		fi, err := os.Stat(fPath)
		if err != nil {
			t.Errorf("output file %d does not exist: %v", i+1, err)
			continue
		}
		if fi.Size() == 0 {
			t.Errorf("output file %d is empty", i+1)
		}
	}
}
