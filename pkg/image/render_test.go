package image

import (
	stdimage "image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCalculateScaledDimensions(t *testing.T) {
	tests := []struct {
		origW, origH     int
		maxCols, maxRows int
		wantCols         int
		wantRows         int
	}{
		// 1:1 square image fitting in 80 cols x 40 rows
		// Pixel canvas = 80 x 80. Orig is 100 x 100. Scale = 0.8 -> cols = 80, pixelH = 80 -> rows = 40
		{100, 100, 80, 40, 80, 40},
		// Wide image 200 x 100 fitting in 80 cols x 40 rows
		// ScaleW = 80/200 = 0.4. ScaleH = 80/100 = 0.8. Min scale = 0.4.
		// Cols = 200 * 0.4 = 80. PixelH = 100 * 0.4 = 40. Rows = 20.
		{200, 100, 80, 40, 80, 20},
		// Tall image 100 x 200 fitting in 80 cols x 40 rows
		// ScaleW = 80/100 = 0.8. ScaleH = 80/200 = 0.4. Min scale = 0.4.
		// Cols = 100 * 0.4 = 40. PixelH = 200 * 0.4 = 80. Rows = 40.
		{100, 200, 80, 40, 40, 40},
		// Zero or invalid dimensions
		{0, 0, 80, 40, 1, 1},
	}

	for _, tt := range tests {
		gotCols, gotRows := CalculateScaledDimensions(tt.origW, tt.origH, tt.maxCols, tt.maxRows)
		if gotCols != tt.wantCols || gotRows != tt.wantRows {
			t.Errorf("CalculateScaledDimensions(%d, %d, %d, %d) = (%d, %d), want (%d, %d)",
				tt.origW, tt.origH, tt.maxCols, tt.maxRows, gotCols, gotRows, tt.wantCols, tt.wantRows)
		}
	}
}

func TestRenderHalfBlock(t *testing.T) {
	// Create a 4x4 test image: top half red, bottom half blue
	img := stdimage.NewRGBA(stdimage.Rect(0, 0, 4, 4))
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	for y := 2; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255})
		}
	}

	// Render into 4 cols x 2 rows
	rendered := RenderHalfBlock(img, 4, 2)
	if rendered == "" {
		t.Fatalf("RenderHalfBlock returned empty string")
	}

	// Should contain ANSI sequences and half block characters
	if !strings.Contains(rendered, "▀") {
		t.Errorf("RenderHalfBlock output missing half block character: %s", rendered)
	}
	if !strings.Contains(rendered, "\x1b[38;2;") || !strings.Contains(rendered, "\x1b[48;2;") {
		t.Errorf("RenderHalfBlock output missing 24-bit TrueColor sequences: %s", rendered)
	}

	// Empty image
	emptyImg := stdimage.NewRGBA(stdimage.Rect(0, 0, 0, 0))
	if res := RenderHalfBlock(emptyImg, 10, 10); res != "" {
		t.Errorf("Expected empty string for 0x0 image, got %q", res)
	}
}

func TestRenderITerm2(t *testing.T) {
	img := stdimage.NewRGBA(stdimage.Rect(0, 0, 10, 10))
	out, err := RenderITerm2(img, 20, 10)
	if err != nil {
		t.Fatalf("RenderITerm2 failed: %v", err)
	}
	if !strings.HasPrefix(out, "\x1b]1337;File=inline=1;") {
		t.Errorf("RenderITerm2 unexpected prefix: %s", out)
	}
}

func TestRenderFile(t *testing.T) {
	tempDir := t.TempDir()
	imgPath := filepath.Join(tempDir, "test.png")

	img := stdimage.NewRGBA(stdimage.Rect(0, 0, 8, 8))
	img.Set(0, 0, color.RGBA{R: 100, G: 150, B: 200, A: 255})

	f, err := os.Create(imgPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	// Test half block
	out, err := RenderFile(imgPath, 8, 4, ProtocolHalfBlock)
	if err != nil {
		t.Fatalf("RenderFile half-block failed: %v", err)
	}
	if !strings.Contains(out, "▀") {
		t.Errorf("RenderFile half-block missing block character")
	}

	// Test iTerm2
	outITerm, err := RenderFile(imgPath, 8, 4, ProtocolITerm2)
	if err != nil {
		t.Fatalf("RenderFile iTerm2 failed: %v", err)
	}
	if !strings.Contains(outITerm, "\x1b]1337;File=inline=1;") {
		t.Errorf("RenderFile iTerm2 missing protocol header")
	}

	// Test Kitty
	outKitty, err := RenderFile(imgPath, 8, 4, ProtocolKitty)
	if err != nil {
		t.Fatalf("RenderFile Kitty failed: %v", err)
	}
	if !strings.Contains(outKitty, "\x1b_G") {
		t.Errorf("RenderFile Kitty missing protocol header")
	}
}
