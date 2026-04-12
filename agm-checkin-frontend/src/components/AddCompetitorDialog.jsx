import { useState, useEffect } from 'react'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import TextField from '@mui/material/TextField'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import FormControlLabel from '@mui/material/FormControlLabel'
import Switch from '@mui/material/Switch'
import Box from '@mui/material/Box'
import Alert from '@mui/material/Alert'
import { createCompetitor } from '../api/competitors'
import { getCurrentEvent } from '../api/events'

const SHIRT_SIZES = ['XS', 'S', 'M', 'L', 'XL', 'XXL']

const empty = {
  nameFirst: '',
  nameLast: '',
  dateOfBirth: '',
  email: '',
  studio: '',
  teacher: '',
  shirtSize: '',
  lastRegisteredEvent: '',
  requiresValidation: false,
}

export default function AddCompetitorDialog({ open, onClose, onCreated }) {
  const [form, setForm] = useState(empty)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  // Pre-populate lastRegisteredEvent with the current event on open.
  useEffect(() => {
    if (!open) return
    setForm(empty)
    setError('')
    getCurrentEvent()
      .then(event => {
        if (event?.id) setForm(prev => ({ ...prev, lastRegisteredEvent: event.id }))
      })
      .catch(() => {})
  }, [open])

  function set(field, value) {
    setForm(prev => ({ ...prev, [field]: value }))
  }

  async function handleSave() {
    if (!form.nameFirst.trim() || !form.nameLast.trim()) {
      setError('First and last name are required.')
      return
    }
    setSaving(true)
    setError('')
    try {
      const payload = {
        nameFirst: form.nameFirst.trim(),
        nameLast: form.nameLast.trim(),
        dateOfBirth: form.dateOfBirth ? `${form.dateOfBirth}T00:00:00Z` : '0001-01-01T00:00:00Z',
        email: form.email.trim(),
        studio: form.studio.trim(),
        teacher: form.teacher.trim(),
        shirtSize: form.shirtSize,
        lastRegisteredEvent: form.lastRegisteredEvent,
        requiresValidation: form.requiresValidation,
        validated: !form.requiresValidation,
      }
      const created = await createCompetitor(payload)
      onCreated(created)
    } catch (err) {
      setError(err.message || 'Failed to create competitor.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onClose={() => !saving && onClose()} maxWidth="sm" fullWidth>
      <DialogTitle>Add Competitor</DialogTitle>
      <DialogContent>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, pt: 1 }}>
          <Box sx={{ display: 'flex', gap: 2 }}>
            <TextField
              label="First Name"
              value={form.nameFirst}
              onChange={e => set('nameFirst', e.target.value)}
              fullWidth
              required
              autoFocus
            />
            <TextField
              label="Last Name"
              value={form.nameLast}
              onChange={e => set('nameLast', e.target.value)}
              fullWidth
              required
            />
          </Box>

          <Box sx={{ display: 'flex', gap: 2 }}>
            <TextField
              label="Date of Birth"
              type="date"
              value={form.dateOfBirth}
              onChange={e => set('dateOfBirth', e.target.value)}
              slotProps={{ inputLabel: { shrink: true } }}
              fullWidth
            />
            <TextField
              label="Email"
              value={form.email}
              onChange={e => set('email', e.target.value)}
              fullWidth
            />
          </Box>

          <Box sx={{ display: 'flex', gap: 2 }}>
            <TextField
              label="Studio"
              value={form.studio}
              onChange={e => set('studio', e.target.value)}
              fullWidth
            />
            <TextField
              label="Teacher"
              value={form.teacher}
              onChange={e => set('teacher', e.target.value)}
              fullWidth
            />
          </Box>

          <Box sx={{ display: 'flex', gap: 2 }}>
            <FormControl fullWidth>
              <InputLabel>Shirt Size</InputLabel>
              <Select
                value={form.shirtSize}
                label="Shirt Size"
                onChange={e => set('shirtSize', e.target.value)}
              >
                <MenuItem value=""><em>Unknown</em></MenuItem>
                {SHIRT_SIZES.map(s => <MenuItem key={s} value={s}>{s}</MenuItem>)}
              </Select>
            </FormControl>
            <TextField
              label="Last Registered Event"
              value={form.lastRegisteredEvent}
              onChange={e => set('lastRegisteredEvent', e.target.value)}
              fullWidth
              helperText="Auto-filled from current event"
            />
          </Box>

          <FormControlLabel
            control={
              <Switch
                checked={form.requiresValidation}
                onChange={e => set('requiresValidation', e.target.checked)}
              />
            }
            label="Requires age/identity validation"
          />

          {error && <Alert severity="error">{error}</Alert>}
        </Box>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2 }}>
        <Button onClick={onClose} disabled={saving}>Cancel</Button>
        <Button variant="contained" onClick={handleSave} disabled={saving}>
          {saving ? 'Adding…' : 'Add Competitor'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}
