const BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'

export async function requestToken(pin, firstName, lastName) {
  const res = await fetch(`${BASE_URL}/api/auth/token`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code: pin, firstName, lastName }),
  })

  if (res.status === 401) throw new Error('invalid_auth')
  if (res.status === 403) throw new Error('blocked')
  if (!res.ok) throw new Error('server_error')

  return res.json() // { token, firstName, lastName }
}
