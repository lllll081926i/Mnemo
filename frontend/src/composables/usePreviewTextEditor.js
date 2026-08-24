import { computed, nextTick, ref } from 'vue'
import { copyText, saveCloudText } from '../api'

export function usePreviewTextEditor({ account, activeFile, emit }) {
  // ---------- 文本与代码专业预览/编辑状态 ----------
  const isMarkdownFile = computed(() => {
    const name = (activeFile.value.name || '').toLowerCase()
    return name.endsWith('.md') || name.endsWith('.markdown')
  })

  // 文本模式：'preview' (代码/文本预览) | 'markdown' (文档渲染) | 'edit' (在线编辑)
  const textMode = ref('preview')
  const fontSize = ref(13.5)
  const wordWrap = ref(false) // 默认不自动换行，保持宽敞横向滚动，避免长行频繁被折断
  const showLineNumbers = ref(true)
  const encoding = ref('UTF-8')
  const copiedFull = ref(false)
  const showSearch = ref(false)
  const searchKw = ref('')
  const searchInputEl = ref(null)

  // 光标位置
  const cursorPos = ref({ line: 1, col: 1 })
  const gutterEl = ref(null)
  const viewEl = ref(null)
  const editorEl = ref(null)
  const confirmLeaveDialog = ref(false)

  const currentText = computed(() => (textMode.value === 'edit' ? editContent.value : text.value))
  const textLines = computed(() => currentText.value.split('\n'))
  const isModified = computed(() => textMode.value === 'edit' && editContent.value !== text.value)

  // 语言标签识别
  const langMeta = computed(() => {
    const name = (activeFile.value.name || '').toLowerCase()
    const ext = name.includes('.') ? name.split('.').pop() : ''
    const map = {
      js: 'JavaScript', mjs: 'JavaScript', cjs: 'JavaScript',
      ts: 'TypeScript', tsx: 'TypeScript React', jsx: 'React JSX',
      vue: 'Vue Component',
      json: 'JSON', json5: 'JSON5', jsonc: 'JSON with Comments',
      html: 'HTML', htm: 'HTML',
      css: 'CSS', scss: 'SCSS', sass: 'SASS', less: 'LESS',
      go: 'Go',
      py: 'Python', pyw: 'Python',
      rs: 'Rust',
      java: 'Java', kt: 'Kotlin',
      c: 'C', cpp: 'C++', cc: 'C++', h: 'C/C++ Header', hpp: 'C++ Header',
      cs: 'C#',
      php: 'PHP',
      rb: 'Ruby',
      sh: 'Shell Script', bash: 'Bash Script', zsh: 'Zsh Script', ps1: 'PowerShell', bat: 'Batch', cmd: 'Batch',
      sql: 'SQL Database',
      yaml: 'YAML', yml: 'YAML',
      xml: 'XML', svg: 'SVG Image/XML',
      toml: 'TOML', ini: 'INI Config', conf: 'Config', env: 'Environment',
      md: 'Markdown', markdown: 'Markdown',
      txt: 'Plain Text', log: 'Log File',
    }
    return map[ext] || (ext ? ext.toUpperCase() : 'Plain Text')
  })

  // 滚动同步（行号与内容区 100% 像素级对齐）
  function onContentScroll(e) {
    if (gutterEl.value) {
      gutterEl.value.scrollTop = e.target.scrollTop
    }
  }

  // 统计字符数/字数
  const charCount = computed(() => currentText.value.length)
  const lineEnding = computed(() => (currentText.value.includes('\r\n') ? 'CRLF' : 'LF'))

  // 搜索匹配拆分
  function searchParts(line) {
    const kw = searchKw.value.trim()
    if (!kw) return null
    const str = String(line || '')
    const i = str.toLowerCase().indexOf(kw.toLowerCase())
    if (i < 0) return null
    return [
      { text: str.slice(0, i), hit: false },
      { text: str.slice(i, i + kw.length), hit: true },
      { text: str.slice(i + kw.length), hit: false },
    ]
  }

  const matchCount = computed(() => {
    const kw = searchKw.value.trim()
    if (!kw) return 0
    let count = 0
    const kwLower = kw.toLowerCase()
    for (const line of textLines.value) {
      let p = 0
      const lower = line.toLowerCase()
      while ((p = lower.indexOf(kwLower, p)) !== -1) {
        count++
        p += kwLower.length
      }
    }
    return count
  })

  function toggleSearch() {
    showSearch.value = !showSearch.value
    if (showSearch.value) {
      nextTick(() => searchInputEl.value?.focus())
    } else {
      searchKw.value = ''
    }
  }

  // 复制全部文本
  async function copyAllText() {
    const ok = await copyText(currentText.value)
    if (ok) {
      copiedFull.value = true
      emit('toast', '已复制全部文本内容', 'success')
      setTimeout(() => { copiedFull.value = false }, 1800)
    } else {
      emit('toast', '复制失败', 'error')
    }
  }

  // 编辑器光标与按键事件
  function updateCursorPos(e) {
    const el = e.target
    if (!el || typeof el.selectionStart !== 'number') return
    const val = el.value.slice(0, el.selectionStart)
    const lines = val.split('\n')
    cursorPos.value = {
      line: lines.length,
      col: lines[lines.length - 1].length + 1,
    }
  }

  function onEditorKeyDown(e) {
    if (e.key === 'Tab') {
      e.preventDefault()
      const el = e.target
      const start = el.selectionStart
      const end = el.selectionEnd
      const val = editContent.value
      editContent.value = val.substring(0, start) + '  ' + val.substring(end)
      nextTick(() => {
        el.selectionStart = el.selectionEnd = start + 2
        updateCursorPos(e)
      })
    } else if ((e.ctrlKey || e.metaKey) && (e.key === 's' || e.code === 'KeyS')) {
      e.preventDefault()
      doSaveText()
    } else if ((e.ctrlKey || e.metaKey) && (e.key === 'f' || e.code === 'KeyF')) {
      e.preventDefault()
      toggleSearch()
    } else {
      nextTick(() => updateCursorPos(e))
    }
  }

  // 保存修改回传云端
  async function doSaveText() {
    if (saving.value || !isModified.value) return
    saving.value = true
    try {
      const parentId = activeFile.value.parent_file_id || 'root'
      await saveCloudText(
        account.user_id,
        account.drive_id,
        parentId,
        activeFile.value.name,
        editContent.value
      )
      text.value = editContent.value
      emit('toast', '保存成功，已上传到网盘', 'success')
      emit('saved')
    } catch (e) {
      emit('toast', '保存失败: ' + String(e), 'error')
    } finally {
      saving.value = false
    }
  }

  // 关闭前未保存检查
  function handleCloseRequest() {
    if (isModified.value) {
      confirmLeaveDialog.value = true
    } else {
      emit('close')
    }
  }

  // ---------- 轻量安全 Markdown 解析器 ----------
  function escapeHtml(str) {
    return String(str || '')
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;')
  }

  function sanitizeUrl(rawUrl) {
    const trimmed = String(rawUrl || '').trim()
    if (/^(https?:\/\/|mailto:|\/|\.\/|#)/i.test(trimmed)) {
      return escapeHtml(trimmed)
    }
    return '#'
  }

  function renderMarkdown(src) {
    if (!src) return ''

    // 代码块提取 (```lang ... ```)
    const codeBlocks = []
    let md = src.replace(/```([a-zA-Z0-9_-]*)\n([\s\S]*?)```/g, (_, lang, code) => {
      const idx = codeBlocks.length
      const safeLang = escapeHtml(lang || 'code')
      const safeCode = escapeHtml(code.trim())
      codeBlocks.push(
        `<div class="md-code-block"><div class="md-code-header"><span class="md-code-lang">${safeLang}</span></div><pre><code>${safeCode}</code></pre></div>`
      )
      return `<!--CODEBLOCK_${idx}-->`
    })

    // 转义 HTML 字符
    md = escapeHtml(md)

    // 行内代码
    md = md.replace(/`([^`]+)`/g, '<code class="md-inline-code">$1</code>')

    // 标题
    md = md.replace(/^###### (.*$)/gim, '<h6 class="md-h6">$1</h6>')
    md = md.replace(/^##### (.*$)/gim, '<h5 class="md-h5">$1</h5>')
    md = md.replace(/^#### (.*$)/gim, '<h4 class="md-h4">$1</h4>')
    md = md.replace(/^### (.*$)/gim, '<h3 class="md-h3">$1</h3>')
    md = md.replace(/^## (.*$)/gim, '<h2 class="md-h2">$1</h2>')
    md = md.replace(/^# (.*$)/gim, '<h1 class="md-h1">$1</h1>')

    // 分割线
    md = md.replace(/^---$/gim, '<hr class="md-hr" />')

    // 引用块 (转义后为 &gt;)
    md = md.replace(/^&gt;\s?(.*$)/gim, '<blockquote class="md-quote">$1</blockquote>')

    // 格式
    md = md.replace(/\*\*\*(.*?)\*\*\*/g, '<strong><em>$1</em></strong>')
    md = md.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>')
    md = md.replace(/\*(.*?)\*/g, '<em>$1</em>')
    md = md.replace(/~~(.*?)~~/g, '<del>$1</del>')

    // 任务列表
    md = md.replace(/^- \[x\] (.*$)/gim, '<li class="md-task-item"><span class="md-task-box checked">✓</span> <span>$1</span></li>')
    md = md.replace(/^- \[ \] (.*$)/gim, '<li class="md-task-item"><span class="md-task-box"></span> <span>$1</span></li>')

    // 列表
    md = md.replace(/^[-\*] (.*$)/gim, '<li class="md-list-item">$1</li>')

    // 链接（严格白名单与 URL 属性清洗）
    md = md.replace(/\[([^\]]+)\]\(([^)]+)\)/g, (_, label, link) => {
      const href = sanitizeUrl(link)
      return `<a href="${href}" target="_blank" rel="noopener noreferrer" class="md-link">${label}</a>`
    })

    // 段落
    md = md.replace(/\n\n/g, '<div class="md-gap"></div>')
    md = md.replace(/\n/g, '<br />')

    // 还原代码块
    codeBlocks.forEach((block, idx) => {
      md = md.replace(`<!--CODEBLOCK_${idx}-->`, block)
    })

    return md
  }

  const renderedMarkdown = computed(() => renderMarkdown(text.value))

  // 文本编码探测
  function decodeText(buf) {
    const u8 = new Uint8Array(buf)
    if (u8.length >= 3 && u8[0] === 0xef && u8[1] === 0xbb && u8[2] === 0xbf) {
      return { text: new TextDecoder('utf-8').decode(u8.subarray(3)), encoding: 'UTF-8 (BOM)' }
    }
    const utf8 = new TextDecoder('utf-8', { fatal: false }).decode(u8)
    let replacements = 0
    for (let i = 0; i < utf8.length; i++) {
      if (utf8.charCodeAt(i) === 0xfffd) replacements++
    }
    if (replacements > utf8.length / 100) {
      try {
        return { text: new TextDecoder('gbk').decode(u8), encoding: 'GBK' }
      } catch {
        // 回退
      }
    }
    return { text: utf8, encoding: 'UTF-8' }
  }

  return {
    text,
    editContent,
    saving,
    isMarkdownFile,
    textMode,
    fontSize,
    wordWrap,
    showLineNumbers,
    encoding,
    copiedFull,
    showSearch,
    searchKw,
    searchInputEl,
    cursorPos,
    gutterEl,
    viewEl,
    editorEl,
    confirmLeaveDialog,
    currentText,
    textLines,
    isModified,
    langMeta,
    onContentScroll,
    charCount,
    lineEnding,
    searchParts,
    matchCount,
    toggleSearch,
    copyAllText,
    updateCursorPos,
    onEditorKeyDown,
    doSaveText,
    handleCloseRequest,
    renderedMarkdown,
    decodeText,
  }
}
