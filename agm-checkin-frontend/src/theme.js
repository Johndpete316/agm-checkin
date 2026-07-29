import { createTheme } from '@mui/material/styles'

// Row hover lives here rather than on `palette.action` so it stays clearly
// distinct from every other surface tint. Rows are separated by dividers, not by
// alternating fills.
const TABLE_COLORS = {
  light: {
    hover: 'rgba(21, 101, 192, 0.10)',
    selected: 'rgba(21, 101, 192, 0.18)',
  },
  dark: {
    hover: 'rgba(66, 165, 245, 0.18)',
    selected: 'rgba(66, 165, 245, 0.30)',
  },
}

export function buildTheme(mode) {
  const table = TABLE_COLORS[mode] ?? TABLE_COLORS.light

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
      table,
    },
    typography: {
      fontFamily: 'Montserrat, sans-serif',
      h5: { fontWeight: 600 },
      h6: { fontWeight: 600 },
    },
    components: {
      // One row rhythm for every table in the app. Pages should not set their
      // own cell padding — that is how the pages drifted apart in the first
      // place.
      MuiTableCell: {
        styleOverrides: {
          root: {
            paddingTop: 12,
            paddingBottom: 12,
            paddingLeft: 16,
            paddingRight: 16,
            lineHeight: 1.55,
          },
          sizeSmall: {
            paddingTop: 9,
            paddingBottom: 9,
            paddingLeft: 12,
            paddingRight: 12,
          },
          head: {
            fontWeight: 700,
            fontSize: '0.7rem',
            letterSpacing: 0.6,
            textTransform: 'uppercase',
            lineHeight: 1.4,
            whiteSpace: 'nowrap',
          },
        },
      },
      MuiTableRow: {
        styleOverrides: {
          root: {
            // Scoped to tbody so header rows are never striped, and written with
            // enough specificity that hover beats the stripe underneath it.
            'tbody &': {
              transition: 'background-color 120ms ease',
            },
            'tbody &.MuiTableRow-hover:hover': {
              backgroundColor: table.hover,
            },
            'tbody &.Mui-selected, tbody &.Mui-selected:hover': {
              backgroundColor: table.selected,
            },
          },
        },
      },
    },
  })
}
