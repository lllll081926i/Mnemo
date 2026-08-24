import { onBeforeUnmount, onMounted, ref } from 'vue'
import { ClosePikPakCaptcha, onEvent } from '../api'
import { info } from '../logger'

const BUTTON_COOLDOWN_CAP_SECONDS = 30
const IGNORE_COOLDOWN_AT_SECONDS = 60 * 60

function embeddedCaptchaURL(challengeURL, callbackURL) {
  try {
    const value = new URL(String(challengeURL || ''))
    if (callbackURL) value.searchParams.set('redirect_uri', callbackURL)
    return value.toString()
  } catch {
    return String(challengeURL || '')
  }
}

function parseRetryAfterSeconds(text) {
  const match = String(text || '').match(/(?:等待|wait(?:ing)?(?:\s+for)?|retry\s+after)\s*(\d+(?:\.\d+)?)\s*(秒(?:钟)?|second(?:s)?|分钟|min(?:ute)?s?|小时|hour(?:s)?)?/i)
  if (!match) return 0
  const value = Number(match[1])
  if (!Number.isFinite(value) || value <= 0) return 0
  const unit = String(match[2] || '').toLowerCase()
  const multiplier = /分钟|min/.test(unit) ? 60 : (/小时|hour/.test(unit) ? 3600 : 1)
  return Math.ceil(value * multiplier)
}

export function usePikPakLogin({ providerId, form, errorText, busy, resumeLogin }) {
  const captchaUrl = ref('')
  const captchaSessionId = ref('')
  const captchaFrameReady = ref(false)
  const captchaSubmitting = ref(false)
  const cooldownSeconds = ref(0)

  let cooldownTimer = null
  let offCaptchaCompleted = null
  let disposed = false
  let completionBusy = false
  let closePromise = Promise.resolve()

  function clearCooldown() {
    if (cooldownTimer) clearInterval(cooldownTimer)
    cooldownTimer = null
    cooldownSeconds.value = 0
  }

  function startCooldown(seconds) {
    clearCooldown()
    const requested = Math.ceil(Number(seconds) || 0)
    // 长等待不能让本地登录按钮持续锁死，短等待仅用于防止连点。
    if (requested <= 0 || requested >= IGNORE_COOLDOWN_AT_SECONDS) return false
    cooldownSeconds.value = Math.min(BUTTON_COOLDOWN_CAP_SECONDS, requested)
    cooldownTimer = setInterval(() => {
      cooldownSeconds.value--
      if (cooldownSeconds.value <= 0) clearCooldown()
    }, 1000)
    return true
  }

  function handleRateLimit(loginError) {
    const text = String(loginError || '')
    if (!/(?:频繁|too[ _-]*(?:many|frequent)|rate[ _-]*limit|request[ _-]*frequency|access[ _-]*prohibited|risk[ _-]*control|429)/i.test(text)) return false
    const requested = parseRetryAfterSeconds(text)
    const riskBlocked = /access[ _-]*prohibited|risk[ _-]*control/i.test(text)
    const cooling = startCooldown(requested || (riskBlocked ? BUTTON_COOLDOWN_CAP_SECONDS : 30))
    errorText.value = cooling
      ? `PikPak 暂时限制了登录请求，请等待 ${cooldownSeconds.value} 秒后再试`
      : 'PikPak 暂时限制了登录请求，请稍后重试'
    return true
  }

  function parseChallenge(loginError) {
    const text = String(loginError)
    const match = text.match(/captcha_required\r?\nurl=(\S+)\r?\ntoken=(\S*)/)
    if (!match) return false
    const session = text.match(/(?:^|\r?\n)session=([A-Za-z0-9_-]+)/)
    const callback = text.match(/(?:^|\r?\n)callback=(\S+)/)
    captchaUrl.value = embeddedCaptchaURL(match[1], callback ? callback[1] : '')
    captchaSessionId.value = session ? session[1] : ''
    form.value.captcha_token = match[2]
    delete form.value.captcha_verified
    delete form.value.captcha_requires_confirmation
    captchaFrameReady.value = false
    return true
  }

  function handleError(loginError) {
    if (handleRateLimit(loginError)) return true
    if (!parseChallenge(loginError)) return false
    errorText.value = '请在登录页面内完成 PikPak 安全验证'
    return true
  }

  function closeSession() {
    const close = closePromise
      .catch(() => {})
      .then(() => ClosePikPakCaptcha())
      .catch(() => {})
    closePromise = close
    return close
  }

  function clearChallenge() {
    captchaSessionId.value = ''
    captchaUrl.value = ''
    captchaFrameReady.value = false
  }

  function reset(closeActiveSession = false) {
    clearChallenge()
    captchaSubmitting.value = false
    delete form.value.captcha_token
    delete form.value.captcha_verified
    delete form.value.captcha_requires_confirmation
    if (closeActiveSession) void closeSession()
  }

  async function completeCaptcha(payload) {
    const sessionID = String(payload?.session_id || '').trim()
    if (
      completionBusy ||
      providerId.value !== 'pikpak' ||
      !captchaUrl.value ||
      !captchaSessionId.value ||
      sessionID !== captchaSessionId.value
    ) return

    info('captcha', 'PikPak captcha completion accepted', {
      session_id: sessionID,
      has_token: !!String(payload?.captcha_token || '').trim(),
    })
    completionBusy = true
    const token = String(payload?.captcha_token || '').trim()
    clearChallenge()
    if (token) form.value.captcha_token = token
    // 部分回调不携带最终 token；滑块已在服务端更新原 token 状态，
    // 后端会按 rclone 流程直接用原 token 发起 signin。
    form.value.captcha_verified = 'true'
    delete form.value.captcha_requires_confirmation
    errorText.value = ''

    try {
      await closeSession()
      if (!disposed && providerId.value === 'pikpak') await resumeLogin()
    } finally {
      completionBusy = false
    }
  }

  async function reloadCaptcha() {
    if (!captchaUrl.value || busy.value) return
    clearChallenge()
    delete form.value.captcha_token
    delete form.value.captcha_verified
    delete form.value.captcha_requires_confirmation
    info('captcha', 'PikPak captcha reload requested')
    await closeSession()
    await resumeLogin()
  }

  async function prepareSubmit(attemptProvider) {
    captchaSubmitting.value = true
    if (attemptProvider !== 'pikpak') return true
    // 用户手动继续时保留原 token，完成的滑块会更新它的服务端状态。
    if (captchaUrl.value) {
      clearChallenge()
      form.value.captcha_verified = 'true'
      delete form.value.captcha_requires_confirmation
      await closeSession()
    }
    await closePromise
    if (disposed || providerId.value !== attemptProvider || captchaUrl.value) return false
    if (cooldownSeconds.value > 0) {
      errorText.value = `PikPak 暂时限制了登录请求，请等待 ${cooldownSeconds.value} 秒后再试`
      return false
    }
    return true
  }

  function finishSubmit() {
    captchaSubmitting.value = false
  }

  onMounted(() => {
    disposed = false
    offCaptchaCompleted = onEvent('pikpak:captcha:completed', (payload) => {
      info('captcha', 'PikPak captcha completion event received', {
        session_id: String(payload?.session_id || ''),
        has_token: !!String(payload?.captcha_token || '').trim(),
      })
      void completeCaptcha(payload)
    })
  })

  onBeforeUnmount(() => {
    disposed = true
    if (offCaptchaCompleted) offCaptchaCompleted()
    offCaptchaCompleted = null
    void closeSession()
    clearCooldown()
  })

  return {
    captchaUrl,
    captchaFrameReady,
    captchaSubmitting,
    cooldownSeconds,
    clearCooldown,
    finishSubmit,
    handleError,
    prepareSubmit,
    reloadCaptcha,
    reset,
  }
}
