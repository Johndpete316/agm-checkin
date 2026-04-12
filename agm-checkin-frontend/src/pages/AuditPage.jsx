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
import Chip from '@mui/material/Chip'
import TextField from '@mui/material/TextField'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import Alert from '@mui/material/Alert'
import Skeleton from '@mui/material/Skeleton'
import Tooltip from '@mui/material/Tooltip'
import { listAuditLogs } from '../api/audit'

const ACTION_OPTIONS = [
  { value: '', label: 'All actions' },
  { value: 'competitor.created', label: 'Competitor created' },
  { value: 'competitor.updated', label: 'Competitor updated' },
  { value: 'competitor.deleted', label: 'Competitor deleted' },
  { value: 'competitor.checked_in', label: 'Competitor checked in' },
  { value: 'competitor.dob_updated', label: 'DOB updated' },
  { value: 'competitor.validated', label: 'Competitor validated' },
  { value: 'staff.role_updated', label: 'Staff role updated' },
  { value: 'staff.revoked', label: 'Staff revoked' },
  { value: 'event.created', label: 'Event created' },
  { value: 'event.set_current', label: 'Event set current' },
]

const ACTION_COLORS = {
  'competitor.deleted': 'error',
  'staff.revoked': 'error',
  'staff.role_updated': 'warning',
  'event.set_current': 'warning',
  'competitor.checked_in': 'success',
  'competitor.validated': 'success',
  'competitor.created': 'primary',
  'event.created': 'primary',
  'competitor.updated': 'default',
  'competitor.dob_updated': 'default',
}

function ActionChip({ action }) {
  const color = ACTION_COLORS[action] ?? 'default'
  const label = action.replace('.', ' · ')
  return <Chip label={label} color={color} size="small" variant="outlined" sx={{ fontFamily: 'monospace', fontSize: '0.72rem' }} />
}

function DetailCell({ detail }) {
  if (!detail || typeof detail !== 'object' || Object.keys(detail).length === 0) {
    return <Typography variant="body2" color="text.disabled">—</Typography>
  }
  const entries = Object.entries(detail)
  return (
    <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
      {entries.map(([k, v]) => (
        <Tooltip key={k} title={k} placement="top">
          <Typography
            variant="caption"
            sx={{
              bgcolor: 'action.selected',
              borderRadius: 0.5,
              px: 0.75,
              py: 0.25,
              fontFamily: 'monospace',
              whiteSpace: 'nowrap',
            }}
          >
            {String(v)}
          </Typography>
        </Tooltip>
      ))}
    </Box>
  )
}

export default function AuditPage() {
  const [logs, setLogs] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [actionFilter, setActionFilter] = useState('')
  const [actorFilter, setActorFilter] = useState('')
  const [actorInput, setActorInput] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const data = await listAuditLogs({ action: actionFilter, actor: actorFilter, limit: 200 })
      setLogs(data)
    } catch {
      setError('Failed to load audit log.')
    } finally {
      setLoading(false)
    }
  }, [actionFilter, actorFilter])

  useEffect(() => { load() }, [load])

  // Debounce actor name filter
  useEffect(() => {
    const t = setTimeout(() => setActorFilter(actorInput.trim()), 400)
    return () => clearTimeout(t)
  }, [actorInput])

  return (
    <Box>
      <Typography variant="h5" fontWeight={700} mb={3}>Audit Log</Typography>

      {error && <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError('')}>{error}</Alert>}

      <Box sx={{ display: 'flex', gap: 2, mb: 2, flexWrap: 'wrap' }}>
        <FormControl size="small" sx={{ minWidth: 200 }}>
          <InputLabel>Action</InputLabel>
          <Select value={actionFilter} label="Action" onChange={e => setActionFilter(e.target.value)}>
            {ACTION_OPTIONS.map(o => (
              <MenuItem key={o.value} value={o.value}>{o.label}</MenuItem>
            ))}
          </Select>
        </FormControl>
        <TextField
          size="small"
          label="Actor name"
          value={actorInput}
          onChange={e => setActorInput(e.target.value)}
          placeholder="Search by name…"
          sx={{ minWidth: 200 }}
        />
      </Box>

      <TableContainer component={Paper} variant="outlined">
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell sx={{ whiteSpace: 'nowrap' }}>Time</TableCell>
              <TableCell>Actor</TableCell>
              <TableCell>Action</TableCell>
              <TableCell>Entity</TableCell>
              <TableCell>Detail</TableCell>
              <TableCell>IP</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading
              ? Array.from({ length: 8 }).map((_, i) => (
                  <TableRow key={i}>
                    {Array.from({ length: 6 }).map((_, j) => (
                      <TableCell key={j}><Skeleton /></TableCell>
                    ))}
                  </TableRow>
                ))
              : logs.length === 0
                ? (
                  <TableRow>
                    <TableCell colSpan={6} align="center" sx={{ color: 'text.secondary', py: 3 }}>
                      No entries found
                    </TableCell>
                  </TableRow>
                )
                : logs.map(entry => (
                  <TableRow key={entry.id} hover>
                    <TableCell sx={{ whiteSpace: 'nowrap', color: 'text.secondary', fontSize: '0.78rem' }}>
                      {new Date(entry.createdAt).toLocaleString()}
                    </TableCell>
                    <TableCell sx={{ whiteSpace: 'nowrap' }}>{entry.actorName}</TableCell>
                    <TableCell><ActionChip action={entry.action} /></TableCell>
                    <TableCell>
                      <Typography variant="body2" sx={{ fontWeight: 500 }}>{entry.entityName}</Typography>
                      <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
                        {entry.entityType}
                      </Typography>
                    </TableCell>
                    <TableCell><DetailCell detail={entry.detail} /></TableCell>
                    <TableCell sx={{ fontFamily: 'monospace', fontSize: '0.75rem', color: 'text.secondary' }}>
                      {entry.ipAddress || '—'}
                    </TableCell>
                  </TableRow>
                ))}
          </TableBody>
        </Table>
      </TableContainer>

      {!loading && logs.length > 0 && (
        <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
          Showing {logs.length} most recent entries
        </Typography>
      )}
    </Box>
  )
}
