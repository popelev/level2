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

export async function postJSON(url, body) {
  const r = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!r.ok) throw new Error(`${r.status} ${await r.text()}`)
  const text = await r.text()
  if (!text) return null
  try {
    return JSON.parse(text)
  } catch {
    return text
  }
}

export async function deleteOK(url) {
  const r = await fetch(url, { method: 'DELETE' })
  if (!r.ok && r.status !== 204) throw new Error(await r.text())
}

/** NDJSON or JSON expand of leaf tags under a browse node. */
export async function expandTags(deviceId, nodeId, parentTagId, onProgress) {
  const r = await fetch('/api/v1/expand', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Accept: 'application/x-ndjson',
    },
    body: JSON.stringify({
      device_id: deviceId,
      node_id: nodeId,
      parent_tag_id: parentTagId || '',
      max_depth: 16,
      stream: true,
    }),
  })
  if (!r.ok) throw new Error(await r.text())
  const ct = r.headers.get('content-type') || ''
  if (!ct.includes('ndjson')) {
    return r.json()
  }
  const reader = r.body.getReader()
  const dec = new TextDecoder()
  let buf = ''
  let tags = null
  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buf += dec.decode(value, { stream: true })
    let nl
    while ((nl = buf.indexOf('\n')) >= 0) {
      const line = buf.slice(0, nl).trim()
      buf = buf.slice(nl + 1)
      if (!line) continue
      let ev
      try {
        ev = JSON.parse(line)
      } catch {
        continue
      }
      if (ev.type === 'progress') {
        onProgress?.(ev)
      } else if (ev.type === 'result') {
        tags = ev.tags || []
      } else if (ev.type === 'error') {
        throw new Error(ev.error || 'expand failed')
      }
    }
  }
  if (!tags) throw new Error('expand returned no result')
  return tags
}

export async function postTagsSimulate(deviceId, body) {
  return postJSON(`/api/v1/devices/${encodeURIComponent(deviceId)}/tags/simulate`, body)
}

export async function postTagsWritable(deviceId, body) {
  return postJSON(`/api/v1/devices/${encodeURIComponent(deviceId)}/tags/writable`, body)
}

export async function postTagsBulk(deviceId, tags) {
  return postJSON(`/api/v1/devices/${encodeURIComponent(deviceId)}/tags/bulk`, { tags })
}

export async function postTagsSync(deviceId, tagIds) {
  const url = `/api/v1/devices/${encodeURIComponent(deviceId)}/tags/sync`
  if (tagIds?.length) {
    return postJSON(url, { tag_ids: tagIds })
  }
  const r = await fetch(url, { method: 'POST' })
  if (!r.ok) throw new Error(await r.text())
  return r.json()
}

export async function upsertDeviceTag(deviceId, body) {
  return postJSON(`/api/v1/devices/${encodeURIComponent(deviceId)}/tags`, body)
}

export async function putDeviceTag(deviceId, tag) {
  const r = await fetch(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/tags/${encodeURIComponent(tag.id)}`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(tag),
    },
  )
  if (!r.ok) throw new Error(await r.text())
  return r.json().catch(() => null)
}

export async function deleteDeviceTag(deviceId, tagId) {
  await deleteOK(
    `/api/v1/devices/${encodeURIComponent(deviceId)}/tags/${encodeURIComponent(tagId)}`,
  )
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

/** Honest UI quality: offline + !simulate → stale (never green good from frozen Live). */
export function displayQuality(sample, { opcConnected, simulate } = {}) {
  if (!sample) return '—'
  if (opcConnected === false && !simulate) return 'stale'
  return formatQuality(sample)
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
  if (!n.includes('timeout') && !n.includes('lifetime') && !n.includes('runtime')) {
    const leaf = n.includes('.') || n.includes('_') ? n.split(/[._]/).pop() : n
    if (
      n.includes('datetime') ||
      n.includes('dateandtime') ||
      n.includes('date_and_time') ||
      n.includes('date_time') ||
      n.includes('timestamp') ||
      leaf === 'time' ||
      n.endsWith('_time')
    ) {
      return 'datetime'
    }
  }
  if (n.startsWith('s') && (n.includes('unit') || n.includes('name') || n.includes('text'))) return 'string'
  if (n.startsWith('i') || n.includes('count')) return 'int64'
  return 'float64'
}

export function sanitizeId(s) {
  return String(s || '').trim().replace(/\s+/g, '_')
}
