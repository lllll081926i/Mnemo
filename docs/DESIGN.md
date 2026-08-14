# Mnemo-Go 前端设计规范

本规范继承自旧版 Mnemo 的 `DESIGN.md` 与 `design-tokens.css`，所有 token 变量与旧版一致。
前端已用精简 Vue 3 重写，视觉风格对齐旧版。

## 设计原则

- 页面以文件、任务和设置内容为中心，不为装饰增加多余元素。
- 浅色/深色主题使用同一套 `design-tokens.css` 语义变量，组件不硬编码颜色。
- 层级只由文字权重、间距、分隔线和侧栏定位表达，不使用卡片中嵌卡片。
- 前端 CSS 使用 `main.css` 中的令牌类；页面添加局部 `<style scoped>` 只解决内容特有的排列。
- 旧版 `design-tokens.css` 的 `--color-primary`、`--bg-*`、`--text-*`、`--radius-*`、
  `--space-*`、`--shadow-*` 等语义变量在新版中完整保留，所有组件样式均引用这些变量，
  不新增硬编码值。

## 令牌职责

| 文件 | 职责 |
| --- | --- |
| `frontend/src/styles/design-tokens.css` | 颜色、文字、边框、阴影、主题语义变量（从旧版复制） |
| `frontend/src/styles/main.css` | 应用壳、侧栏、列表、工具栏、按钮、输入框、模态框、任务列表等共用布局规则 |
| 页面 `<style scoped>` | 该页面独有的排布和响应式细节 |

## 排版与间距

- 基准字号 `13px`；辅助字号 `12px`。
- 侧栏菜单行高 `36px`；列表行高 `38px`；按钮高度 `32px`。
- 圆角统一使用 `--radius-*` 变量。
- 间距使用 `--space-*` 变量，不新增近似数值。

## 页面壳

- 左侧 `account-rail` 宽度 200px，展示账号列表与导航页签。
- 右侧 `main-area` 为平铺内容区，无额外卡片或色块。
- 网盘列表使用 `.file-list` 网格布局，右键菜单用 `.ctx-menu`。
- 传输任务使用 `.task-item` 行式布局，进度条用 `.tbar`。
- 设置页使用 `.setting-row` 标签-控件行。

## 颜色

只使用 `design-tokens.css` 中的语义变量，不直接写十六进制颜色。

## 控件

按钮：`.btn`（常规）/ `.btn.primary`（主色）/ `.btn.danger`（危险）。
输入框：`.input` / `.select`。开关：`.switch`。
模态框：`.modal-mask` + `.modal`。右键菜单：`.ctx-menu` + `.ctx-item`。