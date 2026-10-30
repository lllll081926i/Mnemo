# 文档连续滚动预览

日期：2026-10-30。

## 需求与范围

PDF、Word、PowerPoint 和 Excel 均通过纵向滚动继续阅读。移除上一页、下一页、上一张、下一张按钮。保留页码读数、缩放、适合宽度和工作表选择；工作表属于独立内容，不作为分页处理。

## 实现

- PDF 使用项目已有 PDF.js 的 `PDFViewer`，采用纵向页面列表、可见区域渲染队列与画布回收。保留 Range 按需读取、密码解锁、重试和单页画布像素限制。滚动更新页码，缩放交由阅读器保留位置。关闭或重试时取消观察器和事件监听，销毁旧文档任务。页面初始化失败通过 `pagesPromise` 返回错误，不能一直停在加载状态。
- PPT 使用项目已有渲染器的 `list` 模式。启用窗口渲染、相邻页面预渲染、延迟解析和媒体加载，并将实际滚动容器传入阅读器。页面宽度跟随预览区域，缩放保持阅读位置。保留 ZIP 安全限制和关闭取消信号。
- Excel 移除每 100 行分页。使用固定行高、前后占位和可见区域渲染，保留完整工作表的滚动高度。固定列布局避免内容切换时列宽跳动；表头和行号固定。切换工作表时回到顶部。沿用原有前 50 列展示限制，行数遵循 XLSX 的 1,048,576 行上限。
- Word 原有多页纵向排版已满足需求，保留完整排版与兼容回退。

## 验证

浏览器脚本为 `frontend/document-scroll.browser.cjs`。脚本在本地生成文档，用真实渲染库核验 30 页 PDF、12 张 PPT、4000 行表格与 3 页 Word，不连接网盘账号。

覆盖滚轮跨页、末页/末行可达、无翻页按钮、PDF 画布数量受限、PDF/PPT 缩放位置、窗口尺寸变化、表格可见行数量、固定表头、工作表切换和关闭后无未处理异常。截图保存在系统临时目录 `mnemo-document-scroll-qa`。PDF 组件单测另覆盖密码、失败重试、关闭时迟到响应与页面初始化失败。

前端 37 个测试文件、306 项测试通过。连续滚动浏览器验收通过，并检查实际截图。前端生产构建与 Windows 应用构建通过，测试程序为 `build/bin/Mnemo-review.exe`。仍有原有播放器等第三方包的大块提示。开发验收页的 Vite 热更新 WebSocket 被浏览器本地网络策略阻止，未影响文档模块读取及验收。

运行方式：

```powershell
# 先在 frontend 目录启动 npm run dev。
$env:PLAYWRIGHT_MODULE = '本机 Playwright 模块路径'
node frontend/document-scroll.browser.cjs
```

PDF 集成参考 [Mozilla 的组件示例](https://github.com/mozilla/pdf.js/blob/master/examples/components/simpleviewer.mjs) 和仓库安装的 `pdfjs-dist` 6.2.108；PPT 列表与窗口渲染契约参考仓库安装的 `@aiden0z/pptx-renderer` 1.2.4 README 与类型声明。没有增加运行时依赖。
