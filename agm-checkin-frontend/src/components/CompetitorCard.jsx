import { useState, useEffect } from 'react'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import CardActions from '@mui/material/CardActions'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import Box from '@mui/material/Box'
import Alert from '@mui/material/Alert'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import TextField from '@mui/material/TextField'
import Divider from '@mui/material/Divider'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableHead from '@mui/material/TableHead'
import TableRow from '@mui/material/TableRow'
import CircularProgress from '@mui/material/CircularProgress'
import WarningAmberIcon from '@mui/icons-material/WarningAmber'
import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutline'
import EditIcon from '@mui/icons-material/Edit'
import EmailIcon from '@mui/icons-material/Email'
import { updateCompetitorDOB, validateCompetitor, updateCompetitorContact, getCompetitorSchedule } from '../api/competitors'
import { useAuth } from '../context/AuthContext'

function formatScheduleDate(dateStr) {
  if (!dateStr) return '—'
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return '—'
  return d.toLocaleDateString(undefined, { weekday: 'short', month: 'numeric', day: 'numeric', timeZone: 'UTC' })
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

export default function CompetitorCard({ competitor, onCheckIn, onUpdate, loading }) {
  const { isAdmin } = useAuth()

  const [dialogOpen, setDialogOpen] = useState(false)
  const [editedDOB, setEditedDOB] = useState('')
  const [originalDOB, setOriginalDOB] = useState('')
  const [confirming, setConfirming] = useState(false)
  const [dialogError, setDialogError] = useState('')

  const [schedule, setSchedule] = useState(null)
  const [scheduleLoading, setScheduleLoading] = useState(true)

  useEffect(() => {
    setScheduleLoading(true)
    getCompetitorSchedule(competitor.id)
      .then(setSchedule)
      .catch(() => setSchedule([]))
      .finally(() => setScheduleLoading(false))
  }, [competitor.id])

  const [editOpen, setEditOpen] = useState(false)
  const [editNote, setEditNote] = useState('')
  const [editEmail, setEditEmail] = useState('')
  const [editSaving, setEditSaving] = useState(false)
  const [editError, setEditError] = useState('')

  const isCheckedIn = !!competitor.currentCheckIn?.checkedIn
  const needsValidation = !competitor.dobVerifiedAt
  const age = calculateAge(competitor.dateOfBirth)
  const dob = formatDOB(competitor.dateOfBirth)
  const fullName = `${competitor.nameFirst} ${competitor.nameLast}`

  const handleEditOpen = () => {
    setEditNote(competitor.note ?? '')
    setEditEmail(competitor.email ?? '')
    setEditError('')
    setEditOpen(true)
  }

  const handleEditSave = async () => {
    setEditSaving(true)
    setEditError('')
    try {
      const updated = await updateCompetitorContact(competitor.id, { note: editNote, email: editEmail })
      onUpdate?.(updated)
      setEditOpen(false)
    } catch {
      setEditError('Failed to save. Please try again.')
    } finally {
      setEditSaving(false)
    }
  }

  const handleCheckInClick = () => {
    if (needsValidation) {
      const initial = toInputDate(competitor.dateOfBirth)
      setEditedDOB(initial)
      setOriginalDOB(initial)
      setDialogError('')
      setDialogOpen(true)
    } else {
      onCheckIn(competitor.id)
    }
  }

  const handleConfirm = async () => {
    setConfirming(true)
    setDialogError('')
    try {
      if (editedDOB && editedDOB !== originalDOB) {
        const updated = await updateCompetitorDOB(competitor.id, editedDOB)
        onUpdate?.(updated)
      }
      const validated = await validateCompetitor(competitor.id)
      onUpdate?.(validated)
      setDialogOpen(false)
      onCheckIn(competitor.id)
    } catch {
      setDialogError('Failed to save. Please try again.')
    } finally {
      setConfirming(false)
    }
  }

  return (
    <>
      <Card variant="outlined" sx={{ borderRadius: 2 }}>
        <CardContent sx={{ pb: 1 }}>
          {/* Name + status row */}
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 1.5 }}>
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                <Typography variant="h6" sx={{ fontSize: { xs: '1.2rem', sm: '1.25rem' }, lineHeight: 1.3 }}>
                  {fullName}
                </Typography>
                {needsValidation && !isCheckedIn && (
                  <Chip
                    icon={<WarningAmberIcon fontSize="small" />}
                    label="Validate"
                    color="warning"
                    size="small"
                    variant="outlined"
                  />
                )}
                {competitor.dobVerifiedAt && (
                  <Chip
                    icon={<CheckCircleOutlineIcon fontSize="small" />}
                    label="Validated"
                    color="success"
                    size="small"
                    variant="outlined"
                  />
                )}
                {!competitor.email && !isCheckedIn && (
                  <Chip
                    icon={<EmailIcon fontSize="small" />}
                    label="No Email"
                    color="warning"
                    size="small"
                    variant="outlined"
                  />
                )}
              </Box>
              {competitor.mostRecentEvent && (
                <Typography variant="caption" color="text.secondary">
                  {competitor.mostRecentEvent}
                </Typography>
              )}
            </Box>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 0.5, ml: 1, flexShrink: 0 }}>
              <Button size="small" onClick={handleEditOpen} startIcon={<EditIcon />} variant="outlined">
                Edit
              </Button>
              <Chip
                label={isCheckedIn ? 'Checked In' : 'Pending'}
                color={isCheckedIn ? 'success' : 'default'}
                size="small"
              />
            </Box>
          </Box>

          {/* Key fields */}
          <Box
            sx={{
              display: 'flex',
              gap: { xs: 2.5, sm: 3 },
              flexWrap: 'wrap',
              bgcolor: 'action.hover',
              borderRadius: 1,
              px: 1.5,
              py: 1,
              mb: 1.5,
            }}
          >
            <Box>
              <Typography variant="caption" color="text.secondary" display="block" sx={{ lineHeight: 1.3 }}>Age</Typography>
              <Typography variant="body1" fontWeight={700} sx={{ fontSize: { xs: '1.15rem', sm: '1.05rem' } }}>
                {age !== null ? `${age} yrs` : '—'}
              </Typography>
            </Box>
            {(isAdmin || needsValidation) && (
              <Box>
                <Typography variant="caption" color="text.secondary" display="block" sx={{ lineHeight: 1.3 }}>Date of Birth</Typography>
                <Typography variant="body1" fontWeight={700} sx={{ fontSize: { xs: '1.15rem', sm: '1.05rem' } }}>
                  {dob || '—'}
                </Typography>
              </Box>
            )}
            <Box>
              <Typography variant="caption" color="text.secondary" display="block" sx={{ lineHeight: 1.3 }}>T-Shirt</Typography>
              <Typography variant="body1" fontWeight={700} sx={{ fontSize: { xs: '1.15rem', sm: '1.05rem' } }}>
                {competitor.shirtSize || '—'}
              </Typography>
            </Box>
          </Box>

          {/* Secondary fields */}
          <Box sx={{ display: 'flex', gap: 3, flexWrap: 'wrap', mb: 0.5 }}>
            <Typography variant="body2" color="text.secondary">
              <strong>Studio:</strong> {competitor.studio || '—'}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              <strong>Teacher:</strong> {competitor.teacher || '—'}
            </Typography>
            {competitor.email && (
              <Typography variant="body2" color="text.secondary">
                <strong>Email:</strong> {competitor.email}
              </Typography>
            )}
          </Box>

          {competitor.note && (
            <Alert severity="info" sx={{ mt: 1.5, py: 0.5 }}>
              {competitor.note}
            </Alert>
          )}

          {/* Schedule section */}
          {scheduleLoading && (
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 1.5 }}>
              <CircularProgress size={14} />
              <Typography variant="caption" color="text.secondary">Loading schedule…</Typography>
            </Box>
          )}
          {!scheduleLoading && schedule && schedule.length > 0 && (
            <>
              <Divider sx={{ my: 1.5 }} />
              <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, textTransform: 'uppercase', letterSpacing: 0.5 }}>
                Schedule
              </Typography>
              <Box sx={{ overflowX: 'auto', mt: 0.5 }}>
                <Table size="small" sx={{ minWidth: 400 }}>
                  <TableHead>
                    <TableRow>
                      {['Day', 'Time', 'Room', 'Instrument', 'Category', 'Division'].map(h => (
                        <TableCell key={h} sx={{ fontSize: '0.7rem', fontWeight: 600, py: 0.5, px: 1, color: 'text.secondary', whiteSpace: 'nowrap' }}>
                          {h}
                        </TableCell>
                      ))}
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {schedule.map(entry => (
                      <TableRow key={entry.id} sx={{ '&:last-child td': { border: 0 } }}>
                        <TableCell sx={{ fontSize: '0.75rem', py: 0.5, px: 1, whiteSpace: 'nowrap' }}>{formatScheduleDate(entry.scheduleDate)}</TableCell>
                        <TableCell sx={{ fontSize: '0.75rem', py: 0.5, px: 1, whiteSpace: 'nowrap' }}>{entry.scheduleTime}</TableCell>
                        <TableCell sx={{ fontSize: '0.75rem', py: 0.5, px: 1, whiteSpace: 'nowrap' }}>{entry.room || '—'}</TableCell>
                        <TableCell sx={{ fontSize: '0.75rem', py: 0.5, px: 1 }}>{entry.instrument}</TableCell>
                        <TableCell sx={{ fontSize: '0.75rem', py: 0.5, px: 1 }}>{entry.category}</TableCell>
                        <TableCell sx={{ fontSize: '0.75rem', py: 0.5, px: 1 }}>{entry.division}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </Box>
            </>
          )}

          {isCheckedIn && competitor.currentCheckIn?.checkInDatetime && (
            <>
              <Divider sx={{ my: 1 }} />
              <Typography variant="caption" color="text.secondary">
                Checked in at {new Date(competitor.currentCheckIn.checkInDatetime).toLocaleString()}
                {competitor.currentCheckIn.checkedInBy && ` · ${competitor.currentCheckIn.checkedInBy}`}
              </Typography>
            </>
          )}
        </CardContent>

        {!isCheckedIn && (
          <CardActions sx={{ pt: 0, px: 2, pb: 2 }}>
            <Button
              variant="contained"
              size="large"
              onClick={handleCheckInClick}
              disabled={loading}
              fullWidth
            >
              {loading ? 'Checking in…' : 'Check In'}
            </Button>
          </CardActions>
        )}
      </Card>

      <Dialog open={editOpen} onClose={() => !editSaving && setEditOpen(false)} maxWidth="xs" fullWidth>
        <DialogTitle>Edit Note / Email</DialogTitle>
        <DialogContent>
          <TextField
            fullWidth
            label="Email"
            type="email"
            value={editEmail}
            onChange={e => setEditEmail(e.target.value)}
            sx={{ mt: 1 }}
          />
          <TextField
            fullWidth
            multiline
            minRows={2}
            label="Note"
            value={editNote}
            onChange={e => setEditNote(e.target.value)}
            sx={{ mt: 2 }}
          />
          {editError && <Alert severity="error" sx={{ mt: 2 }}>{editError}</Alert>}
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2 }}>
          <Button onClick={() => setEditOpen(false)} disabled={editSaving}>Cancel</Button>
          <Button variant="contained" onClick={handleEditSave} disabled={editSaving}>
            {editSaving ? 'Saving…' : 'Save'}
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog open={dialogOpen} onClose={() => !confirming && setDialogOpen(false)} maxWidth="xs" fullWidth>
        <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <WarningAmberIcon color="warning" />
          Validate Before Check-In
        </DialogTitle>
        <DialogContent>
          <Typography variant="body1" gutterBottom>
            <strong>{fullName}</strong> requires identity validation.
          </Typography>
          <Box sx={{ display: 'flex', gap: 2, mt: 0.5, mb: 1 }}>
            <Typography variant="body2" color="text.secondary">
              Studio: {competitor.studio || '—'}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              Teacher: {competitor.teacher || '—'}
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
          {dialogError && <Alert severity="error" sx={{ mt: 2 }}>{dialogError}</Alert>}
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2 }}>
          <Button onClick={() => setDialogOpen(false)} disabled={confirming}>Cancel</Button>
          <Button variant="contained" onClick={handleConfirm} disabled={confirming || !editedDOB}>
            {confirming ? 'Saving…' : 'Confirmed — Check In'}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  )
}
