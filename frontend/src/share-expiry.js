export function shareExpiry(record, now = Date.now()) {
  const raw = String(record.expiration || '').trim()
  if (!raw) return { label: '期限未记录', warning: false }
  let expires = 0
  if (/^\d+$/.test(raw)) {
    const number = Number(raw)
    if (number > 0 && number <= 3660 && Number(record.created_at) > 0) expires = (Number(record.created_at) + number * 86400) * 1000
    else if (number >= 1000000000 && number < 100000000000) expires = number * 1000
    else if (number >= 1000000000000 && number < 100000000000000) expires = number
  } else if (/^\d{4}-\d{2}-\d{2}(?:T|$)/.test(raw)) {
    expires = Date.parse(raw)
  }
  if (!Number.isFinite(expires) || expires <= 0) return { label: '期限无法确认', warning: false }
  const remaining = expires - now
  return { label: remaining <= 0 ? '按本机记录已到期，云端未复核' : remaining <= 3 * 86400000 ? '按本机记录将在三天内到期' : `记录期限：${new Date(expires).toLocaleString()}`, warning: remaining <= 3 * 86400000 }
}
