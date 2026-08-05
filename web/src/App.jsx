import { useEffect, useState } from 'react'
import { getJSON } from './api.js'
import ServersPage from './pages/ServersPage.jsx'
import MonitorPage from './pages/MonitorPage.jsx'
import ProjectsPage from './pages/ProjectsPage.jsx'

function deviceFromHash() {
  const h = location.hash || ''
  const q = h.indexOf('?')
  if (q < 0) return ''
  return new URLSearchParams(h.slice(q + 1)).get('device') || ''
}

function currentPage() {
  const h = (location.hash || '#/servers').replace(/^#\/?/, '')
  if (h.startsWith('monitor') || h.startsWith('tags')) return 'monitor'
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
    if (name === 'monitor') setMonitorDevice(deviceFromHash())
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
          onClick={() => go('monitor')}
        >
          Monitored tags
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
