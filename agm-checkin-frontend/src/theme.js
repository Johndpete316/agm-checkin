import { createTheme } from '@mui/material/styles'

export function buildTheme(mode) {
  return createTheme({
    palette: {
      mode,
      primary: {
        main: '#1565C0',
      },
      secondary: {
        main: '#00897B',
      },
      ...(mode === 'light'
        ? { background: { default: '#F0F4F8' } }
        : { background: { default: '#0e1117', paper: '#1a1f2e' } }),
    },
    typography: {
      fontFamily: 'Montserrat, sans-serif',
      h5: { fontWeight: 600 },
      h6: { fontWeight: 600 },
    },
  })
}
