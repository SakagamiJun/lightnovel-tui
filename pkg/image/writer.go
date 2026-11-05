package image

import (
	"errors"
	"io"
	"sync"
)

// LockedWriter serializes all terminal output between Bubble Tea's renderer and
// one-shot graphics commands. Without it a large image transmission could be
// interleaved with a repaint and corrupt both the escape sequence and the UI.
//
// It also exposes Fd/Read/Close so Bubble Tea still recognizes os.Stdout as a
// terminal when the writer is passed via tea.WithOutput; otherwise the program
// would stop receiving window resize messages.
type LockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// NewLockedWriter wraps w with a mutex. A nil writer yields nil so callers can
// disable inline graphics cleanly.
func NewLockedWriter(w io.Writer) *LockedWriter {
	if w == nil {
		return nil
	}
	return &LockedWriter{w: w}
}

// Write implements io.Writer with mutual exclusion.
func (l *LockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// WithLock runs fn while holding the output lock. Long streaming operations
// such as a Kitty image upload should use this so no renderer frame can
// interleave with the sequence.
func (l *LockedWriter) WithLock(fn func(io.Writer) error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return fn(l.w)
}

// Read forwards to the underlying writer when it supports reading. It exists
// only to satisfy terminal file interfaces used by Bubble Tea.
func (l *LockedWriter) Read(p []byte) (int, error) {
	if r, ok := l.w.(io.Reader); ok {
		return r.Read(p)
	}
	return 0, errors.New("输出流不可读取")
}

// Close forwards to the underlying writer when it supports closing.
func (l *LockedWriter) Close() error {
	if c, ok := l.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// Fd returns the underlying file descriptor when available. Bubble Tea uses it
// to query terminal dimensions and to restore terminal state on exit.
func (l *LockedWriter) Fd() uintptr {
	if f, ok := l.w.(interface{ Fd() uintptr }); ok {
		return f.Fd()
	}
	return ^uintptr(0)
}
