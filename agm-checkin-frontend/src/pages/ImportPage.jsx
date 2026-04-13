import { useRef, useState } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import Paper from '@mui/material/Paper'
import Alert from '@mui/material/Alert'
import AlertTitle from '@mui/material/AlertTitle'
import CircularProgress from '@mui/material/CircularProgress'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableContainer from '@mui/material/TableContainer'
import TableHead from '@mui/material/TableHead'
import TableRow from '@mui/material/TableRow'
import Divider from '@mui/material/Divider'
import UploadFileIcon from '@mui/icons-material/UploadFile'
import { importCompetitors } from '../api/competitors'

const PREVIEW_ROWS = 5

// Minimal CSV row parser that handles quoted fields.
function parseCSVRow(line) {
  const fields = []
  let field = ''
  let inQuotes = false
  for (let i = 0; i < line.length; i++) {
    const ch = line[i]
    if (inQuotes) {
      if (ch === '"' && line[i + 1] === '"') {
        field += '"'
        i++
      } else if (ch === '"') {
        inQuotes = false
      } else {
        field += ch
      }
    } else if (ch === '"') {
      inQuotes = true
    } else if (ch === ',') {
      fields.push(field)
      field = ''
    } else {
      field += ch
    }
  }
  fields.push(field)
  return fields
}

function parseCSVPreview(text) {
  const lines = text.split(/\r?\n/).filter(l => l.trim() !== '')
  if (lines.length === 0) return { headers: [], rows: [], totalRows: 0 }
  const headers = parseCSVRow(lines[0])
  const dataLines = lines.slice(1)
  const rows = dataLines.slice(0, PREVIEW_ROWS).map(parseCSVRow)
  return { headers, rows, totalRows: dataLines.length }
}

export default function ImportPage() {
  const fileInputRef = useRef(null)
  const [file, setFile] = useState(null)
  const [preview, setPreview] = useState(null)
  const [dragOver, setDragOver] = useState(false)
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState(null)
  const [error, setError] = useState(null)

  function handleFile(f) {
    if (!f || !f.name.endsWith('.csv')) {
      setError('Please select a .csv file.')
      return
    }
    setFile(f)
    setResult(null)
    setError(null)
    const reader = new FileReader()
    reader.onload = e => setPreview(parseCSVPreview(e.target.result))
    reader.readAsText(f)
  }

  function handleInputChange(e) {
    handleFile(e.target.files[0])
    e.target.value = ''
  }

  function handleDrop(e) {
    e.preventDefault()
    setDragOver(false)
    handleFile(e.dataTransfer.files[0])
  }

  async function handleImport() {
    if (!file) return
    setLoading(true)
    setError(null)
    try {
      const res = await importCompetitors(file)
      setResult(res)
      setFile(null)
      setPreview(null)
    } catch (e) {
      setError(e.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <Box sx={{ maxWidth: 900, mx: 'auto' }}>
      <Typography variant="h5" fontWeight={700} gutterBottom>
        Import Competitors
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        Upload a normalized competitor CSV file to bulk-import historical data. A database
        snapshot is taken automatically before any changes are made so the import can be
        rolled back if needed.
      </Typography>

      {result && (
        <Alert severity="success" sx={{ mb: 3 }} onClose={() => setResult(null)}>
          <AlertTitle>Import complete</AlertTitle>
          <Box component="ul" sx={{ m: 0, pl: 2 }}>
            <li>{result.competitorsCreated} competitor{result.competitorsCreated !== 1 ? 's' : ''} created</li>
            <li>{result.eventsCreated} stub event{result.eventsCreated !== 1 ? 's' : ''} created</li>
            <li>{result.eventEntriesAdded} event registration{result.eventEntriesAdded !== 1 ? 's' : ''} added</li>
          </Box>
          {result.errors?.length > 0 && (
            <Box sx={{ mt: 1 }}>
              <Typography variant="body2" fontWeight={600}>Warnings:</Typography>
              <Box component="ul" sx={{ m: 0, pl: 2 }}>
                {result.errors.map((e, i) => <li key={i}>{e}</li>)}
              </Box>
            </Box>
          )}
        </Alert>
      )}

      {error && (
        <Alert severity="error" sx={{ mb: 3 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}

      {/* Drop zone */}
      <Paper
        variant="outlined"
        sx={{
          p: 4,
          textAlign: 'center',
          cursor: 'pointer',
          borderStyle: 'dashed',
          borderColor: dragOver ? 'primary.main' : 'divider',
          bgcolor: dragOver ? 'action.hover' : 'background.paper',
          transition: 'border-color 0.15s, background-color 0.15s',
        }}
        onClick={() => fileInputRef.current?.click()}
        onDragOver={e => { e.preventDefault(); setDragOver(true) }}
        onDragLeave={() => setDragOver(false)}
        onDrop={handleDrop}
      >
        <UploadFileIcon sx={{ fontSize: 48, color: 'text.secondary', mb: 1 }} />
        <Typography variant="body1" fontWeight={500}>
          {file ? file.name : 'Click or drag a .csv file here'}
        </Typography>
        {!file && (
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
            Generate this file with: <code>go run ./bin/import *.csv &gt; normalized.csv</code>
          </Typography>
        )}
        <input
          ref={fileInputRef}
          type="file"
          accept=".csv"
          style={{ display: 'none' }}
          onChange={handleInputChange}
        />
      </Paper>

      {/* CSV Preview */}
      {preview && (
        <Box sx={{ mt: 3 }}>
          <Box sx={{ display: 'flex', alignItems: 'baseline', gap: 1, mb: 1 }}>
            <Typography variant="subtitle1" fontWeight={600}>Preview</Typography>
            <Typography variant="body2" color="text.secondary">
              showing {Math.min(PREVIEW_ROWS, preview.totalRows)} of {preview.totalRows} data rows
            </Typography>
          </Box>
          <TableContainer component={Paper} variant="outlined" sx={{ overflowX: 'auto' }}>
            <Table size="small">
              <TableHead>
                <TableRow>
                  {preview.headers.map((h, i) => (
                    <TableCell key={i} sx={{ fontWeight: 600, whiteSpace: 'nowrap' }}>{h}</TableCell>
                  ))}
                </TableRow>
              </TableHead>
              <TableBody>
                {preview.rows.map((row, ri) => (
                  <TableRow key={ri}>
                    {preview.headers.map((_, ci) => (
                      <TableCell key={ci} sx={{ whiteSpace: 'nowrap', maxWidth: 180, overflow: 'hidden', textOverflow: 'ellipsis' }}>
                        {row[ci] ?? ''}
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>

          <Divider sx={{ my: 3 }} />

          <Box sx={{ display: 'flex', gap: 2 }}>
            <Button
              variant="contained"
              onClick={handleImport}
              disabled={loading}
              startIcon={loading ? <CircularProgress size={16} color="inherit" /> : null}
            >
              {loading ? 'Importing…' : `Import ${preview.totalRows} competitors`}
            </Button>
            <Button
              variant="outlined"
              onClick={() => { setFile(null); setPreview(null) }}
              disabled={loading}
            >
              Cancel
            </Button>
          </Box>
        </Box>
      )}
    </Box>
  )
}
