# LightNovelReader Core (lnr-core)

`lnr-core` 是轻小说阅读器的 Go 语言核心功能底座。它采用模块化与无外部 UI 耦合设计，作为统一功能引擎支撑命令行 CLI、终端交互界面（TUI）以及未来桌面图形客户端（GUI）。

## 项目定位与架构演进

```
lnr-core/
├── go.mod                # module lnr-core
├── cmd/                  # 多端目标入口 (统一调用底层 pkg/ core)
│   ├── lnr/              # 命令行 CLI 工具 (search / info / download / export)
│   ├── lnr-tui/          # 终端交互界面 TUI (规划中，基于 Bubble Tea)
│   └── lnr-gui/          # 桌面 GUI 客户端 (规划中，基于 Fyne / Wails)
├── pkg/                  # 核心功能底座 (Core Engine Library)
│   ├── model/            # 领域模型 (BookSummary, BookDetail, Volume, Chapter, Element)
│   ├── source/           # 多源抽象与具体实现 (Wenku8 爬虫、会话与 GB18030 转码)
│   ├── downloader/       # 低内存流式下载器 (支持 ProgressEvent 事件通知与限流)
│   ├── epub/             # 符合标准的流式 EPUB 导出打包引擎
│   ├── storage/          # 本地缓存与书目文件管理
│   └── reader/           # 阅读器引擎 (章节文本排版、分页切片与阅读书签记录)
└── internal/
    ├── client/           # HTTP 客户端配置与 Cookie 会话管理
    └── encoding/         # GB18030 / UTF-8 流式转码器
```

## 特性

- **纯净底层设计**：核心包（`pkg/`）无任何控制台硬编码打印，通过强类型事件与通道（`ProgressEvent`）向上层 UI 回传状态。
- **极致内存节省**：全链路采用流式处理（`GB18030ToUTF8Reader`、`sync.Pool` 32KB 缓冲池复用、流式 ZIP 写入），杜绝大内存分配。
- **开箱即用 CLI**：内置 `lnr` 命令行工具，提供搜索、查看、分卷下载与 EPUB 导出功能。
- **阅读底座准备**：提供 `pkg/reader` 引擎，支持断点书签、文本对齐排版与图片混排，可直接被 TUI/GUI 接入。

## 快速上手 (CLI)

### 编译构建
```bash
cd lnr-core
go build -o bin/lnr ./cmd/lnr/main.go
```

### 基础命令 (CLI)
```bash
# 1. 搜索小说
./bin/lnr search "关于我转生变成史莱姆这档事"
./bin/lnr search "伏濑" -a

# 2. 查看详情与分卷目录
./bin/lnr info 4340

# 3. 下载小说与插图 (支持指定分卷或全本)
./bin/lnr download 4340 --volume 1

# 4. 导出为 EPUB 电子书
./bin/lnr export 4340 --volume 1 -o ./slime_vol1.epub
./bin/lnr export 4340 -o ./slime_complete.epub
```

## 终端交互界面 (TUI)

基于 Bubble Tea 打造的高性能轻量级终端阅读器界面：

### 编译与启动
```bash
go build -o bin/lnr-tui ./cmd/lnr-tui/main.go
./bin/lnr-tui
```

### 快捷键导航
- `Tab`：在【📚 本地书架】与【🔍 在线搜索】之间快速切换。
- `↑ / ↓` 或 `k / j`：选择小说或分卷章节。
- `Enter`：进入选中的小说目录或阅读所选章节。
- `Esc`：从阅读界面返回目录，或从目录返回书架（退出时自动记录阅读行数与书签）。
- `j / k / 空格 / PageDown`：在正文阅读视口中流畅滚动与翻页。
- `q`：在书架页面按 `q` 退出程序；任意界面支持 `Ctrl+C` 强制退出。

## 测试与质量保障

在项目根目录下运行全部单元测试：
```bash
go test -v ./...
```
