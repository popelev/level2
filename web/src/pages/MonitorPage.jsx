import { startTransition, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import TreeNode from '../components/TreeNode.jsx'
import { ROOT_ID, getJSON, guessType, sanitizeId } from '../api.js'
import {
  allocateUniqueTagIds,
  leafPathUnderPrefix,
  normalizePath,
  tagIdFromBrowse,
} from '../tagTree.js'

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
  const expandCacheRef = useRef(new Map())
  const pendingLeafRef = useRef(null)
  const leafFlushTimer = useRef(0)

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
    expandCacheRef.current = new Map()
  }

  useEffect(() => {
    if (!deviceId) return
    onError('')
    setAddrChecked(new Map())
    expandCacheRef.current = new Map()
    Promise.all([reloadTree(), refreshTags(deviceId)]).catch((e) => onError(String(e.message || e)))
    const t = setInterval(() => {
      refreshTags(deviceId).catch(() => {})
    }, 5000)
    return () => clearInterval(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [deviceId])

  useEffect(() => () => {
    if (leafFlushTimer.current) window.clearTimeout(leafFlushTimer.current)
  }, [])

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
        datatype: selectedNode.datatype || guessType(selectedNode.browse_name),
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
      const leaves = []
      for (const item of addrChecked.values()) {
        if (item.folder) continue
        leaves.push(item)
      }
      const { tags: payload, skippedDuplicates, renamed } = allocateUniqueTagIds(
        leaves.map((item) => ({
          ...item,
          datatype: item.datatype || guessType(item.browse_name),
        })),
        sanitizeId,
      )
      setMsg(`Writing ${payload.length} tag(s)…`)
      const r = await fetch(`/api/v1/devices/${encodeURIComponent(deviceId)}/tags/bulk`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ tags: payload }),
      })
      if (!r.ok) throw new Error(await r.text())
      const result = await r.json()
      const wrote = result.wrote ?? payload.length
      const skipped = (result.skipped_duplicates ?? 0) + skippedDuplicates
      const errN = Array.isArray(result.errors) ? result.errors.length : 0
      const extra = renamed ? ` (${renamed} id(s) disambiguated)` : ''
      setMsg(`Wrote ${wrote}, skipped ${skipped} duplicates, errors ${errN}${extra}`)
      if (errN && result.errors?.length) {
        onError(result.errors.slice(0, 5).join('; '))
      }
      setAddrChecked(new Map())
      await refreshTags(deviceId)
      await onDevicesChanged()
    } catch (ex) {
      onError(String(ex.message || ex))
    } finally {
      setBulkBusy(false)
    }
  }

  const flushLeafChecks = useCallback(() => {
    leafFlushTimer.current = 0
    const pending = pendingLeafRef.current
    if (!pending?.size) return
    pendingLeafRef.current = null
    startTransition(() => {
      setAddrChecked((prev) => {
        const next = new Map(prev)
        for (const [nodeId, op] of pending) {
          if (op === null) next.delete(nodeId)
          else next.set(nodeId, op)
        }
        return next
      })
    })
  }, [])

  const queueLeafCheck = useCallback((nodeId, value) => {
    if (!pendingLeafRef.current) pendingLeafRef.current = new Map()
    pendingLeafRef.current.set(nodeId, value)
    if (leafFlushTimer.current) return
    leafFlushTimer.current = window.setTimeout(flushLeafChecks, 40)
  }, [flushLeafChecks])

  const removeUnderPath = (prev, folderPath, folderNodeId) => {
    const next = new Map(prev)
    next.delete(folderNodeId)
    const prefix = folderPath ? `${folderPath}/` : ''
    for (const [k, v] of prev) {
      if (k === folderNodeId) continue
      if (v.path === folderPath || (prefix && String(v.path || '').startsWith(prefix))) {
        next.delete(k)
      }
    }
    return next
  }

  const onToggleAddrCheck = async (node, path, checked) => {
    onError('')
    if (node.is_leaf) {
      if (checked) {
        queueLeafCheck(node.node_id, {
          browse_name: node.browse_name,
          node_id: node.node_id,
          path,
          datatype: node.datatype || guessType(node.browse_name),
        })
      } else {
        queueLeafCheck(node.node_id, null)
      }
      return
    }

    if (!checked) {
      // Prefer cache; never re-expand just to uncheck.
      const cached = expandCacheRef.current.get(node.node_id)
      startTransition(() => {
        setAddrChecked((prev) => {
          if (cached?.length) {
            const next = new Map(prev)
            next.delete(node.node_id)
            for (const t of cached) next.delete(t.node_id)
            return next
          }
          return removeUnderPath(prev, path, node.node_id)
        })
      })
      return
    }

    setExpandingId(node.node_id)
    setMsg(`Expanding ${node.browse_name}…`)
    try {
      let expanded = expandCacheRef.current.get(node.node_id)
      if (!expanded) {
        expanded = await getJSONExpand(deviceId, node.node_id, node.browse_name)
        expandCacheRef.current.set(node.node_id, expanded)
      }
      startTransition(() => {
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
              datatype: t.datatype || guessType(name),
              tag_id: t.id,
            })
          }
          return next
        })
      })
      setMsg(`Selected ${expanded.length} leaf tag(s) under ${node.browse_name}`)
    } catch (ex) {
      onError(String(ex.message || ex))
      setMsg('')
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
              {bulkBusy ? 'Writing…' : 'Write selected to DB'}
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
      max_depth: 16,
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
