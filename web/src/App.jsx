import { useEffect, useMemo, useState } from 'react'

async function getJSON(url) {
  const r = await fetch(url)
  if (!r.ok) throw new Error(`${r.status} ${await r.text()}`)
  return r.json()
}

function formatValue(sample) {
  if (!sample) return '—'
  if (sample.value_num != null) return String(sample.value_num)
  if (sample.value_text != null) return sample.value_text
  if (sample.value_bool != null) return String(sample.value_bool)
  return '—'
}

export default function App() {
  const [health, setHealth] = useState('…')
  const [ready, setReady] = useState('…')
  const [devices, setDevices] = useState([])
  const [tags, setTags] = useState([])
  const [nodeId, setNodeId] = useState('ns=0;i=85')
  const [browse, setBrowse] = useState([])
  const [expanded, setExpanded] = useState([])
  const [err, setErr] = useState('')
  const [liveMsg, setLiveMsg] = useState(null)

  const refresh = async () => {
    try {
      setErr('')
      const [h, rd, devs, t] = await Promise.all([
        fetch('/healthz').then((r) => r.text()),
        fetch('/readyz').then(async (r) => ({ ok: r.ok, text: await r.text() })),
        getJSON('/api/v1/devices'),
        getJSON('/api/v1/tags'),
      ])
      setHealth(h.trim())
      setReady(rd.ok ? 'ready' : rd.text)
      setDevices(devs)
      setTags(t)
    } catch (e) {
      setErr(String(e.message || e))
    }
  }

  useEffect(() => {
    refresh()
    const t = setInterval(refresh, 3000)
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${proto}://${location.host}/api/v1/ws/stream`)
    ws.onmessage = (ev) => {
      try {
        setLiveMsg(JSON.parse(ev.data))
      } catch {
        /* ignore */
      }
    }
    return () => {
      clearInterval(t)
      ws.close()
    }
  }, [])

  const doBrowse = async (id = nodeId) => {
    try {
      setErr('')
      setNodeId(id)
      const nodes = await getJSON(`/api/v1/browse?node_id=${encodeURIComponent(id)}`)
      setBrowse(nodes)
    } catch (e) {
      setErr(String(e.message || e))
    }
  }

  const doExpand = async () => {
    try {
      setErr('')
      const tags = await fetch('/api/v1/expand', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ node_id: nodeId, parent_tag_id: 'udt', max_depth: 6 }),
      }).then(async (r) => {
        if (!r.ok) throw new Error(`${r.status} ${await r.text()}`)
        return r.json()
      })
      setExpanded(tags)
    } catch (e) {
      setErr(String(e.message || e))
    }
  }

  const statusPill = useMemo(() => {
    if (ready === 'ready') return <span className="pill ok">OPC ready</span>
    return <span className="pill bad">OPC {ready}</span>
  }, [ready])

  return (
    <div className="app">
      <h1 className="brand">Level2</h1>
      <p className="sub">Platform Console · Connectivity</p>

      <div className="meta">
        <span className="pill ok">health {health}</span>
        {statusPill}
        {liveMsg && (
          <span className="pill">
            ws {liveMsg.tag_id}={formatValue(liveMsg)}
          </span>
        )}
      </div>

      {err && <p className="err">{err}</p>}

      <div className="grid">
        <section>
          <h2>Devices & tags</h2>
          <div className="row">
            <button type="button" onClick={refresh}>Refresh</button>
          </div>
          <table>
            <thead>
              <tr>
                <th>Device</th>
                <th>Endpoint</th>
                <th>Tags</th>
              </tr>
            </thead>
            <tbody>
              {devices.map((d) => (
                <tr key={d.id}>
                  <td className="mono">{d.id}</td>
                  <td className="mono">{d.endpoint}</td>
                  <td>{d.tag_count}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <table style={{ marginTop: 16 }}>
            <thead>
              <tr>
                <th>Tag</th>
                <th>NodeId</th>
                <th>Value</th>
                <th>Q</th>
              </tr>
            </thead>
            <tbody>
              {tags.map((t) => (
                <tr key={t.tag.id}>
                  <td className="mono">{t.tag.id}</td>
                  <td className="mono">{t.tag.node_id}</td>
                  <td className="mono">{formatValue(t.sample)}</td>
                  <td className={t.sample?.quality === 0 ? 'good' : 'badq'}>
                    {t.sample ? t.sample.quality : '—'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>

        <section>
          <h2>Browse / Expand</h2>
          <div className="row">
            <input
              value={nodeId}
              onChange={(e) => setNodeId(e.target.value)}
              placeholder="ns=4;i=4207"
            />
            <button type="button" onClick={() => doBrowse(nodeId)}>Browse</button>
            <button type="button" className="secondary" onClick={doExpand}>Expand</button>
          </div>
          <ul className="tree">
            {browse.map((n) => (
              <li key={n.node_id}>
                <button type="button" className="link" onClick={() => doBrowse(n.node_id)}>
                  {n.browse_name}
                </button>
                <span className="mono">{n.node_id}</span>
                <span className="pill">{n.is_leaf ? 'leaf' : 'node'}</span>
              </li>
            ))}
          </ul>
          {expanded.length > 0 && (
            <>
              <h2 style={{ marginTop: 18 }}>Expanded leaves</h2>
              <table>
                <thead>
                  <tr>
                    <th>Tag id</th>
                    <th>Path</th>
                    <th>Type</th>
                    <th>NodeId</th>
                  </tr>
                </thead>
                <tbody>
                  {expanded.map((e) => (
                    <tr key={e.node_id}>
                      <td className="mono">{e.id}</td>
                      <td className="mono">{e.browse_path}</td>
                      <td>{e.datatype}</td>
                      <td className="mono">{e.node_id}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          )}
        </section>
      </div>
    </div>
  )
}
