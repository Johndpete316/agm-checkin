const BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'

export async function getCompetitors(search = '') {
  const params = search ? `?search=${encodeURIComponent(search)}` : ''
  const res = await fetch(`${BASE_URL}/api/competitors${params}`)
  if (!res.ok) throw new Error('Failed to fetch competitors')
  return res.json()
}

export async function getCompetitor(id) {
  const res = await fetch(`${BASE_URL}/api/competitors/${id}`)
  if (!res.ok) throw new Error('Failed to fetch competitor')
  return res.json()
}

export async function createCompetitor(data) {
  const res = await fetch(`${BASE_URL}/api/competitors`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) throw new Error('Failed to create competitor')
  return res.json()
}

export async function checkInCompetitor(id) {
  const res = await fetch(`${BASE_URL}/api/competitors/${id}/checkin`, {
    method: 'PATCH',
  })
  if (!res.ok) throw new Error('Failed to check in competitor')
  return res.json()
}

export async function deleteCompetitor(id) {
  const res = await fetch(`${BASE_URL}/api/competitors/${id}`, {
    method: 'DELETE',
  })
  if (!res.ok) throw new Error('Failed to delete competitor')
}
