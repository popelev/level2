import { useEffect, useState } from 'react'
import { getJSON } from './api.js'
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
  const h = (location.hash || '#/servers').replace(/^#\/?/, '')
  if (h.startsWith('monitor')) return 'monitor'
  if (h.startsWith('db-list') || h.startsWith('tags')) return 'db-list'
  if (h.startsWith('import')) return 'import'
  if (h.startsWith('diag')) return 'diag'
  if (h.startsWith('capacity') || h.startsWith('storage')) return 'capacity'
  if (h.startsWith('database') || h.startsWith('db-settings')) return 'database'
  if (h.startsWith('project')) return 'projects'
  return 'servers'
}

function NavGroup({ label, children }) {
  return (
    <div className="nav-group">
      <span className="nav-group-label">{label}</span>
      <div className="nav-group-btns">{children}</div>
    </div>
  )
}

export default function App() {
  const [page, setPage] = useState(currentPage)
  const [monitorDevice, setMonitorDevice] = useState(deviceFromHash)
  const [health, setHealth] = useState('…')
  const [ready, setReady] = useState('…')
  const [devices, setDevices] = useState([])
  const [err, setErr] = useState('')

  const refreshStatus = async () => {
    const [h, rd, devs] = await Promise.all([
      fetch('/healthz').then((r) => r.text()),
      fetch('/readyz').then(async (r) => ({ ok: r.ok, text: await r.text() })),
      getJSON('/api/v1/devices'),
    ])
    setHealth(h.trim())
    setReady(rd.ok ? 'ready' : rd.text)
    setDevices(devs)
  }

  useEffect(() => {
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
        <div className="meta">
          <span className="pill ok">health {health}</span>
          <span className={`pill ${ready === 'ready' ? 'ok' : 'bad'}`}>{ready}</span>
        </div>
      </header>

      <nav className="nav">
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
