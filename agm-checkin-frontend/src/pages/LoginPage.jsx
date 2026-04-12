import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import Box from '@mui/material/Box'
import Paper from '@mui/material/Paper'
import TextField from '@mui/material/TextField'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import Alert from '@mui/material/Alert'
import { useAuth } from '../context/AuthContext'
import { requestToken } from '../api/auth'

export default function LoginPage() {
  const { login } = useAuth()
  const navigate = useNavigate()
  const [step, setStep] = useState(1)
  const [code, setCode] = useState('')
  const [firstName, setFirstName] = useState('')
  const [lastName, setLastName] = useState('')
  const [error, setError] = useState('')
  const [blocked, setBlocked] = useState(false)
  const [loading, setLoading] = useState(false)

  function handleStep1(e) {
    e.preventDefault()
    setError('')
    setStep(2)
  }

  async function handleStep2(e) {
    e.preventDefault()
    setLoading(true)
    setError('')
    try {
      const data = await requestToken(code, firstName, lastName)
      login(data.token, data.firstName, data.lastName, data.role)
      navigate('/home', { replace: true })
    } catch (err) {
      if (err.message === 'blocked') {
        setBlocked(true)
        setError('Access denied.')
      } else if (err.message === 'invalid_auth') {
        setStep(1)
        setCode('')
        setError('Incorrect access code.')
      } else {
        setError('Something went wrong. Please try again.')
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <Box
      sx={{
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        minHeight: '100vh',
        px: 2,
      }}
    >
      <Paper elevation={3} sx={{ p: { xs: 3, sm: 4 }, width: '100%', maxWidth: 400, borderRadius: 3 }}>
        <Typography variant="h5" fontWeight={700} mb={0.5}>
          AGM Check-In
        </Typography>

        {step === 1 ? (
          <>
            <Typography variant="body2" color="text.secondary" mb={3}>
              Enter your access code to continue.
            </Typography>
            <form onSubmit={handleStep1}>
              <TextField
                fullWidth
                label="Access code"
                type="password"
                value={code}
                onChange={e => setCode(e.target.value)}
                disabled={blocked}
                autoFocus
                autoComplete="off"
                sx={{ mb: 2 }}
              />
              {error && (
                <Alert severity="error" sx={{ mb: 2 }}>
                  {error}
                </Alert>
              )}
              <Button
                fullWidth
                variant="contained"
                type="submit"
                disabled={!code || blocked}
              >
                Continue
              </Button>
            </form>
          </>
        ) : (
          <>
            <Typography variant="body2" color="text.secondary" mb={3}>
              Enter your name to complete sign-in.
            </Typography>
            <form onSubmit={handleStep2}>
              <TextField
                fullWidth
                label="First name"
                value={firstName}
                onChange={e => setFirstName(e.target.value)}
                autoFocus
                autoComplete="given-name"
                sx={{ mb: 2 }}
              />
              <TextField
                fullWidth
                label="Last name"
                value={lastName}
                onChange={e => setLastName(e.target.value)}
                autoComplete="family-name"
                sx={{ mb: 2 }}
              />
              {error && (
                <Alert severity="error" sx={{ mb: 2 }}>
                  {error}
                </Alert>
              )}
              <Box sx={{ display: 'flex', gap: 1 }}>
                <Button
                  variant="outlined"
                  onClick={() => { setStep(1); setError('') }}
                  disabled={loading}
                  sx={{ flexShrink: 0 }}
                >
                  Back
                </Button>
                <Button
                  fullWidth
                  variant="contained"
                  type="submit"
                  disabled={!firstName.trim() || !lastName.trim() || loading || blocked}
                >
                  {loading ? 'Signing in…' : 'Sign in'}
                </Button>
              </Box>
            </form>
          </>
        )}
      </Paper>
    </Box>
  )
}
