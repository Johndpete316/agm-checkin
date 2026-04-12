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

  function login(token, firstName, lastName) {
    const staff = { firstName, lastName }
    localStorage.setItem('agm_token', token)
    localStorage.setItem('agm_staff', JSON.stringify(staff))
    setAuth({ token, staff })
  }

  function logout() {
    localStorage.removeItem('agm_token')
    localStorage.removeItem('agm_staff')
    setAuth(null)
  }

  return (
    <AuthContext.Provider value={{ token: auth?.token ?? null, staff: auth?.staff ?? null, login, logout }}>
      {children}
    </AuthContext.Provider>
  )
}
