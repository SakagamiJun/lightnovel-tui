package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/SakagamiJun/lightnovel-tui/pkg/source/wenku8"
	"github.com/SakagamiJun/lightnovel-tui/pkg/storage"
	"github.com/SakagamiJun/lightnovel-tui/pkg/tui"
	"github.com/SakagamiJun/lightnovel-tui/pkg/version"
)

func init() {
	runewidth.DefaultCondition.EastAsianWidth = true
}

func main() {
	var (
		cacheDir    string
		showVersion bool
	)
	flag.StringVar(&cacheDir, "cache-dir", "", "本地缓存目录 (默认为 ~/.lnr/cache)")
	flag.BoolVar(&showVersion, "v", false, "显示版本信息")
	flag.BoolVar(&showVersion, "version", false, "显示版本信息")
	flag.Parse()

	if showVersion {
		fmt.Printf("LightNovelReader TUI (lnr-tui)\n")
		fmt.Printf("版本:     %s\n", version.GetVersion())
		fmt.Printf("构建信息: %s\n", version.GetBuildInfo())
		return
	}

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
