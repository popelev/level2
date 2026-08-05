import { useEffect, useMemo, useState } from 'react'

async function getJSON(url) {
  const r = await fetch(url)
  if (!r.ok) throw new Error(`${r.status} ${await r.text()}`)
  return r.json()
}

function formatValue(sample) {
  if (!sample) return '—'
  if (sample.value_num != null) return Number(sample.value_num).toFixed(3)
  if (sample.value_text != null) return sample.value_text
  if (sample.value_bool != null) return String(sample.value_bool)
  return '—'
}

function formatTime(t) {
  if (!t || t.startsWith('0001')) return '—'
  try {
    return new Date(t).toLocaleTimeString()
  } catch {
    return t
  }
}

export default function App() {
  const [health, setHealth] = useState('…')
  const [ready, setReady] = useState('…')
  const [devices, setDevices] = useState([])
  const [tags, setTags] = useState([])
  const [deviceId, setDeviceId] = useState('')
  const [tagId, setTagId] = useState('')
  const [history, setHistory] = useState([])
  const [nodeId, setNodeId] = useState('ns=0;i=85')
  const [browse, setBrowse] = useState([])
  const [expanded, setExpanded] = useState([])
  const [err, setErr] = useState('')
  const [liveMsg, setLiveMsg] = useState(null)
  const [filter, setFilter] = useState('')
  const [importBusy, setImportBusy] = useState(false)
  const [importMsg, setImportMsg] = useState('')
  const [replaceTags, setReplaceTags] = useState(false)

  const selectedDevice = useMemo(
    () => devices.find((d) => d.id === deviceId) || null,
    [devices, deviceId],
  )

  const selectedTag = useMemo(
    () => tags.find((t) => t.tag.id === tagId) || null,
    [tags, tagId],
  )

  const visibleTags = useMemo(() => {
    const q = filter.trim().toLowerCase()
    if (!q) return tags
    return tags.filter(
      (t) =>
        t.tag.id.toLowerCase().includes(q) ||
        t.tag.node_id.toLowerCase().includes(q) ||
        String(t.tag.datatype).toLowerCase().includes(q),
    )
  }, [tags, filter])

  const refresh = async (dev = deviceId) => {
    try {
      setErr('')
      const tagsURL = dev
        ? `/api/v1/tags?device_id=${encodeURIComponent(dev)}`
        : '/api/v1/tags'
      const [h, rd, devs, t] = await Promise.all([
        fetch('/healthz').then((r) => r.text()),
        fetch('/readyz').then(async (r) => ({ ok: r.ok, text: await r.text() })),
        getJSON('/api/v1/devices'),
        getJSON(tagsURL),
      ])
      setHealth(h.trim())
      setReady(rd.ok ? 'ready' : rd.text)
      setDevices(devs)
      setTags(t)
      if (!dev && devs.length && !deviceId) {
        setDeviceId(devs[0].id)
      }
      if (tagId && !t.some((x) => x.tag.id === tagId)) {
        setTagId('')
      }
    } catch (e) {
      setErr(String(e.message || e))
    }
  }

  const loadHistory = async (id) => {
    if (!id) {
      setHistory([])
      return
    }
    try {
      const rows = await getJSON(`/api/v1/tags/${encodeURIComponent(id)}/history?limit=12`)
      setHistory(rows)
    } catch {
      setHistory([])
    }
  }

  useEffect(() => {
    refresh()
    const t = setInterval(() => refresh(deviceId), 3000)
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (!deviceId && devices[0]) {
      setDeviceId(devices[0].id)
      return
    }
    if (deviceId) refresh(deviceId)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [deviceId])

  useEffect(() => {
    loadHistory(tagId)
    const t = setInterval(() => loadHistory(tagId), 5000)
    return () => clearInterval(t)
  }, [tagId])

  const selectDevice = (id) => {
    setDeviceId(id)
    setTagId('')
    setFilter('')
  }

  const selectTag = (row) => {
    setTagId(row.tag.id)
    setNodeId(row.tag.node_id)
  }

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
      const prefix = selectedDevice?.id || 'udt'
      const rows = await fetch('/api/v1/expand', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          node_id: nodeId,
          parent_tag_id: prefix,
          max_depth: 6,
        }),
      }).then(async (r) => {
        if (!r.ok) throw new Error(`${r.status} ${await r.text()}`)
        return r.json()
      })
      setExpanded(rows)
    } catch (e) {
      setErr(String(e.message || e))
    }
  }

  const importExcel = async (file) => {
    if (!deviceId || !file) return
    setImportBusy(true)
    setImportMsg('')
    setErr('')
    try {
      const fd = new FormData()
      fd.append('file', file)
      const q = replaceTags ? '?replace=1' : ''
      const r = await fetch(`/api/v1/devices/${encodeURIComponent(deviceId)}/tags/import${q}`, {
        method: 'POST',
        body: fd,
      })
      const text = await r.text()
      if (!r.ok) throw new Error(text || r.status)
      const data = JSON.parse(text)
      setImportMsg(
        `Импорт: +${data.added} / ~${data.updated} (всего ${data.total})` +
          (data.errors?.length ? `; предупреждений: ${data.errors.length}` : ''),
      )
      await refresh(deviceId)
    } catch (e) {
      setErr(String(e.message || e))
    } finally {
      setImportBusy(false)
    }
  }

  const statusPill = useMemo(() => {
    if (ready === 'ready') return <span className="pill ok">ready</span>
    return <span className="pill bad">{ready}</span>
  }, [ready])

  return (
    <div className="app">
      <header className="top">
        <div>
          <h1 className="brand">Level2</h1>
          <p className="sub">Connectivity · устройство → теги → browse</p>
        </div>
        <div className="meta">
          <span className="pill ok">health {health}</span>
          {statusPill}
          {liveMsg && (
            <span className="pill">
              ws {liveMsg.tag_id}={formatValue(liveMsg)}
            </span>
          )}
          <button type="button" className="secondary" onClick={() => refresh(deviceId)}>
            Refresh
          </button>
        </div>
      </header>

      {err && <p className="err">{err}</p>}

      <div className="layout">
        <aside className="panel">
          <h2>Устройства</h2>
          <ul className="device-list">
            {devices.map((d) => (
              <li key={d.id}>
                <button
                  type="button"
                  className={d.id === deviceId ? 'device active' : 'device'}
                  onClick={() => selectDevice(d.id)}
                >
                  <span className="mono">{d.id}</span>
                  <span className="muted">{d.tag_count} tags</span>
                  <span className="mono small">{d.endpoint}</span>
                </button>
              </li>
            ))}
            {devices.length === 0 && <li className="muted">Нет устройств в config</li>}
          </ul>
          {selectedDevice && (
            <div className="device-meta">
              <div><span className="muted">security</span> {selectedDevice.security}</div>
              <div className="mono small">{selectedDevice.endpoint}</div>
              <div className="import-box">
                <div className="muted">Импорт OPC-нод из Excel</div>
                <p className="hint">
                  Колонки: Area, Path, Signal, MeasurePoint NodeId, DataType, DataType Name
                </p>
                <label className="check">
                  <input
                    type="checkbox"
                    checked={replaceTags}
                    onChange={(e) => setReplaceTags(e.target.checked)}
                  />
                  заменить все теги устройства
                </label>
                <input
                  type="file"
                  accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
                  disabled={importBusy || !deviceId}
                  onChange={(e) => {
                    const f = e.target.files?.[0]
                    e.target.value = ''
                    if (f) importExcel(f)
                  }}
                />
                {importBusy && <div className="muted">Импорт…</div>}
                {importMsg && <div className="good">{importMsg}</div>}
              </div>
            </div>
          )}
        </aside>

        <section className="panel">
          <div className="panel-head">
            <h2>Теги {selectedDevice ? `· ${selectedDevice.id}` : ''}</h2>
            <input
              className="search"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="фильтр id / node / type"
            />
          </div>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Tag</th>
                  <th>Type</th>
                  <th>Value</th>
                  <th>Q</th>
                  <th>Updated</th>
                </tr>
              </thead>
              <tbody>
                {visibleTags.map((t) => (
                  <tr
                    key={t.tag.id}
                    className={t.tag.id === tagId ? 'selected' : ''}
                    onClick={() => selectTag(t)}
                  >
                    <td>
                      <div className="mono">{t.tag.id}</div>
                      <div className="mono small muted">{t.tag.node_id}</div>
                    </td>
                    <td>{t.tag.datatype}</td>
                    <td className="mono">{formatValue(t.sample)}</td>
                    <td className={t.sample?.quality === 0 ? 'good' : 'badq'}>
                      {t.sample ? t.sample.quality : '—'}
                    </td>
                    <td className="muted">{formatTime(t.updated_at || t.sample?.time)}</td>
                  </tr>
                ))}
                {visibleTags.length === 0 && (
                  <tr>
                    <td colSpan={5} className="muted">Нет тегов для устройства</td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          {selectedTag && (
            <div className="tag-detail">
              <h3>Выбранный тег · {selectedTag.tag.id}</h3>
              <div className="detail-grid">
                <div><span className="muted">node_id</span><div className="mono">{selectedTag.tag.node_id}</div></div>
                <div><span className="muted">datatype</span><div>{selectedTag.tag.datatype}</div></div>
                <div><span className="muted">interval</span><div>{selectedTag.tag.interval_ms} ms</div></div>
                <div><span className="muted">enabled</span><div>{String(selectedTag.tag.enabled)}</div></div>
                <div><span className="muted">value</span><div className="mono">{formatValue(selectedTag.sample)}</div></div>
              </div>
              <div className="row">
                <button type="button" onClick={() => doBrowse(selectedTag.tag.node_id)}>
                  Browse node
                </button>
                <button
                  type="button"
                  className="secondary"
                  onClick={() => loadHistory(selectedTag.tag.id)}
                >
                  Reload history
                </button>
              </div>
              <h3>History (last points)</h3>
              <div className="table-wrap compact">
                <table>
                  <thead>
                    <tr>
                      <th>Time</th>
                      <th>Value</th>
                      <th>Q</th>
                    </tr>
                  </thead>
                  <tbody>
                    {history.map((h) => (
                      <tr key={`${h.time}-${h.tag_id}`}>
                        <td className="mono small">{formatTime(h.time)}</td>
                        <td className="mono">{formatValue(h)}</td>
                        <td className={h.quality === 0 ? 'good' : 'badq'}>{h.quality}</td>
                      </tr>
                    ))}
                    {history.length === 0 && (
                      <tr><td colSpan={3} className="muted">Нет точек</td></tr>
                    )}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </section>

        <section className="panel">
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
                <span className="mono small">{n.node_id}</span>
                <span className="pill">{n.is_leaf ? 'leaf' : 'node'}</span>
              </li>
            ))}
          </ul>
          {expanded.length > 0 && (
            <>
              <h3>Expanded leaves</h3>
              <p className="hint">Добавление в config пока вручную / следующим шагом API CRUD</p>
              <div className="table-wrap compact">
                <table>
                  <thead>
                    <tr>
                      <th>Tag id</th>
                      <th>Path</th>
                      <th>Type</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {expanded.map((e) => (
                      <tr key={e.node_id}>
                        <td className="mono">{e.id}</td>
                        <td className="mono small">{e.browse_path}</td>
                        <td>{e.datatype}</td>
                        <td>
                          <button
                            type="button"
                            className="secondary small-btn"
                            onClick={() => setNodeId(e.node_id)}
                          >
                            Use node
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </section>
      </div>
    </div>
  )
}
