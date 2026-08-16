<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { login, saveMounted, SendGuangyaSms, providerIconUrl, OpenBrowser } from '../api'
import UiIcon from './UiIcon.vue'

const props = defineProps({ providers: { type: Array, default: () => [] } })
const emit = defineEmits(['close', 'toast'])

const providerId = ref(localStorage.getItem('login_provider') || 'pikpak')
const form = ref({})
const mountedForm = ref({ name: '', endpoint: '', username: '', password: '', bucket: '', region: '', basePath: '' })
const busy = ref(false)
const smsBusy = ref(false)
const smsCountdown = ref(0)
let smsTimer = null
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
const isLongText = (key) => /cookie|token|bduss|secret/i.test(key)

watch(providerId, (v) => {
  localStorage.setItem('login_provider', v)
  form.value = {}
  mountedForm.value = { name: '', endpoint: '', username: '', password: '', bucket: '', region: '', basePath: '' }
  errorText.value = ''
  resetCaptcha()
})

function onKey(e) { if (e.key === 'Escape') emit('close') }
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  if (smsTimer) clearInterval(smsTimer)
})

// 逐网盘附加帮助（后端 Login.fields 之外的引导）
const PROVIDER_HELP = {
  aliopen: { label: '如何获取 refresh_token？', url: 'https://alist.nn.ci/tool/aliyundrive/request' },
  yike: { label: '如何获取 BDUSS / Cookie？', url: 'https://pan.baidu.com' },
}
const providerHelp = computed(() => PROVIDER_HELP[providerId.value] || null)
function openHelp() { if (providerHelp.value) OpenBrowser(providerHelp.value.url).catch(() => {}) }

// PikPak 滑块验证：后端报 captcha_required 时拿到 url+token，浏览器完成滑块后带 token 重试
const captchaUrl = ref('')
const captchaOpened = ref(false)
function parseCaptcha(err) {
  const m = String(err).match(/captcha_required\nurl=(\S+)\ntoken=(\S*)/)
  if (!m) return false
  captchaUrl.value = m[1]
  form.value.captcha_token = m[2]
  captchaOpened.value = false
  return true
}
function openCaptcha() {
  captchaOpened.value = true
  OpenBrowser(captchaUrl.value).catch(() => {})
}
function resetCaptcha() {
  captchaUrl.value = ''
  captchaOpened.value = false
  delete form.value.captcha_token
}

async function sendSms() {
  if (!form.value.phone) { errorText.value = '请先填写手机号'; return }
  smsBusy.value = true
  errorText.value = ''
  try {
    const r = await SendGuangyaSms(form.value.phone)
    form.value.verification_id = r.verification_id
    form.value.device_id = r.device_id
    emit('toast', '验证码已发送', 'success')
  } catch (e) { errorText.value = String(e); return }
  smsBusy.value = false
  startSmsCountdown()
}

function validate() {
  if (isMounted.value) {
    const m = mountedForm.value
    if (!m.endpoint.trim()) return providerId.value === 's3' ? '请填写 Endpoint' : '请填写 WebDAV 地址'
    if (!m.username.trim()) return providerId.value === 's3' ? '请填写 Access Key ID' : '请填写用户名'
    if (!m.password) return providerId.value === 's3' ? '请填写 Secret Access Key' : '请填写密码'
    if (providerId.value === 's3' && !m.bucket.trim()) return '请填写 Bucket'
    return ''
  }
  if (isOAuth.value) return '' // OAuth 无表单校验
  for (const f of fields.value) {
    if (f.required && !String(form.value[f.key] || '').trim()) return `请填写${f.label}`
  }
  return ''
}

async function submit() {
  if (busy.value) return
  const err = validate()
  if (err) { errorText.value = err; return }
  busy.value = true
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
      errorText.value = ''
    } else {
      errorText.value = String(e)
    }
  }
  busy.value = false
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
                <div class="field"><label>{{ providerId === 's3' ? 'Endpoint' : 'WebDAV 地址' }}<span class="req">*</span></label><input class="input" v-model="mountedForm.endpoint" :placeholder="providerId === 's3' ? 's3.example.com（可选，默认 AWS）' : 'https://dav.example.com'" /></div>
                <div class="field"><label>{{ providerId === 's3' ? 'Access Key ID' : '用户名' }}<span class="req">*</span></label><input class="input" v-model="mountedForm.username" /></div>
                <div class="field"><label>{{ providerId === 's3' ? 'Secret Access Key' : '密码' }}<span class="req">*</span></label><input class="input" type="password" v-model="mountedForm.password" /></div>
                <template v-if="providerId === 's3'">
                  <div class="field"><label>Bucket<span class="req">*</span></label><input class="input" v-model="mountedForm.bucket" /></div>
                  <div class="field"><label>Region（可选）</label><input class="input" v-model="mountedForm.region" placeholder="us-east-1" /></div>
                </template>
                <div class="field"><label>挂载路径（可选）</label><input class="input" v-model="mountedForm.basePath" placeholder="/" /></div>
              </template>

              <!-- OAuth 授权 -->
              <div v-else-if="isOAuth" class="oauth-box">
                <UiIcon name="external" :size="26" style="color:var(--color-primary)" />
                <p>点击下方按钮打开浏览器完成授权，<br />授权成功后自动登录。</p>
              </div>

              <!-- 常规表单 -->
              <template v-else>
                <div v-for="f in fields" :key="f.key" class="field">
                  <label>{{ f.label }}<span v-if="f.required" class="req">*</span></label>
                  <textarea v-if="isLongText(f.key)" class="textarea" v-model="form[f.key]" :placeholder="f.placeholder || ''" rows="3"></textarea>
                  <input v-else class="input" :type="f.type === 'password' ? 'password' : 'text'" v-model="form[f.key]" :placeholder="f.placeholder || ''" />
                  <div v-if="f.hint" class="hint">{{ f.hint }}</div>
                  <button
                    v-if="providerId === 'guangya' && f.key === 'sms_code'"
                    class="btn sm" style="margin-top:8px" :disabled="smsBusy" type="button" @click="sendSms"
                  >{{ smsBusy ? '发送中…' : (smsCountdown > 0 ? smsCountdown + 's 后重发' : '获取验证码') }}</button>
                </div>
                <div v-if="!fields.length" class="hint" style="color:var(--text-tertiary);font-size: 13px">该网盘无需填写表单，直接点击登录。</div>
              </template>

              <!-- 网盘专属帮助链接 -->
              <button v-if="providerHelp" type="button" class="login-help" @click="openHelp">
                <UiIcon name="external" :size="12" /><span>{{ providerHelp.label }}</span>
              </button>

              <!-- PikPak 滑块安全验证 -->
              <div v-if="captchaUrl" class="captcha-box">
                <p>{{ captchaOpened ? '验证窗口已打开，完成滑块后点「完成验证并登录」' : '账号触发了 PikPak 安全验证，需要在浏览器中完成滑块' }}</p>
                <div class="captcha-actions">
                  <button class="btn sm" type="button" @click="openCaptcha">{{ captchaOpened ? '重新打开验证页' : '打开验证页' }}</button>
                  <button v-if="captchaOpened" class="btn primary sm" type="submit" :disabled="busy">完成验证并登录</button>
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
.captcha-actions { display: flex; gap: 8px; }
</style>
