import { useCallback, useEffect, useMemo, useState } from 'react'
import TreeNode from '../components/TreeNode.jsx'
import TagTreeTable from '../components/TagTreeTable.jsx'
import { ROOT_ID, getJSON, guessType, sanitizeId } from '../api.js'
import { normalizePath } from '../tagTree.js'

export default function MonitorPage({ devices, onError, onDevicesChanged }) {
  const [deviceId, setDeviceId] = useState('')
  const [tags, setTags] = useState([])
  const [selectedNode, setSelectedNode] = useState(null)
  const [selectedPath, setSelectedPath] = useState('')
  const [rootKids, setRootKids] = useState([])
  const [treeKey, setTreeKey] = useState(0)
  const [filter, setFilter] = useState('')
  const [importBusy, setImportBusy] = useState(false)
  const [msg, setMsg] = useState('')
  const [replaceTags, setReplaceTags] = useState(false)
  const [addrChecked, setAddrChecked] = useState(() => new Map()) // node_id -> { browse_name, node_id, path, datatype? }
  const [expandingId, setExpandingId] = useState('')
  const [bulkBusy, setBulkBusy] = useState(false)
  const [dbSelected, setDbSelected] = useState(() => new Set())

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
    setSelectedPath('')
    setAddrChecked(new Map())
  }

  useEffect(() => {
    if (!deviceId) return
    onError('')
    setAddrChecked(new Map())
    setDbSelected(new Set())
    Promise.all([reloadTree(), refreshTags(deviceId)]).catch((e) => onError(String(e.message || e)))
    const t = setInterval(() => {
      refreshTags(deviceId).catch(() => {})
    }, 3000)
    return () => clearInterval(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [deviceId])

  const monitoredIds = useMemo(
    () => new Set(tags.map((t) => t.tag.node_id)),
    [tags],
  )

  const checkedIds = useMemo(() => new Set(addrChecked.keys()), [addrChecked])

  const isMonitored = (nodeId) => monitoredIds.has(nodeId)

  const upsertTag = async (body) => {
    const r = await fetch(`/api/v1/devices/${encodeURIComponent(deviceId)}/tags`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!r.ok) throw new Error(await r.text())
  }

  const monitorSelectedNode = async () => {
    if (!deviceId || !selectedNode?.is_leaf) return
    onError('')
    const id = sanitizeId(selectedNode.browse_name || selectedNode.node_id)
    const folderPath = normalizePath(
      selectedPath.includes('/')
        ? selectedPath.slice(0, selectedPath.lastIndexOf('/'))
        : '',
    )
    try {
      await upsertTag({
        id,
        node_id: selectedNode.node_id,
        path: folderPath,
        datatype: guessType(selectedNode.browse_name),
        enabled: true,
        interval_ms: 1000,
      })
      setMsg(`Monitoring ${id} → DB`)
      await refreshTags(deviceId)
      await onDevicesChanged()
    } catch (ex) {
      onError(String(ex.message || ex))
    }
  }

  const writeCheckedToDB = async () => {
    if (!deviceId || addrChecked.size === 0) return
    setBulkBusy(true)
    onError('')
    try {
      let n = 0
      for (const item of addrChecked.values()) {
        if (item.folder) continue
        const id = sanitizeId(item.tag_id || item.browse_name || item.node_id)
        const folderPath = normalizePath(
          item.path.includes('/')
            ? item.path.slice(0, item.path.lastIndexOf('/'))
            : '',
        )
        await upsertTag({
          id,
          node_id: item.node_id,
          path: folderPath,
          datatype: item.datatype || guessType(item.browse_name),
          enabled: true,
          interval_ms: 1000,
        })
        n++
      }
      setMsg(`Wrote ${n} tag(s) → DB`)
      setAddrChecked(new Map())
      await refreshTags(deviceId)
      await onDevicesChanged()
    } catch (ex) {
      onError(String(ex.message || ex))
    } finally {
      setBulkBusy(false)
    }
  }

  const onToggleAddrCheck = async (node, path, checked) => {
    onError('')
    if (node.is_leaf) {
      setAddrChecked((prev) => {
        const next = new Map(prev)
        if (checked) {
          next.set(node.node_id, {
            browse_name: node.browse_name,
            node_id: node.node_id,
            path,
          })
        } else {
          next.delete(node.node_id)
        }
        return next
      })
      return
    }
    // Folder: expand all leaves via API
    if (!checked) {
      setExpandingId(node.node_id)
      try {
        const expanded = await getJSONExpand(deviceId, node.node_id, node.browse_name)
        setAddrChecked((prev) => {
          const next = new Map(prev)
          next.delete(node.node_id)
          for (const t of expanded) next.delete(t.node_id)
          return next
        })
      } catch (ex) {
        // still clear folder key
        setAddrChecked((prev) => {
          const next = new Map(prev)
          next.delete(node.node_id)
          return next
        })
        onError(String(ex.message || ex))
      } finally {
        setExpandingId('')
      }
      return
    }
    setExpandingId(node.node_id)
    try {
      const expanded = await getJSONExpand(deviceId, node.node_id, node.browse_name)
      setAddrChecked((prev) => {
        const next = new Map(prev)
        next.set(node.node_id, {
          browse_name: node.browse_name,
          node_id: node.node_id,
          path,
          folder: true,
        })
        for (const t of expanded) {
          const bp = normalizePath(String(t.browse_path || '').replace(/\./g, '/'))
          const name = leafName(t)
          const leafPath = bp || normalizePath(path ? `${path}/${name}` : name)
          next.set(t.node_id, {
            browse_name: name,
            node_id: t.node_id,
            path: leafPath,
            datatype: t.datatype,
            tag_id: t.id,
          })
        }
        return next
      })
    } catch (ex) {
      onError(String(ex.message || ex))
    } finally {
      setExpandingId('')
    }
  }

  const unmonitorTags = async (tagIds) => {
    if (!deviceId || !tagIds?.length) return
    onError('')
    try {
      for (const tagId of tagIds) {
        const r = await fetch(
          `/api/v1/devices/${encodeURIComponent(deviceId)}/tags/${encodeURIComponent(tagId)}`,
          { method: 'DELETE' },
        )
        if (!r.ok && r.status !== 204) throw new Error(await r.text())
      }
      setDbSelected((prev) => {
        const next = new Set(prev)
        for (const id of tagIds) next.delete(id)
        return next
      })
      await refreshTags(deviceId)
      await onDevicesChanged()
    } catch (ex) {
      onError(String(ex.message || ex))
    }
  }

  const setTagsEnabled = async (tagList, enabled) => {
    if (!deviceId || !tagList?.length) return
    onError('')
    try {
      for (const tag of tagList) {
        const r = await fetch(
          `/api/v1/devices/${encodeURIComponent(deviceId)}/tags/${encodeURIComponent(tag.id)}`,
          {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ ...tag, enabled }),
          },
        )
        if (!r.ok) throw new Error(await r.text())
      }
      await refreshTags(deviceId)
    } catch (ex) {
      onError(String(ex.message || ex))
    }
  }

  const importExcel = async (file) => {
    if (!deviceId || !file) return
    if (replaceTags && !window.confirm('Replace all monitored tags for this server?')) return
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
      const errN = (data.errors && data.errors.length) || 0
      setMsg(
        `Import: +${data.added} / ~${data.updated} (total ${data.total})` +
          (errN ? ` · ${errN} row error(s)` : ''),
      )
      if (errN) onError(data.errors.slice(0, 5).join('; '))
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
        t.tag.node_id.toLowerCase().includes(q) ||
        String(t.tag.path || '').toLowerCase().includes(q),
    )
  }, [tags, filter])

  const leafCheckedCount = useMemo(() => {
    let n = 0
    for (const v of addrChecked.values()) {
      if (!v.folder) n++
    }
    return n
  }, [addrChecked])

  const onDbToggleSelect = (ids, checked) => {
    setDbSelected((prev) => {
      const next = new Set(prev)
      for (const id of ids) {
        if (checked) next.add(id)
        else next.delete(id)
      }
      return next
    })
  }

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
              <div
                className={`tree-row${selectedNode?.node_id === ROOT_ID ? ' selected' : ''}`}
              >
                <span className="tree-check-spacer" />
                <button
                  type="button"
                  className="tree-row-btn"
                  onClick={() => {
                    setSelectedNode({ node_id: ROOT_ID, browse_name: 'Root', is_leaf: false })
                    setSelectedPath('')
                  }}
                >
                  <span className="chev">▾</span>
                  <span className="icon fold" />
                  <span className="name">Root</span>
                </button>
              </div>
              {rootKids.map((n) => (
                <TreeNode
                  key={n.node_id}
                  node={n}
                  depth={1}
                  pathPrefix=""
                  selectedNodeId={selectedNode?.node_id}
                  onSelect={(node, path) => {
                    setSelectedNode(node)
                    setSelectedPath(path || '')
                  }}
                  loadChildren={loadChildren}
                  checkedIds={checkedIds}
                  onToggleCheck={onToggleAddrCheck}
                  expandingId={expandingId}
                  monitoredIds={monitoredIds}
                />
              ))}
            </div>
            <div className="sel-bar">
              <span className="muted small">{leafCheckedCount} selected</span>
              <button type="button" disabled={!leafCheckedCount || bulkBusy} onClick={writeCheckedToDB}>
                Write selected to DB
              </button>
              <button
                type="button"
                className="secondary"
                disabled={!addrChecked.size}
                onClick={() => setAddrChecked(new Map())}
              >
                Clear
              </button>
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
                          if (t) unmonitorTags([t.tag.id])
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
            {dbSelected.size > 0 && (
              <div className="sel-bar">
                <span className="muted small">{dbSelected.size} selected</span>
                <button
                  type="button"
                  className="secondary"
                  onClick={() => {
                    const list = tags.filter((t) => dbSelected.has(t.tag.id)).map((t) => t.tag)
                    setTagsEnabled(list, true)
                  }}
                >
                  Enable
                </button>
                <button
                  type="button"
                  className="secondary"
                  onClick={() => {
                    const list = tags.filter((t) => dbSelected.has(t.tag.id)).map((t) => t.tag)
                    setTagsEnabled(list, false)
                  }}
                >
                  Disable
                </button>
                <button
                  type="button"
                  className="secondary"
                  onClick={() => {
                    if (window.confirm(`Remove ${dbSelected.size} tag(s)?`)) {
                      unmonitorTags([...dbSelected])
                    }
                  }}
                >
                  Remove selected
                </button>
                <button type="button" className="secondary" onClick={() => setDbSelected(new Set())}>
                  Clear
                </button>
              </div>
            )}
            <div className="table-wrap compact tall">
              <TagTreeTable
                tagValues={monitored}
                selected={dbSelected}
                onToggleSelect={onDbToggleSelect}
                onSetEnabled={setTagsEnabled}
                onRemove={unmonitorTags}
              />
            </div>
          </section>
        </div>
      )}
    </div>
  )
}

async function getJSONExpand(deviceId, nodeId, parentTagId) {
  const r = await fetch('/api/v1/expand', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      device_id: deviceId,
      node_id: nodeId,
      parent_tag_id: parentTagId || '',
      max_depth: 8,
    }),
  })
  if (!r.ok) throw new Error(await r.text())
  return r.json()
}

function leafName(expanded) {
  if (expanded.browse_path) {
    const parts = String(expanded.browse_path).split('.')
    return parts[parts.length - 1] || expanded.id
  }
  if (expanded.id && expanded.id.includes('.')) {
    const parts = expanded.id.split('.')
    return parts[parts.length - 1]
  }
  return expanded.id
}
