export async function getJSON(url) {
  const r = await fetch(url)
  if (!r.ok) throw new Error(`${r.status} ${await r.text()}`)
  return r.json()
}

export function formatValue(sample) {
  if (!sample) return '—'
  const num = sample.value_num ?? sample.ValueNum
  const text = sample.value_text ?? sample.ValueText
  const bool = sample.value_bool ?? sample.ValueBool
  if (num != null) return Number(num).toFixed(3)
  if (text != null) return text
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
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleTimeString()
}

export const ROOT_ID = 'ns=0;i=84'

export function guessType(name) {
  const n = String(name || '').toLowerCase()
  if (n.startsWith('b') || n.includes('bool') || n.includes('auto') || n.endsWith('_run')) return 'bool'
  if (n.startsWith('s') && (n.includes('unit') || n.includes('name') || n.includes('text'))) return 'string'
  if (n.startsWith('i') || n.includes('count')) return 'int64'
  return 'float64'
}

export function sanitizeId(s) {
  return String(s || '').trim().replace(/\s+/g, '_')
}
