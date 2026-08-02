package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"

	"lnr-core/pkg/downloader"
	"lnr-core/pkg/epub"
	termimage "lnr-core/pkg/image"
	"lnr-core/pkg/model"
	"lnr-core/pkg/source"
	"lnr-core/pkg/source/wenku8"
	"lnr-core/pkg/storage"
	"lnr-core/pkg/text"
)

func init() {
	runewidth.DefaultCondition.EastAsianWidth = true
}

var (
	cacheDir string
	store    *storage.Storage
	src      source.DataSource
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "lnr",
		Short: "LightNovelReader CLI - 轻小说阅读与下载工具",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var err error
			store, err = storage.NewStorage(cacheDir)
			if err != nil {
				return fmt.Errorf("failed to init storage: %w", err)
			}
			src, err = wenku8.NewWenku8Source()
			if err != nil {
				return fmt.Errorf("failed to init data source: %w", err)
			}
			return nil
		},
	}

	rootCmd.PersistentFlags().StringVar(&cacheDir, "cache-dir", "", "本地缓存目录 (默认为 ~/.lnr/cache)")

	// 1. search command
	searchCmd := &cobra.Command{
		Use:   "search <关键词>",
		Short: "按书名或作者搜索轻小说",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			byAuthor, _ := cmd.Flags().GetBool("author")
			page, _ := cmd.Flags().GetInt("page")
			searchType := source.SearchTypeTitle
			if byAuthor {
				searchType = source.SearchTypeAuthor
			}

			keyword := args[0]
			fmt.Printf("[搜索] 正在检索: %q (第 %d 页)...\n", keyword, page)

			results, totalPages, err := src.Search(context.Background(), searchType, keyword, page)
			if err != nil {
				return fmt.Errorf("搜索失败: %w", err)
			}

			if len(results) == 0 {
				fmt.Println("未找到匹配的小说。")
				return nil
			}

			fmt.Printf("\n共检索到 %d 本小说 (当前第 %d/%d 页):\n", len(results), page, totalPages)
			fmt.Println(strings.Repeat("-", 80))
			for i, book := range results {
				completeStatus := "连载中"
				if book.IsComplete {
					completeStatus = "已完结"
				}
				fmt.Printf("[%2d] %-30s | 作者: %-15s | 文库: %-10s | %-6s | %d 字 | ID: %s\n",
					(page-1)*20+i+1, book.Title, book.Author, book.Publisher, completeStatus, book.WordCount, book.ID)
			}
			fmt.Println(strings.Repeat("-", 80))
			return nil
		},
	}
	searchCmd.Flags().BoolP("author", "a", false, "按作者名搜索 (默认为按书名)")
	searchCmd.Flags().IntP("page", "p", 1, "搜索结果分页页码")

	// 2. info command
	infoCmd := &cobra.Command{
		Use:   "info <书籍ID>",
		Short: "查看小说详情与分卷目录",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bookID := args[0]
			ctx := context.Background()

			fmt.Printf("[详情] 正在获取书籍详情 (ID: %s)...\n", bookID)
			detail, err := src.GetBookDetail(ctx, bookID)
			if err != nil {
				return fmt.Errorf("获取详情失败: %w", err)
			}
			_ = store.SaveBookDetail(detail)

			catalog, err := src.GetCatalog(ctx, bookID)
			if err != nil {
				return fmt.Errorf("获取目录失败: %w", err)
			}
			_ = store.SaveCatalog(catalog)

			fmt.Println("\n" + strings.Repeat("=", 80))
			fmt.Printf("书名: %s\n", detail.Title)
			if detail.Subtitle != "" {
				fmt.Printf("副标题: %s\n", detail.Subtitle)
			}
			fmt.Printf("作者: %s | 文库: %s | 更新时间: %s | 全文: %d 字\n",
				detail.Author, detail.Publisher, detail.LastUpdated, detail.WordCount)
			if len(detail.Tags) > 0 {
				fmt.Printf("标签: %s\n", strings.Join(detail.Tags, " / "))
			}
			fmt.Printf("简介: %s\n", detail.Description)
			fmt.Println(strings.Repeat("=", 80))

			fmt.Printf("\n【分卷目录】(共 %d 卷):\n", len(catalog.Volumes))
			for vi, vol := range catalog.Volumes {
				fmt.Printf("卷 [%d]: %s (共 %d 章)\n", vi+1, vol.Title, len(vol.Chapters))
				for ci, ch := range vol.Chapters {
					if ci < 4 || ci >= len(vol.Chapters)-2 {
						fmt.Printf("    ├── [%s] %s\n", ch.ID, ch.Title)
					} else if ci == 4 {
						fmt.Printf("    ├── ... (%d 章节省略) ...\n", len(vol.Chapters)-6)
					}
				}
			}
			fmt.Println()
			return nil
		},
	}

	// 3. download command
	downloadCmd := &cobra.Command{
		Use:   "download <书籍ID>",
		Short: "下载小说内容与插图至本地缓存",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bookID := args[0]
			targetVol, _ := cmd.Flags().GetInt("volume")
			ctx := context.Background()

			// Ensure catalog exists
			catalog, err := store.LoadCatalog(bookID)
			if err != nil {
				fmt.Println("正在获取书籍目录...")
				detail, err := src.GetBookDetail(ctx, bookID)
				if err != nil {
					return err
				}
				_ = store.SaveBookDetail(detail)

				catalog, err = src.GetCatalog(ctx, bookID)
				if err != nil {
					return err
				}
				_ = store.SaveCatalog(catalog)
			}

			dl, err := downloader.NewDownloader(src, store)
			if err != nil {
				return err
			}

			detail, _ := store.LoadBookDetail(bookID)
			if detail != nil && detail.CoverURL != "" {
				fmt.Println("正在下载封面...")
				_, _ = dl.DownloadCover(ctx, bookID, detail.CoverURL)
			}

			var volumesToDownload []struct {
				idx int
				vol *model.Volume
			}

			if targetVol > 0 {
				if targetVol > len(catalog.Volumes) {
					return fmt.Errorf("卷号 %d 超出范围 (1..%d)", targetVol, len(catalog.Volumes))
				}
				volumesToDownload = append(volumesToDownload, struct {
					idx int
					vol *model.Volume
				}{idx: targetVol, vol: &catalog.Volumes[targetVol-1]})
			} else {
				for i := range catalog.Volumes {
					volumesToDownload = append(volumesToDownload, struct {
						idx int
						vol *model.Volume
					}{idx: i + 1, vol: &catalog.Volumes[i]})
				}
			}

			for _, item := range volumesToDownload {
				fmt.Printf("\n[下载] 开始下载第 [%d/%d] 卷: %s\n", item.idx, len(catalog.Volumes), item.vol.Title)
				err := dl.DownloadVolume(ctx, bookID, item.vol, func(ev downloader.ProgressEvent) {
					fmt.Printf("\r  [%d/%d] (%.1f%%) 正在处理: %-30s", ev.Current, ev.Total, ev.Percentage, ev.ItemTitle)
				})
				if err != nil {
					return fmt.Errorf("\n下载卷 %s 失败: %w", item.vol.Title, err)
				}
				fmt.Println("\n  [完成] 该卷下载完成！")
			}

			fmt.Println("\n[完成] 下载任务全部完成！缓存位于:", store.BookDir(bookID))
			return nil
		},
	}
	downloadCmd.Flags().IntP("volume", "v", 0, "指定下载特定卷 (默认0表示全部下载)")

	// 4. export command
	exportCmd := &cobra.Command{
		Use:   "export <书籍ID>",
		Short: "将已下载的小说导出为 EPUB 电子书",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bookID := args[0]
			volStr, _ := cmd.Flags().GetString("volume")
			noImages, _ := cmd.Flags().GetBool("no-images")
			split, _ := cmd.Flags().GetBool("split")
			outputPath, _ := cmd.Flags().GetString("output")
			traditional, _ := cmd.Flags().GetBool("traditional")
			ctx := context.Background()

			volIndexes, err := epub.ParseVolumeIndexes(volStr)
			if err != nil {
				return fmt.Errorf("分卷参数解析失败: %w", err)
			}

			dl, _ := downloader.NewDownloader(src, store)
			exporter := epub.NewExporter(store, dl)
			exporter.SetTraditional(traditional)

			opt := epub.ExportOption{
				VolumeIndexes: volIndexes,
				NoImages:      noImages,
				SplitVolumes:  split,
			}

			modeDesc := "标准版(含插图)"
			if noImages {
				modeDesc = "纯文本轻量版(无图)"
			}

			if split {
				fmt.Printf("[导出] 正在将书籍 %s 分别导出为独立 EPUB 文件 [%s]...\n", bookID, modeDesc)
				outFiles, err := exporter.ExportWithOptions(ctx, bookID, opt, outputPath, func(current, total int, volTitle, outFile string) {
					fmt.Printf("  [%d/%d] (%.1f%%) 已导出分卷: %s -> %s\n", current, total, float64(current)/float64(total)*100, volTitle, filepath.Base(outFile))
				})
				if err != nil {
					return fmt.Errorf("按分卷分别导出 EPUB 失败: %w", err)
				}
				fmt.Printf("[完成] 成功导出 %d 卷独立 EPUB 文件！\n", len(outFiles))
				return nil
			}

			if len(volIndexes) > 0 {
				var volDesc []string
				for _, v := range volIndexes {
					volDesc = append(volDesc, fmt.Sprintf("%d", v))
				}
				fmt.Printf("[导出] 正在导出书籍 %s 的分卷 [%s] 为 EPUB [%s]...\n", bookID, strings.Join(volDesc, ","), modeDesc)
			} else {
				fmt.Printf("[导出] 正在导出书籍 %s 的全本为 EPUB [%s]...\n", bookID, modeDesc)
			}

			outFiles, err := exporter.ExportWithOptions(ctx, bookID, opt, outputPath, nil)
			if err != nil {
				return fmt.Errorf("导出 EPUB 失败: %w", err)
			}
			for _, f := range outFiles {
				fmt.Printf("[完成] 成功导出 EPUB 文件: %s\n", f)
			}
			return nil
		},
	}
	exportCmd.Flags().StringP("volume", "v", "", "指定导出分卷 (支持单个分卷如 '1'，多个分卷如 '1,2,3' 或范围 '1-3'，默认导出全本)")
	exportCmd.Flags().Bool("no-images", false, "纯文本轻量导出，去除插图与封面，体积缩减 90%+")
	exportCmd.Flags().BoolP("split", "s", false, "将分卷分别导出为独立的 EPUB 文件 (每卷一个 EPUB)")
	exportCmd.Flags().StringP("output", "o", "", "指定导出 EPUB 文件路径或保存目录")
	exportCmd.Flags().BoolP("traditional", "t", false, "转换为繁体中文 (Traditional Chinese) 导出")

	// 5. cover command
	coverCmd := &cobra.Command{
		Use:   "cover <书籍ID>",
		Short: "在终端直接查看轻小说全彩封面",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bookID := args[0]
			openSys, _ := cmd.Flags().GetBool("open")
			maxW, _ := cmd.Flags().GetInt("width")
			maxH, _ := cmd.Flags().GetInt("height")
			protoStr, _ := cmd.Flags().GetString("protocol")
			ctx := context.Background()

			// Locate cover path
			coverPath := store.FindCoverPath(bookID)
			if coverPath == "" {
				// Load detail to get cover URL
				detail, err := store.LoadBookDetail(bookID)
				if err != nil {
					detail, err = src.GetBookDetail(ctx, bookID)
					if err != nil {
						return fmt.Errorf("获取小说详情失败: %w", err)
					}
					_ = store.SaveBookDetail(detail)
				}
				if detail.CoverURL == "" {
					return fmt.Errorf("书籍 %s 未包含封面图片", bookID)
				}

				_ = store.EnsureImageDir(bookID)
				ext := filepath.Ext(detail.CoverURL)
				if ext == "" || len(ext) > 5 {
					ext = ".jpg"
				}
				coverPath = filepath.Join(store.BookDir(bookID), "images", "cover"+ext)
				fmt.Printf("[下载] 正在拉取封面: %s...\n", detail.CoverURL)
				if err := termimage.DownloadImageToFile(ctx, detail.CoverURL, coverPath); err != nil {
					return fmt.Errorf("下载封面失败: %w", err)
				}
			}

			if openSys {
				fmt.Printf("[打开] 正在使用系统查看器打开封面: %s\n", coverPath)
				return termimage.OpenInSystemViewer(coverPath)
			}

			proto := termimage.DetectTerminalProtocol()
			switch strings.ToLower(protoStr) {
			case "halfblock", "ansi":
				proto = termimage.ProtocolHalfBlock
			case "iterm2":
				proto = termimage.ProtocolITerm2
			case "kitty":
				proto = termimage.ProtocolKitty
			}

			fmt.Printf("\n[封面] 书籍 %s 封面预览 (协议: %s):\n\n", bookID, proto.String())
			rendered, err := termimage.RenderFile(coverPath, maxW, maxH, proto)
			if err != nil {
				return fmt.Errorf("渲染封面失败: %w", err)
			}
			fmt.Print(rendered)
			fmt.Printf("\n本地文件: %s\n", coverPath)
			return nil
		},
	}
	coverCmd.Flags().BoolP("open", "o", false, "使用系统默认图片查看器打开")
	coverCmd.Flags().IntP("width", "w", 80, "渲染最大字符列宽")
	coverCmd.Flags().IntP("height", "H", 35, "渲染最大字符行高")
	coverCmd.Flags().String("protocol", "auto", "渲染协议 (auto|halfblock|iterm2|kitty)")

	// 6. image command
	imageCmd := &cobra.Command{
		Use:   "image <书籍ID>",
		Short: "在终端查看轻小说内嵌插图",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			bookID := args[0]
			listOnly, _ := cmd.Flags().GetBool("list")
			imgIdx, _ := cmd.Flags().GetInt("index")
			chapterID, _ := cmd.Flags().GetString("chapter")
			openSys, _ := cmd.Flags().GetBool("open")
			maxW, _ := cmd.Flags().GetInt("width")
			maxH, _ := cmd.Flags().GetInt("height")
			protoStr, _ := cmd.Flags().GetString("protocol")
			ctx := context.Background()

			// Load catalog
			catalog, err := store.LoadCatalog(bookID)
			if err != nil {
				catalog, err = src.GetCatalog(ctx, bookID)
				if err != nil {
					return fmt.Errorf("获取小说目录失败: %w", err)
				}
				_ = store.SaveCatalog(catalog)
			}

			// Collect all illustrations across chapters (or specific chapter)
			type illustrationInfo struct {
				ChapterID    string
				ChapterTitle string
				URL          string
				LocalPath    string
			}
			var allIllustrations []illustrationInfo

			for _, vol := range catalog.Volumes {
				for _, ch := range vol.Chapters {
					if chapterID != "" && ch.ID != chapterID {
						continue
					}
					content, err := store.LoadChapter(bookID, ch.ID)
					if err != nil {
						// Try fetch
						content, err = src.GetChapterContent(ctx, bookID, ch.ID)
						if err == nil {
							_ = store.SaveChapter(content)
						}
					}
					if content != nil {
						for _, el := range content.Elements {
							if el.Type == model.ContentTypeImage && el.URL != "" {
								allIllustrations = append(allIllustrations, illustrationInfo{
									ChapterID:    ch.ID,
									ChapterTitle: ch.Title,
									URL:          el.URL,
									LocalPath:    store.IllustrationPath(bookID, el.URL),
								})
							}
						}
					}
				}
			}

			if len(allIllustrations) == 0 {
				fmt.Printf("[提示] 书籍 %s 尚未发现插图或插图未缓存。\n", bookID)
				return nil
			}

			if listOnly {
				fmt.Printf("\n[插图列表] 共找到 %d 张插图:\n", len(allIllustrations))
				fmt.Println(strings.Repeat("-", 80))
				for i, illu := range allIllustrations {
					cached := "[未缓存]"
					if _, err := os.Stat(illu.LocalPath); err == nil {
						cached = "[已缓存]"
					}
					fmt.Printf("[%2d] %-15s | 章节: %-25s | %s\n", i+1, cached, illu.ChapterTitle, illu.URL)
				}
				fmt.Println(strings.Repeat("-", 80))
				fmt.Printf("使用 'lnr image %s -n <序号>' 在终端直接预览插图\n", bookID)
				return nil
			}

			if imgIdx < 1 || imgIdx > len(allIllustrations) {
				imgIdx = 1
			}
			targetIllu := allIllustrations[imgIdx-1]

			// Ensure downloaded
			if _, err := os.Stat(targetIllu.LocalPath); os.IsNotExist(err) {
				fmt.Printf("[下载] 正在拉取插图 [%d/%d]: %s...\n", imgIdx, len(allIllustrations), targetIllu.URL)
				if err := termimage.DownloadImageToFile(ctx, targetIllu.URL, targetIllu.LocalPath); err != nil {
					return fmt.Errorf("下载插图失败: %w", err)
				}
			}

			if openSys {
				fmt.Printf("[打开] 正在使用系统查看器打开插图: %s\n", targetIllu.LocalPath)
				return termimage.OpenInSystemViewer(targetIllu.LocalPath)
			}

			proto := termimage.DetectTerminalProtocol()
			switch strings.ToLower(protoStr) {
			case "halfblock", "ansi":
				proto = termimage.ProtocolHalfBlock
			case "iterm2":
				proto = termimage.ProtocolITerm2
			case "kitty":
				proto = termimage.ProtocolKitty
			}

			fmt.Printf("\n[插图] 第 %d/%d 张: %s (协议: %s)\n\n", imgIdx, len(allIllustrations), targetIllu.ChapterTitle, proto.String())
			rendered, err := termimage.RenderFile(targetIllu.LocalPath, maxW, maxH, proto)
			if err != nil {
				return fmt.Errorf("渲染插图失败: %w", err)
			}
			fmt.Print(rendered)
			fmt.Printf("\n本地文件: %s\n", targetIllu.LocalPath)
			return nil
		},
	}
	imageCmd.Flags().BoolP("list", "l", false, "列出所有章节插图清单")
	imageCmd.Flags().IntP("index", "n", 1, "指定查看的插图序号 (从 1 开始)")
	imageCmd.Flags().StringP("chapter", "c", "", "指定章节 ID")
	imageCmd.Flags().BoolP("open", "o", false, "使用系统默认图片查看器打开")
	imageCmd.Flags().IntP("width", "w", 80, "渲染最大字符列宽")
	imageCmd.Flags().IntP("height", "H", 35, "渲染最大字符行高")
	imageCmd.Flags().String("protocol", "auto", "渲染协议 (auto|halfblock|iterm2|kitty)")

	// 7. top command
	topCmd := &cobra.Command{
		Use:   "top [hot|anime|update|new|finish]",
		Short: "浏览轻小说排行榜与热门推荐",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			page, _ := cmd.Flags().GetInt("page")
			topTypeStr := "hot"
			if len(args) > 0 {
				topTypeStr = strings.ToLower(args[0])
			}

			tType := source.ToplistHot
			switch topTypeStr {
			case "anime":
				tType = source.ToplistAnime
			case "update", "lastupdate":
				tType = source.ToplistLastUpdate
			case "new", "postdate":
				tType = source.ToplistPostDate
			case "finish", "completed", "full":
				tType = source.ToplistCompleted
			default:
				tType = source.ToplistHot
			}

			title := source.ToplistNameMap[tType]
			fmt.Printf("[榜单] 正在获取「%s」(第 %d 页)...\n", title, page)

			results, totalPages, err := src.GetToplist(context.Background(), tType, page)
			if err != nil {
				return fmt.Errorf("获取榜单失败: %w", err)
			}

			if len(results) == 0 {
				fmt.Println("未获取到榜单数据。")
				return nil
			}

			fmt.Printf("\n「%s」共 %d 本小说 (当前第 %d/%d 页):\n", title, len(results), page, totalPages)
			fmt.Println(strings.Repeat("-", 80))
			for i, book := range results {
				status := "连载中"
				if book.IsComplete {
					status = "已完结"
				}
				fmt.Printf("[%2d] %-30s | 作者: %-15s | 文库: %-10s | %-6s | %d 字 | ID: %s\n",
					(page-1)*20+i+1, book.Title, book.Author, book.Publisher, status, book.WordCount, book.ID)
			}
			fmt.Println(strings.Repeat("-", 80))
			fmt.Println("使用 'lnr info <ID>' 查看详情，使用 'lnr download <ID>' 下载")
			return nil
		},
	}
	topCmd.Flags().IntP("page", "p", 1, "榜单分页页码")

	// 8. tags command
	tagsCmd := &cobra.Command{
		Use:   "tags",
		Short: "查看文库轻小说分类标签库",
		RunE: func(cmd *cobra.Command, args []string) error {
			tags := src.GetTags()
			fmt.Printf("\n[标签库] 文库分类标签 (共 %d 个):\n", len(tags))
			fmt.Println(strings.Repeat("-", 70))
			for i, t := range tags {
				fmt.Printf("%-12s", fmt.Sprintf("[%s]", t))
				if (i+1)%5 == 0 {
					fmt.Println()
				}
			}
			if len(tags)%5 != 0 {
				fmt.Println()
			}
			fmt.Println(strings.Repeat("-", 70))
			fmt.Println("使用 'lnr tag <标签名>' 浏览该标签分类小说")
			return nil
		},
	}

	// 9. tag command
	tagCmd := &cobra.Command{
		Use:   "tag <标签名>",
		Short: "按分类标签浏览轻小说",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tag := args[0]
			page, _ := cmd.Flags().GetInt("page")
			fmt.Printf("[标签] 正在浏览标签「%s」(第 %d 页)...\n", tag, page)

			results, totalPages, err := src.GetTagBooks(context.Background(), tag, page)
			if err != nil {
				return fmt.Errorf("获取分类小说失败: %w", err)
			}

			if len(results) == 0 {
				fmt.Printf("未找到标签「%s」下的小说。\n", tag)
				return nil
			}

			fmt.Printf("\n标签「%s」共检索到 %d 本小说 (当前第 %d/%d 页):\n", tag, len(results), page, totalPages)
			fmt.Println(strings.Repeat("-", 80))
			for i, book := range results {
				status := "连载中"
				if book.IsComplete {
					status = "已完结"
				}
				fmt.Printf("[%2d] %-30s | 作者: %-15s | 文库: %-10s | %-6s | %d 字 | ID: %s\n",
					(page-1)*20+i+1, book.Title, book.Author, book.Publisher, status, book.WordCount, book.ID)
			}
			fmt.Println(strings.Repeat("-", 80))
			fmt.Println("使用 'lnr info <ID>' 查看详情，使用 'lnr download <ID>' 下载")
			return nil
		},
	}
	tagCmd.Flags().IntP("page", "p", 1, "分类分页页码")

	// 10. update command
	updateCmd := &cobra.Command{
		Use:   "update [书籍ID]",
		Short: "检查本地书架更新或同步增量章节",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			checkOnly, _ := cmd.Flags().GetBool("check")
			updateAll, _ := cmd.Flags().GetBool("all")
			ctx := context.Background()

			cachedBooks, err := store.ListCachedBooks()
			if err != nil {
				return fmt.Errorf("读取本地书架失败: %w", err)
			}

			if len(cachedBooks) == 0 {
				fmt.Println("[提示] 本地书架暂无缓存小说。")
				return nil
			}

			// If specific book ID given
			if len(args) > 0 {
				targetID := args[0]
				fmt.Printf("[更新] 正在检查书籍 %s 的更新...\n", targetID)
				info, err := store.CheckBookUpdate(ctx, src, targetID)
				if err != nil {
					return fmt.Errorf("检查更新失败: %w", err)
				}
				if !info.HasUpdate {
					fmt.Printf("[最新] 书籍「%s」已是最新版本 (本地 %d 章)。\n", info.Title, info.LocalChapters)
					return nil
				}
				fmt.Printf("[更新发现] 书籍「%s」有 %d 个新章节 (本地 %d 章 -> 远端 %d 章)\n",
					info.Title, info.NewChapterCount, info.LocalChapters, info.RemoteChapters)

				if checkOnly {
					return nil
				}

				// Perform sync: download missing chapters and illustrations
				fmt.Println("[同步] 开始同步增量章节...")
				dl, err := downloader.NewDownloader(src, store)
				if err != nil {
					return err
				}
				_ = store.SaveCatalog(info.RemoteCatalog)

				for _, vol := range info.RemoteCatalog.Volumes {
					hasMissing := false
					for _, ch := range vol.Chapters {
						if _, err := store.LoadChapter(targetID, ch.ID); err != nil {
							hasMissing = true
							break
						}
					}
					if hasMissing {
						fmt.Printf("  正在下载分卷: %s\n", vol.Title)
						err := dl.DownloadVolume(ctx, targetID, &vol, func(ev downloader.ProgressEvent) {
							fmt.Printf("\r    [%d/%d] 正在处理: %-30s", ev.Current, ev.Total, ev.ItemTitle)
						})
						if err != nil {
							fmt.Printf("\n    [警告] 分卷 %s 下载部分出错: %v\n", vol.Title, err)
						} else {
							fmt.Printf("\n    [完成] 分卷 %s 同步完成\n", vol.Title)
						}
					}
				}
				if detail, err := src.GetBookDetail(ctx, targetID); err == nil {
					_ = store.SaveBookDetail(detail)
				}
				fmt.Printf("[完成] 书籍「%s」增量更新同步完毕！\n", info.Title)
				return nil
			}

			// Batch check or batch sync across bookshelf
			fmt.Printf("[更新检查] 正在扫描全书架 %d 本小说...\n", len(cachedBooks))
			var updateList []*storage.BookUpdateInfo
			for i, b := range cachedBooks {
				fmt.Printf("\r  [%d/%d] 正在对比: %-30s", i+1, len(cachedBooks), b.Title)
				info, err := store.CheckBookUpdate(ctx, src, b.ID)
				if err == nil && info.HasUpdate {
					updateList = append(updateList, info)
				}
			}
			fmt.Println()

			if len(updateList) == 0 {
				fmt.Println("\n[最新] 全书架所有书籍均为最新章节，无需更新。")
				return nil
			}

			fmt.Printf("\n[更新报告] 共有 %d 本小说发现新章节:\n", len(updateList))
			fmt.Println(strings.Repeat("-", 80))
			for i, item := range updateList {
				fmt.Printf("[%2d] %-30s | 本地: %-4d章 -> 远端: %-4d章 | +%-2d 新章节 | ID: %s\n",
					i+1, item.Title, item.LocalChapters, item.RemoteChapters, item.NewChapterCount, item.BookID)
			}
			fmt.Println(strings.Repeat("-", 80))

			if checkOnly || !updateAll {
				fmt.Println("使用 'lnr update --all' 批量同步全部更新，或使用 'lnr update <ID>' 同步指定小说")
				return nil
			}

			// Synchronize all
			fmt.Println("\n[同步开始] 正在批量同步增量章节...")
			dl, err := downloader.NewDownloader(src, store)
			if err != nil {
				return err
			}

			for idx, item := range updateList {
				fmt.Printf("\n[%d/%d] 正在同步: %s (ID: %s)\n", idx+1, len(updateList), item.Title, item.BookID)
				_ = store.SaveCatalog(item.RemoteCatalog)

				for _, vol := range item.RemoteCatalog.Volumes {
					hasMissing := false
					for _, ch := range vol.Chapters {
						if _, err := store.LoadChapter(item.BookID, ch.ID); err != nil {
							hasMissing = true
							break
						}
					}
					if hasMissing {
						_ = dl.DownloadVolume(ctx, item.BookID, &vol, func(ev downloader.ProgressEvent) {
							fmt.Printf("\r  [%d/%d] 正在处理: %-30s", ev.Current, ev.Total, ev.ItemTitle)
						})
					}
				}
				if detail, err := src.GetBookDetail(ctx, item.BookID); err == nil {
					_ = store.SaveBookDetail(detail)
				}
				fmt.Printf("\n  [完成] %s 同步完毕\n", item.Title)
			}

			fmt.Println("\n[全部完成] 全书架增量更新同步已全部就绪！")
			return nil
		},
	}
	updateCmd.Flags().BoolP("check", "c", false, "仅检查更新，不执行下载同步")
	updateCmd.Flags().BoolP("all", "a", false, "自动同步全书架所有有更新的书籍")

	// 11. clean command
	cleanCmd := &cobra.Command{
		Use:   "clean",
		Short: "分析本地存储占用并定向清理插图缓存或导出文件",
		RunE: func(cmd *cobra.Command, args []string) error {
			cleanImages, _ := cmd.Flags().GetBool("images")
			cleanEpubs, _ := cmd.Flags().GetBool("epubs")
			cleanAll, _ := cmd.Flags().GetBool("all")

			if cleanImages {
				freed, err := store.CleanImagesOnly(true)
				if err != nil {
					return fmt.Errorf("清理插图失败: %w", err)
				}
				fmt.Printf("[清理完成] 成功清理插图缓存，共释放空间: %s (封面图片已保留)\n", formatBytes(freed))
				return nil
			}

			if cleanEpubs {
				freed, err := store.CleanEpubsOnly("")
				if err != nil {
					return fmt.Errorf("清理导出文件失败: %w", err)
				}
				fmt.Printf("[清理完成] 成功清理导出目录中的 EPUB 文件，共释放空间: %s\n", formatBytes(freed))
				return nil
			}

			if cleanAll {
				sizeBefore, _ := store.CalculateCacheSize()
				if err := store.ClearCache(); err != nil {
					return fmt.Errorf("清空缓存失败: %w", err)
				}
				freedEpubs, _ := store.CleanEpubsOnly("")
				fmt.Printf("[清理完成] 成功清空全部小说缓存与导出文件，共释放空间: %s\n", formatBytes(sizeBefore+freedEpubs))
				return nil
			}

			// Default: display storage analysis breakdown
			breakdown, err := store.GetStorageBreakdown("")
			if err != nil {
				return fmt.Errorf("获取存储占用分析失败: %w", err)
			}

			fmt.Println("==================================================")
			fmt.Println("             LNR 本地存储占用深度分析              ")
			fmt.Println("==================================================")
			fmt.Printf("• 缓存目录: %s\n", store.BaseDir())
			fmt.Printf("• 正文文本占用: %s\n", formatBytes(breakdown.TextBytes))
			fmt.Printf("• 插图图片占用: %s\n", formatBytes(breakdown.ImageBytes))
			fmt.Printf("• 导出电子书:   %s\n", formatBytes(breakdown.EpubBytes))
			fmt.Printf("• 总磁盘占用:   %s\n", formatBytes(breakdown.TotalBytes))
			fmt.Println("--------------------------------------------------")

			if len(breakdown.BookItems) > 0 {
				fmt.Println("[缓存占用排行 Top 5]")
				limit := 5
				if len(breakdown.BookItems) < limit {
					limit = len(breakdown.BookItems)
				}
				for i := 0; i < limit; i++ {
					item := breakdown.BookItems[i]
					fmt.Printf("  %d. %s (%s) - 总计: %s [插图: %s | 文本: %s]\n",
						i+1, item.Title, item.BookID,
						formatBytes(item.TotalBytes), formatBytes(item.ImageBytes), formatBytes(item.TextBytes))
				}
				fmt.Println("--------------------------------------------------")
			}

			fmt.Println("定向清理提示:")
			fmt.Println("• lnr clean --images  : 仅删除插图缓存释放大空间 (保留正文与封面)")
			fmt.Println("• lnr clean --epubs   : 仅清理已导出的 EPUB 电子书文件")
			fmt.Println("• lnr clean --all     : 清空全部小说缓存与导出文件")
			return nil
		},
	}
	cleanCmd.Flags().BoolP("images", "i", false, "仅清理插图缓存 (保留正文与书籍封面)")
	cleanCmd.Flags().BoolP("epubs", "e", false, "仅清理已导出的 EPUB 文件")
	cleanCmd.Flags().BoolP("all", "a", false, "清空所有本地小说缓存与导出文件")

	// 12. rule command
	ruleCmd := &cobra.Command{
		Use:   "rule",
		Short: "排版规范化与正则清洗规则管理",
	}

	ruleListCmd := &cobra.Command{
		Use:   "list",
		Short: "列出所有排版清洗规则",
		RunE: func(cmd *cobra.Command, args []string) error {
			rules, err := store.LoadRules()
			if err != nil {
				return fmt.Errorf("加载规则失败: %w", err)
			}
			if len(rules) == 0 {
				fmt.Println("当前未配置任何排版规则。")
				return nil
			}
			fmt.Printf("当前排版清洗规则列表 (共 %d 条):\n", len(rules))
			fmt.Println(strings.Repeat("-", 90))
			for i, r := range rules {
				status := "[已启用]"
				if !r.Enabled {
					status = "[已禁用]"
				}
				mode := "[文本]"
				if r.IsRegex {
					mode = "[正则]"
				}
				scope := "[全局]"
				if r.BookID != "" {
					scope = fmt.Sprintf("[小说:%s]", r.BookID)
				}
				fmt.Printf("[%2d] %-12s | %-8s | %-6s | %-12s | %-24s -> %q (ID: %s)\n",
					i+1, status, mode, scope, r.Name, r.Pattern, r.Replacement, r.ID)
			}
			return nil
		},
	}

	ruleAddCmd := &cobra.Command{
		Use:   "add",
		Short: "添加新的排版清洗规则",
		RunE: func(cmd *cobra.Command, args []string) error {
			name, _ := cmd.Flags().GetString("name")
			pattern, _ := cmd.Flags().GetString("pattern")
			replace, _ := cmd.Flags().GetString("replace")
			isRegex, _ := cmd.Flags().GetBool("regex")
			bookID, _ := cmd.Flags().GetString("book")

			if name == "" || pattern == "" {
				return fmt.Errorf("规则名称 (--name) 与匹配表达式 (--pattern) 均不能为空")
			}

			rule := text.FormattingRule{
				Name:        name,
				Pattern:     pattern,
				Replacement: replace,
				IsRegex:     isRegex,
				BookID:      bookID,
				Enabled:     true,
			}

			if err := store.AddRule(rule); err != nil {
				return fmt.Errorf("添加规则失败: %w", err)
			}

			fmt.Printf("[成功] 已成功添加排版规则: %q\n", name)
			return nil
		},
	}
	ruleAddCmd.Flags().StringP("name", "n", "", "规则名称 (必填)")
	ruleAddCmd.Flags().StringP("pattern", "p", "", "匹配表达式 (必填)")
	ruleAddCmd.Flags().StringP("replace", "r", "", "替换文本")
	ruleAddCmd.Flags().Bool("regex", false, "是否为正则表达式")
	ruleAddCmd.Flags().StringP("book", "b", "", "限定适用的特定小说ID (默认留空表示全局适用)")

	ruleToggleCmd := &cobra.Command{
		Use:   "toggle <规则ID>",
		Short: "切换指定规则的启用/禁用状态",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ruleID := args[0]
			enabled, err := store.ToggleRule(ruleID)
			if err != nil {
				return fmt.Errorf("切换规则状态失败: %w", err)
			}
			state := "已启用"
			if !enabled {
				state = "已禁用"
			}
			fmt.Printf("[成功] 规则 %q 当前状态: [%s]\n", ruleID, state)
			return nil
		},
	}

	ruleDeleteCmd := &cobra.Command{
		Use:   "delete <规则ID>",
		Short: "删除指定的排版规则",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ruleID := args[0]
			if err := store.DeleteRule(ruleID); err != nil {
				return fmt.Errorf("删除规则失败: %w", err)
			}
			fmt.Printf("[成功] 已删除规则: %q\n", ruleID)
			return nil
		},
	}

	ruleCmd.AddCommand(ruleListCmd, ruleAddCmd, ruleToggleCmd, ruleDeleteCmd)

	// 13. publisher command
	publisherCmd := &cobra.Command{
		Use:   "publisher [class_id | list]",
		Short: "按出版社/文库维度精细淘书",
		RunE: func(cmd *cobra.Command, args []string) error {
			pubs := src.GetPublishers()

			if len(args) == 0 || args[0] == "list" {
				fmt.Printf("\n[文库淘书] Wenku8 官方收录文库一览 (共 %d 家):\n", len(pubs))
				fmt.Println(strings.Repeat("-", 70))
				for _, p := range pubs {
					fmt.Printf("  [%2d] %-14s", p.ClassID, p.Name)
					if p.ClassID%4 == 0 {
						fmt.Println()
					}
				}
				if len(pubs)%4 != 0 {
					fmt.Println()
				}
				fmt.Println(strings.Repeat("-", 70))
				fmt.Println("使用 'lnr publisher <文库ID或名称>' 浏览指定文库小说 (例如: lnr publisher 1)")
				return nil
			}

			targetStr := args[0]
			var targetClassID int
			var targetName string

			for _, p := range pubs {
				if fmt.Sprintf("%d", p.ClassID) == targetStr || p.Name == targetStr {
					targetClassID = p.ClassID
					targetName = p.Name
					break
				}
			}
			if targetClassID == 0 {
				return fmt.Errorf("未找到文库 '%s'，请运行 'lnr publisher list' 查看所有文库列表", targetStr)
			}

			page, _ := cmd.Flags().GetInt("page")
			if page < 1 {
				page = 1
			}

			fmt.Printf("[淘书] 正在浏览文库「%s」(ID: %d，第 %d 页)...\n", targetName, targetClassID, page)
			results, totalPages, err := src.GetPublisherBooks(context.Background(), targetClassID, page)
			if err != nil {
				return fmt.Errorf("获取文库小说失败: %w", err)
			}

			if len(results) == 0 {
				fmt.Printf("文库「%s」暂无书籍数据。\n", targetName)
				return nil
			}

			fmt.Printf("\n文库「%s」共检索到 %d 本小说 (当前第 %d/%d 页):\n", targetName, len(results), page, totalPages)
			fmt.Println(strings.Repeat("-", 80))
			for i, book := range results {
				status := "连载中"
				if book.IsComplete {
					status = "已完结"
				}
				fmt.Printf("[%2d] %-30s | 作者: %-15s | 文库: %-10s | %-6s | %d 字 | ID: %s\n",
					(page-1)*20+i+1, book.Title, book.Author, book.Publisher, status, book.WordCount, book.ID)
			}
			fmt.Println(strings.Repeat("-", 80))
			fmt.Println("使用 'lnr info <ID>' 查看详情，使用 'lnr download <ID>' 下载")
			return nil
		},
	}
	publisherCmd.Flags().IntP("page", "p", 1, "文库列表分页页码")

	rootCmd.AddCommand(searchCmd, infoCmd, downloadCmd, exportCmd, coverCmd, imageCmd, topCmd, tagsCmd, tagCmd, publisherCmd, updateCmd, cleanCmd, ruleCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	// Helper to avoid unused strconv import if needed
	_ = strconv.Itoa(0)
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
