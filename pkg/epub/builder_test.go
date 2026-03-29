package epub

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"lnr-cli/pkg/model"
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
