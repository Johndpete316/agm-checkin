import { useState, useEffect, useCallback } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Paper from '@mui/material/Paper'
import Grid from '@mui/material/Grid'
import CircularProgress from '@mui/material/CircularProgress'
import Alert from '@mui/material/Alert'
import Divider from '@mui/material/Divider'
import {
  PieChart,
  Pie,
  Cell,
  Tooltip,
  Legend,
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  ResponsiveContainer,
} from 'recharts'
import { getCompetitors } from '../api/competitors'
import { getCurrentEvent } from '../api/events'

const PIE_COLORS = ['#1565C0', '#90A4AE']
const SHIRT_COLORS = { handedOut: '#1565C0', remaining: '#90A4AE' }
const STUDIO_COLOR = '#1565C0'

// Canonical shirt size order
const SHIRT_SIZE_ORDER = ['YXS', 'YS', 'YM', 'YL', 'XS', 'S', 'M', 'L', 'XL', 'XXL', 'XXXL']

function StatCard({ label, value, sub, highlight }) {
  return (
    <Paper
      elevation={0}
      variant="outlined"
      sx={{
        borderRadius: 3,
        p: 2.5,
        flex: 1,
        ...(highlight && { borderColor: 'warning.main', bgcolor: 'warning.50' }),
      }}
    >
      <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, textTransform: 'uppercase', letterSpacing: 0.8 }}>
        {label}
      </Typography>
      <Typography variant="h4" sx={{ fontWeight: 700, mt: 0.5, lineHeight: 1, color: highlight ? 'warning.dark' : 'inherit' }}>
        {value}
      </Typography>
      {sub && (
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          {sub}
        </Typography>
      )}
    </Paper>
  )
}

const CustomTooltip = ({ active, payload, label }) => {
  if (!active || !payload?.length) return null
  return (
    <Paper elevation={3} sx={{ px: 2, py: 1, borderRadius: 2 }}>
      {label && <Typography variant="caption" color="text.secondary">{label}</Typography>}
      {payload.map((p, i) => (
        <Box key={i} sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Box sx={{ width: 10, height: 10, borderRadius: '50%', bgcolor: p.fill || p.color }} />
          <Typography variant="body2" fontWeight={600}>{p.name}:</Typography>
          <Typography variant="body2" color="text.secondary">{p.value}</Typography>
        </Box>
      ))}
    </Paper>
  )
}

export default function StatsPage() {
  const [competitors, setCompetitors] = useState([])
  const [currentEvent, setCurrentEvent] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

  const fetchData = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const [data, event] = await Promise.all([getCompetitors(), getCurrentEvent()])
      setCompetitors(data)
      setCurrentEvent(event)
    } catch (err) {
      if (err.message !== 'unauthorized') setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  // Only count competitors registered for the current event
  const current = competitors.filter(c => c.currentCheckIn !== null && c.currentCheckIn !== undefined)

  const total = current.length
  const checkedIn = current.filter(c => c.currentCheckIn?.checkedIn).length
  const remaining = total - checkedIn
  const pct = total > 0 ? Math.round((checkedIn / total) * 100) : 0
  const validationPending = current.filter(c => c.requiresValidation && !c.validated && !c.currentCheckIn?.checkedIn).length

  const pieData = [
    { name: 'Checked In', value: checkedIn },
    { name: 'Remaining', value: remaining },
  ]

  const checkInsByDay = current
    .filter(c => c.currentCheckIn?.checkedIn && c.currentCheckIn?.checkInDatetime)
    .reduce((acc, c) => {
      const date = new Date(c.currentCheckIn.checkInDatetime).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
      acc[date] = (acc[date] || 0) + 1
      return acc
    }, {})

  const barData = Object.entries(checkInsByDay)
    .map(([date, count]) => ({ date, count }))
    .sort((a, b) => new Date(a.date) - new Date(b.date))

  // T-shirt breakdown: total registered vs handed out (checked in) per size
  const shirtData = (() => {
    const sizes = {}
    current.forEach(c => {
      const size = c.shirtSize || 'N/A'
      if (!sizes[size]) sizes[size] = { total: 0, handedOut: 0 }
      sizes[size].total++
      if (c.currentCheckIn?.checkedIn) sizes[size].handedOut++
    })
    return Object.entries(sizes)
      .map(([size, { total: t, handedOut }]) => ({
        size,
        handedOut,
        remaining: t - handedOut,
        total: t,
      }))
      .sort((a, b) => {
        const ai = SHIRT_SIZE_ORDER.indexOf(a.size)
        const bi = SHIRT_SIZE_ORDER.indexOf(b.size)
        if (ai === -1 && bi === -1) return a.size.localeCompare(b.size)
        if (ai === -1) return 1
        if (bi === -1) return -1
        return ai - bi
      })
  })()

  const totalShirtsOut = shirtData.reduce((s, d) => s + d.handedOut, 0)
  const totalShirtsLeft = shirtData.reduce((s, d) => s + d.remaining, 0)

  // Top studios by registered competitors (current event)
  const studioData = (() => {
    const studios = {}
    current.forEach(c => {
      const name = c.studio?.trim() || 'Unknown'
      studios[name] = (studios[name] || 0) + 1
    })
    return Object.entries(studios)
      .map(([name, count]) => ({ name, count }))
      .sort((a, b) => b.count - a.count)
      .slice(0, 10)
  })()

  return (
    <Box sx={{ mt: 4, maxWidth: 1100, mx: 'auto' }}>
      <Box sx={{ mb: 3 }}>
        <Typography variant="h5">
          Check-In Statistics
        </Typography>
        {currentEvent && (
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
            {currentEvent.name} · {new Date(currentEvent.startDate).toLocaleDateString(undefined, { month: 'long', day: 'numeric', year: 'numeric', timeZone: 'UTC' })}
          </Typography>
        )}
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}

      {loading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', mt: 6 }}>
          <CircularProgress />
        </Box>
      ) : (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
          {/* Summary row */}
          <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap' }}>
            <StatCard label="Total Competitors" value={total} sub="registered for current event" />
            <StatCard label="Checked In" value={checkedIn} sub={`${pct}% of total`} />
            <StatCard label="Remaining" value={remaining} />
            {validationPending > 0 && (
              <StatCard
                label="Validation Pending"
                value={validationPending}
                sub="require ID check before check-in"
                highlight
              />
            )}
          </Box>

          <Grid container spacing={3}>
            {/* Pie chart */}
            <Grid item xs={12} md={5}>
              <Paper variant="outlined" sx={{ borderRadius: 3, p: 3, height: '100%' }}>
                <Typography variant="h6" gutterBottom>
                  Check-In Status
                </Typography>
                <Divider sx={{ mb: 2 }} />
                <ResponsiveContainer width="100%" height={260}>
                  <PieChart>
                    <Pie
                      data={pieData}
                      dataKey="value"
                      nameKey="name"
                      cx="50%"
                      cy="50%"
                      innerRadius={60}
                      outerRadius={100}
                      paddingAngle={3}
                    >
                      {pieData.map((_, i) => (
                        <Cell key={i} fill={PIE_COLORS[i]} />
                      ))}
                    </Pie>
                    <Tooltip content={<CustomTooltip />} />
                    <Legend
                      formatter={(value) => (
                        <Typography component="span" variant="body2">{value}</Typography>
                      )}
                    />
                  </PieChart>
                </ResponsiveContainer>
              </Paper>
            </Grid>

            {/* Bar chart — check-ins by day */}
            <Grid item xs={12} md={7}>
              <Paper variant="outlined" sx={{ borderRadius: 3, p: 3, height: '100%' }}>
                <Typography variant="h6" gutterBottom>
                  Check-Ins by Day
                </Typography>
                <Divider sx={{ mb: 2 }} />
                {barData.length === 0 ? (
                  <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: 260 }}>
                    <Typography color="text.secondary">No check-ins recorded yet.</Typography>
                  </Box>
                ) : (
                  <ResponsiveContainer width="100%" height={260}>
                    <BarChart data={barData} barSize={40}>
                      <CartesianGrid strokeDasharray="4 4" vertical={false} stroke="#E0E0E0" />
                      <XAxis dataKey="date" axisLine={false} tickLine={false} tick={{ fontSize: 13 }} />
                      <YAxis allowDecimals={false} axisLine={false} tickLine={false} tick={{ fontSize: 13 }} width={30} />
                      <Tooltip content={<CustomTooltip />} cursor={{ fill: 'rgba(0,0,0,0.04)' }} />
                      <Bar dataKey="count" name="Check-Ins" fill="#1565C0" radius={[6, 6, 0, 0]} />
                    </BarChart>
                  </ResponsiveContainer>
                )}
              </Paper>
            </Grid>

            {/* T-shirt breakdown */}
            <Grid item xs={12}>
              <Paper variant="outlined" sx={{ borderRadius: 3, p: 3 }}>
                <Box sx={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', flexWrap: 'wrap', gap: 1, mb: 1 }}>
                  <Typography variant="h6">T-Shirt Inventory</Typography>
                  <Box sx={{ display: 'flex', gap: 3 }}>
                    <Box sx={{ textAlign: 'right' }}>
                      <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, textTransform: 'uppercase', letterSpacing: 0.8 }}>Handed Out</Typography>
                      <Typography variant="h6" fontWeight={700} color="primary.main">{totalShirtsOut}</Typography>
                    </Box>
                    <Box sx={{ textAlign: 'right' }}>
                      <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, textTransform: 'uppercase', letterSpacing: 0.8 }}>Remaining</Typography>
                      <Typography variant="h6" fontWeight={700}>{totalShirtsLeft}</Typography>
                    </Box>
                  </Box>
                </Box>
                <Divider sx={{ mb: 2 }} />
                {shirtData.length === 0 ? (
                  <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: 180 }}>
                    <Typography color="text.secondary">No shirt size data available.</Typography>
                  </Box>
                ) : (
                  <ResponsiveContainer width="100%" height={220}>
                    <BarChart data={shirtData} barSize={36}>
                      <CartesianGrid strokeDasharray="4 4" vertical={false} stroke="#E0E0E0" />
                      <XAxis dataKey="size" axisLine={false} tickLine={false} tick={{ fontSize: 13 }} />
                      <YAxis allowDecimals={false} axisLine={false} tickLine={false} tick={{ fontSize: 13 }} width={30} />
                      <Tooltip content={<CustomTooltip />} cursor={{ fill: 'rgba(0,0,0,0.04)' }} />
                      <Legend
                        formatter={(value) => (
                          <Typography component="span" variant="body2">{value}</Typography>
                        )}
                      />
                      <Bar dataKey="handedOut" name="Handed Out" stackId="shirts" fill={SHIRT_COLORS.handedOut} radius={[0, 0, 0, 0]} />
                      <Bar dataKey="remaining" name="Remaining" stackId="shirts" fill={SHIRT_COLORS.remaining} radius={[6, 6, 0, 0]} />
                    </BarChart>
                  </ResponsiveContainer>
                )}
              </Paper>
            </Grid>

            {/* Top studios */}
            {studioData.length > 0 && (
              <Grid item xs={12}>
                <Paper variant="outlined" sx={{ borderRadius: 3, p: 3 }}>
                  <Typography variant="h6" gutterBottom>
                    Top Studios
                  </Typography>
                  <Divider sx={{ mb: 2 }} />
                  <ResponsiveContainer width="100%" height={Math.max(180, studioData.length * 36)}>
                    <BarChart data={studioData} layout="vertical" barSize={22} margin={{ left: 8, right: 24 }}>
                      <CartesianGrid strokeDasharray="4 4" horizontal={false} stroke="#E0E0E0" />
                      <XAxis type="number" allowDecimals={false} axisLine={false} tickLine={false} tick={{ fontSize: 13 }} />
                      <YAxis
                        type="category"
                        dataKey="name"
                        axisLine={false}
                        tickLine={false}
                        tick={{ fontSize: 13 }}
                        width={160}
                      />
                      <Tooltip content={<CustomTooltip />} cursor={{ fill: 'rgba(0,0,0,0.04)' }} />
                      <Bar dataKey="count" name="Competitors" fill={STUDIO_COLOR} radius={[0, 6, 6, 0]} />
                    </BarChart>
                  </ResponsiveContainer>
                </Paper>
              </Grid>
            )}
          </Grid>
        </Box>
      )}
    </Box>
  )
}
