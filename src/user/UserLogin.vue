<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useUserStore } from '../store'
import UserDAL from '../user/userdal'
import message from '../utils/message'
import { beginPikPakLoginCaptcha, loginPikPak, loginPikPakWithCaptcha } from '../pikpak/auth'
import { getDriveProviderMeta } from '../utils/driveProvider'
import { createWebDavConnection, createWebDavUserToken, saveWebDavConnection, testWebDavConnection } from '../utils/webdavClient'
import { createS3Connection, createS3UserToken, saveS3Connection, testS3Connection } from '../utils/s3Client'
import { loginGofile } from '../gofile/auth'
import { buildOneDriveAuthUrl, createOneDrivePkceVerifier, exchangeOneDriveCodeForToken, ONEDRIVE_CLIENT_ID } from '../onedrive/auth'
import { buildDropboxAuthUrl, createDropboxPkceVerifier, DROPBOX_APP_KEY, exchangeDropboxCodeForToken } from '../dropbox/auth'
import { buildGoogleDriveAuthUrl, createGoogleDrivePkceVerifier, exchangeGoogleDriveCodeForToken, GOOGLE_DRIVE_CLIENT_ID, GOOGLE_DRIVE_CLIENT_SECRET } from '../gdrive/auth'

type LoginProvider = 'pikpak' | 'onedrive' | 'dropbox' | 'gdrive' | 'gofile' | 'webdav' | 's3'
type OAuthLoginProvider = 'onedrive' | 'dropbox' | 'gdrive'
type OAuthLoginContext = { provider: OAuthLoginProvider; verifier: string; redirectUri: string }

const useUser = useUserStore()
const loginProviders: LoginProvider[] = ['pikpak', 'onedrive', 'dropbox', 'gdrive', 'gofile', 'webdav', 's3']
const loginProvider = ref<LoginProvider>('pikpak')
const activeLoginProviderMeta = computed(() => getDriveProviderMeta(loginProvider.value))
const pikpakUsername = ref('')
const pikpakPassword = ref('')
const pikpakLoading = ref(false)
const pikpakCaptchaLoading = ref(false)
const pikpakCaptchaUrl = ref('')
const pikpakCaptchaToken = ref('')
const pikpakCaptchaDeviceId = ref('')
const pikpakCaptchaVerified = ref(false)
const pikpakCaptchaError = ref('')
let removePikPakCaptchaListeners: (() => void) | null = null
const gofileToken = ref('')
const gofileLoading = ref(false)
const webDavForm = ref({ name: '', url: '', username: '', password: '', rootPath: '/' })
const webDavLoading = ref(false)
const s3Form = ref({ name: '', endpoint: '', region: 'us-east-1', accessKeyId: '', secretAccessKey: '', sessionToken: '', bucket: '', rootPrefix: '', forcePathStyle: true })
const s3Loading = ref(false)
const oauthLoading = ref<OAuthLoginProvider | ''>('')
const oauthLoginContexts = new Map<string, OAuthLoginContext>()

const isOAuthProvider = (provider: LoginProvider): provider is OAuthLoginProvider => provider === 'onedrive' || provider === 'dropbox' || provider === 'gdrive'

const handleModalOpen = () => {
  const stored = localStorage.getItem('login_provider')
  if (loginProviders.includes(stored as LoginProvider)) loginProvider.value = stored as LoginProvider
}

watch(loginProvider, provider => {
  localStorage.setItem('login_provider', provider)
  if (provider !== 'pikpak') resetPikPakCaptcha()
})

watch(pikpakUsername, () => {
  // Username changes invalidate captcha binding.
  if (pikpakCaptchaToken.value || pikpakCaptchaUrl.value) resetPikPakCaptcha()
})

const getOAuthClientId = (provider: OAuthLoginProvider) => provider === 'onedrive' ? ONEDRIVE_CLIENT_ID.trim() : provider === 'dropbox' ? DROPBOX_APP_KEY.trim() : GOOGLE_DRIVE_CLIENT_ID.trim()

const cancelOAuthLogins = () => {
  for (const state of oauthLoginContexts.keys()) window.WebOAuthCancel?.(state)
  oauthLoginContexts.clear()
  oauthLoading.value = ''
}

const startOAuthLogin = async (provider: OAuthLoginProvider) => {
  if (oauthLoading.value) return
  const clientId = getOAuthClientId(provider)
  if (!clientId) return message.error(`${getDriveProviderMeta(provider).label} 授权配置缺失`)
  oauthLoading.value = provider
  let state = ''
  try {
    const session = await window.WebOAuthBegin(provider)
    state = session.state || ''
    if (!session.ok || !state || !session.redirectUri) throw new Error(session.error || '无法创建应用内授权会话')
    const verifier = provider === 'onedrive' ? createOneDrivePkceVerifier() : provider === 'dropbox' ? createDropboxPkceVerifier() : createGoogleDrivePkceVerifier()
    oauthLoginContexts.set(state, { provider, verifier, redirectUri: session.redirectUri })
    const authUrl = provider === 'onedrive'
      ? await buildOneDriveAuthUrl(clientId, verifier, state, session.redirectUri)
      : provider === 'dropbox'
        ? await buildDropboxAuthUrl(clientId, verifier, state, session.redirectUri)
        : await buildGoogleDriveAuthUrl(clientId, verifier, state, session.redirectUri)
    const opened = await window.WebOAuthOpen(state, authUrl)
    if (!opened.ok) throw new Error(opened.error || '应用内登录窗口打开失败')
  } catch (error: any) {
    if (state) await window.WebOAuthCancel?.(state)
    oauthLoginContexts.delete(state)
    oauthLoading.value = ''
    message.error(error?.message || '登录失败')
  }
}

const removeOAuthCallback = window.WebOAuthOnCallback?.(async payload => {
  const context = oauthLoginContexts.get(payload.state)
  if (!context || context.provider !== payload.provider) return
  oauthLoginContexts.delete(payload.state)
  try {
    if (payload.error || !payload.code) throw new Error(payload.errorDescription || payload.error || '授权未完成')
    const clientId = getOAuthClientId(context.provider)
    const token = context.provider === 'onedrive'
      ? await exchangeOneDriveCodeForToken(payload.code, clientId, context.verifier, context.redirectUri)
      : context.provider === 'dropbox'
        ? await exchangeDropboxCodeForToken(payload.code, clientId, context.verifier, context.redirectUri)
        : await exchangeGoogleDriveCodeForToken(payload.code, clientId, GOOGLE_DRIVE_CLIENT_SECRET, context.verifier, context.redirectUri)
    if (!token) throw new Error('授权成功，但账号信息获取失败')
    await UserDAL.UserLogin(token, true)
    useUser.userShowLogin = false
  } catch (error: any) {
    message.error(error?.message || '登录失败')
  } finally {
    oauthLoading.value = ''
  }
})

onBeforeUnmount(() => {
  removeOAuthCallback?.()
  cancelOAuthLogins()
  resetPikPakCaptcha()
})

const getPikPakCaptchaWebview = () => document.getElementById('pikpak-captcha-webview') as any

const cleanupPikPakCaptchaWebview = () => {
  removePikPakCaptchaListeners?.()
  removePikPakCaptchaListeners = null
}

const resetPikPakCaptcha = () => {
  cleanupPikPakCaptchaWebview()
  pikpakCaptchaUrl.value = ''
  pikpakCaptchaToken.value = ''
  pikpakCaptchaDeviceId.value = ''
  pikpakCaptchaVerified.value = false
  pikpakCaptchaError.value = ''
  pikpakCaptchaLoading.value = false
}

const markPikPakCaptchaVerified = (token?: string) => {
  if (token) pikpakCaptchaToken.value = token
  if (!pikpakCaptchaToken.value) return
  pikpakCaptchaVerified.value = true
  pikpakCaptchaError.value = ''
  message.success('验证完成，请点击登录')
}

const extractCaptchaTokenFromUrl = (value: string) => {
  try {
    const url = new URL(value)
    return url.searchParams.get('captcha_token') || url.searchParams.get('captchaToken') || url.hash.match(/captcha_token=([^&]+)/)?.[1] || ''
  } catch {
    return ''
  }
}

const injectPikPakCaptchaHooks = (webview: any) => {
  const script = `(() => {
    if (window.__mnemoPikPakCaptchaHooked) return
    window.__mnemoPikPakCaptchaHooked = true
    const report = (token) => {
      if (!token || typeof token !== 'string') return
      console.log('captcha_token=' + token)
      try { window.parent && window.parent.postMessage({ type: 'pikpak-captcha', captcha_token: token }, '*') } catch (e) {}
    }
    window.addEventListener('message', (event) => {
      try {
        const data = event && event.data
        if (!data) return
        if (typeof data === 'string') {
          const m = data.match(/captcha_token[=:\\"']+([A-Za-z0-9._-]+)/i)
          if (m && m[1]) report(m[1])
          return
        }
        const token = data.captcha_token || data.captchaToken || data.ticket || (data.data && (data.data.captcha_token || data.data.ticket))
        if (token) report(String(token))
      } catch (e) {}
    }, true)
    const patch = (obj, key) => {
      if (!obj || typeof obj[key] !== 'function') return
      const original = obj[key]
      obj[key] = function () {
        try {
          for (const arg of arguments) {
            if (typeof arg === 'string' && arg.length > 20) report(arg)
            if (arg && typeof arg === 'object') {
              const token = arg.captcha_token || arg.captchaToken || arg.ticket
              if (token) report(String(token))
            }
          }
        } catch (e) {}
        return original.apply(this, arguments)
      }
    }
    try { patch(window, 'captchaCallback') } catch (e) {}
    try { patch(window, 'verifyCallback') } catch (e) {}
  })()`
  try {
    if (typeof webview.executeJavaScript === 'function') {
      void webview.executeJavaScript(script, false).catch(() => undefined)
    }
  } catch {
    // ignore injection failures; navigation/console hooks remain
  }
}

const attachPikPakCaptchaWebview = async () => {
  await nextTick()
  const webview = getPikPakCaptchaWebview()
  if (!webview || !pikpakCaptchaUrl.value) return
  cleanupPikPakCaptchaWebview()
  const handleNavigation = (event: any) => {
    const url = String(event?.url || webview.getURL?.() || '')
    const token = extractCaptchaTokenFromUrl(url)
    if (token && token !== pikpakCaptchaToken.value) markPikPakCaptchaVerified(token)
    // Some challenge pages return to mypikpak.com with success without repeating token.
    if (/captcha.*success|verify.*ok|signin_check.*pass/i.test(url) && pikpakCaptchaToken.value) {
      markPikPakCaptchaVerified(pikpakCaptchaToken.value)
    }
  }
  const handleConsole = (event: any) => {
    const msg = String(event?.message || '')
    const match = msg.match(/captcha_token[=:]\s*([A-Za-z0-9._-]+)/i)
    if (match?.[1]) markPikPakCaptchaVerified(match[1])
  }
  const handleIpcMessage = (event: any) => {
    try {
      const channel = event?.channel
      const args = event?.args || []
      if (channel === 'console-message' || channel === 'pikpak-captcha') {
        const token = String(args[0] || args[1] || '')
        if (token.length > 16) markPikPakCaptchaVerified(token)
      }
    } catch {
      // ignore
    }
  }
  const handleFinish = () => {
    pikpakCaptchaLoading.value = false
    injectPikPakCaptchaHooks(webview)
  }
  const handleFail = (event: any) => {
    if (event?.errorCode === -3) return
    // Ignore secondary resource failures (CDN fonts/scripts) if main document loaded.
    const isMain = event?.isMainFrame !== false
    if (!isMain) return
    pikpakCaptchaLoading.value = false
    pikpakCaptchaError.value = '验证页面加载失败，请点「刷新验证」或用浏览器打开'
  }
  webview.addEventListener('will-navigate', handleNavigation)
  webview.addEventListener('did-navigate', handleNavigation)
  webview.addEventListener('did-redirect-navigation', handleNavigation)
  webview.addEventListener('did-navigate-in-page', handleNavigation)
  webview.addEventListener('console-message', handleConsole)
  webview.addEventListener('ipc-message', handleIpcMessage)
  webview.addEventListener('did-finish-load', handleFinish)
  webview.addEventListener('did-stop-loading', handleFinish)
  webview.addEventListener('did-fail-load', handleFail)
  removePikPakCaptchaListeners = () => {
    webview.removeEventListener('will-navigate', handleNavigation)
    webview.removeEventListener('did-navigate', handleNavigation)
    webview.removeEventListener('did-redirect-navigation', handleNavigation)
    webview.removeEventListener('did-navigate-in-page', handleNavigation)
    webview.removeEventListener('console-message', handleConsole)
    webview.removeEventListener('ipc-message', handleIpcMessage)
    webview.removeEventListener('did-finish-load', handleFinish)
    webview.removeEventListener('did-stop-loading', handleFinish)
    webview.removeEventListener('did-fail-load', handleFail)
  }
  try {
    // Prefer attribute src binding; only force loadURL when guest is already attached.
    if (webview.getURL && webview.getURL() && webview.getURL() !== pikpakCaptchaUrl.value) {
      const load = webview.loadURL(pikpakCaptchaUrl.value)
      if (load?.catch) {
        load.catch((err: any) => {
          if (!/ERR_(ABORTED|FAILED)/i.test(err?.message || '')) {
            pikpakCaptchaError.value = '验证页面打开失败'
            pikpakCaptchaLoading.value = false
          }
        })
      }
    }
  } catch {
    // Attribute src will still load the guest page.
  }
}

const openPikPakCaptchaInBrowser = async () => {
  if (!pikpakCaptchaUrl.value) return
  try {
    await window.WebOpenExternal?.(pikpakCaptchaUrl.value)
    message.info('已在系统浏览器打开验证页；若验证完成后仍无法登录，请点「刷新验证」再试')
  } catch {
    message.error('无法打开系统浏览器')
  }
}

const requestPikPakCaptcha = async () => {
  if (!pikpakUsername.value.trim()) return message.error('请先输入 PikPak 账号')
  pikpakCaptchaLoading.value = true
  pikpakCaptchaVerified.value = false
  pikpakCaptchaError.value = ''
  try {
    const { deviceId, captcha } = await beginPikPakLoginCaptcha(pikpakUsername.value.trim())
    pikpakCaptchaDeviceId.value = deviceId
    pikpakCaptchaToken.value = captcha.captchaToken
    if (captcha.challengeUrl) {
      pikpakCaptchaUrl.value = captcha.challengeUrl
      await attachPikPakCaptchaWebview()
      message.info('请完成下方图形验证')
    } else {
      pikpakCaptchaUrl.value = ''
      cleanupPikPakCaptchaWebview()
      markPikPakCaptchaVerified(captcha.captchaToken)
      pikpakCaptchaLoading.value = false
    }
  } catch (error: any) {
    resetPikPakCaptcha()
    message.error(error?.message || '获取 PikPak 验证码失败')
  }
}

const submitPikPakLogin = async () => {
  if (pikpakLoading.value || !pikpakUsername.value.trim() || !pikpakPassword.value) return message.error('请输入 PikPak 账号和密码')
  pikpakLoading.value = true
  try {
    if (!pikpakCaptchaToken.value || (pikpakCaptchaUrl.value && !pikpakCaptchaVerified.value)) {
      await requestPikPakCaptcha()
      if (pikpakCaptchaUrl.value && !pikpakCaptchaVerified.value) {
        message.warning('请先完成图形验证，再点击登录')
        return
      }
    }
    const token = pikpakCaptchaToken.value
      ? await loginPikPakWithCaptcha(pikpakUsername.value.trim(), pikpakPassword.value, pikpakCaptchaToken.value, pikpakCaptchaDeviceId.value || undefined)
      : await loginPikPak(pikpakUsername.value.trim(), pikpakPassword.value)
    await UserDAL.UserLogin(token, true)
    resetPikPakCaptcha()
    useUser.userShowLogin = false
  } catch (error: any) {
    if (error?.code === 'PikPak_CAPTCHA_REQUIRED' || error?.challengeUrl) {
      pikpakCaptchaDeviceId.value = error.deviceId || pikpakCaptchaDeviceId.value
      pikpakCaptchaToken.value = error.captchaToken || pikpakCaptchaToken.value
      pikpakCaptchaUrl.value = error.challengeUrl || ''
      pikpakCaptchaVerified.value = false
      await attachPikPakCaptchaWebview()
      message.warning('请完成图形验证后再登录')
    } else {
      message.error(error?.message || 'PikPak 登录失败')
      // Force a fresh captcha after failed attempt.
      pikpakCaptchaVerified.value = false
      pikpakCaptchaToken.value = ''
    }
  } finally {
    pikpakLoading.value = false
  }
}

const submitGofileLogin = async () => {
  if (!gofileToken.value.trim()) return message.error('请输入 GoFile Account API Token')
  gofileLoading.value = true
  try {
    await UserDAL.UserLogin(await loginGofile(gofileToken.value), true)
    useUser.userShowLogin = false
  } catch (error: any) {
    message.error(error?.message || 'GoFile 登录失败')
  } finally {
    gofileLoading.value = false
  }
}

const submitWebDavLogin = async () => {
  const form = webDavForm.value
  if (!form.name.trim() || !form.url.trim() || !form.username.trim() || !form.password.trim()) return message.error('请填写 WebDAV 名称、地址、用户名和密码')
  webDavLoading.value = true
  try {
    const connection = createWebDavConnection(form)
    await testWebDavConnection(connection)
    saveWebDavConnection(connection)
    await UserDAL.UserLogin(createWebDavUserToken(connection), true)
    useUser.userShowLogin = false
  } catch (error: any) {
    message.error(`添加 WebDAV 失败: ${error?.message || '未知错误'}`)
  } finally {
    webDavLoading.value = false
  }
}

const submitS3Login = async () => {
  const form = s3Form.value
  if (!form.name.trim() || !form.bucket.trim() || !form.accessKeyId.trim() || !form.secretAccessKey.trim()) return message.error('请填写 S3 名称、Bucket、Access Key 和 Secret Key')
  s3Loading.value = true
  try {
    const connection = createS3Connection(form)
    await testS3Connection(connection)
    saveS3Connection(connection)
    await UserDAL.UserLogin(createS3UserToken(connection), true)
    useUser.userShowLogin = false
  } catch (error: any) {
    message.error(`添加 S3 失败: ${error?.message || '未知错误'}`)
  } finally {
    s3Loading.value = false
  }
}

const handleClose = () => {
  cancelOAuthLogins()
  resetPikPakCaptcha()
  pikpakPassword.value = ''
  gofileToken.value = ''
}
</script>

<template>
  <a-modal
    title="网盘账号登录"
    v-model:visible="useUser.userShowLogin"
    :mask-closable="false"
    :footer="false"
    :modal-class="loginProvider === 'pikpak' && pikpakCaptchaUrl ? 'userloginmodal userloginmodal--captcha' : 'userloginmodal'"
    @before-open="handleModalOpen"
    @close="handleClose"
  >
    <div class="login-modal-body" :class="{ 'login-modal-body--captcha': loginProvider === 'pikpak' && !!pikpakCaptchaUrl }">
      <aside class="login-provider-sidebar">
        <button v-for="provider in loginProviders" :key="provider" class="login-provider-side-item" :class="{ active: loginProvider === provider }" @click="loginProvider = provider">
          <img v-if="getDriveProviderMeta(provider).icon" class="login-provider-icon" :src="getDriveProviderMeta(provider).icon" :alt="getDriveProviderMeta(provider).label" />
          <span>{{ getDriveProviderMeta(provider).label }}</span>
        </button>
      </aside>
      <section class="login-provider-content">
        <div class="login-provider-heading"><img v-if="activeLoginProviderMeta.icon" :src="activeLoginProviderMeta.icon" :alt="activeLoginProviderMeta.label" /><span>{{ activeLoginProviderMeta.label }}</span></div>
        <form v-if="loginProvider === 'pikpak'" class="login-form pikpak-login-form" @submit.prevent="submitPikPakLogin">
          <a-input v-model="pikpakUsername" placeholder="PikPak 邮箱 / 手机号 / 用户名" allow-clear />
          <a-input-password v-model="pikpakPassword" placeholder="PikPak 密码" allow-clear />
          <div class="pikpak-captcha-panel">
            <div class="pikpak-captcha-panel-head">
              <span class="pikpak-captcha-title">安全验证</span>
              <div class="pikpak-captcha-actions">
                <a-button type="text" size="mini" :loading="pikpakCaptchaLoading" @click.prevent="requestPikPakCaptcha">{{ pikpakCaptchaUrl || pikpakCaptchaToken ? '刷新验证' : '获取验证码' }}</a-button>
                <a-button v-if="pikpakCaptchaUrl" type="text" size="mini" @click.prevent="openPikPakCaptchaInBrowser">浏览器打开</a-button>
              </div>
            </div>
            <div class="pikpak-captcha-status" :class="{ ok: pikpakCaptchaVerified, err: !!pikpakCaptchaError }">
              {{ pikpakCaptchaVerified ? '验证已完成，可登录' : pikpakCaptchaError || (pikpakCaptchaUrl ? '请在下方完成滑块/图形验证（嵌入登录窗）' : '点击「获取验证码」后，验证会嵌在登录窗口内显示') }}
            </div>
            <div class="pikpak-captcha-frame-wrap" :class="{ empty: !pikpakCaptchaUrl }">
              <webview
                v-if="pikpakCaptchaUrl"
                id="pikpak-captcha-webview"
                class="pikpak-captcha-webview"
                :src="pikpakCaptchaUrl"
                allowpopups
                webpreferences="contextIsolation=yes, nodeIntegration=no, sandbox=no, webSecurity=no"
              ></webview>
              <div v-else class="pikpak-captcha-placeholder">验证区域（嵌入登录界面，不会单独弹窗）</div>
            </div>
          </div>
          <a-button html-type="submit" type="primary" long :loading="pikpakLoading" :disabled="!pikpakCaptchaToken || (!!pikpakCaptchaUrl && !pikpakCaptchaVerified)">添加 PikPak 账号</a-button>
        </form>
        <div v-else-if="isOAuthProvider(loginProvider)" class="login-form oauth-login"><a-button type="primary" long :loading="oauthLoading === loginProvider" @click="startOAuthLogin(loginProvider)">在 Mnemo 中登录</a-button></div>
        <form v-else-if="loginProvider === 'gofile'" class="login-form" @submit.prevent="submitGofileLogin"><a-input-password v-model="gofileToken" placeholder="GoFile Account API Token" allow-clear /><a-button html-type="submit" type="primary" long :loading="gofileLoading">登录 GoFile</a-button></form>
        <form v-else-if="loginProvider === 'webdav'" class="login-form" @submit.prevent="submitWebDavLogin"><a-input v-model="webDavForm.name" placeholder="连接名称" allow-clear /><a-input v-model="webDavForm.url" placeholder="WebDAV 地址" allow-clear /><a-input v-model="webDavForm.username" placeholder="用户名" allow-clear /><a-input-password v-model="webDavForm.password" placeholder="密码" allow-clear /><a-input v-model="webDavForm.rootPath" placeholder="挂载路径，默认 /" allow-clear /><a-button html-type="submit" type="primary" long :loading="webDavLoading">连接 WebDAV</a-button></form>
        <form v-else class="s3-login-form" @submit.prevent="submitS3Login"><a-input v-model="s3Form.name" class="wide" placeholder="连接名称" allow-clear /><a-input v-model="s3Form.endpoint" class="wide" placeholder="Endpoint（可选）" allow-clear /><a-input v-model="s3Form.region" placeholder="Region" allow-clear /><a-input v-model="s3Form.bucket" placeholder="Bucket" allow-clear /><a-input v-model="s3Form.accessKeyId" placeholder="Access Key ID" allow-clear /><a-input-password v-model="s3Form.secretAccessKey" placeholder="Secret Access Key" allow-clear /><a-input-password v-model="s3Form.sessionToken" class="wide" placeholder="Session Token（可选）" allow-clear /><a-input v-model="s3Form.rootPrefix" placeholder="根前缀（可选）" allow-clear /><label><a-switch v-model="s3Form.forcePathStyle" size="small" /> 路径样式</label><a-button class="wide" html-type="submit" type="primary" long :loading="s3Loading">连接 S3</a-button></form>
      </section>
    </div>
  </a-modal>
</template>

<style lang="less">
.userloginmodal .arco-modal-body { min-height: 440px; padding: 0 16px 16px !important; }
.userloginmodal--captcha.arco-modal,
.userloginmodal--captcha .arco-modal { width: 720px !important; max-width: calc(100vw - 48px); }
</style>

<style lang="less" scoped>
.login-modal-body { display: flex; width: 540px; height: 458px; overflow: hidden; transition: width 0.2s ease, height 0.2s ease; }
.login-modal-body--captcha { width: 680px; height: 560px; }
.login-provider-sidebar { flex: 0 0 148px; padding: 8px 8px 8px 0; overflow-y: auto; border-right: 1px solid var(--color-border-2); }
.login-provider-side-item { display: flex; align-items: center; width: 100%; min-height: 36px; gap: 8px; padding: 0 10px; margin-bottom: 3px; color: var(--color-text-2); background: transparent; border: 0; border-radius: 6px; cursor: pointer; text-align: left; }
.login-provider-side-item:hover, .login-provider-side-item.active { color: rgb(var(--primary-6)); background: var(--color-fill-2); }
.login-provider-icon { width: 16px; height: 16px; object-fit: contain; }
.login-provider-content { flex: 1; min-width: 0; min-height: 0; padding: 8px 0 0 16px; overflow: auto; display: flex; flex-direction: column; gap: 12px; }
.login-provider-heading { display: flex; align-items: center; gap: 8px; font-weight: 600; color: var(--color-text-1); }
.login-provider-heading img { width: 20px; height: 20px; object-fit: contain; }
.login-form { display: grid; gap: 10px; width: 100%; max-width: 360px; }
.pikpak-login-form { max-width: 100%; }
.pikpak-captcha-panel { display: grid; gap: 8px; padding: 10px; border: 1px solid var(--color-border-2); border-radius: 8px; background: var(--color-fill-1); }
.pikpak-captcha-panel-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.pikpak-captcha-actions { display: flex; align-items: center; gap: 2px; }
.pikpak-captcha-title { font-size: 13px; font-weight: 600; color: var(--color-text-1); }
.pikpak-captcha-status { font-size: 12px; color: var(--color-text-3); line-height: 1.4; }
.pikpak-captcha-status.ok { color: rgb(var(--green-6)); }
.pikpak-captcha-status.err { color: rgb(var(--red-6)); }
.pikpak-captcha-frame-wrap { width: 100%; height: 240px; border: 1px solid var(--color-border-2); border-radius: 6px; overflow: hidden; background: #fff; }
.login-modal-body--captcha .pikpak-captcha-frame-wrap { height: 300px; }
.pikpak-captcha-frame-wrap.empty { display: grid; place-items: center; background: var(--color-fill-2); }
.pikpak-captcha-placeholder { padding: 16px; font-size: 12px; color: var(--color-text-3); text-align: center; }
.pikpak-captcha-webview { width: 100%; height: 100%; border: 0; display: flex; }
.login-provider-content { flex: 1; min-width: 0; padding: 8px 0 0 16px; }
.login-provider-heading { display: flex; align-items: center; justify-content: center; gap: 8px; height: 28px; margin-bottom: 20px; font-weight: 600; }
.login-provider-heading img { width: 22px; height: 22px; object-fit: contain; }
.login-form { display: flex; width: 300px; margin: 64px auto 0; flex-direction: column; gap: 12px; }
.oauth-login { align-items: stretch; }
.s3-login-form { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 9px; width: 330px; margin: 28px auto 0; }
.wide { grid-column: 1 / -1; }
</style>
