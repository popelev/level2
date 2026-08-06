import { useCallback, useEffect, useState } from 'react'
import { formatBytes, getJSON } from '../api.js'

function formatRate(n) {
  if (n == null || Number.isNaN(Number(n))) return '—'
  const v = Number(n)
  if (v >= 100) return v.toFixed(0)
  if (v >= 10) return v.toFixed(1)
  return v.toFixed(2)
}

function formatETA(seconds) {
  if (seconds == null || Number.isNaN(Number(seconds)) || !Number.isFinite(Number(seconds))) {
    return '—'
  }
  let s = Math.max(0, Number(seconds))
  if (s < 60) return `${Math.round(s)} s`
  const days = Math.floor(s / 86400)
  s -= days * 86400
  const hours = Math.floor(s / 3600)
  s -= hours * 3600
  const mins = Math.floor(s / 60)
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${mins}m`
  return `${mins}m`
}

export default function CapacityPage({ onError }) {
  const [data, setData] = useState(null)

  const load = useCallback(async () => {
    const st = await getJSON('/api/v1/diagnostics/capacity')
    setData(st)
  }, [])

  useEffect(() => {
    onError('')
    load().catch((e) => onError(String(e.message || e)))
    const t = setInterval(() => {
      load().catch(() => {})
    }, 8000)
    return () => clearInterval(t)
  }, [load, onError])

  const mem = data?.collector_memory
  const freeKnown = data?.free_bytes != null
  const growth = data?.growth_bytes_per_sec

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2>Capacity / Storage</h2>
          <p className="muted">
            Disk use of TimescaleDB and estimate of time to fill at the current write rate
          </p>
        </div>
        <button type="button" className="secondary small-btn" onClick={() => load().catch((e) => onError(String(e.message || e)))}>
          Refresh
        </button>
      </div>

      {!data ? (
        <p className="muted">Loading…</p>
      ) : (
        <>
          {data.error && <p className="err">{data.error}</p>}

          <div className="cap-hero">
            <div className="cap-stat">
              <div className="cap-label">DB used</div>
              <div className="cap-value">{formatBytes(data.database_size_bytes)}</div>
            </div>
            <div className="cap-stat">
              <div className="cap-label">Samples table</div>
              <div className="cap-value">{formatBytes(data.samples_size_bytes)}</div>
            </div>
            <div className="cap-stat">
              <div className="cap-label">Free</div>
              <div className="cap-value">{freeKnown ? formatBytes(data.free_bytes) : 'unknown'}</div>
              <div className="cap-sub muted small">{data.free_bytes_source || ''}</div>
            </div>
            <div className="cap-stat accent">
              <div className="cap-label">ETA to full</div>
              <div className="cap-value">{formatETA(data.eta_seconds)}</div>
              <div className="cap-sub muted small">
                {!freeKnown
                  ? 'Set LEVEL2_DB_CAPACITY_BYTES for ETA'
                  : growth > 0
                    ? `~${formatBytes(growth)}/s growth`
                    : 'no recent writes'}
              </div>
            </div>
          </div>

          <section className="panel">
            <h3>Write rate & tags</h3>
            <div className="detail-grid">
              <div>
                <div className="muted small">Monitored tags</div>
                <div>{data.tag_count?.toLocaleString?.() ?? data.tag_count ?? '—'}</div>
              </div>
              <div>
                <div className="muted small">Samples / sec</div>
                <div>{formatRate(data.samples_per_sec)}</div>
                <div className="muted small">last {data.window_seconds || 300}s window</div>
              </div>
              <div>
                <div className="muted small">Samples (last 5 min)</div>
                <div>{(data.samples_last_5min ?? 0).toLocaleString()}</div>
              </div>
              <div>
                <div className="muted small">Avg sample size</div>
                <div>{formatBytes(data.avg_sample_bytes)}</div>
              </div>
              <div>
                <div className="muted small">Approx rows</div>
                <div>{(data.samples_approx_rows ?? 0).toLocaleString()}</div>
              </div>
              <div>
                <div className="muted small">Growth</div>
                <div>{formatBytes(data.growth_bytes_per_sec)}/s</div>
              </div>
            </div>
            {data.capacity_bytes != null && (
              <p className="hint">Configured capacity limit: {formatBytes(data.capacity_bytes)}</p>
            )}
          </section>

          <section className="panel">
            <h3>Collector process memory</h3>
            <div className="detail-grid">
              <div>
                <div className="muted small">Alloc</div>
                <div>{formatBytes(mem?.alloc_bytes)}</div>
              </div>
              <div>
                <div className="muted small">Heap</div>
                <div>{formatBytes(mem?.heap_alloc_bytes)}</div>
              </div>
              <div>
                <div className="muted small">Sys</div>
                <div>{formatBytes(mem?.sys_bytes)}</div>
              </div>
              <div>
                <div className="muted small">GC cycles</div>
                <div>{mem?.num_gc ?? '—'}</div>
              </div>
            </div>
          </section>

          {data.metrics && (
            <section className="panel">
              <h3>Historian counters</h3>
              <div className="diag-metrics">
                <span className="pill">written {Math.round(data.metrics.samples_written_total || 0).toLocaleString()}</span>
                <span className="pill">spooled {Math.round(data.metrics.samples_spooled_total || 0).toLocaleString()}</span>
                <span className={`pill${data.metrics.write_errors_total > 0 ? ' bad' : ''}`}>
                  write errors {Math.round(data.metrics.write_errors_total || 0)}
                </span>
                <span className="pill">spool files {Math.round(data.metrics.spool_depth || 0)}</span>
              </div>
            </section>
          )}
        </>
      )}
    </div>
  )
}
