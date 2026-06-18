import { ref, nextTick } from 'vue'
import { streamText, stepCountIs } from 'ai'
import { createOpenAI } from '@ai-sdk/openai'
import { createOpenAICompatible } from '@ai-sdk/openai-compatible'
import { z } from 'zod'
import type { AIModelConfig } from '../../utils/bookAI'
import { getAIConfig } from '../../utils/bookAI'
import { searchAllDrives } from '../../utils/globalSearch'
import type { GlobalSearchResult } from '../../utils/globalSearch'
import type { ChatMessage, MessagePart, FileResult, LinkResult } from './types'
import AliShare from '../../aliapi/share'
import AliFileCmd from '../../aliapi/filecmd'
import UserDAL from '../../user/userdal'
import { parseQuarkShareLink } from '../../quark/share'

const CHAT_KEY = 'ai_search_chat_history_v2'

function loadHistory(): ChatMessage[] {
  try {
    const raw = localStorage.getItem(CHAT_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter((m: any) => m?.id && Array.isArray(m.parts))
  } catch {
    return []
  }
}

function saveHistory(messages: ChatMessage[]) {
  try { localStorage.setItem(CHAT_KEY, JSON.stringify(messages.slice(-50))) } catch {}
}

export function useAISearchChat(phSearchFn: (kw: string) => Promise<any>) {
  const messages = ref<ChatMessage[]>(loadHistory())
  const loading = ref(false)
  const streamingMessageId = ref('')
  let abortController: AbortController | null = null

  function appendPart(msgId: string, part: MessagePart) {
    const msg = messages.value.find(m => m.id === msgId)
    if (msg) msg.parts = [...msg.parts, part]
  }

  function updateToolPart(msgId: string, toolType: string, input: any, fn: (part: any) => void) {
    const msg = messages.value.find(m => m.id === msgId)
    if (!msg) return
    const parts = [...msg.parts]
    let idx = -1
    for (let i = parts.length - 1; i >= 0; i--) {
      const p = parts[i]
      if (p.type === toolType && (p as any).input?.keyword === input?.keyword) { idx = i; break }
    }
    if (idx >= 0) {
      const updated = { ...parts[idx] }
      fn(updated)
      parts[idx] = updated
      msg.parts = parts
    }
  }

  function scrollBottom() {
    nextTick(() => {
      const el = document.querySelector('.ai-messages')
      if (el) el.scrollTop = el.scrollHeight
    })
  }

  async function getUserIdForPlatform(platform: 'aliyun' | 'quark'): Promise<{ userId: string; driveId: string } | null> {
    const users = await UserDAL.GetUserListFromDB()
    for (const u of users) {
      if (!u?.user_id || !u?.access_token) continue
      if (platform === 'quark' && u.tokenfrom === 'quark') return { userId: u.user_id, driveId: 'quark' }
      if (platform === 'aliyun' && u.tokenfrom === 'aliyun') return { userId: u.user_id, driveId: u.default_drive_id || '' }
    }
    return null
  }

  async function sendMessage(text: string) {
    const kw = text.trim()
    if (!kw || loading.value) return

    const config = getAIConfig()
    if (!config) return

    if (abortController) { abortController.abort(); abortController = null }

    const userMsgId = `${Date.now()}-u`
    const aiMsgId = `${Date.now()}-a`

    messages.value = [...messages.value, { id: userMsgId, role: 'user', parts: [{ type: 'text', text: kw }] }]
    messages.value = [...messages.value, { id: aiMsgId, role: 'assistant', parts: [] }]
    streamingMessageId.value = aiMsgId
    loading.value = true
    saveHistory(messages.value)
    scrollBottom()

    try {
      const isOpenAI = config.providerName === 'openai' || config.providerName === 'ai-gateway'
      const model = isOpenAI
        ? createOpenAI({ name: config.providerName || 'openai', apiKey: config.apiKey, baseURL: config.endpoint })(config.modelId)
        : createOpenAICompatible({ name: config.providerName, apiKey: config.apiKey, baseURL: config.endpoint })(config.modelId)

      const apiMessages = messages.value
        .filter(m => m.role === 'user' || m.role === 'assistant')
        .map(m => ({
          role: m.role as 'user' | 'assistant',
          content: m.parts.filter(p => p.type === 'text').map(p => (p as any).text).join('\n'),
        }))

      const result = streamText({
        model,
        system: `你是 BoxPlayer 智能搜索助手。你必须通过调用工具来完成任务，禁止凭空编造文件信息。

## 你的工具
- listDrives: 列出用户所有已登录的网盘
- searchMyFiles: 搜索用户所有云盘中的文件
- searchPanHub: 搜索全网公开网盘分享链接
- findDuplicates: 扫描云盘查找重复文件
- analyzeStorage: 分析存储空间，找大文件和旧文件
- categorizeFiles: 按类型分类文件，提供整理方案
- importShare: 导入阿里云盘/夸克分享链接，转存到用户网盘
- downloadFiles: 添加文件下载任务
- moveFiles: 移动文件到指定目录（需用户确认）
- deleteFiles: 删除文件移入回收站（需用户确认）

## 核心规则（必须遵守）
1. 用户提到文件相关操作，必须调用对应工具，不能只回复文字
2. 禁止在未调用工具的情况下编造文件名、大小等信息
3. 多网盘场景必须先调用 listDrives：
   - 用户要求整理/分析/查重时，先调用 listDrives 列出所有网盘
   - 然后在聊天中让用户选择要操作哪个网盘还是全部
   - 用户指定后再执行操作
   - 导入分享：必须问用户保存到阿里云盘还是夸克
4. 工具返回结果后，简要总结即可
5. moveFiles 和 deleteFiles 必须先展示确认信息
6. 完全无关的问题可以正常简短回复
7. 最多调用工具 5 次

## 回复格式
每次回复必须先写一行【思考】说明你的分析和决策理由，然后再执行操作或给出结论。
例如：
【思考】用户想整理文件。需要先调用 listDrives 列出所有网盘让用户选择。
然后调用对应工具。`,
        messages: apiMessages,
        tools: {
          listDrives: {
            description: '列出用户所有已登录的网盘',
            inputSchema: z.object({}),
            execute: async () => {
              const users = await UserDAL.GetUserListFromDB()
              const drives = users
                .filter(u => !!u?.user_id && !!u?.access_token)
                .map(u => ({
                  userId: u.user_id,
                  name: u.nick_name || u.user_name || u.name || u.user_id,
                  platform: u.tokenfrom || 'aliyun',
                  driveId: u.default_drive_id || u.drive_id || '',
                }))
              return { count: drives.length, drives }
            },
          },

          searchMyFiles: {
            description: '搜索用户所有已登录云盘中的文件',
            inputSchema: z.object({ keyword: z.string().describe('搜索关键词') }),
            execute: async (args: any) => {
              const keyword = args.keyword
              appendPart(aiMsgId, {
                type: 'tool-searchMyFiles',
                state: 'running',
                input: { keyword },
              } as MessagePart)
              scrollBottom()

              try {
                const r = await searchAllDrives(keyword)
                const files: FileResult[] = r.slice(0, 30).map((f: GlobalSearchResult) => ({
                  name: f.name, ext: f.ext, size: f.size, isDir: f.isDir,
                  provider: f.provider, providerName: f.providerName,
                  driveId: f.drive_id, fileId: f.file_id,
                  parentFileId: f.parent_file_id, userId: f.user_id, source: f.source,
                }))
                updateToolPart(aiMsgId, 'tool-searchMyFiles', { keyword }, (part: any) => {
                  part.state = 'done'
                  part.output = { total: r.length, files }
                })
                scrollBottom()
                return { total: r.length, files }
              } catch (e: any) {
                updateToolPart(aiMsgId, 'tool-searchMyFiles', { keyword }, (part: any) => {
                  part.state = 'error'
                  part.error = e?.message || '搜索失败'
                })
                scrollBottom()
                return { total: 0, files: [], error: e?.message }
              }
            },
          },
          searchPanHub: {
            description: '搜索全网公开网盘分享链接',
            inputSchema: z.object({ keyword: z.string().describe('搜索关键词') }),
            execute: async (args: any) => {
              const keyword = args.keyword
              appendPart(aiMsgId, {
                type: 'tool-searchPanHub',
                state: 'running',
                input: { keyword },
              } as MessagePart)
              scrollBottom()

              try {
                const resp = await fetch(
                  `https://searchdrive.vercel.app/api/search?kw=${encodeURIComponent(keyword)}&res=merged_by_type&src=all`
                )
                const d = await resp.json()
                if (d?.code === 0 && d?.data?.merged_by_type) {
                  const all: LinkResult[] = []
                  for (const [, items] of Object.entries(d.data.merged_by_type as Record<string, any[]>)) {
                    all.push(...items.map((i: any) => ({
                      type: i.type || '', url: i.url || '', note: i.note || '', password: i.password || '',
                    })))
                  }
                  updateToolPart(aiMsgId, 'tool-searchPanHub', { keyword }, (part: any) => {
                    part.state = 'done'
                    part.output = { total: all.length, links: all }
                  })
                  scrollBottom()
                  return { total: all.length, links: all.slice(0, 30) }
                }
                updateToolPart(aiMsgId, 'tool-searchPanHub', { keyword }, (part: any) => {
                  part.state = 'done'
                  part.output = { total: 0, links: [] }
                })
                scrollBottom()
                return { total: 0, links: [] }
              } catch (e: any) {
                updateToolPart(aiMsgId, 'tool-searchPanHub', { keyword }, (part: any) => {
                  part.state = 'error'
                  part.error = e?.message || '搜索失败'
                })
                scrollBottom()
                return { total: 0, links: [], error: e?.message }
              }
            },
          },
          importShare: {
            description: '导入分享链接，将阿里云盘或夸克网盘分享的文件转存到用户网盘',
            inputSchema: z.object({
              url: z.string().describe('分享链接 URL'),
              password: z.string().optional().describe('提取码'),
            }),
            execute: async (args: any) => {
              const { url, password } = args
              const isQuark = /pan\.quark\.cn\/s\//.test(url)
              const isAliyun = /(aliyundrive|alipan)\.com\/s\//.test(url)
              if (!isQuark && !isAliyun) {
                appendPart(aiMsgId, { type: 'tool-importShare', state: 'error', input: { url, password: password || '' }, error: '仅支持阿里云盘和夸克网盘的分享链接' } as MessagePart)
                scrollBottom()
                return { error: 'unsupported platform' }
              }
              appendPart(aiMsgId, { type: 'tool-importShare', state: 'parsing', input: { url, password: password || '' } } as MessagePart)
              scrollBottom()
              const platform = isQuark ? 'quark' : 'aliyun'
              const account = await getUserIdForPlatform(platform)
              if (!account) {
                updateToolPart(aiMsgId, 'tool-importShare', { url }, (p: any) => { p.state = 'error'; p.error = `未登录${platform === 'quark' ? '夸克' : '阿里云'}盘` })
                scrollBottom()
                return { error: 'no account' }
              }
              try {
                let shareToken: string; let shareId: string
                if (isQuark) {
                  const parsed = parseQuarkShareLink(url + (password ? ` 提取码:${password}` : ''))
                  if (!parsed.id) throw new Error('解析夸克分享链接失败')
                  shareId = parsed.id.replace('quark:', '')
                  const { apiQuarkShareToken } = await import('../../quark/share')
                  shareToken = await apiQuarkShareToken(shareId, password || '')
                } else {
                  shareId = url.split(/\.com\/s\/([\w]+)/)[1]
                  shareToken = await AliShare.ApiGetShareToken(shareId, password || '')
                  if (!shareToken || shareToken.startsWith('，')) throw new Error('获取分享token失败')
                }
                updateToolPart(aiMsgId, 'tool-importShare', { url }, (p: any) => { p.state = 'listing' })
                scrollBottom()
                const fileResp = isQuark
                  ? await (await import('../../quark/share')).apiQuarkShareFileList(shareId, shareToken, 'root')
                  : await AliShare.ApiShareFileList(shareId, shareToken, 'root')
                const files = fileResp?.items || []
                if (!files.length) {
                  updateToolPart(aiMsgId, 'tool-importShare', { url }, (p: any) => { p.state = 'done'; p.output = { shareName: '', fileCount: 0, savedCount: 0, platform: platform === 'quark' ? '夸克网盘' : '阿里云盘' } })
                  return { savedCount: 0, fileCount: 0 }
                }
                const fileIds = files.map((f: any) => f.file_id)
                updateToolPart(aiMsgId, 'tool-importShare', { url }, (p: any) => { p.state = 'saving' })
                scrollBottom()
                const result = await AliShare.ApiSaveShareFilesBatch(shareId, shareToken, account.userId, account.driveId, 'root', fileIds)
                updateToolPart(aiMsgId, 'tool-importShare', { url }, (p: any) => {
                  p.state = result === 'success' ? 'done' : 'error'
                  p.output = { shareName: (fileResp as any)?.share_name || files[0]?.name || '', fileCount: files.length, savedCount: result === 'success' ? files.length : 0, platform: platform === 'quark' ? '夸克网盘' : '阿里云盘' }
                  if (result !== 'success') p.error = result
                })
                scrollBottom()
                return { savedCount: result === 'success' ? files.length : 0, fileCount: files.length, shareName: (fileResp as any)?.share_name || '' }
              } catch (e: any) {
                updateToolPart(aiMsgId, 'tool-importShare', { url }, (p: any) => { p.state = 'error'; p.error = e?.message || '导入失败' })
                scrollBottom()
                return { error: e?.message }
              }
            },
          },

          downloadFiles: {
            description: '添加文件下载任务',
            inputSchema: z.object({ files: z.array(z.object({ name: z.string(), fileId: z.string(), driveId: z.string(), userId: z.string() })) }),
            execute: async (args: any) => {
              const files = args.files || []
              if (!files.length) return { total: 0, success: 0 }
              appendPart(aiMsgId, { type: 'tool-downloadFiles', state: 'running', input: { files } } as MessagePart)
              scrollBottom()
              try {
                const { default: DownDAL } = await import('../../down/DownDAL')
                const models = files.map((f: any) => ({ user_id: f.userId, drive_id: f.driveId, file_id: f.fileId, file_name: f.name, parent_file_id: 'root', size: 0, ext: '', category: 'other', icon: '', thumbnail: '', description: '', encType: '', password: '' }))
                DownDAL.aAddDownload(models as any, '', false)
                updateToolPart(aiMsgId, 'tool-downloadFiles', {}, (p: any) => { p.state = 'done'; p.output = { total: files.length, success: files.length } })
                scrollBottom()
                return { total: files.length, success: files.length }
              } catch (e: any) {
                updateToolPart(aiMsgId, 'tool-downloadFiles', {}, (p: any) => { p.state = 'error'; p.error = e?.message || '添加下载失败' })
                scrollBottom()
                return { error: e?.message }
              }
            },
          },

          findDuplicates: {
            description: '扫描所有云盘查找重复文件',
            inputSchema: z.object({}),
            execute: async () => {
              appendPart(aiMsgId, { type: 'tool-findDuplicates', state: 'scanning' } as MessagePart)
              scrollBottom()
              try {
                const allFiles = await searchAllDrives('')
                const map = new Map<string, FileResult[]>()
                for (const f of allFiles) {
                  if (f.isDir) continue
                  const key = `${f.name}::${f.size}`
                  if (!map.has(key)) map.set(key, [])
                  map.get(key)!.push({ name: f.name, ext: f.ext, size: f.size, isDir: false, provider: f.provider, providerName: f.providerName, driveId: f.drive_id, fileId: f.file_id, parentFileId: f.parent_file_id, userId: f.user_id, source: f.source })
                }
                const groups = Array.from(map.entries()).filter(([, files]) => files.length > 1).map(([key, files]) => ({ name: key.split('::')[0], size: Number(key.split('::')[1]), files })).sort((a, b) => b.size * (b.files.length - 1) - a.size * (a.files.length - 1)).slice(0, 20)
                updateToolPart(aiMsgId, 'tool-findDuplicates', {}, (p: any) => { p.state = 'done'; p.output = { totalFiles: allFiles.length, groups } })
                scrollBottom()
                return { totalFiles: allFiles.length, groupCount: groups.length }
              } catch (e: any) {
                updateToolPart(aiMsgId, 'tool-findDuplicates', {}, (p: any) => { p.state = 'error'; p.error = e?.message || '扫描失败' })
                scrollBottom()
                return { error: e?.message }
              }
            },
          },

          analyzeStorage: {
            description: '分析存储空间使用情况',
            inputSchema: z.object({}),
            execute: async () => {
              appendPart(aiMsgId, { type: 'tool-analyzeStorage', state: 'scanning' } as MessagePart)
              scrollBottom()
              try {
                const allFiles = await searchAllDrives('')
                const driveMap = new Map<string, { totalSize: number; count: number; files: FileResult[] }>()
                for (const f of allFiles) {
                  const key = f.providerName
                  if (!driveMap.has(key)) driveMap.set(key, { totalSize: 0, count: 0, files: [] })
                  const d = driveMap.get(key)!
                  d.totalSize += f.size; d.count++
                  d.files.push({ name: f.name, ext: f.ext, size: f.size, isDir: f.isDir, provider: f.provider, providerName: f.providerName, driveId: f.drive_id, fileId: f.file_id, parentFileId: f.parent_file_id, userId: f.user_id, source: f.source })
                }
                const drives = Array.from(driveMap.entries()).map(([name, d]) => ({ name, totalSize: d.totalSize, fileCount: d.count, topLarge: [...d.files].sort((a, b) => b.size - a.size).slice(0, 10) }))
                const oldestFiles: FileResult[] = []
                updateToolPart(aiMsgId, 'tool-analyzeStorage', {}, (p: any) => { p.state = 'done'; p.output = { drives, oldestFiles, unusedFiles: [] } })
                scrollBottom()
                return { drives: drives.length, totalFiles: allFiles.length }
              } catch (e: any) {
                updateToolPart(aiMsgId, 'tool-analyzeStorage', {}, (p: any) => { p.state = 'error'; p.error = e?.message || '分析失败' })
                scrollBottom()
                return { error: e?.message }
              }
            },
          },

          categorizeFiles: {
            description: '分析文件类型分布并提供分类整理方案',
            inputSchema: z.object({}),
            execute: async () => {
              appendPart(aiMsgId, { type: 'tool-categorizeFiles', state: 'planning' } as MessagePart)
              scrollBottom()
              try {
                const allFiles = await searchAllDrives('')
                const catMap: Record<string, { exts: string[]; count: number; size: number }> = {
                  '视频': { exts: ['mp4','mkv','avi','mov','wmv','flv','webm'], count: 0, size: 0 },
                  '文档': { exts: ['pdf','doc','docx','txt','md','xls','xlsx','ppt','pptx'], count: 0, size: 0 },
                  '音频': { exts: ['mp3','flac','wav','aac','ogg','m4a'], count: 0, size: 0 },
                  '图片': { exts: ['jpg','jpeg','png','gif','bmp','webp','svg'], count: 0, size: 0 },
                  '压缩包': { exts: ['zip','rar','7z','tar','gz'], count: 0, size: 0 },
                  '其他': { exts: [], count: 0, size: 0 },
                }
                for (const f of allFiles) {
                  if (f.isDir) continue
                  const ext = (f.ext || '').toLowerCase()
                  let found = false
                  for (const [name, cat] of Object.entries(catMap)) {
                    if (name === '其他') continue
                    if (cat.exts.includes(ext)) { cat.count++; cat.size += f.size; found = true; break }
                  }
                  if (!found) { catMap['其他'].count++; catMap['其他'].size += f.size }
                }
                const categories = Object.entries(catMap).filter(([, c]) => c.count > 0).map(([name, c]) => ({ name, pattern: c.exts.slice(0, 5).join(', ') + (c.exts.length > 5 ? '…' : ''), fileCount: c.count, totalSize: c.size }))
                updateToolPart(aiMsgId, 'tool-categorizeFiles', {}, (p: any) => { p.state = 'done'; p.output = { categories } })
                scrollBottom()
                return { categoryCount: categories.length }
              } catch (e: any) {
                updateToolPart(aiMsgId, 'tool-categorizeFiles', {}, (p: any) => { p.state = 'error'; p.error = e?.message || '分析失败' })
                scrollBottom()
                return { error: e?.message }
              }
            },
          },

          moveFiles: {
            description: '移动文件到指定目录（需要用户确认）',
            inputSchema: z.object({ files: z.array(z.object({ name: z.string(), fileId: z.string(), driveId: z.string(), userId: z.string() })), targetDir: z.string() }),
            execute: async (args: any) => {
              const { files, targetDir } = args
              if (!files?.length) return { total: 0, success: 0 }
              appendPart(aiMsgId, { type: 'tool-moveFiles', state: 'confirm', input: { files, targetDir } } as MessagePart)
              scrollBottom()
              return { pending: true }
            },
          },

          deleteFiles: {
            description: '删除文件（需要用户确认，移入回收站）',
            inputSchema: z.object({ files: z.array(z.object({ name: z.string(), fileId: z.string(), driveId: z.string(), userId: z.string() })) }),
            execute: async (args: any) => {
              const { files } = args
              if (!files?.length) return { total: 0, success: 0 }
              appendPart(aiMsgId, { type: 'tool-deleteFiles', state: 'confirm', input: { files } } as MessagePart)
              scrollBottom()
              return { pending: true }
            },
          },
        },
        stopWhen: stepCountIs(5),
        temperature: 0.7,
      })

      // stream text delta, split reasoning from text
      let textPart: any = null
      let reasoningPart: any = null
      for await (const part of result.fullStream) {
        if (part.type === 'text-delta') {
          const delta = part.text
          // detect reasoning block: 【思考】... or 【分析】...
          const reasoningMatch = delta.match(/【(思考|分析)】([\\s\\S]*)/)
          if (reasoningMatch && !reasoningPart) {
            reasoningPart = { type: 'reasoning', text: reasoningMatch[2] }
            appendPart(aiMsgId, reasoningPart)
            const afterReasoning = delta.slice(reasoningMatch[0].length).trim()
            if (afterReasoning) {
              textPart = { type: 'text', text: afterReasoning }
              appendPart(aiMsgId, textPart)
            }
          } else if (reasoningPart && !textPart) {
            reasoningPart.text += delta
          } else {
            if (!textPart) {
              textPart = { type: 'text', text: '' }
              appendPart(aiMsgId, textPart)
            }
            textPart.text += delta
          }
          scrollBottom()
        }
      }
    } catch (e: any) {
      if (e?.name === 'AbortError') return
      appendPart(aiMsgId, {
        type: 'text',
        text: `\n\n❌ ${e?.message || 'AI 请求失败'}`,
      })
    } finally {
      loading.value = false
      streamingMessageId.value = ''
      saveHistory(messages.value)
    }
  }

  function clear() {
    if (abortController) { abortController.abort(); abortController = null }
    messages.value = []
    loading.value = false
    streamingMessageId.value = ''
    localStorage.removeItem(CHAT_KEY)
  }

  async function confirmAction(msgId: string, partIndex: number) {
    const msg = messages.value.find(m => m.id === msgId)
    if (!msg) return
    const part = msg.parts[partIndex] as any
    if (!part || (part.type !== 'tool-moveFiles' && part.type !== 'tool-deleteFiles')) return
    if (part.state !== 'confirm') return
    part.state = 'running'
    scrollBottom()
    try {
      const { files, targetDir } = part.input || {}
      const userId = files[0]?.userId
      const driveId = files[0]?.driveId
      const fileIds = files.map((f: any) => f.fileId)
      if (part.type === 'tool-moveFiles') {
        const result = await AliFileCmd.ApiMoveBatch(userId, driveId, fileIds, driveId, targetDir || 'root')
        const failed = result?.length || 0
        part.state = 'done'
        part.output = { total: files.length, success: files.length - failed, failed }
      } else {
        const result = await AliFileCmd.ApiDeleteBatch(userId, driveId, fileIds)
        const failed = result?.length || 0
        part.state = 'done'
        part.output = { total: files.length, success: files.length - failed, failed }
      }
    } catch (e: any) {
      part.state = 'error'
      part.error = e?.message || '操作失败'
    }
    scrollBottom()
    saveHistory(messages.value)
  }

  function cancelAction(msgId: string, partIndex: number) {
    const msg = messages.value.find(m => m.id === msgId)
    if (!msg) return
    const part = msg.parts[partIndex] as any
    if (part && (part.type === 'tool-moveFiles' || part.type === 'tool-deleteFiles') && part.state === 'confirm') {
      part.state = 'done'
      part.output = { total: part.input?.files?.length || 0, success: 0, failed: 0 }
    }
    saveHistory(messages.value)
  }

  return { messages, loading, streamingMessageId, sendMessage, clear, confirmAction, cancelAction }
}
