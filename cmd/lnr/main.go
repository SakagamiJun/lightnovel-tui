package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"lnr-core/pkg/downloader"
	"lnr-core/pkg/epub"
	termimage "lnr-core/pkg/image"
	"lnr-core/pkg/model"
	"lnr-core/pkg/source"
	"lnr-core/pkg/source/wenku8"
	"lnr-core/pkg/storage"
)

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
			targetVol, _ := cmd.Flags().GetInt("volume")
			split, _ := cmd.Flags().GetBool("split")
			outputPath, _ := cmd.Flags().GetString("output")
			ctx := context.Background()

			dl, _ := downloader.NewDownloader(src, store)
			exporter := epub.NewExporter(store, dl)

			if split {
				fmt.Printf("[导出] 正在将书籍 %s 的所有分卷分别导出为独立 EPUB 文件...\n", bookID)
				outFiles, err := exporter.ExportAllVolumes(ctx, bookID, outputPath, func(current, total int, volTitle, outFile string) {
					fmt.Printf("  [%d/%d] (%.1f%%) 已导出分卷: %s -> %s\n", current, total, float64(current)/float64(total)*100, volTitle, filepath.Base(outFile))
				})
				if err != nil {
					return fmt.Errorf("按分卷分别导出 EPUB 失败: %w", err)
				}
				fmt.Printf("[完成] 成功导出全部 %d 卷独立 EPUB 文件！\n", len(outFiles))
				return nil
			}

			if targetVol > 0 {
				fmt.Printf("[导出] 正在导出书籍 %s 的第 %d 卷为 EPUB...\n", bookID, targetVol)
				outFile, err := exporter.ExportVolume(ctx, bookID, targetVol, outputPath)
				if err != nil {
					return fmt.Errorf("导出分卷 EPUB 失败: %w", err)
				}
				fmt.Printf("[完成] 成功导出分卷 EPUB 文件: %s\n", outFile)
			} else {
				fmt.Printf("[导出] 正在导出书籍 %s 的全本为 EPUB...\n", bookID)
				outFile, err := exporter.ExportFullBook(ctx, bookID, outputPath)
				if err != nil {
					return fmt.Errorf("导出全本 EPUB 失败: %w", err)
				}
				fmt.Printf("[完成] 成功导出全本 EPUB 文件: %s\n", outFile)
			}
			return nil
		},
	}
	exportCmd.Flags().IntP("volume", "v", 0, "指定导出分卷 (默认0表示整本导出)")
	exportCmd.Flags().BoolP("split", "s", false, "将所有分卷分别导出为独立的 EPUB 文件 (每卷一个 EPUB)")
	exportCmd.Flags().StringP("output", "o", "", "指定导出 EPUB 文件路径或保存目录")

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

	rootCmd.AddCommand(searchCmd, infoCmd, downloadCmd, exportCmd, coverCmd, imageCmd, topCmd, tagsCmd, tagCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	// Helper to avoid unused strconv import if needed
	_ = strconv.Itoa(0)
}
