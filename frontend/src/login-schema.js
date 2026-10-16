export function loginDefaults(config = {}) {
  const values = Object.fromEntries((config.fields || []).filter(field => field.default !== undefined && field.default !== '').map(field => [field.key, field.default]))
  if (config.methods?.length) values.login_method = config.methods[0].id
  return values
}

export function loginFields(config = {}, method) {
  return (config.fields || []).filter(field => field.type !== 'oauth' && !field.hidden && field.purpose !== 'mode' && field.key !== 'login_mode' && (!method || method.fieldKeys.includes(field.key)))
}

export function loginFieldVisible(field, mode) {
  return !field.modes?.length || field.modes.includes(mode)
}

export function loginFieldRequired(field, mode, method) {
  return !!field.required || !!field.requiredModes?.includes(mode) || !!method?.requiredKeys?.includes(field.key)
}

export function selectLoginMethod(form, method) {
  return { ...Object.fromEntries(Object.entries(form).filter(([key]) => method.fieldKeys.includes(key))), login_method: method.id }
}
