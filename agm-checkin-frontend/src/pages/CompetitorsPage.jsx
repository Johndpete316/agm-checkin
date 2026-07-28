import { useState, useEffect, useRef, useMemo } from 'react'
import ViewColumnIcon from '@mui/icons-material/ViewColumn'
import FilterListIcon from '@mui/icons-material/FilterList'
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
import TextField from '@mui/material/TextField'
import Tooltip from '@mui/material/Tooltip'
import Divider from '@mui/material/Divider'
import Popover from '@mui/material/Popover'
import FormGroup from '@mui/material/FormGroup'
import FormControlLabel from '@mui/material/FormControlLabel'
import Checkbox from '@mui/material/Checkbox'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'
import InputLabel from '@mui/material/InputLabel'
import FormControl from '@mui/material/FormControl'
import WarningAmberIcon from '@mui/icons-material/WarningAmber'
import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutline'
import {
  getCompetitors,
  EVENT_SCOPE_ALL,
  EVENT_SCOPE_CURRENT,
} from '../api/competitors'
import { listEvents, getCurrentEvent } from '../api/events'
import { useAuth } from '../context/AuthContext'
import AddCompetitorDialog from '../components/AddCompetitorDialog'
import CompetitorDetailDialog from '../components/CompetitorDetailDialog'

// Preferred shirt size order
const SHIRT_ORDER = ['XS', 'S', 'M', 'L', 'XL', 'XXL']

// Each column has a unique key, an optional sort field, and a label.
// sort: null means the column is not sortable.
const COLUMNS = [
  { key: 'name',        sort: 'nameLast',           label: 'Name' },
  { key: 'event',       sort: 'mostRecentEvent',     label: 'Event' },
  { key: 'page',        sort: 'pageNumber',          label: 'Page' },
  { key: 'studio',      sort: 'studio',              label: 'Studio' },
  { key: 'teacher',     sort: 'teacher',             label: 'Teacher' },
  { key: 'shirt',       sort: 'shirtSize',           label: 'Shirt' },
  { key: 'dob',         sort: 'dateOfBirth',         label: 'DOB / Age' },
  { key: 'email',       sort: 'email',               label: 'Email' },
  { key: 'validated',   sort: 'dobVerifiedAt',       label: 'Validated' },
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
      if (Array.isArray(arr) && arr.length > 0) {
        const known = new Set(COLUMNS.map(c => c.key))
        const filtered = arr.filter(key => known.has(key))
        const migratedFlag = localStorage.getItem('agm_competitors_columns_page_migrated_v1')
        if (!migratedFlag && !filtered.includes('page')) {
          filtered.push('page')
          localStorage.setItem('agm_competitors_columns', JSON.stringify(filtered))
          localStorage.setItem('agm_competitors_columns_page_migrated_v1', '1')
        }
        return new Set(filtered)
      }
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

export default function CompetitorsPage() {
  const { isAdmin } = useAuth()
  const [competitors, setCompetitors] = useState([])
  const [events, setEvents] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [order, setOrder] = useState('asc')
  const [orderBy, setOrderBy] = useState('nameLast')
  const [selectedCompetitorID, setSelectedCompetitorID] = useState(null)
  const [addOpen, setAddOpen] = useState(false)

  // Column visibility — persisted in localStorage
  const [visibleColumns, setVisibleColumns] = useState(loadVisibleColumns)
  const [columnsAnchor, setColumnsAnchor] = useState(null)
  const [filtersAnchor, setFiltersAnchor] = useState(null)

  // Filters. filterEvent is not one of these — it selects which roster the server
  // sends rather than narrowing what is already loaded.
  const [searchText, setSearchText] = useState('')
  const [filterStudio, setFilterStudio] = useState('')
  const [filterTeacher, setFilterTeacher] = useState('')
  const [filterShirt, setFilterShirt] = useState('')
  const [filterValidated, setFilterValidated] = useState('')
  const [filterStatus, setFilterStatus] = useState('')

  // Starts on the current event so the first load fetches one roster instead of
  // every competitor ever imported. The sentinel avoids waiting on getCurrentEvent
  // to learn the ID, and is swapped for the real one once that resolves.
  const [filterEvent, setFilterEvent] = useState(EVENT_SCOPE_CURRENT)

  // The roster currently in state, so resolving the sentinel to the equivalent
  // concrete event ID does not refetch what we already have.
  const loadedScopeRef = useRef(EVENT_SCOPE_CURRENT)

  const anyFilterActive = searchText || filterStudio || filterTeacher || filterShirt || filterValidated || filterStatus
  const activeFilterCount = [searchText, filterStudio, filterTeacher, filterShirt, filterValidated, filterStatus]
    .filter(Boolean)
    .length

  const clearFilters = () => {
    setSearchText('')
    setFilterStudio('')
    setFilterTeacher('')
    setFilterShirt('')
    setFilterValidated('')
    setFilterStatus('')
  }

  useEffect(() => {
    let cancelled = false

    const load = async () => {
      setLoading(true)
      setError(null)

      // Fire all three in parallel so they overlap on the wire.
      const competitorsPromise = getCompetitors('', EVENT_SCOPE_CURRENT)
      const eventsPromise = listEvents()
      const currentEventPromise = getCurrentEvent()

      // Unblock the table as soon as competitor data arrives — don't wait for filter metadata.
      try {
        const data = await competitorsPromise
        if (!cancelled) setCompetitors(data)
      } catch (err) {
        if (!cancelled) setError(err.message)
      } finally {
        if (!cancelled) setLoading(false)
      }

      // Populate filter state when the secondary requests finish.
      const [eventsResult, currentEventResult] = await Promise.allSettled([eventsPromise, currentEventPromise])
      if (cancelled) return
      if (eventsResult.status === 'fulfilled') setEvents(eventsResult.value)
      if (currentEventResult.status === 'fulfilled' && currentEventResult.value?.id) {
        loadedScopeRef.current = currentEventResult.value.id
        setFilterEvent(currentEventResult.value.id)
      }
    }

    load()
    return () => { cancelled = true }
  }, [])

  // Changing the event selector pulls that roster from the server.
  useEffect(() => {
    if (filterEvent === loadedScopeRef.current) return
    loadedScopeRef.current = filterEvent

    let cancelled = false
    const load = async () => {
      setLoading(true)
      setError(null)
      try {
        const data = await getCompetitors('', filterEvent)
        if (!cancelled) setCompetitors(data)
      } catch (err) {
        if (!cancelled) setError(err.message)
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    load()
    return () => { cancelled = true }
  }, [filterEvent])

  // Every event is selectable, not just those present in the loaded roster —
  // the roster now holds one event, so it can no longer imply the list.
  const availableEvents = useMemo(() => [...events].reverse(), [events])

  // Unique studios, teachers, shirt sizes derived from loaded data
  const uniqueStudios = useMemo(() => {
    const vals = [...new Set(competitors.map(c => c.studio).filter(Boolean))].sort()
    return vals
  }, [competitors])

  const uniqueTeachers = useMemo(() => {
    const vals = [...new Set(competitors.map(c => c.teacher).filter(Boolean))].sort()
    return vals
  }, [competitors])

  const uniqueShirts = useMemo(() => {
    const found = new Set(competitors.map(c => c.shirtSize).filter(Boolean))
    const ordered = SHIRT_ORDER.filter(s => found.has(s))
    const rest = [...found].filter(s => !SHIRT_ORDER.includes(s)).sort()
    return [...ordered, ...rest]
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

  const updateLocalCompetitor = (updated) => {
    setCompetitors(prev => prev.map(c => (c.id === updated.id ? { ...c, ...updated } : c)))
  }

  const isCheckedIn = (competitor) => !!competitor.currentCheckIn?.checkedIn

  const displayed = useMemo(() => {
    let list = [...competitors].sort(getComparator(order, orderBy))

    if (searchText.trim()) {
      const q = searchText.trim().toLowerCase()
      list = list.filter(c =>
        `${c.nameFirst} ${c.nameLast}`.toLowerCase().includes(q) ||
        (c.studio || '').toLowerCase().includes(q) ||
        (c.teacher || '').toLowerCase().includes(q) ||
        (c.email || '').toLowerCase().includes(q) ||
        (c.shirtSize || '').toLowerCase().includes(q)
      )
    }
    // No event filter here — the server already scoped the roster to filterEvent.
    if (filterStudio)  list = list.filter(c => c.studio === filterStudio)
    if (filterTeacher) list = list.filter(c => c.teacher === filterTeacher)
    if (filterShirt)   list = list.filter(c => c.shirtSize === filterShirt)
    if (filterValidated === 'yes')   list = list.filter(c => !!c.dobVerifiedAt)
    if (filterValidated === 'needs') list = list.filter(c => !c.dobVerifiedAt)
    if (filterStatus === 'checkedin') list = list.filter(c => !!c.currentCheckIn?.checkedIn)
    if (filterStatus === 'pending')   list = list.filter(c => !c.currentCheckIn?.checkedIn)

    return list
  }, [competitors, order, orderBy, searchText, filterStudio, filterTeacher, filterShirt, filterValidated, filterStatus])

  const selectedCompetitor = useMemo(
    () => competitors.find(c => c.id === selectedCompetitorID) || null,
    [competitors, selectedCompetitorID]
  )

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
          {/* Search + controls */}
          <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1, alignItems: 'center', mb: 1 }}>
            <TextField
              size="small"
              placeholder="Search name, studio, teacher, email, shirt…"
              value={searchText}
              onChange={e => setSearchText(e.target.value)}
              sx={{ minWidth: 220, flex: 1 }}
            />

            <Button
              size="small"
              variant={anyFilterActive ? 'contained' : 'outlined'}
              startIcon={<FilterListIcon />}
              onClick={e => setFiltersAnchor(e.currentTarget)}
            >
              Filters{activeFilterCount ? ` (${activeFilterCount})` : ''}
            </Button>

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

          {anyFilterActive && (
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 1 }}>
              {displayed.length} of {competitors.length} competitors
            </Typography>
          )}

          <Typography
            variant="caption"
            color="text.secondary"
            sx={{ display: { xs: 'none', md: 'block' }, mb: 1 }}
          >
            Click any row to open details and actions.
          </Typography>

          {/* Mobile card list */}
          <Box sx={{ display: { xs: 'flex', md: 'none' }, flexDirection: 'column', gap: 1.5 }}>
            {displayed.map(competitor => {
              const age = calculateAge(competitor.dateOfBirth)
              const dob = formatDOB(competitor.dateOfBirth)
              return (
                <Paper
                  key={competitor.id}
                  variant="outlined"
                  sx={{
                    borderRadius: 2,
                    p: 2,
                    cursor: 'pointer',
                    transition: 'box-shadow 120ms ease, border-color 120ms ease',
                    '&:hover': {
                      boxShadow: 2,
                      borderColor: 'primary.main',
                    },
                    '&:focus-visible': {
                      outline: '2px solid',
                      outlineColor: 'primary.main',
                      outlineOffset: '2px',
                    },
                  }}
                  role="button"
                  tabIndex={0}
                  onClick={() => setSelectedCompetitorID(competitor.id)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      setSelectedCompetitorID(competitor.id)
                    }
                  }}
                >
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
                      {competitor.mostRecentEvent && (
                        <Typography variant="caption" color="text.secondary" noWrap display="block">
                          {competitor.mostRecentEvent}
                        </Typography>
                      )}
                    </Box>
                    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 0.5, flexShrink: 0 }}>
                      <Chip
                        label={isCheckedIn(competitor) ? 'Checked In' : 'Pending'}
                        color={isCheckedIn(competitor) ? 'success' : 'default'}
                        size="small"
                      />
                      {competitor.dobVerifiedAt ? (
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
                    {(isAdmin || !competitor.dobVerifiedAt) && (
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
                  <Typography variant="caption" color="text.secondary" sx={{ mt: 1.5, display: 'block' }}>
                    Tap for details and actions
                  </Typography>
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
                </TableRow>
              </TableHead>
              <TableBody>
                {displayed.map(competitor => {
                  const age = calculateAge(competitor.dateOfBirth)
                  const dob = formatDOB(competitor.dateOfBirth)
                  return (
                    <TableRow
                      key={competitor.id}
                      hover
                      tabIndex={0}
                      role="button"
                      onClick={() => setSelectedCompetitorID(competitor.id)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' || e.key === ' ') {
                          e.preventDefault()
                          setSelectedCompetitorID(competitor.id)
                        }
                      }}
                      sx={{
                        cursor: 'pointer',
                        transition: 'background-color 120ms ease, box-shadow 120ms ease',
                        '&:hover': {
                          bgcolor: 'action.selected',
                        },
                        '&:focus-visible': {
                          outline: '2px solid',
                          outlineColor: 'primary.main',
                          outlineOffset: '-2px',
                        },
                      }}
                    >
                      {vis('name') && (
                        <TableCell>{competitor.nameFirst} {competitor.nameLast}</TableCell>
                      )}
                      {vis('event') && (
                        <TableCell sx={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                          {competitor.mostRecentEvent || '—'}
                        </TableCell>
                      )}
                      {vis('page') && (
                        <TableCell>{competitor.pageNumber || '—'}</TableCell>
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
                          {(isAdmin || !competitor.dobVerifiedAt)
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
                          {competitor.dobVerifiedAt ? (
                            <Tooltip title={`Verified by ${competitor.dobVerifiedBy || 'unknown'}`}>
                              <CheckCircleOutlineIcon fontSize="small" color="success" />
                            </Tooltip>
                          ) : (
                            <Tooltip title="Requires validation">
                              <WarningAmberIcon fontSize="small" color="warning" />
                            </Tooltip>
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
        open={Boolean(filtersAnchor)}
        anchorEl={filtersAnchor}
        onClose={() => setFiltersAnchor(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
        transformOrigin={{ vertical: 'top', horizontal: 'left' }}
      >
        <Box sx={{ p: 2, minWidth: 300, maxWidth: 380, display: 'flex', flexDirection: 'column', gap: 1.5 }}>
          <Typography variant="subtitle2">Filters</Typography>

          {/* Hidden until the sentinel resolves to a real event ID — until then
              there is no honest label for what the table is showing. */}
          {isAdmin && filterEvent !== EVENT_SCOPE_CURRENT && availableEvents.length > 1 && (
            <FormControl size="small" fullWidth>
              <InputLabel>Event</InputLabel>
              <Select
                value={filterEvent}
                label="Event"
                onChange={e => setFilterEvent(e.target.value)}
              >
                <MenuItem value={EVENT_SCOPE_ALL}><em>All</em></MenuItem>
                {availableEvents.map(e => (
                  <MenuItem key={e.id} value={e.id}>{e.name}</MenuItem>
                ))}
              </Select>
            </FormControl>
          )}

          <FormControl size="small" fullWidth>
            <InputLabel>Studio</InputLabel>
            <Select
              value={filterStudio}
              label="Studio"
              onChange={e => setFilterStudio(e.target.value)}
            >
              <MenuItem value=""><em>All</em></MenuItem>
              {uniqueStudios.map(s => (
                <MenuItem key={s} value={s}>{s}</MenuItem>
              ))}
            </Select>
          </FormControl>

          <FormControl size="small" fullWidth>
            <InputLabel>Teacher</InputLabel>
            <Select
              value={filterTeacher}
              label="Teacher"
              onChange={e => setFilterTeacher(e.target.value)}
            >
              <MenuItem value=""><em>All</em></MenuItem>
              {uniqueTeachers.map(t => (
                <MenuItem key={t} value={t}>{t}</MenuItem>
              ))}
            </Select>
          </FormControl>

          <FormControl size="small" fullWidth>
            <InputLabel>Shirt</InputLabel>
            <Select
              value={filterShirt}
              label="Shirt"
              onChange={e => setFilterShirt(e.target.value)}
            >
              <MenuItem value=""><em>All</em></MenuItem>
              {uniqueShirts.map(s => (
                <MenuItem key={s} value={s}>{s}</MenuItem>
              ))}
            </Select>
          </FormControl>

          <FormControl size="small" fullWidth>
            <InputLabel>Validated</InputLabel>
            <Select
              value={filterValidated}
              label="Validated"
              onChange={e => setFilterValidated(e.target.value)}
            >
              <MenuItem value=""><em>All</em></MenuItem>
              <MenuItem value="yes">Validated</MenuItem>
              <MenuItem value="needs">Needs Validation</MenuItem>
            </Select>
          </FormControl>

          <FormControl size="small" fullWidth>
            <InputLabel>Status</InputLabel>
            <Select
              value={filterStatus}
              label="Status"
              onChange={e => setFilterStatus(e.target.value)}
            >
              <MenuItem value=""><em>All</em></MenuItem>
              <MenuItem value="checkedin">Checked In</MenuItem>
              <MenuItem value="pending">Pending</MenuItem>
            </Select>
          </FormControl>

          <Box sx={{ display: 'flex', justifyContent: 'space-between', pt: 0.5 }}>
            <Button size="small" onClick={clearFilters} disabled={!anyFilterActive}>
              Clear
            </Button>
            <Button size="small" onClick={() => setFiltersAnchor(null)}>
              Done
            </Button>
          </Box>
        </Box>
      </Popover>

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

      <CompetitorDetailDialog
        open={!!selectedCompetitor}
        competitor={selectedCompetitor}
        eventScope={filterEvent}
        onClose={() => setSelectedCompetitorID(null)}
        onUpdated={(updated) => {
          const existing = competitors.find(c => c.id === updated.id)
          updateLocalCompetitor({ ...updated, currentCheckIn: updated.currentCheckIn ?? existing?.currentCheckIn })
        }}
      />
    </Box>
  )
}
