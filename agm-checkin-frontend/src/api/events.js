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

export async function listEvents() {
  const res = await apiFetch(`${BASE_URL}/api/events`)
  if (!res.ok) throw new Error('Failed to fetch events')
  return res.json()
}

export async function getCurrentEvent() {
  const res = await apiFetch(`${BASE_URL}/api/events/current`)
  if (!res.ok) throw new Error('Failed to fetch current event')
  return res.json() // null if no current event
}

export async function createEvent(data) {
  const res = await apiFetch(`${BASE_URL}/api/events`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || 'Failed to create event')
  }
  return res.json()
}

export async function setCurrentEvent(id) {
  const res = await apiFetch(`${BASE_URL}/api/events/${id}/current`, {
    method: 'PATCH',
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || 'Failed to set current event')
  }
  return res.json()
}
