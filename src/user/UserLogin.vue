<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useUserStore } from '../store'
import UserDAL from '../user/userdal'
import message from '../utils/message'
import { loginPikPak } from '../pikpak/auth'
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
})

const submitPikPakLogin = async () => {
  if (pikpakLoading.value || !pikpakUsername.value.trim() || !pikpakPassword.value) return message.error('请输入 PikPak 账号和密码')
  pikpakLoading.value = true
  try {
    await UserDAL.UserLogin(await loginPikPak(pikpakUsername.value.trim(), pikpakPassword.value), true)
    useUser.userShowLogin = false
  } catch (error: any) {
    message.error(error?.message || 'PikPak 登录失败')
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
    modal-class="userloginmodal"
    @before-open="handleModalOpen"
    @close="handleClose"
  >
    <div class="login-modal-body">
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
          <a-button html-type="submit" type="primary" long :loading="pikpakLoading">添加 PikPak 账号</a-button>
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
</style>

<style lang="less" scoped>
.login-modal-body { display: flex; width: 540px; height: 458px; overflow: hidden; transition: width 0.2s ease, height 0.2s ease; }
.login-provider-sidebar { flex: 0 0 148px; padding: 8px 8px 8px 0; overflow-y: auto; border-right: 1px solid var(--color-border-2); }
.login-provider-side-item { display: flex; align-items: center; width: 100%; min-height: 36px; gap: 8px; padding: 0 10px; margin-bottom: 3px; color: var(--color-text-2); background: transparent; border: 0; border-radius: 6px; cursor: pointer; text-align: left; }
.login-provider-side-item:hover, .login-provider-side-item.active { color: rgb(var(--primary-6)); background: var(--color-fill-2); }
.login-provider-icon { width: 16px; height: 16px; object-fit: contain; }
.login-provider-content { flex: 1; min-width: 0; min-height: 0; padding: 8px 0 0 16px; overflow: auto; display: flex; flex-direction: column; gap: 12px; }
.login-provider-heading { display: flex; align-items: center; gap: 8px; font-weight: 600; color: var(--color-text-1); }
.login-provider-heading img { width: 20px; height: 20px; object-fit: contain; }
.login-form { display: grid; gap: 10px; width: 100%; max-width: 360px; }
.login-provider-content { flex: 1; min-width: 0; padding: 8px 0 0 16px; }
.login-provider-heading { display: flex; align-items: center; justify-content: center; gap: 8px; height: 28px; margin-bottom: 20px; font-weight: 600; }
.login-provider-heading img { width: 22px; height: 22px; object-fit: contain; }
.login-form { display: flex; width: 300px; margin: 64px auto 0; flex-direction: column; gap: 12px; }
.oauth-login { align-items: stretch; }
.s3-login-form { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 9px; width: 330px; margin: 28px auto 0; }
.wide { grid-column: 1 / -1; }
</style>
