package text

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/SakagamiJun/lightnovel-tui/pkg/model"
)

// watermarkKeywords contains substrings that identify advertisement or site watermark lines in Wenku8.
var watermarkKeywords = []string{
	"wenku8.com",
	"wenku8.cn",
	"轻小说文库",
	"本文来自 轻小说文库",
	"最新最全的日本动漫轻小说",
}

// IsWatermark returns true if the given line is an advertisement or watermark line.
func IsWatermark(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	if lower == "" {
		return false
	}
	for _, kw := range watermarkKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// NormalizePunctuation normalizes half-width punctuation commonly found in web scrapes
// into standard Chinese full-width punctuation, while preserving numbers, ellipses, etc.
func NormalizePunctuation(s string) string {
	if s == "" {
		return ""
	}

	// First handle ellipses
	s = strings.ReplaceAll(s, "……", "……")
	s = strings.ReplaceAll(s, "...", "……")
	s = strings.ReplaceAll(s, "..", "……")

	runes := []rune(s)
	n := len(runes)
	var sb strings.Builder
	sb.Grow(len(s) + 8)

	for i := 0; i < n; i++ {
		r := runes[i]
		switch r {
		case ',':
			sb.WriteRune('，')
		case '?':
			sb.WriteRune('？')
		case '!':
			sb.WriteRune('！')
		case ';':
			sb.WriteRune('；')
		case ':':
			// If surrounded by digits (like 12:00) keep colon
			if i > 0 && i+1 < n && unicode.IsDigit(runes[i-1]) && unicode.IsDigit(runes[i+1]) {
				sb.WriteRune(':')
			} else {
				sb.WriteRune('：')
			}
		case '(':
			sb.WriteRune('（')
		case ')':
			sb.WriteRune('）')
		case '.':
			// If surrounded by digits (like 3.14) keep dot
			if i > 0 && i+1 < n && unicode.IsDigit(runes[i-1]) && unicode.IsDigit(runes[i+1]) {
				sb.WriteRune('.')
			} else {
				sb.WriteRune('。')
			}
		default:
			sb.WriteRune(r)
		}
	}

	return sb.String()
}

// CleanParagraph trims spaces, checks for watermarks, and standardizes punctuation.
// Returns an empty string if the paragraph is empty or is a watermark line.
func CleanParagraph(p string) string {
	trimmed := strings.Trim(p, " \t\r\n\u3000")
	if trimmed == "" {
		return ""
	}
	if IsWatermark(trimmed) {
		return ""
	}
	return NormalizePunctuation(trimmed)
}

// IndentParagraph adds standard Chinese novel two full-width ideographic spaces indentation.
func IndentParagraph(p string) string {
	if p == "" {
		return ""
	}
	return "\u3000\u3000" + p
}

// FormatNovelLines formats content elements into clean display lines.
// FormatNovelLines formats content elements into clean display lines.
// It removes watermarks, normalizes punctuation, applies full-width indentation,
// and optionally converts text to Traditional Chinese.
func FormatNovelLines(elements []model.ContentElement, traditional bool) ([]string, []string) {
	return FormatNovelLinesWithRules(elements, traditional, "", nil)
}

// FormatNovelLinesWithRules formats content elements into clean display lines with custom formatting rules applied.
func FormatNovelLinesWithRules(elements []model.ContentElement, traditional bool, bookID string, rules []FormattingRule) ([]string, []string) {
	lines := make([]string, 0, len(elements)*2)
	illustrations := make([]string, 0)

	for _, el := range elements {
		if el.Type == model.ContentTypeText {
			for _, rawLine := range strings.Split(el.Text, "\n") {
				cleaned := CleanParagraph(rawLine)
				if cleaned == "" {
					continue
				}
				if len(rules) > 0 {
					cleaned = ApplyRules(cleaned, bookID, rules)
					cleaned = strings.Trim(cleaned, " \t\r\n\u3000")
					if cleaned == "" {
						continue
					}
				}
				if traditional {
					cleaned = ToTraditional(cleaned)
				}
				lines = append(lines, IndentParagraph(cleaned))
			}
		} else if el.Type == model.ContentTypeImage && el.URL != "" {
			illustrations = append(illustrations, el.URL)
			imgIndex := len(illustrations)
			badge := fmt.Sprintf("[插图: 第 %d 张] (按 [i] 查看插图大图)", imgIndex)
			if traditional {
				badge = ToTraditional(badge)
			}
			lines = append(lines, badge)
		}
	}

	return lines, illustrations
}
