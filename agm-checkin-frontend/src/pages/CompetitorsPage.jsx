import { useState, useEffect, useCallback } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableContainer from '@mui/material/TableContainer'
import TableHead from '@mui/material/TableHead'
import TableRow from '@mui/material/TableRow'
import TableSortLabel from '@mui/material/TableSortLabel'
import Paper from '@mui/material/Paper'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import CircularProgress from '@mui/material/CircularProgress'
import Alert from '@mui/material/Alert'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import WarningAmberIcon from '@mui/icons-material/WarningAmber'
import { getCompetitors, checkInCompetitor } from '../api/competitors'

const columns = [
  { id: 'name', label: 'Name' },
  { id: 'division', label: 'Division' },
  { id: 'dateOfBirth', label: 'Date of Birth' },
  { id: 'isCheckedIn', label: 'Status' },
  { id: 'checkInDateTime', label: 'Check-In Time' },
]

function descendingComparator(a, b, orderBy) {
  const aVal = a[orderBy] ?? ''
  const bVal = b[orderBy] ?? ''
  if (bVal < aVal) return -1
  if (bVal > aVal) return 1
  return 0
}

function getComparator(order, orderBy) {
  return order === 'desc'
    ? (a, b) => descendingComparator(a, b, orderBy)
    : (a, b) => -descendingComparator(a, b, orderBy)
}

function formatDOB(dob) {
  if (!dob) return '—'
  return new Date(dob).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

export default function CompetitorsPage() {
  const [competitors, setCompetitors] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [order, setOrder] = useState('asc')
  const [orderBy, setOrderBy] = useState('name')
  const [checkingIn, setCheckingIn] = useState(null)
  const [validateTarget, setValidateTarget] = useState(null)

  const fetchCompetitors = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const data = await getCompetitors()
      setCompetitors(data)
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchCompetitors()
  }, [fetchCompetitors])

  const handleSort = (column) => {
    const isAsc = orderBy === column && order === 'asc'
    setOrder(isAsc ? 'desc' : 'asc')
    setOrderBy(column)
  }

  const handleCheckInClick = (competitor) => {
    if (competitor.requiresValidation) {
      setValidateTarget(competitor)
    } else {
      doCheckIn(competitor.id)
    }
  }

  const doCheckIn = async (id) => {
    setCheckingIn(id)
    try {
      const updated = await checkInCompetitor(id)
      setCompetitors(prev => prev.map(c => (c.id === id ? updated : c)))
    } catch (err) {
      setError(err.message)
    } finally {
      setCheckingIn(null)
    }
  }

  const handleConfirm = () => {
    const id = validateTarget.id
    setValidateTarget(null)
    doCheckIn(id)
  }

  const sorted = [...competitors].sort(getComparator(order, orderBy))

  return (
    <Box sx={{ mt: 4 }}>
      <Typography variant="h5" gutterBottom>
        All Competitors
      </Typography>
      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}
      {loading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
          <CircularProgress />
        </Box>
      ) : (
        <TableContainer component={Paper} sx={{ borderRadius: 2 }}>
          <Table>
            <TableHead>
              <TableRow sx={{ '& th': { fontWeight: 600 } }}>
                {columns.map(col => (
                  <TableCell key={col.id}>
                    <TableSortLabel
                      active={orderBy === col.id}
                      direction={orderBy === col.id ? order : 'asc'}
                      onClick={() => handleSort(col.id)}
                    >
                      {col.label}
                    </TableSortLabel>
                  </TableCell>
                ))}
                <TableCell>Action</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {sorted.map(competitor => (
                <TableRow key={competitor.id} hover>
                  <TableCell>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      {competitor.name}
                      {competitor.requiresValidation && !competitor.isCheckedIn && (
                        <WarningAmberIcon fontSize="small" color="warning" titleAccess="Requires validation" />
                      )}
                    </Box>
                  </TableCell>
                  <TableCell>{competitor.division}</TableCell>
                  <TableCell>{formatDOB(competitor.dateOfBirth)}</TableCell>
                  <TableCell>
                    <Chip
                      label={competitor.isCheckedIn ? 'Checked In' : 'Pending'}
                      color={competitor.isCheckedIn ? 'success' : 'default'}
                      size="small"
                    />
                  </TableCell>
                  <TableCell>
                    {competitor.checkInDateTime
                      ? new Date(competitor.checkInDateTime).toLocaleString()
                      : '—'}
                  </TableCell>
                  <TableCell>
                    {!competitor.isCheckedIn && (
                      <Button
                        size="small"
                        variant="outlined"
                        onClick={() => handleCheckInClick(competitor)}
                        disabled={checkingIn === competitor.id}
                      >
                        {checkingIn === competitor.id ? 'Checking in…' : 'Check In'}
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      <Dialog open={!!validateTarget} onClose={() => setValidateTarget(null)} maxWidth="xs" fullWidth>
        <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <WarningAmberIcon color="warning" />
          Validate Before Check-In
        </DialogTitle>
        {validateTarget && (
          <DialogContent>
            <Typography variant="body1" gutterBottom>
              <strong>{validateTarget.name}</strong> requires identity validation.
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Division: {validateTarget.division}
            </Typography>
            <Box sx={{ mt: 2, p: 2, borderRadius: 2, bgcolor: 'action.hover' }}>
              <Typography variant="caption" color="text.secondary" display="block">
                Date of Birth
              </Typography>
              <Typography variant="h6">
                {formatDOB(validateTarget.dateOfBirth)}
              </Typography>
            </Box>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
              Confirm you have verified the competitor's date of birth before proceeding.
            </Typography>
          </DialogContent>
        )}
        <DialogActions sx={{ px: 3, pb: 2 }}>
          <Button onClick={() => setValidateTarget(null)}>Cancel</Button>
          <Button variant="contained" onClick={handleConfirm}>
            Confirmed — Check In
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}
