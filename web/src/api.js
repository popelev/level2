export async function getJSON(url) {
  const r = await fetch(url)
  if (!r.ok) throw new Error(`${r.status} ${await r.text()}`)
  return r.json()
}

export async function putJSON(url, body) {
  const r = await fetch(url, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!r.ok) throw new Error(`${r.status} ${await r.text()}`)
  return r.json()
}

export function formatBytes(n) {
  if (n == null || Number.isNaN(Number(n))) return '—'
  const v = Number(n)
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let x = v
  while (x >= 1024 && i < units.length - 1) {
    x /= 1024
    i += 1
  }
  return `${x < 10 && i > 0 ? x.toFixed(2) : x.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

function looksLikeISODateTime(s) {
  return /^\d{4}-\d{2}-\d{2}([T ]\d{2}:\d{2})/.test(String(s))
}

export function formatValue(sample) {
  if (!sample) return '—'
  const num = sample.value_num ?? sample.ValueNum
  const text = sample.value_text ?? sample.ValueText
  const bool = sample.value_bool ?? sample.ValueBool
  if (num != null) return Number(num).toFixed(3)
  if (text != null && text !== '') {
    // Datetime samples store RFC3339 in value_text — show locale datetime.
    if (looksLikeISODateTime(text)) {
      const d = new Date(text)
      if (!Number.isNaN(d.getTime()) && d.getFullYear() >= 1970) {
        return d.toLocaleString()
      }
    }
    return text
  }
  if (bool != null) return String(bool)
  return '—'
}

export function formatQuality(sample) {
  if (!sample) return '—'
  const q = sample.quality ?? sample.Quality
  if (q === 0 || q === '0') return 'good'
  if (q == null) return '—'
  return 'bad'
}

export function formatSampleTime(sample, updatedAt) {
  const t = sample?.time ?? sample?.Time ?? updatedAt
  if (!t) return '—'
  const d = new Date(t)
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 1970) return '—'
  return d.toLocaleTimeString()
}

export const ROOT_ID = 'ns=0;i=84'

export function guessType(name) {
  const n = String(name || '').toLowerCase()
  if (n.startsWith('b') || n.includes('bool') || n.includes('auto') || n.endsWith('_run')) return 'bool'
  if (n.endsWith('_maintenance') || n.endsWith('_operation') || n.includes('harvesting')) return 'bool'
  if (n.includes('_mode_') && !n.includes('rvalue')) return 'bool'
  if (n.endsWith('_active')) return 'bool'
  if (n.includes('datetime') || n.includes('date_time') || (n.endsWith('_time') && !n.includes('runtime'))) return 'datetime'
  if (n.startsWith('s') && (n.includes('unit') || n.includes('name') || n.includes('text'))) return 'string'
  if (n.startsWith('i') || n.includes('count')) return 'int64'
  return 'float64'
}

export function sanitizeId(s) {
  return String(s || '').trim().replace(/\s+/g, '_')
}
