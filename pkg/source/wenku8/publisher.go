package wenku8

import (
	"context"
	"fmt"

	"github.com/PuerkitoBio/goquery"

	"github.com/SakagamiJun/lightnovel-tui/pkg/model"
	"github.com/SakagamiJun/lightnovel-tui/pkg/source"
)

// DefaultWenku8Publishers defines the official 14 publishing houses on Wenku8.
var DefaultWenku8Publishers = []source.PublisherInfo{
	{ClassID: 1, Name: "电击文库"},
	{ClassID: 2, Name: "富士见文库"},
	{ClassID: 3, Name: "角川文库"},
	{ClassID: 4, Name: "MF文库J"},
	{ClassID: 5, Name: "Fami通文库"},
	{ClassID: 6, Name: "GA文库"},
	{ClassID: 7, Name: "HJ文库"},
	{ClassID: 8, Name: "一迅社"},
	{ClassID: 9, Name: "集英社"},
	{ClassID: 10, Name: "小学馆"},
	{ClassID: 11, Name: "讲谈社"},
	{ClassID: 12, Name: "少女文库"},
	{ClassID: 13, Name: "其他文库"},
	{ClassID: 14, Name: "游戏剧本"},
}

// GetPublishers returns the preset 14 Wenku8 publishers.
func (s *Wenku8Source) GetPublishers() []source.PublisherInfo {
	pubs := make([]source.PublisherInfo, len(DefaultWenku8Publishers))
	copy(pubs, DefaultWenku8Publishers)
	return pubs
}

// GetPublisherBooks fetches books belonging to a specific publisher class ID.
func (s *Wenku8Source) GetPublisherBooks(ctx context.Context, classID int, page int) ([]model.BookSummary, int, error) {
	if page < 1 {
		page = 1
	}

	reqURL := fmt.Sprintf("%s/modules/article/articlelist.php?class=%d&page=%d", s.host, classID, page)
	stream, err := s.client.GetStream(ctx, reqURL)
	if err != nil {
		return nil, 0, err
	}
	defer stream.Close()

	doc, err := goquery.NewDocumentFromReader(stream)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to parse publisher HTML: %w", err)
	}

	totalPages := extractPagination(doc)
	results := s.extractBookCards(doc)

	return results, totalPages, nil
}
