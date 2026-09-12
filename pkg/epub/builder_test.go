package epub

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SakagamiJun/lightnovel-tui/pkg/model"
	"github.com/SakagamiJun/lightnovel-tui/pkg/storage"
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
		BookSummary: model.BookSummary{
			ID:          bookID,
			Title:       "刀剑神域",
			Author:      "川原砾",
			Publisher:   "电击文库",
			Description: "SAO测试轻小说",
		},
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

func TestTraditionalEPUBBuilder(t *testing.T) {
	builder := NewBuilder("1001", "关于我转生变成史莱姆这档事", "伏濑", "微杂志社", "史莱姆冒险")
	builder.SetTraditional(true)

	builder.AddChapter("ch1", "第一章 异界转生", []model.ContentElement{
		{Type: model.ContentTypeText, Text: "今天天气真好,阳光明媚.\nwenku8.com 录入\n新的冒险开始了!"},
	}, nil)

	var buf bytes.Buffer
	if err := builder.WriteTo(&buf, nil); err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader failed: %v", err)
	}

	for _, f := range zr.File {
		if f.Name == "OEBPS/chapter_ch1.xhtml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("failed to open chapter xhtml: %v", err)
			}
			defer rc.Close()
			content, _ := io.ReadAll(rc)
			s := string(content)
			if !strings.Contains(s, "第一章 異界轉生") {
				t.Errorf("expected traditional chapter title, got: %s", s)
			}
			if !strings.Contains(s, "今天天氣真好，陽光明媚。") {
				t.Errorf("expected cleaned and converted traditional text, got: %s", s)
			}
			if strings.Contains(s, "wenku8.com") {
				t.Errorf("expected watermark filtered out, got: %s", s)
			}
		} else if f.Name == "OEBPS/content.opf" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("failed to open content.opf: %v", err)
			}
			defer rc.Close()
			content, _ := io.ReadAll(rc)
			s := string(content)
			if !strings.Contains(s, "<dc:language>zh-TW</dc:language>") {
				t.Errorf("expected zh-TW language tag in opf, got: %s", s)
			}
			if !strings.Contains(s, "關于我轉生變成史萊姆這檔事") {
				t.Errorf("expected traditional book title in opf, got: %s", s)
			}
		}
	}
}

func TestParseVolumeIndexes(t *testing.T) {
	tests := []struct {
		input    string
		expected []int
		wantErr  bool
	}{
		{"", nil, false},
		{"0", nil, false},
		{"1", []int{1}, false},
		{"1,2,3", []int{1, 2, 3}, false},
		{"3,1,2", []int{1, 2, 3}, false},   // sorting
		{"1,2,2,3", []int{1, 2, 3}, false}, // deduplication
		{"1-3", []int{1, 2, 3}, false},     // range
		{"1, 3-5, 7", []int{1, 3, 4, 5, 7}, false},
		{"abc", nil, true},
		{"1-", nil, true},
		{"5-2", nil, true},
		{"-1", nil, true},
	}

	for _, tc := range tests {
		res, err := ParseVolumeIndexes(tc.input)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseVolumeIndexes(%q) expected error, got nil", tc.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseVolumeIndexes(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if len(res) != len(tc.expected) {
			t.Errorf("ParseVolumeIndexes(%q) expected %v, got %v", tc.input, tc.expected, res)
			continue
		}
		for i := range res {
			if res[i] != tc.expected[i] {
				t.Errorf("ParseVolumeIndexes(%q) expected %v, got %v", tc.input, tc.expected, res)
				break
			}
		}
	}
}

func TestPureTextNoImagesExport(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pure-text-epub-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	coverPath := filepath.Join(tmpDir, "cover.jpg")
	_ = os.WriteFile(coverPath, []byte("fake-cover"), 0644)
	imgPath := filepath.Join(tmpDir, "pic.jpg")
	_ = os.WriteFile(imgPath, []byte("fake-pic"), 0644)

	builder := NewBuilder("noimg_1", "无图轻小说", "作者", "出版社", "简介")
	builder.SetCover(coverPath)
	builder.SetNoImages(true)

	imgResolver := func(url string) string {
		return imgPath
	}

	builder.AddChapter("ch1", "第一章", []model.ContentElement{
		{Type: model.ContentTypeText, Text: "纯文本内容。"},
		{Type: model.ContentTypeImage, URL: "http://example.com/pic.jpg"},
	}, imgResolver)

	var buf bytes.Buffer
	if err := builder.WriteTo(&buf, imgResolver); err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip reader failed: %v", err)
	}

	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "OEBPS/images/") {
			t.Errorf("expected no images in archive when NoImages=true, found %s", f.Name)
		}
		if f.Name == "OEBPS/chapter_ch1.xhtml" {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open xhtml failed: %v", err)
			}
			content, _ := io.ReadAll(rc)
			rc.Close()
			if strings.Contains(string(content), "<img") {
				t.Errorf("expected no <img> tag in xhtml when NoImages=true, got: %s", string(content))
			}
		}
	}
}

func TestExportWithOptionsSubset(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "exporter-options-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}

	bookID := "book_sub"
	_ = store.SaveBookDetail(&model.BookDetail{
		BookSummary: model.BookSummary{
			ID:    bookID,
			Title: "魔法禁书目录",
		},
	})
	_ = store.SaveCatalog(&model.BookCatalog{
		BookID: bookID,
		Volumes: []model.Volume{
			{ID: "v1", Title: "第1卷", Chapters: []model.Chapter{{ID: "c1", Title: "第1话"}}},
			{ID: "v2", Title: "第2卷", Chapters: []model.Chapter{{ID: "c2", Title: "第2话"}}},
			{ID: "v3", Title: "第3卷", Chapters: []model.Chapter{{ID: "c3", Title: "第3话"}}},
		},
	})
	_ = store.SaveChapter(&model.ChapterContent{BookID: bookID, ID: "c1", Elements: []model.ContentElement{{Type: model.ContentTypeText, Text: "第1卷内容"}}})
	_ = store.SaveChapter(&model.ChapterContent{BookID: bookID, ID: "c2", Elements: []model.ContentElement{{Type: model.ContentTypeText, Text: "第2卷内容"}}})
	_ = store.SaveChapter(&model.ChapterContent{BookID: bookID, ID: "c3", Elements: []model.ContentElement{{Type: model.ContentTypeText, Text: "第3卷内容"}}})

	exporter := NewExporter(store, nil)

	// Export subset volumes 1 and 2
	outFiles, err := exporter.ExportWithOptions(context.Background(), bookID, ExportOption{
		VolumeIndexes: []int{1, 2},
		NoImages:      true,
		SplitVolumes:  false,
	}, tmpDir, nil)
	if err != nil {
		t.Fatalf("ExportWithOptions failed: %v", err)
	}
	if len(outFiles) != 1 {
		t.Fatalf("expected 1 merged file, got %d", len(outFiles))
	}
	if !strings.Contains(filepath.Base(outFiles[0]), "魔法禁书目录_卷1,2.epub") {
		t.Errorf("unexpected filename: %s", outFiles[0])
	}
}
