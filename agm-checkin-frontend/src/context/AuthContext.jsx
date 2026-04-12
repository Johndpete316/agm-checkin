import { createContext, useContext, useState } from 'react'

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

  return (
    <AuthContext.Provider value={{ token: auth?.token ?? null, staff: auth?.staff ?? null, isAdmin, login, logout }}>
      {children}
    </AuthContext.Provider>
  )
}
