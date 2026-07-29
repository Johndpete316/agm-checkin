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

// Every schedule entry for one event, chronological. Omit eventId for the
// current event.
export async function getEventSchedule(eventId = '') {
  const params = eventId ? `?eventId=${encodeURIComponent(eventId)}` : ''
  const res = await apiFetch(`${BASE_URL}/api/schedule${params}`)
  if (!res.ok) throw new Error('Failed to fetch schedule')
  return res.json()
}
