<script setup lang="ts">
import { computed, h, ref, watch } from 'vue'
import { ITokenInfo, useSettingStore, useUserStore } from '../store'
import UserDAL from '../user/userdal'
import Config from '../config'
import message from '../utils/message'
import DebugLog from '../utils/debuglog'
import { GetDeviceId, GetSignature } from '../aliapi/utils'
import AliUser from '../aliapi/user'
import AliHttp from '../aliapi/alihttp'
import { Input, Modal, Space } from '@arco-design/web-vue'
import { QRCode as AntQRCode } from 'ant-design-vue'
import { loginPikPak } from '../pikpak/auth'
import { completeQuarkQrLogin, pollQuarkQrStatus, requestQuarkQrCode } from '../quark/auth'
import { normalizeCloud139Token } from '../cloud139/auth'
import { Cloud189QrState, pollCloud189QrLogin, requestCloud189QrCode } from '../cloud189/auth'
import { GuangyaSmsState, generateGuangyaDid, requestGuangyaSmsCode, submitGuangyaSmsCode } from '../guangya/auth'
import { getDriveProviderMeta } from '../utils/driveProvider'
import { createWebDavConnection, createWebDavUserToken, saveWebDavConnection, testWebDavConnection } from '../utils/webdavClient'
import { createS3Connection, createS3UserToken, saveS3Connection, testS3Connection } from '../utils/s3Client'
import { ALIYUN_APP_ID, ALIYUN_APP_SECRET } from '../secrets.generated'

const useUser = useUserStore()
const settingStore = useSettingStore()
const loginCur = ref(1)
const loginToken = ref<ITokenInfo>()
const loginStatus = ref<'wait' | 'error' | 'finish' | 'process'>('process')
const loginLoading = ref(true)
const client_id = ref(ALIYUN_APP_ID)
const client_secret = ref(ALIYUN_APP_SECRET)

const intervalId = ref()
const qrCodeUrl = ref('')
const qrCodeStatusType = ref()
const qrCodeStatusTips = ref()

type LoginProvider = 'aliyun' | '139' | '189' | 'guangya' | 'pikpak' | 'quark' | 'webdav' | 's3'

const loginProvider = ref<LoginProvider>('aliyun')
const loginProviders: LoginProvider[] = ['aliyun', '139', '189', 'guangya', 'pikpak', 'quark', 'webdav', 's3']
const getLoginProviderMeta = (provider: LoginProvider) => getDriveProviderMeta(provider)
const activeLoginProviderMeta = computed(() => getLoginProviderMeta(loginProvider.value))
const pikpakUsername = ref('')
const pikpakPassword = ref('')
const pikpakLoading = ref(false)
const quarkLoading = ref(false)
const quarkTips = ref('请使用夸克 App 扫码')
const quarkQrToken = ref('')
const quarkQrUrl = ref('')
const quarkQrStatusType = ref<'info' | 'success' | 'warning' | 'error'>('info')
const cloud139Authorization = ref('')
const cloud139Loading = ref(false)
const cloud189Loading = ref(false)
const cloud189Tips = ref('请使用天翼云盘 App 扫码')
const cloud189QrState = ref<Cloud189QrState | null>(null)
const cloud189QrStatusType = ref<'info' | 'success' | 'warning' | 'error'>('info')
const cloud189QrUrl = ref('')
const guangyaPhone = ref('')
const guangyaCode = ref('')
const guangyaDeviceId = ref(generateGuangyaDid())
const guangyaSmsState = ref<GuangyaSmsState | null>(null)
const guangyaLoading = ref(false)
const webDavForm = ref({ name: '', url: '', username: '', password: '', rootPath: '/' })
const webDavLoading = ref(false)
const s3Form = ref({ name: '', endpoint: '', region: 'us-east-1', accessKeyId: '', secretAccessKey: '', sessionToken: '', bucket: '', rootPrefix: '', forcePathStyle: true })
const s3Loading = ref(false)
let quarkTimer: any = null
let quarkPolling = false
let cloud189Timer: any = null
let cloud189Polling = false
let loginOpenTimer: any = null
let aliyunLoginHandled = false

const getAliyunLoginWebview = () => document.getElementById('loginiframe') as any

const stopAliyunLoginWebview = () => {
  const webview = getAliyunLoginWebview()
  if (!webview) return
  try {
    webview.stop()
  } catch {
    // ignore webview stop errors while switching providers
  }
}

const clearOpenTimers = () => {
  if (loginOpenTimer) {
    clearTimeout(loginOpenTimer)
    loginOpenTimer = null
  }
}

const handleModalOpen = () => {
  const stored = localStorage.getItem('login_provider')
  if (
    stored === 'aliyun' ||
    stored === '139' ||
    stored === '189' ||
    stored === 'guangya' ||
    stored === 'pikpak' ||
    stored === 'quark' ||
    stored === 'webdav' ||
    stored === 's3'
  ) {
    loginProvider.value = stored
  }
  if (loginProvider.value === 'pikpak') {
    loginLoading.value = false
  } else if (loginProvider.value === 'quark') {
    handleOpenQuark()
  } else if (loginProvider.value === '139') {
    loginLoading.value = false
  } else if (loginProvider.value === '189') {
    handleOpen189()
  } else if (loginProvider.value === 'guangya') {
    loginLoading.value = false
  } else if (loginProvider.value === 'webdav') {
    loginLoading.value = false
  } else if (loginProvider.value === 's3') {
    loginLoading.value = false
  } else {
    handleOpen()
  }
}

watch(loginProvider, () => {
  if (!useUser.userShowLogin) return
  clearOpenTimers()
  if (loginProvider.value !== 'quark') clearQuarkTimer()
  if (loginProvider.value !== '189') clearCloud189Timer()
  if (loginProvider.value !== 'aliyun') stopAliyunLoginWebview()
  if (loginProvider.value === 'pikpak') {
    loginLoading.value = false
  } else if (loginProvider.value === 'quark') {
    handleOpenQuark()
  } else if (loginProvider.value === '139') {
    loginLoading.value = false
  } else if (loginProvider.value === '189') {
    handleOpen189()
  } else if (loginProvider.value === 'guangya') {
    loginLoading.value = false
  } else if (loginProvider.value === 'webdav') {
    loginLoading.value = false
  } else if (loginProvider.value === 's3') {
    loginLoading.value = false
  } else {
    handleOpen()
  }
})

const cb = (val: any) => {
  settingStore.updateStore(val)
}

function b64decode(e: string) {
  const t = atob(e)
  let r = t.length
  const n = new Uint8Array(r)
  while (r--) n[r] = t.charCodeAt(r)
  return new Blob([n])
}

function readData(e: string) {
  return new Promise<string>(function (resolve, reject) {
    const n = b64decode(e)
    const i = new FileReader()
    i.onloadend = function (e) {
      resolve((e?.target?.result as string | undefined) || '')
    }
    i.onerror = function (e) {
      return reject(e)
    }
    i.readAsText(n, 'gbk')
  })
}

const refreshStepTips = (status: 'error' | 'finish' | 'process', index: number) => {
  loginStatus.value = status
  loginLoading.value = index !== loginCur.value
  loginCur.value = index
}

const refreshQrCodeStatus = (codeUrl: string = '', type: string = 'info', tips: string = '请用阿里云盘 App 扫码') => {
  qrCodeUrl.value = codeUrl
  qrCodeStatusType.value = type
  qrCodeStatusTips.value = tips
}

const handleOpen = () => {
  clearOpenTimers()
  loginOpenTimer = setTimeout(() => {
    if (loginProvider.value !== 'aliyun' || !useUser.userShowLogin) return
    const webview = getAliyunLoginWebview()
    if (!webview) {
      message.error('严重错误：无法打开登录弹窗，请退出Mnemo后重新运行')
      return
    }
    if (import.meta.env.DEV) {
      try {
        webview.openDevTools({ mode: 'bottom', activate: false })
      } catch (err: any) {
        DebugLog.mSaveWarning('Aliyun login webview DevTools open failed ' + (err?.message || err))
      }
    }
    aliyunLoginHandled = false
    const extractBizExt = (payload: string) => {
      try {
        const parsed = JSON.parse(payload)
        if (parsed?.bizExt) return String(parsed.bizExt)
      } catch {
        // Some versions of the login page print a JavaScript object instead of JSON.
      }
      const match = payload.match(/["']?bizExt["']?\s*[:=]\s*["']([^"']+)["']/i)
      return match?.[1] || ''
    }
    const handleLoginPayload = (payload: string) => {
      try {
        const parsed = JSON.parse(payload)
        if (parsed?.code && !aliyunLoginHandled) {
          aliyunLoginHandled = true
          loginStepFirst(payload)
          try {
            webview.stop()
          } catch {
            // ignore navigation stop errors after the OAuth callback is received
          }
          return true
        }
      } catch {
        // Continue with the legacy bizExt parser below.
      }
      const bizExt = extractBizExt(payload)
      if (aliyunLoginHandled || !bizExt) return false
      aliyunLoginHandled = true
      loginStepFirst(JSON.stringify({ bizExt }))
      try {
        webview.stop()
      } catch {
        // ignore navigation stop errors after the login callback is received
      }
      return true
    }
    const handleLoginNavigation = (event: any) => {
      const url = event?.url || ''
      if (!url) return
      try {
        const parsed = new URL(url)
        const code = parsed.searchParams.get('code') || ''
        if (code && handleLoginPayload(JSON.stringify({ code }))) {
          event?.preventDefault?.()
          return
        }
        if (!url.includes('bizExt')) return
        const bizExt = parsed.searchParams.get('bizExt') || new URLSearchParams(parsed.hash.replace(/^#\??/, '')).get('bizExt')
        if (bizExt && handleLoginPayload(JSON.stringify({ bizExt }))) event?.preventDefault?.()
      } catch (err: any) {
        DebugLog.mSaveWarning('Aliyun login callback parse failed ' + (err?.message || err))
      }
    }
    webview.addEventListener('will-navigate', handleLoginNavigation)
    webview.addEventListener('did-navigate', handleLoginNavigation)
    webview.addEventListener('did-redirect-navigation', handleLoginNavigation)
    webview.addEventListener('did-navigate-in-page', handleLoginNavigation)
    webview.addEventListener('console-message', (e: any) => {
      const msg = e.message || ''
      loginLoading.value = false
      handleLoginPayload(msg)
    })
    const load = webview.loadURL(Config.loginUrl, { httpReferrer: Config.referer })
    if (load?.catch) {
      load.catch((err: any) => {
        loginLoading.value = false
        if (loginProvider.value === 'aliyun' && useUser.userShowLogin) DebugLog.mSaveWarning('Aliyun login webview load failed ' + (err?.message || err))
      })
    }
    webview.addEventListener('did-finish-load', () => {
      loginLoading.value = false
    })
    webview.addEventListener('did-fail-load', () => {
      loginLoading.value = false
    })
  }, 1000)
}

const handleClose = () => {
  loginLoading.value = true
  client_id.value = ALIYUN_APP_ID
  client_secret.value = ALIYUN_APP_SECRET
  clearInterval(intervalId.value)
  clearOpenTimers()
  stopAliyunLoginWebview()
  clearQuarkTimer()
  clearCloud189Timer()
  refreshStepTips('process', 1)
  refreshQrCodeStatus()
  pikpakPassword.value = ''
  pikpakLoading.value = false
  quarkLoading.value = false
  quarkTips.value = '请使用夸克 App 扫码'
  quarkQrToken.value = ''
  quarkQrUrl.value = ''
  quarkQrStatusType.value = 'info'
  cloud139Authorization.value = ''
  cloud139Loading.value = false
  cloud189Loading.value = false
  cloud189Tips.value = '请使用天翼云盘 App 扫码'
  cloud189QrState.value = null
  cloud189QrStatusType.value = 'info'
  cloud189QrUrl.value = ''
  guangyaCode.value = ''
  guangyaSmsState.value = null
  guangyaLoading.value = false
  webDavForm.value = { name: '', url: '', username: '', password: '', rootPath: '/' }
  webDavLoading.value = false
  s3Form.value = { name: '', endpoint: '', region: 'us-east-1', accessKeyId: '', secretAccessKey: '', sessionToken: '', bucket: '', rootPrefix: '', forcePathStyle: true }
  s3Loading.value = false
}

const submitWebDavLogin = async () => {
  const form = webDavForm.value
  if (!form.name.trim() || !form.url.trim() || !form.username.trim() || !form.password.trim()) {
    message.error('请填写 WebDAV 名称、地址、用户名和密码')
    return
  }

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
  if (s3Loading.value) return
  const form = s3Form.value
  if (!form.name.trim() || !form.bucket.trim() || !form.accessKeyId.trim() || !form.secretAccessKey.trim()) {
    message.error('请填写 S3 名称、Bucket、Access Key 和 Secret Key')
    return
  }
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

const submitPikPakLogin = async () => {
  if (pikpakLoading.value) return
  const username = pikpakUsername.value.trim()
  if (!username || !pikpakPassword.value) {
    message.error('请输入 PikPak 账号和密码')
    return
  }
  pikpakLoading.value = true
  try {
    const token = await loginPikPak(username, pikpakPassword.value)
    await UserDAL.UserLogin(token, true)
    useUserStore().userShowLogin = false
  } catch (err: any) {
    message.error(err?.message || 'PikPak 登录失败')
  } finally {
    pikpakLoading.value = false
  }
}

const submitCloud139Login = async () => {
  if (cloud139Loading.value) return
  if (!cloud139Authorization.value.trim()) {
    message.error('请输入 139 云盘 Authorization')
    return
  }
  cloud139Loading.value = true
  try {
    const token = normalizeCloud139Token(cloud139Authorization.value.trim())
    await UserDAL.UserLogin(token, true)
    useUserStore().userShowLogin = false
  } catch (err: any) {
    message.error(err?.message || '139 云盘登录失败')
  } finally {
    cloud139Loading.value = false
  }
}

const sendGuangyaSmsCode = async () => {
  if (guangyaLoading.value) return
  const phone = guangyaPhone.value.trim()
  if (!phone) {
    message.error('请输入光鸭云盘手机号，例如 +86 13800138000')
    return
  }
  guangyaLoading.value = true
  try {
    guangyaDeviceId.value = guangyaDeviceId.value || generateGuangyaDid()
    guangyaSmsState.value = await requestGuangyaSmsCode(phone, guangyaDeviceId.value)
    message.success('验证码已发送')
  } catch (err: any) {
    message.error(err?.message || '发送光鸭云盘验证码失败')
  } finally {
    guangyaLoading.value = false
  }
}

const submitGuangyaLogin = async () => {
  if (guangyaLoading.value) return
  if (!guangyaSmsState.value) {
    message.error('请先发送短信验证码')
    return
  }
  const code = guangyaCode.value.trim()
  if (!code) {
    message.error('请输入短信验证码')
    return
  }
  guangyaLoading.value = true
  try {
    const token = await submitGuangyaSmsCode(guangyaSmsState.value, code, guangyaDeviceId.value)
    await UserDAL.UserLogin(token, true)
    useUserStore().userShowLogin = false
  } catch (err: any) {
    message.error(err?.message || '光鸭云盘登录失败')
  } finally {
    guangyaLoading.value = false
  }
}

const clearCloud189Timer = () => {
  if (cloud189Timer) {
    clearTimeout(cloud189Timer)
    cloud189Timer = null
  }
}

const handleOpen189 = async () => {
  clearOpenTimers()
  clearCloud189Timer()
  if (cloud189Loading.value) return
  loginLoading.value = true
  cloud189Loading.value = true
  cloud189QrStatusType.value = 'info'
  cloud189Tips.value = '正在获取天翼云盘登录二维码...'
  cloud189QrUrl.value = ''
  cloud189QrState.value = null
  try {
    const state = await requestCloud189QrCode()
    cloud189QrState.value = state
    cloud189QrUrl.value = state.qrUrl
    cloud189Tips.value = '请使用天翼云盘 App 扫码'
    loginLoading.value = false
    pollCloud189Status()
  } catch (err: any) {
    cloud189QrStatusType.value = 'error'
    cloud189Tips.value = err?.message || '获取天翼云盘二维码失败'
    message.error(cloud189Tips.value)
    loginLoading.value = false
  } finally {
    cloud189Loading.value = false
  }
}

const pollCloud189Status = async () => {
  if (loginProvider.value !== '189' || !useUser.userShowLogin || !cloud189QrState.value) return
  if (cloud189Polling) return
  cloud189Polling = true
  const state = cloud189QrState.value
  try {
    const result = await pollCloud189QrLogin(state)
    if (cloud189QrState.value !== state) return
    if (result.status === 'success' && result.token) {
      clearCloud189Timer()
      cloud189QrStatusType.value = 'success'
      cloud189Tips.value = '扫码成功，正在登录...'
      await UserDAL.UserLogin(result.token, true)
      useUserStore().userShowLogin = false
      return
    }
    if (result.status === 'expired' || result.status === 'failed') {
      clearCloud189Timer()
      cloud189QrStatusType.value = result.status === 'expired' ? 'warning' : 'error'
      cloud189Tips.value = result.message
      return
    }
    cloud189QrStatusType.value = 'info'
    cloud189Tips.value = result.message || '请使用天翼云盘 App 扫码'
    cloud189Timer = setTimeout(pollCloud189Status, 1500)
  } catch (err: any) {
    cloud189QrStatusType.value = 'error'
    cloud189Tips.value = err?.message || '获取天翼云盘扫码状态失败'
    cloud189Timer = setTimeout(pollCloud189Status, 2000)
  } finally {
    cloud189Polling = false
  }
}

const clearQuarkTimer = () => {
  if (quarkTimer) {
    clearTimeout(quarkTimer)
    quarkTimer = null
  }
}

const handleOpenQuark = async () => {
  clearOpenTimers()
  clearQuarkTimer()
  if (quarkLoading.value) return
  loginLoading.value = true
  quarkLoading.value = true
  quarkQrUrl.value = ''
  quarkQrToken.value = ''
  quarkQrStatusType.value = 'info'
  quarkTips.value = '正在获取夸克登录二维码...'
  try {
    const qr = await requestQuarkQrCode()
    quarkQrToken.value = qr.token
    quarkQrUrl.value = qr.qrUrl
    quarkTips.value = '请使用夸克 App 扫码'
    loginLoading.value = false
    quarkLoading.value = false
    pollQuarkStatus()
  } catch (err: any) {
    quarkQrStatusType.value = 'error'
    quarkTips.value = err?.message || '获取夸克登录二维码失败'
    message.error(quarkTips.value)
    loginLoading.value = false
    quarkLoading.value = false
  }
}

const pollQuarkStatus = async () => {
  if (loginProvider.value !== 'quark' || !useUser.userShowLogin || !quarkQrToken.value) return
  if (quarkPolling) return
  quarkPolling = true
  const pollingToken = quarkQrToken.value
  try {
    const status = await pollQuarkQrStatus(pollingToken)
    if (quarkQrToken.value !== pollingToken) return
    if (status.status === 'confirmed') {
      clearQuarkTimer()
      quarkQrToken.value = ''
      quarkLoading.value = true
      quarkQrStatusType.value = 'success'
      quarkTips.value = '扫码成功，正在登录...'
      try {
        const token = await completeQuarkQrLogin(status.serviceTicket)
        await UserDAL.UserLogin(token, true)
        useUserStore().userShowLogin = false
      } catch (err: any) {
        quarkQrStatusType.value = 'error'
        quarkTips.value = err?.message || '获取夸克账号信息失败'
        message.error(quarkTips.value)
      }
      return
    }
    if (status.status === 'expired') {
      clearQuarkTimer()
      quarkQrStatusType.value = 'warning'
      quarkTips.value = status.message || '二维码已失效，请刷新'
      return
    }
    if (status.status === 'failed') {
      clearQuarkTimer()
      quarkQrStatusType.value = 'error'
      quarkTips.value = status.message || '夸克登录失败'
      return
    }
    quarkQrStatusType.value = 'info'
    quarkTips.value = status.message || '请使用夸克 App 扫码'
    quarkTimer = setTimeout(pollQuarkStatus, 1500)
  } catch (err: any) {
    if (quarkQrToken.value !== pollingToken) return
    quarkQrStatusType.value = 'error'
    quarkTips.value = err?.message || '获取夸克扫码状态失败'
    if (loginProvider.value === 'quark' && useUser.userShowLogin && quarkQrToken.value) {
      quarkTimer = setTimeout(pollQuarkStatus, 2000)
    }
  } finally {
    quarkPolling = false
    quarkLoading.value = false
  }
}

const loginStepFirst = async (msg: string) => {
  let data: { bizExt?: string; code?: string } = {}
  try {
    data = JSON.parse(msg)
  } catch {}
  if (!data.bizExt && !data.code) {
    refreshStepTips('error', 1)
    DebugLog.mSaveDanger('登录失败：' + msg)
    return
  }
  const resultPromise = data.code
    ? AliUser.LoginByOAuthCode(data.code).then((resp: any) => {
        if (!AliHttp.IsSuccess(resp.code)) throw new Error(resp.body?.message || resp.body?.code || `OAuth code exchange failed: ${resp.code}`)
        const body = resp.body?.token_info || resp.body?.tokenInfo || resp.body
        return {
          accessToken: body.accessToken || body.access_token,
          refreshToken: body.refreshToken || body.refresh_token,
          tokenType: body.tokenType || body.token_type,
          expiresIn: body.expiresIn || body.expires_in,
          userId: body.userId || body.user_id,
          userName: body.userName || body.user_name,
          avatar: body.avatar,
          nickName: body.nickName || body.nick_name,
          defaultSboxDriveId: body.defaultSboxDriveId || body.default_sbox_drive_id,
          role: body.role,
          status: body.status,
          expireTime: body.expireTime || body.expire_time,
          state: body.state,
          dataPinSetup: body.dataPinSetup || body.data_pin_setup,
          isFirstLogin: body.isFirstLogin || body.is_first_login,
          needRpVerify: body.needRpVerify || body.need_rp_verify
        }
      })
    : readData(data.bizExt || '').then((jsonstr: string) => JSON.parse(jsonstr).pds_login_result)
  resultPromise
    .then((result: any) => {
      try {
        const deviceId = GetDeviceId(result.userId.toString())
        const { signature } = GetSignature(0, result.userId.toString(), deviceId)
        const token: ITokenInfo = {
          tokenfrom: 'aliyun',
          access_token: result.accessToken,
          refresh_token: result.refreshToken,
          session_expires_in: 0,
          open_api_token_type: '',
          open_api_access_token: '',
          open_api_refresh_token: '',
          open_api_expires_in: 0,
          expires_in: result.expiresIn,
          token_type: result.tokenType,
          user_id: result.userId,
          user_name: result.userName,
          avatar: result.avatar,
          nick_name: result.nickName,
          default_drive_id: '',
          default_sbox_drive_id: result.defaultSboxDriveId,
          resource_drive_id: '',
          backup_drive_id: '',
          sbox_drive_id: '',
          role: result.role,
          status: result.status,
          expire_time: result.expireTime,
          state: result.state,
          pin_setup: result.dataPinSetup,
          is_first_login: result.isFirstLogin,
          need_rp_verify: result.needRpVerify,
          name: '',
          spu_id: '',
          is_expires: false,
          used_size: 0,
          total_size: 0,
          free_size: 0,
          space_expire: false,
          spaceinfo: '',
          pic_drive_id: '',
          vipname: '',
          vipexpire: '',
          vipIcon: '',
          device_id: deviceId,
          signature: signature,
          signInfo: {
            signMon: -1,
            signDay: -1
          }
        }
        loginToken.value = token
        if (settingStore.uiEnableOpenApiType === 'custom') {
          client_id.value = settingStore.uiOpenApiClientId.trim()
          client_secret.value = settingStore.uiOpenApiClientSecret.trim()
        } else {
          client_id.value = ALIYUN_APP_ID
          client_secret.value = ALIYUN_APP_SECRET
        }
        refreshStepTips('process', 2)
        loginStepSecond(token)
      } catch (err: any) {
        refreshStepTips('error', 1)
        message.error('登录失败：' + (err.message || '解析失败'))
        DebugLog.mSaveDanger('登录失败：' + (err.message || '解析失败'), JSON.stringify(err))
      }
    })
    .catch((err: any) => {
      refreshStepTips('error', 1)
      message.error('登录结果读取失败：' + (err?.message || '请重试'))
      DebugLog.mSaveDanger('Aliyun login result read failed', err)
    })
}

const loginStepSecond = async (token: ITokenInfo) => {
  if (!token) {
    refreshStepTips('process', 1)
    message.error('请重新登录')
    return
  }
  if (!client_id.value.trim() || !client_secret.value.trim()) {
    await settingStore.updateStore({ uiEnableOpenApiType: 'custom' })
    refreshStepTips('error', 2)
    handlerChangeType()
    return
  }
  loginLoading.value = false
  clearInterval(intervalId.value)
  let codeUrl = ''
  try {
    codeUrl = await AliUser.OpenApiQrCodeUrl(client_id.value, client_secret.value, 250, 250)
  } catch (err: any) {
    refreshQrCodeStatus('', 'error', '获取第二个二维码失败，请检查网络或开发者配置')
    refreshStepTips('error', 2)
    DebugLog.mSaveDanger('Aliyun second QR code request failed', err)
    return
  }
  if (!codeUrl) {
    refreshQrCodeStatus('', 'error', '获取二维码失败')
    refreshStepTips('error', 2)
    handlerChangeType()
    return
  }
  refreshQrCodeStatus(codeUrl, 'info', '状态：等待扫码登录')
  refreshStepTips('process', 2)
  // 监听状态
  intervalId.value = setInterval(async () => {
    try {
      const result = await AliUser.OpenApiQrCodeStatus(codeUrl)
      if (!result || typeof result !== 'object') return
      const { authCode, statusCode, statusType, statusTips } = result
      if (!statusCode) {
        refreshQrCodeStatus()
        clearInterval(intervalId.value)
        return
      }
      refreshQrCodeStatus(codeUrl, statusType, statusTips)
      if (statusCode === 'QRCodeExpired') {
        clearInterval(intervalId.value)
        refreshQrCodeStatus()
        return
      }
      if (authCode && statusCode === 'LoginSuccess') {
        // 构造请求体
        await AliUser.OpenApiLoginByAuthCode(token, client_id.value, client_secret.value, authCode)
        loginSuccess(token)
        clearInterval(intervalId.value)
      }
    } catch (err: any) {
      clearInterval(intervalId.value)
      refreshQrCodeStatus('', 'error', '第二个二维码状态获取失败，请点击二维码刷新')
      DebugLog.mSaveWarning('Aliyun second QR code status failed', err)
    }
  }, 1500)
}

const handlerChangeType = () => {
  clearInterval(intervalId.value)
  refreshQrCodeStatus()
  if (settingStore.uiEnableOpenApiType === 'custom') {
    Modal.open({
      title: '输入开发者账号',
      bodyStyle: { minWidth: '340px' },
      content: () =>
        h(Space, { direction: 'vertical' }, () => [
          h(Input, {
            type: 'text',
            tabindex: '-1',
            allowClear: true,
            modelValue: settingStore.uiOpenApiClientId.trim(),
            style: { width: '340px' },
            placeholder: '客户端ID',
            'onUpdate:modelValue': (e) => cb({ uiOpenApiClientId: e.trim() })
          }),
          h(Input, {
            type: 'text',
            tabindex: '-1',
            allowClear: true,
            modelValue: settingStore.uiOpenApiClientSecret.trim(),
            style: { width: '340px' },
            placeholder: '客户端密钥',
            'onUpdate:modelValue': (e) => cb({ uiOpenApiClientSecret: e.trim() })
          })
        ]),
      okText: '确认',
      cancelText: '取消',
      onBeforeOk: async (e: any) => {
        if (settingStore.uiOpenApiClientId && settingStore.uiOpenApiClientSecret) {
          client_id.value = settingStore.uiOpenApiClientId
          client_secret.value = settingStore.uiOpenApiClientSecret
          handleRefreshQrCodeUrl()
          return true
        } else {
          message.error('请输入开发者账号')
          return false
        }
      }
    })
  } else {
    client_id.value = ALIYUN_APP_ID
    client_secret.value = ALIYUN_APP_SECRET
    handleRefreshQrCodeUrl()
  }
}

const handleRefreshQrCodeUrl = () => {
  refreshQrCodeStatus()
  clearInterval(intervalId.value)
  loginStepSecond(loginToken.value!!)
}

const loginSuccess = (token: ITokenInfo) => {
  UserDAL.UserLogin(token, true)
    .then(() => {
      if (window.WebClearCookies) {
        window.WebClearCookies({
          origin: 'https://auth.aliyundrive.com',
          storages: ['cookies', 'localstorage']
        })
      }
      refreshStepTips('process', 3)
      refreshQrCodeStatus()
      useUserStore().userShowLogin = false
    })
    .catch(() => {
      useUserStore().userShowLogin = false
      if (window.WebClearCookies) {
        window.WebClearCookies({
          origin: 'https://auth.aliyundrive.com',
          storages: ['cookies', 'localstorage']
        })
      }
      refreshQrCodeStatus()
    })
}
</script>

<template>
  <a-modal title="网盘账号登录" v-model:visible="useUser.userShowLogin" :mask-closable="false" unmount-on-close :footer="false" class="userloginmodal" @before-open="handleModalOpen" @close="handleClose">
    <div class="modalbody login-modal-body">
      <aside class="login-provider-sidebar">
        <button v-for="provider in loginProviders" :key="provider" class="login-provider-side-item" :class="{ active: loginProvider === provider }" :title="getLoginProviderMeta(provider).label" @click="loginProvider = provider">
          <span v-if="getLoginProviderMeta(provider).icon" class="login-provider-icon">
            <img :src="getLoginProviderMeta(provider).icon" :alt="getLoginProviderMeta(provider).label" />
          </span>
          <span class="login-provider-side-label">{{ getLoginProviderMeta(provider).label }}</span>
        </button>
      </aside>

      <section class="login-provider-content">
        <div class="login-provider-heading" :title="activeLoginProviderMeta.label">
          <span v-if="activeLoginProviderMeta.icon" class="login-provider-heading-icon">
            <img :src="activeLoginProviderMeta.icon" :alt="activeLoginProviderMeta.label" />
          </span>
          <span>{{ activeLoginProviderMeta.label }}</span>
        </div>

        <div v-if="loginProvider === 'aliyun'">
          <a-steps v-model:current="loginCur" :status="loginStatus">
            <a-step description="扫码或账号登录">第一次扫码</a-step>
            <a-step description="手机授权">第二次扫码</a-step>
          </a-steps>
          <div id="logindiv">
            <div class="logincontent">
              <div id="loginframediv" class="loginframe">
                <a-spin class="loading" :size="32" v-if="loginLoading" tip="加载中，请稍后..." />
                <Webview id="loginiframe" v-show="!loginLoading && loginCur === 1"
                         partition="persist:mnemo-aliyun-login"
                         plugins nodeintegration disablewebsecurity
                         webpreferences="allowRunningInsecureContent"
                         src="about:blank" style="width: 100%; height: 400px; border: none; overflow: hidden" />
                <div class="qrcodeframe" v-if="loginCur === 2 && !loginLoading">
                  <a-image width="250" height="250" :hide-footer="true" :preview="false" :show-loader="true" @click="handleRefreshQrCodeUrl" style="display: inline-block" :src="qrCodeUrl"></a-image>
                  <a-alert banner center :show-icon="false" :type="qrCodeStatusType">
                    {{ qrCodeStatusTips }}
                  </a-alert>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div v-else-if="loginProvider === '139'">
          <div id="logindiv">
            <div class="logincontent">
              <div class="pikpak-login-form">
                <a-textarea v-model="cloud139Authorization" placeholder="粘贴 139 云盘 Authorization（Basic 后面的完整值也可以）" :auto-size="{ minRows: 4, maxRows: 6 }" allow-clear />
                <a-button type="primary" long :loading="cloud139Loading" @click="submitCloud139Login">登录 139 云盘</a-button>
              </div>
            </div>
          </div>
        </div>

        <div v-else-if="loginProvider === '189'">
          <div id="logindiv">
            <div class="logincontent">
              <div id="loginframediv" class="loginframe">
                <a-spin class="loading" :size="32" v-if="loginLoading" tip="加载中，请稍后..." />
                <div class="qrcodeframe" v-if="!loginLoading">
                  <div class="cloud189-qrcode-wrap">
                    <AntQRCode :value="cloud189QrUrl || 'cloud189'" :size="250" color="#000" bg-color="#fff" />
                  </div>
                  <a-alert banner center :show-icon="false" :type="cloud189QrStatusType">
                    {{ cloud189Tips }}
                  </a-alert>
                </div>
              </div>
              <div class="quark-login-toolbar" v-if="!loginLoading">
                <a-button type="primary" :loading="cloud189Loading" @click="handleOpen189">刷新二维码</a-button>
              </div>
            </div>
          </div>
        </div>

        <div v-else-if="loginProvider === 'guangya'">
          <div id="logindiv">
            <div class="logincontent">
              <div class="pikpak-login-form">
                <a-input v-model="guangyaPhone" placeholder="手机号，例如 +86 13800138000" allow-clear />
                <a-input v-model="guangyaCode" placeholder="短信验证码" allow-clear @press-enter="submitGuangyaLogin" />
                <a-space direction="vertical" fill>
                  <a-button type="outline" long :loading="guangyaLoading" @click="sendGuangyaSmsCode">{{ guangyaSmsState ? '重新发送验证码' : '发送验证码' }}</a-button>
                  <a-button type="primary" long :loading="guangyaLoading" @click="submitGuangyaLogin">登录光鸭云盘</a-button>
                </a-space>
              </div>
            </div>
          </div>
        </div>

        <div v-else-if="loginProvider === 'pikpak'">
          <div id="logindiv">
            <div class="logincontent">
              <div class="pikpak-login-form">
                <a-input v-model="pikpakUsername" placeholder="PikPak 邮箱 / 手机号 / 用户名" allow-clear />
                <a-input-password v-model="pikpakPassword" placeholder="PikPak 密码" allow-clear @press-enter="submitPikPakLogin" />
                <a-button type="primary" long :loading="pikpakLoading" @click="submitPikPakLogin">登录 PikPak</a-button>
              </div>
            </div>
          </div>
        </div>

        <div v-else-if="loginProvider === 'quark'">
          <div id="logindiv">
            <div class="logincontent quark-logincontent">
              <a-spin class="loading" :size="32" v-if="loginLoading" tip="加载中，请稍后..." />
              <div class="qrcodeframe" v-if="!loginLoading">
                <AntQRCode v-if="quarkQrUrl" :value="quarkQrUrl" :size="250" color="#000" bg-color="#fff" @click="handleOpenQuark" />
                <a-alert banner center :show-icon="false" :type="quarkQrStatusType">
                  {{ quarkTips }}
                </a-alert>
              </div>
              <div class="quark-login-toolbar" v-if="!loginLoading">
                <a-button type="primary" :loading="quarkLoading" @click="handleOpenQuark">刷新二维码</a-button>
              </div>
            </div>
          </div>
        </div>

        <div v-else-if="loginProvider === 'webdav'">
          <div id="logindiv">
            <div class="logincontent">
              <form class="webdav-login-form" @submit.prevent="submitWebDavLogin">
                <a-input v-model="webDavForm.name" placeholder="连接名称（必填）" allow-clear />
                <a-input v-model="webDavForm.url" placeholder="WebDAV 地址，例如 https://example.com/dav" allow-clear />
                <a-input v-model="webDavForm.username" placeholder="用户名" allow-clear />
                <a-input-password v-model="webDavForm.password" placeholder="密码" allow-clear />
                <a-input v-model="webDavForm.rootPath" placeholder="挂载路径，默认 /" allow-clear />
                <a-button html-type="submit" type="primary" long :loading="webDavLoading">连接 WebDAV</a-button>
              </form>
            </div>
          </div>
        </div>

        <div v-else-if="loginProvider === 's3'">
          <div id="logindiv">
            <div class="logincontent">
              <form class="s3-login-form" @submit.prevent="submitS3Login">
                <a-input v-model="s3Form.name" class="s3-field-wide" placeholder="连接名称（必填且不可重复）" allow-clear />
                <a-input v-model="s3Form.endpoint" class="s3-field-wide" placeholder="Endpoint（AWS 官方可留空）" allow-clear />
                <a-input v-model="s3Form.region" placeholder="Region" allow-clear />
                <a-input v-model="s3Form.bucket" placeholder="Bucket" allow-clear />
                <a-input v-model="s3Form.accessKeyId" placeholder="Access Key ID" allow-clear />
                <a-input-password v-model="s3Form.secretAccessKey" placeholder="Secret Access Key" allow-clear />
                <a-input-password v-model="s3Form.sessionToken" class="s3-field-wide" placeholder="Session Token（可选）" allow-clear />
                <a-input v-model="s3Form.rootPrefix" placeholder="根前缀（可选）" allow-clear />
                <label class="s3-path-style">
                  <a-switch v-model="s3Form.forcePathStyle" size="small" />
                  路径样式
                </label>
                <a-button class="s3-field-wide" html-type="submit" type="primary" long :loading="s3Loading">连接 S3</a-button>
              </form>
            </div>
          </div>
        </div>
      </section>
    </div>
  </a-modal>
</template>
<style lang="less" scoped>
#logindiv {
  overflow: hidden;
  text-align: center;

  .logincontent {
    position: relative;
    width: 348px;
    height: 367px;
    min-height: 400px;
    margin: 0 auto;
    overflow: hidden;
    text-align: center;

    .loginframe {
      overflow: hidden;
      position: relative;
      width: 100%;
      height: 100%
    }

    .qrcodeframe {
      border-radius: 10px;
      padding: 5px;
      box-shadow: grey 0 0 10px;
      margin: 40px 15px 15px 15px;
    }

    .cloud189-qrcode-wrap {
      display: flex;
      justify-content: center;
    }

    .loading {
      min-height: 60px;
      position: absolute;
      top: 50%;
      left: 50%;
      transform: translate(-50%, -50%);
    }
  }
}

.userloginmodal .arco-modal-body {
  min-height: 440px;
  padding: 0 16px 16px 16px !important;
}

.login-modal-body {
  display: flex;
  width: 540px;
  height: 458px;
  overflow: hidden;
}

.login-provider-sidebar {
  flex: 0 0 148px;
  height: 100%;
  padding: 8px 8px 8px 0;
  overflow-y: auto;
  overflow-x: hidden;
  border-right: 1px solid var(--color-border-2);
}

.login-provider-sidebar::-webkit-scrollbar {
  width: 6px;
}

.login-provider-sidebar::-webkit-scrollbar-thumb {
  background: var(--color-fill-3);
  border-radius: 999px;
}

.login-provider-side-item {
  width: 100%;
  min-height: 36px;
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 10px;
  margin-bottom: 3px;
  color: var(--color-text-2);
  background: transparent;
  border: 0;
  border-radius: 6px;
  cursor: pointer;
  text-align: left;
  transition:
    background-color 0.15s,
    color 0.15s;
}

.login-provider-side-item:hover {
  color: var(--color-text-1);
  background: var(--color-fill-2);
}

.login-provider-side-item.active {
  color: rgb(var(--primary-6));
  background: rgba(var(--primary-6), 0.12);
  font-weight: 600;
}

.login-provider-side-label {
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  font-size: 13px;
}

.login-provider-content {
  flex: 1 1 auto;
  min-width: 0;
  height: 100%;
  padding: 8px 0 0 16px;
  overflow: hidden;
}

.login-provider-tab {
  display: inline-flex;
  align-items: center;
  max-width: 92px;
  gap: 5px;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  line-height: 1;
}

.login-provider-icon,
.login-provider-heading-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
}

.login-provider-icon {
  width: 16px;
  height: 16px;
  overflow: hidden;
}

.login-provider-heading-icon {
  width: 22px;
  height: 22px;
  overflow: hidden;
}

.login-provider-icon img,
.login-provider-heading-icon img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
  color: transparent;
  font-size: 0;
}

.login-provider-heading {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 28px;
  margin: 0 0 8px;
  gap: 8px;
  color: var(--color-text-1);
  font-size: 14px;
  font-weight: 600;
}

.pikpak-login-form {
  display: flex;
  flex-direction: column;
  gap: 12px;
  width: 300px;
  margin: 64px auto 0;
}

.webdav-login-form {
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: 300px;
  margin: 34px auto 0;
}

.s3-login-form {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 9px;
  width: 330px;
  margin: 28px auto 0;
  text-align: left;
}

.s3-field-wide {
  grid-column: 1 / -1;
}

.s3-path-style {
  display: flex;
  align-items: center;
  gap: 7px;
  min-height: 32px;
  color: var(--text-primary);
  font-size: 12px;
}

.quark-logincontent {
  height: 430px !important;
  min-height: 430px !important;
}

.quark-login-toolbar {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 8px 0;
}
</style>
