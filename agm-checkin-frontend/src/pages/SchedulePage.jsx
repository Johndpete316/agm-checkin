import { useCallback, useEffect, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Paper from '@mui/material/Paper'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableHead from '@mui/material/TableHead'
import TableRow from '@mui/material/TableRow'
import TextField from '@mui/material/TextField'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import ToggleButton from '@mui/material/ToggleButton'
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup'
import FormControlLabel from '@mui/material/FormControlLabel'
import Switch from '@mui/material/Switch'
import Chip from '@mui/material/Chip'
import Alert from '@mui/material/Alert'
import Skeleton from '@mui/material/Skeleton'
import Divider from '@mui/material/Divider'
import Tooltip from '@mui/material/Tooltip'
import Collapse from '@mui/material/Collapse'
import ButtonBase from '@mui/material/ButtonBase'
import Button from '@mui/material/Button'
import Backdrop from '@mui/material/Backdrop'
import CircularProgress from '@mui/material/CircularProgress'
import InputAdornment from '@mui/material/InputAdornment'
import IconButton from '@mui/material/IconButton'
import useMediaQuery from '@mui/material/useMediaQuery'
import { useTheme } from '@mui/material/styles'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import ChevronRightIcon from '@mui/icons-material/ChevronRight'
import SearchIcon from '@mui/icons-material/Search'
import ClearIcon from '@mui/icons-material/Clear'
import HistoryIcon from '@mui/icons-material/History'
import ScheduleIcon from '@mui/icons-material/Schedule'
import NotificationsActiveIcon from '@mui/icons-material/NotificationsActive'
import { getEventSchedule } from '../api/schedule'
import { getCompetitor } from '../api/competitors'
import { listEvents, getCurrentEvent } from '../api/events'
import CompetitorDetailDialog from '../components/CompetitorDetailDialog'

// The section is the user's pivot; inside it each category runs as one
// contiguous, non-overlapping block — a real session staff work through, not
// just a filter. Days divide the sections in between.
const GROUP_OPTIONS = [
  { value: 'room', label: 'Room' },
  { value: 'day', label: 'Day' },
  { value: 'category', label: 'Category' },
]

// What names a block, after the section and day headers have had their say.
const BLOCK_FIELDS = {
  room: ['category'],
  day: ['room', 'category'],
  category: ['room'],
}

// Tagged on a block when the block has exactly one non-blank value for them.
// Deliberately not inherited by day or section headers: a tag means one thing
// only — "this block is all X" — and you never have to look up the page to work
// out where it came from.
const TAG_FIELDS = ['instrument', 'division']

// Each kind of tag gets its own colour, so two chips that look alike never mean
// different things. Instrument describes the session; division describes who is
// in it.
const TAG_COLOR = { instrument: 'secondary', division: 'default' }

const SOON_WINDOW_MIN = 30
const UNASSIGNED = 'Unassigned'

// scheduleDate is a Postgres date column, so it arrives as UTC midnight. Read
// the calendar day off it in UTC, then place sortOrder (minutes since midnight)
// on the local clock — slot times are venue-local wall times, never UTC.
function entryStart(entry) {
  if (!entry?.scheduleDate) return null
  const d = new Date(entry.scheduleDate)
  if (isNaN(d.getTime())) return null
  const start = new Date(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate())
  start.setMinutes(entry.sortOrder || 0)
  return start
}

function entryStatus(entry, now) {
  const start = entryStart(entry)
  if (!start) return 'upcoming'
  const diffMin = (start.getTime() - now.getTime()) / 60000
  if (diffMin < 0) return 'past'
  if (diffMin <= SOON_WINDOW_MIN) return 'soon'
  return 'upcoming'
}

function formatDay(dateStr, opts = { weekday: 'short', month: 'short', day: 'numeric' }) {
  if (!dateStr) return ''
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return ''
  return d.toLocaleDateString(undefined, { ...opts, timeZone: 'UTC' })
}

function dayKey(dateStr) {
  if (!dateStr) return ''
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return ''
  return d.toISOString().slice(0, 10)
}

// `key` buckets rows — blanks need a stable bucket, hence UNASSIGNED. `label` is
// what a human sees, and stays empty for blanks so callers can omit them rather
// than print a placeholder the database never meant as a value.
const FIELDS = {
  day: {
    name: 'Day',
    key: e => dayKey(e.scheduleDate),
    label: (e, opts) => formatDay(e.scheduleDate, opts),
    longLabel: e => formatDay(e.scheduleDate, { weekday: 'long', month: 'long', day: 'numeric' }),
  },
  room: { name: 'Room', key: e => e.room?.trim() || UNASSIGNED, label: e => e.room?.trim() || '' },
  instrument: { name: 'Instrument', key: e => e.instrument?.trim() || UNASSIGNED, label: e => e.instrument?.trim() || '' },
  category: { name: 'Category', key: e => e.category?.trim() || UNASSIGNED, label: e => e.category?.trim() || '' },
  division: { name: 'Division', key: e => e.division?.trim() || UNASSIGNED, label: e => e.division?.trim() || '' },
}

function distinct(rows, field) {
  return new Set(rows.map(FIELDS[field].key))
}

// Which of `fields` hold one single non-blank value across every row given.
function constantFields(rows, fields) {
  return fields.filter(f =>
    distinct(rows, f).size === 1 && Boolean(FIELDS[f].label(rows[0])))
}

function tagsFor(rows, fields) {
  return fields.map(f => ({ field: f, label: FIELDS[f].label(rows[0]) }))
}

function byTime(a, b) {
  return (a.scheduleDate || '').localeCompare(b.scheduleDate || '') ||
    (a.sortOrder || 0) - (b.sortOrder || 0) ||
    (a.nameLast || '').localeCompare(b.nameLast || '')
}

function StatusIcon({ status }) {
  if (status === 'past') {
    return (
      <Tooltip title="Already passed" placement="right">
        <HistoryIcon fontSize="small" sx={{ color: 'text.disabled', display: 'block' }} />
      </Tooltip>
    )
  }
  if (status === 'soon') {
    return (
      <Tooltip title={`Starts within ${SOON_WINDOW_MIN} minutes`} placement="right">
        <NotificationsActiveIcon fontSize="small" color="warning" sx={{ display: 'block' }} />
      </Tooltip>
    )
  }
  return (
    <Tooltip title="Upcoming" placement="right">
      <ScheduleIcon fontSize="small" sx={{ color: 'primary.main', display: 'block' }} />
    </Tooltip>
  )
}

function Tag({ field, label }) {
  return (
    <Chip
      size="small"
      variant="outlined"
      color={TAG_COLOR[field] ?? 'default'}
      label={label}
      sx={{ height: 19, fontSize: '0.66rem', '& .MuiChip-label': { px: 0.75 } }}
    />
  )
}

function TagRow({ tags }) {
  return tags.map(t => <Tag key={`${t.field}:${t.label}`} field={t.field} label={t.label} />)
}

export default function SchedulePage() {
  const theme = useTheme()
  const isMobile = useMediaQuery(theme.breakpoints.down('md'))

  const [events, setEvents] = useState([])
  const [eventId, setEventId] = useState('')
  const [entries, setEntries] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [groupBy, setGroupBy] = useState('room')
  const [search, setSearch] = useState('')
  const [hidePast, setHidePast] = useState(false)
  const [now, setNow] = useState(() => new Date())
  // Explicit user toggles only; everything else falls back to a computed default.
  const [overrides, setOverrides] = useState({})

  // A schedule row carries only the competitor's id and name, so opening the
  // detail dialog means fetching the full record first.
  const [selectedId, setSelectedId] = useState(null)
  const [selectedCompetitor, setSelectedCompetitor] = useState(null)
  const [detailLoading, setDetailLoading] = useState(false)

  useEffect(() => {
    const t = setInterval(() => setNow(new Date()), 30000)
    return () => clearInterval(t)
  }, [])

  useEffect(() => {
    let cancelled = false
    Promise.all([listEvents(), getCurrentEvent().catch(() => null)])
      .then(([list, current]) => {
        if (cancelled) return
        setEvents(list || [])
        setEventId(current?.id || list?.[0]?.id || '')
      })
      .catch(() => { if (!cancelled) setError('Failed to load events.') })
    return () => { cancelled = true }
  }, [])

  useEffect(() => {
    if (!eventId) return
    let cancelled = false
    setLoading(true)
    setError('')
    getEventSchedule(eventId)
      .then(data => { if (!cancelled) setEntries(data || []) })
      .catch(() => { if (!cancelled) setError('Failed to load schedule.') })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [eventId])

  // Changing the pivot invalidates every key, so drop stale toggles.
  useEffect(() => { setOverrides({}) }, [groupBy, eventId])

  useEffect(() => {
    if (!selectedId) return
    let cancelled = false
    setDetailLoading(true)
    getCompetitor(selectedId)
      .then(data => { if (!cancelled) setSelectedCompetitor(data) })
      .catch(() => {
        if (cancelled) return
        setError('Failed to load competitor.')
        setSelectedId(null)
      })
      .finally(() => { if (!cancelled) setDetailLoading(false) })
    return () => { cancelled = true }
  }, [selectedId])

  function closeDetail() {
    setSelectedId(null)
    setSelectedCompetitor(null)
  }

  // Props shared by every clickable slot row, so mouse and keyboard behave the
  // same here as they do on the competitors table.
  const rowProps = useCallback(competitorId => ({
    tabIndex: 0,
    role: 'button',
    onClick: () => setSelectedId(competitorId),
    onKeyDown: e => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault()
        setSelectedId(competitorId)
      }
    },
  }), [])

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    return entries.filter(e => {
      if (hidePast && entryStatus(e, now) === 'past') return false
      if (!q) return true
      return [
        e.nameFirst, e.nameLast, `${e.nameFirst} ${e.nameLast}`,
        e.room, e.instrument, e.category, e.division, e.pageNumber,
      ].some(v => (v || '').toLowerCase().includes(q))
    })
  }, [entries, search, hidePast, now])

  const sections = useMemo(() => {
    const blockFields = BLOCK_FIELDS[groupBy]
    // Grouping by day already puts the date on the section header.
    const splitByDay = groupBy !== 'day'
    const top = new Map()

    for (const entry of filtered) {
      const topKey = FIELDS[groupBy].key(entry)
      if (!top.has(topKey)) top.set(topKey, [])
      top.get(topKey).push(entry)
    }

    const sortKeys = (a, b) => {
      if (a === UNASSIGNED) return 1
      if (b === UNASSIGNED) return -1
      return groupBy === 'day'
        ? a.localeCompare(b)
        : a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' })
    }

    const buildBlocks = (rows, keyPrefix) => {
      const map = new Map()
      for (const entry of rows) {
        const k = blockFields.map(f => FIELDS[f].key(entry)).join('|')
        if (!map.has(k)) map.set(k, [])
        map.get(k).push(entry)
      }
      return [...map.entries()]
        .map(([k, blockRows]) => {
          const sorted = [...blockRows].sort(byTime)
          const first = sorted[0]
          const last = sorted[sorted.length - 1]
          return {
            key: `${keyPrefix}|${k}`,
            headline: blockFields.map(f => FIELDS[f].label(first)).filter(Boolean).join(' · '),
            timeRange: first.scheduleTime === last.scheduleTime
              ? first.scheduleTime
              : `${first.scheduleTime} – ${last.scheduleTime}`,
            tags: tagsFor(sorted, constantFields(sorted, TAG_FIELDS)),
            rows: sorted,
            remaining: sorted.filter(e => entryStatus(e, now) !== 'past').length,
            // Fields still varying inside the block, and named by no header,
            // have to stay as columns.
            varying: ['day', 'room', 'instrument', 'category', 'division'].filter(f =>
              f !== groupBy && !blockFields.includes(f) &&
              !(splitByDay && f === 'day') && distinct(sorted, f).size > 1),
          }
        })
        .sort((a, b) => byTime(a.rows[0], b.rows[0]))
    }

    return [...top.entries()]
      .sort((a, b) => sortKeys(a[0], b[0]))
      .map(([topKey, rows]) => {
        let runs
        if (splitByDay) {
          const byDay = new Map()
          for (const entry of rows) {
            const k = FIELDS.day.key(entry)
            if (!byDay.has(k)) byDay.set(k, [])
            byDay.get(k).push(entry)
          }
          runs = [...byDay.entries()]
            .sort((a, b) => a[0].localeCompare(b[0]))
            .map(([k, dayRows]) => ({
              key: `${topKey}|${k}`,
              label: FIELDS.day.label(dayRows[0], { weekday: 'long', month: 'short', day: 'numeric' }),
              blocks: buildBlocks(dayRows, `${topKey}|${k}`),
              remaining: dayRows.filter(e => entryStatus(e, now) !== 'past').length,
            }))
        } else {
          runs = [{ key: topKey, label: '', blocks: buildBlocks(rows, topKey), remaining: 1 }]
        }

        const label = groupBy === 'day'
          ? FIELDS.day.longLabel(rows[0])
          : (FIELDS[groupBy].label(rows[0]) || UNASSIGNED)

        return {
          key: topKey,
          label,
          runs,
          count: rows.length,
          remaining: rows.filter(e => entryStatus(e, now) !== 'past').length,
        }
      })
  }, [filtered, groupBy, now])

  const stats = useMemo(() => {
    let past = 0
    let soon = 0
    let next = null
    for (const e of entries) {
      const status = entryStatus(e, now)
      if (status === 'past') past++
      if (status === 'soon') soon++
      if (status !== 'past') {
        const start = entryStart(e)
        if (start && (!next || start < next.start)) next = { start, entry: e }
      }
    }
    return { total: entries.length, past, soon, remaining: entries.length - past, next }
  }, [entries, now])

  // The block staff care about: the earliest one not yet finished. It is the
  // only block open on arrival, so the page lands on a list of sessions rather
  // than every slot in the event at once.
  const focusKey = useMemo(() => {
    for (const section of sections) {
      for (const run of section.runs) {
        for (const block of run.blocks) {
          if (block.remaining > 0) return block.key
        }
      }
    }
    return null
  }, [sections])

  const searching = search.trim().length > 0
  const isOpen = useCallback((key, dflt) => overrides[key] ?? dflt, [overrides])
  const toggle = useCallback((key, dflt) =>
    setOverrides(prev => ({ ...prev, [key]: !(prev[key] ?? dflt) })), [])

  function setAll(open) {
    const next = {}
    for (const section of sections) {
      next[section.key] = open
      for (const run of section.runs) {
        next[run.key] = open
        for (const block of run.blocks) next[block.key] = open
      }
    }
    setOverrides(next)
  }

  return (
    <Box>
      <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 1.5, flexWrap: 'wrap', mb: 2 }}>
        <Typography variant="h5" fontWeight={700}>Schedule</Typography>
        <Typography variant="caption" color="text.secondary">
          as of {now.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })}
        </Typography>
      </Box>

      {error && <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError('')}>{error}</Alert>}

      <Paper variant="outlined" sx={{ p: { xs: 1.5, md: 2 }, mb: 2 }}>
        <Box sx={{ display: 'flex', gap: { xs: 1, md: 2 }, flexWrap: 'wrap', alignItems: 'center' }}>
          <FormControl size="small" sx={{ minWidth: 150, flexGrow: { xs: 1, md: 0 } }}>
            <InputLabel>Event</InputLabel>
            <Select value={eventId} label="Event" onChange={e => setEventId(e.target.value)}>
              {events.map(ev => (
                <MenuItem key={ev.id} value={ev.id}>
                  {ev.name}{ev.isCurrent ? ' — current' : ''}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          <ToggleButtonGroup
            size="small"
            exclusive
            value={groupBy}
            onChange={(_, v) => v && setGroupBy(v)}
            sx={{ flexGrow: { xs: 1, md: 0 } }}
          >
            {GROUP_OPTIONS.map(o => (
              <ToggleButton key={o.value} value={o.value} sx={{ px: 1.5, textTransform: 'none', flexGrow: { xs: 1, md: 0 } }}>
                {o.label}
              </ToggleButton>
            ))}
          </ToggleButtonGroup>

          <TextField
            size="small"
            placeholder="Search name, room, category…"
            value={search}
            onChange={e => setSearch(e.target.value)}
            sx={{ minWidth: 200, flexGrow: 1, maxWidth: { md: 320 } }}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start"><SearchIcon fontSize="small" /></InputAdornment>
              ),
              endAdornment: search ? (
                <InputAdornment position="end">
                  <IconButton size="small" onClick={() => setSearch('')}><ClearIcon fontSize="small" /></IconButton>
                </InputAdornment>
              ) : null,
            }}
          />

          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, ml: { md: 'auto' } }}>
            <FormControlLabel
              control={<Switch size="small" checked={hidePast} onChange={e => setHidePast(e.target.checked)} />}
              label={<Typography variant="body2">Hide past</Typography>}
              sx={{ mr: 0.5 }}
            />
            <Button size="small" onClick={() => setAll(true)}>Expand</Button>
            <Button size="small" onClick={() => setAll(false)}>Collapse</Button>
          </Box>
        </Box>

        <Divider sx={{ my: 1.5 }} />

        <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap', alignItems: 'center' }}>
          <Chip size="small" variant="outlined" label={`${stats.total} slots`} />
          {stats.past > 0 && (
            <Chip size="small" variant="outlined" icon={<HistoryIcon />} label={`${stats.past} passed`} />
          )}
          {stats.past > 0 && (
            <Chip size="small" variant="outlined" color="primary" icon={<ScheduleIcon />} label={`${stats.remaining} remaining`} />
          )}
          {stats.soon > 0 && (
            <Chip size="small" color="warning" icon={<NotificationsActiveIcon />} label={`${stats.soon} starting soon`} />
          )}
          {stats.next && (
            <Typography variant="caption" color="text.secondary" sx={{ ml: { sm: 'auto' } }}>
              Next: {formatDay(stats.next.entry.scheduleDate)} {stats.next.entry.scheduleTime}
              {stats.next.entry.room ? ` · ${stats.next.entry.room}` : ''}
            </Typography>
          )}
        </Box>
      </Paper>

      {loading ? (
        Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} variant="rounded" height={110} sx={{ mb: 1.5 }} />
        ))
      ) : sections.length === 0 ? (
        <Paper variant="outlined" sx={{ p: 4, textAlign: 'center' }}>
          <Typography color="text.secondary">
            {entries.length === 0
              ? 'No schedule entries for this event yet.'
              : 'No slots match the current filters.'}
          </Typography>
        </Paper>
      ) : (
        sections.map(section => {
          const sectionDefault = searching || section.remaining > 0
          const sectionOpen = isOpen(section.key, sectionDefault)
          return (
            <Paper key={section.key} variant="outlined" sx={{ mb: 1.5, overflow: 'hidden' }}>
              <ButtonBase
                onClick={() => toggle(section.key, sectionDefault)}
                sx={{
                  width: '100%', px: 1, py: 1.25, justifyContent: 'flex-start',
                  bgcolor: 'action.hover',
                }}
              >
                {sectionOpen ? <ExpandMoreIcon /> : <ChevronRightIcon />}
                <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 1.25, flexWrap: 'wrap', ml: 0.5, textAlign: 'left' }}>
                  <Typography variant="subtitle1" fontWeight={700}>{section.label}</Typography>
                  <Typography variant="caption" color="text.secondary">
                    {section.remaining === 0
                      ? `${section.count} slots · all passed`
                      : section.remaining < section.count
                        ? `${section.remaining} of ${section.count} left`
                        : `${section.count} slots`}
                  </Typography>
                </Box>
              </ButtonBase>

              <Collapse in={sectionOpen} unmountOnExit>
                {section.runs.map(run => {
                  const runDefault = searching || run.remaining > 0
                  const runOpen = run.label ? isOpen(run.key, runDefault) : true
                  return (
                  <Box key={run.key}>
                    {run.label && (
                      // No rule and no fill — the day is set apart by colour and
                      // the space above it. Lines and bands are what made this
                      // read as just another row.
                      <ButtonBase
                        onClick={() => toggle(run.key, runDefault)}
                        sx={{
                          width: '100%', justifyContent: 'flex-start', gap: 0.25,
                          px: 0.75, pt: 2, pb: 0.75,
                          color: 'primary.main',
                          opacity: run.remaining === 0 ? 0.5 : 1,
                        }}
                      >
                        {runOpen
                          ? <ExpandMoreIcon sx={{ fontSize: '1.1rem' }} />
                          : <ChevronRightIcon sx={{ fontSize: '1.1rem' }} />}
                        <Typography
                          sx={{
                            fontSize: '0.72rem', fontWeight: 700,
                            textTransform: 'uppercase', letterSpacing: 0.9,
                          }}
                        >
                          {run.label}
                        </Typography>
                      </ButtonBase>
                    )}

                    <Collapse in={runOpen} unmountOnExit>
                    {run.blocks.map(block => {
                      const allPassed = block.remaining === 0
                      const blockDefault = searching || block.key === focusKey
                      const blockOpen = isOpen(block.key, blockDefault)
                      const columns = [
                        { id: 'time', label: 'Time', render: e => e.scheduleTime || '—', time: true },
                        { id: 'competitor', label: 'Competitor', render: e => `${e.nameFirst} ${e.nameLast}`.trim() || '—', nowrap: true },
                        ...block.varying.map(f => ({
                          id: f, label: FIELDS[f].name, render: e => FIELDS[f].label(e) || '—',
                        })),
                        { id: 'page', label: 'Page', render: e => e.pageNumber || '—', nowrap: true },
                      ]
                      return (
                        <Box
                          key={block.key}
                          sx={{
                            '&:not(:first-of-type)': { borderTop: '1px solid', borderColor: 'divider' },
                          }}
                        >
                          <ButtonBase
                            onClick={() => toggle(block.key, blockDefault)}
                            sx={{
                              width: '100%', px: 1, py: 1.3, justifyContent: 'flex-start',
                              alignItems: 'flex-start',
                              opacity: allPassed ? 0.55 : 1,
                              '&:hover': { bgcolor: 'table.hover' },
                            }}
                          >
                            {blockOpen
                              ? <ExpandMoreIcon fontSize="small" sx={{ color: 'text.disabled', mt: '1px' }} />
                              : <ChevronRightIcon fontSize="small" sx={{ color: 'text.disabled', mt: '1px' }} />}
                            <Box sx={{
                              display: 'grid',
                              gridTemplateColumns: { xs: '1fr', md: '11.5rem 1fr' },
                              gap: { xs: 0.15, md: 1 },
                              alignItems: 'baseline',
                              ml: 0.5, minWidth: 0, textAlign: 'left', width: '100%',
                            }}>
                              {/* Fixed-width column so every start time lines up. */}
                              <Typography
                                variant="body2"
                                fontWeight={700}
                                sx={{ fontVariantNumeric: 'tabular-nums', whiteSpace: 'nowrap' }}
                              >
                                {block.timeRange}
                              </Typography>
                              <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 0.75, flexWrap: 'wrap', minWidth: 0 }}>
                                <Typography variant="body2">{block.headline}</Typography>
                                <TagRow tags={block.tags} />
                              </Box>
                            </Box>
                          </ButtonBase>

                          <Collapse in={blockOpen} unmountOnExit>
                            {isMobile ? (
                              <Box sx={{ pb: 0.5 }}>
                                {block.rows.map(entry => {
                                  const status = entryStatus(entry, now)
                                  return (
                                    <Box
                                      key={entry.id}
                                      {...rowProps(entry.competitorId)}
                                      sx={{
                                        display: 'grid',
                                        gridTemplateColumns: '1.5rem 5.5rem 1fr',
                                        alignItems: 'baseline',
                                        gap: 1, pl: 1.5, pr: 1.5, py: 1.15,
                                        cursor: 'pointer',
                                        // The table rows get their dividers from the
                                        // theme; this grid has to earn them by hand.
                                        borderTop: '1px solid',
                                        borderColor: 'divider',
                                        opacity: status === 'past' ? 0.45 : 1,
                                        ...(status === 'soon' && {
                                          bgcolor: 'table.hover',
                                          borderLeft: '3px solid',
                                          borderLeftColor: 'warning.main',
                                          pl: 'calc(12px - 3px)',
                                        }),
                                        '&:focus-visible': {
                                          outline: '2px solid',
                                          outlineColor: 'primary.main',
                                          outlineOffset: '-2px',
                                        },
                                      }}
                                    >
                                      <Box sx={{ alignSelf: 'center' }}><StatusIcon status={status} /></Box>
                                      <Typography variant="body2" sx={{ fontVariantNumeric: 'tabular-nums', fontWeight: 600, whiteSpace: 'nowrap' }}>
                                        {entry.scheduleTime}
                                      </Typography>
                                      <Box sx={{ minWidth: 0 }}>
                                        <Typography variant="body2">
                                          {`${entry.nameFirst} ${entry.nameLast}`.trim()}
                                        </Typography>
                                        {(block.varying.length > 0 || entry.pageNumber) && (
                                          <Typography variant="caption" color="text.secondary">
                                            {[...block.varying.map(f => FIELDS[f].label(entry)),
                                              entry.pageNumber ? `p.${entry.pageNumber}` : null]
                                              .filter(Boolean).join(' · ')}
                                          </Typography>
                                        )}
                                      </Box>
                                    </Box>
                                  )
                                })}
                              </Box>
                            ) : (
                              <Box sx={{ overflowX: 'auto', pb: 0.75 }}>
                                <Table size="small">
                                  <TableHead>
                                    <TableRow>
                                      <TableCell sx={{ width: 44 }} />
                                      {columns.map(c => (
                                        <TableCell
                                          key={c.id}
                                          sx={{
                                            color: 'text.secondary',
                                            width: c.time ? '7.5rem' : undefined,
                                          }}
                                        >
                                          {c.label}
                                        </TableCell>
                                      ))}
                                    </TableRow>
                                  </TableHead>
                                  <TableBody>
                                    {block.rows.map(entry => {
                                      const status = entryStatus(entry, now)
                                      return (
                                        <TableRow
                                          key={entry.id}
                                          hover
                                          {...rowProps(entry.competitorId)}
                                          sx={{
                                            cursor: 'pointer',
                                            opacity: status === 'past' ? 0.45 : 1,
                                            ...(status === 'soon' && {
                                              borderLeft: '3px solid',
                                              borderLeftColor: 'warning.main',
                                            }),
                                            '&:focus-visible': {
                                              outline: '2px solid',
                                              outlineColor: 'primary.main',
                                              outlineOffset: '-2px',
                                            },
                                          }}
                                        >
                                          <TableCell sx={{ pl: 2, pr: 0, width: 44 }}>
                                            <StatusIcon status={status} />
                                          </TableCell>
                                          {columns.map(c => (
                                            <TableCell
                                              key={c.id}
                                              sx={{
                                                fontSize: '0.8rem',
                                                whiteSpace: c.nowrap || c.time ? 'nowrap' : 'normal',
                                                fontWeight: c.time ? 600 : 400,
                                                fontVariantNumeric: c.time ? 'tabular-nums' : 'normal',
                                                width: c.time ? '7.5rem' : undefined,
                                              }}
                                            >
                                              {c.render(entry)}
                                            </TableCell>
                                          ))}
                                        </TableRow>
                                      )
                                    })}
                                  </TableBody>
                                </Table>
                              </Box>
                            )}
                          </Collapse>
                        </Box>
                      )
                    })}
                    </Collapse>
                  </Box>
                  )
                })}
              </Collapse>
            </Paper>
          )
        })
      )}

      {detailLoading && !selectedCompetitor && (
        <Backdrop open sx={{ zIndex: theme => theme.zIndex.modal - 1, color: '#fff' }}>
          <CircularProgress color="inherit" />
        </Backdrop>
      )}

      <CompetitorDetailDialog
        open={!!selectedCompetitor}
        competitor={selectedCompetitor}
        eventScope={eventId}
        onClose={closeDetail}
        onUpdated={updated => {
          setSelectedCompetitor(prev => (prev && prev.id === updated.id ? { ...prev, ...updated } : prev))
          // A rename has to show up in the rows behind the dialog.
          setEntries(prev => prev.map(e => e.competitorId === updated.id
            ? { ...e, nameFirst: updated.nameFirst, nameLast: updated.nameLast }
            : e))
        }}
      />
    </Box>
  )
}
