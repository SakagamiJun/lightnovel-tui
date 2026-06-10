package image

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	stdimage "image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Protocol represents the terminal image display mechanism.
type Protocol int

const (
	// ProtocolHalfBlock renders image using 24-bit TrueColor ANSI half-block characters (▀).
	// Universally compatible with 100% of modern ANSI terminals.
	ProtocolHalfBlock Protocol = iota
	// ProtocolITerm2 renders image using iTerm2 / WezTerm inline image protocol.
	ProtocolITerm2
	// ProtocolKitty renders image using Kitty graphics protocol.
	ProtocolKitty
)

// String returns human-readable label for protocol.
func (p Protocol) String() string {
	switch p {
	case ProtocolITerm2:
		return "iTerm2"
	case ProtocolKitty:
		return "Kitty"
	default:
		return "ANSI-HalfBlock"
	}
}

// DetectTerminalProtocol auto-detects the best supported protocol for the current environment.
func DetectTerminalProtocol() Protocol {
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return ProtocolKitty
	}
	termProg := os.Getenv("TERM_PROGRAM")
	if termProg == "iTerm.app" || termProg == "WezTerm" || os.Getenv("LC_TERMINAL") == "iTerm2" {
		return ProtocolITerm2
	}
	return ProtocolHalfBlock
}

// OpenInSystemViewer opens the specified image file in the default system viewer asynchronously.
func OpenInSystemViewer(filePath string) error {
	if filePath == "" {
		return fmt.Errorf("empty file path")
	}
	if _, err := os.Stat(filePath); err != nil {
		return err
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", filePath)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", filePath)
	default:
		cmd = exec.Command("xdg-open", filePath)
	}
	return cmd.Start()
}

// DownloadImageToFile downloads an image from imgURL to destPath streaming with anti-hotlinking referer.
func DownloadImageToFile(ctx context.Context, imgURL, destPath string) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", imgURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://www.wenku8.cc/")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	buf := byteBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	buf.Grow(32 * 1024)
	chunk := buf.Bytes()[:32*1024]
	defer byteBufferPool.Put(buf)

	_, err = io.CopyBuffer(out, resp.Body, chunk)
	return err
}

// LoadImage loads and decodes an image file from disk.
func LoadImage(filePath string) (stdimage.Image, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := stdimage.Decode(f)
	if err != nil {
		return nil, err
	}
	return img, nil
}

// CalculateScaledDimensions computes terminal column width and character row height
// preserving the image aspect ratio under a standard 1:2 terminal cell aspect ratio.
// (Because 1 character cell has 2 vertical half-block pixels, terminal pixel canvas is cols x (rows*2)).
func CalculateScaledDimensions(origW, origH, maxCols, maxRows int) (cols, rows int) {
	if origW <= 0 || origH <= 0 || maxCols <= 0 || maxRows <= 0 {
		return 1, 1
	}

	maxPixelW := float64(maxCols)
	maxPixelH := float64(maxRows * 2)

	scaleW := maxPixelW / float64(origW)
	scaleH := maxPixelH / float64(origH)

	scale := scaleW
	if scaleH < scale {
		scale = scaleH
	}

	cols = int(float64(origW) * scale)
	if cols < 1 {
		cols = 1
	}
	if cols > maxCols {
		cols = maxCols
	}

	pixelH := int(float64(origH) * scale)
	// Round up pixelH to even number so every row has upper and lower pixels
	if pixelH%2 != 0 {
		pixelH++
	}
	if pixelH < 2 {
		pixelH = 2
	}
	rows = pixelH / 2
	if rows > maxRows {
		rows = maxRows
	}

	return cols, rows
}

// byteBufferPool pools bytes.Buffer to minimize heap garbage during repeated rendering.
var byteBufferPool = sync.Pool{
	New: func() any {
		return new(bytes.Buffer)
	},
}

// RenderHalfBlock renders the given image into a multi-line ANSI TrueColor string
// using half-block characters (▀).
func RenderHalfBlock(img stdimage.Image, maxCols, maxRows int) string {
	bounds := img.Bounds()
	origW := bounds.Dx()
	origH := bounds.Dy()
	if origW <= 0 || origH <= 0 {
		return ""
	}

	targetCols, targetRows := CalculateScaledDimensions(origW, origH, maxCols, maxRows)
	targetPixelH := targetRows * 2

	// Pre-estimate buffer capacity to avoid re-allocations
	// ~30 bytes per half block cell + newlines
	buf := byteBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	buf.Grow(targetCols * targetRows * 35)
	defer byteBufferPool.Put(buf)

	type rgb struct {
		r, g, b uint8
	}

	var lastFg, lastBg rgb
	hasLastFg, hasLastBg := false, false

	for charY := 0; charY < targetRows; charY++ {
		topPixelY := charY * 2
		botPixelY := topPixelY + 1

		for x := 0; x < targetCols; x++ {
			// Sample top pixel
			tr, tg, tb := sampleArea(img, bounds, x, topPixelY, targetCols, targetPixelH)
			topColor := rgb{tr, tg, tb}

			// Sample bottom pixel
			var botColor rgb
			if botPixelY < targetPixelH {
				br, bg, bb := sampleArea(img, bounds, x, botPixelY, targetCols, targetPixelH)
				botColor = rgb{br, bg, bb}
			} else {
				botColor = rgb{0, 0, 0}
			}

			// Foreground (top half of ▀)
			if !hasLastFg || lastFg != topColor {
				fmt.Fprintf(buf, "\x1b[38;2;%d;%d;%dm", topColor.r, topColor.g, topColor.b)
				lastFg = topColor
				hasLastFg = true
			}

			// Background (bottom half of ▀)
			if !hasLastBg || lastBg != botColor {
				fmt.Fprintf(buf, "\x1b[48;2;%d;%d;%dm", botColor.r, botColor.g, botColor.b)
				lastBg = botColor
				hasLastBg = true
			}

			buf.WriteString("▀")
		}

		// Reset ANSI state at end of line
		buf.WriteString("\x1b[0m\n")
		hasLastFg, hasLastBg = false, false
	}

	return buf.String()
}

// sampleArea computes the averaged RGB value for a target pixel mapped back to source image bounds.
func sampleArea(img stdimage.Image, b stdimage.Rectangle, tx, ty, targetW, targetH int) (uint8, uint8, uint8) {
	sx0 := b.Min.X + (tx*b.Dx())/targetW
	sx1 := b.Min.X + ((tx+1)*b.Dx())/targetW
	if sx1 <= sx0 {
		sx1 = sx0 + 1
	}

	sy0 := b.Min.Y + (ty*b.Dy())/targetH
	sy1 := b.Min.Y + ((ty+1)*b.Dy())/targetH
	if sy1 <= sy0 {
		sy1 = sy0 + 1
	}

	dx := sx1 - sx0
	dy := sy1 - sy0

	// For tiny boxes, sum all source pixels
	if dx*dy <= 16 {
		var sumR, sumG, sumB, count uint64
		for y := sy0; y < sy1; y++ {
			for x := sx0; x < sx1; x++ {
				r, g, b, a := img.At(x, y).RGBA()
				if a > 0 {
					// Premultiplied alpha to straight RGB
					r = (r * 0xffff) / a
					g = (g * 0xffff) / a
					b = (b * 0xffff) / a
				}
				sumR += uint64(r >> 8)
				sumG += uint64(g >> 8)
				sumB += uint64(b >> 8)
				count++
			}
		}
		if count > 0 {
			return uint8(sumR / count), uint8(sumG / count), uint8(sumB / count)
		}
		return 0, 0, 0
	}

	// For larger downsample regions, sample a fast 3x3 grid to minimize CPU cycles
	var sumR, sumG, sumB uint64
	stepX := dx / 3
	if stepX < 1 {
		stepX = 1
	}
	stepY := dy / 3
	if stepY < 1 {
		stepY = 1
	}

	var count uint64
	for sy := sy0 + stepY/2; sy < sy1; sy += stepY {
		for sx := sx0 + stepX/2; sx < sx1; sx += stepX {
			r, g, b, a := img.At(sx, sy).RGBA()
			if a > 0 {
				r = (r * 0xffff) / a
				g = (g * 0xffff) / a
				b = (b * 0xffff) / a
			}
			sumR += uint64(r >> 8)
			sumG += uint64(g >> 8)
			sumB += uint64(b >> 8)
			count++
		}
	}
	if count > 0 {
		return uint8(sumR / count), uint8(sumG / count), uint8(sumB / count)
	}
	return 0, 0, 0
}

// RenderITerm2File reads an image file from disk and encodes it using the iTerm2 inline protocol.
func RenderITerm2File(filePath string, maxCols, maxRows int) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("\x1b]1337;File=inline=1;width=%d;height=%d;preserveAspectRatio=1:%s\a\n", maxCols, maxRows, encoded), nil
}

// RenderITerm2 encodes an in-memory image into JPEG and outputs the iTerm2 inline protocol escape sequence.
func RenderITerm2(img stdimage.Image, maxCols, maxRows int) (string, error) {
	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return fmt.Sprintf("\x1b]1337;File=inline=1;width=%d;height=%d;preserveAspectRatio=1:%s\a\n", maxCols, maxRows, encoded), nil
}

// RenderKittyFile encodes an image file using Kitty graphics protocol chunks.
func RenderKittyFile(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return formatKittyChunks(data), nil
}

// formatKittyChunks formats binary data into Kitty APC G escape chunks.
func formatKittyChunks(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	const chunkSize = 4096

	var sb strings.Builder
	for i := 0; i < len(encoded); i += chunkSize {
		end := i + chunkSize
		m := 1
		if end >= len(encoded) {
			end = len(encoded)
			m = 0
		}
		chunk := encoded[i:end]
		if i == 0 {
			sb.WriteString(fmt.Sprintf("\x1b_Ga=T,f=100,m=%d;%s\x1b\\", m, chunk))
		} else {
			sb.WriteString(fmt.Sprintf("\x1b_Gm=%d;%s\x1b\\", m, chunk))
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

// RenderFile renders an image file into terminal output using the specified protocol.
func RenderFile(filePath string, maxCols, maxRows int, proto Protocol) (string, error) {
	switch proto {
	case ProtocolITerm2:
		out, err := RenderITerm2File(filePath, maxCols, maxRows)
		if err == nil {
			return out, nil
		}
		// Fallback to half-block on error
	case ProtocolKitty:
		out, err := RenderKittyFile(filePath)
		if err == nil {
			return out, nil
		}
		// Fallback to half-block on error
	}

	img, err := LoadImage(filePath)
	if err != nil {
		return "", err
	}
	return RenderHalfBlock(img, maxCols, maxRows), nil
}

// RenderAutoFile auto-detects the terminal protocol and renders the image file.
func RenderAutoFile(filePath string, maxCols, maxRows int) (string, error) {
	return RenderFile(filePath, maxCols, maxRows, DetectTerminalProtocol())
}
