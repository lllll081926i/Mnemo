<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { login, saveMounted, SendGuangyaSms, providerIconUrl, OpenBrowser } from '../api'
import UiIcon from './UiIcon.vue'

const props = defineProps({ providers: { type: Array, default: () => [] } })
const emit = defineEmits(['close', 'toast'])

const providerId = ref(localStorage.getItem('login_provider') || 'pikpak')
const form = ref({})
const mountedForm = ref({ name: '', endpoint: '', username: '', password: '', bucket: '', region: '', rootPath: '', basePath: '', sessionToken: '', forcePathStyle: true })
const busy = ref(false)
const smsBusy = ref(false)
const smsCountdown = ref(0)
let smsTimer = null
const pikpakCooldownSeconds = ref(0)
let pikpakCooldownTimer = null
function startSmsCountdown() {
  smsCountdown.value = 60
  if (smsTimer) clearInterval(smsTimer)
  smsTimer = setInterval(() => {
    smsCountdown.value--
    if (smsCountdown.value <= 0) {
      clearInterval(smsTimer)
      smsTimer = null
    }
  }, 1000)
}
function clearPikPakCooldown() {
  if (pikpakCooldownTimer) clearInterval(pikpakCooldownTimer)
  pikpakCooldownTimer = null
  pikpakCooldownSeconds.value = 0
}
function startPikPakCooldown(seconds) {
  clearPikPakCooldown()
  pikpakCooldownSeconds.value = Math.max(30, Math.ceil(Number(seconds) || 0))
  pikpakCooldownTimer = setInterval(() => {
    pikpakCooldownSeconds.value--
    if (pikpakCooldownSeconds.value <= 0) clearPikPakCooldown()
  }, 1000)
}
function handlePikPakRateLimit(error) {
  const text = String(error || '')
  if (!/(?:频繁|too[ _-]*(?:many|frequent)|rate[ _-]*limit|request[ _-]*frequency|429)/i.test(text)) return false
  const match = text.match(/(?:等待|wait(?:ing)?(?:\s+for)?)\s*(\d+)\s*(?:秒|second)?/i)
  startPikPakCooldown(match ? Number(match[1]) : 30)
  errorText.value = `PikPak 暂时限制了登录请求，请等待 ${pikpakCooldownSeconds.value} 秒后再试`
  return true
}
const errorText = ref('')

const availableProviders = computed(() => props.providers.filter((p) => p.ID !== 'yike'))
const provider = computed(() => availableProviders.value.find((p) => p.ID === providerId.value) || availableProviders.value[0] || null)
const fields = computed(() => (provider.value && provider.value.Login && provider.value.Login.fields) || [])
const isMounted = computed(() => providerId.value === 'webdav' || providerId.value === 's3')
const isOAuthField = (field) => field.type === 'oauth'
const isCookieField = (field) => /cookie|cookies|bduss/i.test(`${field.key} ${field.label}`)
const isAccountField = (field) => /^(?:username|password|phone|email|sms_code)$/i.test(field.key)
const hasAccountLogin = computed(() => fields.value.some(isAccountField))
const hasPhoneLogin = computed(() => fields.value.some((field) => field.key === 'phone' || /手机/.test(field.label)))
const hasEmailLogin = computed(() => fields.value.some((field) => field.key === 'email' || /邮箱/.test(field.label)))
const isOAuth = computed(() => !isMounted.value && !hasAccountLogin.value && fields.value.length > 0 && fields.value.every(isOAuthField))
const visibleFields = computed(() => {
  const nonOAuthFields = fields.value.filter((field) => !isOAuthField(field))
  return hasAccountLogin.value ? nonOAuthFields.filter((field) => !isCookieField(field)) : nonOAuthFields
})
const isTokenField = (field) => /token|authorization|secret/i.test(`${field.key} ${field.label}`)
const hasCookieLogin = computed(() => !hasAccountLogin.value && visibleFields.value.some(isCookieField))
const hasTokenLogin = computed(() => !hasAccountLogin.value && visibleFields.value.some(isTokenField))
const isLongText = (key) => /cookie|token|bduss|secret|authorization/i.test(key)
const isPan139DirectLogin = computed(() => providerId.value === 'pan139' && String(form.value.authorization || '').trim() !== '')
const hasRefreshToken = computed(() => String(form.value.refresh_token || '').trim() !== '')
const loginSubtitle = computed(() => {
  if (isMounted.value) return '连接设置'
  if (isOAuth.value) return '浏览器授权'
  if (hasPhoneLogin.value && hasEmailLogin.value) return '手机号/邮箱登录'
  if (hasPhoneLogin.value) return '手机号登录'
  if (hasEmailLogin.value) return '邮箱登录'
  if (hasAccountLogin.value) return '账号密码登录'
  if (hasCookieLogin.value) return 'Cookie 登录'
  if (hasTokenLogin.value) return '令牌登录'
  return '登录设置'
})
function isFieldRequired(field) {
  if (isPan139DirectLogin.value && (field.key === 'username' || field.key === 'password')) return false
  return field.required
}
function fieldInputType(field) {
  return field.type === 'password' ? 'password' : 'text'
}
function fieldInputMode(field) {
  if (/phone|sms_code/i.test(field.key)) return 'numeric'
  if (/email/i.test(field.key)) return 'email'
  return undefined
}

// Captcha state is initialized before the provider watcher. A stale saved
// provider can be corrected synchronously as soon as the provider list arrives.
const captchaUrl = ref('')
const captchaCompleted = ref(false)
const captchaFrameReady = ref(false)
const captchaSubmitting = ref(false)
const captchaFrame = ref(null)
const pan189Captcha = ref('')

watch(providerId, (v) => {
  localStorage.setItem('login_provider', v)
  form.value = {}
  mountedForm.value = { name: '', endpoint: '', username: '', password: '', bucket: '', region: '', rootPath: '', basePath: '', sessionToken: '', forcePathStyle: true }
  errorText.value = ''
  resetCaptcha()
  if (v !== 'pikpak') clearPikPakCooldown()
})

// A hidden or removed provider must not remain selected through an old
// localStorage value while the visible panel falls back to another item.
watch(availableProviders, (list) => {
  if (!list.length) return
  if (!list.some((p) => p.ID === providerId.value)) providerId.value = list[0].ID
}, { immediate: true })

function onKey(e) { if (e.key === 'Escape') emit('close') }
onMounted(() => {
  window.addEventListener('keydown', onKey)
  window.addEventListener('message', onCaptchaMessage)
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  window.removeEventListener('message', onCaptchaMessage)
  if (smsTimer) clearInterval(smsTimer)
  clearPikPakCooldown()
})

// 逐网盘附加帮助（后端 Login.fields 之外的引导）
const PROVIDER_HELP = {
  aliopen: { label: '如何获取 refresh_token？', url: 'https://alist.nn.ci/tool/aliyundrive/request' },
}
const providerHelp = computed(() => PROVIDER_HELP[providerId.value] || null)
function openHelp() { if (providerHelp.value) OpenBrowser(providerHelp.value.url).catch(() => {}) }

// PikPak 滑块验证
function findCaptchaToken(value, depth = 0) {
  if (value == null || depth > 5) return ''
  if (typeof value === 'string') {
    const text = value.trim()
    try {
      const parsed = JSON.parse(text)
      return findCaptchaToken(parsed, depth + 1)
    } catch {
      return text.length > 20 && /^[A-Za-z0-9._~+/=-]+$/.test(text) ? text : ''
    }
  }
  if (Array.isArray(value)) {
    for (const item of value) {
      const token = findCaptchaToken(item, depth + 1)
      if (token) return token
    }
    return ''
  }
  if (typeof value !== 'object') return ''
  for (const key of ['captcha_token', 'captchaToken', 'token']) {
    const token = String(value[key] || '').trim()
    if (token.length > 20) return token
  }
  for (const item of Object.values(value)) {
    if (item == null || (typeof item !== 'object' && typeof item !== 'string')) continue
    const token = findCaptchaToken(item, depth + 1)
    if (token) return token
  }
  return ''
}
function acceptCaptchaToken(token) {
  const value = String(token || '').trim()
  if (!value || value.length <= 20) return false
  form.value.captcha_token = value
  form.value.captcha_verified = 'true'
  captchaCompleted.value = true
  errorText.value = ''
  return true
}
function onCaptchaMessage(event) {
  if (providerId.value !== 'pikpak' || !captchaUrl.value) return
  const frame = captchaFrame.value
  if (!frame || event.source !== frame.contentWindow) return
  try {
    const challenge = new URL(captchaUrl.value)
    const source = new URL(event.origin)
    const trustedPikPakHost = /(^|\.)mypikpak\.(com|net)$/i.test(source.hostname)
    if (source.origin !== challenge.origin && !(source.protocol === 'https:' && trustedPikPakHost)) return
  } catch {
    return
  }
  const token = findCaptchaToken(event.data)
  if (acceptCaptchaToken(token)) void submit()
}
function parseCaptcha(err) {
  const m = String(err).match(/captcha_required\nurl=(\S+)\ntoken=(\S*)/)
  if (!m) return false
  captchaUrl.value = m[1]
  form.value.captcha_token = m[2]
  delete form.value.captcha_verified
  captchaCompleted.value = false
  captchaFrameReady.value = false
  return true
}
async function reloadCaptcha() {
  if (!captchaUrl.value || busy.value) return
  captchaCompleted.value = false
  captchaFrameReady.value = false
  captchaUrl.value = ''
  delete form.value.captcha_token
  delete form.value.captcha_verified
  await submit()
}

// 天翼云 189 图形验证码
function parse189Captcha(err) {
  const m = String(err).match(/captcha_required_189\nimage=(\S+)/)
  if (!m) return false
  pan189Captcha.value = m[1]
  form.value.validate_code = ''
  return true
}

function resetCaptcha() {
  captchaUrl.value = ''
  captchaCompleted.value = false
  captchaFrameReady.value = false
  captchaSubmitting.value = false
  pan189Captcha.value = ''
  delete form.value.captcha_token
  delete form.value.captcha_verified
  delete form.value.validate_code
}

async function sendSms() {
  if (!String(form.value.phone || '').trim()) { errorText.value = '请先填写手机号'; return }
  smsBusy.value = true
  errorText.value = ''
  try {
    const r = await SendGuangyaSms(form.value.phone)
    form.value.verification_id = r.verification_id
    form.value.device_id = r.device_id
    form.value.captcha_token = r.captcha_token || ''
    emit('toast', '验证码已发送', 'success')
    startSmsCountdown()
  } catch (e) {
    errorText.value = String(e)
  } finally {
    smsBusy.value = false
  }
}

function validate() {
  if (isMounted.value) {
    const m = mountedForm.value
    if (providerId.value === 'webdav' && !m.endpoint.trim()) return '请填写 WebDAV 地址'
    if (!m.username.trim()) return providerId.value === 's3' ? '请填写 Access Key ID' : '请填写用户名'
    if (!m.password) return providerId.value === 's3' ? '请填写 Secret Access Key' : '请填写密码'
    if (providerId.value === 's3' && !m.bucket.trim()) return '请填写 Bucket'
    return ''
  }
  if (isOAuth.value) return '' // OAuth 无表单校验
  const value = (key) => String(form.value[key] || '').trim()
  if (providerId.value === 'lanzou' && (!value('username') || !value('password'))) {
    return '请填写蓝奏云账号和密码'
  }
  if (providerId.value === 'guangya') {
    if (value('refresh_token')) return ''
    if (!value('phone')) return '请填写手机号'
    if (!value('sms_code')) return '请填写短信验证码'
    if (!value('verification_id')) return '请先获取短信验证码'
    return ''
  }
  if (providerId.value === 'pan189' && pan189Captcha.value && !value('validate_code')) {
    return '请填写图形验证码'
  }
  for (const f of visibleFields.value) {
    if (isFieldRequired(f) && !value(f.key)) return `请填写${f.label}`
  }
  // 全可选字段的 provider 至少填一项
  const allOpt = visibleFields.value.length > 0 && visibleFields.value.every((f) => !f.required)
  if (allOpt && !visibleFields.value.some((f) => value(f.key))) return '至少填写一个字段'
  return ''
}

async function submit() {
  if (busy.value) return
  if (providerId.value === 'pikpak' && pikpakCooldownSeconds.value > 0) {
    errorText.value = `PikPak 暂时限制了登录请求，请等待 ${pikpakCooldownSeconds.value} 秒后再试`
    return
  }
  const err = validate()
  if (err) { errorText.value = err; return }
  busy.value = true
  captchaSubmitting.value = true
  errorText.value = ''
  try {
    if (isMounted.value) {
      await saveMounted(providerId.value, { ...mountedForm.value })
    } else {
      await login(providerId.value, { ...form.value })
    }
    emit('toast', isOAuth.value ? '授权成功' : '登录成功', 'success')
    emit('close')
  } catch (e) {
    if (providerId.value === 'pikpak' && handlePikPakRateLimit(e)) {
      // Keep the challenge state intact while the provider cooldown runs.
    } else if (providerId.value === 'pikpak' && parseCaptcha(e)) {
      errorText.value = '请在登录窗口内完成安全验证'
    } else if (providerId.value === 'pan189' && parse189Captcha(e)) {
      errorText.value = '请输入图片中的验证码'
    } else {
      errorText.value = String(e)
    }
  }
  busy.value = false
  captchaSubmitting.value = false
}
</script>

<template>
  <teleport to="body">
    <transition name="modal-fade">
      <div class="modal-mask" @click.self="emit('close')">
        <div class="modal login-modal">
          <div class="modal-head login-modal-head">
            <h3>添加网盘账号</h3>
            <button class="icon-btn login-close" title="关闭 (Esc)" @click="emit('close')"><UiIcon name="close" :size="14" /></button>
          </div>
          <div class="login-body">
            <div class="login-side" role="tablist" aria-label="网盘服务">
              <button
                v-for="p in availableProviders"
                :key="p.ID"
                type="button"
                role="tab"
                class="lp-item"
                :class="{ active: p.ID === providerId }"
                :aria-selected="p.ID === providerId"
                :title="p.Meta.label"
                @click="providerId = p.ID"
              >
                <img :src="providerIconUrl(p.Meta)" alt="" />
                <span>{{ p.Meta.label }}</span>
              </button>
            </div>

            <form v-if="provider" class="login-form" @submit.prevent="submit">
              <div class="lf-head">
                <img :src="providerIconUrl(provider.Meta)" alt="" />
                <div>
                  <div class="lf-title">{{ provider.Meta.label }}</div>
                  <div class="lf-sub">{{ loginSubtitle }}</div>
                </div>
              </div>

              <div class="login-form-content">
                <!-- 挂载存储（WebDAV / S3） -->
                <template v-if="isMounted">
                  <div class="login-section">
                    <div class="field login-field"><label>连接名称</label><input class="input" v-model="mountedForm.name" placeholder="我的 WebDAV / S3" /></div>
                    <div class="field login-field"><label>{{ providerId === 's3' ? 'Endpoint (可选)' : 'WebDAV 地址' }}<span v-if="providerId !== 's3'" class="req">*</span></label><input class="input" v-model="mountedForm.endpoint" :placeholder="providerId === 's3' ? 's3.us-east-1.amazonaws.com (可选，默认 AWS)' : 'https://dav.example.com'" /></div>
                    <div class="field login-field"><label>{{ providerId === 's3' ? 'Access Key ID' : '用户名' }}<span class="req">*</span></label><input class="input" v-model="mountedForm.username" /></div>
                    <div class="field login-field"><label>{{ providerId === 's3' ? 'Secret Access Key' : '密码' }}<span class="req">*</span></label><input class="input" type="password" v-model="mountedForm.password" /></div>
                    <template v-if="providerId === 's3'">
                      <div class="field login-field"><label>Bucket<span class="req">*</span></label><input class="input" v-model="mountedForm.bucket" /></div>
                      <div class="field login-field"><label>Region (可选)</label><input class="input" v-model="mountedForm.region" placeholder="us-east-1" /></div>
                      <div class="field login-field"><label>Session Token (可选)</label><input class="input" type="password" v-model="mountedForm.sessionToken" /></div>
                      <div class="field login-field">
                        <label>路径风格</label>
                        <div class="switch-row">
                          <button class="switch" :class="{ on: mountedForm.forcePathStyle }" type="button" role="switch" :aria-checked="mountedForm.forcePathStyle" @click="mountedForm.forcePathStyle = !mountedForm.forcePathStyle"></button>
                          <span class="hint">开启用于 MinIO、OSS 等兼容服务；AWS S3 可关闭</span>
                        </div>
                      </div>
                    </template>
                    <div v-if="providerId === 'webdav'" class="field login-field"><label>根目录 (可选)</label><input class="input" v-model="mountedForm.rootPath" placeholder="/" /></div>
                    <div v-else class="field login-field"><label>挂载路径 (可选)</label><input class="input" v-model="mountedForm.basePath" placeholder="/" /></div>
                  </div>
                </template>

                <!-- OAuth 授权 -->
                <div v-else-if="isOAuth" class="login-state-card oauth-box">
                  <div class="login-state-icon"><UiIcon name="external" :size="24" /></div>
                  <div>
                    <strong>在浏览器中完成授权</strong>
                    <p>点击下方按钮打开授权页面，完成后会自动返回并登录。</p>
                  </div>
                </div>
                <!-- 常规表单 -->
                <template v-else>
                  <div v-if="visibleFields.length" class="login-section">
                    <div v-for="f in visibleFields" :key="f.key" class="field login-field">
                      <label>{{ f.label }}<span v-if="isFieldRequired(f)" class="req">*</span></label>
                      <textarea v-if="isLongText(f.key)" class="textarea" v-model="form[f.key]" :placeholder="f.placeholder || ''" rows="3"></textarea>
                      <input v-else class="input" :type="fieldInputType(f)" :inputmode="fieldInputMode(f)" v-model="form[f.key]" :placeholder="f.placeholder || ''" />
                      <div v-if="providerId === 'pan189' && f.key === 'validate_code' && pan189Captcha" class="captcha-image-row">
                        <img :src="pan189Captcha" alt="图形验证码" />
                        <span>请输入图片中的字符</span>
                      </div>
                      <div v-if="f.hint" class="hint">{{ f.hint }}</div>
                      <div v-if="providerId === 'guangya' && f.key === 'sms_code' && !hasRefreshToken" class="field-action-row">
                        <button class="btn sm" :disabled="smsBusy || smsCountdown > 0" type="button" @click="sendSms">{{ smsBusy ? '发送中…' : (smsCountdown > 0 ? smsCountdown + ' 秒后重发' : '获取验证码') }}</button>
                      </div>
                    </div>
                  </div>
                  <div v-else class="login-empty-state">该网盘无需填写表单，直接点击登录。</div>
                </template>

                <!-- 网盘专属帮助链接 -->
                <button v-if="providerHelp" type="button" class="login-help" @click="openHelp">
                  <UiIcon name="external" :size="12" /><span>{{ providerHelp.label }}</span>
                </button>

                <!-- PikPak 滑块安全验证 -->
                <div v-if="captchaUrl" class="login-state-card captcha-box">
                  <div class="captcha-head">
                    <div>
                      <strong>{{ captchaCompleted ? '验证已完成' : '请在下方完成安全验证' }}</strong>
                      <p>{{ captchaCompleted ? '已收到验证结果，正在继续登录' : (captchaFrameReady ? '完成验证后将自动继续登录；未自动继续时请点击下方按钮' : '正在加载验证页面…') }}</p>
                    </div>
                    <button class="btn sm" type="button" :disabled="captchaSubmitting || pikpakCooldownSeconds > 0" @click="reloadCaptcha">重新加载</button>
                  </div>
                  <iframe
                    class="captcha-frame"
                    :src="captchaUrl"
                    title="PikPak 安全验证"
                    ref="captchaFrame"
                    referrerpolicy="strict-origin-when-cross-origin"
                    allow="clipboard-read; clipboard-write"
                    @load="captchaFrameReady = true"
                  ></iframe>
                  <div class="captcha-actions">
                    <span v-if="captchaCompleted" class="captcha-state">已收到验证结果</span>
                    <button class="btn primary sm" type="button" :disabled="busy || pikpakCooldownSeconds > 0" @click="submit">{{ busy ? '登录中…' : (pikpakCooldownSeconds > 0 ? `${pikpakCooldownSeconds} 秒后重试` : (captchaCompleted ? '验证完成并登录' : '我已完成验证，继续登录')) }}</button>
                  </div>
                </div>

                <!-- 统一错误提示（带 Shake 抖动动画） -->
                <div v-if="errorText" class="form-error shake">
                  <UiIcon name="warning" :size="14" /><span>{{ errorText }}</span>
                </div>
              </div>

              <div class="modal-actions login-actions">
                <button class="btn" type="button" @click="emit('close')">取消</button>
                <button class="btn primary" type="submit" :disabled="busy || (providerId === 'pikpak' && pikpakCooldownSeconds > 0)">
                  <span v-if="busy" class="spin spin-on-primary"></span>
                  {{ busy ? '处理中…' : isOAuth ? '打开授权页面' : (isMounted ? '保存连接' : '登录') }}
                </button>
              </div>
            </form>
          </div>
        </div>
      </div>
    </transition>
  </teleport>
</template>

<style scoped>
.login-close { width: 28px; height: 28px; }
.login-side .lp-item {
  width: 100%; border: 0; background: transparent; font: inherit; text-align: left;
}
.login-side .lp-item:focus-visible,
.switch:focus-visible {
  outline: none; box-shadow: var(--ring-focus);
}
.login-form { display: flex; flex-direction: column; }
.login-form-content { display: grid; gap: 14px; }
.login-section { display: grid; gap: 14px; }
.login-field { margin: 0 !important; }
.login-field > label { font-weight: 600; }
.req { color: var(--color-error); margin-left: 2px; }
.switch-row { display: flex; align-items: center; gap: 9px; min-height: 24px; }
.switch { border: 0; padding: 0; }
.switch-row .hint { margin: 0; line-height: 1.45; }
.login-state-card {
  display: flex; align-items: flex-start; gap: 12px;
  padding: 16px; border: 1px solid var(--border-light);
  border-radius: var(--radius-sm); background: var(--bg-subtle);
  color: var(--text-secondary); font-size: 13px; line-height: 1.55;
}
.login-state-card strong { display: block; color: var(--text-primary); font-size: 13.5px; }
.login-state-card p { margin: 3px 0 0; }
.login-state-icon {
  display: inline-flex; align-items: center; justify-content: center;
  width: 34px; height: 34px; flex: 0 0 34px;
  border-radius: var(--radius-sm); background: var(--listselectbg); color: var(--color-primary);
}
.oauth-box { min-height: 82px; align-items: center; }
.field-action-row { display: flex; margin-top: 8px; }
.captcha-image-row {
  display: flex; align-items: center; gap: 10px; margin-top: 8px;
  color: var(--text-tertiary); font-size: 12px;
}
.captcha-image-row img {
  height: 36px; min-width: 96px; object-fit: contain;
  border: 1px solid var(--border-light); border-radius: var(--radius-xs); background: #fff;
}
.login-empty-state {
  padding: 14px; border: 1px dashed var(--border-light); border-radius: var(--radius-sm);
  color: var(--text-tertiary); font-size: 13px; line-height: 1.5;
}
.login-help {
  display: inline-flex; align-items: center; gap: 5px; justify-self: start;
  border: none; background: none; padding: 0;
  color: var(--color-primary); font-size: 13px; cursor: pointer;
}
.login-help:hover { text-decoration: underline; }
.captcha-box {
  display: block; padding: 12px;
  background: color-mix(in srgb, var(--color-warning) 8%, transparent);
  border-color: color-mix(in srgb, var(--color-warning) 28%, var(--border-light));
  font-size: 13px; color: var(--text-secondary); line-height: 1.6;
}
.captcha-head { display:flex; align-items:flex-start; justify-content:space-between; gap:12px; }
.captcha-head strong { display:block; color:var(--text-primary); font-size:13px; }
.captcha-head p { margin:3px 0 0; }
.captcha-frame {
  display:block; width:100%; height:320px; margin-top:10px;
  border:1px solid var(--border-light); border-radius:var(--radius-sm);
  background:#fff;
}
.captcha-actions { display:flex; align-items:center; justify-content:space-between; gap:8px; margin-top:10px; }
.captcha-state { color:var(--color-success); font-size:12px; }
.login-actions {
  flex-shrink: 0; margin-top: 16px; padding: 14px 0 0;
  border-top: 1px solid var(--border-lighter);
}
@media (max-width: 640px) {
  .login-side .lp-item { width: auto; text-align: center; }
  .login-state-card { padding: 14px; }
  .captcha-frame { height:280px; }
  .captcha-head { align-items:stretch; flex-direction:column; }
  .captcha-head .btn { align-self:flex-start; }
  .captcha-actions { align-items: stretch; flex-direction: column; }
  .captcha-actions .btn { width: 100%; }
}
</style>
