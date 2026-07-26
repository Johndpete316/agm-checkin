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
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import TextField from '@mui/material/TextField'
import Alert from '@mui/material/Alert'
import Skeleton from '@mui/material/Skeleton'
import Tooltip from '@mui/material/Tooltip'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import CircularProgress from '@mui/material/CircularProgress'
import StarIcon from '@mui/icons-material/Star'
import StarBorderIcon from '@mui/icons-material/StarBorder'
import GroupAddIcon from '@mui/icons-material/GroupAdd'
import { listEvents, createEvent, setCurrentEvent, copyRoster } from '../api/events'

export default function EventsPage() {
  const [events, setEvents] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [creating, setCreating] = useState(false)
  const [settingCurrent, setSettingCurrent] = useState(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [form, setForm] = useState({ id: '', name: '', startDate: '', endDate: '' })
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [copyTarget, setCopyTarget] = useState(null)
  const [copySource, setCopySource] = useState('')
  const [preview, setPreview] = useState(null)
  const [previewing, setPreviewing] = useState(false)
  const [copying, setCopying] = useState(false)
  const [copyError, setCopyError] = useState('')
  const [copyResult, setCopyResult] = useState(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const data = await listEvents()
      setEvents(data)
    } catch {
      setError('Failed to load events.')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { load() }, [load])

  async function handleCreate() {
    if (!form.id.trim() || !form.name.trim()) {
      setFormError('ID and name are required.')
      return
    }
    setSaving(true)
    setFormError('')
    try {
      const event = await createEvent({
        id: form.id.trim(),
        name: form.name.trim(),
        startDate: form.startDate ? new Date(form.startDate).toISOString() : new Date().toISOString(),
        endDate: form.endDate ? new Date(form.endDate).toISOString() : new Date().toISOString(),
      })
      setEvents(prev => [event, ...prev])
      setDialogOpen(false)
      setForm({ id: '', name: '', startDate: '', endDate: '' })
    } catch (err) {
      setFormError(err.message || 'Failed to create event.')
    } finally {
      setSaving(false)
    }
  }

  async function handleSetCurrent(id) {
    setSettingCurrent(id)
    setError('')
    try {
      const updated = await setCurrentEvent(id)
      setEvents(prev => prev.map(e => ({ ...e, isCurrent: e.id === updated.id })))
    } catch (err) {
      setError(err.message || 'Failed to set current event.')
    } finally {
      setSettingCurrent(null)
    }
  }

  function openCopy(event) {
    setCopyTarget(event)
    setCopySource('')
    setPreview(null)
    setCopyError('')
    setCopyResult(null)
  }

  async function handleSourceChange(sourceId) {
    setCopySource(sourceId)
    setPreview(null)
    setCopyError('')
    setCopyResult(null)
    if (!sourceId) return
    setPreviewing(true)
    try {
      setPreview(await copyRoster(copyTarget.id, sourceId, { dryRun: true }))
    } catch (err) {
      setCopyError(err.message || 'Failed to preview the carry-forward.')
    } finally {
      setPreviewing(false)
    }
  }

  async function handleCopy() {
    setCopying(true)
    setCopyError('')
    try {
      setCopyResult(await copyRoster(copyTarget.id, copySource))
    } catch (err) {
      setCopyError(err.message || 'Failed to copy the roster.')
    } finally {
      setCopying(false)
    }
  }

  return (
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Typography variant="h5" fontWeight={700}>Events</Typography>
        <Button variant="contained" onClick={() => setDialogOpen(true)}>
          New Event
        </Button>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError('')}>{error}</Alert>
      )}

      <TableContainer component={Paper} variant="outlined">
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>ID</TableCell>
              <TableCell>Name</TableCell>
              <TableCell>Dates</TableCell>
              <TableCell>Status</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading
              ? Array.from({ length: 3 }).map((_, i) => (
                  <TableRow key={i}>
                    {Array.from({ length: 5 }).map((_, j) => (
                      <TableCell key={j}><Skeleton /></TableCell>
                    ))}
                  </TableRow>
                ))
              : events.length === 0
                ? (
                  <TableRow>
                    <TableCell colSpan={5} align="center" sx={{ color: 'text.secondary', py: 3 }}>
                      No events yet
                    </TableCell>
                  </TableRow>
                )
                : events.map(event => (
                  <TableRow key={event.id} hover>
                    <TableCell sx={{ fontFamily: 'monospace' }}>{event.id}</TableCell>
                    <TableCell>{event.name}</TableCell>
                    <TableCell>
                      {event.startDate
                        ? `${new Date(event.startDate).toLocaleDateString()} – ${new Date(event.endDate).toLocaleDateString()}`
                        : '—'}
                    </TableCell>
                    <TableCell>
                      {event.isCurrent && (
                        <Chip label="Current" color="primary" size="small" />
                      )}
                    </TableCell>
                    <TableCell align="right">
                      <Box sx={{ display: 'flex', gap: 1, justifyContent: 'flex-end' }}>
                        <Tooltip title="Add another event's competitors to this roster">
                          <Button
                            size="small"
                            variant="outlined"
                            startIcon={<GroupAddIcon />}
                            onClick={() => openCopy(event)}
                          >
                            Carry Forward
                          </Button>
                        </Tooltip>
                        <Tooltip title={event.isCurrent ? 'Already current event' : 'Set as current event'}>
                          <span>
                            <Button
                              size="small"
                              variant={event.isCurrent ? 'contained' : 'outlined'}
                              startIcon={event.isCurrent ? <StarIcon /> : <StarBorderIcon />}
                              disabled={event.isCurrent || settingCurrent === event.id}
                              onClick={() => handleSetCurrent(event.id)}
                            >
                              {event.isCurrent ? 'Current' : settingCurrent === event.id ? 'Setting…' : 'Set Current'}
                            </Button>
                          </span>
                        </Tooltip>
                      </Box>
                    </TableCell>
                  </TableRow>
                ))}
          </TableBody>
        </Table>
      </TableContainer>

      <Dialog open={!!copyTarget} onClose={() => !copying && setCopyTarget(null)} maxWidth="xs" fullWidth>
        <DialogTitle>Carry Forward Roster</DialogTitle>
        <DialogContent>
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, pt: 1 }}>
            <Typography variant="body2" color="text.secondary">
              Adds every competitor from the event you pick to{' '}
              <strong>{copyTarget?.name}</strong>. Nobody is marked as checked in, and
              anyone already on this roster is left untouched.
            </Typography>

            <FormControl fullWidth>
              <InputLabel>Copy From</InputLabel>
              <Select
                value={copySource}
                label="Copy From"
                disabled={copying || !!copyResult}
                onChange={e => handleSourceChange(e.target.value)}
              >
                {events.filter(e => e.id !== copyTarget?.id).map(e => (
                  <MenuItem key={e.id} value={e.id}>{e.name} ({e.id})</MenuItem>
                ))}
              </Select>
            </FormControl>

            {previewing && (
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <CircularProgress size={16} />
                <Typography variant="body2" color="text.secondary">Checking…</Typography>
              </Box>
            )}

            {preview && !copyResult && (
              <Alert severity={preview.copied > 0 ? 'info' : 'warning'}>
                {preview.copied > 0
                  ? `${preview.copied} of ${preview.sourceRoster} competitors will be added. ${preview.alreadyOnTarget} are already on this roster.`
                  : `Nothing to add — all ${preview.sourceRoster} competitors are already on this roster.`}
              </Alert>
            )}

            {copyResult && (
              <Alert severity="success">
                Added {copyResult.copied} competitor{copyResult.copied === 1 ? '' : 's'} to {copyTarget?.name}.
              </Alert>
            )}

            {copyError && <Alert severity="error">{copyError}</Alert>}
          </Box>
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2 }}>
          <Button onClick={() => setCopyTarget(null)} disabled={copying}>
            {copyResult ? 'Done' : 'Cancel'}
          </Button>
          {!copyResult && (
            <Button
              variant="contained"
              onClick={handleCopy}
              disabled={copying || previewing || !preview || preview.copied === 0}
            >
              {copying ? 'Copying…' : `Add ${preview?.copied ?? 0}`}
            </Button>
          )}
        </DialogActions>
      </Dialog>

      <Dialog open={dialogOpen} onClose={() => !saving && setDialogOpen(false)} maxWidth="xs" fullWidth>
        <DialogTitle>New Event</DialogTitle>
        <DialogContent>
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, pt: 1 }}>
            <TextField
              label="Event ID"
              placeholder="glr-2027"
              value={form.id}
              onChange={e => setForm(p => ({ ...p, id: e.target.value }))}
              helperText="Short slug used in the system (e.g. glr-2027)"
              fullWidth
            />
            <TextField
              label="Name"
              placeholder="GLR 2027"
              value={form.name}
              onChange={e => setForm(p => ({ ...p, name: e.target.value }))}
              fullWidth
            />
            <Box sx={{ display: 'flex', gap: 2 }}>
              <TextField
                label="Start Date"
                type="date"
                value={form.startDate}
                onChange={e => setForm(p => ({ ...p, startDate: e.target.value }))}
                slotProps={{ inputLabel: { shrink: true } }}
                fullWidth
              />
              <TextField
                label="End Date"
                type="date"
                value={form.endDate}
                onChange={e => setForm(p => ({ ...p, endDate: e.target.value }))}
                slotProps={{ inputLabel: { shrink: true } }}
                fullWidth
              />
            </Box>
            {formError && <Alert severity="error">{formError}</Alert>}
          </Box>
        </DialogContent>
        <DialogActions sx={{ px: 3, pb: 2 }}>
          <Button onClick={() => { setDialogOpen(false); setFormError('') }} disabled={saving}>Cancel</Button>
          <Button variant="contained" onClick={handleCreate} disabled={saving}>
            {saving ? 'Creating…' : 'Create'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}
