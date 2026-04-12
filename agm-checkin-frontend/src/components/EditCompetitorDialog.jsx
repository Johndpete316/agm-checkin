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
import { updateCompetitor } from '../api/competitors'

const SHIRT_SIZES = ['XS', 'S', 'M', 'L', 'XL', 'XXL']
const EVENTS = ['glr-2026', 'nat-2025', 'glr-2025', 'nat-2024']

function toInputDate(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime()) || d.getFullYear() < 1900) return ''
  const y = d.getUTCFullYear()
  const m = String(d.getUTCMonth() + 1).padStart(2, '0')
  const day = String(d.getUTCDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}


export default function EditCompetitorDialog({ competitor, onClose, onSaved }) {
  const [form, setForm] = useState({})
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!competitor) return
    setForm({
      nameFirst: competitor.nameFirst ?? '',
      nameLast: competitor.nameLast ?? '',
      dateOfBirth: toInputDate(competitor.dateOfBirth),
      email: competitor.email ?? '',
      studio: competitor.studio ?? '',
      teacher: competitor.teacher ?? '',
      shirtSize: competitor.shirtSize ?? '',
      lastRegisteredEvent: competitor.lastRegisteredEvent ?? '',
      requiresValidation: competitor.requiresValidation ?? false,
      validated: competitor.validated ?? false,
    })
    setError('')
  }, [competitor])

  function set(field, value) {
    setForm(prev => ({ ...prev, [field]: value }))
  }

  async function handleSave() {
    setSaving(true)
    setError('')
    try {
      const payload = {
        ...competitor,
        nameFirst: form.nameFirst,
        nameLast: form.nameLast,
        dateOfBirth: form.dateOfBirth ? `${form.dateOfBirth}T00:00:00Z` : competitor.dateOfBirth,
        email: form.email,
        studio: form.studio,
        teacher: form.teacher,
        shirtSize: form.shirtSize,
        lastRegisteredEvent: form.lastRegisteredEvent,
        requiresValidation: form.requiresValidation,
        validated: form.validated,
      }
      const updated = await updateCompetitor(competitor.id, payload)
      onSaved(updated)
    } catch (err) {
      setError(err.message || 'Failed to save changes.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={!!competitor} onClose={() => !saving && onClose()} maxWidth="sm" fullWidth>
      <DialogTitle>
        Edit Competitor
      </DialogTitle>
      <DialogContent>
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, pt: 1 }}>
          <Box sx={{ display: 'flex', gap: 2 }}>
            <TextField
              label="First Name"
              value={form.nameFirst ?? ''}
              onChange={e => set('nameFirst', e.target.value)}
              fullWidth
            />
            <TextField
              label="Last Name"
              value={form.nameLast ?? ''}
              onChange={e => set('nameLast', e.target.value)}
              fullWidth
            />
          </Box>

          <Box sx={{ display: 'flex', gap: 2 }}>
            <TextField
              label="Date of Birth"
              type="date"
              value={form.dateOfBirth ?? ''}
              onChange={e => set('dateOfBirth', e.target.value)}
              slotProps={{ inputLabel: { shrink: true } }}
              fullWidth
            />
            <TextField
              label="Email"
              value={form.email ?? ''}
              onChange={e => set('email', e.target.value)}
              fullWidth
            />
          </Box>

          <Box sx={{ display: 'flex', gap: 2 }}>
            <TextField
              label="Studio"
              value={form.studio ?? ''}
              onChange={e => set('studio', e.target.value)}
              fullWidth
            />
            <TextField
              label="Teacher"
              value={form.teacher ?? ''}
              onChange={e => set('teacher', e.target.value)}
              fullWidth
            />
          </Box>

          <Box sx={{ display: 'flex', gap: 2 }}>
            <FormControl fullWidth>
              <InputLabel>Shirt Size</InputLabel>
              <Select
                value={form.shirtSize ?? ''}
                label="Shirt Size"
                onChange={e => set('shirtSize', e.target.value)}
              >
                <MenuItem value=""><em>None</em></MenuItem>
                {SHIRT_SIZES.map(s => <MenuItem key={s} value={s}>{s}</MenuItem>)}
              </Select>
            </FormControl>
            <FormControl fullWidth>
              <InputLabel>Last Registered Event</InputLabel>
              <Select
                value={form.lastRegisteredEvent ?? ''}
                label="Last Registered Event"
                onChange={e => set('lastRegisteredEvent', e.target.value)}
              >
                <MenuItem value=""><em>None</em></MenuItem>
                {EVENTS.map(e => <MenuItem key={e} value={e}>{e}</MenuItem>)}
              </Select>
            </FormControl>
          </Box>

          <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap' }}>
            <FormControlLabel
              control={<Switch checked={form.requiresValidation ?? false} onChange={e => set('requiresValidation', e.target.checked)} />}
              label="Requires Validation"
            />
            <FormControlLabel
              control={<Switch checked={form.validated ?? false} onChange={e => set('validated', e.target.checked)} />}
              label="Validated"
            />
          </Box>

          {error && <Alert severity="error">{error}</Alert>}
        </Box>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2 }}>
        <Button onClick={onClose} disabled={saving}>Cancel</Button>
        <Button variant="contained" onClick={handleSave} disabled={saving}>
          {saving ? 'Saving…' : 'Save'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}
