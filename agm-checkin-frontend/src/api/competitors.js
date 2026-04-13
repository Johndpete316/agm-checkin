const BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'

function authHeaders() {
  const token = sessionStorage.getItem('agm_token')
  return token ? { Authorization: `Bearer ${token}` } : {}
}

async function apiFetch(url, options = {}) {
  const res = await fetch(url, {
    ...options,
    headers: { ...options.headers, ...authHeaders() },
  })
  if (res.status === 401) {
    sessionStorage.removeItem('agm_token')
    sessionStorage.removeItem('agm_staff')
    window.location.href = '/login'
    throw new Error('unauthorized')
  }
  return res
}

export async function getCompetitors(search = '') {
  const params = search ? `?search=${encodeURIComponent(search)}` : ''
  const res = await apiFetch(`${BASE_URL}/api/competitors${params}`)
  if (!res.ok) throw new Error('Failed to fetch competitors')
  return res.json()
}

export async function getCompetitor(id) {
  const res = await apiFetch(`${BASE_URL}/api/competitors/${id}`)
  if (!res.ok) throw new Error('Failed to fetch competitor')
  return res.json()
}

export async function createCompetitor(data) {
  const res = await apiFetch(`${BASE_URL}/api/competitors`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) throw new Error('Failed to create competitor')
  return res.json()
}

export async function checkInCompetitor(id) {
  const res = await apiFetch(`${BASE_URL}/api/competitors/${id}/checkin`, {
    method: 'PATCH',
  })
  if (!res.ok) throw new Error('Failed to check in competitor')
  return res.json()
}

// dateOfBirth must be a YYYY-MM-DD string
export async function updateCompetitorDOB(id, dateOfBirth) {
  const res = await apiFetch(`${BASE_URL}/api/competitors/${id}/dob`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ dateOfBirth: `${dateOfBirth}T00:00:00Z` }),
  })
  if (!res.ok) throw new Error('Failed to update date of birth')
  return res.json()
}

export async function validateCompetitor(id) {
  const res = await apiFetch(`${BASE_URL}/api/competitors/${id}/validate`, {
    method: 'PATCH',
  })
  if (!res.ok) throw new Error('Failed to validate competitor')
  return res.json()
}

export async function updateCompetitor(id, data) {
  const res = await apiFetch(`${BASE_URL}/api/competitors/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) throw new Error('Failed to update competitor')
  return res.json()
}

export async function getCompetitorEvents(id) {
  const res = await apiFetch(`${BASE_URL}/api/competitors/${id}/events`)
  if (!res.ok) throw new Error('Failed to fetch competitor event history')
  return res.json()
}

export async function deleteCompetitor(id) {
  const res = await apiFetch(`${BASE_URL}/api/competitors/${id}`, {
    method: 'DELETE',
  })
  if (!res.ok) throw new Error('Failed to delete competitor')
}

export async function importCompetitors(file) {
  const form = new FormData()
  form.append('file', file)
  const res = await apiFetch(`${BASE_URL}/api/competitors/import`, {
    method: 'POST',
    body: form,
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || 'Import failed')
  }
  return res.json()
}
