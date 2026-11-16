# 墨页

简约的本地 Markdown 阅读器，使用 Go、Wails v3、Vue 3 和 TypeScript，当前交付 Windows x64 桌面版。

## 使用

运行 `bin/MarkdownLite.exe`。通过顶栏“打开”选择一个或多个 `.md` / `.markdown` 文件，也可将文件拖入窗口，或把文件路径作为启动参数传入。

浅色标题栏搭配书页图标，紧凑顶栏放置打开文件、文件标签和原文 / 分栏 / 预览切换。拖动文件标签可以调整顺序，拖到标签栏边缘会自动滚动，按 Esc 可取消拖动。点击鼠标中键可以关闭标签，有未保存修改时会提示处理。右侧“…”菜单提供复制 Markdown 和编辑锁定。

分栏宽度支持拖动调整。文件单独保留阅读位置、显示模式和编辑撤销历史。

编辑锁定默认开启。关闭锁定即可修改原文，预览会随输入更新。修改后的标签显示小圆点，“保存”按钮按需出现。切回锁定后可继续保存已有改动，关闭标签或退出时会提示处理尚未保存的修改。

“复制 Markdown”复制当前文件的完整原文，包含尚未保存的修改，保留原文中的标记、空格和换行。预览支持表格、任务列表、代码高亮和相对路径图片。文件使用 UTF-8 编码。

| 快捷键 | 操作 |
|---|---|
| Ctrl+O | 打开文件，可多选 |
| Ctrl+W | 关闭当前标签 |
| 鼠标中键 | 关闭指向的标签 |
| Ctrl+Tab / Ctrl+Shift+Tab | 下一个 / 上一个标签 |
| Ctrl+Shift+L | 切换全局编辑锁定 |
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
