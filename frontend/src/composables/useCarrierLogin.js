import { computed, onBeforeUnmount, ref } from 'vue'
import { SendGuangyaSms, SendPan139SMS, SendPan189SMS } from '../api'

export function useCarrierLogin({ providerId, form, errorText, emit }) {
  const smsBusy = ref(false)
  const smsCountdown = ref(0)
  const pan139SMSRequired = ref(false)
  const pan189Captcha = ref('')
  const isPan139DirectLogin = computed(() => providerId.value === 'pan139' && String(form.value.authorization || '').trim() !== '')

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

  async function sendSMS({ missingMessage, request, onSuccess }) {
    const username = String(form.value.username || form.value.phone || '').trim()
    if (!username) {
      errorText.value = missingMessage
      return
    }
    smsBusy.value = true
    errorText.value = ''
    try {
      const result = await request(username)
      if (onSuccess) onSuccess(result)
      emit('toast', '验证码已发送', 'success')
      startSmsCountdown()
      return result
    } catch (loginError) {
      errorText.value = String(loginError)
    } finally {
      smsBusy.value = false
    }
  }

  async function sendGuangyaSms() {
    await sendSMS({
      missingMessage: '请先填写手机号',
      request: (phone) => SendGuangyaSms(phone),
      onSuccess: (result) => {
        form.value.verification_id = result.verification_id
        form.value.device_id = result.device_id
        form.value.captcha_token = result.captcha_token || ''
      },
    })
  }

  function sendPan139Sms() {
    return sendSMS({
      missingMessage: '请先填写账号',
      request: (username) => SendPan139SMS(username),
    })
  }

  function sendPan189Sms() {
    return sendSMS({
      missingMessage: '请先填写手机号',
      request: (username) => SendPan189SMS(username),
    })
  }

  function parsePan189Captcha(loginError) {
    const match = String(loginError).match(/captcha_required_189\r?\nimage=(\S+)/)
    if (!match) return false
    pan189Captcha.value = match[1]
    form.value.validate_code = ''
    errorText.value = '请输入图片中的验证码'
    return true
  }

  function parsePan189CaptchaExpired(loginError) {
    if (!/captcha_expired_189/i.test(String(loginError))) return false
    pan189Captcha.value = ''
    form.value.validate_code = ''
    errorText.value = '验证码已过期，请重新登录'
    return true
  }

  function parsePan189CaptchaRetry(loginError) {
    if (!/captcha_retry_189/i.test(String(loginError))) return false
    pan189Captcha.value = ''
    form.value.validate_code = ''
    errorText.value = '验证码不正确，请重新登录'
    return true
  }

  function parsePan139SMSRequired(loginError) {
    if (!/pan139_sms_required/i.test(String(loginError))) return false
    pan139SMSRequired.value = true
    form.value.login_mode = 'sms'
    form.value.sms_code = ''
    errorText.value = '请获取并填写短信验证码'
    return true
  }

  function handleError(attemptProvider, loginError) {
    if (attemptProvider === 'pan189') {
      return parsePan189Captcha(loginError) || parsePan189CaptchaRetry(loginError) || parsePan189CaptchaExpired(loginError)
    }
    return attemptProvider === 'pan139' && parsePan139SMSRequired(loginError)
  }

  function fieldVisible(field) {
    if (providerId.value === 'pan139') {
      if (field.key === 'password') return !pan139SMSRequired.value
      if (field.key === 'sms_code') return pan139SMSRequired.value
    }
    if (providerId.value === 'pan189') {
      const smsMode = form.value.login_mode === 'sms'
      if (field.key === 'password' || field.key === 'validate_code') return !smsMode
      if (field.key === 'sms_code') return smsMode
    }
    return true
  }

  function subtitle() {
    if (providerId.value === 'pan139' && pan139SMSRequired.value) return '短信安全验证'
    if (providerId.value === 'pan189' && form.value.login_mode === 'sms') return '短信验证码登录'
    return ''
  }

  // null 表示当前不是运营商专属流程；空字符串表示专属校验已通过。
  function validate(value) {
    if (providerId.value === 'pan189') {
      if (!value('username')) return '请填写手机号/邮箱'
      if (value('login_mode') === 'sms') {
        if (!value('sms_code')) return '请填写短信验证码'
      } else {
        if (!value('password')) return '请填写密码'
        if (pan189Captcha.value && !value('validate_code')) return '请填写图形验证码'
      }
      return ''
    }
    if (providerId.value === 'pan139') {
      if (!value('username')) return '请填写手机号/账号'
      if (pan139SMSRequired.value) {
        if (!value('sms_code')) return '请填写短信验证码'
      } else if (!value('password')) {
        return '请填写密码'
      }
      return ''
    }
    return null
  }

  function reset() {
    pan189Captcha.value = ''
    pan139SMSRequired.value = false
    delete form.value.validate_code
    delete form.value.sms_code
  }

  onBeforeUnmount(() => {
    if (smsTimer) clearInterval(smsTimer)
    smsTimer = null
  })

  return {
    fieldVisible,
    handleError,
    isPan139DirectLogin,
    pan189Captcha,
    reset,
    sendGuangyaSms,
    sendPan139Sms,
    sendPan189Sms,
    smsBusy,
    smsCountdown,
    subtitle,
    validate,
  }
}
