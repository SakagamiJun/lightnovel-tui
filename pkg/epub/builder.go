package epub

import (
	"archive/zip"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"lnr-core/pkg/model"
	"lnr-core/pkg/text"
)

// bufferPool provides reusable 32KB buffers for streaming zip writes.
var bufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, 32*1024)
		return &buf
	},
}

// ChapterItem holds information for EPUB manifest and spine.
type ChapterItem struct {
	ID       string
	Title    string
	FileName string
	Elements []model.ContentElement
}

// ImageItem holds information for images embedded in EPUB.
type ImageItem struct {
	ID        string
	LocalPath string
	EPUBPath  string
	MediaType string
}

// Builder constructs an EPUB book in a streaming manner to minimize memory consumption.
type Builder struct {
	BookID      string
	Title       string
	Author      string
	Publisher   string
	Description string
	CoverPath   string
	Chapters    []ChapterItem
	Images      map[string]ImageItem // localPath -> ImageItem
	Traditional bool
	Rules       []text.FormattingRule
}

// NewBuilder creates a new EPUB builder instance.
func NewBuilder(bookID, title, author, publisher, description string) *Builder {
	return &Builder{
		BookID:      bookID,
		Title:       title,
		Author:      author,
		Publisher:   publisher,
		Description: description,
		Chapters:    make([]ChapterItem, 0),
		Images:      make(map[string]ImageItem),
	}
}

// SetRules configures custom formatting and watermark cleaning rules.
func (b *Builder) SetRules(rules []text.FormattingRule) {
	b.Rules = rules
}

// SetTraditional configures whether to convert exported text to Traditional Chinese.
func (b *Builder) SetTraditional(traditional bool) {
	b.Traditional = traditional
}

// SetCover specifies the local path to the cover image.
func (b *Builder) SetCover(coverPath string) {
	b.CoverPath = coverPath
}

// AddChapter adds a chapter to the EPUB.
func (b *Builder) AddChapter(id, title string, elements []model.ContentElement, imageResolver func(url string) string) {
	fileName := fmt.Sprintf("chapter_%s.xhtml", id)
	// Register local images
	for _, el := range elements {
		if el.Type == model.ContentTypeImage && el.URL != "" && imageResolver != nil {
			localPath := imageResolver(el.URL)
			if localPath != "" {
				if _, exists := b.Images[localPath]; !exists {
					ext := strings.ToLower(filepath.Ext(localPath))
					mediaType := "image/jpeg"
					if ext == ".png" {
						mediaType = "image/png"
					} else if ext == ".gif" {
						mediaType = "image/gif"
					}
					imgID := fmt.Sprintf("img_%d", len(b.Images)+1)
					epubPath := fmt.Sprintf("images/%s%s", imgID, ext)
					b.Images[localPath] = ImageItem{
						ID:        imgID,
						LocalPath: localPath,
						EPUBPath:  epubPath,
						MediaType: mediaType,
					}
				}
			}
		}
	}

	b.Chapters = append(b.Chapters, ChapterItem{
		ID:       id,
		Title:    title,
		FileName: fileName,
		Elements: elements,
	})
}

// WriteTo streams the generated EPUB into an io.Writer (e.g. an os.File) without holding everything in RAM.
func (b *Builder) WriteTo(w io.Writer, imageResolver func(url string) string) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	// 1. mimetype (MUST be first, uncompressed)
	mimetypeHeader := &zip.FileHeader{
		Name:   "mimetype",
		Method: zip.Store,
	}
	mw, err := zw.CreateHeader(mimetypeHeader)
	if err != nil {
		return fmt.Errorf("failed to create mimetype entry: %w", err)
	}
	if _, err := io.WriteString(mw, "application/epub+zip"); err != nil {
		return err
	}

	// 2. META-INF/container.xml
	cw, err := zw.Create("META-INF/container.xml")
	if err != nil {
		return err
	}
	containerXML := `<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
    <rootfiles>
        <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
    </rootfiles>
</container>`
	if _, err := io.WriteString(cw, containerXML); err != nil {
		return err
	}

	// 3. Write Cover Image if present
	hasCover := false
	var coverItem ImageItem
	if b.CoverPath != "" {
		if _, err := os.Stat(b.CoverPath); err == nil {
			hasCover = true
			ext := strings.ToLower(filepath.Ext(b.CoverPath))
			mediaType := "image/jpeg"
			if ext == ".png" {
				mediaType = "image/png"
			}
			coverItem = ImageItem{
				ID:        "cover_image",
				LocalPath: b.CoverPath,
				EPUBPath:  "images/cover" + ext,
				MediaType: mediaType,
			}
			if err := streamFileToZip(zw, "OEBPS/"+coverItem.EPUBPath, coverItem.LocalPath); err != nil {
				return fmt.Errorf("failed to stream cover image: %w", err)
			}
		}
	}

	// 4. Stream illustration images into OEBPS/images/
	for _, img := range b.Images {
		if _, err := os.Stat(img.LocalPath); err == nil {
			if err := streamFileToZip(zw, "OEBPS/"+img.EPUBPath, img.LocalPath); err != nil {
				return fmt.Errorf("failed to stream image %s: %w", img.LocalPath, err)
			}
		}
	}

	// 5. Stream XHTML Chapters into OEBPS/
	for _, ch := range b.Chapters {
		chWriter, err := zw.Create("OEBPS/" + ch.FileName)
		if err != nil {
			return fmt.Errorf("failed to create chapter entry: %w", err)
		}
		if err := b.writeChapterXHTML(chWriter, ch, imageResolver); err != nil {
			return fmt.Errorf("failed to write chapter xhtml: %w", err)
		}
	}

	// 6. Write OEBPS/toc.ncx
	ncxWriter, err := zw.Create("OEBPS/toc.ncx")
	if err != nil {
		return err
	}
	if err := b.writeTocNcx(ncxWriter); err != nil {
		return err
	}

	// 7. Write OEBPS/nav.xhtml (EPUB3 navigation)
	navWriter, err := zw.Create("OEBPS/nav.xhtml")
	if err != nil {
		return err
	}
	if err := b.writeNavXHTML(navWriter); err != nil {
		return err
	}

	// 8. Write OEBPS/content.opf
	opfWriter, err := zw.Create("OEBPS/content.opf")
	if err != nil {
		return err
	}
	return b.writeContentOpf(opfWriter, hasCover, coverItem)
}

func (b *Builder) writeChapterXHTML(w io.Writer, ch ChapterItem, imageResolver func(url string) string) error {
	chTitle := ch.Title
	if b.Traditional {
		chTitle = text.ToTraditional(chTitle)
	}

	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head>
    <title>`)
	sb.WriteString(html.EscapeString(chTitle))
	sb.WriteString(`</title>
    <style type="text/css">
        body { font-family: "PingFang SC", "Heiti SC", "Microsoft YaHei", sans-serif; line-height: 1.8; margin: 1.5em; }
        h1, h2, h3 { text-align: center; margin: 1.5em 0; }
        p { text-indent: 2em; margin: 0.8em 0; }
        .illust-container { text-align: center; margin: 1.5em 0; }
        .illust-container img { max-width: 100%; height: auto; }
    </style>
</head>
<body>
    <h2>`)
	sb.WriteString(html.EscapeString(chTitle))
	sb.WriteString("</h2>\n")

	for _, el := range ch.Elements {
		if el.Type == model.ContentTypeText {
			paras := strings.Split(el.Text, "\n")
			for _, p := range paras {
				p = text.CleanParagraph(p)
				if p != "" {
					if len(b.Rules) > 0 {
						p = text.ApplyRules(p, b.BookID, b.Rules)
						p = strings.Trim(p, " \t\r\n\u3000")
					}
					if p == "" {
						continue
					}
					if b.Traditional {
						p = text.ToTraditional(p)
					}
					sb.WriteString("    <p>")
					sb.WriteString(html.EscapeString(p))
					sb.WriteString("</p>\n")
				}
			}
		} else if el.Type == model.ContentTypeImage && el.URL != "" && imageResolver != nil {
			localPath := imageResolver(el.URL)
			if item, ok := b.Images[localPath]; ok {
				sb.WriteString(fmt.Sprintf("    <div class=\"illust-container\"><img src=\"%s\" alt=\"illustration\" /></div>\n", item.EPUBPath))
			}
		}
	}

	sb.WriteString("</body>\n</html>\n")
	_, err := io.WriteString(w, sb.String())
	return err
}

func (b *Builder) writeTocNcx(w io.Writer) error {
	var sb strings.Builder
	bookTitle := b.Title
	if b.Traditional {
		bookTitle = text.ToTraditional(bookTitle)
	}

	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1">
    <head>
        <meta name="dtb:uid" content="urn:uuid:`)
	sb.WriteString(html.EscapeString(b.BookID))
	sb.WriteString(`"/>
        <meta name="dtb:depth" content="1"/>
        <meta name="dtb:totalPageCount" content="0"/>
        <meta name="dtb:maxPageNumber" content="0"/>
    </head>
    <docTitle><text>`)
	sb.WriteString(html.EscapeString(bookTitle))
	sb.WriteString(`</text></docTitle>
    <navMap>
`)
	for i, ch := range b.Chapters {
		title := ch.Title
		if b.Traditional {
			title = text.ToTraditional(title)
		}
		sb.WriteString(fmt.Sprintf(`        <navPoint id="navPoint-%d" playOrder="%d">
            <navLabel><text>%s</text></navLabel>
            <content src="%s"/>
        </navPoint>
`, i+1, i+1, html.EscapeString(title), ch.FileName))
	}
	sb.WriteString("    </navMap>\n</ncx>\n")
	_, err := io.WriteString(w, sb.String())
	return err
}

func (b *Builder) writeNavXHTML(w io.Writer) error {
	var sb strings.Builder
	tocTitle := "目录"
	if b.Traditional {
		tocTitle = "目錄"
	}

	sb.WriteString(`<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head>
    <title>Table of Contents</title>
</head>
<body>
    <nav epub:type="toc" id="toc">
        <h1>` + tocTitle + `</h1>
        <ol>
`)
	for _, ch := range b.Chapters {
		title := ch.Title
		if b.Traditional {
			title = text.ToTraditional(title)
		}
		sb.WriteString(fmt.Sprintf(`            <li><a href="%s">%s</a></li>
`, ch.FileName, html.EscapeString(title)))
	}
	sb.WriteString(`        </ol>
    </nav>
</body>
</html>
`)
	_, err := io.WriteString(w, sb.String())
	return err
}

func (b *Builder) writeContentOpf(w io.Writer, hasCover bool, coverItem ImageItem) error {
	var sb strings.Builder
	now := time.Now().UTC().Format(time.RFC3339)

	bookTitle := b.Title
	author := b.Author
	publisher := b.Publisher
	desc := b.Description
	lang := "zh-CN"
	if b.Traditional {
		bookTitle = text.ToTraditional(bookTitle)
		author = text.ToTraditional(author)
		publisher = text.ToTraditional(publisher)
		desc = text.ToTraditional(desc)
		lang = "zh-TW"
	}

	sb.WriteString(fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" unique-identifier="BookId" version="3.0">
    <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
        <dc:identifier id="BookId">urn:uuid:%s</dc:identifier>
        <dc:title>%s</dc:title>
        <dc:language>%s</dc:language>
        <dc:creator>%s</dc:creator>
        <dc:publisher>%s</dc:publisher>
        <dc:description>%s</dc:description>
        <meta property="dcterms:modified">%s</meta>
`, html.EscapeString(b.BookID), html.EscapeString(bookTitle), lang, html.EscapeString(author),
		html.EscapeString(publisher), html.EscapeString(desc), now))

	if hasCover {
		sb.WriteString(`        <meta name="cover" content="cover-image"/>` + "\n")
	}
	sb.WriteString("    </metadata>\n    <manifest>\n")
	sb.WriteString(`        <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>` + "\n")
	sb.WriteString(`        <item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>` + "\n")

	if hasCover {
		sb.WriteString(fmt.Sprintf(`        <item id="cover-image" href="%s" media-type="%s" properties="cover-image"/>`+"\n",
			coverItem.EPUBPath, coverItem.MediaType))
	}

	for _, img := range b.Images {
		sb.WriteString(fmt.Sprintf(`        <item id="%s" href="%s" media-type="%s"/>`+"\n",
			img.ID, img.EPUBPath, img.MediaType))
	}

	for _, ch := range b.Chapters {
		sb.WriteString(fmt.Sprintf(`        <item id="ch_%s" href="%s" media-type="application/xhtml+xml"/>`+"\n",
			ch.ID, ch.FileName))
	}

	sb.WriteString("    </manifest>\n    <spine toc=\"ncx\">\n")
	for _, ch := range b.Chapters {
		sb.WriteString(fmt.Sprintf(`        <itemref idref="ch_%s"/>`+"\n", ch.ID))
	}
	sb.WriteString("    </spine>\n</package>\n")

	_, err := io.WriteString(w, sb.String())
	return err
}

func streamFileToZip(zw *zip.Writer, zipEntryName, localPath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	w, err := zw.Create(zipEntryName)
	if err != nil {
		return err
	}

	bufPtr := bufferPool.Get().(*[]byte)
	defer bufferPool.Put(bufPtr)

	_, err = io.CopyBuffer(w, file, *bufPtr)
	return err
}
