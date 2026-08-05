import { useCallback, useEffect, useMemo, useState } from 'react'

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

const ROOT_ID = 'ns=0;i=84'

function TreeNode({ node, depth, deviceId, selectedNodeId, onSelect, loadChildren }) {
  const [open, setOpen] = useState(false)
  const [kids, setKids] = useState(null)
  const [loading, setLoading] = useState(false)

  const toggle = async (e) => {
    e.stopPropagation()
    if (node.is_leaf) {
      onSelect(node)
      return
    }
    if (!open && kids == null) {
      setLoading(true)
      try {
        const rows = await loadChildren(node.node_id)
        setKids(rows)
      } finally {
        setLoading(false)
      }
    }
    setOpen((v) => !v)
    onSelect(node)
  }

  return (
    <div className="tree-node">
      <button
        type="button"
        className={`tree-row${selectedNodeId === node.node_id ? ' selected' : ''}`}
        style={{ paddingLeft: 8 + depth * 14 }}
        onClick={toggle}
      >
        <span className="chev">{node.is_leaf ? '·' : open ? '▾' : '▸'}</span>
        <span className={`icon ${node.is_leaf ? 'var' : 'fold'}`} />
        <span className="name">{node.browse_name || node.display_name}</span>
        {loading && <span className="muted small">…</span>}
      </button>
      {open && kids && kids.map((ch) => (
        <TreeNode
          key={ch.node_id}
          node={ch}
          depth={depth + 1}
          deviceId={deviceId}
          selectedNodeId={selectedNodeId}
          onSelect={onSelect}
          loadChildren={loadChildren}
        />
      ))}
    </div>
  )
}

export default function App() {
  const [health, setHealth] = useState('…')
  const [ready, setReady] = useState('…')
  const [devices, setDevices] = useState([])
  const [deviceId, setDeviceId] = useState('')
  const [tags, setTags] = useState([])
  const [selectedNode, setSelectedNode] = useState(null)
  const [rootKids, setRootKids] = useState([])
  const [err, setErr] = useState('')
  const [treeKey, setTreeKey] = useState(0)
  const [showAdd, setShowAdd] = useState(false)
  const [editForm, setEditForm] = useState({
    id: '', endpoint: 'opc.tcp://10.14.10.16:4840', username: '', password: '', security: 'None',
  })
  const [importBusy, setImportBusy] = useState(false)
  const [importMsg, setImportMsg] = useState('')
  const [replaceTags, setReplaceTags] = useState(false)
  const [filter, setFilter] = useState('')

  const selectedDevice = useMemo(
    () => devices.find((d) => d.id === deviceId) || null,
    [devices, deviceId],
  )

  const loadChildren = useCallback(async (nodeId) => {
    if (!deviceId) return []
    return getJSON(
      `/api/v1/browse?device_id=${encodeURIComponent(deviceId)}&node_id=${encodeURIComponent(nodeId)}`,
    )
  }, [deviceId])

  const refreshDevices = async () => {
    const [h, rd, devs] = await Promise.all([
      fetch('/healthz').then((r) => r.text()),
      fetch('/readyz').then(async (r) => ({ ok: r.ok, text: await r.text() })),
      getJSON('/api/v1/devices'),
    ])
    setHealth(h.trim())
    setReady(rd.ok ? 'ready' : rd.text)
    setDevices(devs)
    if (!deviceId && devs[0]) setDeviceId(devs[0].id)
    if (deviceId && !devs.some((d) => d.id === deviceId) && devs[0]) {
      setDeviceId(devs[0].id)
    }
  }

  const refreshTags = async (dev = deviceId) => {
    if (!dev) {
      setTags([])
      return
    }
    const t = await getJSON(`/api/v1/tags?device_id=${encodeURIComponent(dev)}`)
    setTags(t)
  }

  const reloadTree = async () => {
    if (!deviceId) {
      setRootKids([])
      return
    }
    const kids = await loadChildren(ROOT_ID)
    setRootKids(kids)
    setTreeKey((k) => k + 1)
    setSelectedNode({ node_id: ROOT_ID, browse_name: 'Root', is_leaf: false })
  }

  useEffect(() => {
    refreshDevices().catch((e) => setErr(String(e.message || e)))
    const t = setInterval(() => {
      refreshDevices().catch(() => {})
      refreshTags().catch(() => {})
    }, 4000)
    return () => clearInterval(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (!deviceId) return
    setErr('')
    Promise.all([reloadTree(), refreshTags(deviceId)]).catch((e) => setErr(String(e.message || e)))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [deviceId])

  const saveDevice = async (e) => {
    e.preventDefault()
    setErr('')
    try {
      const exists = devices.some((d) => d.id === editForm.id)
      const r = await fetch(
        exists ? `/api/v1/devices/${encodeURIComponent(editForm.id)}` : '/api/v1/devices',
        {
          method: exists ? 'PUT' : 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(editForm),
        },
      )
      if (!r.ok) throw new Error(await r.text())
      setShowAdd(false)
      await refreshDevices()
      setDeviceId(editForm.id)
    } catch (ex) {
      setErr(String(ex.message || ex))
    }
  }

  const removeDevice = async () => {
    if (!deviceId) return
    if (!window.confirm(`Remove server "${deviceId}"?`)) return
    setErr('')
    try {
      const r = await fetch(`/api/v1/devices/${encodeURIComponent(deviceId)}`, { method: 'DELETE' })
      if (!r.ok && r.status !== 204) throw new Error(await r.text())
      setDeviceId('')
      await refreshDevices()
    } catch (ex) {
      setErr(String(ex.message || ex))
    }
  }

  const importExcel = async (file) => {
    if (!deviceId || !file) return
    setImportBusy(true)
    setImportMsg('')
    try {
      const fd = new FormData()
      fd.append('file', file)
      const q = replaceTags ? '?replace=1' : ''
      const r = await fetch(`/api/v1/devices/${encodeURIComponent(deviceId)}/tags/import${q}`, {
        method: 'POST',
        body: fd,
      })
      const text = await r.text()
      if (!r.ok) throw new Error(text)
      const data = JSON.parse(text)
      setImportMsg(`Import: +${data.added} / ~${data.updated} (total ${data.total})`)
      await refreshTags(deviceId)
      await refreshDevices()
    } catch (ex) {
      setErr(String(ex.message || ex))
    } finally {
      setImportBusy(false)
    }
  }

  const doExpand = async () => {
    if (!selectedNode || selectedNode.is_leaf) return
    setErr('')
    try {
      const rows = await fetch('/api/v1/expand', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          device_id: deviceId,
          node_id: selectedNode.node_id,
          parent_tag_id: selectedNode.browse_name || 'udt',
          max_depth: 6,
        }),
      }).then(async (r) => {
        if (!r.ok) throw new Error(await r.text())
        return r.json()
      })
      setImportMsg(`Expand found ${rows.length} leaf nodes`)
    } catch (ex) {
      setErr(String(ex.message || ex))
    }
  }

  const guessType = (name) => {
    const n = String(name || '').toLowerCase()
    if (n.startsWith('b') || n.includes('bool') || n.includes('auto') || n.endsWith('_run')) return 'bool'
    if (n.startsWith('s') && (n.includes('unit') || n.includes('name') || n.includes('text'))) return 'string'
    if (n.startsWith('i') || n.includes('count')) return 'int64'
    return 'float64'
  }

  const isMonitored = (nodeId) => tags.some((t) => t.tag.node_id === nodeId)

  const monitorSelectedNode = async () => {
    if (!deviceId || !selectedNode?.is_leaf) return
    setErr('')
    const id = String(selectedNode.browse_name || selectedNode.node_id).replace(/\s+/g, '_')
    try {
      const r = await fetch(`/api/v1/devices/${encodeURIComponent(deviceId)}/tags`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          id,
          node_id: selectedNode.node_id,
          datatype: guessType(selectedNode.browse_name),
          enabled: true,
          interval_ms: 1000,
        }),
      })
      if (!r.ok) throw new Error(await r.text())
      setImportMsg(`Monitoring ${id} → DB`)
      await refreshTags(deviceId)
      await refreshDevices()
    } catch (ex) {
      setErr(String(ex.message || ex))
    }
  }

  const unmonitorTag = async (tagId) => {
    if (!deviceId || !tagId) return
    setErr('')
    try {
      const r = await fetch(
        `/api/v1/devices/${encodeURIComponent(deviceId)}/tags/${encodeURIComponent(tagId)}`,
        { method: 'DELETE' },
      )
      if (!r.ok && r.status !== 204) throw new Error(await r.text())
      await refreshTags(deviceId)
      await refreshDevices()
    } catch (ex) {
      setErr(String(ex.message || ex))
    }
  }

  const setTagEnabled = async (tag, enabled) => {
    if (!deviceId) return
    setErr('')
    try {
      const r = await fetch(
        `/api/v1/devices/${encodeURIComponent(deviceId)}/tags/${encodeURIComponent(tag.id)}`,
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ ...tag, enabled }),
        },
      )
      if (!r.ok) throw new Error(await r.text())
      await refreshTags(deviceId)
    } catch (ex) {
      setErr(String(ex.message || ex))
    }
  }

  const monitored = useMemo(() => {
    const q = filter.trim().toLowerCase()
    if (!q) return tags
    return tags.filter(
      (t) =>
        t.tag.id.toLowerCase().includes(q) ||
        t.tag.node_id.toLowerCase().includes(q),
    )
  }, [tags, filter])

  return (
    <div className="app ua">
      <header className="top">
        <div>
          <h1 className="brand">Level2</h1>
          <p className="sub">Connectivity · UaExpert-style project & address space</p>
        </div>
        <div className="meta">
          <span className="pill ok">health {health}</span>
          <span className={`pill ${ready === 'ready' ? 'ok' : 'bad'}`}>{ready}</span>
        </div>
      </header>

      {err && <p className="err">{err}</p>}

      <div className="ua-layout">
        <section className="panel address">
          <div className="panel-head">
            <h2>Address Space</h2>
            <div className="row tight">
              <button type="button" className="secondary" onClick={reloadTree} disabled={!deviceId}>
                Refresh
              </button>
              <button
                type="button"
                className="secondary"
                onClick={doExpand}
                disabled={!selectedNode || selectedNode.is_leaf}
              >
                Expand
              </button>
            </div>
          </div>
          {!deviceId && <p className="muted">Select a server in Project</p>}
          {deviceId && (
            <div className="address-tree" key={`${deviceId}-${treeKey}`}>
              <button
                type="button"
                className={`tree-row${selectedNode?.node_id === ROOT_ID ? ' selected' : ''}`}
                onClick={() => setSelectedNode({ node_id: ROOT_ID, browse_name: 'Root', is_leaf: false })}
              >
                <span className="chev">▾</span>
                <span className="icon fold" />
                <span className="name">Root</span>
              </button>
              {rootKids.map((n) => (
                <TreeNode
                  key={n.node_id}
                  node={n}
                  depth={1}
                  deviceId={deviceId}
                  selectedNodeId={selectedNode?.node_id}
                  onSelect={setSelectedNode}
                  loadChildren={loadChildren}
                />
              ))}
            </div>
          )}
          {selectedNode && (
            <div className="node-detail">
              <div className="mono">{selectedNode.browse_name}</div>
              <div className="mono small muted">{selectedNode.node_id}</div>
              <div className="muted small">{selectedNode.is_leaf ? 'Variable' : 'Object / folder'}</div>
              {selectedNode.is_leaf && deviceId && (
                <div className="row tight" style={{ marginTop: 8 }}>
                  <button type="button" onClick={monitorSelectedNode}>
                    {isMonitored(selectedNode.node_id) ? 'Update in DB list' : 'Write to DB (monitor)'}
                  </button>
                  {isMonitored(selectedNode.node_id) && (
                    <button
                      type="button"
                      className="secondary"
                      onClick={() => {
                        const t = tags.find((x) => x.tag.node_id === selectedNode.node_id)
                        if (t) unmonitorTag(t.tag.id)
                      }}
                    >
                      Stop writing
                    </button>
                  )}
                </div>
              )}
              <p className="hint">
                Address Space is browse-only. Only Monitored tags are polled and stored in TimescaleDB.
              </p>
            </div>
          )}
        </section>

        <div className="right-col">
          <section className="panel project">
            <div className="panel-head">
              <h2>Project</h2>
              <div className="row tight">
                <button
                  type="button"
                  onClick={() => {
                    setEditForm({
                      id: '',
                      endpoint: 'opc.tcp://10.14.10.16:4840',
                      username: '',
                      password: '',
                      security: 'None',
                    })
                    setShowAdd(true)
                  }}
                >
                  + Server
                </button>
                <button type="button" className="secondary" onClick={removeDevice} disabled={!deviceId}>
                  Remove
                </button>
              </div>
            </div>
            <div className="project-tree">
              <div className="proj-label">Servers</div>
              {devices.map((d) => (
                <button
                  key={d.id}
                  type="button"
                  className={`server-row${d.id === deviceId ? ' selected' : ''}`}
                  onClick={() => setDeviceId(d.id)}
                  onDoubleClick={() => {
                    setEditForm({
                      id: d.id,
                      endpoint: d.endpoint,
                      username: d.username || '',
                      password: '',
                      security: d.security || 'None',
                    })
                    setShowAdd(true)
                  }}
                >
                  <span className={`dot ${d.connected ? 'on' : 'off'}`} />
                  <span className="mono">{d.id}</span>
                  <span className="mono small muted">{d.endpoint}</span>
                </button>
              ))}
              {devices.length === 0 && <p className="muted">No servers — add one</p>}
            </div>
            {showAdd && (
              <form className="device-form" onSubmit={saveDevice}>
                <h3>{devices.some((d) => d.id === editForm.id) ? 'Edit server' : 'Add server'}</h3>
                <label>Id<input required value={editForm.id} onChange={(e) => setEditForm({ ...editForm, id: e.target.value })} disabled={devices.some((d) => d.id === editForm.id)} /></label>
                <label>Endpoint<input required value={editForm.endpoint} onChange={(e) => setEditForm({ ...editForm, endpoint: e.target.value })} /></label>
                <label>Username<input value={editForm.username} onChange={(e) => setEditForm({ ...editForm, username: e.target.value })} /></label>
                <label>Password<input type="password" value={editForm.password} onChange={(e) => setEditForm({ ...editForm, password: e.target.value })} /></label>
                <label>Security
                  <select value={editForm.security} onChange={(e) => setEditForm({ ...editForm, security: e.target.value })}>
                    <option>None</option>
                    <option>Sign</option>
                    <option>SignAndEncrypt</option>
                  </select>
                </label>
                <div className="row">
                  <button type="submit">Save</button>
                  <button type="button" className="secondary" onClick={() => setShowAdd(false)}>Cancel</button>
                </div>
              </form>
            )}
            {selectedDevice && (
              <div className="import-box">
                <div className="muted">Excel import → monitored tags</div>
                <label className="check">
                  <input type="checkbox" checked={replaceTags} onChange={(e) => setReplaceTags(e.target.checked)} />
                  replace all tags
                </label>
                <input
                  type="file"
                  accept=".xlsx"
                  disabled={importBusy}
                  onChange={(e) => {
                    const f = e.target.files?.[0]
                    e.target.value = ''
                    if (f) importExcel(f)
                  }}
                />
                {importMsg && <div className="good small">{importMsg}</div>}
              </div>
            )}
          </section>

          <section className="panel monitored">
            <div className="panel-head">
              <h2>Monitored tags → DB</h2>
              <input
                className="search"
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
                placeholder="filter"
              />
            </div>
            <p className="hint">
              Only this list is polled and written to TimescaleDB. Disable stops writes without deleting.
            </p>
            <div className="table-wrap compact">
              <table>
                <thead>
                  <tr>
                    <th>Tag</th>
                    <th>Value</th>
                    <th>On</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {monitored.map((t) => (
                    <tr key={t.tag.id} className={t.tag.enabled ? '' : 'dim'}>
                      <td>
                        <div className="mono">{t.tag.id}</div>
                        <div className="mono small muted">{t.tag.node_id}</div>
                      </td>
                      <td className="mono">{formatValue(t.sample)}</td>
                      <td>
                        <input
                          type="checkbox"
                          checked={!!t.tag.enabled}
                          onChange={(e) => setTagEnabled(t.tag, e.target.checked)}
                          title="enabled = write to DB"
                        />
                      </td>
                      <td>
                        <button
                          type="button"
                          className="secondary small-btn"
                          onClick={() => unmonitorTag(t.tag.id)}
                        >
                          Remove
                        </button>
                      </td>
                    </tr>
                  ))}
                  {monitored.length === 0 && (
                    <tr><td colSpan={4} className="muted">No monitored tags</td></tr>
                  )}
                </tbody>
              </table>
            </div>
          </section>
        </div>
      </div>
    </div>
  )
}
