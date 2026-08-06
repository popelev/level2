import { useCallback, useEffect, useState } from 'react'
import { getJSON } from '../api.js'

const CATEGORIES = [
  { id: 'all', label: 'All' },
  { id: 'opc_read', label: 'OPC read' },
  { id: 'db_write', label: 'DB write' },
]

function formatTime(iso) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleString()
}

export default function DiagnosticsPage({ onError }) {
  const [category, setCategory] = useState('all')
  const [errorsOnly, setErrorsOnly] = useState(false)
  const [entries, setEntries] = useState([])
  const [metrics, setMetrics] = useState(null)
  const [paused, setPaused] = useState(false)

  const load = useCallback(async () => {
    const q = new URLSearchParams({
      category,
      limit: '400',
    })
    if (errorsOnly) q.set('errors_only', '1')
    const data = await getJSON(`/api/v1/diagnostics/logs?${q}`)
    setEntries(data.entries || [])
    setMetrics(data.metrics || null)
  }, [category, errorsOnly])

  useEffect(() => {
    onError('')
    load().catch((e) => onError(String(e.message || e)))
  }, [load, onError])

  useEffect(() => {
    if (paused) return undefined
    const t = setInterval(() => {
      load().catch(() => {})
    }, 2000)
    return () => clearInterval(t)
  }, [load, paused])

  const clearLog = async () => {
    onError('')
    try {
      const r = await fetch('/api/v1/diagnostics/logs', { method: 'DELETE' })
      if (!r.ok) throw new Error(await r.text())
      await load()
    } catch (e) {
      onError(String(e.message || e))
    }
  }

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2>Diagnostics</h2>
          <p className="muted">OPC read issues and TimescaleDB write activity (ring buffer, last ~3000 events)</p>
        </div>
      </div>

      {metrics && (
        <div className="diag-metrics">
          <span className="pill">written {Math.round(metrics.samples_written_total || 0).toLocaleString()}</span>
          <span className="pill">spooled {Math.round(metrics.samples_spooled_total || 0).toLocaleString()}</span>
          <span className={`pill${metrics.write_errors_total > 0 ? ' bad' : ''}`}>
            write errors {Math.round(metrics.write_errors_total || 0)}
          </span>
          <span className="pill">spool files {Math.round(metrics.spool_depth || 0)}</span>
        </div>
      )}

      <section className="panel">
        <div className="panel-head diag-toolbar">
          <label className="diag-filter">
            Source
            <select value={category} onChange={(e) => setCategory(e.target.value)}>
              {CATEGORIES.map((c) => (
                <option key={c.id} value={c.id}>{c.label}</option>
              ))}
            </select>
          </label>
          <label className="diag-check">
            <input
              type="checkbox"
              checked={errorsOnly}
              onChange={(e) => setErrorsOnly(e.target.checked)}
            />
            Errors only (warn + error)
          </label>
          <button type="button" className="secondary small-btn" onClick={() => load()}>
            Refresh
          </button>
          <button type="button" className="secondary small-btn" onClick={() => setPaused((p) => !p)}>
            {paused ? 'Resume' : 'Pause'}
          </button>
          <button type="button" className="secondary small-btn" onClick={clearLog}>
            Clear
          </button>
        </div>

        <div className="diag-log-wrap">
          {entries.length === 0 ? (
            <p className="muted">No log entries for this filter yet.</p>
          ) : (
            <table className="diag-log">
              <thead>
                <tr>
                  <th>Time</th>
                  <th>Level</th>
                  <th>Source</th>
                  <th>Message</th>
                  <th>Detail</th>
                </tr>
              </thead>
              <tbody>
                {entries.map((e, i) => (
                  <tr key={`${e.time}-${i}`} className={`lvl-${e.level}`}>
                    <td className="mono small">{formatTime(e.time)}</td>
                    <td>{e.level}</td>
                    <td>{e.category === 'opc_read' ? 'OPC' : 'DB'}</td>
                    <td>
                      {e.message}
                      {e.count > 0 && <span className="muted"> · n={e.count}</span>}
                      {e.device_id && <span className="muted small"> · {e.device_id}</span>}
                      {e.tag_id && <span className="mono small muted"> · {e.tag_id}</span>}
                    </td>
                    <td className="mono small detail">{e.detail || '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </section>
    </div>
  )
}
