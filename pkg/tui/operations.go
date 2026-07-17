package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"lnr-core/pkg/downloader"
	"lnr-core/pkg/epub"
	"lnr-core/pkg/model"
	"lnr-core/pkg/source"
	"lnr-core/pkg/storage"
	"lnr-core/pkg/tui/common"
)

type progressUpdateMsg struct {
	text string
	sub  <-chan string
}

func listenProgress(sub <-chan string) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-sub
		if !ok {
			return nil
		}
		return progressUpdateMsg{text: msg, sub: sub}
	}
}

func startDownloadTask(src source.DataSource, store *storage.Storage, bookID string, volumeIndex int, onComplete func()) tea.Cmd {
	ch := make(chan string, 16)

	go func() {
		defer close(ch)
		ctx := context.Background()

		ch <- "[下载] 正在准备下载任务..."

		// 1. Ensure detail is cached
		detail, err := store.LoadBookDetail(bookID)
		if err != nil || detail == nil {
			ch <- fmt.Sprintf("[下载] 正在联网获取书籍信息 (ID: %s)...", bookID)
			ctxTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
			detail, err = src.GetBookDetail(ctxTimeout, bookID)
			cancel()
			if err != nil {
				ch <- fmt.Sprintf("[错误] 获取书籍详情失败: %v", err)
				return
			}
			_ = store.SaveBookDetail(detail)
		}

		// 2. Ensure catalog is cached
		catalog, err := store.LoadCatalog(bookID)
		if err != nil || catalog == nil {
			ch <- fmt.Sprintf("[下载] 正在获取《%s》完整分卷目录...", detail.Title)
			ctxTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
			catalog, err = src.GetCatalog(ctxTimeout, bookID)
			cancel()
			if err != nil {
				ch <- fmt.Sprintf("[错误] 获取分卷目录失败: %v", err)
				return
			}
			_ = store.SaveCatalog(catalog)
		}

		dl, err := downloader.NewDownloader(src, store)
		if err != nil {
			ch <- fmt.Sprintf("[错误] 初始化下载器失败: %v", err)
			return
		}

		// Download cover
		if detail.CoverURL != "" {
			_, _ = dl.DownloadCover(ctx, bookID, detail.CoverURL)
		}

		var volumesToDownload []struct {
			idx int
			vol *model.Volume
		}

		if volumeIndex > 0 {
			if volumeIndex > len(catalog.Volumes) {
				ch <- fmt.Sprintf("[错误] 分卷序号 %d 超出有效范围 (1..%d)", volumeIndex, len(catalog.Volumes))
				return
			}
			volumesToDownload = append(volumesToDownload, struct {
				idx int
				vol *model.Volume
			}{idx: volumeIndex, vol: &catalog.Volumes[volumeIndex-1]})
		} else {
			for i := range catalog.Volumes {
				volumesToDownload = append(volumesToDownload, struct {
					idx int
					vol *model.Volume
				}{idx: i + 1, vol: &catalog.Volumes[i]})
			}
		}

		totalVols := len(volumesToDownload)
		for currentVolIdx, item := range volumesToDownload {
			ch <- fmt.Sprintf("[下载中] 《%s》[%d/%d 卷: %s] 准备开始...", detail.Title, currentVolIdx+1, totalVols, item.vol.Title)
			err := dl.DownloadVolume(ctx, bookID, item.vol, func(ev downloader.ProgressEvent) {
				msg := fmt.Sprintf("[下载中] 《%s》[%d/%d 卷] (%.1f%%) %s", detail.Title, currentVolIdx+1, totalVols, ev.Percentage, ev.ItemTitle)
				select {
				case ch <- msg:
				default:
				}
			})
			if err != nil {
				ch <- fmt.Sprintf("[错误] 下载分卷 %s 失败: %v", item.vol.Title, err)
				return
			}
		}

		if volumeIndex > 0 {
			ch <- fmt.Sprintf("[完成] 《%s》第 %d 卷下载完成！", detail.Title, volumeIndex)
		} else {
			ch <- fmt.Sprintf("[完成] 《%s》全书下载完成，已保存至本地书架！", detail.Title)
		}

		if onComplete != nil {
			onComplete()
		}
	}()

	return listenProgress(ch)
}

func startExportWithOptionsTask(src source.DataSource, store *storage.Storage, bookID string, opt epub.ExportOption, customExportDir string, onComplete func()) tea.Cmd {
	ch := make(chan string, 16)

	go func() {
		defer close(ch)
		if onComplete != nil {
			defer onComplete()
		}
		ctx := context.Background()

		ch <- "[导出] 正在准备 EPUB 导出任务..."

		// 1. Ensure detail is cached
		detail, err := store.LoadBookDetail(bookID)
		if err != nil || detail == nil {
			if src == nil {
				ch <- fmt.Sprintf("[错误] 未找到书籍 ID %s 的缓存信息", bookID)
				return
			}
			ctxTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
			detail, err = src.GetBookDetail(ctxTimeout, bookID)
			cancel()
			if err != nil {
				ch <- fmt.Sprintf("[错误] 获取书籍详情失败: %v", err)
				return
			}
			_ = store.SaveBookDetail(detail)
		}

		// 2. Ensure catalog is cached
		catalog, err := store.LoadCatalog(bookID)
		if err != nil || catalog == nil {
			if src == nil {
				ch <- fmt.Sprintf("[错误] 未找到书籍 ID %s 的目录缓存", bookID)
				return
			}
			ctxTimeout, cancel := context.WithTimeout(ctx, 15*time.Second)
			catalog, err = src.GetCatalog(ctxTimeout, bookID)
			cancel()
			if err != nil {
				ch <- fmt.Sprintf("[错误] 获取分卷目录失败: %v", err)
				return
			}
			_ = store.SaveCatalog(catalog)
		}

		exportDir := customExportDir
		if exportDir == "" {
			home, _ := os.UserHomeDir()
			exportDir = filepath.Join(home, ".lnr", "exports")
		}
		_ = os.MkdirAll(exportDir, 0755)

		dl, err := downloader.NewDownloader(src, store)
		if err != nil {
			ch <- fmt.Sprintf("[错误] 初始化下载器失败: %v", err)
			return
		}

		var volumesToCheck []struct {
			idx int
			vol *model.Volume
		}
		if len(opt.VolumeIndexes) > 0 {
			for _, idx := range opt.VolumeIndexes {
				if idx >= 1 && idx <= len(catalog.Volumes) {
					volumesToCheck = append(volumesToCheck, struct {
						idx int
						vol *model.Volume
					}{idx: idx, vol: &catalog.Volumes[idx-1]})
				}
			}
		} else {
			for i := range catalog.Volumes {
				volumesToCheck = append(volumesToCheck, struct {
					idx int
					vol *model.Volume
				}{idx: i + 1, vol: &catalog.Volumes[i]})
			}
		}

		// Ensure all needed chapters are cached
		for _, item := range volumesToCheck {
			missing := false
			for _, chInfo := range item.vol.Chapters {
				if _, err := store.LoadChapter(bookID, chInfo.ID); err != nil {
					missing = true
					break
				}
			}
			if missing {
				ch <- fmt.Sprintf("[导出] 正在自动缓存缺少的分卷章节: %s...", item.vol.Title)
				if err := dl.DownloadVolume(ctx, bookID, item.vol, nil); err != nil {
					ch <- fmt.Sprintf("[错误] 缓存分卷 %s 失败: %v", item.vol.Title, err)
					return
				}
			}
		}

		exporter := epub.NewExporter(store, dl)

		modeDesc := "标准版(含插图)"
		if opt.NoImages {
			modeDesc = "纯文本轻量版(无图)"
		}

		ch <- fmt.Sprintf("[导出中] 正在打包 EPUB [%s]...", modeDesc)
		outFiles, err := exporter.ExportWithOptions(ctx, bookID, opt, exportDir, func(current, total int, volTitle, outFile string) {
			msg := fmt.Sprintf("[导出中] 《%s》[%d/%d] 成功打包: %s", detail.Title, current, total, filepath.Base(outFile))
			select {
			case ch <- msg:
			default:
			}
		})
		if err != nil {
			ch <- fmt.Sprintf("[错误] 导出 EPUB 失败: %v", err)
			return
		}

		if opt.SplitVolumes {
			ch <- fmt.Sprintf("[完成] 成功导出全部分卷 EPUB (共 %d 卷) 到目录: %s", len(outFiles), exportDir)
		} else if len(outFiles) == 1 {
			ch <- fmt.Sprintf("[完成] 成功导出 EPUB: %s", outFiles[0])
		} else {
			ch <- fmt.Sprintf("[完成] 成功导出全部分卷 EPUB (共 %d 卷) 到目录: %s", len(outFiles), exportDir)
		}
	}()

	return listenProgress(ch)
}

func startExportTask(src source.DataSource, store *storage.Storage, bookID string, volumeIndex int, customExportDir string) tea.Cmd {
	var opt epub.ExportOption
	if volumeIndex == common.ExportModeAllVolumesSeparate {
		opt = epub.ExportOption{SplitVolumes: true}
	} else if volumeIndex > 0 {
		opt = epub.ExportOption{VolumeIndexes: []int{volumeIndex}}
	} else {
		opt = epub.ExportOption{SplitVolumes: false}
	}
	return startExportWithOptionsTask(src, store, bookID, opt, customExportDir, nil)
}

func sanitizeFileName(name string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		" ", "_",
	)
	return replacer.Replace(strings.TrimSpace(name))
}
