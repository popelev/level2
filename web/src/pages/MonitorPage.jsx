import { useEffect, useMemo, useState } from 'react'
import TreeNode from '../components/TreeNode.jsx'
import {
  ROOT_ID,
  getJSON,
  postTagsBulk,
  sanitizeId,
  upsertDeviceTag,
  deleteDeviceTag,
} from '../api.js'
import {
  allocateUniqueTagIds,
  normalizePath,
  tagIdFromBrowse,
} from '../tagTree.js'
import useBrowseTree from '../hooks/useBrowseTree.js'

export default function MonitorPage({ devices, onError, onDevicesChanged, initialDeviceId }) {
  const [deviceId, setDeviceId] = useState('')
  const [tags, setTags] = useState([])
  const [msg, setMsg] = useState('')
  const [bulkBusy, setBulkBusy] = useState(false)

  const {
    selectedNode,
    setSelectedNode,
    selectedPath,
    setSelectedPath,
    rootKids,
    treeKey,
    expandingId,
    loadChildren,
    reloadTree,
    resetSelectionCaches,
    isAddrChecked,
    onToggleAddrCheck,
    collectLeavesFromSelection,
    hasSelection,
    selLabel,
    clearSelection,
    addrChecked,
  } = useBrowseTree({ deviceId, onError, setMsg })

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

  const refreshTags = async (dev = deviceId) => {
    if (!dev) {
      setTags([])
      return
    }
    setTags(await getJSON(`/api/v1/tags?device_id=${encodeURIComponent(dev)}`))
  }

  useEffect(() => {
    if (!deviceId) return
    onError('')
    resetSelectionCaches()
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

  const isMonitored = (nodeId) => monitoredIds.has(nodeId)

  const unmonitorTags = async (tagIds) => {
    if (!deviceId || !tagIds?.length) return
    onError('')
    try {
      for (const tagId of tagIds) {
        await deleteDeviceTag(deviceId, tagId)
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
      await upsertDeviceTag(deviceId, {
        id,
        node_id: selectedNode.node_id,
        path: folderPath,
        // Prefer OPC datatype from browse; empty → server resolves via Attribute Read.
        datatype: selectedNode.datatype || '',
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
      const leaves = await collectLeavesFromSelection((m) => setMsg(m))
      const { tags: payload, skippedDuplicates, renamed } = allocateUniqueTagIds(
        leaves.map((item) => ({
          ...item,
          // Empty datatype → server Attribute Read; never invent via name heuristics here.
          datatype: item.datatype || '',
        })),
        sanitizeId,
      )
      setMsg(`Writing ${payload.length} tag(s)…`)
      const result = await postTagsBulk(deviceId, payload)
      const wrote = result.wrote ?? payload.length
      const skipped = (result.skipped_duplicates ?? 0) + skippedDuplicates
      const errN = Array.isArray(result.errors) ? result.errors.length : 0
      const extra = renamed ? ` (${renamed} id(s) disambiguated)` : ''
      setMsg(`Wrote ${wrote}, skipped ${skipped} duplicates, errors ${errN}${extra}`)
      if (errN && result.errors?.length) {
        onError(result.errors.slice(0, 5).join('; '))
      }
      clearSelection({ clearMsg: false })
      await refreshTags(deviceId)
      await onDevicesChanged()
    } catch (ex) {
      onError(String(ex.message || ex))
    } finally {
      setBulkBusy(false)
    }
  }

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
              onClick={clearSelection}
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
