import { createContext, useContext, useMemo, useState } from 'react'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { ThemeProvider } from '@mui/material/styles'
import CssBaseline from '@mui/material/CssBaseline'
import Box from '@mui/material/Box'
import NavBar from './components/NavBar'
import CheckInPage from './pages/CheckInPage'
import CompetitorsPage from './pages/CompetitorsPage'
import StatsPage from './pages/StatsPage'
import { buildTheme } from './theme'

export const ColorModeContext = createContext({ toggle: () => {} })
export const useColorMode = () => useContext(ColorModeContext)

function getInitialMode() {
  const stored = localStorage.getItem('colorMode')
  if (stored === 'light' || stored === 'dark') return stored
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
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
        <BrowserRouter>
          <NavBar />
          <Box component="main" sx={{ pt: 3, px: 3, pb: 6 }}>
            <Routes>
              <Route path="/" element={<Navigate to="/home" replace />} />
              <Route path="/home" element={<CheckInPage />} />
              <Route path="/competitors" element={<CompetitorsPage />} />
              <Route path="/stats" element={<StatsPage />} />
            </Routes>
          </Box>
        </BrowserRouter>
      </ThemeProvider>
    </ColorModeContext.Provider>
  )
}
