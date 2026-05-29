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

	rootCmd.AddCommand(searchCmd, infoCmd, downloadCmd, exportCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	// Helper to avoid unused strconv import if needed
	_ = strconv.Itoa(0)
}
