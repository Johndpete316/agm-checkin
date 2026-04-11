import { useState } from 'react'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import CardActions from '@mui/material/CardActions'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import Box from '@mui/material/Box'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import WarningAmberIcon from '@mui/icons-material/WarningAmber'

function formatDOB(dob) {
  if (!dob) return '—'
  return new Date(dob).toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
}

export default function CompetitorCard({ competitor, onCheckIn, loading }) {
  const [dialogOpen, setDialogOpen] = useState(false)

  const handleCheckInClick = () => {
    if (competitor.requiresValidation) {
      setDialogOpen(true)
    } else {
      onCheckIn(competitor.id)
    }
  }

  const handleConfirm = () => {
    setDialogOpen(false)
    onCheckIn(competitor.id)
  }

  return (
    <>
      <Card variant="outlined" sx={{ borderRadius: 2 }}>
        <CardContent sx={{ pb: 0 }}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
            <Box>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Typography variant="h6">{competitor.name}</Typography>
                {competitor.requiresValidation && !competitor.isCheckedIn && (
                  <Chip
                    icon={<WarningAmberIcon fontSize="small" />}
                    label="Validate"
                    color="warning"
                    size="small"
                    variant="outlined"
                  />
                )}
              </Box>
              <Typography variant="body2" color="text.secondary">
                {competitor.division}
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                DOB: {formatDOB(competitor.dateOfBirth)}
              </Typography>
            </Box>
            <Chip
              label={competitor.isCheckedIn ? 'Checked In' : 'Pending'}
              color={competitor.isCheckedIn ? 'success' : 'default'}
              size="small"
              sx={{ mt: 0.5 }}
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

      <Dialog open={dialogOpen} onClose={() => setDialogOpen(false)} maxWidth="xs" fullWidth>
        <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <WarningAmberIcon color="warning" />
          Validate Before Check-In
        </DialogTitle>
        <DialogContent>
          <Typography variant="body1" gutterBottom>
            <strong>{competitor.name}</strong> requires identity validation.
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Division: {competitor.division}
          </Typography>
          <Box
            sx={{
              mt: 2,
              p: 2,
              borderRadius: 2,
              bgcolor: 'action.hover',
              display: 'inline-block',
              width: '100%',
            }}
          >
            <Typography variant="caption" color="text.secondary" display="block">
              Date of Birth
            </Typography>
            <Typography variant="h6">
              {formatDOB(competitor.dateOfBirth)}
            </Typography>
          </Box>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
            Confirm you have verified the competitor's date of birth before proceeding.
          </Typography>
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2 }}>
          <Button onClick={() => setDialogOpen(false)}>Cancel</Button>
          <Button variant="contained" onClick={handleConfirm}>
            Confirmed — Check In
          </Button>
        </DialogActions>
      </Dialog>
    </>
  )
}
