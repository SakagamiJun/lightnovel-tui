package image

import (
	"bytes"
	"encoding/base64"
	stdimage "image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestKittyPlaceholderGrid(t *testing.T) {
	const imageID = 0x010203
	grid := KittyPlaceholderGrid(imageID, 3, 2)
	if len(grid) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(grid))
	}

	for row, line := range grid {
		if got := ansi.StringWidth(line); got != 3 {
			t.Errorf("row %d width = %d, want 3 (line=%q)", row, got, line)
		}
		if got := strings.Count(line, string(kittyPlaceholderRune)); got != 3 {
			t.Errorf("row %d placeholder count = %d, want 3", row, got)
		}
		if !strings.Contains(line, "\x1b[38;2;1;2;3m") {
			t.Errorf("row %d missing image id foreground color: %q", row, line)
		}
	}

	// Row and column diacritics must follow the protocol ordering.
	if !strings.Contains(grid[0], string(kittyRowColumnDiacritics[0])+string(kittyRowColumnDiacritics[0])) {
		t.Errorf("first cell missing row=0/column=0 diacritics: %q", grid[0])
	}
	if !strings.Contains(grid[1], string(kittyRowColumnDiacritics[1])+string(kittyRowColumnDiacritics[0])) {
		t.Errorf("second row missing row=1/column=0 diacritics: %q", grid[1])
	}
	if !strings.Contains(grid[0], string(kittyRowColumnDiacritics[0])+string(kittyRowColumnDiacritics[1])) {
		t.Errorf("second column missing row=0/column=1 diacritics: %q", grid[0])
	}

	if got := KittyPlaceholderGrid(0, 3, 2); got != nil {
		t.Errorf("expected nil grid for image id 0, got %v", got)
	}
	if got := KittyPlaceholderGrid(kittyMaxImageID+1, 3, 2); got != nil {
		t.Errorf("expected nil grid for out-of-range image id, got %v", got)
	}
	large := KittyPlaceholderGrid(kittyMaxImageID, 72, 40)
	for row, line := range large {
		if got := ansi.StringWidth(line); got != 72 {
			t.Errorf("large grid row %d width = %d, want 72", row, got)
		}
	}

	if got := KittyPlaceholderGrid(imageID, 0, 2); got != nil {
		t.Errorf("expected nil grid for zero columns, got %v", got)
	}
}

func TestKittyChunkWriterExactBoundary(t *testing.T) {
	payload := make([]byte, kittyRawChunkSize)
	for i := range payload {
		payload[i] = byte(i)
	}

	var out bytes.Buffer
	kw := &kittyChunkWriter{w: &out, header: "a=T,f=100,i=1,"}
	if _, err := kw.Write(payload); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if !strings.HasSuffix(out.String(), "\x1b\\") {
		t.Fatalf("first chunk was not terminated: %q", out.String())
	}
	if err := kw.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	commands := extractKittyCommands(t, out.String())
	if len(commands) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(commands), commands)
	}
	if !strings.Contains(commands[0], "m=1;") {
		t.Errorf("first command should advertise more data: %q", commands[0])
	}
	if !strings.HasSuffix(commands[1], "m=0;") {
		t.Errorf("terminating command should be an empty m=0 chunk: %q", commands[1])
	}

	var decoded []byte
	for i, cmd := range commands {
		control, encoded := splitKittyCommand(t, cmd)
		chunk, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatalf("command %d has invalid base64: %v", i, err)
		}
		decoded = append(decoded, chunk...)
		if i == 0 && !strings.Contains(control, "a=T,f=100,i=1,") {
			t.Errorf("unexpected first command control: %q", control)
		}
	}
	if !bytes.Equal(decoded, payload) {
		t.Errorf("decoded payload mismatch: got %d bytes, want %d", len(decoded), len(payload))
	}
}

func TestTransmitKittyImagePNG(t *testing.T) {
	pngPath := writeNoisePNG(t, 64, 64)
	pngData, err := os.ReadFile(pngPath)
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	const imageID = 0x123456
	if err := TransmitKittyImage(&out, pngPath, imageID, 7, 5); err != nil {
		t.Fatalf("TransmitKittyImage failed: %v", err)
	}

	commands := extractKittyCommands(t, out.String())
	if len(commands) < 2 {
		t.Fatalf("expected a chunked transmission, got %d command(s)", len(commands))
	}

	firstControl, _ := splitKittyCommand(t, commands[0])
	for _, want := range []string{"a=T", "f=100", "i=1193046", "p=1", "U=1", "c=7", "r=5", "q=2", "m=1"} {
		if !strings.Contains(firstControl, want) {
			t.Errorf("first command missing %q: %q", want, firstControl)
		}
	}

	var decoded []byte
	for i, cmd := range commands {
		control, encoded := splitKittyCommand(t, cmd)
		if len(encoded) > kittyChunkSize {
			t.Errorf("command %d payload length %d exceeds chunk limit", i, len(encoded))
		}
		if i > 0 && strings.Contains(control, "a=T") {
			t.Errorf("command %d repeats the full header: %q", i, control)
		}
		chunk, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatalf("command %d has invalid base64: %v", i, err)
		}
		decoded = append(decoded, chunk...)
	}
	if !bytes.Equal(decoded, pngData) {
		t.Errorf("transmitted PNG bytes differ from source file")
	}

	lastControl, _ := splitKittyCommand(t, commands[len(commands)-1])
	if !strings.Contains(lastControl, "m=0") {
		t.Errorf("last command should set m=0: %q", lastControl)
	}
}

func TestTransmitKittyImageJPEGConvertsToPNG(t *testing.T) {
	dir := t.TempDir()
	jpegPath := filepath.Join(dir, "illustration.jpg")
	img := stdimage.NewRGBA(stdimage.Rect(0, 0, 32, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 8), G: uint8(y * 10), B: 120, A: 255})
		}
	}
	f, err := os.Create(jpegPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	var out bytes.Buffer
	if err := TransmitKittyImage(&out, jpegPath, 42, 4, 3); err != nil {
		t.Fatalf("TransmitKittyImage JPEG failed: %v", err)
	}

	var decoded []byte
	for _, cmd := range extractKittyCommands(t, out.String()) {
		_, encoded := splitKittyCommand(t, cmd)
		chunk, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatalf("invalid base64: %v", err)
		}
		decoded = append(decoded, chunk...)
	}
	decodedImage, err := png.Decode(bytes.NewReader(decoded))
	if err != nil {
		t.Fatalf("transmitted payload is not a valid PNG: %v", err)
	}
	if got := decodedImage.Bounds(); got.Dx() != 32 || got.Dy() != 24 {
		t.Errorf("decoded PNG size = %dx%d, want 32x24", got.Dx(), got.Dy())
	}
}

func TestKittyPlaceAndDelete(t *testing.T) {
	var out bytes.Buffer
	if err := PlaceKittyImage(&out, 9, 12, 6); err != nil {
		t.Fatalf("PlaceKittyImage failed: %v", err)
	}
	if got, want := out.String(), "\x1b_Ga=p,U=1,q=2,i=9,p=1,c=12,r=6\x1b\\"; got != want {
		t.Errorf("PlaceKittyImage = %q, want %q", got, want)
	}

	out.Reset()
	if err := DeleteKittyImage(&out, 9); err != nil {
		t.Fatalf("DeleteKittyImage failed: %v", err)
	}
	if got, want := out.String(), "\x1b_Ga=d,d=I,i=9,q=2\x1b\\"; got != want {
		t.Errorf("DeleteKittyImage = %q, want %q", got, want)
	}
}

func TestSupportsKittyPlaceholders(t *testing.T) {
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("KITTY_PID", "")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	t.Setenv(kittyProtocolEnv, "")

	if SupportsKittyPlaceholders() {
		t.Errorf("expected no Kitty placeholder support in a generic terminal")
	}

	t.Setenv("TERM", "xterm-kitty")
	if !SupportsKittyPlaceholders() {
		t.Errorf("expected Kitty placeholder support for xterm-kitty")
	}

	t.Setenv("TERM", "xterm-ghostty")
	if !SupportsKittyPlaceholders() {
		t.Errorf("expected Kitty placeholder support for xterm-ghostty")
	}

	t.Setenv("TERM", "xterm-256color")
	t.Setenv("TERM_PROGRAM", "ghostty")
	if !SupportsKittyPlaceholders() {
		t.Errorf("expected Kitty placeholder support for TERM_PROGRAM=ghostty")
	}

	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("KITTY_WINDOW_ID", "1")
	if !SupportsKittyPlaceholders() {
		t.Errorf("expected Kitty placeholder support with KITTY_WINDOW_ID")
	}

	t.Setenv(kittyProtocolEnv, "off")
	if SupportsKittyPlaceholders() {
		t.Errorf("expected explicit opt-out to win")
	}

	t.Setenv(kittyProtocolEnv, "kitty")
	if !SupportsKittyPlaceholders() {
		t.Errorf("expected explicit opt-in to win")
	}
}

func TestLockedWriter(t *testing.T) {
	var out bytes.Buffer
	lw := NewLockedWriter(&out)
	if lw == nil {
		t.Fatal("expected non-nil writer")
	}
	if _, err := lw.Write([]byte("a")); err != nil {
		t.Fatal(err)
	}
	if err := lw.WithLock(func(w io.Writer) error {
		_, err := w.Write([]byte("b"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "ab" {
		t.Errorf("LockedWriter output = %q, want %q", got, "ab")
	}
	if got := lw.Fd(); got != ^uintptr(0) {
		t.Errorf("Fd for a bytes.Buffer = %d, want invalid", got)
	}
	lw.Stop()
	if _, err := lw.Write([]byte("c")); err != io.ErrClosedPipe {
		t.Errorf("Write after Stop returned %v, want io.ErrClosedPipe", err)
	}
	if err := lw.WithLock(func(w io.Writer) error { return nil }); err != io.ErrClosedPipe {
		t.Errorf("WithLock after Stop returned %v, want io.ErrClosedPipe", err)
	}
	if NewLockedWriter(nil) != nil {
		t.Errorf("expected nil LockedWriter for nil writer")
	}
}

func writeNoisePNG(t *testing.T, width, height int) string {
	t.Helper()
	img := stdimage.NewRGBA(stdimage.Rect(0, 0, width, height))
	rng := rand.New(rand.NewSource(1))
	if _, err := rng.Read(img.Pix); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "noise.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func extractKittyCommands(t *testing.T, out string) []string {
	t.Helper()
	var commands []string
	rest := out
	for {
		start := strings.Index(rest, "\x1b_G")
		if start < 0 {
			break
		}
		rest = rest[start+3:]
		end := strings.Index(rest, "\x1b\\")
		if end < 0 {
			t.Fatalf("unterminated Kitty command in %q", rest)
		}
		commands = append(commands, rest[:end])
		rest = rest[end+2:]
	}
	if len(commands) == 0 {
		t.Fatalf("no Kitty commands found in output")
	}
	return commands
}

func splitKittyCommand(t *testing.T, cmd string) (control, payload string) {
	t.Helper()
	idx := strings.IndexByte(cmd, ';')
	if idx < 0 {
		t.Fatalf("malformed Kitty command (missing semicolon): %q", cmd)
	}
	return cmd[:idx], cmd[idx+1:]
}
