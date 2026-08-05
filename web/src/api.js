export async function getJSON(url) {
  const r = await fetch(url)
  if (!r.ok) throw new Error(`${r.status} ${await r.text()}`)
  return r.json()
}

export function formatValue(sample) {
  if (!sample) return '—'
  if (sample.value_num != null) return Number(sample.value_num).toFixed(3)
  if (sample.value_text != null) return sample.value_text
  if (sample.value_bool != null) return String(sample.value_bool)
  return '—'
}

export const ROOT_ID = 'ns=0;i=84'

export function guessType(name) {
  const n = String(name || '').toLowerCase()
  if (n.startsWith('b') || n.includes('bool') || n.includes('auto') || n.endsWith('_run')) return 'bool'
  if (n.startsWith('s') && (n.includes('unit') || n.includes('name') || n.includes('text'))) return 'string'
  if (n.startsWith('i') || n.includes('count')) return 'int64'
  return 'float64'
}
