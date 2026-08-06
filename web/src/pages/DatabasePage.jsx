import { useCallback, useEffect, useState } from 'react'
import { getJSON } from '../api.js'

export default function DatabasePage({ onError }) {
  const [data, setData] = useState(null)

  const load = useCallback(async () => {
    const st = await getJSON('/api/v1/database/status')
    setData(st)
  }, [])

  useEffect(() => {
    onError('')
    load().catch((e) => onError(String(e.message || e)))
    const t = setInterval(() => {
      load().catch(() => {})
    }, 5000)
    return () => clearInterval(t)
  }, [load, onError])

  const ok = data?.ready || data?.ping_ok

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2>Database</h2>
          <p className="muted">Timescale / Postgres connection used by the collector historian</p>
        </div>
        <button type="button" className="secondary small-btn" onClick={() => load().catch((e) => onError(String(e.message || e)))}>
          Refresh
        </button>
      </div>

      {!data ? (
        <p className="muted">Loading…</p>
      ) : (
        <>
          <div className="diag-metrics">
            <span className={`pill ${ok ? 'ok' : 'bad'}`}>{ok ? 'connected' : 'disconnected'}</span>
            <span className={`pill ${data.ready ? 'ok' : 'bad'}`}>ready {data.ready ? 'yes' : 'no'}</span>
            {data.ping_error && <span className="pill bad">{data.ping_error}</span>}
          </div>

          <section className="panel">
            <h3>Connection</h3>
            <div className="detail-grid">
              <div>
                <div className="muted small">DATABASE_URL</div>
                <div className="mono">{data.url_masked || '—'}</div>
              </div>
              <div>
                <div className="muted small">Host</div>
                <div className="mono">{data.host || '—'}{data.port ? `:${data.port}` : ''}</div>
              </div>
              <div>
                <div className="muted small">Database</div>
                <div className="mono">{data.database || '—'}</div>
              </div>
              <div>
                <div className="muted small">User</div>
                <div className="mono">{data.user || '—'}</div>
              </div>
              <div>
                <div className="muted small">SSL mode</div>
                <div className="mono">{data.sslmode || '—'}</div>
              </div>
            </div>
            <p className="hint">Password is never returned by the API.</p>
          </section>

          <section className="panel">
            <h3>Server</h3>
            <div className="detail-grid">
              <div>
                <div className="muted small">Postgres</div>
                <div className="mono">{data.server_version || '—'}</div>
              </div>
              <div>
                <div className="muted small">Timescale</div>
                <div className="mono">{data.timescale_version || 'not installed / unknown'}</div>
              </div>
            </div>
          </section>

          <section className="panel">
            <h3>Connection pool</h3>
            <div className="detail-grid">
              <div>
                <div className="muted small">Max</div>
                <div>{data.pool_max_conns ?? '—'}</div>
              </div>
              <div>
                <div className="muted small">Total</div>
                <div>{data.pool_total_conns ?? '—'}</div>
              </div>
              <div>
                <div className="muted small">Idle</div>
                <div>{data.pool_idle_conns ?? '—'}</div>
              </div>
              <div>
                <div className="muted small">Acquired</div>
                <div>{data.pool_acquired_conns ?? '—'}</div>
              </div>
            </div>
          </section>
        </>
      )}
    </div>
  )
}
