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
import Divider from '@mui/material/Divider'
import Typography from '@mui/material/Typography'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableHead from '@mui/material/TableHead'
import TableRow from '@mui/material/TableRow'
import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutline'
import RemoveCircleOutlineIcon from '@mui/icons-material/RemoveCircleOutline'
import { updateCompetitor, getCompetitorEvents } from '../api/competitors'
import { listEvents } from '../api/events'

const SHIRT_SIZES = ['Adult XL', 'Adult L', 'Adult M', 'Adult S', 'Youth XL', 'Youth L', 'Youth M', 'Youth S']

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
  const [eventHistory, setEventHistory] = useState([])
  const [events, setEvents] = useState([])

  useEffect(() => {
    listEvents().then(setEvents).catch(() => {})
  }, [])

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
      // An action, not a stored value: picking an event adds them to that
      // roster. Pre-filling it would silently re-register them on every save.
      registerForEvent: '',
      dobVerified: !!competitor.dobVerifiedAt,
      note: competitor.note ?? '',
    })
    setError('')
    setEventHistory([])
    getCompetitorEvents(competitor.id)
      .then(setEventHistory)
      .catch(() => {}) // non-critical — form still works without history
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
        registerForEvent: form.registerForEvent,
        // The server owns when and by whom; this only says whether.
        dobVerifiedAt: form.dobVerified ? (competitor.dobVerifiedAt ?? new Date().toISOString()) : null,
        note: form.note,
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
              <InputLabel>Add To Event</InputLabel>
              <Select
                value={form.registerForEvent ?? ''}
                label="Add To Event"
                onChange={e => set('registerForEvent', e.target.value)}
              >
                <MenuItem value=""><em>No change</em></MenuItem>
                {events.map(e => (
                  <MenuItem key={e.id} value={e.id}>{e.name} ({e.id})</MenuItem>
                ))}
              </Select>
            </FormControl>
          </Box>

          <TextField
            label="Note"
            value={form.note ?? ''}
            onChange={e => set('note', e.target.value)}
            fullWidth
            multiline
            minRows={2}
            placeholder="Internal staff note (visible to all staff)"
          />

          <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap', alignItems: 'center' }}>
            <FormControlLabel
              control={<Switch checked={form.dobVerified ?? false} onChange={e => set('dobVerified', e.target.checked)} />}
              label="Date of Birth Verified"
            />
            {competitor?.dobVerifiedAt && (
              <Typography variant="caption" color="text.secondary">
                {`by ${competitor.dobVerifiedBy || 'unknown'} on ${new Date(competitor.dobVerifiedAt).toLocaleDateString()}`}
              </Typography>
            )}
          </Box>

          {error && <Alert severity="error">{error}</Alert>}

          {/* Event history */}
          {eventHistory.length > 0 && (
            <>
              <Divider />
              <Box>
                <Typography variant="subtitle2" sx={{ mb: 1 }}>Event History</Typography>
                <Table size="small">
                  <TableHead>
                    <TableRow sx={{ '& th': { fontWeight: 600 } }}>
                      <TableCell>Event</TableCell>
                      <TableCell>Checked In</TableCell>
                      <TableCell>Check-In Time</TableCell>
                      <TableCell>By</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {eventHistory.map(ev => (
                      <TableRow key={ev.eventId ?? ev.id}>
                        <TableCell sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                          {ev.eventId}
                        </TableCell>
                        <TableCell>
                          {ev.checkedIn
                            ? <CheckCircleOutlineIcon fontSize="small" color="success" />
                            : <RemoveCircleOutlineIcon fontSize="small" color="disabled" />
                          }
                        </TableCell>
                        <TableCell>
                          {ev.checkInDatetime
                            ? new Date(ev.checkInDatetime).toLocaleString()
                            : <Typography variant="body2" color="text.disabled">—</Typography>
                          }
                        </TableCell>
                        <TableCell>
                          {ev.checkedInBy || <Typography variant="body2" color="text.disabled">—</Typography>}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </Box>
            </>
          )}
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
