import { useCallback, useEffect, useState } from 'react'
import { getJSON, putJSON } from '../api.js'

function formatMs(v) {
  if (v == null || Number.isNaN(Number(v))) return '—'
  const n = Number(v)
  if (n >= 100) return `${Math.round(n)} ms`
  if (n >= 10) return `${n.toFixed(1)} ms`
  return `${n.toFixed(0)} ms`
}

function formatRate(v) {
  if (v == null || Number.isNaN(Number(v))) return '—'
  const n = Number(v)
  if (n <= 0) return '0 /s'
  if (n >= 100) return `${n.toFixed(0)} /s`
  if (n >= 10) return `${n.toFixed(1)} /s`
  return `${n.toFixed(2)} /s`
}

function formatTime(iso) {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleTimeString()
}

function dropsLine(n, label = 'drops') {
  const c = Number(n) || 0
  const cls = c > 0 ? 'badq' : 'muted'
  return (
    <div className={`cap-sub small ${cls}`}>
      {c} {label} / last hour
    </div>
  )
}

export default function OverviewPage({ ready, onError, onNavigate, onStatusRefresh }) {
  const [data, setData] = useState(null)
  const [simBusy, setSimBusy] = useState(false)
  const [showLegacy, setShowLegacy] = useState(false)

  const load = useCallback(async () => {
    const st = await getJSON('/api/v1/status/summary')
    setData(st)
  }, [])

  const toggleTagSim = async (enabled) => {
    if (
      !window.confirm(
        enabled
          ? 'Enable LEGACY global tag_simulation? All enabled tags mocked and OPC collect paused. Prefer per-tag Sim on DB write list. Requires collector recreate.'
          : 'Disable legacy global tag_simulation in config? Requires collector recreate to apply.',
      )
    ) {
      return
    }
    setSimBusy(true)
    onError('')
    try {
      const r = await putJSON('/api/v1/tag-simulation', { enabled })
      if (r.restart_required) {
        onError(r.note || 'Saved — recreate collector to apply tag_simulation.')
      }
      await load()
      if (onStatusRefresh) await onStatusRefresh().catch(() => {})
    } catch (e) {
      onError(String(e.message || e))
    } finally {
      setSimBusy(false)
    }
  }

  useEffect(() => {
    onError('')
    load().catch((e) => onError(String(e.message || e)))
    const t = setInterval(() => {
      load().catch(() => {})
    }, 4000)
    return () => clearInterval(t)
  }, [load, onError])

  const resetAlarms = async () => {
    if (
      !window.confirm(
        'Clear Recent errors and reset last-hour drop counters (collector / OPC / DB)? This does not change live connection state.',
      )
    ) {
      return
    }
    onError('')
    try {
      const r = await fetch('/api/v1/diagnostics/reset', { method: 'POST' })
      if (!r.ok) throw new Error(await r.text())
      await load()
      if (onStatusRefresh) {
        await onStatusRefresh().catch(() => {})
      }
    } catch (e) {
      onError(String(e.message || e))
    }
  }

  // Primary = /readyz (OPC connected, SIM, or legacy global tag simulation)
  const globalSim = !!(data?.tag_simulation || data?.sim_browser)
  const tagsSimulated = data?.tags_simulated ?? 0
  const collectorReady = ready === 'ready' || !!data?.collector_ready
  const processOk = data?.api_ok !== false
  const collectorPrimary = globalSim
    ? (data?.sim_browser ? 'sim browser' : 'sim all')
    : collectorReady
      ? 'ready'
      : (data?.ready_detail || (ready && ready !== '…' ? ready : 'not ready'))
  const disc = (data?.devices_disconnected ?? 0) > 0
  const badTags = (data?.quality_bad ?? 0) > 0
  // Do not paint Tags green while OPC/collector is down — Live may still hold last Good.
  // Global simulation is explicit opt-in: show as sim, not as live PLC health.
  const tagsOffline = !globalSim && (!collectorReady || disc)
  const tagsDegraded = tagsOffline || (!globalSim && badTags)
  const errors = data?.recent_errors || []
  const collectorDrops = data?.collector_down_last_hour ?? 0
  const opcDrops = data?.opc_disconnects_last_hour ?? 0
  const dbDrops = data?.db_write_errors_last_hour ?? 0
  const liveGood = data?.quality_good ?? 0
  const tagsSub = globalSim
    ? 'simulation · not live PLC'
    : tagsOffline
      ? `stale · not connected${badTags ? ` · ${data.quality_bad} bad` : ''}${tagsSimulated ? ` · ${tagsSimulated} sim` : ''}`
      : `${data.quality_bad ?? 0} bad quality${
          data.quality_good_pct != null ? ` · ${Math.round(data.quality_good_pct)}% good` : ''
        }${tagsSimulated ? ` · ${tagsSimulated} sim` : ''}`
  const collectorSub = globalSim
    ? (processOk ? 'process ok · synthetic tags' : 'process down')
    : processOk
      ? 'process ok'
      : 'process down'

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2>Overview</h2>
          <p className="muted">Platform state at a glance</p>
        </div>
        <button
          type="button"
          className="secondary small-btn"
          onClick={() => load().catch((e) => onError(String(e.message || e)))}
        >
          Refresh
        </button>
      </div>

      {!data ? (
        <p className="muted">Loading…</p>
      ) : (
        <>
          <div className="cap-hero">
            <div className={`cap-stat${collectorReady && !globalSim ? ' accent' : ''}`}>
              <div className="cap-label">Collector</div>
              <div
                className={`cap-value ${globalSim ? '' : collectorReady ? 'good' : 'badq'}`}
                style={{ fontSize: '1.1rem' }}
              >
                {collectorPrimary}
              </div>
              <div className={`cap-sub small ${processOk ? 'muted' : 'badq'}`}>
                {collectorSub}
              </div>
              {dropsLine(collectorDrops)}
            </div>
            <div className={`cap-stat${disc ? '' : ' accent'}`}>
              <div className="cap-label">Servers</div>
              <div className="cap-value">{data.devices_total ?? 0}</div>
              <div className="cap-sub small muted">
                <span className="good">{data.devices_connected ?? 0} up</span>
                {' · '}
                <span className={disc ? 'badq' : ''}>{data.devices_disconnected ?? 0} down</span>
              </div>
              {dropsLine(opcDrops)}
            </div>
            <div className={`cap-stat${tagsDegraded || globalSim ? '' : ' accent'}`}>
              <div className="cap-label">Tags</div>
              <div className={`cap-value${tagsDegraded ? ' badq' : ''}`}>
                {tagsOffline ? liveGood : (data.tags_enabled ?? 0)}
                <span className="muted" style={{ fontSize: '0.7em' }}>/{data.tags_total ?? 0}</span>
              </div>
              <div className={`cap-sub small ${tagsDegraded ? 'badq' : 'muted'}`}>
                {tagsSub}
              </div>
              {tagsSimulated > 0 && (
                <div className="cap-sub small">
                  <span className="pill sim-count-pill">sim {tagsSimulated}</span>
                </div>
              )}
            </div>
            <div className="cap-stat">
              <div className="cap-label">Poll avg</div>
              <div className="cap-value">{formatMs(data.poll_avg_ms)}</div>
              <div className="cap-sub small muted">last ≤5 samples</div>
            </div>
            <div className="cap-stat">
              <div className="cap-label">DB write</div>
              <div className="cap-value">{formatRate(data.samples_per_sec)}</div>
              <div className="cap-sub small muted">
                {Math.round(data.samples_written_total || 0).toLocaleString()} written
                {data.write_errors_total > 0 && (
                  <span className="badq"> · {Math.round(data.write_errors_total)} err</span>
                )}
              </div>
              {dropsLine(dbDrops, 'errors')}
            </div>
            <div className={`cap-stat${data.database_connected ? ' accent' : ''}`}>
              <div className="cap-label">Database</div>
              <div className={`cap-value ${data.database_connected ? 'good' : 'badq'}`} style={{ fontSize: '1.1rem' }}>
                {data.database_connected ? 'up' : 'down'}
              </div>
              <div className="cap-sub small muted">historian</div>
            </div>
          </div>

          <section className="panel overview-links">
            <h3>Tag simulation</h3>
            <p className="muted" style={{ marginTop: 0 }}>
              Prefer <strong>per-tag Sim</strong> on the DB write list (default off; never auto on OPC disconnect).
              Real OPC keeps running for tags without simulate. See docs/tag-simulation.md.
            </p>
            <div className="overview-link-row" style={{ alignItems: 'center', gap: 12, flexWrap: 'wrap' }}>
              <button type="button" className="nav-btn" onClick={() => onNavigate('db-list')}>
                Open DB write list
              </button>
              {tagsSimulated > 0 && (
                <span className="pill sim-count-pill">sim {tagsSimulated}</span>
              )}
              <button
                type="button"
                className="secondary small-btn"
                onClick={() => setShowLegacy((v) => !v)}
              >
                {showLegacy ? 'Hide legacy' : 'Legacy: simulate all…'}
              </button>
            </div>
            {showLegacy && (
              <div className="overview-link-row" style={{ alignItems: 'center', gap: 12, marginTop: 10 }}>
                <label className="muted" style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <input
                    type="checkbox"
                    disabled={simBusy || !!data.sim_browser}
                    checked={!!globalSim}
                    onChange={(e) => toggleTagSim(e.target.checked)}
                  />
                  Legacy global master (all tags · pauses OPC)
                </label>
                {data.sim_browser && (
                  <span className="muted small">LEVEL2_SIM_BROWSER is on (full sim)</span>
                )}
              </div>
            )}
          </section>

          <section className="panel overview-links">
            <h3>Jump to</h3>
            <div className="overview-link-row">
              <button type="button" className="nav-btn" onClick={() => onNavigate('monitor')}>
                Address Space
              </button>
              <button type="button" className="nav-btn" onClick={() => onNavigate('db-list')}>
                DB write list
              </button>
              <button type="button" className="nav-btn" onClick={() => onNavigate('diag')}>
                Diagnostics
              </button>
              <button type="button" className="nav-btn" onClick={() => onNavigate('database')}>
                Database
              </button>
              <button type="button" className="nav-btn" onClick={() => onNavigate('servers')}>
                Servers
              </button>
            </div>
          </section>

          <section className="panel">
            <div className="panel-head">
              <h3 style={{ margin: 0 }}>Recent errors</h3>
              <div className="panel-head-actions">
                <button type="button" className="secondary small-btn danger-btn" onClick={resetAlarms}>
                  Reset alarms
                </button>
                <button type="button" className="secondary small-btn" onClick={() => onNavigate('diag')}>
                  Open diagnostics
                </button>
              </div>
            </div>
            {errors.length === 0 ? (
              <p className="muted">No recent OPC/DB warnings or errors.</p>
            ) : (
              <div className="diag-log-wrap">
                <table className="diag-log">
                  <thead>
                    <tr>
                      <th>Time</th>
                      <th>Level</th>
                      <th>Source</th>
                      <th>Message</th>
                    </tr>
                  </thead>
                  <tbody>
                    {errors.map((e, i) => (
                      <tr key={`${e.time}-${i}`} className={`lvl-${e.level}`}>
                        <td className="mono small">{formatTime(e.time)}</td>
                        <td>{e.level}</td>
                        <td>{e.category === 'opc_read' ? 'OPC' : e.category === 'opc_write' ? 'Write' : e.category === 'db_write' ? 'DB' : e.category}</td>
                        <td>
                          {e.message}
                          {e.device_id && <span className="muted small"> · {e.device_id}</span>}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>
        </>
      )}
    </div>
  )
}
