package wenku8

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestDefaultTags(t *testing.T) {
	src := &Wenku8Source{}
	tags := src.GetTags()
	if len(tags) < 40 {
		t.Errorf("expected at least 40 tags, got %d", len(tags))
	}

	found := false
	for _, tag := range tags {
		if tag == "恋爱" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected tags to contain '恋爱'")
	}
}

func TestExtractPagination(t *testing.T) {
	html := `<html><body><div id="pagelink"><em>1 / 15</em></div></body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}

	pages := extractPagination(doc)
	if pages != 15 {
		t.Errorf("expected 15 pages, got %d", pages)
	}
}

func TestExtractBookCards(t *testing.T) {
	html := `
	<html>
	<body>
	<div id="content">
		<table class="grid">
			<tr>
				<td>
					<div>
						<div><a href="/book/1001.htm" title="刀剑神域 (Sword Art Online)"><img src="http://img.wenku8.com/1001s.jpg"/></a></div>
						<div>
							<p>作者: 川原砾 / 分类: 电击文库</p>
							<p>更新: 2024-01-01 / 字数: 2500K / 连载中</p>
							<p><span>科幻 冒险 游戏</span></p>
							<p>简介: 无法完全攻略就无法离开游戏...</p>
						</div>
					</div>
				</td>
			</tr>
		</table>
	</div>
	</body>
	</html>
	`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}

	cards := extractBookCards(doc)
	if len(cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(cards))
	}
	card := cards[0]
	if card.ID != "1001" {
		t.Errorf("expected ID 1001, got %s", card.ID)
	}
	if card.Title != "刀剑神域" {
		t.Errorf("expected Title 刀剑神域, got %s", card.Title)
	}
	if card.Subtitle != "Sword Art Online" {
		t.Errorf("expected Subtitle Sword Art Online, got %s", card.Subtitle)
	}
	if card.Author != "川原砾" {
		t.Errorf("expected Author 川原砾, got %s", card.Author)
	}
	if card.WordCount != 2500000 {
		t.Errorf("expected WordCount 2500000, got %d", card.WordCount)
	}
	if len(card.Tags) != 3 {
		t.Errorf("expected 3 tags, got %d", len(card.Tags))
	}
}

func TestDefaultPublishers(t *testing.T) {
	src := &Wenku8Source{}
	pubs := src.GetPublishers()
	if len(pubs) != 14 {
		t.Fatalf("expected 14 publishers, got %d", len(pubs))
	}
	if pubs[0].ClassID != 1 || pubs[0].Name != "电击文库" {
		t.Errorf("expected class 1 to be 电击文库, got %+v", pubs[0])
	}
	if pubs[2].ClassID != 3 || pubs[2].Name != "角川文库" {
		t.Errorf("expected class 3 to be 角川文库, got %+v", pubs[2])
	}
	if pubs[13].ClassID != 14 || pubs[13].Name != "游戏剧本" {
		t.Errorf("expected class 14 to be 游戏剧本, got %+v", pubs[13])
	}
}
