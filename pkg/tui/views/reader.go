package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"lnr-core/pkg/reader"
	"lnr-core/pkg/source"
	"lnr-core/pkg/storage"
	"lnr-core/pkg/tui/common"
	"lnr-core/pkg/tui/theme"
)

type chapterContentLoadedMsg struct {
	err error
}

// ReaderView renders chapter content using a scrollable viewport.
type ReaderView struct {
	store     *storage.Storage
	src       source.DataSource
	reader    *reader.Reader
	bookID    string
	chapterID string
	viewport  viewport.Model
	width     int
	height    int
	ready     bool
	loading   bool
	err       error
}

// NewReaderView constructs an immersive terminal reader view.
func NewReaderView(store *storage.Storage, src source.DataSource) *ReaderView {
	return &ReaderView{
		store: store,
		src:   src,
	}
}

// OpenChapter loads and displays a chapter in viewport.
func (v *ReaderView) OpenChapter(bookID, chapterID string) tea.Cmd {
	v.bookID = bookID
	v.chapterID = chapterID
	v.loading = true
	v.err = nil

	return func() tea.Msg {
		r, err := reader.NewReader(v.store, bookID)
		if err != nil {
			return chapterContentLoadedMsg{err: err}
		}
		v.reader = r

		// Ensure chapter cached
		_, err = v.store.LoadChapter(bookID, chapterID)
		if err != nil {
			// Fetch on-demand
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			ch, err := v.src.GetChapterContent(ctx, bookID, chapterID)
			if err != nil {
				return chapterContentLoadedMsg{err: err}
			}
			_ = v.store.SaveChapter(ch)
		}

		_, err = v.reader.LoadChapter(chapterID)
		return chapterContentLoadedMsg{err: err}
	}
}

func (v *ReaderView) Init() tea.Cmd {
	return nil
}

func (v *ReaderView) SetSize(width, height int) {
	v.width = width
	v.height = height
	if !v.ready {
		v.viewport = viewport.New(width, height-2)
		v.ready = true
	} else {
		v.viewport.Width = width
		v.viewport.Height = height - 2
	}
}

func (v *ReaderView) Update(msg tea.Msg) (*ReaderView, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case chapterContentLoadedMsg:
		v.loading = false
		v.err = msg.err
		if v.err == nil && v.reader != nil {
			lines := v.reader.Lines()
			content := strings.Join(lines, "\n\n")
			v.viewport.SetContent(content)
			v.viewport.GotoTop()

			// Check if bookmark exists
			p, err := v.reader.LoadProgress()
			if err == nil && p != nil && p.ChapterID == v.chapterID {
				// Scroll to saved position
				v.viewport.SetYOffset(p.LineIndex)
			}
		}
		return v, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// Save progress on exit
			if v.reader != nil {
				_ = v.reader.SaveProgress(&reader.ReadingProgress{
					BookID:     v.bookID,
					ChapterID:  v.chapterID,
					LineIndex:  v.viewport.YOffset,
					LastReadAt: time.Now().Unix(),
				})
			}
			return v, func() tea.Msg {
				return common.SwitchViewMsg{
					Target: common.ViewCatalog,
					BookID: v.bookID,
				}
			}
		}
	}

	var vpCmd tea.Cmd
	v.viewport, vpCmd = v.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return v, tea.Batch(cmds...)
}

func (v *ReaderView) View() string {
	if v.loading {
		return "\n⏳ 正在获取正文内容，请稍候..."
	}
	if v.err != nil {
		return fmt.Sprintf("\n❌ 加载正文出错: %v\n按 [Esc] 返回目录", v.err)
	}

	title := ""
	if v.reader != nil && v.reader.CurrentChapter() != nil {
		title = v.reader.CurrentChapter().Title
	}

	header := lipgloss.NewStyle().Bold(true).Foreground(theme.AccentColor).
		Render(fmt.Sprintf("📖 %s  (进度: %.1f%%) - [j/k/空格] 翻页, [Esc] 退出", title, v.viewport.ScrollPercent()*100))

	return header + "\n\n" + v.viewport.View()
}
