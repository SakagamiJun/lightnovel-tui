package wenku8

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/SakagamiJun/lnovel_tui/internal/client"
	"github.com/SakagamiJun/lnovel_tui/internal/encoding"
	"github.com/SakagamiJun/lnovel_tui/pkg/model"
	"github.com/SakagamiJun/lnovel_tui/pkg/source"
)

const (
	DefaultHost = "https://www.wenku8.cc"
)

var (
	titleRegex = regexp.MustCompile(`^(.*?) ?[（(](.*?)[）)] ?$`)
)

type Wenku8Source struct {
	client     *client.HttpClient
	host       string
	searchLock sync.Mutex
	lastSearch time.Time
	descCache  sync.Map // bookID -> string (full description)
}

// NewWenku8Source constructs a Wenku8 data source.
func NewWenku8Source() (*Wenku8Source, error) {
	c, err := client.NewHttpClient()
	if err != nil {
		return nil, err
	}
	return &Wenku8Source{
		client: c,
		host:   DefaultHost,
	}, nil
}

func (s *Wenku8Source) Name() string {
	return "wenku8"
}

// Search searches novels on Wenku8 by title or author.
// Handles the 5-second search frequency limit automatically.
func (s *Wenku8Source) Search(ctx context.Context, searchType source.SearchType, keyword string, page int) ([]model.BookSummary, int, error) {
	s.searchLock.Lock()
	defer s.searchLock.Unlock()

	// Ensure at least 5.2 seconds between searches
	if elapsed := time.Since(s.lastSearch); elapsed < 5200*time.Millisecond {
		time.Sleep(5200*time.Millisecond - elapsed)
	}
	s.lastSearch = time.Now()

	if page < 1 {
		page = 1
	}

	encodedKey, err := encoding.URLEncodeGBK(keyword)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to encode keyword: %w", err)
	}

	searchURL := fmt.Sprintf("%s/modules/article/search.php?searchtype=%s&searchkey=%s&page=%d",
		s.host, searchType, encodedKey, page)

	stream, err := s.client.GetStream(ctx, searchURL)
	if err != nil {
		return nil, 0, err
	}
	defer stream.Close()

	doc, err := goquery.NewDocumentFromReader(stream)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to parse HTML: %w", err)
	}

	fullText := doc.Text()
	if strings.Contains(fullText, "两次搜索的间隔时间不得少于 5 秒") {
		return nil, 0, errors.New("search rate limit exceeded, please retry in a few seconds")
	}
	if strings.Contains(fullText, "用户登录") || strings.Contains(fullText, "请输入用户名") {
		return nil, 0, errors.New("wenku8 搜索需要登录会话，请配置 LNR_WENKU8_COOKIE 环境变量")
	}

	// Check if redirected directly to a single book page
	var singleBookID string
	doc.Find("a").Each(func(_ int, a *goquery.Selection) {
		if strings.TrimSpace(a.Text()) == "小说目录" {
			href, _ := a.Attr("href")
			// e.g. /novel/4/4340/index.htm
			parts := strings.Split(href, "/")
			if len(parts) >= 4 {
				singleBookID = parts[3]
			}
		}
	})
	if singleBookID != "" {
		detail, err := s.GetBookDetail(ctx, singleBookID)
		if err != nil {
			return nil, 0, err
		}
		return []model.BookSummary{detail.BookSummary}, 1, nil
	}

	// Multiple books returned
	totalPages := 1
	pageLink := doc.Find("#pagelink em").Text()
	if pageLink != "" {
		parts := strings.Split(pageLink, "/")
		if len(parts) == 2 {
			if p, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
				totalPages = p
			}
		}
	}

	var results []model.BookSummary
	seen := make(map[string]bool)

	// Top-level item cards in search result
	doc.Find("#content table.grid tr td > div").Each(func(i int, sel *goquery.Selection) {
		summary := s.parseBookCard(sel)
		if summary != nil && !seen[summary.ID] {
			seen[summary.ID] = true
			results = append(results, *summary)
		}
	})

	return results, totalPages, nil
}

// GetCachedDescription retrieves the cached full description for a novel, if available.
func (s *Wenku8Source) GetCachedDescription(bookID string) (string, bool) {
	if s == nil || bookID == "" {
		return "", false
	}
	if val, ok := s.descCache.Load(bookID); ok {
		if desc, ok := val.(string); ok && desc != "" {
			return desc, true
		}
	}
	return "", false
}

// SetCachedDescription caches the full description for a novel.
func (s *Wenku8Source) SetCachedDescription(bookID, desc string) {
	if s != nil && bookID != "" && desc != "" {
		s.descCache.Store(bookID, desc)
	}
}

// EnrichDescriptions enhances novel summaries with full descriptions by consulting
// in-memory cache first, and concurrently fetching book detail for incomplete synopses.
func (s *Wenku8Source) EnrichDescriptions(ctx context.Context, books []model.BookSummary, maxWorkers int) []model.BookSummary {
	if s == nil || len(books) == 0 {
		return books
	}
	if maxWorkers <= 0 {
		maxWorkers = 6
	}

	enriched := make([]model.BookSummary, len(books))
	copy(enriched, books)

	type fetchTask struct {
		index  int
		bookID string
	}
	var tasks []fetchTask

	for i := range enriched {
		b := &enriched[i]
		if cached, ok := s.GetCachedDescription(b.ID); ok && cached != "" {
			b.Description = cached
			continue
		}
		clean := strings.TrimSpace(b.Description)
		if clean == "" || strings.HasSuffix(clean, "…") || strings.HasSuffix(clean, "...") || strings.HasSuffix(clean, "─…") || len([]rune(clean)) <= 60 {
			tasks = append(tasks, fetchTask{index: i, bookID: b.ID})
		}
	}

	if len(tasks) == 0 {
		return enriched
	}

	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	for _, t := range tasks {
		wg.Add(1)
		go func(task fetchTask) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			detail, err := s.GetBookDetail(ctx, task.bookID)
			if err == nil && detail != nil && detail.Description != "" {
				s.SetCachedDescription(task.bookID, detail.Description)
				enriched[task.index].Description = detail.Description
			}
		}(t)
	}

	wg.Wait()
	return enriched
}

func (s *Wenku8Source) parseBookCard(sel *goquery.Selection) *model.BookSummary {
	linkSel := sel.Find("div:first-child a")
	if linkSel.Length() == 0 {
		return nil
	}

	href, _ := linkSel.Attr("href")
	if !strings.Contains(href, "/book/") {
		return nil
	}

	id := strings.TrimSuffix(strings.TrimPrefix(href, "/book/"), ".htm")
	if id == "" {
		return nil
	}

	rawTitle, _ := linkSel.Attr("title")
	if rawTitle == "" {
		rawTitle = strings.TrimSpace(sel.Find("div:last-child b a").AttrOr("title", ""))
	}
	if rawTitle == "" {
		rawTitle = strings.TrimSpace(sel.Find("div:last-child b a").Text())
	}

	title := rawTitle
	subtitle := ""
	if matches := titleRegex.FindStringSubmatch(rawTitle); len(matches) == 3 {
		title = strings.TrimSpace(matches[1])
		subtitle = strings.TrimSpace(matches[2])
	}

	coverURL, _ := linkSel.Find("img").Attr("src")

	author := ""
	publisher := ""
	infoP2 := sel.Find("div:last-child p:nth-of-type(1)").Text()
	for _, part := range strings.Split(infoP2, "/") {
		kv := strings.SplitN(strings.TrimSpace(part), ":", 2)
		if len(kv) == 2 {
			switch strings.TrimSpace(kv[0]) {
			case "作者":
				author = strings.TrimSpace(kv[1])
			case "分类":
				publisher = strings.TrimSpace(kv[1])
			}
		}
	}

	lastUpdated := ""
	wordCount := 0
	isComplete := false
	infoP3 := sel.Find("div:last-child p:nth-of-type(2)").Text()
	for _, part := range strings.Split(infoP3, "/") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "已完结") {
			isComplete = true
		}
		kv := strings.SplitN(part, ":", 2)
		if len(kv) == 2 {
			k := strings.TrimSpace(kv[0])
			v := strings.TrimSpace(kv[1])
			switch k {
			case "更新":
				lastUpdated = v
			case "字数":
				vClean := strings.TrimSuffix(v, "K")
				if n, err := strconv.Atoi(vClean); err == nil {
					wordCount = n * 1000
				}
			}
		}
	}

	var tags []string
	desc := ""

	sel.Find("div:last-child p").Each(func(_ int, p *goquery.Selection) {
		text := strings.TrimSpace(p.Text())
		if strings.HasPrefix(text, "简介:") || strings.HasPrefix(text, "简介：") {
			desc = strings.TrimPrefix(text, "简介:")
			desc = strings.TrimPrefix(desc, "简介：")
			desc = strings.TrimSpace(desc)
		} else if strings.HasPrefix(text, "Tags:") || strings.HasPrefix(text, "Tags：") || p.Find("span").Length() > 0 {
			rawSpan := p.Find("span").Text()
			if rawSpan != "" {
				tags = strings.Fields(strings.TrimSpace(rawSpan))
			}
		}
	})

	if desc == "" {
		desc = sel.Find("div:last-child p:nth-of-type(4)").Text()
		desc = strings.TrimPrefix(desc, "简介:")
		desc = strings.TrimPrefix(desc, "简介：")
		desc = strings.TrimSpace(desc)
	}

	if s != nil {
		if cachedDesc, ok := s.GetCachedDescription(id); ok && cachedDesc != "" {
			desc = cachedDesc
		}
	}

	return &model.BookSummary{
		ID:          id,
		Title:       title,
		Subtitle:    subtitle,
		Author:      author,
		CoverURL:    coverURL,
		Description: desc,
		Tags:        tags,
		Publisher:   publisher,
		WordCount:   wordCount,
		LastUpdated: lastUpdated,
		IsComplete:  isComplete,
	}
}

func parseBookCard(sel *goquery.Selection) *model.BookSummary {
	var src *Wenku8Source
	return src.parseBookCard(sel)
}
