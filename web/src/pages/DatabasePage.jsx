import { useCallback, useEffect, useState } from 'react'
import { formatBytes, getJSON } from '../api.js'

const WIPE_CONFIRM = 'WIPE'

export default function DatabasePage({ onError }) {
  const [data, setData] = useState(null)
  const [wipeConfirm, setWipeConfirm] = useState('')
  const [clearTagsToo, setClearTagsToo] = useState(false)
  const [wipeBusy, setWipeBusy] = useState(false)
  const [wipeMsg, setWipeMsg] = useState('')

  const load = useCallback(async () => {
    const [st, pol] = await Promise.all([
      getJSON('/api/v1/database/status'),
      getJSON('/api/v1/database/capacity-policy').catch(() => ({})),
    ])
    setData({ ...st, ...pol })
  }, [])

  useEffect(() => {
    onError('')
    load().catch((e) => onError(String(e.message || e)))
    const t = setInterval(() => {
      load().catch(() => {})
    }, 5000)
    return () => clearInterval(t)
  }, [load, onError])

  const wipeSamples = async () => {
    if (wipeConfirm !== WIPE_CONFIRM) return
    setWipeBusy(true)
    setWipeMsg('')
    onError('')
    try {
      const r = await fetch('/api/v1/database/wipe-samples?confirm=wipe', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ clear_tags: clearTagsToo }),
      })
      const text = await r.text()
      if (!r.ok) throw new Error(`${r.status} ${text}`)
      let out = {}
      try {
        out = JSON.parse(text)
      } catch {
        /* ignore */
      }
      const parts = [
        `History wiped via ${out.method || 'unknown'}`,
        out.approx_rows_before != null ? `(~${out.approx_rows_before} rows before)` : '',
      ]
      if (out.reseeded != null || out.live_cleared != null) {
        parts.push(`· re-seeded ${out.reseeded ?? 0} (cleared Live ${out.live_cleared ?? 0})`)
      }
      if (out.reseed_error) {
        parts.push(`· reseed warning: ${out.reseed_error}`)
      }
      if (out.clear_tags) {
        parts.push(`· tags removed: ${out.tags_removed ?? 0} on ${out.devices_cleared ?? 0} device(s)`)
      }
      setWipeMsg(parts.filter(Boolean).join(' '))
      setWipeConfirm('')
      setClearTagsToo(false)
      await load()
    } catch (e) {
      onError(String(e.message || e))
    } finally {
      setWipeBusy(false)
    }
  }

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
          <section className="panel">
            <h3>Capacity policy</h3>
            <p className="hint">
              Configure the disk fraction and full-disk action on the{' '}
              <strong>Capacity</strong> page (slider + stop / drop oldest / expand).
            </p>
            <div className="detail-grid">
              <div>
                <div className="muted small">capacity_percent</div>
                <div className="mono">{data.capacity_percent != null ? `${data.capacity_percent}%` : '—'}</div>
              </div>
              <div>
                <div className="muted small">full_policy</div>
                <div className="mono">{data.full_policy || '—'}</div>
              </div>
              <div>
                <div className="muted small">Byte limit</div>
                <div className="mono">{data.limit_bytes != null ? formatBytes(data.limit_bytes) : '—'}</div>
              </div>
            </div>
          </section>

          <section className="panel">
            <h3>Clear history</h3>
            <p className="hint">
              Wipe all historian samples in <span className="mono">collector.samples</span> (TRUNCATE).
              After wipe the collector clears Live and re-seeds Timescale from the last Live snapshot
              so charts refill immediately (Phase 1 on-change suppress otherwise keeps the historian empty).
              Servers stay configured. Per-server tag cleanup without wiping history is on{' '}
              <strong>Projects → Clear tags</strong>.
            </p>
            <label className="row" style={{ gap: 8, alignItems: 'center', marginTop: 10 }}>
              <input
                type="checkbox"
                checked={clearTagsToo}
                onChange={(e) => setClearTagsToo(e.target.checked)}
                disabled={wipeBusy || !ok}
              />
              <span>Also clear all monitored tags (config only)</span>
            </label>
            <div className="row" style={{ marginTop: 12, gap: 10, flexWrap: 'wrap', alignItems: 'center' }}>
              <label className="muted small">
                Type <span className="mono">{WIPE_CONFIRM}</span> to confirm
                <input
                  type="text"
                  className="mono"
                  value={wipeConfirm}
                  onChange={(e) => setWipeConfirm(e.target.value)}
                  placeholder={WIPE_CONFIRM}
                  autoComplete="off"
                  disabled={wipeBusy || !ok}
                  style={{ display: 'block', marginTop: 4, minWidth: 120 }}
                />
              </label>
              <button
                type="button"
                className="secondary danger-btn"
                disabled={wipeBusy || !ok || wipeConfirm !== WIPE_CONFIRM}
                onClick={() => wipeSamples()}
              >
                {wipeBusy ? 'Wiping…' : 'Wipe database samples'}
              </button>
            </div>
            {wipeMsg && <p className="good small" style={{ marginTop: 10 }}>{wipeMsg}</p>}
          </section>
        </>
      )}
    </div>
  )
}
