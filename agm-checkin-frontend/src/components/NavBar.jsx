import AppBar from '@mui/material/AppBar'
import Toolbar from '@mui/material/Toolbar'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import IconButton from '@mui/material/IconButton'
import Box from '@mui/material/Box'
import Divider from '@mui/material/Divider'
import DarkModeIcon from '@mui/icons-material/DarkMode'
import LightModeIcon from '@mui/icons-material/LightMode'
import LogoutIcon from '@mui/icons-material/Logout'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useColorMode } from '../App'
import { useAuth } from '../context/AuthContext'

const navLinks = [
  { label: 'Check In', path: '/home' },
  { label: 'Competitors', path: '/competitors' },
  { label: 'Stats', path: '/stats' },
]

export default function NavBar() {
  const location = useLocation()
  const navigate = useNavigate()
  const { mode, toggle } = useColorMode()
  const { staff, logout } = useAuth()

  function handleLogout() {
    logout()
    navigate('/login', { replace: true })
  }

  return (
    <AppBar position="static">
      <Toolbar>
        <Typography variant="h6" sx={{ flexGrow: 1 }}>
          AGM Check-In
        </Typography>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          {navLinks.map(link => (
            <Button
              key={link.path}
              component={Link}
              to={link.path}
              color="inherit"
              sx={{
                fontWeight: location.pathname === link.path ? 700 : 400,
                borderBottom: location.pathname === link.path ? '2px solid white' : '2px solid transparent',
                borderRadius: 0,
              }}
            >
              {link.label}
            </Button>
          ))}

          <Divider orientation="vertical" flexItem sx={{ mx: 1, borderColor: 'rgba(255,255,255,0.3)' }} />

          {staff && (
            <Typography variant="body2" sx={{ opacity: 0.85 }}>
              {staff.firstName} {staff.lastName}
            </Typography>
          )}

          <IconButton color="inherit" onClick={toggle} size="small">
            {mode === 'dark' ? <LightModeIcon /> : <DarkModeIcon />}
          </IconButton>

          <IconButton color="inherit" onClick={handleLogout} size="small" title="Sign out">
            <LogoutIcon fontSize="small" />
          </IconButton>
        </Box>
      </Toolbar>
    </AppBar>
  )
}
