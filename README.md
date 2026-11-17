# 墨页

简约的本地 Markdown 阅读器，使用 Go、Wails v3、Vue 3 和 TypeScript，当前交付 Windows x64 桌面版。

## 下载

从 [Releases](https://github.com/fengfengzhidao/markdown-preview/releases/latest) 下载 Windows x64 版本。ZIP 解压后运行 `MarkdownLite.exe`，也可以直接下载 EXE。系统需要 WebView2 Runtime，发布附件中的 `SHA256SUMS.txt` 提供文件校验值。

## 使用

运行 `bin/MarkdownLite.exe`。通过顶栏“打开”选择一个或多个 `.md` / `.markdown` 文件，也可将文件拖入窗口，或把文件路径作为启动参数传入。

窗口标题栏搭配书页图标，紧凑顶栏放置打开文件、文件标签和原文 / 分栏 / 预览切换。拖动文件标签可以调整顺序，拖到标签栏边缘会自动滚动，按 Esc 可取消拖动。点击鼠标中键可以关闭标签，有未保存修改时会提示处理。右侧“…”菜单提供复制 Markdown、编辑锁定和黑夜模式切换，主题选择会在下次启动时恢复。

未打开文件时显示欢迎页，提供功能说明、使用指引、快捷键，以及打开文件和新建文档的入口。

分栏宽度支持拖动调整。文件单独保留阅读位置、显示模式和编辑撤销历史。

标签总宽度超出顶栏可用空间时，显示“更多文章”按钮。点击可从菜单中切换到任意已打开的文章，菜单同时显示文件名、路径、当前文章和未保存状态，支持上下方向键选择及 Enter 切换。

右键文件标签或“更多文章”中的条目，可打开当前文件夹、复制文件路径或重命名。重命名会同步修改磁盘文件与标签信息。未保存文档可以先改名，首次保存时使用所选名称。

编辑锁定默认开启。关闭锁定即可修改原文，预览会随输入更新。修改后的标签显示小圆点，“保存”按钮按需出现。切回锁定后可继续保存已有改动，关闭标签或退出时会提示处理尚未保存的修改。

点击顶栏“新建”或按 Ctrl+N 创建未命名文档，编辑锁定默认关闭，可直接输入。锁定状态按标签页单独保留。首次按 Ctrl+S 选择保存位置，取消后保留当前文稿，后续保存继续使用所选路径。

“复制 Markdown”复制当前文件的完整原文，包含尚未保存的修改，保留原文中的标记、空格和换行。预览支持表格、任务列表、代码高亮和相对路径图片。文件使用 UTF-8 编码。

| 快捷键 | 操作 |
|---|---|
| Ctrl+N | 新建 Markdown 文档，默认允许编辑 |
| Ctrl+O | 打开文件，可多选 |
| Ctrl+W | 关闭当前标签 |
| 鼠标中键 | 关闭指向的标签 |
| Ctrl+Tab / Ctrl+Shift+Tab | 下一个 / 上一个标签 |
| Ctrl+Shift+L | 切换当前文件的编辑锁定 |
| Ctrl+Shift+C | 复制完整 Markdown |
| Ctrl+S | 保存当前文件 |

保存使用同目录临时文件写入、同步和替换。保存前检查磁盘版本，遇到外部改动时提供重新载入、覆盖保存和取消。

## 开发与构建

需要 Windows x64、Go 1.25+、Node.js 22.12+ 和 Wails CLI `v3.0.0-beta.19`。Go 模块指定 `go1.25.13` 工具链，Go 可按需自动下载。

```powershell
git clone https://github.com/fengfengzhidao/markdown-preview.git
cd markdown-preview
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.19
wails3 build
```

构建任务安装前端依赖、生成 Go 绑定、编译前端并输出 `bin/MarkdownLite.exe`。Windows 需要 WebView2 Runtime。

开发模式使用以下命令。

```powershell
wails3 task dependencies
wails3 dev -config ./build/config.yml -port 9256
```

前端构建完成后可执行文件存储与图片路径的回归检查。

```powershell
go test ./...
go vet ./...
```
