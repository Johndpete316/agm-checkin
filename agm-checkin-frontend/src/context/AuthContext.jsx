import { createContext, useContext, useState, useEffect } from 'react'

const BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'

const AuthContext = createContext(null)

export function useAuth() {
  return useContext(AuthContext)
}

function getStoredAuth() {
  const token = localStorage.getItem('agm_token')
  const raw = localStorage.getItem('agm_staff')
  if (token && raw) {
    try {
      return { token, staff: JSON.parse(raw) }
    } catch {
      return null
    }
  }
  return null
}

export function AuthProvider({ children }) {
  const [auth, setAuth] = useState(getStoredAuth)

  function login(token, firstName, lastName, role = 'registration') {
    const staff = { firstName, lastName, role }
    localStorage.setItem('agm_token', token)
    localStorage.setItem('agm_staff', JSON.stringify(staff))
    setAuth({ token, staff })
  }

  function logout() {
    localStorage.removeItem('agm_token')
    localStorage.removeItem('agm_staff')
    setAuth(null)
  }

  const isAdmin = auth?.staff?.role === 'admin'

  useEffect(() => {
    async function syncRole() {
      const stored = getStoredAuth()
      if (!stored) return
      try {
        const res = await fetch(`${BASE_URL}/api/auth/me`, {
          headers: { Authorization: `Bearer ${stored.token}` },
        })
        if (res.status === 401) {
          // Token has been revoked — force logout
          localStorage.removeItem('agm_token')
          localStorage.removeItem('agm_staff')
          setAuth(null)
          return
        }
        if (!res.ok) return
        const data = await res.json()
        const updatedStaff = { ...stored.staff, role: data.role }
        localStorage.setItem('agm_staff', JSON.stringify(updatedStaff))
        setAuth({ token: stored.token, staff: updatedStaff })
      } catch {
        // Network error — leave existing auth state alone
      }
    }

    syncRole()

    function handleVisibilityChange() {
      if (document.visibilityState === 'visible') syncRole()
    }
    document.addEventListener('visibilitychange', handleVisibilityChange)
    return () => document.removeEventListener('visibilitychange', handleVisibilityChange)
  }, [])

  return (
    <AuthContext.Provider value={{ token: auth?.token ?? null, staff: auth?.staff ?? null, isAdmin, login, logout }}>
      {children}
    </AuthContext.Provider>
  )
}
