package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"lnr-core/pkg/source/wenku8"
	"lnr-core/pkg/storage"
	"lnr-core/pkg/tui"
)

func init() {
	runewidth.DefaultCondition.EastAsianWidth = true
}

func main() {
	var cacheDir string
	flag.StringVar(&cacheDir, "cache-dir", "", "本地缓存目录 (默认为 ~/.lnr/cache)")
	flag.Parse()

	store, err := storage.NewStorage(cacheDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化缓存目录失败: %v\n", err)
		os.Exit(1)
	}

	src, err := wenku8.NewWenku8Source()
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化数据源失败: %v\n", err)
		os.Exit(1)
	}

	appModel := tui.NewAppModel(store, src)
	p := tea.NewProgram(appModel, tea.WithAltScreen(), tea.WithMouseCellMotion())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "运行 TUI 失败: %v\n", err)
		os.Exit(1)
	}
}
