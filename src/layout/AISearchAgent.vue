<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'
import { Sparkles, User, Bot, Loader2 } from 'lucide-vue-next'
import { useAppStore } from '../store'
import { useAISearchChat } from './aisearch/useAISearchChat'
import ReasonChain from './aisearch/ReasonChain.vue'
import ClarifyCard from './aisearch/ClarifyCard.vue'
import SearchFilesCard from './aisearch/SearchFilesCard.vue'
import SearchLinksCard from './aisearch/SearchLinksCard.vue'
import SummaryCard from './aisearch/SummaryCard.vue'
import LoadingIndicator from './aisearch/LoadingIndicator.vue'
import FollowUpBar from './aisearch/FollowUpBar.vue'
import { renderMarkdown } from './aisearch/markdown'
import type { FileResult } from './aisearch/types'

const props = defineProps<{ aiEnabled: boolean; keyword: string; trigger: number; phSearch: (kw: string) => Promise<any> }>()

const appStore = useAppStore()
const { messages, loading, sendMessage, clear } = useAISearchChat(props.phSearch)
const chatContainer = ref<HTMLElement>()

function handleSend(q?: string) {
  const kw = (q || props.keyword || '').trim()
  if (!kw) return
  sendMessage(kw)
}

function handleClarifySelect(option: string) {
  handleSend(option)
}

function handleFollowUp(query: string) {
  handleSend(query)
}

function handleFileNavigate(file: FileResult) {
  appStore.toggleTab('pan')
  nextTick(async () => {
    const { default: usePanTreeStore } = await import('../pan/pantreestore')
    const { default: PanDAL } = await import('../pan/pandal')
    const { default: UserDAL } = await import('../user/userdal')
    const panTreeStore = usePanTreeStore()
    if (panTreeStore.user_id !== file.userId) await UserDAL.UserChange(file.userId)
    await nextTick()
    let fileId = file.parentFileId || file.fileId
    if (fileId === '/' || !fileId) {
      const map: Record<string, string> = {
        baidu: 'baidu_root', cloud123: 'cloud_root', '115': 'drive115_root',
        quark: 'quark_root', pikpak: 'pikpak_root', dropbox: 'dropbox_root',
        onedrive: 'onedrive_root', box: 'box_root',
      }
      fileId = map[file.provider] || fileId
    }
    PanDAL.aReLoadOneDirToShow(file.driveId || fileId, fileId, true)
  })
}

function handleRetryTool(_messageId: string, _toolType: string, input: any) {
  if (input?.keyword) handleSend(input.keyword)
}

watch(() => props.trigger, () => {
  if (props.keyword.trim()) handleSend()
})

const DEFAULT_FOLLOWUPS = [
  '帮我找科幻电影',
  '搜索 PDF 文档',
  '最近有什么新文件',
]
</script>

<template>
  <div class="ai-chat">
    <!-- empty state -->
    <div v-if="messages.length === 0 && !loading" class="ai-empty">
      <Sparkles :size="48" :stroke-width="1" class="ai-empty-icon" />
      <div class="ai-empty-title">AI 智能搜索</div>
      <div class="ai-empty-desc">在上方搜索框输入自然语言，AI 帮你搜索所有云盘和全网资源</div>
      <div class="ai-empty-hints">
        <button
          v-for="hint in DEFAULT_FOLLOWUPS"
          :key="hint"
          class="ai-hint"
          type="button"
          @click="handleSend(hint)"
        >
          {{ hint }}
        </button>
      </div>
    </div>

    <!-- messages -->
    <div v-else ref="chatContainer" class="ai-messages">
      <div
        v-for="msg in messages"
        :key="msg.id"
        class="ai-msg"
        :class="'ai-msg--' + msg.role"
      >
        <!-- avatar -->
        <div class="ai-msg-avatar">
          <User v-if="msg.role === 'user'" :size="16" :stroke-width="2" />
          <Bot v-else :size="16" :stroke-width="2" />
        </div>

        <!-- parts -->
        <div class="ai-msg-body">
          <template v-for="(part, pi) in msg.parts" :key="pi">
            <!-- text -->
            <div v-if="part.type === 'text'" class="ai-msg-text" v-html="renderMarkdown((part as any).text)" />

            <!-- reasoning -->
            <ReasonChain v-else-if="part.type === 'reasoning'" :text="(part as any).text" />

            <!-- clarification -->
            <ClarifyCard
              v-else-if="part.type === 'clarification'"
              :question="(part as any).question"
              :options="(part as any).options"
              @select="handleClarifySelect"
            />

            <!-- tool: searchMyFiles -->
            <SearchFilesCard
              v-else-if="part.type === 'tool-searchMyFiles'"
              :state="(part as any).state"
              :input="(part as any).input"
              :output="(part as any).output"
              :error="(part as any).error"
              @navigate="handleFileNavigate"
              @retry="handleRetryTool(msg.id, 'tool-searchMyFiles', (part as any).input)"
            />

            <!-- tool: searchPanHub -->
            <SearchLinksCard
              v-else-if="part.type === 'tool-searchPanHub'"
              :state="(part as any).state"
              :input="(part as any).input"
              :output="(part as any).output"
              :error="(part as any).error"
              @retry="handleRetryTool(msg.id, 'tool-searchPanHub', (part as any).input)"
            />

            <!-- summary -->
            <SummaryCard
              v-else-if="part.type === 'summary'"
              :text="(part as any).text"
              :followups="(part as any).followups"
              @followup="handleFollowUp"
            />
          </template>
        </div>
      </div>

      <!-- streaming indicator -->
      <LoadingIndicator v-if="loading" />
    </div>

    <!-- follow-up bar -->
    <FollowUpBar
      v-if="messages.length > 0 && !loading"
      :suggestions="DEFAULT_FOLLOWUPS"
      @select="handleFollowUp"
    />

    <!-- footer -->
    <div v-if="messages.length > 0" class="ai-footer">
      <button class="ai-clear-btn" type="button" @click="clear">清空对话</button>
      <span class="ai-msg-count">{{ messages.filter(m => m.role === 'user').length }} 轮对话</span>
    </div>
  </div>
</template>

<style scoped>
.ai-chat { display: flex; flex-direction: column; height: 100%; background: var(--color-bg-1); }

.ai-empty { display: flex; flex-direction: column; align-items: center; justify-content: center; flex: 1; padding: 40px 24px; text-align: center; }
.ai-empty-icon { color: var(--color-text-4); opacity: 0.3; margin-bottom: 12px; }
.ai-empty-title { font-size: 18px; font-weight: 600; color: var(--color-text-2); margin-bottom: 6px; }
.ai-empty-desc { font-size: 13px; color: var(--color-text-4); margin-bottom: 20px; }
.ai-empty-hints { display: flex; flex-wrap: wrap; gap: 8px; justify-content: center; }
.ai-hint { padding: 6px 14px; font-size: 13px; color: var(--color-text-3); background: var(--color-fill-1); border: 1px solid var(--color-border-2); border-radius: 16px; cursor: pointer; transition: all 0.15s; font-family: inherit; }
.ai-hint:hover { color: rgb(var(--primary-6)); border-color: rgb(var(--primary-6)); }

.ai-messages { flex: 1; overflow-y: auto; padding: 16px 48px; display: flex; flex-direction: column; gap: 16px; }
.ai-msg { display: flex; gap: 10px; max-width: 85%; }
.ai-msg--user { align-self: flex-end; flex-direction: row-reverse; }
.ai-msg--assistant { align-self: flex-start; }
.ai-msg-avatar { display: flex; align-items: center; justify-content: center; width: 28px; height: 28px; border-radius: 8px; flex-shrink: 0; }
.ai-msg--user .ai-msg-avatar { background: rgb(var(--primary-6)); color: #fff; }
.ai-msg--assistant .ai-msg-avatar { background: var(--color-fill-2); color: var(--color-text-3); }
.ai-msg-body { min-width: 0; flex: 1; }
.ai-msg--user .ai-msg-body { text-align: right; }
.ai-msg-text { font-size: 14px; line-height: 1.65; color: var(--color-text-1); word-break: break-word; }
.ai-msg--user .ai-msg-text { background: rgb(var(--primary-6)); color: #fff; padding: 10px 14px; border-radius: 12px 12px 4px 12px; display: inline-block; text-align: left; max-width: 100%; }
.ai-msg--assistant .ai-msg-text { padding: 4px 0; }
.ai-msg-text :deep(strong) { font-weight: 600; }
.ai-msg-text :deep(code) { padding: 1px 4px; font-size: 12px; background: var(--color-fill-2); border-radius: 3px; }
.ai-msg-text :deep(a) { color: rgb(var(--primary-6)); }

.ai-footer { display: flex; align-items: center; justify-content: space-between; padding: 8px 48px; border-top: 1px solid var(--color-border-2); background: var(--color-bg-1); }
.ai-clear-btn { padding: 4px 12px; font-size: 12px; color: var(--color-text-4); background: transparent; border: 0; border-radius: 4px; cursor: pointer; font-family: inherit; }
.ai-clear-btn:hover { color: rgb(var(--danger-6)); background: var(--color-fill-2); }
.ai-msg-count { font-size: 12px; color: var(--color-text-4); }

@media (max-width: 720px) { .ai-messages { padding: 16px; } .ai-footer { padding: 8px 16px; } }
</style>
