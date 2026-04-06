# LightNovelReader CLI (lnr)

LightNovelReader CLI 是一个用 Go 语言编写的轻量级轻小说阅读与下载命令行工具。

## 特性

- **轻量低内存**：采用流式 GB18030 字符集转码、受控协程并发、复用缓冲区，保持极低内存占用。
- **小说搜索**：支持按书名或作者检索轻小说，自动处理搜索频率限制。
- **详情与目录查看**：快速浏览小说详细元数据、文库分类、状态、字数与完整分卷章节树。
- **流式下载**：支持整本或指定分卷下载，完整抓取正文段落与全高清插图，支持本地断点续传。
- **EPUB 电子书导出**：支持按分卷或整本打包导出为标准 EPUB 格式，内嵌插图、目录导航与自适应排版。
- **高扩展性**：基于统一 `DataSource` 抽象，便于后续扩展 TUI 交互式终端阅读界面（如 Bubble Tea）与其他书源。

## 编译与安装

```bash
cd lnr-cli
go build -o bin/lnr ./cmd/lnr/main.go
```

## 使用说明

### 1. 搜索小说
```bash
# 按书名搜索
./bin/lnr search "关于我转生变成史莱姆这档事"

# 按作者搜索
./bin/lnr search "伏濑" -a

# 翻页搜索
./bin/lnr search "史莱姆" -p 2
```

### 2. 查看小说详情与分卷目录
```bash
./bin/lnr info 4340
```

### 3. 下载小说与插图
```bash
# 下载指定分卷（如第1卷）
./bin/lnr download 4340 --volume 1

# 下载全本所有分卷
./bin/lnr download 4340
```

### 4. 导出为 EPUB 电子书
```bash
# 导出指定分卷
./bin/lnr export 4340 --volume 1 -o ./slime_vol1.epub

# 导出全本单文件
./bin/lnr export 4340 -o ./slime_complete.epub
```
