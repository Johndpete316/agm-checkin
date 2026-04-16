import { useState, useEffect, useCallback } from 'react'
import Box from '@mui/material/Box'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import CircularProgress from '@mui/material/CircularProgress'
import Alert from '@mui/material/Alert'
import CompetitorCard from '../components/CompetitorCard'
import { getCompetitors, checkInCompetitor } from '../api/competitors'

export default function CheckInPage() {
  const [search, setSearch] = useState('')
  const [competitors, setCompetitors] = useState([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(null)
  const [checkingIn, setCheckingIn] = useState(null)

  const fetchCompetitors = useCallback(async (query) => {
    setLoading(true)
    setError(null)
    try {
      const data = await getCompetitors(query)
      setCompetitors(data)
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (!search.trim()) {
      setCompetitors([])
      setLoading(false)
      return
    }
    setLoading(true)
    const timeout = setTimeout(() => {
      fetchCompetitors(search.trim())
    }, 150)
    return () => clearTimeout(timeout)
  }, [search, fetchCompetitors])

  const handleUpdate = (updated) => {
    setCompetitors(prev => prev.map(c => (c.id === updated.id ? { ...c, ...updated } : c)))
  }

  const handleCheckIn = async (id) => {
    setCheckingIn(id)
    try {
      const updated = await checkInCompetitor(id)
      setCompetitors(prev => prev.map(c => (c.id === id ? { ...c, ...updated } : c)))
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
            endAdornment: loading ? <CircularProgress size={20} sx={{ mr: 1 }} /> : null,
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
        {!loading && competitors.length === 0 && search.trim() && (
          <Typography color="text.secondary" textAlign="center" sx={{ mt: 2 }}>
            No competitors found for "{search}"
          </Typography>
        )}
        {!search.trim() && (
          <Typography color="text.secondary" textAlign="center" sx={{ mt: 2 }}>
            Start typing to search for a competitor
          </Typography>
        )}
      </Box>
    </Box>
  )
}
