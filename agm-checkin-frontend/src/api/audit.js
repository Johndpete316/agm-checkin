const BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'

function authHeaders() {
  const token = localStorage.getItem('agm_token')
  return token ? { Authorization: `Bearer ${token}` } : {}
}

async function apiFetch(url, options = {}) {
  const res = await fetch(url, {
    ...options,
    headers: { ...options.headers, ...authHeaders() },
  })
  if (res.status === 401) {
    localStorage.removeItem('agm_token')
    localStorage.removeItem('agm_staff')
    window.location.href = '/login'
    throw new Error('unauthorized')
  }
  return res
}

export async function listAuditLogs({ action = '', actor = '', limit = 100 } = {}) {
  const params = new URLSearchParams()
  if (action) params.set('action', action)
  if (actor) params.set('actor', actor)
  if (limit) params.set('limit', String(limit))
  const res = await apiFetch(`${BASE_URL}/api/audit?${params}`)
  if (!res.ok) throw new Error('Failed to fetch audit log')
  return res.json()
}
