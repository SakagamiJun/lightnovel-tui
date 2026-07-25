package theme

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

func TestTruncateBehavior(t *testing.T) {
	testStr := "吊车尾魔法使玛吉，在历经无数研究后，竟然创造出了妖艳的人型史莱姆——“史莱子”。"
	t.Logf("testStr len: %d, runewidth: %d, ansi.StringWidth: %d", len(testStr), runewidth.StringWidth(testStr), ansi.StringWidth(testStr))

	for width := 20; width <= 80; width += 20 {
		truncANSI := TruncateANSI(testStr, width, "...")
		truncRW := runewidth.Truncate(testStr, width, "...")
		t.Logf("width=%d:\n  TruncateANSI: %q (ansiWidth=%d, rwWidth=%d)\n  runewidth:    %q (ansiWidth=%d, rwWidth=%d)",
			width,
			truncANSI, ansi.StringWidth(truncANSI), runewidth.StringWidth(truncANSI),
			truncRW, ansi.StringWidth(truncRW), runewidth.StringWidth(truncRW),
		)

		if ansi.StringWidth(truncANSI) > width {
			t.Errorf("TruncateANSI width %d exceeded budget %d", ansi.StringWidth(truncANSI), width)
		}
	}
}
