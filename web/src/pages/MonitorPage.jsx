import { useCallback, useEffect, useMemo, useState } from 'react'
import TreeNode from '../components/TreeNode.jsx'
import { ROOT_ID, formatValue, getJSON, guessType } from '../api.js'

export default function MonitorPage({ devices, onError, onDevicesChanged }) {
  const [deviceId, setDeviceId] = useState('')
  const [tags, setTags] = useState([])
  const [selectedNode, setSelectedNode] = useState(null)
  const [rootKids, setRootKids] = useState([])
  const [treeKey, setTreeKey] = useState(0)
  const [filter, setFilter] = useState('')
  const [importBusy, setImportBusy] = useState(false)
  const [msg, setMsg] = useState('')
  const [replaceTags, setReplaceTags] = useState(false)

  useEffect(() => {
    if (!deviceId && devices[0]) setDeviceId(devices[0].id)
    if (deviceId && devices.length && !devices.some((d) => d.id === deviceId)) {
      setDeviceId(devices[0]?.id || '')
    }
  }, [devices, deviceId])

  const loadChildren = useCallback(async (nodeId) => {
    if (!deviceId) return []
    return getJSON(
      `/api/v1/browse?device_id=${encodeURIComponent(deviceId)}&node_id=${encodeURIComponent(nodeId)}`,
    )
  }, [deviceId])

  const refreshTags = async (dev = deviceId) => {
    if (!dev) {
      setTags([])
      return
    }
    setTags(await getJSON(`/api/v1/tags?device_id=${encodeURIComponent(dev)}`))
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
    if (!deviceId) return
    onError('')
    Promise.all([reloadTree(), refreshTags(deviceId)]).catch((e) => onError(String(e.message || e)))
    const t = setInterval(() => {
      refreshTags(deviceId).catch(() => {})
    }, 3000)
    return () => clearInterval(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [deviceId])

  const isMonitored = (nodeId) => tags.some((t) => t.tag.node_id === nodeId)

  const monitorSelectedNode = async () => {
    if (!deviceId || !selectedNode?.is_leaf) return
    onError('')
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
      setMsg(`Monitoring ${id} → DB`)
      await refreshTags(deviceId)
      await onDevicesChanged()
    } catch (ex) {
      onError(String(ex.message || ex))
    }
  }

  const unmonitorTag = async (tagId) => {
    if (!deviceId || !tagId) return
    onError('')
    try {
      const r = await fetch(
        `/api/v1/devices/${encodeURIComponent(deviceId)}/tags/${encodeURIComponent(tagId)}`,
        { method: 'DELETE' },
      )
      if (!r.ok && r.status !== 204) throw new Error(await r.text())
      await refreshTags(deviceId)
      await onDevicesChanged()
    } catch (ex) {
      onError(String(ex.message || ex))
    }
  }

  const setTagEnabled = async (tag, enabled) => {
    if (!deviceId) return
    onError('')
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
      onError(String(ex.message || ex))
    }
  }

  const importExcel = async (file) => {
    if (!deviceId || !file) return
    setImportBusy(true)
    setMsg('')
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
      setMsg(`Import: +${data.added} / ~${data.updated} (total ${data.total})`)
      await refreshTags(deviceId)
      await onDevicesChanged()
    } catch (ex) {
      onError(String(ex.message || ex))
    } finally {
      setImportBusy(false)
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
    <div className="page">
      <div className="page-head">
        <div>
          <h2>Monitored tags</h2>
          <p className="muted">Choose nodes to write to TimescaleDB · browse address space · Excel import</p>
        </div>
        <label className="server-pick">
          Server
          <select value={deviceId} onChange={(e) => setDeviceId(e.target.value)}>
            {devices.map((d) => (
              <option key={d.id} value={d.id}>{d.id}</option>
            ))}
          </select>
        </label>
      </div>

      {!deviceId && <p className="muted">Create a server on the Servers page first</p>}

      {deviceId && (
        <div className="monitor-layout">
          <section className="panel address">
            <div className="panel-head">
              <h3>Address Space</h3>
              <button type="button" className="secondary" onClick={reloadTree}>Refresh</button>
            </div>
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
                  selectedNodeId={selectedNode?.node_id}
                  onSelect={setSelectedNode}
                  loadChildren={loadChildren}
                />
              ))}
            </div>
            {selectedNode && (
              <div className="node-detail">
                <div className="mono">{selectedNode.browse_name}</div>
                <div className="mono small muted">{selectedNode.node_id}</div>
                {selectedNode.is_leaf && (
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
              </div>
            )}
          </section>

          <section className="panel monitored">
            <div className="panel-head">
              <h3>DB write list</h3>
              <input
                className="search"
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
                placeholder="filter"
              />
            </div>
            <p className="hint">Only enabled tags in this list are polled and stored.</p>
            <div className="import-box">
              <div className="muted">Excel import</div>
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
              {msg && <div className="good small">{msg}</div>}
            </div>
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
      )}
    </div>
  )
}
