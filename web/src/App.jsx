import { useEffect, useState } from 'react'
import { getJSON } from './api.js'
import ServersPage from './pages/ServersPage.jsx'
import MonitorPage from './pages/MonitorPage.jsx'
import ProjectsPage from './pages/ProjectsPage.jsx'

import DbWriteListPage from './pages/DbWriteListPage.jsx'
import TagsImportExportPage from './pages/TagsImportExportPage.jsx'

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
  if (h.startsWith('project')) return 'projects'
  return 'servers'
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
          onClick={() => go('monitor', monitorDevice ? `device=${encodeURIComponent(monitorDevice)}` : undefined)}
        >
          Address Space
        </button>
        <button
          type="button"
          className={page === 'db-list' ? 'nav-btn active' : 'nav-btn'}
          onClick={() => go('db-list', monitorDevice ? `device=${encodeURIComponent(monitorDevice)}` : undefined)}
        >
          DB write list
        </button>
        <button
          type="button"
          className={page === 'import' ? 'nav-btn active' : 'nav-btn'}
          onClick={() => go('import', monitorDevice ? `device=${encodeURIComponent(monitorDevice)}` : undefined)}
        >
          Import / Export
        </button>
        <button
          type="button"
          className={page === 'projects' ? 'nav-btn active' : 'nav-btn'}
          onClick={() => go('projects')}
        >
          Projects
        </button>
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
    </div>
  )
}
