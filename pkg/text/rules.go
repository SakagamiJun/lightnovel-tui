package text

import (
	"regexp"
	"strings"
	"sync"
)

// FormattingRule defines a text cleaning rule that can be applied to novel chapters.
type FormattingRule struct {
	ID          string `json:"id"`
	BookID      string `json:"book_id,omitempty"` // "" for global rules; non-empty for book-specific rules
	Name        string `json:"name"`
	IsRegex     bool   `json:"is_regex"`
	Pattern     string `json:"pattern"`
	Replacement string `json:"replacement"`
	Enabled     bool   `json:"enabled"`
}

// DefaultRules provides built-in high-frequency formatting and watermark cleaning rules.
var DefaultRules = []FormattingRule{
	{
		ID:          "builtin_ellipsis",
		Name:        "规范省略号 (.. -> ……)",
		IsRegex:     true,
		Pattern:     `\.{2,}`,
		Replacement: "……",
		Enabled:     true,
	},
	{
		ID:          "builtin_dash",
		Name:        "规范破折号 (-- -> ——)",
		IsRegex:     true,
		Pattern:     `-{2,}`,
		Replacement: "——",
		Enabled:     true,
	},
	{
		ID:          "builtin_watermark",
		Name:        "过滤转帖水印广告",
		IsRegex:     true,
		Pattern:     `(?i)^.*(百度贴吧|最新最快|首发|动漫之家|录入).*$`,
		Replacement: "",
		Enabled:     true,
	},
	{
		ID:          "builtin_duplicate_lines",
		Name:        "消除连续冗余空行",
		IsRegex:     true,
		Pattern:     `\n{3,}`,
		Replacement: "\n\n",
		Enabled:     true,
	},
}

var (
	regexCache   = make(map[string]*regexp.Regexp)
	regexCacheMu sync.RWMutex
)

func getCompiledRegex(pattern string) (*regexp.Regexp, error) {
	regexCacheMu.RLock()
	re, ok := regexCache[pattern]
	regexCacheMu.RUnlock()
	if ok {
		return re, nil
	}

	regexCacheMu.Lock()
	defer regexCacheMu.Unlock()
	if re, ok := regexCache[pattern]; ok {
		return re, nil
	}

	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	regexCache[pattern] = compiled
	return compiled, nil
}

// ApplyRules processes text through the enabled rules matching the specified bookID.
// Rules are evaluated sequentially in order.
func ApplyRules(input string, bookID string, rules []FormattingRule) string {
	if len(rules) == 0 || input == "" {
		return input
	}

	result := input
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if r.BookID != "" && r.BookID != bookID {
			continue
		}
		if r.Pattern == "" {
			continue
		}

		if r.IsRegex {
			re, err := getCompiledRegex(r.Pattern)
			if err == nil && re != nil {
				result = re.ReplaceAllString(result, r.Replacement)
			}
		} else {
			result = strings.ReplaceAll(result, r.Pattern, r.Replacement)
		}
	}

	return result
}
