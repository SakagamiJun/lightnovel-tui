package image

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	stdimage "image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"strings"
	"sync"
)

const (
	// kittyPlaceholderRune is the Unicode placeholder cell defined by the
	// Kitty graphics protocol. The terminal replaces these cells with the
	// referenced image, which makes them safe to move around as normal text.
	kittyPlaceholderRune = '\U0010EEEE'

	// kittyChunkSize is the maximum number of base64 characters allowed in a
	// single Kitty graphics escape code.
	kittyChunkSize = 4096

	// kittyRawChunkSize is the number of raw bytes that encode to exactly one
	// full protocol chunk.
	kittyRawChunkSize = kittyChunkSize / 4 * 3

	// kittyMaxImageID is the largest image id that can be encoded in the
	// 24-bit foreground color of a Unicode placeholder.
	kittyMaxImageID = 0xFFFFFF

	// kittyProtocolOptIn can force-enable or disable placeholder rendering on
	// terminals that do not expose a recognizable environment variable.
	kittyProtocolEnv = "LNR_IMAGE_PROTOCOL"
)

// kittyRowColumnDiacritics maps a zero-based row/column index to the combining
// mark used by the Kitty placeholder protocol. The ordering is part of the
// protocol and is copied from kitty's gen/rowcolumn-diacritics.txt.
var kittyRowColumnDiacritics = []rune{
	0x0305, 0x030D, 0x030E, 0x0310, 0x0312, 0x033D, 0x033E, 0x033F, 0x0346, 0x034A, 0x034B, 0x034C,
	0x0350, 0x0351, 0x0352, 0x0357, 0x035B, 0x0363, 0x0364, 0x0365, 0x0366, 0x0367, 0x0368, 0x0369,
	0x036A, 0x036B, 0x036C, 0x036D, 0x036E, 0x036F, 0x0483, 0x0484, 0x0485, 0x0486, 0x0487, 0x0592,
	0x0593, 0x0594, 0x0595, 0x0597, 0x0598, 0x0599, 0x059C, 0x059D, 0x059E, 0x059F, 0x05A0, 0x05A1,
	0x05A8, 0x05A9, 0x05AB, 0x05AC, 0x05AF, 0x05C4, 0x0610, 0x0611, 0x0612, 0x0613, 0x0614, 0x0615,
	0x0616, 0x0617, 0x0657, 0x0658, 0x0659, 0x065A, 0x065B, 0x065D, 0x065E, 0x06D6, 0x06D7, 0x06D8,
	0x06D9, 0x06DA, 0x06DB, 0x06DC, 0x06DF, 0x06E0, 0x06E1, 0x06E2, 0x06E4, 0x06E7, 0x06E8, 0x06EB,
	0x06EC, 0x0730, 0x0732, 0x0733, 0x0735, 0x0736, 0x073A, 0x073D, 0x073F, 0x0740, 0x0741, 0x0743,
	0x0745, 0x0747, 0x0749, 0x074A, 0x07EB, 0x07EC, 0x07ED, 0x07EE, 0x07EF, 0x07F0, 0x07F1, 0x07F3,
	0x0816, 0x0817, 0x0818, 0x0819, 0x081B, 0x081C, 0x081D, 0x081E, 0x081F, 0x0820, 0x0821, 0x0822,
	0x0823, 0x0825, 0x0826, 0x0827, 0x0829, 0x082A, 0x082B, 0x082C, 0x082D, 0x0951, 0x0953, 0x0954,
	0x0F82, 0x0F83, 0x0F86, 0x0F87, 0x135D, 0x135E, 0x135F, 0x17DD, 0x193A, 0x1A17, 0x1A75, 0x1A76,
	0x1A77, 0x1A78, 0x1A79, 0x1A7A, 0x1A7B, 0x1A7C, 0x1B6B, 0x1B6D, 0x1B6E, 0x1B6F, 0x1B70, 0x1B71,
	0x1B72, 0x1B73, 0x1CD0, 0x1CD1, 0x1CD2, 0x1CDA, 0x1CDB, 0x1CE0, 0x1DC0, 0x1DC1, 0x1DC3, 0x1DC4,
	0x1DC5, 0x1DC6, 0x1DC7, 0x1DC8, 0x1DC9, 0x1DCB, 0x1DCC, 0x1DD1, 0x1DD2, 0x1DD3, 0x1DD4, 0x1DD5,
	0x1DD6, 0x1DD7, 0x1DD8, 0x1DD9, 0x1DDA, 0x1DDB, 0x1DDC, 0x1DDD, 0x1DDE, 0x1DDF, 0x1DE0, 0x1DE1,
	0x1DE2, 0x1DE3, 0x1DE4, 0x1DE5, 0x1DE6, 0x1DFE, 0x20D0, 0x20D1, 0x20D4, 0x20D5, 0x20D6, 0x20D7,
	0x20DB, 0x20DC, 0x20E1, 0x20E7, 0x20E9, 0x20F0, 0x2CEF, 0x2CF0, 0x2CF1, 0x2DE0, 0x2DE1, 0x2DE2,
	0x2DE3, 0x2DE4, 0x2DE5, 0x2DE6, 0x2DE7, 0x2DE8, 0x2DE9, 0x2DEA, 0x2DEB, 0x2DEC, 0x2DED, 0x2DEE,
	0x2DEF, 0x2DF0, 0x2DF1, 0x2DF2, 0x2DF3, 0x2DF4, 0x2DF5, 0x2DF6, 0x2DF7, 0x2DF8, 0x2DF9, 0x2DFA,
	0x2DFB, 0x2DFC, 0x2DFD, 0x2DFE, 0x2DFF, 0xA66F, 0xA67C, 0xA67D, 0xA6F0, 0xA6F1, 0xA8E0, 0xA8E1,
	0xA8E2, 0xA8E3, 0xA8E4, 0xA8E5, 0xA8E6, 0xA8E7, 0xA8E8, 0xA8E9, 0xA8EA, 0xA8EB, 0xA8EC, 0xA8ED,
	0xA8EE, 0xA8EF, 0xA8F0, 0xA8F1, 0xAAB0, 0xAAB2, 0xAAB3, 0xAAB7, 0xAAB8, 0xAABE, 0xAABF, 0xAAC1,
	0xFE20, 0xFE21, 0xFE22, 0xFE23, 0xFE24, 0xFE25, 0xFE26, 0x10A0F, 0x10A38, 0x1D185, 0x1D186, 0x1D187,
	0x1D188, 0x1D189, 0x1D1AA, 0x1D1AB, 0x1D1AC, 0x1D1AD, 0x1D242, 0x1D243, 0x1D244,
}

var kittyCopyBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 32*1024)
		return &buf
	},
}

// SupportsKittyPlaceholders reports whether the current terminal is known to
// support Kitty graphics rendered through Unicode placeholders. This is the
// only Kitty display mode that behaves like ordinary text cells and therefore
// survives the Bubble Tea renderer's line-based repainting.
//
// The detection is deliberately conservative: forcing it for an unsupported
// terminal would leave private-use placeholder glyphs on screen.
func SupportsKittyPlaceholders() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(kittyProtocolEnv))) {
	case "kitty", "kitty-placeholder", "kitty-placeholders":
		return true
	case "off", "none", "halfblock", "half-block":
		return false
	}

	termProgram := strings.ToLower(strings.TrimSpace(os.Getenv("TERM_PROGRAM")))
	// IDEs and other embedded terminals can inherit KITTY_WINDOW_ID from the
	// terminal that launched them, yet do not implement U+10EEEE placeholders.
	switch {
	case os.Getenv("WEZTERM_PANE") != "",
		strings.Contains(termProgram, "wezterm"),
		strings.Contains(termProgram, "vscode"),
		strings.Contains(termProgram, "cursor"),
		strings.Contains(termProgram, "warp"):
		return false
	}
	// Multiplexers need explicit Kitty graphics passthrough before placeholder
	// cells can resolve to images; otherwise they render as PUA glyphs.
	if os.Getenv("TMUX") != "" || os.Getenv("STY") != "" || os.Getenv("ZELLIJ") != "" {
		return false
	}

	term := strings.ToLower(os.Getenv("TERM"))
	if strings.Contains(term, "kitty") || strings.Contains(term, "ghostty") || termProgram == "ghostty" {
		return true
	}

	// Plain KITTY_WINDOW_ID can be inherited by unrelated terminal emulators.
	// Trust it only when TERM_PROGRAM does not identify a different terminal.
	if os.Getenv("KITTY_WINDOW_ID") != "" || os.Getenv("KITTY_PID") != "" {
		return termProgram == "" || termProgram == "kitty"
	}
	return false
}

// NewKittyImageID returns a random image id that can be encoded in the
// foreground color of a Unicode placeholder.
func NewKittyImageID() uint32 {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0x4C4E52 // "LNR"
	}
	id := uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
	if id == 0 {
		return 1
	}
	return id
}

// KittyPlaceholderGrid renders rows of Unicode placeholder cells for imageID.
// Each returned string contains the SGR foreground color that encodes the image
// id followed by exactly cols placeholder cells, so its visual width is cols.
// The caller should position the grid at the virtual placement created by
// TransmitKittyImage.
func KittyPlaceholderGrid(imageID uint32, cols, rows int) []string {
	if imageID == 0 || imageID > kittyMaxImageID || cols <= 0 || rows <= 0 {
		return nil
	}
	if cols > len(kittyRowColumnDiacritics) {
		cols = len(kittyRowColumnDiacritics)
	}
	if rows > len(kittyRowColumnDiacritics) {
		rows = len(kittyRowColumnDiacritics)
	}

	prefix := fmt.Sprintf("\x1b[38;2;%d;%d;%dm",
		(imageID>>16)&0xFF, (imageID>>8)&0xFF, imageID&0xFF)
	lines := make([]string, rows)
	for row := 0; row < rows; row++ {
		var sb strings.Builder
		sb.Grow(len(prefix) + cols*10 + 4)
		sb.WriteString(prefix)
		rowMark := kittyRowColumnDiacritics[row]
		for col := 0; col < cols; col++ {
			sb.WriteRune(kittyPlaceholderRune)
			sb.WriteRune(rowMark)
			sb.WriteRune(kittyRowColumnDiacritics[col])
		}
		sb.WriteString("\x1b[0m")
		lines[row] = sb.String()
	}
	return lines
}

// TransmitKittyImage streams an image file to the terminal as a Kitty virtual
// placement covering cols x rows. For PNG files the original bytes are streamed
// straight through; other formats supported by Go's image package are decoded
// and re-encoded as PNG because f=100 accepts PNG payloads.
//
// The sender never buffers the whole base64 payload: raw bytes are read in
// 3072-byte blocks, encoded into a stack-allocated 4096-byte chunk and written
// immediately. Callers must hold the terminal writer lock for the duration of
// this call (see LockedWriter.WithLock) so renderer frames cannot interleave
// with the escape sequence.
func TransmitKittyImage(w io.Writer, filePath string, imageID uint32, cols, rows int) error {
	if w == nil {
		return errors.New("Kitty 图像输出流为空")
	}
	if imageID == 0 || imageID > kittyMaxImageID {
		return fmt.Errorf("无效的 Kitty 图像 ID: %d", imageID)
	}
	if cols <= 0 || rows <= 0 {
		return fmt.Errorf("无效的 Kitty 图像尺寸: %dx%d", cols, rows)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, format, err := stdimage.DecodeConfig(f)
	if err != nil {
		return fmt.Errorf("解析图片格式失败: %w", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("读取图片失败: %w", err)
	}

	bw := bufio.NewWriterSize(w, 64*1024)
	chunks := &kittyChunkWriter{
		w:      bw,
		header: fmt.Sprintf("a=T,f=100,i=%d,p=1,U=1,C=1,c=%d,r=%d,q=2,", imageID, cols, rows),
	}

	if format == "png" {
		if err := copyStream(chunks, f); err != nil {
			return err
		}
	} else {
		img, _, err := stdimage.Decode(f)
		if err != nil {
			return fmt.Errorf("解码图片失败: %w", err)
		}
		if err := png.Encode(chunks, img); err != nil {
			return fmt.Errorf("编码 PNG 失败: %w", err)
		}
	}

	if err := chunks.Close(); err != nil {
		return err
	}
	return bw.Flush()
}

// PlaceKittyImage re-creates the virtual placement for an already transmitted
// image. It is used after a terminal resize to make an existing image follow
// the new modal geometry without retransmitting the pixel data.
func PlaceKittyImage(w io.Writer, imageID uint32, cols, rows int) error {
	if w == nil {
		return errors.New("Kitty 图像输出流为空")
	}
	if imageID == 0 || imageID > kittyMaxImageID {
		return fmt.Errorf("无效的 Kitty 图像 ID: %d", imageID)
	}
	if cols <= 0 || rows <= 0 {
		return fmt.Errorf("无效的 Kitty 图像尺寸: %dx%d", cols, rows)
	}
	_, err := fmt.Fprintf(w, "\x1b_Ga=p,U=1,q=2,i=%d,p=1,C=1,c=%d,r=%d\x1b\\", imageID, cols, rows)
	return err
}

// DeleteKittyImage removes an image and its virtual placement, freeing the
// terminal-side pixel data.
func DeleteKittyImage(w io.Writer, imageID uint32) error {
	if w == nil || imageID == 0 || imageID > kittyMaxImageID {
		return nil
	}
	_, err := fmt.Fprintf(w, "\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", imageID)
	return err
}

func copyStream(dst io.Writer, src io.Reader) error {
	bufp := kittyCopyBufferPool.Get().(*[]byte)
	defer kittyCopyBufferPool.Put(bufp)
	_, err := io.CopyBuffer(dst, src, *bufp)
	return err
}

type kittyChunkWriter struct {
	w      io.Writer
	header string
	buf    [kittyRawChunkSize]byte
	n      int
	wrote  bool
}

func (kw *kittyChunkWriter) Write(p []byte) (int, error) {
	total := len(p)
	for len(p) > 0 {
		n := copy(kw.buf[kw.n:], p)
		kw.n += n
		p = p[n:]
		if kw.n == len(kw.buf) {
			if err := kw.emit(false); err != nil {
				return total - len(p), err
			}
		}
	}
	return total, nil
}

func (kw *kittyChunkWriter) Close() error {
	if kw.n > 0 {
		return kw.emit(true)
	}
	if kw.wrote {
		// The payload ended exactly on a chunk boundary. The previous chunk
		// advertised m=1, so an empty terminating chunk is required.
		return writeAll(kw.w, "\x1b_Gm=0;\x1b\\")
	}
	return errors.New("图片数据为空")
}

func (kw *kittyChunkWriter) emit(last bool) error {
	var encoded [kittyChunkSize]byte
	n := base64.StdEncoding.EncodedLen(kw.n)
	base64.StdEncoding.Encode(encoded[:n], kw.buf[:kw.n])
	kw.n = 0

	more := "m=1;"
	if last {
		more = "m=0;"
	}

	var err error
	if !kw.wrote {
		err = writeAll(kw.w, "\x1b_G"+kw.header+more)
		kw.wrote = true
	} else {
		err = writeAll(kw.w, "\x1b_G"+more)
	}
	if err != nil {
		return err
	}
	if err := writeBytes(kw.w, encoded[:n]); err != nil {
		return err
	}
	return writeAll(kw.w, "\x1b\\")
}

func writeAll(w io.Writer, s string) error {
	n, err := io.WriteString(w, s)
	if err != nil {
		return err
	}
	if n != len(s) {
		return io.ErrShortWrite
	}
	return nil
}

func writeBytes(w io.Writer, p []byte) error {
	n, err := w.Write(p)
	if err != nil {
		return err
	}
	if n != len(p) {
		return io.ErrShortWrite
	}
	return nil
}
