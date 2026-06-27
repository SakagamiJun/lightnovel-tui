package text

import (
	"reflect"
	"testing"

	"lnr-core/pkg/model"
)

func TestSimplifiedTraditionalConversion(t *testing.T) {
	tests := []struct {
		simplified  string
		traditional string
	}{
		{"轻小说文库与魔法少女", "輕小說文庫與魔法少女"},
		{"欢迎来到实力至上主义的教室", "歡迎來到實力至上主義的教室"},
		{"关于我转生变成史莱姆这档事", "關于我轉生變成史萊姆這檔事"},
		{"无职转生～到了异世界就拿出真本事～", "無職轉生～到了異世界就拿出真本事～"},
		{"刀剑神域", "刀劍神域"},
	}

	for _, tt := range tests {
		trad := ToTraditional(tt.simplified)
		if trad != tt.traditional {
			t.Errorf("ToTraditional(%q) = %q, want %q", tt.simplified, trad, tt.traditional)
		}
		simp := ToSimplified(tt.traditional)
		if simp != tt.simplified {
			t.Errorf("ToSimplified(%q) = %q, want %q", tt.traditional, simp, tt.simplified)
		}
	}
}

func TestNormalizePunctuation(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"你好,世界!今天天气真好?", "你好，世界！今天天气真好？"},
		{"时间是12:30,版本号是3.14.", "时间是12:30，版本号是3.14。"},
		{"这是一段对话(括号内容);结束了...", "这是一段对话（括号内容）；结束了……"},
	}

	for _, tt := range tests {
		got := NormalizePunctuation(tt.input)
		if got != tt.want {
			t.Errorf("NormalizePunctuation(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestWatermarkAndClean(t *testing.T) {
	if !IsWatermark("本文来自 轻小说文库(http://www.wenku8.com)") {
		t.Errorf("expected watermark detection for wenku8 line")
	}
	if !IsWatermark("最新最全的日本动漫轻小说") {
		t.Errorf("expected watermark detection for site slogan")
	}
	if IsWatermark("今天男主角来到了新的城市。") {
		t.Errorf("unexpected watermark detection for normal novel text")
	}

	cleaned := CleanParagraph("  轻小说文库(www.wenku8.com)  ")
	if cleaned != "" {
		t.Errorf("CleanParagraph watermark should return empty string, got: %q", cleaned)
	}

	cleaned = CleanParagraph("  “你好呀!”小声说道.  ")
	want := "“你好呀！”小声说道。"
	if cleaned != want {
		t.Errorf("CleanParagraph = %q, want %q", cleaned, want)
	}

	indented := IndentParagraph(cleaned)
	if indented != "\u3000\u3000"+want {
		t.Errorf("IndentParagraph failed: %q", indented)
	}
}

func TestFormatNovelLines(t *testing.T) {
	elements := []model.ContentElement{
		{Type: model.ContentTypeText, Text: "第一句话,你好!\nwenku8.com 最新发布\n第二句话.很高兴见到你."},
		{Type: model.ContentTypeImage, URL: "https://example.com/cover.jpg"},
	}

	lines, illusts := FormatNovelLines(elements, false)
	if len(illusts) != 1 || illusts[0] != "https://example.com/cover.jpg" {
		t.Errorf("unexpected illustrations: %v", illusts)
	}

	wantLines := []string{
		"\u3000\u3000第一句话，你好！",
		"\u3000\u3000第二句话。很高兴见到你。",
		"[插图: 第 1 张] (按 [i] 查看插图大图)",
	}
	if !reflect.DeepEqual(lines, wantLines) {
		t.Errorf("FormatNovelLines got %v, want %v", lines, wantLines)
	}

	tradLines, _ := FormatNovelLines(elements, true)
	if len(tradLines) != 3 {
		t.Fatalf("expected 3 lines in traditional mode")
	}
	if !reflect.DeepEqual(tradLines[0], "\u3000\u3000第一句話，你好！") {
		t.Errorf("FormatNovelLines traditional got %q", tradLines[0])
	}
}
