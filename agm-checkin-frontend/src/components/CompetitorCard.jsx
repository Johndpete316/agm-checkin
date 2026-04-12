import { useState } from 'react'
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
import WarningAmberIcon from '@mui/icons-material/WarningAmber'
import { updateCompetitorDOB, validateCompetitor } from '../api/competitors'

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

// Convert stored DOB to YYYY-MM-DD for the date input using UTC to avoid day-shift
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
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editedDOB, setEditedDOB] = useState('')
  const [confirming, setConfirming] = useState(false)
  const [dialogError, setDialogError] = useState('')

  const needsValidation = competitor.requiresValidation && !competitor.validated
  const age = calculateAge(competitor.dateOfBirth)
  const fullName = `${competitor.nameFirst} ${competitor.nameLast}`

  const handleCheckInClick = () => {
    if (needsValidation) {
      setEditedDOB(toInputDate(competitor.dateOfBirth))
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
      const originalDOB = toInputDate(competitor.dateOfBirth)
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
        <CardContent sx={{ pb: 0 }}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
            <Box sx={{ flex: 1, minWidth: 0 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                <Typography variant="h6">{fullName}</Typography>
                {needsValidation && !competitor.isCheckedIn && (
                  <Chip
                    icon={<WarningAmberIcon fontSize="small" />}
                    label="Validate"
                    color="warning"
                    size="small"
                    variant="outlined"
                  />
                )}
              </Box>
              <Box sx={{ display: 'flex', gap: 3, mt: 0.75, flexWrap: 'wrap' }}>
                <Typography variant="body2" color="text.secondary">
                  <strong>Studio:</strong> {competitor.studio || '—'}
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  <strong>Teacher:</strong> {competitor.teacher || '—'}
                </Typography>
              </Box>
              <Box sx={{ display: 'flex', gap: 3, mt: 0.25, flexWrap: 'wrap' }}>
                <Typography variant="body2" color="text.secondary">
                  <strong>Age:</strong> {age !== null ? age : '—'}
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  <strong>Shirt:</strong> {competitor.shirtSize || '—'}
                </Typography>
              </Box>
            </Box>
            <Chip
              label={competitor.isCheckedIn ? 'Checked In' : 'Pending'}
              color={competitor.isCheckedIn ? 'success' : 'default'}
              size="small"
              sx={{ mt: 0.5, ml: 1, flexShrink: 0 }}
            />
          </Box>
          {competitor.isCheckedIn && competitor.checkInDateTime && (
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
              Checked in at {new Date(competitor.checkInDateTime).toLocaleString()}
            </Typography>
          )}
        </CardContent>
        {!competitor.isCheckedIn && (
          <CardActions>
            <Button
              size="small"
              variant="contained"
              onClick={handleCheckInClick}
              disabled={loading}
            >
              {loading ? 'Checking in…' : 'Check In'}
            </Button>
          </CardActions>
        )}
      </Card>

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
          {dialogError && (
            <Alert severity="error" sx={{ mt: 2 }}>
              {dialogError}
            </Alert>
          )}
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2 }}>
          <Button onClick={() => setDialogOpen(false)} disabled={confirming}>
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
    </>
  )
}
