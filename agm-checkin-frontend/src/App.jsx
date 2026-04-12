import { createContext, useContext, useMemo, useState } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { ThemeProvider } from '@mui/material/styles'
import CssBaseline from '@mui/material/CssBaseline'
import Box from '@mui/material/Box'
import NavBar from './components/NavBar'
import CheckInPage from './pages/CheckInPage'
import CompetitorsPage from './pages/CompetitorsPage'
import StatsPage from './pages/StatsPage'
import LoginPage from './pages/LoginPage'
import ManageUsersPage from './pages/ManageUsersPage'
import EventsPage from './pages/EventsPage'
import { buildTheme } from './theme'
import { AuthProvider, useAuth } from './context/AuthContext'

export const ColorModeContext = createContext({ toggle: () => {} })
export const useColorMode = () => useContext(ColorModeContext)

function getInitialMode() {
  const stored = localStorage.getItem('colorMode')
  if (stored === 'light' || stored === 'dark') return stored
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function ProtectedRoute({ children }) {
  const { token } = useAuth()
  if (!token) return <Navigate to="/login" replace />
  return children
}

function AdminRoute({ children }) {
  const { token, isAdmin } = useAuth()
  if (!token) return <Navigate to="/login" replace />
  if (!isAdmin) return <Navigate to="/home" replace />
  return children
}

function AppLayout() {
  const { token } = useAuth()

  return (
    <>
      {token && <NavBar />}
      <Box
        component="main"
        sx={token ? { pt: 3, px: { xs: 1.5, sm: 3 }, pb: 6 } : {}}
      >
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/" element={<Navigate to="/home" replace />} />
          <Route path="/home" element={<ProtectedRoute><CheckInPage /></ProtectedRoute>} />
          <Route path="/competitors" element={<ProtectedRoute><CompetitorsPage /></ProtectedRoute>} />
          <Route path="/stats" element={<ProtectedRoute><StatsPage /></ProtectedRoute>} />
          <Route path="/events" element={<AdminRoute><EventsPage /></AdminRoute>} />
          <Route path="/manage-users" element={<AdminRoute><ManageUsersPage /></AdminRoute>} />
          <Route path="*" element={<Navigate to="/home" replace />} />
        </Routes>
      </Box>
    </>
  )
}

export default function App() {
  const [mode, setMode] = useState(getInitialMode)

  const colorMode = useMemo(() => ({
    toggle: () =>
      setMode(prev => {
        const next = prev === 'light' ? 'dark' : 'light'
        localStorage.setItem('colorMode', next)
        return next
      }),
    mode,
  }), [mode])

  const theme = useMemo(() => buildTheme(mode), [mode])

  return (
    <ColorModeContext.Provider value={colorMode}>
      <ThemeProvider theme={theme}>
        <CssBaseline />
        <AuthProvider>
          <BrowserRouter>
            <AppLayout />
          </BrowserRouter>
        </AuthProvider>
      </ThemeProvider>
    </ColorModeContext.Provider>
  )
}
