const BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'

async function handleResponse(res) {
  if (res.status === 401) {
    localStorage.removeItem('agm_token')
    localStorage.removeItem('agm_staff')
    window.location.href = '/login'
    throw new Error('unauthorized')
  }
  if (res.status === 403) throw new Error('forbidden')
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || 'server_error')
  }
  return res
}

export async function listStaff(token) {
  const res = await fetch(`${BASE_URL}/api/staff`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  await handleResponse(res)
  return res.json()
}

export async function updateStaffRole(token, id, role) {
  const res = await fetch(`${BASE_URL}/api/staff/${id}/role`, {
    method: 'PATCH',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify({ role }),
  })
  await handleResponse(res)
  return res.json()
}

export async function revokeStaff(token, id) {
  const res = await fetch(`${BASE_URL}/api/staff/${id}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` },
  })
  await handleResponse(res)
}
