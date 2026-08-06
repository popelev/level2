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
  const [excludedIds, setExcludedIds] = useState(() => new Set())
  const [expandingId, setExpandingId] = useState('')
  const [bulkBusy, setBulkBusy] = useState(false)
  const [expandGen, setExpandGen] = useState(0)
  const expandCacheRef = useRef(new Map())
  const pendingLeafRef = useRef(null)
  const leafFlushTimer = useRef(0)
  const expandAbortRef = useRef(0)

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
    setExcludedIds(new Set())
    expandCacheRef.current = new Map()
    expandAbortRef.current += 1
    setExpandGen((g) => g + 1)
  }

  useEffect(() => {
    if (!deviceId) return
    onError('')
    setAddrChecked(new Map())
    setExcludedIds(new Set())
    expandCacheRef.current = new Map()
    expandAbortRef.current += 1
    setExpandGen((g) => g + 1)
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

  const folderPaths = useMemo(() => {
    const out = []
    for (const v of addrChecked.values()) {
      if (v.folder) out.push(v.path || '')
    }
    return out
  }, [addrChecked])

  const isUnderCheckedFolder = useCallback((path) => {
    for (const fp of folderPaths) {
      if (path === fp || (fp && String(path || '').startsWith(`${fp}/`))) return true
    }
    return false
  }, [folderPaths])

  const isAddrChecked = useCallback((nodeId, path) => {
    if (excludedIds.has(nodeId)) return false
    if (addrChecked.has(nodeId)) return true
    return isUnderCheckedFolder(path)
  }, [addrChecked, excludedIds, isUnderCheckedFolder])

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

  const collectLeavesFromSelection = async (onProgress) => {
    const leaves = []
    const folders = []
    for (const item of addrChecked.values()) {
      if (item.folder) folders.push(item)
      else if (!excludedIds.has(item.node_id)) leaves.push(item)
    }
    for (let i = 0; i < folders.length; i++) {
      const f = folders[i]
      onProgress?.(`Expanding ${f.browse_name} (${i + 1}/${folders.length})…`)
      let expanded = expandCacheRef.current.get(f.node_id)
      if (!expanded) {
        expanded = await getJSONExpand(deviceId, f.node_id, f.browse_name)
        expandCacheRef.current.set(f.node_id, expanded)
        setExpandGen((g) => g + 1)
      }
      for (const t of expanded) {
        if (excludedIds.has(t.node_id)) continue
        const name = leafName(t)
        const rel = t.browse_path || name
        const leafPath = leafPathUnderPrefix(f.path, rel)
        leaves.push({
          browse_name: name,
          node_id: t.node_id,
          path: leafPath,
          datatype: t.datatype || guessType(name),
          tag_id: t.id,
        })
      }
    }
    return leaves
  }

  const writeCheckedToDB = async () => {
    if (!deviceId || addrChecked.size === 0) return
    setBulkBusy(true)
    onError('')
    try {
      const leaves = await collectLeavesFromSelection((m) => setMsg(m))
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
      setExcludedIds(new Set())
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
      setExcludedIds((prev) => {
        let changed = false
        const next = new Set(prev)
        for (const [nodeId, op] of pending) {
          // Leaf queue never adds exclusions (folder-covered unchecks do that
          // directly). Checking a leaf clears a prior exclusion.
          if (op !== null && next.delete(nodeId)) changed = true
        }
        return changed ? next : prev
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

  const prefetchFolderExpand = async (node, path, token) => {
    setExpandingId(node.node_id)
    setMsg(`Loading tags under ${node.browse_name}…`)
    try {
      let expanded = expandCacheRef.current.get(node.node_id)
      if (!expanded) {
        expanded = await getJSONExpand(deviceId, node.node_id, node.browse_name)
        if (token !== expandAbortRef.current) return
        expandCacheRef.current.set(node.node_id, expanded)
        setExpandGen((g) => g + 1)
      }
      if (token !== expandAbortRef.current) return
      setMsg(`Selected ${expanded.length} leaf tag(s) under ${node.browse_name}`)
    } catch (ex) {
      if (token !== expandAbortRef.current) return
      onError(String(ex.message || ex))
      setMsg('')
    } finally {
      if (token === expandAbortRef.current) setExpandingId('')
    }
  }

  const onToggleAddrCheck = async (node, path, checked) => {
    onError('')
    if (node.is_leaf) {
      if (checked) {
        if (isUnderCheckedFolder(path)) {
          // Covered by ancestor folder — clear exclusion only.
          setExcludedIds((prev) => {
            if (!prev.has(node.node_id)) return prev
            const next = new Set(prev)
            next.delete(node.node_id)
            return next
          })
          return
        }
        queueLeafCheck(node.node_id, {
          browse_name: node.browse_name,
          node_id: node.node_id,
          path,
          datatype: node.datatype || guessType(node.browse_name),
        })
      } else if (isUnderCheckedFolder(path)) {
        setExcludedIds((prev) => {
          const next = new Set(prev)
          next.add(node.node_id)
          return next
        })
      } else {
        queueLeafCheck(node.node_id, null)
      }
      return
    }

    if (!checked) {
      expandAbortRef.current += 1
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
        setExcludedIds((prev) => {
          if (!prev.size) return prev
          const next = new Set(prev)
          if (cached?.length) {
            for (const t of cached) next.delete(t.node_id)
          }
          return next
        })
      })
      setExpandingId('')
      setMsg('')
      return
    }

    // Instant folder select — do not wait for OPC expand or put thousands of
    // leaves into React state. Prefetch expand into cache for count / write.
    const token = ++expandAbortRef.current
    startTransition(() => {
      setAddrChecked((prev) => {
        const next = new Map(prev)
        next.set(node.node_id, {
          browse_name: node.browse_name,
          node_id: node.node_id,
          path,
          folder: true,
        })
        return next
      })
    })
    void prefetchFolderExpand(node, path, token)
  }

  const selectionSummary = useMemo(() => {
    let leafN = 0
    let folderN = 0
    let pendingFolders = 0
    const seen = new Set()
    for (const [id, v] of addrChecked) {
      if (v.folder) {
        folderN++
        const cached = expandCacheRef.current.get(id)
        if (!cached) {
          pendingFolders++
          continue
        }
        for (const t of cached) {
          if (excludedIds.has(t.node_id) || seen.has(t.node_id)) continue
          seen.add(t.node_id)
          leafN++
        }
      } else if (!excludedIds.has(id) && !seen.has(id)) {
        seen.add(id)
        leafN++
      }
    }
    return { leafN, folderN, pendingFolders }
    // expandGen bumps when cache fills so count refreshes without putting leaves in state
  }, [addrChecked, excludedIds, expandGen])

  const hasSelection = addrChecked.size > 0
  const selLabel = selectionSummary.pendingFolders
    ? `${selectionSummary.folderN} folder(s) loading…`
    : `${selectionSummary.leafN} selected`

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
                isChecked={isAddrChecked}
                onToggleCheck={onToggleAddrCheck}
                expandingId={expandingId}
                monitoredIds={monitoredIds}
              />
            ))}
          </div>
          <div className="sel-bar">
            <span className="muted small">{selLabel}</span>
            <button type="button" disabled={!hasSelection || bulkBusy} onClick={writeCheckedToDB}>
              {bulkBusy ? 'Writing…' : 'Write selected to DB'}
            </button>
            <button
              type="button"
              className="secondary"
              disabled={!hasSelection}
              onClick={() => {
                expandAbortRef.current += 1
                setAddrChecked(new Map())
                setExcludedIds(new Set())
                setExpandingId('')
                setMsg('')
              }}
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
