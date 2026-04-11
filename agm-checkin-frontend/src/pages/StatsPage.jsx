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

const PIE_COLORS = ['#1565C0', '#90A4AE']

function StatCard({ label, value, sub }) {
  return (
    <Paper elevation={0} variant="outlined" sx={{ borderRadius: 3, p: 2.5, flex: 1 }}>
      <Typography variant="caption" color="text.secondary" sx={{ fontWeight: 600, textTransform: 'uppercase', letterSpacing: 0.8 }}>
        {label}
      </Typography>
      <Typography variant="h4" sx={{ fontWeight: 700, mt: 0.5, lineHeight: 1 }}>
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

const CustomTooltip = ({ active, payload }) => {
  if (!active || !payload?.length) return null
  return (
    <Paper elevation={3} sx={{ px: 2, py: 1, borderRadius: 2 }}>
      <Typography variant="body2" fontWeight={600}>{payload[0].name}</Typography>
      <Typography variant="body2" color="text.secondary">{payload[0].value}</Typography>
    </Paper>
  )
}

export default function StatsPage() {
  const [competitors, setCompetitors] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)

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

  const total = competitors.length
  const checkedIn = competitors.filter(c => c.isCheckedIn).length
  const remaining = total - checkedIn
  const pct = total > 0 ? Math.round((checkedIn / total) * 100) : 0

  const pieData = [
    { name: 'Checked In', value: checkedIn },
    { name: 'Remaining', value: remaining },
  ]

  const checkInsByDay = competitors
    .filter(c => c.isCheckedIn && c.checkInDateTime)
    .reduce((acc, c) => {
      const date = new Date(c.checkInDateTime).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
      acc[date] = (acc[date] || 0) + 1
      return acc
    }, {})

  const barData = Object.entries(checkInsByDay)
    .map(([date, count]) => ({ date, count }))
    .sort((a, b) => new Date(a.date) - new Date(b.date))

  return (
    <Box sx={{ mt: 4, maxWidth: 1100, mx: 'auto' }}>
      <Typography variant="h5" gutterBottom>
        Check-In Statistics
      </Typography>

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
            <StatCard label="Total Competitors" value={total} />
            <StatCard label="Checked In" value={checkedIn} sub={`${pct}% of total`} />
            <StatCard label="Remaining" value={remaining} />
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

            {/* Bar chart */}
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
                      <XAxis
                        dataKey="date"
                        axisLine={false}
                        tickLine={false}
                        tick={{ fontSize: 13 }}
                      />
                      <YAxis
                        allowDecimals={false}
                        axisLine={false}
                        tickLine={false}
                        tick={{ fontSize: 13 }}
                        width={30}
                      />
                      <Tooltip content={<CustomTooltip />} cursor={{ fill: 'rgba(0,0,0,0.04)' }} />
                      <Bar dataKey="count" name="Check-Ins" fill="#1565C0" radius={[6, 6, 0, 0]} />
                    </BarChart>
                  </ResponsiveContainer>
                )}
              </Paper>
            </Grid>
          </Grid>
        </Box>
      )}
    </Box>
  )
}
