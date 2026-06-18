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
    const idx = parts.findLastIndex(
      p => p.type === toolType && (p as any).input?.keyword === input?.keyword
    )
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
        system: `你是 BoxPlayer 智能搜索助手。帮用户搜索云盘文件和全网资源。

## 你的能力
- searchMyFiles: 搜索用户已登录的所有云盘
- searchPanHub: 搜索全网公开网盘分享链接

## 工作流程
1. 先分析用户意图，如果查询太模糊（如"找东西"、"帮我搜"），反问用户澄清需求
2. 制定搜索策略：决定调用哪些工具、使用什么关键词
3. 执行搜索，综合两个工具的结果给出总结

## 规则
- 用中文回复
- 列出文件名、大小、来源
- 无关搜索的问题正常简短回答
- 最多搜索 5 次（避免过度搜索）
- 结果有文件时，优先展示具体文件信息而非泛泛描述`,
        messages: apiMessages,
        tools: {
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
        },
        stopWhen: stepCountIs(5),
        temperature: 0.7,
      })

      // stream text delta into a text part
      let textPart: any = null
      for await (const part of result.fullStream) {
        if (part.type === 'text-delta') {
          if (!textPart) {
            textPart = { type: 'text', text: '' }
            appendPart(aiMsgId, textPart)
          }
          textPart.text += part.textDelta
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

  return { messages, loading, streamingMessageId, sendMessage, clear }
}
