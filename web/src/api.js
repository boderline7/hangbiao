const jsonHeaders = { 'Content-Type': 'application/json' }

async function request(url, options = {}) {
  const res = await fetch(url, options)
  const text = await res.text()
  let data = null
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    data = { error: text }
  }
  if (!res.ok) {
    throw new Error((data && data.error) || `请求失败（HTTP ${res.status}）`)
  }
  return data
}

export const api = {
  state: () => request('/api/state'),
  save: (state) =>
    request('/api/state', {
      method: 'PUT',
      headers: jsonHeaders,
      body: JSON.stringify({
        groups: state.groups,
        rules: state.rules,
        ruleSets: state.ruleSets,
        settings: state.settings,
      }),
    }),
  presets: () => request('/api/presets'),
  syncPresets: () => request('/api/presets/sync', { method: 'POST' }),
  applyPreset: (id) => request(`/api/presets/${id}/apply`, { method: 'POST' }),
  addSub: (payload) =>
    request('/api/subs', { method: 'POST', headers: jsonHeaders, body: JSON.stringify(payload) }),
  refreshSub: (id) => request(`/api/subs/${id}/refresh`, { method: 'POST' }),
  refreshAllSubs: () => request('/api/subs/refresh', { method: 'POST' }),
  checkRuleSets: () => request('/api/rulesets/check', { method: 'POST' }),
  deleteSub: (id) => request(`/api/subs/${id}`, { method: 'DELETE' }),
  exportConfig: (format) => request(`/api/export?format=${encodeURIComponent(format)}`),
  downloadUrl: (format) => `/api/export?format=${encodeURIComponent(format)}&download=1`,
}
