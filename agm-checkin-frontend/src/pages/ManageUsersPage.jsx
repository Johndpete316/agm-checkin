import { useEffect, useState, useCallback } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableContainer from '@mui/material/TableContainer'
import TableHead from '@mui/material/TableHead'
import TableRow from '@mui/material/TableRow'
import Paper from '@mui/material/Paper'
import Chip from '@mui/material/Chip'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import Alert from '@mui/material/Alert'
import Skeleton from '@mui/material/Skeleton'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogContentText from '@mui/material/DialogContentText'
import DialogActions from '@mui/material/DialogActions'
import Button from '@mui/material/Button'
import BlockIcon from '@mui/icons-material/Block'
import { useAuth } from '../context/AuthContext'
import { listStaff, updateStaffRole, revokeStaff } from '../api/staff'

export default function ManageUsersPage() {
  const { token, staff: currentStaff } = useAuth()
  const isSelf = (s) => s.firstName === currentStaff?.firstName && s.lastName === currentStaff?.lastName
  const [staff, setStaff] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [revokeTarget, setRevokeTarget] = useState(null)
  const [revoking, setRevoking] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const data = await listStaff(token)
      setStaff(data)
    } catch (err) {
      setError(err.message === 'forbidden' ? 'Admin access required.' : 'Failed to load users.')
    } finally {
      setLoading(false)
    }
  }, [token])

  useEffect(() => { load() }, [load])

  async function handleRoleChange(id, role) {
    try {
      const updated = await updateStaffRole(token, id, role)
      setStaff(prev => prev.map(s => s.id === id ? updated : s))
    } catch (err) {
      setError(err.message || 'Failed to update role.')
    }
  }

  async function handleRevoke() {
    if (!revokeTarget) return
    setRevoking(true)
    try {
      await revokeStaff(token, revokeTarget.id)
      setStaff(prev => prev.filter(s => s.id !== revokeTarget.id))
      setRevokeTarget(null)
    } catch (err) {
      setError(err.message || 'Failed to revoke access.')
    } finally {
      setRevoking(false)
    }
  }

  return (
    <Box>
      <Typography variant="h5" fontWeight={700} mb={3}>
        Manage Users
      </Typography>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError('')}>
          {error}
        </Alert>
      )}

      <TableContainer component={Paper} variant="outlined">
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>Name</TableCell>
              <TableCell>Role</TableCell>
              <TableCell>Created</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading
              ? Array.from({ length: 4 }).map((_, i) => (
                  <TableRow key={i}>
                    {Array.from({ length: 4 }).map((_, j) => (
                      <TableCell key={j}><Skeleton /></TableCell>
                    ))}
                  </TableRow>
                ))
              : staff.map(s => (
                  <TableRow key={s.id} hover>
                    <TableCell>
                      {s.firstName} {s.lastName}
                      {isSelf(s) && <Chip label="you" size="small" sx={{ ml: 1 }} />}
                    </TableCell>
                    <TableCell>
                      <Select
                        value={s.role}
                        size="small"
                        onChange={e => handleRoleChange(s.id, e.target.value)}
                        sx={{ minWidth: 130 }}
                      >
                        <MenuItem value="registration">Registration</MenuItem>
                        <MenuItem value="admin">Admin</MenuItem>
                      </Select>
                    </TableCell>
                    <TableCell>
                      {new Date(s.createdAt).toLocaleDateString()}
                    </TableCell>
                    <TableCell align="right">
                      <Tooltip title={isSelf(s) ? 'Cannot revoke your own access' : 'Revoke access'}>
                        <span>
                          <IconButton
                            size="small"
                            color="error"
                            disabled={isSelf(s)}
                            onClick={() => setRevokeTarget(s)}
                          >
                            <BlockIcon fontSize="small" />
                          </IconButton>
                        </span>
                      </Tooltip>
                    </TableCell>
                  </TableRow>
                ))}
          </TableBody>
        </Table>
      </TableContainer>

      <Dialog open={!!revokeTarget} onClose={() => !revoking && setRevokeTarget(null)}>
        <DialogTitle>Revoke access?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            This will permanently revoke the token for{' '}
            <strong>{revokeTarget?.firstName} {revokeTarget?.lastName}</strong>.
            They will need to sign in again with the access code to regain access.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setRevokeTarget(null)} disabled={revoking}>
            Cancel
          </Button>
          <Button color="error" variant="contained" onClick={handleRevoke} disabled={revoking}>
            {revoking ? 'Revoking…' : 'Revoke'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}
