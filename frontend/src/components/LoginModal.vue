<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { login, saveMounted, SendGuangyaSms, providerIconUrl, OpenBrowser, OpenPikPakCaptcha, ClosePikPakCaptcha, onEvent } from '../api'
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
let captchaEventOff = null
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
const errorText = ref('')

const provider = computed(() => props.providers.find((p) => p.ID === providerId.value) || props.providers[0])
const fields = computed(() => (provider.value && provider.value.Login && provider.value.Login.fields) || [])
const isMounted = computed(() => providerId.value === 'webdav' || providerId.value === 's3')
const isOAuth = computed(() => fields.value.some((f) => f.type === 'oauth'))
const isCookieField = (field) => /cookie|cookies|bduss/i.test(`${field.key} ${field.label}`)
const hasNonCookieLogin = computed(() => fields.value.some((field) => !isCookieField(field)))
const visibleFields = computed(() => hasNonCookieLogin.value
  ? fields.value.filter((field) => !isCookieField(field))
  : fields.value)
const isLongText = (key) => /cookie|token|bduss|secret|authorization/i.test(key)
const isPan139DirectLogin = computed(() => providerId.value === 'pan139' && String(form.value.authorization || '').trim() !== '')
function isFieldRequired(field) {
  if (isPan139DirectLogin.value && (field.key === 'username' || field.key === 'password')) return false
  return field.required
}

watch(providerId, (v) => {
  localStorage.setItem('login_provider', v)
  form.value = {}
  mountedForm.value = { name: '', endpoint: '', username: '', password: '', bucket: '', region: '', rootPath: '', basePath: '', sessionToken: '', forcePathStyle: true }
  errorText.value = ''
  resetCaptcha()
})

// A provider removed from a migrated config must not remain selectable by its
// stale localStorage id while the panel visually falls back to the first item.
watch(() => props.providers, (list) => {
  if (!list.length) return
  if (!list.some((p) => p.ID === providerId.value)) providerId.value = list[0].ID
}, { immediate: true })

function onKey(e) { if (e.key === 'Escape') emit('close') }
onMounted(() => {
  window.addEventListener('keydown', onKey)
  window.addEventListener('message', onCaptchaMessage)
  captchaEventOff = onEvent('pikpak:captcha:completed', (payload) => {
    if (providerId.value !== 'pikpak' || !captchaUrl.value) return
    const resultURL = String(payload?.url || '')
    if (resultURL && resultURL !== captchaUrl.value) return
    if (!acceptCaptchaToken(payload?.captcha_token)) {
      errorText.value = '验证码已完成，但没有收到有效结果，请重新验证'
      captchaNativeOpened.value = false
      return
    }
    captchaNativeOpened.value = true
    void submit()
  })
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  window.removeEventListener('message', onCaptchaMessage)
  captchaEventOff?.()
  captchaEventOff = null
  void ClosePikPakCaptcha()
  if (smsTimer) clearInterval(smsTimer)
})

// 逐网盘附加帮助（后端 Login.fields 之外的引导）
const PROVIDER_HELP = {
  aliopen: { label: '如何获取 refresh_token？', url: 'https://alist.nn.ci/tool/aliyundrive/request' },
  yike: { label: '如何获取 BDUSS / Cookie？', url: 'https://pan.baidu.com' },
}
const providerHelp = computed(() => PROVIDER_HELP[providerId.value] || null)
function openHelp() { if (providerHelp.value) OpenBrowser(providerHelp.value.url).catch(() => {}) }

// PikPak 滑块验证
const captchaUrl = ref('')
const captchaCompleted = ref(false)
const captchaFrameReady = ref(false)
const captchaSubmitting = ref(false)
const captchaNativeOpened = ref(false)
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
  let allowed = false
  try {
    const challenge = new URL(captchaUrl.value)
    allowed = !event.origin || event.origin === challenge.origin || /(^|\.)mypikpak\.(com|net)$/i.test(new URL(event.origin).hostname)
  } catch { /* ignore malformed browser messages */ }
  if (!allowed) return
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
  captchaNativeOpened.value = false
  return true
}
async function openCaptchaWebView() {
  if (!captchaUrl.value || captchaNativeOpened.value) return
  try {
    await OpenPikPakCaptcha(captchaUrl.value)
    captchaNativeOpened.value = true
    errorText.value = '请在应用内验证窗口完成安全验证，完成后会自动继续登录'
  } catch {
    // The inline iframe remains available when WebView2 is unavailable.
    captchaNativeOpened.value = false
    errorText.value = '应用内验证窗口不可用，请直接在下方完成安全验证'
  }
}
function reloadCaptcha() {
  if (!captchaUrl.value) return
  void ClosePikPakCaptcha()
  captchaCompleted.value = false
  captchaFrameReady.value = false
  captchaNativeOpened.value = false
  captchaUrl.value = `${captchaUrl.value}${captchaUrl.value.includes('?') ? '&' : '?'}_mneno=${Date.now()}`
  delete form.value.captcha_token
  delete form.value.captcha_verified
}

// 天翼云 189 图形验证码
const pan189Captcha = ref('')
function parse189Captcha(err) {
  const m = String(err).match(/captcha_required_189\nimage=(\S+)/)
  if (!m) return false
  pan189Captcha.value = m[1]
  return true
}

function resetCaptcha() {
  void ClosePikPakCaptcha()
  captchaUrl.value = ''
  captchaCompleted.value = false
  captchaFrameReady.value = false
  captchaSubmitting.value = false
  captchaNativeOpened.value = false
  pan189Captcha.value = ''
  delete form.value.captcha_token
  delete form.value.captcha_verified
  delete form.value.validate_code
}

async function sendSms() {
  if (!form.value.phone) { errorText.value = '请先填写手机号'; return }
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
  for (const f of visibleFields.value) {
    if (isFieldRequired(f) && !String(form.value[f.key] || '').trim()) return `请填写${f.label}`
  }
  // 全可选字段的 provider 至少填一项
  const allOpt = visibleFields.value.length > 0 && visibleFields.value.every((f) => !f.required)
  if (allOpt && !visibleFields.value.some((f) => String(form.value[f.key] || '').trim())) return '至少填写一个字段'
  return ''
}

async function submit() {
  if (busy.value) return
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
    if (providerId.value === 'pikpak' && parseCaptcha(e)) {
      errorText.value = '请完成安全验证'
      await openCaptchaWebView()
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
          <div class="modal-head">
            <h3>添加网盘账号</h3>
            <button class="icon-btn" style="width:28px;height:28px" title="关闭 (Esc)" @click="emit('close')"><UiIcon name="close" :size="14" /></button>
          </div>
          <div class="login-body">
            <div class="login-side">
              <div
                v-for="p in providers"
                :key="p.ID"
                class="lp-item"
                :class="{ active: p.ID === providerId }"
                @click="providerId = p.ID"
              >
                <img :src="providerIconUrl(p.Meta)" alt="" />
                <span>{{ p.Meta.label }}</span>
              </div>
            </div>

            <form class="login-form" @submit.prevent="submit">
              <div class="lf-head" v-if="provider">
                <img :src="providerIconUrl(provider.Meta)" alt="" />
                <div>
                  <div class="lf-title">{{ provider.Meta.label }}</div>
                  <div class="lf-sub">{{ isMounted ? '挂载存储连接' : isOAuth ? '浏览器 OAuth 授权' : '账号登录' }}</div>
                </div>
              </div>

              <!-- 挂载存储（WebDAV / S3） -->
              <template v-if="isMounted">
                <div class="field"><label>连接名称</label><input class="input" v-model="mountedForm.name" placeholder="我的 WebDAV / S3" /></div>
                <div class="field"><label>{{ providerId === 's3' ? 'Endpoint (可选)' : 'WebDAV 地址' }}<span v-if="providerId !== 's3'" class="req">*</span></label><input class="input" v-model="mountedForm.endpoint" :placeholder="providerId === 's3' ? 's3.us-east-1.amazonaws.com (可选，默认 AWS)' : 'https://dav.example.com'" /></div>
                <div class="field"><label>{{ providerId === 's3' ? 'Access Key ID' : '用户名' }}<span class="req">*</span></label><input class="input" v-model="mountedForm.username" /></div>
                <div class="field"><label>{{ providerId === 's3' ? 'Secret Access Key' : '密码' }}<span class="req">*</span></label><input class="input" type="password" v-model="mountedForm.password" /></div>
                <template v-if="providerId === 's3'">
                  <div class="field"><label>Bucket<span class="req">*</span></label><input class="input" v-model="mountedForm.bucket" /></div>
                  <div class="field"><label>Region (可选)</label><input class="input" v-model="mountedForm.region" placeholder="us-east-1" /></div>
                  <div class="field"><label>Session Token (可选)</label><input class="input" type="password" v-model="mountedForm.sessionToken" /></div>
                  <div class="field"><label>路径风格</label><div class="switch" :class="{ on: mountedForm.forcePathStyle }" @click="mountedForm.forcePathStyle = !mountedForm.forcePathStyle"></div><div class="hint">开启用于 MinIO、OSS 等兼容服务；AWS S3 可关闭</div></div>
                </template>
                <div v-if="providerId === 'webdav'" class="field"><label>根目录 (可选)</label><input class="input" v-model="mountedForm.rootPath" placeholder="/" /></div>
                <div v-else class="field"><label>挂载路径 (可选)</label><input class="input" v-model="mountedForm.basePath" placeholder="/" /></div>
              </template>

              <!-- OAuth 授权 -->
              <div v-else-if="isOAuth" class="oauth-box">
                <UiIcon name="external" :size="26" style="color:var(--color-primary)" />
                <p>点击下方按钮打开浏览器完成授权，<br />授权成功后自动登录。</p>
              </div>

              <!-- 常规表单 -->
              <template v-else>
                <div v-for="f in visibleFields" :key="f.key" class="field">
                  <label>{{ f.label }}<span v-if="isFieldRequired(f)" class="req">*</span></label>
                  <textarea v-if="isLongText(f.key)" class="textarea" v-model="form[f.key]" :placeholder="f.placeholder || ''" rows="3"></textarea>
                  <input v-else class="input" :type="f.type === 'password' ? 'password' : 'text'" v-model="form[f.key]" :placeholder="f.placeholder || ''" />
                  <div v-if="f.hint" class="hint">{{ f.hint }}</div>
                  <button
                    v-if="providerId === 'guangya' && f.key === 'sms_code'"
                    class="btn sm" style="margin-top:8px" :disabled="smsBusy" type="button" @click="sendSms"
                  >{{ smsBusy ? '发送中…' : (smsCountdown > 0 ? smsCountdown + 's 后重发' : '获取验证码') }}</button>
                </div>
                <div v-if="!visibleFields.length" class="hint" style="color:var(--text-tertiary);font-size: 13px">该网盘无需填写表单，直接点击登录。</div>
              </template>

              <!-- 网盘专属帮助链接 -->
              <button v-if="providerHelp" type="button" class="login-help" @click="openHelp">
                <UiIcon name="external" :size="12" /><span>{{ providerHelp.label }}</span>
              </button>

              <!-- PikPak 滑块安全验证 -->
              <div v-if="captchaUrl" class="captcha-box">
                <div class="captcha-head">
                  <div>
                    <strong>{{ captchaCompleted ? '验证已完成' : '请在下方完成安全验证' }}</strong>
                    <p>{{ captchaCompleted ? '正在使用最新验证结果继续登录' : (captchaNativeOpened ? '请在应用内验证窗口完成滑块' : (captchaFrameReady ? '完成滑块后将自动继续登录' : '正在加载验证页面…')) }}</p>
                  </div>
                  <button class="btn sm" type="button" :disabled="captchaSubmitting" @click="reloadCaptcha">重新加载</button>
                </div>
                <div v-if="captchaNativeOpened" class="captcha-native-state">
                  <UiIcon name="monitor" :size="22" />
                  <span>验证窗口已打开，完成后会自动回传结果</span>
                  <button class="btn sm" type="button" :disabled="captchaSubmitting" @click="openCaptchaWebView">重新打开验证窗口</button>
                </div>
                <iframe
                  v-if="!captchaNativeOpened"
                  class="captcha-frame"
                  :src="captchaUrl"
                  title="PikPak 安全验证"
                  referrerpolicy="strict-origin-when-cross-origin"
                  allow="clipboard-read; clipboard-write"
                  @load="captchaFrameReady = true"
                ></iframe>
                <div class="captcha-actions">
                  <span v-if="captchaCompleted" class="captcha-state">已收到验证结果</span>
                  <button v-if="!captchaNativeOpened" class="btn sm" type="button" :disabled="captchaSubmitting" @click="openCaptchaWebView">在应用内打开验证</button>
                  <button v-if="captchaCompleted" class="btn primary sm" type="submit" :disabled="busy">{{ busy ? '登录中…' : '验证完成并登录' }}</button>
                </div>
              </div>

              <!-- 天翼云 189 图形验证码 -->
              <div v-if="pan189Captcha" class="captcha-box">
                <p style="margin-bottom:6px">请输入下图中的验证码：</p>
                <div style="display:flex;align-items:center;gap:10px;margin-bottom:8px">
                  <img :src="pan189Captcha" alt="图形验证码" style="height:36px;border-radius:var(--radius-xs);background:#fff;border:1px solid var(--border-light)" />
                </div>
              </div>

              <!-- 统一错误提示（带 Shake 抖动动画） -->
              <div v-if="errorText" class="form-error shake">
                <UiIcon name="warning" :size="14" /><span>{{ errorText }}</span>
              </div>

              <div class="modal-actions" style="padding:6px 0 0">
                <button class="btn" type="button" @click="emit('close')">取消</button>
                <button class="btn primary" type="submit" :disabled="busy">
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
.req { color: var(--color-error); margin-left: 2px; }
.oauth-box {
  display: flex; flex-direction: column; align-items: center; gap: 10px;
  padding: 28px 16px; text-align: center;
  color: var(--text-secondary); font-size: 13.5px; line-height: 1.7;
  background: var(--bg-subtle); border-radius: var(--radius-md);
}
.oauth-box p { margin: 0; }
.login-help {
  display: inline-flex; align-items: center; gap: 5px; align-self: flex-start;
  border: none; background: none; padding: 0; margin-top: 8px;
  color: var(--color-primary); font-size: 13px; cursor: pointer;
}
.login-help:hover { text-decoration: underline; }
.captcha-box {
  margin-top: 10px; padding: 10px 12px;
  border-radius: var(--radius-sm);
  background: color-mix(in srgb, var(--color-warning) 8%, transparent);
  box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--color-warning) 25%, transparent);
  font-size: 13px; color: var(--text-secondary); line-height: 1.6;
}
.captcha-box p { margin: 0 0 8px; }
.captcha-head { display:flex; align-items:flex-start; justify-content:space-between; gap:12px; }
.captcha-head strong { display:block; color:var(--text-primary); font-size:13px; }
.captcha-head p { margin:3px 0 0; }
.captcha-frame {
  display:block; width:100%; height:320px; margin-top:10px;
  border:1px solid var(--border-light); border-radius:var(--radius-sm);
  background:#fff;
}
.captcha-native-state {
  min-height: 190px; margin-top:10px; padding:24px 16px;
  display:flex; flex-direction:column; align-items:center; justify-content:center; gap:10px;
  border:1px dashed var(--border-light); border-radius:var(--radius-sm);
  color:var(--text-secondary); text-align:center;
}
.captcha-actions { display:flex; align-items:center; justify-content:space-between; gap:8px; margin-top:10px; }
.captcha-state { color:var(--color-success); font-size:12px; }
@media (max-width: 640px) {
  .captcha-frame { height:300px; }
  .captcha-head { align-items:stretch; flex-direction:column; }
  .captcha-head .btn { align-self:flex-start; }
}
</style>
