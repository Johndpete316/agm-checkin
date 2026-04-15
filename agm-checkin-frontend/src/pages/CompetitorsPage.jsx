import { useState, useEffect, useCallback, useMemo } from 'react'
import EditIcon from '@mui/icons-material/Edit'
import ViewColumnIcon from '@mui/icons-material/ViewColumn'
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
import Popover from '@mui/material/Popover'
import FormGroup from '@mui/material/FormGroup'
import FormControlLabel from '@mui/material/FormControlLabel'
import Checkbox from '@mui/material/Checkbox'
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
import AddCompetitorDialog from '../components/AddCompetitorDialog'

// Chronological order used for event filter display
const EVENT_ORDER = ['nat-2024', 'glr-2025', 'nat-2025', 'glr-2026']

// Each column has a unique key, an optional sort field, and a label.
// sort: null means the column is not sortable.
const COLUMNS = [
  { key: 'name',        sort: 'nameLast',           label: 'Name' },
  { key: 'event',       sort: 'lastRegisteredEvent', label: 'Event' },
  { key: 'studio',      sort: 'studio',              label: 'Studio' },
  { key: 'teacher',     sort: 'teacher',             label: 'Teacher' },
  { key: 'shirt',       sort: 'shirtSize',           label: 'Shirt' },
  { key: 'dob',         sort: 'dateOfBirth',         label: 'DOB / Age' },
  { key: 'email',       sort: 'email',               label: 'Email' },
  { key: 'validated',   sort: 'validated',           label: 'Validated' },
  { key: 'status',      sort: null,                  label: 'Status' },
  { key: 'checkinTime', sort: null,                  label: 'Check-In Time' },
  { key: 'note',        sort: 'note',               label: 'Note' },
]

// note and checkinTime are off by default — too wide for most workflows
const DEFAULT_VISIBLE_KEYS = new Set(
  COLUMNS.filter(c => c.key !== 'note' && c.key !== 'checkinTime').map(c => c.key)
)

function loadVisibleColumns() {
  try {
    const stored = localStorage.getItem('agm_competitors_columns')
    if (stored) {
      const arr = JSON.parse(stored)
      if (Array.isArray(arr) && arr.length > 0) return new Set(arr)
    }
  } catch {}
  return new Set(DEFAULT_VISIBLE_KEYS)
}

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
  const [addOpen, setAddOpen] = useState(false)

  // Column visibility — persisted in localStorage
  const [visibleColumns, setVisibleColumns] = useState(loadVisibleColumns)
  const [columnsAnchor, setColumnsAnchor] = useState(null)

  // Event filter — null means all events shown
  const [eventFilter, setEventFilter] = useState(null)

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

  // Derive which event IDs are present in the loaded data, in chronological order
  const availableEvents = useMemo(() => {
    const found = new Set(competitors.map(c => c.lastRegisteredEvent).filter(Boolean))
    return EVENT_ORDER.filter(e => found.has(e))
  }, [competitors])

  const handleSort = (sortField) => {
    const isAsc = orderBy === sortField && order === 'asc'
    setOrder(isAsc ? 'desc' : 'asc')
    setOrderBy(sortField)
  }

  const toggleColumn = (key) => {
    setVisibleColumns(prev => {
      const next = new Set(prev)
      if (next.has(key)) {
        if (next.size === 1) return prev // always keep at least one column
        next.delete(key)
      } else {
        next.add(key)
      }
      localStorage.setItem('agm_competitors_columns', JSON.stringify([...next]))
      return next
    })
  }

  const toggleEvent = (eventId) => {
    const current = eventFilter ?? new Set(availableEvents)
    const next = new Set(current)
    if (next.has(eventId)) next.delete(eventId)
    else next.add(eventId)
    // Normalize: if all events are selected, store null (no filter active)
    setEventFilter(next.size === availableEvents.length ? null : next)
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

  // Apply event filter
  const displayed = eventFilter === null
    ? sorted
    : sorted.filter(c => eventFilter.has(c.lastRegisteredEvent))

  const vis = (key) => visibleColumns.has(key)

  return (
    <Box sx={{ mt: 4 }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
        <Typography variant="h5">All Competitors</Typography>
        {isAdmin && (
          <Button variant="contained" onClick={() => setAddOpen(true)}>
            Add Competitor
          </Button>
        )}
      </Box>

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
          {/* Toolbar: event filter (all breakpoints) + Columns button (desktop only) */}
          {availableEvents.length > 1 && (
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mb: 2, flexWrap: 'wrap' }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flexWrap: 'wrap' }}>
                <Typography variant="body2" color="text.secondary" sx={{ mr: 0.5 }}>
                  Event:
                </Typography>
                <FormGroup row>
                  {availableEvents.map(eventId => (
                    <FormControlLabel
                      key={eventId}
                      control={
                        <Checkbox
                          size="small"
                          checked={eventFilter === null || eventFilter.has(eventId)}
                          onChange={() => toggleEvent(eventId)}
                        />
                      }
                      label={<Typography variant="body2">{eventId}</Typography>}
                      sx={{ mr: 1.5 }}
                    />
                  ))}
                </FormGroup>
              </Box>
              <Box sx={{ ml: 'auto', display: { xs: 'none', md: 'block' } }}>
                <Button
                  size="small"
                  variant="outlined"
                  startIcon={<ViewColumnIcon />}
                  onClick={e => setColumnsAnchor(e.currentTarget)}
                >
                  Columns
                </Button>
              </Box>
            </Box>
          )}

          {/* Show Columns button even when there's only one event */}
          {availableEvents.length <= 1 && (
            <Box sx={{ display: { xs: 'none', md: 'flex' }, justifyContent: 'flex-end', mb: 2 }}>
              <Button
                size="small"
                variant="outlined"
                startIcon={<ViewColumnIcon />}
                onClick={e => setColumnsAnchor(e.currentTarget)}
              >
                Columns
              </Button>
            </Box>
          )}

          {/* Mobile card list */}
          <Box sx={{ display: { xs: 'flex', md: 'none' }, flexDirection: 'column', gap: 1.5 }}>
            {displayed.map(competitor => {
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
                      {competitor.validated ? (
                          <Chip icon={<CheckCircleOutlineIcon />} label="Validated" color="success" size="small" variant="outlined" />
                        ) : (
                          <Chip icon={<WarningAmberIcon />} label="Validate" color="warning" size="small" variant="outlined" />
                        )
                      }
                    </Box>
                  </Box>
                  <Divider sx={{ my: 1 }} />
                  <Box sx={{ display: 'flex', gap: 2.5, flexWrap: 'wrap', mb: 0.5 }}>
                    <Box>
                      <Typography variant="caption" color="text.secondary" display="block" sx={{ lineHeight: 1.3 }}>Age</Typography>
                      <Typography variant="body1" fontWeight={700}>{age !== null ? `${age} yrs` : '—'}</Typography>
                    </Box>
                    {(isAdmin || (competitor.requiresValidation && !competitor.validated)) && (
                      <Box>
                        <Typography variant="caption" color="text.secondary" display="block" sx={{ lineHeight: 1.3 }}>Date of Birth</Typography>
                        <Typography variant="body1" fontWeight={700}>{dob || '—'}</Typography>
                      </Box>
                    )}
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
            <Table
              size="small"
              sx={{
                '& td, & th': { fontSize: '0.78rem', px: 1.25, py: 0.6 },
                '& tbody tr:nth-of-type(even)': {
                  bgcolor: 'action.hover',
                },
              }}
            >
              <TableHead>
                <TableRow sx={{ '& th': { fontWeight: 600 } }}>
                  {COLUMNS.map(col => vis(col.key) && (
                    <TableCell key={col.key}>
                      {col.sort ? (
                        <TableSortLabel
                          active={orderBy === col.sort}
                          direction={orderBy === col.sort ? order : 'asc'}
                          onClick={() => handleSort(col.sort)}
                        >
                          {col.label}
                        </TableSortLabel>
                      ) : col.label}
                    </TableCell>
                  ))}
                  <TableCell>Action</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {displayed.map(competitor => {
                  const age = calculateAge(competitor.dateOfBirth)
                  const dob = formatDOB(competitor.dateOfBirth)
                  return (
                    <TableRow key={competitor.id} hover>
                      {vis('name') && (
                        <TableCell>{competitor.nameFirst} {competitor.nameLast}</TableCell>
                      )}
                      {vis('event') && (
                        <TableCell sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                          {competitor.lastRegisteredEvent || '—'}
                        </TableCell>
                      )}
                      {vis('studio') && (
                        <TableCell>{competitor.studio || '—'}</TableCell>
                      )}
                      {vis('teacher') && (
                        <TableCell>{competitor.teacher || '—'}</TableCell>
                      )}
                      {vis('shirt') && (
                        <TableCell>{competitor.shirtSize || '—'}</TableCell>
                      )}
                      {vis('dob') && (
                        <TableCell sx={{ whiteSpace: 'nowrap' }}>
                          {(isAdmin || (competitor.requiresValidation && !competitor.validated))
                            ? (dob ? `${dob}${age !== null ? ` · ${age} yrs` : ''}` : '—')
                            : (age !== null ? `${age} yrs` : '—')
                          }
                        </TableCell>
                      )}
                      {vis('email') && (
                        <TableCell>{competitor.email || '—'}</TableCell>
                      )}
                      {vis('validated') && (
                        <TableCell>
                          {competitor.validated ? (
                            <Tooltip title="Validated">
                              <CheckCircleOutlineIcon fontSize="small" color="success" />
                            </Tooltip>
                          ) : competitor.requiresValidation ? (
                            <Tooltip title="Requires validation">
                              <WarningAmberIcon fontSize="small" color="warning" />
                            </Tooltip>
                          ) : (
                            <Typography variant="body2" color="text.disabled">—</Typography>
                          )}
                        </TableCell>
                      )}
                      {vis('status') && (
                        <TableCell>
                          <Chip
                            label={isCheckedIn(competitor) ? 'Checked In' : 'Pending'}
                            color={isCheckedIn(competitor) ? 'success' : 'default'}
                            size="small"
                          />
                        </TableCell>
                      )}
                      {vis('checkinTime') && (
                        <TableCell sx={{ whiteSpace: 'nowrap' }}>
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
                      )}
                      {vis('note') && (
                        <TableCell sx={{ maxWidth: 200 }}>
                          {competitor.note ? (
                            <Tooltip title={competitor.note} placement="top">
                              <Typography variant="body2" noWrap sx={{ maxWidth: 190 }}>
                                {competitor.note}
                              </Typography>
                            </Tooltip>
                          ) : <Typography variant="body2" color="text.disabled">—</Typography>}
                        </TableCell>
                      )}
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

      {/* Column visibility popover */}
      <Popover
        open={Boolean(columnsAnchor)}
        anchorEl={columnsAnchor}
        onClose={() => setColumnsAnchor(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
      >
        <Box sx={{ p: 2, minWidth: 180 }}>
          <Typography variant="subtitle2" sx={{ mb: 1 }}>Show columns</Typography>
          <FormGroup>
            {COLUMNS.map(col => (
              <FormControlLabel
                key={col.key}
                control={
                  <Checkbox
                    size="small"
                    checked={vis(col.key)}
                    onChange={() => toggleColumn(col.key)}
                  />
                }
                label={<Typography variant="body2">{col.label}</Typography>}
              />
            ))}
          </FormGroup>
        </Box>
      </Popover>

      <AddCompetitorDialog
        open={addOpen}
        onClose={() => setAddOpen(false)}
        onCreated={created => {
          setCompetitors(prev => [{ ...created, currentCheckIn: null }, ...prev])
          setAddOpen(false)
        }}
      />

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
