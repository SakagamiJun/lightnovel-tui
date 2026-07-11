package text

import (
	"testing"
)

func TestApplyRules(t *testing.T) {
	rules := []FormattingRule{
		{
			ID:          "r1",
			Name:        "Ellipsis",
			IsRegex:     true,
			Pattern:     `\.{2,}`,
			Replacement: "……",
			Enabled:     true,
		},
		{
			ID:          "r2",
			Name:        "Dash",
			IsRegex:     true,
			Pattern:     `-{2,}`,
			Replacement: "——",
			Enabled:     true,
		},
		{
			ID:          "r3",
			Name:        "Watermark",
			IsRegex:     true,
			Pattern:     `(?i)^.*(百度贴吧|最新最快|首发).*$`,
			Replacement: "",
			Enabled:     true,
		},
		{
			ID:          "r4_disabled",
			Name:        "Disabled Rule",
			IsRegex:     false,
			Pattern:     "foo",
			Replacement: "bar",
			Enabled:     false,
		},
		{
			ID:          "r5_book_specific",
			Name:        "Book specific fix",
			BookID:      "book_123",
			IsRegex:     false,
			Pattern:     "桐谷和人",
			Replacement: "桐人",
			Enabled:     true,
		},
	}

	// 1. Test ellipsis and dash
	input1 := "“……这真的可能吗......？”少年喃喃自语--然后拔出了长剑。"
	output1 := ApplyRules(input1, "other_book", rules)
	expected1 := "“……这真的可能吗……？”少年喃喃自语——然后拔出了长剑。"
	if output1 != expected1 {
		t.Errorf("expected %q, got %q", expected1, output1)
	}

	// 2. Test watermark removal
	input2 := "本章由百度贴吧手打团首发制作，严禁商业用途"
	output2 := ApplyRules(input2, "other_book", rules)
	if output2 != "" {
		t.Errorf("expected empty string for watermark, got %q", output2)
	}

	// 3. Test disabled rule
	input3 := "foo baz"
	output3 := ApplyRules(input3, "other_book", rules)
	if output3 != "foo baz" {
		t.Errorf("disabled rule should not modify text, got %q", output3)
	}

	// 4. Test book-specific rule
	input4 := "桐谷和人走向前方"
	outputOther := ApplyRules(input4, "other_book", rules)
	if outputOther != "桐谷和人走向前方" {
		t.Errorf("book specific rule should not apply to other_book, got %q", outputOther)
	}
	outputMatch := ApplyRules(input4, "book_123", rules)
	if outputMatch != "桐人走向前方" {
		t.Errorf("book specific rule should apply to book_123, got %q", outputMatch)
	}
}
