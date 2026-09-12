package wenku8

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"

	"github.com/SakagamiJun/lightnovel-tui/pkg/model"
)

// GetChapterContent fetches and parses the chapter text and illustrations.
func (s *Wenku8Source) GetChapterContent(ctx context.Context, bookID, chapterID string) (*model.ChapterContent, error) {
	idNum, err := strconv.Atoi(bookID)
	if err != nil {
		return nil, fmt.Errorf("invalid book id: %s", bookID)
	}

	url := fmt.Sprintf("%s/novel/%d/%s/%s.htm", s.host, idNum/1000, bookID, chapterID)
	stream, err := s.client.GetStream(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch chapter %s of book %s: %w", chapterID, bookID, err)
	}
	defer stream.Close()

	doc, err := goquery.NewDocumentFromReader(stream)
	if err != nil {
		return nil, fmt.Errorf("failed to parse chapter HTML: %w", err)
	}

	if strings.Contains(doc.Text(), "因版权问题") {
		return nil, fmt.Errorf("chapter %s is unavailable due to copyright restrictions", chapterID)
	}

	rawTitle := strings.TrimSpace(doc.Find("#title").Text())
	contentDiv := doc.Find("#content")
	if contentDiv.Length() == 0 {
		return nil, fmt.Errorf("failed to locate #content for chapter %s", chapterID)
	}

	// Parse elements in streaming order (paragraphs and images)
	var elements []model.ContentElement
	var currentParagraph strings.Builder

	flushParagraph := func() {
		if currentParagraph.Len() > 0 {
			text := strings.TrimSpace(currentParagraph.String())
			if text != "" {
				elements = append(elements, model.ContentElement{
					Type: model.ContentTypeText,
					Text: text,
				})
			}
			currentParagraph.Reset()
		}
	}

	firstNode := contentDiv.Nodes[0].FirstChild
	for node := firstNode; node != nil; node = node.NextSibling {
		if node.Type == html.TextNode {
			t := strings.ReplaceAll(node.Data, "\u00a0", "  ")
			currentParagraph.WriteString(t)
		} else if node.Type == html.ElementNode {
			sel := goquery.NewDocumentFromNode(node).Selection
			if sel.Is("div.divimage") {
				flushParagraph()
				imgSrc, exists := sel.Find("img").Attr("src")
				if exists && imgSrc != "" {
					elements = append(elements, model.ContentElement{
						Type: model.ContentTypeImage,
						URL:  imgSrc,
					})
				}
			} else if sel.Is("br") || sel.Is("p") {
				// line break or paragraph boundary
				currentParagraph.WriteString("\n")
			}
		}
	}
	flushParagraph()

	// Parse prev/next chapter navigation
	prevCh := ""
	nextCh := ""
	doc.Find("#foottext a").Each(func(_ int, a *goquery.Selection) {
		text := strings.TrimSpace(a.Text())
		href, _ := a.Attr("href")
		if strings.Contains(href, ".htm") && !strings.Contains(href, "index.htm") {
			targetID := strings.TrimSuffix(href, ".htm")
			if strings.Contains(text, "上一页") || strings.Contains(text, "上一章") {
				prevCh = targetID
			} else if strings.Contains(text, "下一页") || strings.Contains(text, "下一章") {
				nextCh = targetID
			}
		}
	})

	return &model.ChapterContent{
		ID:          chapterID,
		BookID:      bookID,
		Title:       rawTitle,
		Elements:    elements,
		PrevChapter: prevCh,
		NextChapter: nextCh,
	}, nil
}
