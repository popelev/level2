import { useEffect, useState } from 'react'
import { getJSON } from './api.js'
import OverviewPage from './pages/OverviewPage.jsx'
import ServersPage from './pages/ServersPage.jsx'
import MonitorPage from './pages/MonitorPage.jsx'
import ProjectsPage from './pages/ProjectsPage.jsx'
import DbWriteListPage from './pages/DbWriteListPage.jsx'
import TagsImportExportPage from './pages/TagsImportExportPage.jsx'
import DiagnosticsPage from './pages/DiagnosticsPage.jsx'
import DatabasePage from './pages/DatabasePage.jsx'
import CapacityPage from './pages/CapacityPage.jsx'

function deviceFromHash() {
  const h = location.hash || ''
  const q = h.indexOf('?')
  if (q < 0) return ''
  return new URLSearchParams(h.slice(q + 1)).get('device') || ''
}

function currentPage() {
  const raw = location.hash || '#/overview'
  const h = raw.replace(/^#\/?/, '')
  if (!h || h.startsWith('overview') || h.startsWith('home')) return 'overview'
  if (h.startsWith('monitor')) return 'monitor'
  if (h.startsWith('db-list') || h.startsWith('tags')) return 'db-list'
  if (h.startsWith('import')) return 'import'
  if (h.startsWith('diag')) return 'diag'
  if (h.startsWith('capacity') || h.startsWith('storage')) return 'capacity'
  if (h.startsWith('database') || h.startsWith('db-settings')) return 'database'
  if (h.startsWith('project')) return 'projects'
  if (h.startsWith('servers')) return 'servers'
  return 'overview'
}

function NavGroup({ label, children }) {
  return (
    <div className="nav-group">
      {label ? <span className="nav-group-label">{label}</span> : null}
      <div className="nav-group-btns">{children}</div>
    </div>
  )
}

function formatPollMs(ms) {
  if (ms == null || Number.isNaN(Number(ms))) return null
  const n = Number(ms)
  if (n >= 1000) return `${(n / 1000).toFixed(n >= 10000 ? 0 : 1)}s`
  return `${Math.round(n)} ms`
}

function formatRate(n) {
  if (n == null || Number.isNaN(Number(n)) || n <= 0) return '0/s'
  if (n >= 100) return `${Math.round(n)}/s`
  if (n >= 10) return `${n.toFixed(1)}/s`
  return `${n.toFixed(2)}/s`
}

function StatusPills({ summary }) {
  if (!summary) {
    return (
      <>
        <span className="pill">API: …</span>
        <span className="pill">Collector: …</span>
      </>
    )
  }

  const apiOk = summary.api_ok !== false
  const ready = !!(summary.collector_ready ?? summary.ready)
  const total = summary.devices_total ?? summary.servers?.total ?? 0
  const connected = summary.devices_connected ?? summary.servers?.connected ?? 0
  const tagsEnabled = summary.tags_enabled ?? summary.tags?.enabled ?? 0
  const tagsTotal = summary.tags_total ?? summary.tags?.total ?? 0
  const qualityGood = summary.quality_good ?? summary.tags?.good_quality ?? 0
  const qualityBad = summary.quality_bad ?? summary.tags?.bad_quality ?? 0
  const qualityPct = summary.quality_good_pct ?? summary.tags?.good_pct
  const samplesPerSec = summary.samples_per_sec ?? summary.db_write?.samples_per_sec ?? 0
  const samplesWritten = summary.samples_written_total ?? summary.db_write?.samples_written_total ?? 0
  const writeErrors = summary.write_errors_total ?? summary.db_write?.write_errors_total ?? 0
  const spoolDepth = summary.spool_depth ?? summary.db_write?.spool_depth ?? 0
  const collectorDrops = summary.collector_down_last_hour ?? 0
  const opcDrops = summary.opc_disconnects_last_hour ?? 0
  const dbDrops = summary.db_write_errors_last_hour ?? 0

  const opcOk = total === 0 ? ready : connected > 0 && connected === total
  const opcLabel = total === 0
    ? (ready ? 'OPC: idle' : 'OPC: none')
    : `OPC: ${connected}/${total}`

  const sampled = qualityGood + qualityBad
  let qualityClass = ''
  let qualityLabel = 'Quality: —'
  if (sampled > 0) {
    qualityClass = qualityBad > 0 ? 'bad' : 'ok'
    qualityLabel = qualityBad > 0 && qualityBad <= 20
      ? `Quality: ${qualityBad} bad`
      : `Quality: ${qualityPct != null ? Math.round(qualityPct) : '—'}% good`
  } else if (tagsEnabled > 0) {
    qualityClass = 'bad'
    qualityLabel = 'Quality: no data'
  }

  const pollLabel = summary.poll_avg_ms != null
    ? `Poll: ${formatPollMs(summary.poll_avg_ms)}`
    : 'Poll: —'
  const writeErr = writeErrors > 0 || spoolDepth > 0 || dbDrops > 0
  const dbLabel = `DB: ${formatRate(samplesPerSec)}`

  return (
    <>
      <span className={`pill ${apiOk ? 'ok' : 'bad'}`} title="Process HTTP API is up (/healthz)">
        {apiOk ? 'API: healthy' : 'API: down'}
      </span>
      <span
        className={`pill ${ready ? 'ok' : 'bad'}`}
        title={`Same as /readyz: collector is ready when OPC is connected (or SIM mode). ${collectorDrops} not-ready drops / last hour`}
      >
        {ready ? 'Collector: ready' : 'Collector: not ready'}
      </span>
      <span
        className={`pill ${opcOk ? 'ok' : 'bad'}`}
        title={`OPC UA devices connected vs configured. ${opcDrops} disconnects / last hour`}
      >
        {opcLabel}
      </span>
      <span
        className={`pill ${qualityClass}`}
        title={`Tags ${tagsEnabled}/${tagsTotal} enabled · Good ${qualityGood} · Bad ${qualityBad}`}
      >
        {qualityLabel}
      </span>
      <span
        className="pill"
        title="Average wall-clock gap between last samples (poll_avg_ms across tags)"
      >
        {pollLabel}
      </span>
      <span
        className={`pill ${writeErr ? 'bad' : (samplesPerSec > 0 ? 'ok' : '')}`}
        title={`Historian write rate · written ${Math.round(samplesWritten).toLocaleString()} · errors ${Math.round(writeErrors)} · spool ${Math.round(spoolDepth)} · ${dbDrops} write errors / last hour`}
      >
        {dbLabel}
      </span>
    </>
  )
}

export default function App() {
  const [page, setPage] = useState(currentPage)
  const [monitorDevice, setMonitorDevice] = useState(deviceFromHash)
  const [summary, setSummary] = useState(null)
  const [devices, setDevices] = useState([])
  const [err, setErr] = useState('')

  const refreshStatus = async () => {
    try {
      const [sum, devs] = await Promise.all([
        getJSON('/api/v1/status/summary'),
        getJSON('/api/v1/devices'),
      ])
      setSummary(sum)
      setDevices(devs)
    } catch (e) {
      setSummary({ api_ok: false, ready: false, collector_ready: false, health: 'down' })
      throw e
    }
  }

  const health = summary?.api_ok === false ? 'down' : (summary ? 'ok' : '…')
  const ready = !summary
    ? '…'
    : (summary.collector_ready || summary.ready ? 'ready' : (summary.ready_detail || 'not ready'))

  useEffect(() => {
    if (!location.hash || location.hash === '#' || location.hash === '#/') {
      location.hash = '#/overview'
    }
    const onHash = () => {
      setPage(currentPage())
      setMonitorDevice(deviceFromHash())
    }
    window.addEventListener('hashchange', onHash)
    refreshStatus().catch((e) => setErr(String(e.message || e)))
    const t = setInterval(() => {
      refreshStatus().catch(() => {})
    }, 4000)
    return () => {
      window.removeEventListener('hashchange', onHash)
      clearInterval(t)
    }
  }, [])

  const go = (name, query) => {
    location.hash = query ? `#/${name}?${query}` : `#/${name}`
    setPage(name)
    if (name === 'monitor' || name === 'import' || name === 'db-list') setMonitorDevice(deviceFromHash())
  }

  const deviceQ = monitorDevice ? `device=${encodeURIComponent(monitorDevice)}` : undefined

  return (
    <div className="app ua">
      <header className="top">
        <div>
          <h1 className="brand">Level2</h1>
          <p className="sub">Platform Console · Connectivity</p>
        </div>
        <div className="meta status-strip" aria-label="System status">
          <StatusPills summary={summary} />
        </div>
      </header>

      <nav className="nav">
        <NavGroup>
          <button
            type="button"
            className={page === 'overview' ? 'nav-btn active' : 'nav-btn'}
            onClick={() => go('overview')}
          >
            Overview
          </button>
        </NavGroup>

        <NavGroup label="Connectivity">
          <button
            type="button"
            className={page === 'servers' ? 'nav-btn active' : 'nav-btn'}
            onClick={() => go('servers')}
          >
            Servers
          </button>
          <button
            type="button"
            className={page === 'monitor' ? 'nav-btn active' : 'nav-btn'}
            onClick={() => go('monitor', deviceQ)}
          >
            Address Space
          </button>
        </NavGroup>

        <NavGroup label="Data">
          <button
            type="button"
            className={page === 'db-list' ? 'nav-btn active' : 'nav-btn'}
            onClick={() => go('db-list', deviceQ)}
          >
            DB write list
          </button>
          <button
            type="button"
            className={page === 'import' ? 'nav-btn active' : 'nav-btn'}
            onClick={() => go('import', deviceQ)}
          >
            Import / Export
          </button>
        </NavGroup>

        <NavGroup label="Config">
          <button
            type="button"
            className={page === 'projects' ? 'nav-btn active' : 'nav-btn'}
            onClick={() => go('projects')}
          >
            Projects
          </button>
          <button
            type="button"
            className={page === 'database' ? 'nav-btn active' : 'nav-btn'}
            onClick={() => go('database')}
          >
            Database
          </button>
        </NavGroup>

        <NavGroup label="System">
          <button
            type="button"
            className={page === 'diag' ? 'nav-btn active' : 'nav-btn'}
            onClick={() => go('diag')}
          >
            Diagnostics
          </button>
          <button
            type="button"
            className={page === 'capacity' ? 'nav-btn active' : 'nav-btn'}
            onClick={() => go('capacity')}
          >
            Capacity
          </button>
        </NavGroup>
      </nav>

      {err && <p className="err">{err}</p>}

      {page === 'overview' && (
        <OverviewPage
          health={health}
          ready={ready}
          onError={setErr}
          onNavigate={(name) => go(name, name === 'monitor' || name === 'db-list' ? deviceQ : undefined)}
        />
      )}
      {page === 'servers' && (
        <ServersPage
          devices={devices}
          onChanged={refreshStatus}
          onError={setErr}
        />
      )}
      {page === 'monitor' && (
        <MonitorPage
          key={monitorDevice || 'default'}
          devices={devices}
          initialDeviceId={monitorDevice}
          onError={setErr}
          onDevicesChanged={refreshStatus}
        />
      )}
      {page === 'db-list' && (
        <DbWriteListPage
          key={monitorDevice || 'db-default'}
          devices={devices}
          initialDeviceId={monitorDevice}
          onError={setErr}
          onDevicesChanged={refreshStatus}
        />
      )}
      {page === 'import' && (
        <TagsImportExportPage
          key={monitorDevice || 'import-default'}
          devices={devices}
          initialDeviceId={monitorDevice}
          onError={setErr}
          onDevicesChanged={refreshStatus}
        />
      )}
      {page === 'projects' && (
        <ProjectsPage
          devices={devices}
          onError={setErr}
          onDevicesChanged={refreshStatus}
        />
      )}
      {page === 'database' && (
        <DatabasePage onError={setErr} />
      )}
      {page === 'diag' && (
        <DiagnosticsPage onError={setErr} />
      )}
      {page === 'capacity' && (
        <CapacityPage onError={setErr} />
      )}
    </div>
  )
}
