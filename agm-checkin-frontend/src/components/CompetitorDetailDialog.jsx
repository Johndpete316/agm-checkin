import { useEffect, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Alert from '@mui/material/Alert'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import CircularProgress from '@mui/material/CircularProgress'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import Divider from '@mui/material/Divider'
import FormControl from '@mui/material/FormControl'
import FormControlLabel from '@mui/material/FormControlLabel'
import InputLabel from '@mui/material/InputLabel'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import Switch from '@mui/material/Switch'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableHead from '@mui/material/TableHead'
import TableRow from '@mui/material/TableRow'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import useMediaQuery from '@mui/material/useMediaQuery'
import { useTheme } from '@mui/material/styles'
import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline'
import WarningAmberIcon from '@mui/icons-material/WarningAmber'
import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutline'
import EditIcon from '@mui/icons-material/Edit'
import SaveIcon from '@mui/icons-material/Save'
import CancelIcon from '@mui/icons-material/Cancel'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp'
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown'
import {
  EVENT_SCOPE_ALL,
  checkInCompetitor,
  createCompetitorScheduleEntry,
  deleteCompetitorScheduleEntry,
  getCompetitorSchedule,
  updateCompetitor,
  updateCompetitorDOB,
  updateCompetitorScheduleEntry,
  validateCompetitor,
} from '../api/competitors'
import { listEvents } from '../api/events'
import { useAuth } from '../context/AuthContext'

const SHIRT_SIZES = ['XS', 'S', 'M', 'L', 'XL', 'XXL']

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

function toInputDate(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime()) || d.getFullYear() < 1900) return ''
  const y = d.getUTCFullYear()
  const m = String(d.getUTCMonth() + 1).padStart(2, '0')
  const day = String(d.getUTCDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

function formatScheduleDate(dateStr) {
  if (!dateStr) return '-'
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return '-'
  return d.toLocaleDateString(undefined, { weekday: 'short', month: 'numeric', day: 'numeric', timeZone: 'UTC' })
}

function toScheduleInputDate(iso) {
  if (!iso) return ''
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  const y = d.getUTCFullYear()
  const m = String(d.getUTCMonth() + 1).padStart(2, '0')
  const day = String(d.getUTCDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

function toTwentyFourHourTime(timeStr) {
  const value = (timeStr || '').trim()
  if (!value) return ''
  const ampm = value.match(/^(\d{1,2}):(\d{2})\s*([AaPp][Mm])$/)
  if (ampm) {
    let hour = Number(ampm[1])
    const minute = ampm[2]
    const period = ampm[3].toLowerCase()
    if (hour === 12) hour = 0
    if (period === 'pm') hour += 12
    return `${String(hour).padStart(2, '0')}:${minute}`
  }

  const twentyFour = value.match(/^(\d{1,2}):(\d{2})$/)
  if (!twentyFour) return ''
  const hour = Number(twentyFour[1])
  const minute = Number(twentyFour[2])
  if (hour < 0 || hour > 23 || minute < 0 || minute > 59) return ''
  return `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`
}

function toDateTimeLocalValue(scheduleDate, scheduleTime) {
  const date = (scheduleDate || '').trim()
  const time24 = toTwentyFourHourTime(scheduleTime)
  if (!date || !time24) return ''
  return `${date}T${time24}`
}

function fromDateTimeLocalValue(value) {
  if (!value || !value.includes('T')) return { scheduleDate: '', scheduleTime: '' }
  const [scheduleDate, scheduleTime] = value.split('T')
  return { scheduleDate, scheduleTime: scheduleTime.slice(0, 5) }
}

function scheduleEntryToDraft(entry, idx) {
  return {
    localId: entry.id || `new-${idx}-${Math.random().toString(36).slice(2, 8)}`,
    id: entry.id,
    eventId: entry.eventId || '',
    scheduleDate: toScheduleInputDate(entry.scheduleDate),
    scheduleTime: entry.scheduleTime || '',
    pageNumber: entry.pageNumber || '',
    room: entry.room || '',
    instrument: entry.instrument || '',
    category: entry.category || '',
    division: entry.division || '',
  }
}

function makeNewScheduleRow(eventId) {
  return {
    localId: `new-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
    id: '',
    eventId: eventId || '',
    scheduleDate: '',
    scheduleTime: '',
    pageNumber: '',
    room: '',
    instrument: '',
    category: '',
    division: '',
  }
}

function competitorToForm(competitor) {
  return {
    nameFirst: competitor.nameFirst ?? '',
    nameLast: competitor.nameLast ?? '',
    dateOfBirth: toInputDate(competitor.dateOfBirth),
    email: competitor.email ?? '',
    studio: competitor.studio ?? '',
    teacher: competitor.teacher ?? '',
    shirtSize: competitor.shirtSize ?? '',
    registerForEvent: '',
    dobVerified: !!competitor.dobVerifiedAt,
    note: competitor.note ?? '',
  }
}

export default function CompetitorDetailDialog({ open, competitor, eventScope, onClose, onUpdated }) {
  const { isAdmin } = useAuth()
  const theme = useTheme()
  const fullScreen = useMediaQuery(theme.breakpoints.down('sm'))

  const [editing, setEditing] = useState(false)
  const [schedule, setSchedule] = useState([])
  const [scheduleDraft, setScheduleDraft] = useState([])
  const [scheduleLoading, setScheduleLoading] = useState(false)
  const [scheduleError, setScheduleError] = useState('')

  const [events, setEvents] = useState([])
  const [eventsLoading, setEventsLoading] = useState(false)

  const [form, setForm] = useState({})
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [checkingIn, setCheckingIn] = useState(false)

  const [validateOpen, setValidateOpen] = useState(false)
  const [editedDOB, setEditedDOB] = useState('')
  const [confirming, setConfirming] = useState(false)
  const [dialogError, setDialogError] = useState('')

  const selectedEventId = useMemo(() => {
    if (!eventScope || eventScope === EVENT_SCOPE_ALL) return undefined
    return eventScope
  }, [eventScope])

  const scheduleEventOptions = useMemo(() => {
    const base = events.map(event => ({ id: event.id, name: event.name || event.id }))
    const seen = new Set(base.map(event => event.id))
    for (const row of scheduleDraft) {
      if (row.eventId && !seen.has(row.eventId)) {
        base.push({ id: row.eventId, name: row.eventId })
        seen.add(row.eventId)
      }
    }
    return base
  }, [events, scheduleDraft])

  useEffect(() => {
    if (!open || !competitor) return

    setEditing(false)
    setError('')
    setForm(competitorToForm(competitor))

    setScheduleLoading(true)
    setScheduleError('')
    getCompetitorSchedule(competitor.id, selectedEventId)
      .then(data => {
        const list = Array.isArray(data) ? data : []
        setSchedule(list)
        setScheduleDraft(list.map((entry, idx) => scheduleEntryToDraft(entry, idx)))
      })
      .catch(() => {
        setSchedule([])
        setScheduleDraft([])
        setScheduleError('Failed to load schedule.')
      })
      .finally(() => setScheduleLoading(false))
  }, [open, competitor, selectedEventId])

  useEffect(() => {
    if (!open || !competitor || !editing || !isAdmin) return

    setEventsLoading(true)
    listEvents()
      .then(data => setEvents(Array.isArray(data) ? data : []))
      .catch(() => setEvents([]))
      .finally(() => setEventsLoading(false))
  }, [open, competitor, editing, isAdmin])

  if (!competitor) return null

  const isCheckedIn = !!competitor.currentCheckIn?.checkedIn
  const needsValidation = !competitor.dobVerifiedAt
  const age = calculateAge(competitor.dateOfBirth)
  const dob = formatDOB(competitor.dateOfBirth)

  const setField = (field, value) => {
    setForm(prev => ({ ...prev, [field]: value }))
  }

  const setScheduleField = (localId, field, value) => {
    setScheduleDraft(prev => prev.map(row => (row.localId === localId ? { ...row, [field]: value } : row)))
  }

  const removeScheduleRow = (localId) => {
    setScheduleDraft(prev => prev.filter(row => row.localId !== localId))
  }

  const addScheduleRow = () => {
    const fallbackEventId = selectedEventId || events[0]?.id || ''
    setScheduleDraft(prev => [...prev, makeNewScheduleRow(fallbackEventId)])
  }

  const moveScheduleRow = (fromIndex, toIndex) => {
    if (toIndex < 0 || toIndex >= scheduleDraft.length) return
    setScheduleDraft(prev => {
      const next = [...prev]
      const [row] = next.splice(fromIndex, 1)
      next.splice(toIndex, 0, row)
      return next
    })
  }

  const setScheduleDateTime = (localId, value) => {
    const next = fromDateTimeLocalValue(value)
    setScheduleDraft(prev => prev.map(row => (
      row.localId === localId
        ? { ...row, scheduleDate: next.scheduleDate, scheduleTime: next.scheduleTime }
        : row
    )))
  }

  const refreshSchedule = async () => {
    const refreshed = await getCompetitorSchedule(competitor.id, selectedEventId)
    const list = Array.isArray(refreshed) ? refreshed : []
    setSchedule(list)
    setScheduleDraft(list.map((entry, idx) => scheduleEntryToDraft(entry, idx)))
  }

  const schedulePayload = (row, index) => ({
    eventId: row.eventId,
    instrument: row.instrument.trim(),
    scheduleDate: `${row.scheduleDate}T00:00:00Z`,
    scheduleTime: row.scheduleTime.trim(),
    room: row.room.trim(),
    category: row.category.trim(),
    division: row.division.trim(),
    pageNumber: row.pageNumber.trim(),
    sortOrder: index,
  })

  const validateScheduleDraft = () => {
    for (let i = 0; i < scheduleDraft.length; i++) {
      const row = scheduleDraft[i]
      if (!row.eventId) throw new Error(`Schedule row ${i + 1}: Event is required.`)
      if (!row.scheduleDate) throw new Error(`Schedule row ${i + 1}: Day is required.`)
      if (!row.scheduleTime.trim()) throw new Error(`Schedule row ${i + 1}: Time is required.`)
      if (!row.instrument.trim()) throw new Error(`Schedule row ${i + 1}: Instrument is required.`)
      if (!row.category.trim()) throw new Error(`Schedule row ${i + 1}: Category is required.`)
    }
  }

  const handleClose = () => {
    if (!saving && !checkingIn && !confirming) {
      onClose()
    }
  }

  const updateLocalFromServer = (updated) => {
    onUpdated?.(updated)
  }

  const doCheckIn = async () => {
    setCheckingIn(true)
    setError('')
    try {
      const updated = await checkInCompetitor(competitor.id)
      updateLocalFromServer(updated)
    } catch (err) {
      setError(err.message || 'Failed to check in competitor.')
    } finally {
      setCheckingIn(false)
    }
  }

  const handleCheckInClick = () => {
    if (needsValidation) {
      setEditedDOB(toInputDate(competitor.dateOfBirth))
      setDialogError('')
      setValidateOpen(true)
      return
    }
    doCheckIn()
  }

  const handleConfirmValidation = async () => {
    setConfirming(true)
    setDialogError('')
    try {
      const originalDOB = toInputDate(competitor.dateOfBirth)
      if (editedDOB && editedDOB !== originalDOB) {
        const updated = await updateCompetitorDOB(competitor.id, editedDOB)
        updateLocalFromServer(updated)
      }
      const validated = await validateCompetitor(competitor.id)
      updateLocalFromServer(validated)
      setValidateOpen(false)
      await doCheckIn()
    } catch {
      setDialogError('Failed to save. Please try again.')
    } finally {
      setConfirming(false)
    }
  }

  const handleSave = async () => {
    setSaving(true)
    setError('')
    try {
      validateScheduleDraft()

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
        dobVerifiedAt: form.dobVerified ? (competitor.dobVerifiedAt ?? new Date().toISOString()) : null,
        note: form.note,
      }
      const updated = await updateCompetitor(competitor.id, payload)

      const originalById = new Map(schedule.filter(entry => !!entry.id).map(entry => [entry.id, entry]))
      const keptIds = new Set(scheduleDraft.filter(row => !!row.id).map(row => row.id))

      for (const id of originalById.keys()) {
        if (!keptIds.has(id)) {
          await deleteCompetitorScheduleEntry(id)
        }
      }

      for (const [index, row] of scheduleDraft.entries()) {
        const body = schedulePayload(row, index)
        if (row.id) {
          await updateCompetitorScheduleEntry(row.id, body)
        } else {
          await createCompetitorScheduleEntry(competitor.id, body)
        }
      }

      await refreshSchedule()
      updateLocalFromServer(updated)
      setEditing(false)
    } catch (err) {
      setError(err.message || 'Failed to save changes.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <Dialog
        open={open}
        onClose={handleClose}
        maxWidth="lg"
        fullWidth
        fullScreen={fullScreen}
      >
        <DialogTitle>
          {editing ? 'Edit Competitor' : `${competitor.nameFirst} ${competitor.nameLast}`}
        </DialogTitle>
        <DialogContent>
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, pt: 1 }}>
            {error && <Alert severity="error">{error}</Alert>}

            {!editing && (
              <>
                <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap', alignItems: 'center' }}>
                  <Chip
                    label={isCheckedIn ? 'Checked In' : 'Pending'}
                    color={isCheckedIn ? 'success' : 'default'}
                    size="small"
                  />
                  {needsValidation ? (
                    <Chip icon={<WarningAmberIcon />} label="Validate" color="warning" size="small" variant="outlined" />
                  ) : (
                    <Chip icon={<CheckCircleOutlineIcon />} label="Validated" color="success" size="small" variant="outlined" />
                  )}
                  {competitor.mostRecentEvent && (
                    <Chip label={competitor.mostRecentEvent} size="small" variant="outlined" />
                  )}
                </Box>

                <Box
                  sx={{
                    display: 'flex',
                    gap: 3,
                    flexWrap: 'wrap',
                    bgcolor: 'action.hover',
                    borderRadius: 1,
                    px: 2,
                    py: 1.25,
                  }}
                >
                  <Box>
                    <Typography variant="caption" color="text.secondary" display="block">Age</Typography>
                    <Typography variant="body1" fontWeight={700}>{age !== null ? `${age} yrs` : '-'}</Typography>
                  </Box>
                  {(isAdmin || needsValidation) && (
                    <Box>
                      <Typography variant="caption" color="text.secondary" display="block">Date of Birth</Typography>
                      <Typography variant="body1" fontWeight={700}>{dob || '-'}</Typography>
                    </Box>
                  )}
                  <Box>
                    <Typography variant="caption" color="text.secondary" display="block">T-Shirt</Typography>
                    <Typography variant="body1" fontWeight={700}>{competitor.shirtSize || '-'}</Typography>
                  </Box>
                </Box>

                <Box sx={{ display: 'flex', gap: 3, flexWrap: 'wrap' }}>
                  <Typography variant="body2" color="text.secondary"><strong>Studio:</strong> {competitor.studio || '-'}</Typography>
                  <Typography variant="body2" color="text.secondary"><strong>Teacher:</strong> {competitor.teacher || '-'}</Typography>
                  <Typography variant="body2" color="text.secondary"><strong>Email:</strong> {competitor.email || '-'}</Typography>
                </Box>

                {competitor.note && <Alert severity="info">{competitor.note}</Alert>}

                {isCheckedIn && competitor.currentCheckIn?.checkInDatetime && (
                  <Typography variant="body2" color="text.secondary">
                    Checked in at {new Date(competitor.currentCheckIn.checkInDatetime).toLocaleString()}
                    {competitor.currentCheckIn.checkedInBy && ` - ${competitor.currentCheckIn.checkedInBy}`}
                  </Typography>
                )}

                <Divider />
                <Typography variant="subtitle2">Schedule</Typography>
                {scheduleError && <Alert severity="warning">{scheduleError}</Alert>}
                {scheduleLoading && (
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <CircularProgress size={16} />
                    <Typography variant="caption" color="text.secondary">Loading schedule...</Typography>
                  </Box>
                )}
                {!scheduleLoading && schedule.length === 0 && (
                  <Typography variant="body2" color="text.secondary">
                    No schedule entries for this competitor in the selected event.
                  </Typography>
                )}
                {!scheduleLoading && schedule.length > 0 && (
                  <Box sx={{ overflowX: 'auto' }}>
                    <Table size="small" sx={{ minWidth: 500 }}>
                      <TableHead>
                        <TableRow>
                          {['Day', 'Time', 'Page', 'Room', 'Instrument', 'Category', 'Division'].map(h => (
                            <TableCell key={h} sx={{ fontWeight: 600, color: 'text.secondary', whiteSpace: 'nowrap' }}>
                              {h}
                            </TableCell>
                          ))}
                        </TableRow>
                      </TableHead>
                      <TableBody>
                        {schedule.map(entry => (
                          <TableRow key={entry.id}>
                            <TableCell sx={{ whiteSpace: 'nowrap' }}>{formatScheduleDate(entry.scheduleDate)}</TableCell>
                            <TableCell sx={{ whiteSpace: 'nowrap' }}>{entry.scheduleTime || '-'}</TableCell>
                            <TableCell sx={{ whiteSpace: 'nowrap' }}>{entry.pageNumber || '-'}</TableCell>
                            <TableCell sx={{ whiteSpace: 'nowrap' }}>{entry.room || '-'}</TableCell>
                            <TableCell>{entry.instrument || '-'}</TableCell>
                            <TableCell>{entry.category || '-'}</TableCell>
                            <TableCell>{entry.division || '-'}</TableCell>
                          </TableRow>
                        ))}
                      </TableBody>
                    </Table>
                  </Box>
                )}
              </>
            )}

            {editing && isAdmin && (
              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                <Box sx={{ display: 'flex', gap: 2 }}>
                  <TextField
                    label="First Name"
                    value={form.nameFirst ?? ''}
                    onChange={e => setField('nameFirst', e.target.value)}
                    fullWidth
                  />
                  <TextField
                    label="Last Name"
                    value={form.nameLast ?? ''}
                    onChange={e => setField('nameLast', e.target.value)}
                    fullWidth
                  />
                </Box>

                <Box sx={{ display: 'flex', gap: 2 }}>
                  <TextField
                    label="Date of Birth"
                    type="date"
                    value={form.dateOfBirth ?? ''}
                    onChange={e => setField('dateOfBirth', e.target.value)}
                    slotProps={{ inputLabel: { shrink: true } }}
                    fullWidth
                  />
                  <TextField
                    label="Email"
                    value={form.email ?? ''}
                    onChange={e => setField('email', e.target.value)}
                    fullWidth
                  />
                </Box>

                <Box sx={{ display: 'flex', gap: 2 }}>
                  <TextField
                    label="Studio"
                    value={form.studio ?? ''}
                    onChange={e => setField('studio', e.target.value)}
                    fullWidth
                  />
                  <TextField
                    label="Teacher"
                    value={form.teacher ?? ''}
                    onChange={e => setField('teacher', e.target.value)}
                    fullWidth
                  />
                </Box>

                <Box sx={{ display: 'flex', gap: 2 }}>
                  <FormControl fullWidth>
                    <InputLabel>Shirt Size</InputLabel>
                    <Select
                      value={form.shirtSize ?? ''}
                      label="Shirt Size"
                      onChange={e => setField('shirtSize', e.target.value)}
                    >
                      <MenuItem value=""><em>Unknown</em></MenuItem>
                      {SHIRT_SIZES.map(size => <MenuItem key={size} value={size}>{size}</MenuItem>)}
                    </Select>
                  </FormControl>

                  <FormControl fullWidth disabled={eventsLoading}>
                    <InputLabel>Add To Event</InputLabel>
                    <Select
                      value={form.registerForEvent ?? ''}
                      label="Add To Event"
                      onChange={e => setField('registerForEvent', e.target.value)}
                    >
                      <MenuItem value=""><em>No change</em></MenuItem>
                      {events.map(event => (
                        <MenuItem key={event.id} value={event.id}>{event.name} ({event.id})</MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                </Box>

                <TextField
                  label="Note"
                  value={form.note ?? ''}
                  onChange={e => setField('note', e.target.value)}
                  fullWidth
                  multiline
                  minRows={2}
                  placeholder="Internal staff note"
                />

                <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap', alignItems: 'center' }}>
                  <FormControlLabel
                    control={<Switch checked={form.dobVerified ?? false} onChange={e => setField('dobVerified', e.target.checked)} />}
                    label="Date of Birth Verified"
                  />
                  {competitor.dobVerifiedAt && (
                    <Typography variant="caption" color="text.secondary">
                      by {competitor.dobVerifiedBy || 'unknown'} on {new Date(competitor.dobVerifiedAt).toLocaleDateString()}
                    </Typography>
                  )}
                </Box>

                <Divider />
                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                  <Typography variant="subtitle2">Schedule</Typography>
                  <Button size="small" startIcon={<AddCircleOutlineIcon />} onClick={addScheduleRow} disabled={saving}>
                    Add Row
                  </Button>
                </Box>
                <Typography variant="caption" color="text.secondary">
                  Entry order controls check-in sort order.
                </Typography>
                {scheduleDraft.length === 0 && (
                  <Typography variant="body2" color="text.secondary">
                    No schedule entries yet. Add a row to create one.
                  </Typography>
                )}
                {scheduleDraft.map((row, idx) => (
                  <Box
                    key={row.localId}
                    sx={{
                      border: '1px solid',
                      borderColor: 'divider',
                      borderRadius: 1,
                      p: 1.5,
                      display: 'flex',
                      flexDirection: 'column',
                      gap: 1,
                    }}
                  >
                    <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 1 }}>
                      <Typography variant="caption" color="text.secondary">Entry {idx + 1}</Typography>
                      <Box sx={{ display: 'flex', gap: 0.5, alignItems: 'center' }}>
                        <Button
                          size="small"
                          onClick={() => moveScheduleRow(idx, idx - 1)}
                          disabled={saving || idx === 0}
                          startIcon={<KeyboardArrowUpIcon />}
                        >
                          Up
                        </Button>
                        <Button
                          size="small"
                          onClick={() => moveScheduleRow(idx, idx + 1)}
                          disabled={saving || idx === scheduleDraft.length - 1}
                          startIcon={<KeyboardArrowDownIcon />}
                        >
                          Down
                        </Button>
                        <Button
                          size="small"
                          color="error"
                          startIcon={<DeleteOutlineIcon />}
                          onClick={() => removeScheduleRow(row.localId)}
                          disabled={saving}
                        >
                          Remove
                        </Button>
                      </Box>
                    </Box>

                    <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
                      <FormControl size="small" sx={{ minWidth: 150, flex: 1 }} disabled={eventsLoading}>
                        <InputLabel>Event</InputLabel>
                        <Select
                          value={row.eventId}
                          label="Event"
                          onChange={e => setScheduleField(row.localId, 'eventId', e.target.value)}
                        >
                          <MenuItem value=""><em>Select event</em></MenuItem>
                          {scheduleEventOptions.map(event => (
                            <MenuItem key={event.id} value={event.id}>{event.name} ({event.id})</MenuItem>
                          ))}
                        </Select>
                      </FormControl>
                      <TextField
                        label="Date / Time"
                        type="datetime-local"
                        size="small"
                        value={toDateTimeLocalValue(row.scheduleDate, row.scheduleTime)}
                        onChange={e => setScheduleDateTime(row.localId, e.target.value)}
                        slotProps={{ inputLabel: { shrink: true } }}
                        sx={{ minWidth: 220, flex: 1.2 }}
                      />
                      <TextField
                        label="Page"
                        size="small"
                        value={row.pageNumber}
                        onChange={e => setScheduleField(row.localId, 'pageNumber', e.target.value)}
                        sx={{ minWidth: 90, flex: 1 }}
                      />
                    </Box>

                    <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
                      <TextField
                        label="Room"
                        size="small"
                        value={row.room}
                        onChange={e => setScheduleField(row.localId, 'room', e.target.value)}
                        sx={{ minWidth: 95, flex: 1 }}
                      />
                      <TextField
                        label="Instrument"
                        size="small"
                        value={row.instrument}
                        onChange={e => setScheduleField(row.localId, 'instrument', e.target.value)}
                        sx={{ minWidth: 130, flex: 1 }}
                      />
                      <TextField
                        label="Category"
                        size="small"
                        value={row.category}
                        onChange={e => setScheduleField(row.localId, 'category', e.target.value)}
                        sx={{ minWidth: 130, flex: 1 }}
                      />
                      <TextField
                        label="Division"
                        size="small"
                        value={row.division}
                        onChange={e => setScheduleField(row.localId, 'division', e.target.value)}
                        sx={{ minWidth: 130, flex: 1 }}
                      />
                    </Box>
                  </Box>
                ))}
              </Box>
            )}
          </Box>
        </DialogContent>

        <DialogActions
          sx={{
            px: 3,
            pb: 2,
            justifyContent: 'space-between',
            flexWrap: 'wrap',
            gap: 1,
          }}
        >
          <Box sx={{ display: 'flex', gap: 1 }}>
            {!editing && !isCheckedIn && (
              <Button variant="contained" onClick={handleCheckInClick} disabled={checkingIn || saving}>
                {checkingIn ? 'Checking in...' : 'Check In'}
              </Button>
            )}
            {!editing && isAdmin && (
              <Button variant="outlined" startIcon={<EditIcon />} onClick={() => setEditing(true)} disabled={checkingIn || saving}>
                Edit
              </Button>
            )}
            {editing && (
              <>
                <Button variant="contained" startIcon={<SaveIcon />} onClick={handleSave} disabled={saving || checkingIn}>
                  {saving ? 'Saving...' : 'Save'}
                </Button>
                <Button
                  variant="outlined"
                  startIcon={<CancelIcon />}
                  onClick={() => {
                    setForm(competitorToForm(competitor))
                    setScheduleDraft(schedule.map((entry, idx) => scheduleEntryToDraft(entry, idx)))
                    setEditing(false)
                  }}
                  disabled={saving || checkingIn}
                >
                  Cancel Edit
                </Button>
              </>
            )}
          </Box>

          <Button onClick={handleClose} disabled={saving || checkingIn || confirming}>Close</Button>
        </DialogActions>
      </Dialog>

      <Dialog
        open={validateOpen}
        onClose={() => !confirming && setValidateOpen(false)}
        maxWidth="xs"
        fullWidth
      >
        <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <WarningAmberIcon color="warning" />
          Validate Before Check-In
        </DialogTitle>
        <DialogContent>
          <Typography variant="body1" gutterBottom>
            <strong>{competitor.nameFirst} {competitor.nameLast}</strong> requires identity validation.
          </Typography>
          <Box sx={{ display: 'flex', gap: 2, mt: 0.5, mb: 1 }}>
            <Typography variant="body2" color="text.secondary">Studio: {competitor.studio || '-'}</Typography>
            <Typography variant="body2" color="text.secondary">Teacher: {competitor.teacher || '-'}</Typography>
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
          <Button onClick={() => setValidateOpen(false)} disabled={confirming}>Cancel</Button>
          <Button variant="contained" onClick={handleConfirmValidation} disabled={confirming || !editedDOB}>
            {confirming ? 'Saving...' : 'Confirmed - Check In'}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  )
}
