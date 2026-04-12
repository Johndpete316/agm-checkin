import { useState, useEffect, useCallback } from 'react'
import EditIcon from '@mui/icons-material/Edit'
import IconButton from '@mui/material/IconButton'
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
import TextField from '@mui/material/TextField'
import Tooltip from '@mui/material/Tooltip'
import Divider from '@mui/material/Divider'
import WarningAmberIcon from '@mui/icons-material/WarningAmber'
import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutline'
import {
  getCompetitors,
  checkInCompetitor,
  updateCompetitorDOB,
  validateCompetitor,
} from '../api/competitors'
import { useAuth } from '../context/AuthContext'
import EditCompetitorDialog from '../components/EditCompetitorDialog'

// Sortable columns — id must match the JSON field name from the API
const columns = [
  { id: 'nameLast', label: 'Name' },
  { id: 'lastRegisteredEvent', label: 'Event' },
  { id: 'studio', label: 'Studio' },
  { id: 'teacher', label: 'Teacher' },
  { id: 'shirtSize', label: 'Shirt' },
  { id: 'dateOfBirth', label: 'DOB / Age' },
  { id: 'email', label: 'Email' },
  { id: 'validated', label: 'Validated' },
  { id: 'currentCheckIn', label: 'Status' },
  { id: 'currentCheckIn', label: 'Check-In Time' },
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

function calculateAge(dob) {
  if (!dob) return null
  const birth = new Date(dob)
  if (isNaN(birth.getTime()) || birth.getFullYear() < 1900) return null
  const today = new Date()
  let age = today.getFullYear() - birth.getFullYear()
  if (
    today.getMonth() < birth.getMonth() ||
    (today.getMonth() === birth.getMonth() && today.getDate() < birth.getDate())
  ) {
    age--
  }
  return age
}

function formatDOB(dob) {
  if (!dob) return null
  const d = new Date(dob)
  if (isNaN(d.getTime()) || d.getFullYear() < 1900) return null
  return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric', timeZone: 'UTC' })
}

function toInputDate(dob) {
  if (!dob) return ''
  const d = new Date(dob)
  if (isNaN(d.getTime()) || d.getFullYear() < 1900) return ''
  const y = d.getUTCFullYear()
  const m = String(d.getUTCMonth() + 1).padStart(2, '0')
  const day = String(d.getUTCDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

export default function CompetitorsPage() {
  const { isAdmin } = useAuth()
  const [competitors, setCompetitors] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [order, setOrder] = useState('asc')
  const [orderBy, setOrderBy] = useState('nameLast')
  const [checkingIn, setCheckingIn] = useState(null)
  const [validateTarget, setValidateTarget] = useState(null)
  const [editedDOB, setEditedDOB] = useState('')
  const [confirming, setConfirming] = useState(false)
  const [dialogError, setDialogError] = useState('')
  const [editTarget, setEditTarget] = useState(null)

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

  const updateLocalCompetitor = (updated) => {
    setCompetitors(prev => prev.map(c => (c.id === updated.id ? { ...c, ...updated } : c)))
  }

  const isCheckedIn = (competitor) => !!competitor.currentCheckIn?.checkedIn

  const handleCheckInClick = (competitor) => {
    if (competitor.requiresValidation && !competitor.validated) {
      setEditedDOB(toInputDate(competitor.dateOfBirth))
      setDialogError('')
      setValidateTarget(competitor)
    } else {
      doCheckIn(competitor.id)
    }
  }

  const doCheckIn = async (id) => {
    setCheckingIn(id)
    try {
      const updated = await checkInCompetitor(id)
      updateLocalCompetitor(updated)
    } catch (err) {
      setError(err.message)
    } finally {
      setCheckingIn(null)
    }
  }

  const handleConfirm = async () => {
    if (!validateTarget) return
    setConfirming(true)
    setDialogError('')
    try {
      const originalDOB = toInputDate(validateTarget.dateOfBirth)
      if (editedDOB && editedDOB !== originalDOB) {
        const updated = await updateCompetitorDOB(validateTarget.id, editedDOB)
        updateLocalCompetitor(updated)
      }
      const validated = await validateCompetitor(validateTarget.id)
      updateLocalCompetitor(validated)
      const id = validateTarget.id
      setValidateTarget(null)
      doCheckIn(id)
    } catch {
      setDialogError('Failed to save. Please try again.')
    } finally {
      setConfirming(false)
    }
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
        <>
          {/* Mobile card list */}
          <Box sx={{ display: { xs: 'flex', md: 'none' }, flexDirection: 'column', gap: 1.5 }}>
            {sorted.map(competitor => {
              const age = calculateAge(competitor.dateOfBirth)
              const dob = formatDOB(competitor.dateOfBirth)
              return (
                <Paper key={competitor.id} variant="outlined" sx={{ borderRadius: 2, p: 2 }}>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 1 }}>
                    <Box sx={{ minWidth: 0 }}>
                      <Typography variant="subtitle1" fontWeight={600} noWrap>
                        {competitor.nameFirst} {competitor.nameLast}
                      </Typography>
                      <Typography variant="body2" color="text.secondary" noWrap>
                        {competitor.studio || '—'}
                      </Typography>
                      <Typography variant="body2" color="text.secondary" noWrap>
                        {competitor.teacher || '—'}
                      </Typography>
                      {competitor.lastRegisteredEvent && (
                        <Typography variant="caption" color="text.secondary" noWrap display="block">
                          {competitor.lastRegisteredEvent}
                        </Typography>
                      )}
                    </Box>
                    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 0.5, flexShrink: 0 }}>
                      <Chip
                        label={isCheckedIn(competitor) ? 'Checked In' : 'Pending'}
                        color={isCheckedIn(competitor) ? 'success' : 'default'}
                        size="small"
                      />
                      {competitor.requiresValidation && (
                        competitor.validated ? (
                          <Chip icon={<CheckCircleOutlineIcon />} label="Validated" color="success" size="small" variant="outlined" />
                        ) : (
                          <Chip icon={<WarningAmberIcon />} label="Validate" color="warning" size="small" variant="outlined" />
                        )
                      )}
                    </Box>
                  </Box>
                  <Divider sx={{ my: 1 }} />
                  <Box sx={{ display: 'flex', gap: 2.5, flexWrap: 'wrap', mb: 0.5 }}>
                    <Box>
                      <Typography variant="caption" color="text.secondary" display="block" sx={{ lineHeight: 1.3 }}>Age</Typography>
                      <Typography variant="body1" fontWeight={700}>{age !== null ? `${age} yrs` : '—'}</Typography>
                    </Box>
                    <Box>
                      <Typography variant="caption" color="text.secondary" display="block" sx={{ lineHeight: 1.3 }}>Date of Birth</Typography>
                      <Typography variant="body1" fontWeight={700}>{dob || '—'}</Typography>
                    </Box>
                    <Box>
                      <Typography variant="caption" color="text.secondary" display="block" sx={{ lineHeight: 1.3 }}>T-Shirt</Typography>
                      <Typography variant="body1" fontWeight={700}>{competitor.shirtSize || '—'}</Typography>
                    </Box>
                  </Box>
                  <Box sx={{ mt: 1.5, display: 'flex', gap: 1 }}>
                    {!isCheckedIn(competitor) && (
                      <Button
                        size="small"
                        variant="outlined"
                        onClick={() => handleCheckInClick(competitor)}
                        disabled={checkingIn === competitor.id}
                        fullWidth
                      >
                        {checkingIn === competitor.id ? 'Checking in…' : 'Check In'}
                      </Button>
                    )}
                    {isAdmin && (
                      <IconButton size="small" onClick={() => setEditTarget(competitor)}>
                        <EditIcon fontSize="small" />
                      </IconButton>
                    )}
                  </Box>
                </Paper>
              )
            })}
          </Box>

          {/* Desktop table */}
          <TableContainer component={Paper} sx={{ borderRadius: 2, display: { xs: 'none', md: 'block' } }}>
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
                {sorted.map(competitor => {
                  const age = calculateAge(competitor.dateOfBirth)
                  const dob = formatDOB(competitor.dateOfBirth)
                  return (
                    <TableRow key={competitor.id} hover>
                      <TableCell>{competitor.nameFirst} {competitor.nameLast}</TableCell>
                      <TableCell sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                        {competitor.lastRegisteredEvent || '—'}
                      </TableCell>
                      <TableCell>{competitor.studio || '—'}</TableCell>
                      <TableCell>{competitor.teacher || '—'}</TableCell>
                      <TableCell>{competitor.shirtSize || '—'}</TableCell>
                      <TableCell>
                        <Typography variant="body2">{dob || '—'}</Typography>
                        {age !== null && (
                          <Typography variant="caption" color="text.secondary">{age} yrs</Typography>
                        )}
                      </TableCell>
                      <TableCell>{competitor.email || '—'}</TableCell>
                      <TableCell>
                        {competitor.requiresValidation ? (
                          competitor.validated ? (
                            <Tooltip title="Validated">
                              <CheckCircleOutlineIcon fontSize="small" color="success" />
                            </Tooltip>
                          ) : (
                            <Tooltip title="Requires validation">
                              <WarningAmberIcon fontSize="small" color="warning" />
                            </Tooltip>
                          )
                        ) : (
                          <Typography variant="body2" color="text.disabled">—</Typography>
                        )}
                      </TableCell>
                      <TableCell>
                        <Chip
                          label={isCheckedIn(competitor) ? 'Checked In' : 'Pending'}
                          color={isCheckedIn(competitor) ? 'success' : 'default'}
                          size="small"
                        />
                      </TableCell>
                      <TableCell>
                        {competitor.currentCheckIn?.checkInDatetime ? (
                          <>
                            {new Date(competitor.currentCheckIn.checkInDatetime).toLocaleString()}
                            {competitor.currentCheckIn.checkedInBy && (
                              <Typography variant="caption" color="text.secondary" display="block">
                                {competitor.currentCheckIn.checkedInBy}
                              </Typography>
                            )}
                          </>
                        ) : '—'}
                      </TableCell>
                      <TableCell>
                        <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>
                          {!isCheckedIn(competitor) && (
                            <Button
                              size="small"
                              variant="outlined"
                              onClick={() => handleCheckInClick(competitor)}
                              disabled={checkingIn === competitor.id}
                            >
                              {checkingIn === competitor.id ? 'Checking in…' : 'Check In'}
                            </Button>
                          )}
                          {isAdmin && (
                            <IconButton size="small" onClick={() => setEditTarget(competitor)}>
                              <EditIcon fontSize="small" />
                            </IconButton>
                          )}
                        </Box>
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </TableContainer>
        </>
      )}

      <EditCompetitorDialog
        competitor={editTarget}
        onClose={() => setEditTarget(null)}
        onSaved={updated => {
          const existing = competitors.find(c => c.id === updated.id)
          updateLocalCompetitor({ ...updated, currentCheckIn: existing?.currentCheckIn })
          setEditTarget(null)
        }}
      />

      <Dialog
        open={!!validateTarget}
        onClose={() => !confirming && setValidateTarget(null)}
        maxWidth="xs"
        fullWidth
      >
        <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <WarningAmberIcon color="warning" />
          Validate Before Check-In
        </DialogTitle>
        {validateTarget && (
          <DialogContent>
            <Typography variant="body1" gutterBottom>
              <strong>{validateTarget.nameFirst} {validateTarget.nameLast}</strong> requires identity validation.
            </Typography>
            <Box sx={{ display: 'flex', gap: 2, mb: 1 }}>
              <Typography variant="body2" color="text.secondary">
                Studio: {validateTarget.studio || '—'}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Teacher: {validateTarget.teacher || '—'}
              </Typography>
            </Box>
            <TextField
              fullWidth
              label="Date of Birth"
              type="date"
              value={editedDOB}
              onChange={e => setEditedDOB(e.target.value)}
              slotProps={{ inputLabel: { shrink: true } }}
              sx={{ mt: 1 }}
              helperText="Update if the date on file is incorrect."
            />
            {dialogError && (
              <Alert severity="error" sx={{ mt: 2 }}>
                {dialogError}
              </Alert>
            )}
          </DialogContent>
        )}
        <DialogActions sx={{ px: 3, pb: 2 }}>
          <Button onClick={() => setValidateTarget(null)} disabled={confirming}>
            Cancel
          </Button>
          <Button
            variant="contained"
            onClick={handleConfirm}
            disabled={confirming || !editedDOB}
          >
            {confirming ? 'Saving…' : 'Confirmed — Check In'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}
