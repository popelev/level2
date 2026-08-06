import { useCallback, useEffect, useMemo, useState } from 'react'
import TreeNode from '../components/TreeNode.jsx'
import { ROOT_ID, getJSON, guessType, sanitizeId } from '../api.js'
import { leafPathUnderPrefix, normalizePath, tagIdFromBrowse } from '../tagTree.js'

export default function MonitorPage({ devices, onError, onDevicesChanged, initialDeviceId }) {
  const [deviceId, setDeviceId] = useState('')
  const [tags, setTags] = useState([])
  const [selectedNode, setSelectedNode] = useState(null)
  const [selectedPath, setSelectedPath] = useState('')
  const [rootKids, setRootKids] = useState([])
  const [treeKey, setTreeKey] = useState(0)
  const [msg, setMsg] = useState('')
  const [addrChecked, setAddrChecked] = useState(() => new Map())
  const [expandingId, setExpandingId] = useState('')
  const [bulkBusy, setBulkBusy] = useState(false)

  useEffect(() => {
    if (initialDeviceId && devices.some((d) => d.id === initialDeviceId)) {
      setDeviceId(initialDeviceId)
      return
    }
    if (!deviceId && devices[0]) setDeviceId(devices[0].id)
    if (deviceId && devices.length && !devices.some((d) => d.id === deviceId)) {
      setDeviceId(devices[0]?.id || '')
    }
  }, [devices, deviceId, initialDeviceId])

  const currentDevice = devices.find((d) => d.id === deviceId)

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
    Promise.all([reloadTree(), refreshTags(deviceId)]).catch((e) => onError(String(e.message || e)))
    const t = setInterval(() => {
      refreshTags(deviceId).catch(() => {})
    }, 5000)
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
      await refreshTags(deviceId)
      await onDevicesChanged()
    } catch (ex) {
      onError(String(ex.message || ex))
    }
  }

  const monitorSelectedNode = async () => {
    if (!deviceId || !selectedNode?.is_leaf) return
    onError('')
    const folderPath = normalizePath(
      selectedPath.includes('/')
        ? selectedPath.slice(0, selectedPath.lastIndexOf('/'))
        : '',
    )
    const id = tagIdFromBrowse(folderPath, selectedNode.browse_name, sanitizeId)
    try {
      await upsertTag({
        id,
        node_id: selectedNode.node_id,
        path: folderPath,
        datatype: guessType(selectedNode.browse_name),
        enabled: true,
        interval_ms: 1000,
      })
      setMsg(`Added ${id} to DB write list`)
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
        const folderPath = normalizePath(
          item.path.includes('/')
            ? item.path.slice(0, item.path.lastIndexOf('/'))
            : '',
        )
        const id = tagIdFromBrowse(
          folderPath,
          item.browse_name || item.tag_id,
          sanitizeId,
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
      setMsg(`Wrote ${n} tag(s) to DB write list`)
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
          const name = leafName(t)
          const rel = t.browse_path || name
          const leafPath = leafPathUnderPrefix(path, rel)
          next.set(t.node_id, {
            browse_name: name,
            node_id: t.node_id,
            path: leafPath,
            datatype: t.datatype,
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

  const leafCheckedCount = useMemo(() => {
    let n = 0
    for (const v of addrChecked.values()) {
      if (!v.folder) n++
    }
    return n
  }, [addrChecked])

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2>Address Space</h2>
          <p className="muted">Browse OPC UA and add tags to the DB write list</p>
        </div>
        <label className="server-pick">
          Server
          <select
            value={deviceId}
            onChange={(e) => {
              const id = e.target.value
              setDeviceId(id)
              location.hash = `#/monitor?device=${encodeURIComponent(id)}`
            }}
          >
            {devices.map((d) => (
              <option key={d.id} value={d.id}>{d.id}</option>
            ))}
          </select>
        </label>
      </div>

      {!deviceId && <p className="muted">Create a server on the Servers page first</p>}

      {deviceId && currentDevice && !currentDevice.connected && (
        <p className="err">Server “{deviceId}” is disconnected — browse may be unavailable</p>
      )}

      {msg && <p className="good small">{msg}</p>}

      {deviceId && (
        <section className="panel address address-only">
          <div className="panel-head">
            <h3>Tree</h3>
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
