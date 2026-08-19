package wenku8

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"github.com/SakagamiJun/lnovel_tui/pkg/model"
)

// GetBookDetail fetches detailed information about a book.
func (s *Wenku8Source) GetBookDetail(ctx context.Context, bookID string) (*model.BookDetail, error) {
	url := fmt.Sprintf("%s/book/%s.htm", s.host, bookID)
	stream, err := s.client.GetStream(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch book detail (%s): %w", bookID, err)
	}
	defer stream.Close()

	doc, err := goquery.NewDocumentFromReader(stream)
	if err != nil {
		return nil, fmt.Errorf("failed to parse book detail HTML: %w", err)
	}

	if strings.Contains(doc.Text(), "因版权问题") {
		return nil, fmt.Errorf("book %s is blocked due to copyright restrictions on Wenku8", bookID)
	}

	firstTable := doc.Find("#content table").First()

	// Title
	rawTitle := strings.TrimSpace(firstTable.Find("td span b").First().Text())
	if rawTitle == "" {
		rawTitle = strings.TrimSpace(firstTable.Find("b").First().Text())
	}
	rawTitle = strings.TrimSuffix(rawTitle, "[推一下!]")
	rawTitle = strings.TrimSpace(rawTitle)
	if rawTitle == "" {
		return nil, fmt.Errorf("failed to parse book title for ID %s", bookID)
	}

	title := rawTitle
	subtitle := ""
	if matches := titleRegex.FindStringSubmatch(rawTitle); len(matches) == 3 {
		title = strings.TrimSpace(matches[1])
		subtitle = strings.TrimSpace(matches[2])
	}

	// Iterate td items in first table
	publisher := ""
	author := ""
	status := ""
	lastUpdated := ""
	wordCount := 0

	firstTable.Find("td").Each(func(_ int, td *goquery.Selection) {
		text := strings.TrimSpace(td.Text())
		if strings.HasPrefix(text, "文库分类：") {
			publisher = strings.TrimSpace(strings.TrimPrefix(text, "文库分类："))
		} else if strings.HasPrefix(text, "小说作者：") {
			author = strings.TrimSpace(strings.TrimPrefix(text, "小说作者："))
		} else if strings.HasPrefix(text, "文章状态：") {
			status = strings.TrimSpace(strings.TrimPrefix(text, "文章状态："))
		} else if strings.HasPrefix(text, "最后更新：") {
			lastUpdated = strings.TrimSpace(strings.TrimPrefix(text, "最后更新："))
		} else if strings.HasPrefix(text, "全文长度：") {
			rawWords := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(text, "全文长度："), "字"))
			wordCount, _ = strconv.Atoi(rawWords)
		}
	})

	// Second table for cover and description
	secondTable := doc.Find("#content table:nth-of-type(2)")
	coverURL, _ := secondTable.Find("tr td:nth-child(1) img").Attr("src")

	rawTags := secondTable.Find("tr td:nth-child(2) span:nth-of-type(1) b").Text()
	rawTags = strings.TrimSpace(strings.ReplaceAll(rawTags, "作品Tags：", ""))
	var tags []string
	if rawTags != "" {
		tags = strings.Fields(rawTags)
	}

	desc := strings.TrimSpace(secondTable.Find("tr td:nth-child(2) span:nth-of-type(6)").Text())
	if desc != "" {
		s.SetCachedDescription(bookID, desc)
	}

	return &model.BookDetail{
		BookSummary: model.BookSummary{
			ID:          bookID,
			Title:       title,
			Subtitle:    subtitle,
			Author:      author,
			CoverURL:    coverURL,
			Description: desc,
			Tags:        tags,
			Publisher:   publisher,
			WordCount:   wordCount,
			LastUpdated: lastUpdated,
			IsComplete:  strings.Contains(status, "已完结"),
		},
	}, nil
}

// GetCatalog fetches the volumes and chapters of a book.
func (s *Wenku8Source) GetCatalog(ctx context.Context, bookID string) (*model.BookCatalog, error) {
	idNum, err := strconv.Atoi(bookID)
	if err != nil {
		return nil, fmt.Errorf("invalid book id: %s", bookID)
	}
	url := fmt.Sprintf("%s/novel/%d/%s/index.htm", s.host, idNum/1000, bookID)

	stream, err := s.client.GetStream(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch catalog (%s): %w", bookID, err)
	}
	defer stream.Close()

	doc, err := goquery.NewDocumentFromReader(stream)
	if err != nil {
		return nil, fmt.Errorf("failed to parse catalog HTML: %w", err)
	}

	var volumes []model.Volume
	var currentVolume *model.Volume

	doc.Find("table tr").Each(func(_ int, tr *goquery.Selection) {
		// Check for volume row
		vcss := tr.Find("td.vcss")
		if vcss.Length() > 0 {
			if currentVolume != nil {
				volumes = append(volumes, *currentVolume)
			}
			vid, _ := vcss.Attr("vid")
			vTitle := strings.TrimSpace(vcss.Text())
			currentVolume = &model.Volume{
				ID:       vid,
				Title:    vTitle,
				Chapters: []model.Chapter{},
			}
			return
		}

		// Chapter row
		if currentVolume != nil {
			tr.Find("td a").Each(func(_ int, a *goquery.Selection) {
				href, exists := a.Attr("href")
				if !exists {
					return
				}
				chapterID := strings.TrimSuffix(href, ".htm")
				chapterTitle := strings.TrimSpace(a.Text())
				if chapterID != "" && chapterTitle != "" {
					currentVolume.Chapters = append(currentVolume.Chapters, model.Chapter{
						ID:    chapterID,
						Title: chapterTitle,
					})
				}
			})
		}
	})

	if currentVolume != nil {
		volumes = append(volumes, *currentVolume)
	}

	return &model.BookCatalog{
		BookID:  bookID,
		Volumes: volumes,
	}, nil
}
