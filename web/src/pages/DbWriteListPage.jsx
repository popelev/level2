import { useEffect, useMemo, useState } from 'react'
import TagTreeTable from '../components/TagTreeTable.jsx'
import { getJSON } from '../api.js'

export default function DbWriteListPage({ devices, onError, onDevicesChanged, initialDeviceId }) {
  const [deviceId, setDeviceId] = useState('')
  const [tags, setTags] = useState([])
  const [filter, setFilter] = useState('')
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
    setDbSelected(new Set())
    refreshTags(deviceId).catch((e) => onError(String(e.message || e)))
    const t = setInterval(() => {
      refreshTags(deviceId).catch(() => {})
    }, 3000)
    return () => clearInterval(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [deviceId])

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

  const updateTag = async (tag) => {
    if (!deviceId || !tag?.id) return
    onError('')
    try {
      const r = await fetch(
        `/api/v1/devices/${encodeURIComponent(deviceId)}/tags/${encodeURIComponent(tag.id)}`,
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(tag),
        },
      )
      if (!r.ok) throw new Error(await r.text())
      await refreshTags(deviceId)
    } catch (ex) {
      onError(String(ex.message || ex))
    }
  }

  const filtered = useMemo(() => {
    const q = filter.trim().toLowerCase()
    if (!q) return tags
    return tags.filter(
      (t) =>
        t.tag.id.toLowerCase().includes(q) ||
        t.tag.node_id.toLowerCase().includes(q) ||
        String(t.tag.path || '').toLowerCase().includes(q),
    )
  }, [tags, filter])

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
        <p className="err">Server “{deviceId}” is disconnected — live values may be stale</p>
      )}

      {deviceId && (
        <section className="panel monitored db-list-panel">
          <div className="panel-head">
            <h3>{currentDevice?.tag_count ?? tags.length} tag(s)</h3>
            <input
              className="search"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              placeholder="filter"
            />
          </div>
          <p className="hint">
            Only enabled tags are polled. Add tags from Address Space or Import / Export.
          </p>
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
          <div className="table-wrap compact db-list-table">
            <TagTreeTable
              tagValues={filtered}
              selected={dbSelected}
              onToggleSelect={onDbToggleSelect}
              onSetEnabled={setTagsEnabled}
              onRemove={unmonitorTags}
              onUpdateTag={updateTag}
            />
          </div>
        </section>
      )}
    </div>
  )
}
