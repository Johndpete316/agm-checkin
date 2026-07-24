import { useState, useEffect, useCallback, useMemo, useDeferredValue } from 'react'
import Box from '@mui/material/Box'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import CircularProgress from '@mui/material/CircularProgress'
import Alert from '@mui/material/Alert'
import CompetitorCard from '../components/CompetitorCard'
import { getCompetitors, checkInCompetitor } from '../api/competitors'
import { getCurrentEvent } from '../api/events'

export default function CheckInPage() {
  const [search, setSearch] = useState('')
  const [allCompetitors, setAllCompetitors] = useState([])
  const [prefetching, setPrefetching] = useState(true)
  const [error, setError] = useState(null)
  const [checkingIn, setCheckingIn] = useState(null)

  useEffect(() => {
    const prefetch = async () => {
      try {
        // Fire both in parallel; getCurrentEvent warms the cache for other pages.
        const [competitors] = await Promise.all([getCompetitors(), getCurrentEvent()])
        // Current-event competitors first, then alpha by last name.
        competitors.sort((a, b) => {
          const aIn = a.currentCheckIn != null
          const bIn = b.currentCheckIn != null
          if (aIn !== bIn) return bIn ? 1 : -1
          return a.nameLast.localeCompare(b.nameLast)
        })
        setAllCompetitors(competitors)
      } catch (err) {
        setError(err.message)
      } finally {
        setPrefetching(false)
      }
    }
    prefetch()
  }, [])

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
    try {
      const updated = await checkInCompetitor(id)
      setAllCompetitors(prev => prev.map(c => (c.id === id ? { ...c, ...updated } : c)))
    } catch (err) {
      setError(err.message)
    } finally {
      setCheckingIn(null)
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
