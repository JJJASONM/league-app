// Shared date formatting utilities used across domain components.
//
// displayDate(raw, fallback) -- format a YYYY-MM-DD or ISO string as "Jul 6, 2026"
// fmtDate(raw, fallback)     -- alias for displayDate
// fmtDateRange(start, end)   -- "Jul 6, 2026 - Aug 10, 2026"

// displayDate formats a YYYY-MM-DD or ISO date string for display.
// Returns fallback when raw is absent or unparseable.
export function displayDate(raw, fallback = 'TBD') {
  if (!raw) return fallback;
  const parts = raw.slice(0, 10).split('-').map(Number);
  if (parts.length !== 3 || parts.some(isNaN)) return fallback;
  const [y, mo, d] = parts;
  const dt = new Date(y, mo - 1, d);
  if (isNaN(dt)) return fallback;
  return dt.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
}

// fmtDate is an alias for displayDate kept for naming consistency across call sites.
export function fmtDate(raw, fallback = 'TBD') {
  return displayDate(raw, fallback);
}

// fmtDateRange formats a start/end pair as "Jul 6, 2026 - Aug 10, 2026".
// Returns "TBD" for any absent or unparseable date.
export function fmtDateRange(start, end) {
  return displayDate(start, 'TBD') + ' - ' + displayDate(end, 'TBD');
}
