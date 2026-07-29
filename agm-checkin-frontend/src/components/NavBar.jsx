import { useState } from 'react'
import AppBar from '@mui/material/AppBar'
import Toolbar from '@mui/material/Toolbar'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import IconButton from '@mui/material/IconButton'
import Box from '@mui/material/Box'
import Drawer from '@mui/material/Drawer'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemText from '@mui/material/ListItemText'
import Divider from '@mui/material/Divider'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Collapse from '@mui/material/Collapse'
import DarkModeIcon from '@mui/icons-material/DarkMode'
import LightModeIcon from '@mui/icons-material/LightMode'
import LogoutIcon from '@mui/icons-material/Logout'
import MenuIcon from '@mui/icons-material/Menu'
import ArrowDropDownIcon from '@mui/icons-material/ArrowDropDown'
import ExpandLessIcon from '@mui/icons-material/ExpandLess'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useColorMode } from '../App'
import { useAuth } from '../context/AuthContext'
import logo from '../assets/agm-125th-logo.png'

const baseNavLinks = [
  { label: 'Check In', path: '/home' },
  { label: 'Competitors', path: '/competitors' },
  { label: 'Schedule', path: '/schedule' },
  { label: 'Stats', path: '/stats' },
]

// Every admin-only destination lives behind one nav entry, so the bar itself is
// the same four links for everyone. Add to this list to extend the group.
const adminGroup = {
  label: 'Admin',
  links: [
    { label: 'Events', path: '/events' },
    { label: 'Manage Users', path: '/manage-users' },
    { label: 'Import Data', path: '/import' },
    { label: 'Audit Log', path: '/audit' },
  ],
}

export default function NavBar() {
  const location = useLocation()
  const navigate = useNavigate()
  const { mode, toggle } = useColorMode()
  const { staff, isAdmin, logout } = useAuth()

  const navLinks = baseNavLinks
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [adminAnchor, setAdminAnchor] = useState(null)

  const inAdminGroup = adminGroup.links.some(link => link.path === location.pathname)
  // The drawer group starts open when you are already inside it, so the current
  // page is never hidden behind a collapsed section.
  const [adminExpanded, setAdminExpanded] = useState(inAdminGroup)

  function handleLogout() {
    logout()
    navigate('/login', { replace: true })
  }

  const drawer = (
    <Box sx={{ width: 260 }} role="presentation">
      {staff && (
        <Box sx={{ px: 2, py: 2 }}>
          <Typography variant="body2" color="text.secondary" sx={{ fontSize: '0.75rem', textTransform: 'uppercase', letterSpacing: 0.8 }}>
            Signed in as
          </Typography>
          <Typography variant="subtitle1" fontWeight={600}>
            {staff.firstName} {staff.lastName}
          </Typography>
        </Box>
      )}
      <Divider />
      <List disablePadding>
        {navLinks.map(link => (
          <ListItem key={link.path} disablePadding>
            <ListItemButton
              component={Link}
              to={link.path}
              selected={location.pathname === link.path}
              onClick={() => setDrawerOpen(false)}
            >
              <ListItemText primary={link.label} />
            </ListItemButton>
          </ListItem>
        ))}

        {isAdmin && (
          <>
            <ListItem disablePadding>
              <ListItemButton onClick={() => setAdminExpanded(open => !open)}>
                <ListItemText primary={adminGroup.label} />
                {adminExpanded ? <ExpandLessIcon fontSize="small" /> : <ExpandMoreIcon fontSize="small" />}
              </ListItemButton>
            </ListItem>
            <Collapse in={adminExpanded} unmountOnExit>
              <List disablePadding>
                {adminGroup.links.map(link => (
                  <ListItem key={link.path} disablePadding>
                    <ListItemButton
                      component={Link}
                      to={link.path}
                      selected={location.pathname === link.path}
                      onClick={() => setDrawerOpen(false)}
                      sx={{ pl: 4 }}
                    >
                      <ListItemText primary={link.label} primaryTypographyProps={{ variant: 'body2' }} />
                    </ListItemButton>
                  </ListItem>
                ))}
              </List>
            </Collapse>
          </>
        )}
      </List>
      <Divider />
      <List disablePadding>
        <ListItem disablePadding>
          <ListItemButton onClick={() => { toggle(); setDrawerOpen(false) }}>
            <ListItemText primary={mode === 'dark' ? 'Light mode' : 'Dark mode'} />
            {mode === 'dark' ? <LightModeIcon fontSize="small" /> : <DarkModeIcon fontSize="small" />}
          </ListItemButton>
        </ListItem>
        <ListItem disablePadding>
          <ListItemButton onClick={() => { handleLogout(); setDrawerOpen(false) }}>
            <ListItemText primary="Sign out" />
            <LogoutIcon fontSize="small" />
          </ListItemButton>
        </ListItem>
      </List>
    </Box>
  )

  return (
    <>
      <AppBar position="static">
        <Toolbar>
          <Box sx={{ flexGrow: 1, display: 'flex', alignItems: 'center', gap: 1.5 }}>
            <Box
              component="img"
              src={logo}
              alt="AGM logo"
              sx={{ height: 36, width: 'auto', display: 'block' }}
            />
            <Typography variant="h6">
              AGM Check-In
            </Typography>
          </Box>

          {/* Desktop nav */}
          <Box sx={{ display: { xs: 'none', md: 'flex' }, alignItems: 'center', gap: 1 }}>
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

            {isAdmin && (
              <>
                <Button
                  color="inherit"
                  onClick={e => setAdminAnchor(e.currentTarget)}
                  endIcon={<ArrowDropDownIcon />}
                  aria-haspopup="menu"
                  aria-expanded={Boolean(adminAnchor)}
                  sx={{
                    fontWeight: inAdminGroup ? 700 : 400,
                    borderBottom: inAdminGroup ? '2px solid white' : '2px solid transparent',
                    borderRadius: 0,
                  }}
                >
                  {adminGroup.label}
                </Button>
                <Menu
                  anchorEl={adminAnchor}
                  open={Boolean(adminAnchor)}
                  onClose={() => setAdminAnchor(null)}
                  anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
                  transformOrigin={{ vertical: 'top', horizontal: 'right' }}
                >
                  {adminGroup.links.map(link => (
                    <MenuItem
                      key={link.path}
                      component={Link}
                      to={link.path}
                      selected={location.pathname === link.path}
                      onClick={() => setAdminAnchor(null)}
                    >
                      {link.label}
                    </MenuItem>
                  ))}
                </Menu>
              </>
            )}

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

          {/* Mobile hamburger */}
          <IconButton
            color="inherit"
            edge="end"
            onClick={() => setDrawerOpen(true)}
            sx={{ display: { xs: 'flex', md: 'none' } }}
          >
            <MenuIcon />
          </IconButton>
        </Toolbar>
      </AppBar>

      <Drawer
        anchor="right"
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
      >
        {drawer}
      </Drawer>
    </>
  )
}
