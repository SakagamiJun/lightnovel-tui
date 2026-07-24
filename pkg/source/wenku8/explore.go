package wenku8

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"lnr-core/internal/encoding"
	"lnr-core/pkg/model"
	"lnr-core/pkg/source"
)

// DefaultWenku8Tags defines the 50 high-frequency novel categories from Wenku8.
var DefaultWenku8Tags = []string{
	"校园", "青春", "恋爱", "治愈", "群像",
	"竞技", "音乐", "美食", "旅行", "欢乐向",
	"经营", "职场", "斗智", "脑洞", "宅文化",
	"穿越", "奇幻", "魔法", "异能", "战斗",
	"科幻", "机战", "战争", "冒险", "龙傲天",
	"悬疑", "犯罪", "复仇", "黑暗", "猎奇",
	"惊悚", "间谍", "末日", "游戏", "大逃杀",
	"青梅竹马", "妹妹", "女儿", "JK", "JC",
	"大小姐", "性转", "伪娘", "人外",
	"后宫", "百合", "耽美", "NTR", "女性视角",
}

// GetTags returns the preset Wenku8 novel tags.
func (s *Wenku8Source) GetTags() []string {
	tags := make([]string, len(DefaultWenku8Tags))
	copy(tags, DefaultWenku8Tags)
	return tags
}

// GetToplist fetches leaderboard books for the given toplist type.
func (s *Wenku8Source) GetToplist(ctx context.Context, tType source.ToplistType, page int) ([]model.BookSummary, int, error) {
	if page < 1 {
		page = 1
	}

	var reqURL string
	if tType == source.ToplistCompleted {
		reqURL = fmt.Sprintf("%s/modules/article/articlelist.php?fullflag=1&page=%d", s.host, page)
	} else {
		reqURL = fmt.Sprintf("%s/modules/article/toplist.php?sort=%s&page=%d", s.host, tType, page)
	}

	stream, err := s.client.GetStream(ctx, reqURL)
	if err != nil {
		return nil, 0, err
	}
	defer stream.Close()

	doc, err := goquery.NewDocumentFromReader(stream)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to parse toplist HTML: %w", err)
	}

	totalPages := extractPagination(doc)
	results := s.extractBookCards(doc)

	return results, totalPages, nil
}

// GetTagBooks searches novels matching a specific category tag.
func (s *Wenku8Source) GetTagBooks(ctx context.Context, tag string, page int) ([]model.BookSummary, int, error) {
	if page < 1 {
		page = 1
	}

	encodedTag, err := encoding.URLEncodeGBK(tag)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to encode tag: %w", err)
	}

	reqURL := fmt.Sprintf("%s/modules/article/tags.php?t=%s&page=%d", s.host, encodedTag, page)
	stream, err := s.client.GetStream(ctx, reqURL)
	if err != nil {
		return nil, 0, err
	}
	defer stream.Close()

	doc, err := goquery.NewDocumentFromReader(stream)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to parse tag HTML: %w", err)
	}

	totalPages := extractPagination(doc)
	results := s.extractBookCards(doc)

	return results, totalPages, nil
}

func extractPagination(doc *goquery.Document) int {
	totalPages := 1
	pageLink := doc.Find("#pagelink em").Text()
	if pageLink != "" {
		parts := strings.Split(pageLink, "/")
		if len(parts) == 2 {
			if p, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && p > 0 {
				totalPages = p
			}
		}
	}
	return totalPages
}

func (s *Wenku8Source) extractBookCards(doc *goquery.Document) []model.BookSummary {
	var results []model.BookSummary
	seen := make(map[string]bool)

	doc.Find("#content table tr td > div, #content table.grid tr td > div").Each(func(i int, sel *goquery.Selection) {
		summary := s.parseBookCard(sel)
		if summary != nil && !seen[summary.ID] {
			seen[summary.ID] = true
			results = append(results, *summary)
		}
	})
	return results
}

func extractBookCards(doc *goquery.Document) []model.BookSummary {
	var src *Wenku8Source
	return src.extractBookCards(doc)
}
