import { useCallback, useEffect, useMemo, useState } from 'react'
import TagTreeTable from '../components/TagTreeTable.jsx'
import TagPager from '../components/TagPager.jsx'
import { displayQuality, getJSON, postTagsSync, putDeviceTag } from '../api.js'
import useTagFlags from '../hooks/useTagFlags.js'

const PAGE_SIZE = 50

function isBadTag(tv, opcConnected) {
  const q = displayQuality(tv.sample, { opcConnected, simulate: !!tv.tag?.simulate })
  return q === 'bad' || q === 'stale'
}

export default function DbWriteListPage({ devices, onError, onDevicesChanged, initialDeviceId }) {
  const [deviceId, setDeviceId] = useState('')
  const [tags, setTags] = useState([])
  const [filter, setFilter] = useState('')
  const [badOnly, setBadOnly] = useState(false)
  const [simOnly, setSimOnly] = useState(false)
  const [page, setPage] = useState(1)
  const [dbSelected, setDbSelected] = useState(() => new Set())

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
  const opcConnected = currentDevice ? !!currentDevice.connected : undefined

  const refreshTags = useCallback(async (dev = deviceId) => {
    if (!dev) {
      setTags([])
      return
    }
    setTags(await getJSON(`/api/v1/tags?device_id=${encodeURIComponent(dev)}`))
  }, [deviceId])

  const {
    bulkBusy,
    setBulkBusy,
    msg,
    setMsg,
    setTagsSimulate,
    setTagsWritable,
    bulkSimulate,
    bulkWritable,
    setTagsEnabled,
    unmonitorTags: unmonitorTagsBase,
  } = useTagFlags({
    deviceId,
    tags,
    setTags,
    selectedIds: dbSelected,
    onError,
    refreshTags,
  })

  useEffect(() => {
    if (!deviceId) return
    onError('')
    setDbSelected(new Set())
    refreshTags(deviceId).catch((e) => onError(String(e.message || e)))
    const t = setInterval(() => {
      refreshTags(deviceId).catch(() => {})
    }, 3000)
    return () => clearInterval(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [deviceId])

  const unmonitorTags = async (tagIds) => {
    try {
      await unmonitorTagsBase(tagIds, {
        onRemoved: (ids) => {
          setDbSelected((prev) => {
            const next = new Set(prev)
            for (const id of ids) next.delete(id)
            return next
          })
        },
      })
      await onDevicesChanged()
    } catch {
      // onError already reported inside the hook
    }
  }

  const updateTag = async (tag) => {
    if (!deviceId || !tag?.id) return
    onError('')
    setTags((prev) =>
      prev.map((tv) => (tv.tag.id === tag.id ? { ...tv, tag: { ...tv.tag, ...tag } } : tv)),
    )
    try {
      const saved = await putDeviceTag(deviceId, tag)
      if (saved?.datatype) {
        setTags((prev) =>
          prev.map((tv) =>
            tv.tag.id === tag.id ? { ...tv, tag: { ...tv.tag, ...saved } } : tv,
          ),
        )
      }
      await refreshTags(deviceId)
    } catch (ex) {
      onError(String(ex.message || ex))
      await refreshTags(deviceId).catch(() => {})
    }
  }

  const badTags = useMemo(
    () => tags.filter((tv) => isBadTag(tv, opcConnected)),
    [tags, opcConnected],
  )
  const simTags = useMemo(() => tags.filter((t) => !!t.tag.simulate), [tags])
  const simCount = simTags.length
  const writableCount = useMemo(() => tags.filter((t) => !!t.tag.writable).length, [tags])

  const filtered = useMemo(() => {
    const q = filter.trim().toLowerCase()
    let list = tags
    if (badOnly) list = list.filter((tv) => isBadTag(tv, opcConnected))
    if (simOnly) list = list.filter((t) => !!t.tag.simulate)
    if (!q) return list
    return list.filter(
      (t) =>
        t.tag.id.toLowerCase().includes(q) ||
        t.tag.node_id.toLowerCase().includes(q) ||
        String(t.tag.path || '').toLowerCase().includes(q),
    )
  }, [tags, filter, badOnly, simOnly, opcConnected])

  const sortedFiltered = useMemo(() => {
    const list = [...filtered]
    list.sort((a, b) => {
      const pa = String(a.tag.path || '')
      const pb = String(b.tag.path || '')
      if (pa !== pb) return pa.localeCompare(pb)
      return a.tag.id.localeCompare(b.tag.id)
    })
    return list
  }, [filtered])

  const totalPages = Math.max(1, Math.ceil(sortedFiltered.length / PAGE_SIZE))
  const pageClamped = Math.min(Math.max(1, page), totalPages)

  useEffect(() => {
    setPage(1)
  }, [deviceId, filter, badOnly, simOnly])

  useEffect(() => {
    if (page !== pageClamped) setPage(pageClamped)
  }, [page, pageClamped])

  const pageTags = useMemo(() => {
    const start = (pageClamped - 1) * PAGE_SIZE
    return sortedFiltered.slice(start, pageClamped * PAGE_SIZE)
  }, [sortedFiltered, pageClamped])

  const pageFrom = sortedFiltered.length === 0 ? 0 : (pageClamped - 1) * PAGE_SIZE + 1
  const pageTo = Math.min(pageClamped * PAGE_SIZE, sortedFiltered.length)

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

  const pickDevice = (id) => {
    setDeviceId(id)
    location.hash = `#/db-list?device=${encodeURIComponent(id)}`
  }

  const syncFromOpc = async (tagIds = null, mode = 'all') => {
    if (!deviceId) return
    const all = !tagIds?.length
    const n = all ? tags.length : tagIds.length
    if (n === 0) return
    const label =
      mode === 'bad'
        ? `Sync parameters from OPC for ${n} bad tag(s) by NodeId?`
        : all
          ? `Sync parameters from OPC for all ${n} tag(s) by NodeId?`
          : `Sync parameters from OPC for ${n} selected tag(s) by NodeId?`
    if (!window.confirm(label)) return
    const busyKey = mode === 'bad' ? 'sync-bad' : all ? 'sync-all' : 'sync-sel'
    setBulkBusy(busyKey)
    onError('')
    setMsg('')
    try {
      const data = await postTagsSync(deviceId, tagIds?.length ? tagIds : null)
      setMsg(`Synced ${data.total} tag(s); ${data.updated} datatype(s) updated`)
      await refreshTags(deviceId)
      await onDevicesChanged()
    } catch (ex) {
      onError(String(ex.message || ex))
    } finally {
      setBulkBusy('')
    }
  }

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2>DB write list</h2>
          <p className="muted">Tags polled and stored in TimescaleDB · live values refresh every 3s</p>
        </div>
        <label className="server-pick">
          Server
          <select value={deviceId} onChange={(e) => pickDevice(e.target.value)}>
            {devices.map((d) => (
              <option key={d.id} value={d.id}>{d.id}</option>
            ))}
          </select>
        </label>
      </div>

      {!deviceId && <p className="muted">Create a server on the Servers page first</p>}

      {deviceId && currentDevice && !currentDevice.connected && (
        <p className="err">Server “{deviceId}” is disconnected — live values may be stale (except simulated tags)</p>
      )}

      {deviceId && (
        <section className="panel monitored db-list-panel">
          <div className="panel-head">
            <h3>
              {currentDevice?.tag_count ?? tags.length} tag(s)
              {simCount > 0 && (
                <span className="sim-text small" title="Tags with simulate=true">
                  {' '}· {simCount} sim
                </span>
              )}
              {writableCount > 0 && (
                <span className="muted small" title="Tags with writable=true">
                  {' '}· {writableCount} writable
                </span>
              )}
              {badTags.length > 0 && (
                <span className="badq small"> · {badTags.length} bad</span>
              )}
              {(sortedFiltered.length > PAGE_SIZE || badOnly || simOnly || filter.trim()) && (
                <span className="muted small">
                  {' '}
                  · showing {pageFrom}–{pageTo}
                  {` of ${sortedFiltered.length}`}
                </span>
              )}
            </h3>
            <div className="db-list-filters">
              <label className="diag-check">
                <input
                  type="checkbox"
                  checked={badOnly}
                  onChange={(e) => setBadOnly(e.target.checked)}
                />
                Bad only
              </label>
              <label className="diag-check">
                <input
                  type="checkbox"
                  checked={simOnly}
                  onChange={(e) => setSimOnly(e.target.checked)}
                />
                Simulated only
              </label>
              <input
                className="search"
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
                placeholder="filter"
              />
            </div>
          </div>
          <p className="hint">
            Only enabled tags are polled. Use <strong>Sim</strong> for per-tag mock samples (OPC continues for others).
            <strong> Writable</strong> allows the Write API (default off). Add tags from Address Space or Import / Export.
          </p>
          <div className="sel-bar db-actions">
            <button
              type="button"
              className="secondary"
              disabled={!!bulkBusy || !tags.length}
              onClick={() => syncFromOpc()}
            >
              {bulkBusy === 'sync-all' ? 'Syncing…' : 'Sync all from OPC'}
            </button>
            <button
              type="button"
              className="secondary"
              disabled={!!bulkBusy || badTags.length === 0}
              onClick={() => syncFromOpc(badTags.map((t) => t.tag.id), 'bad')}
              title="Sync datatypes from OPC for tags with bad quality"
            >
              {bulkBusy === 'sync-bad' ? 'Syncing…' : `Sync bad (${badTags.length})`}
            </button>
            <button
              type="button"
              className="secondary"
              disabled={!!bulkBusy || !tags.length}
              onClick={() => bulkSimulate(true, 'all')}
            >
              {bulkBusy === 'sim-on-all' ? '…' : 'Sim all'}
            </button>
            <button
              type="button"
              className="secondary"
              disabled={!!bulkBusy || simCount === 0}
              onClick={() => bulkSimulate(false, 'all')}
            >
              {bulkBusy === 'sim-off-all' ? '…' : 'Unsim all'}
            </button>
            <button
              type="button"
              className="secondary"
              disabled={!!bulkBusy || !tags.length}
              onClick={() => bulkWritable(true, 'all')}
              title="Allow Write API for all tags on this server"
            >
              {bulkBusy === 'wr-on-all' ? '…' : 'Writable all'}
            </button>
            <button
              type="button"
              className="secondary"
              disabled={!!bulkBusy || writableCount === 0}
              onClick={() => bulkWritable(false, 'all')}
            >
              {bulkBusy === 'wr-off-all' ? '…' : 'Unwritable all'}
            </button>
          </div>
          {msg && <p className="ok small">{msg}</p>}
          {dbSelected.size > 0 && (
            <div className="sel-bar">
              <span className="muted small">{dbSelected.size} selected</span>
              <button
                type="button"
                className="secondary"
                disabled={!!bulkBusy}
                onClick={() => syncFromOpc([...dbSelected])}
              >
                {bulkBusy === 'sync-sel' ? 'Syncing…' : 'Sync selected from OPC'}
              </button>
              <button
                type="button"
                className="secondary"
                disabled={!!bulkBusy}
                onClick={() => bulkSimulate(true, 'selected')}
              >
                {bulkBusy === 'sim-on-selected' ? '…' : 'Sim selected'}
              </button>
              <button
                type="button"
                className="secondary"
                disabled={!!bulkBusy}
                onClick={() => bulkSimulate(false, 'selected')}
              >
                {bulkBusy === 'sim-off-selected' ? '…' : 'Unsim selected'}
              </button>
              <button
                type="button"
                className="secondary"
                disabled={!!bulkBusy}
                onClick={() => bulkWritable(true, 'selected')}
              >
                {bulkBusy === 'wr-on-selected' ? '…' : 'Writable selected'}
              </button>
              <button
                type="button"
                className="secondary"
                disabled={!!bulkBusy}
                onClick={() => bulkWritable(false, 'selected')}
              >
                {bulkBusy === 'wr-off-selected' ? '…' : 'Unwritable selected'}
              </button>
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
          <div className="table-wrap compact db-list-table">
            <TagTreeTable
              tagValues={pageTags}
              selected={dbSelected}
              onToggleSelect={onDbToggleSelect}
              onSetEnabled={setTagsEnabled}
              onSetSimulate={setTagsSimulate}
              onSetWritable={setTagsWritable}
              onRemove={unmonitorTags}
              onUpdateTag={updateTag}
              opcConnected={opcConnected}
            />
          </div>
          {sortedFiltered.length > PAGE_SIZE && (
            <div className="pager-wrap">
              <TagPager page={pageClamped} totalPages={totalPages} onPageChange={setPage} />
            </div>
          )}
        </section>
      )}
    </div>
  )
}
