import { useState, useEffect, useRef, useCallback, useMemo, useDeferredValue } from 'react'
import Box from '@mui/material/Box'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import CircularProgress from '@mui/material/CircularProgress'
import Alert from '@mui/material/Alert'
import CompetitorCard from '../components/CompetitorCard'
import { getCompetitors, checkInCompetitor } from '../api/competitors'
import { getCurrentEvent } from '../api/events'

// How often to pull a fresh roster while the tab is open. Several desks check
// people in at once, so a list held from open goes stale within minutes.
const REFRESH_INTERVAL_MS = 30_000

// Current-event competitors first, then alpha by last name.
function sortCompetitors(competitors) {
  return [...competitors].sort((a, b) => {
    const aIn = a.currentCheckIn != null
    const bIn = b.currentCheckIn != null
    if (aIn !== bIn) return bIn ? 1 : -1
    return a.nameLast.localeCompare(b.nameLast)
  })
}

export default function CheckInPage() {
  const [search, setSearch] = useState('')
  const [allCompetitors, setAllCompetitors] = useState([])
  const [prefetching, setPrefetching] = useState(true)
  const [error, setError] = useState(null)
  const [checkingIn, setCheckingIn] = useState(null)

  // A refresh landing mid-check-in would momentarily revert the card to Pending,
  // since the server response we merged is newer than the list in flight.
  const checkingInRef = useRef(null)
  const refreshingRef = useRef(false)

  const refresh = useCallback(async () => {
    if (refreshingRef.current || checkingInRef.current) return
    refreshingRef.current = true
    try {
      const competitors = await getCompetitors()
      setAllCompetitors(sortCompetitors(competitors))
    } catch {
      // Background refresh — keep showing the roster we already have rather than
      // surfacing an error over a working screen.
    } finally {
      refreshingRef.current = false
    }
  }, [])

  useEffect(() => {
    const prefetch = async () => {
      try {
        // Fire both in parallel; getCurrentEvent warms the cache for other pages.
        const [competitors] = await Promise.all([getCompetitors(), getCurrentEvent()])
        setAllCompetitors(sortCompetitors(competitors))
      } catch (err) {
        setError(err.message)
      } finally {
        setPrefetching(false)
      }
    }
    prefetch()
  }, [])

  // Other desks' check-ins, and rosters changed by an admin or import, only
  // reach this page through a refetch.
  useEffect(() => {
    const onVisibilityChange = () => {
      if (document.visibilityState === 'visible') refresh()
    }
    document.addEventListener('visibilitychange', onVisibilityChange)
    const timer = setInterval(() => {
      if (document.visibilityState === 'visible') refresh()
    }, REFRESH_INTERVAL_MS)
    return () => {
      document.removeEventListener('visibilitychange', onVisibilityChange)
      clearInterval(timer)
    }
  }, [refresh])

  const deferredSearch = useDeferredValue(search)

  const competitors = useMemo(() => {
    if (!deferredSearch.trim()) return []
    const q = deferredSearch.trim().toLowerCase()
    return allCompetitors.filter(c => {
      const fwd = `${c.nameFirst} ${c.nameLast}`.toLowerCase()
      const rev = `${c.nameLast} ${c.nameFirst}`.toLowerCase()
      return fwd.includes(q) || rev.includes(q)
    })
  }, [deferredSearch, allCompetitors])

  const handleUpdate = useCallback((updated) => {
    setAllCompetitors(prev => prev.map(c => (c.id === updated.id ? { ...c, ...updated } : c)))
  }, [])

  const handleCheckIn = async (id) => {
    setCheckingIn(id)
    checkingInRef.current = id
    try {
      const updated = await checkInCompetitor(id)
      setAllCompetitors(prev => prev.map(c => (c.id === id ? { ...c, ...updated } : c)))
    } catch (err) {
      setError(err.message)
    } finally {
      setCheckingIn(null)
      checkingInRef.current = null
    }
  }

  return (
    <Box sx={{ maxWidth: 680, mx: 'auto', mt: 4 }}>
      <Typography variant="h5" gutterBottom>
        Competitor Check-In
      </Typography>
      <TextField
        fullWidth
        label="Search by name"
        variant="outlined"
        value={search}
        onChange={e => setSearch(e.target.value)}
        sx={{ mb: 3 }}
        autoFocus
        slotProps={{
          input: {
            sx: { fontSize: { xs: '1.15rem', sm: '1rem' } },
            endAdornment: prefetching ? <CircularProgress size={20} sx={{ mr: 1 }} /> : null,
          },
        }}
      />
      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}
      <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
        {competitors.map(competitor => (
          <CompetitorCard
            key={competitor.id}
            competitor={competitor}
            onCheckIn={handleCheckIn}
            onUpdate={handleUpdate}
            loading={checkingIn === competitor.id}
          />
        ))}
        {!prefetching && competitors.length === 0 && deferredSearch.trim() && (
          <Typography color="text.secondary" textAlign="center" sx={{ mt: 2 }}>
            No competitors found for "{deferredSearch}"
          </Typography>
        )}
        {!search.trim() && (
          <Typography color="text.secondary" textAlign="center" sx={{ mt: 2 }}>
            {prefetching ? 'Loading competitor data…' : 'Start typing to search for a competitor'}
          </Typography>
        )}
      </Box>
    </Box>
  )
}
