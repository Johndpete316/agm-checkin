import AppBar from '@mui/material/AppBar'
import Toolbar from '@mui/material/Toolbar'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import IconButton from '@mui/material/IconButton'
import Box from '@mui/material/Box'
import DarkModeIcon from '@mui/icons-material/DarkMode'
import LightModeIcon from '@mui/icons-material/LightMode'
import { Link, useLocation } from 'react-router-dom'
import { useColorMode } from '../App'

const navLinks = [
  { label: 'Check In', path: '/home' },
  { label: 'Competitors', path: '/competitors' },
  { label: 'Stats', path: '/stats' },
]

export default function NavBar() {
  const location = useLocation()
  const { mode, toggle } = useColorMode()

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
          <IconButton color="inherit" onClick={toggle} size="small" sx={{ ml: 1 }}>
            {mode === 'dark' ? <LightModeIcon /> : <DarkModeIcon />}
          </IconButton>
        </Box>
      </Toolbar>
    </AppBar>
  )
}
