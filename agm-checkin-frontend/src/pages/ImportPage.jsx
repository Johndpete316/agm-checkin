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
import Chip from '@mui/material/Chip'
import Accordion from '@mui/material/Accordion'
import AccordionSummary from '@mui/material/AccordionSummary'
import AccordionDetails from '@mui/material/AccordionDetails'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import UploadFileIcon from '@mui/icons-material/UploadFile'
import { importCompetitors, updateCompetitorDOB, getCompetitor, updateCompetitor } from '../api/competitors'

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
  const [conflicts, setConflicts] = useState([]) // unresolved field conflicts
  const [resolvingId, setResolvingId] = useState(null) // competitorId+field key currently being saved

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
    setConflicts([])
    try {
      const res = await importCompetitors(file)
      setResult(res)
      setConflicts(res.fieldConflicts ?? [])
      setFile(null)
      setPreview(null)
    } catch (e) {
      setError(e.message)
    } finally {
      setLoading(false)
    }
  }

  const FIELD_LABELS = {
    email: 'Email',
    studio: 'Studio',
    teacher: 'Teacher',
    shirtSize: 'Shirt Size',
    dateOfBirth: 'Date of Birth',
  }

  async function resolveConflict(conflict, useImport) {
    setResolvingId(conflict.competitorId + conflict.field)
    try {
      if (useImport) {
        if (conflict.field === 'dateOfBirth') {
          await updateCompetitorDOB(conflict.competitorId, conflict.importValue)
        } else {
          // Fetch the full record first so Save doesn't zero out other fields.
          const current = await getCompetitor(conflict.competitorId)
          await updateCompetitor(conflict.competitorId, { ...current, [conflict.field]: conflict.importValue })
        }
      }
      // Whether keeping existing or switching to import, this conflict is resolved.
      setConflicts(prev => prev.filter(
        c => !(c.competitorId === conflict.competitorId && c.field === conflict.field)
      ))
    } catch (e) {
      setError(`Failed to update ${FIELD_LABELS[conflict.field] ?? conflict.field} for ${conflict.name}: ${e.message}`)
    } finally {
      setResolvingId(null)
    }
  }

  return (
    <Box sx={{ maxWidth: 900, mx: 'auto' }}>
      <Typography variant="h5" fontWeight={700} gutterBottom>
        Import Competitors
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Upload a normalized competitor CSV file to bulk-import historical data. A database
        snapshot is taken automatically before any changes are made so the import can be
        rolled back if needed.
      </Typography>

      <Accordion disableGutters variant="outlined" sx={{ mb: 3 }}>
        <AccordionSummary expandIcon={<ExpandMoreIcon />}>
          <Typography variant="body2" fontWeight={600}>File format &amp; merge rules</Typography>
        </AccordionSummary>
        <AccordionDetails>
          <Typography variant="body2" gutterBottom>
            The file must be a <strong>.csv</strong> with a header row containing exactly these
            column names (order does not matter, extra columns are ignored):
          </Typography>
          <TableContainer component={Paper} variant="outlined" sx={{ mb: 2, overflowX: 'auto' }}>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell sx={{ fontWeight: 600 }}>Column</TableCell>
                  <TableCell sx={{ fontWeight: 600 }}>Format</TableCell>
                  <TableCell sx={{ fontWeight: 600 }}>Notes</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {[
                  ['first_name', 'string', 'Required — rows with no name are skipped'],
                  ['last_name', 'string', 'Required — rows with no name are skipped'],
                  ['studio', 'string', 'Leave blank if unknown'],
                  ['teacher', 'string', 'Display name, e.g. "Smith, Jane"'],
                  ['email', 'string', 'Student or parent email only — no teacher addresses'],
                  ['shirt_size', 'string', 'Adult XL / L / M / S or Youth XL / L / M / S'],
                  ['date_of_birth', 'YYYY-MM-DD or blank', 'Leave blank if unknown'],
                  ['requires_validation', 'true / false', 'Whether ID check is needed at check-in'],
                  ['validated', 'true / false', 'Whether ID has already been verified in a prior event'],
                  ['events', 'pipe-separated IDs', 'e.g. nat-2024|glr-2025|glr-2026'],
                ].map(([col, fmt, notes]) => (
                  <TableRow key={col}>
                    <TableCell><code>{col}</code></TableCell>
                    <TableCell sx={{ whiteSpace: 'nowrap' }}>{fmt}</TableCell>
                    <TableCell>{notes}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>

          <Typography variant="body2" fontWeight={600} gutterBottom>Example row</Typography>
          <Box sx={{ overflowX: 'auto', mb: 2 }}>
            <Box component="pre" sx={{ fontSize: '0.72rem', m: 0, p: 1, bgcolor: 'action.hover', borderRadius: 1, whiteSpace: 'pre' }}>
{`first_name,last_name,studio,teacher,email,shirt_size,date_of_birth,requires_validation,validated,events
Jane,Smith,Westside Dance,"Emshwiller, Michael",jane@example.com,Youth M,2012-04-15,true,false,nat-2024|glr-2025|glr-2026`}
            </Box>
          </Box>

          <Typography variant="body2" fontWeight={600} gutterBottom>How merging works</Typography>
          <Typography variant="body2" gutterBottom>
            Competitors are matched by <strong>first name + last name</strong> (case-insensitive).
            If a competitor with the same name already exists in the database:
          </Typography>
          <Box component="ul" sx={{ mt: 0, mb: 1, pl: 3 }}>
            <Box component="li">
              <Typography variant="body2">
                <strong>Auto-fill:</strong> If a field is blank in the database but the import has a
                value, it is filled in automatically. This covers email, studio, teacher, shirt size,
                and date of birth.
              </Typography>
            </Box>
            <Box component="li" sx={{ mt: 0.5 }}>
              <Typography variant="body2">
                <strong>Conflict:</strong> If both the database and the import have a value for the
                same field and they differ, a conflict is raised. You will be asked to choose which
                value to keep after the import runs. Fields that can conflict: email, studio, teacher,
                shirt size, and date of birth.
              </Typography>
            </Box>
            <Box component="li" sx={{ mt: 0.5 }}>
              <Typography variant="body2">
                <strong>Not modified by import:</strong> requires_validation, validated, note,
                last_registered_event, and any check-in records are never overwritten on an existing
                record. Event registrations are added (missing rows only) but existing ones are
                never removed.
              </Typography>
            </Box>
            <Box component="li" sx={{ mt: 0.5 }}>
              <Typography variant="body2">
                <strong>Ambiguous name:</strong> If more than one competitor in the database shares
                the same name, the row is skipped and listed as a warning. Resolve these manually.
              </Typography>
            </Box>
          </Box>
          <Typography variant="body2" color="text.secondary">
            Generate this file from raw event CSVs using:{' '}
            <code>go run ./bin/import *.csv &gt; normalized.csv</code>
          </Typography>
        </AccordionDetails>
      </Accordion>

      {result && (
        <Alert severity="success" sx={{ mb: 3 }} onClose={() => { setResult(null); setConflicts([]) }}>
          <AlertTitle>Import complete</AlertTitle>
          <Box component="ul" sx={{ m: 0, pl: 2 }}>
            <li>{result.competitorsCreated} competitor{result.competitorsCreated !== 1 ? 's' : ''} created</li>
            <li>{result.competitorsMatched} existing competitor{result.competitorsMatched !== 1 ? 's' : ''} matched</li>
            <li>{result.fieldsUpdated} missing field{result.fieldsUpdated !== 1 ? 's' : ''} filled in</li>
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

      {conflicts.length > 0 && (
        <Paper variant="outlined" sx={{ mb: 3, p: 2, borderColor: 'warning.main' }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
            <Typography variant="subtitle1" fontWeight={700}>
              Field Conflicts — Action Required
            </Typography>
            <Chip label={conflicts.length} size="small" color="warning" />
          </Box>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            These competitors have a value in both the database and the import file that differ.
            Choose which value to keep for each one. This list will be lost if you navigate away,
            so resolve them now.
          </Typography>
          <TableContainer>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell sx={{ fontWeight: 600 }}>Name</TableCell>
                  <TableCell sx={{ fontWeight: 600 }}>Field</TableCell>
                  <TableCell sx={{ fontWeight: 600 }}>Existing</TableCell>
                  <TableCell sx={{ fontWeight: 600 }}>Import</TableCell>
                  <TableCell />
                </TableRow>
              </TableHead>
              <TableBody>
                {conflicts.map(conflict => {
                  const key = conflict.competitorId + conflict.field
                  const saving = resolvingId === key
                  return (
                    <TableRow key={key}>
                      <TableCell>{conflict.name}</TableCell>
                      <TableCell>{FIELD_LABELS[conflict.field] ?? conflict.field}</TableCell>
                      <TableCell>{conflict.existingValue}</TableCell>
                      <TableCell>{conflict.importValue}</TableCell>
                      <TableCell align="right">
                        <Box sx={{ display: 'flex', gap: 1, justifyContent: 'flex-end' }}>
                          <Button
                            size="small"
                            variant="outlined"
                            disabled={saving}
                            onClick={() => resolveConflict(conflict, false)}
                          >
                            Keep existing
                          </Button>
                          <Button
                            size="small"
                            variant="contained"
                            disabled={saving}
                            startIcon={saving ? <CircularProgress size={12} color="inherit" /> : null}
                            onClick={() => resolveConflict(conflict, true)}
                          >
                            Use import
                          </Button>
                        </Box>
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </TableContainer>
        </Paper>
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
            Must be a .csv file — see format requirements above
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
