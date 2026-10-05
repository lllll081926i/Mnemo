function initializeThunderVerification(callbackURL) {
  if (window.parent !== window || location.protocol !== 'https:' ||
      !(location.hostname === 'xunlei.com' || location.hostname.endsWith('.xunlei.com')) ||
      location.pathname !== '/xlcaptcha/vertifyPhone.html' || window.__mnemoThunderVerificationInitialized) return;

  function showError(message) {
    let notice = document.getElementById('mnemo-verification-error');
    if (!notice) {
      notice = document.createElement('div');
      notice.id = 'mnemo-verification-error';
      notice.setAttribute('role', 'alert');
      notice.style.cssText = 'position:fixed;left:20px;right:20px;top:20px;padding:16px;border:1px solid #e5c4c4;border-radius:10px;background:#fff6f6;color:#8a2525;font:15px/1.6 sans-serif;z-index:2147483647';
      document.documentElement.appendChild(notice);
    }
    notice.textContent = message;
  }

  const sdk = window.XlCaptcha;
  if (!sdk || typeof sdk.init !== 'function' || !document.getElementById('captch-wrap')) {
    showError('迅雷验证组件未加载，请关闭此窗口并重新打开；若仍失败，请检查网络或代理。');
    return;
  }
  const query = new URLSearchParams(location.search);
  if (['appid', 'appName', 'deviceid', 'creditkey', 'mobile', 'userID'].some(key => !query.get(key))) {
    showError('迅雷验证页面缺少必要的账号或设备信息，请关闭验证窗口并重新登录。');
    return;
  }

  try {
    window.__mnemoThunderVerificationInitialized = true;
    sdk.init({
      IFRAME_BOX_ID: 'captch-wrap',
      PANEL_THEME: 'default',
      LANGUAGE: 'zh-CN',
      CLOSEABLE: false,
      VERTIFY_SUCC_FUNC(result) {
        if (!result || Number(result.code) !== 200) return;
        const creditKey = typeof result.creditkey === 'string' ? result.creditkey.trim() : '';
        const captchaToken = typeof result.captcha_token === 'string' ? result.captcha_token.trim() : '';
        if (!creditKey && !captchaToken) {
          showError('迅雷未返回完整验证结果，请关闭验证窗口并重新登录。');
          return;
        }
        const callback = new URL(callbackURL);
        if (creditKey) callback.searchParams.set('creditkey', creditKey);
        if (captchaToken) callback.searchParams.set('captcha_token', captchaToken);
        location.replace(callback.href);
      },
    });
    setTimeout(() => {
      if (!sdk.baseParams || !sdk.baseParams.mobile || !sdk.baseParams.userID) {
        showError('迅雷验证页面未能完成加载，请关闭此窗口并重新打开；若仍失败，请检查网络或代理。');
      }
    }, 15000);
  } catch {
    window.__mnemoThunderVerificationInitialized = false;
    showError('迅雷验证组件初始化失败，请关闭验证窗口并重新登录。');
  }
}
