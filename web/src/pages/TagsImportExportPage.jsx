import { useEffect, useState } from 'react'

export default function TagsImportExportPage({ devices, onError, onDevicesChanged, initialDeviceId }) {
  const [deviceId, setDeviceId] = useState('')
  const [importBusy, setImportBusy] = useState(false)
  const [replaceTags, setReplaceTags] = useState(false)
  const [msg, setMsg] = useState('')

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

  const current = devices.find((d) => d.id === deviceId)

  const pickDevice = (id) => {
    setDeviceId(id)
    location.hash = `#/import?device=${encodeURIComponent(id)}`
  }

  const exportTags = () => {
    if (!deviceId) return
    onError('')
    window.location.href = `/api/v1/devices/${encodeURIComponent(deviceId)}/tags.xlsx`
    setMsg(`Download started for ${deviceId}`)
  }

  const importExcel = async (file) => {
    if (!deviceId || !file) return
    if (replaceTags && !window.confirm('Replace all monitored tags for this server?')) return
    setImportBusy(true)
    setMsg('')
    onError('')
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
      await onDevicesChanged()
    } catch (ex) {
      onError(String(ex.message || ex))
    } finally {
      setImportBusy(false)
    }
  }

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2>Import / Export</h2>
          <p className="muted">Plant Excel (Area, Path, Signal, NodeId) for one server</p>
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

      {deviceId && (
        <section className="panel">
          <p className="hint">
            Columns: Area, Path, Signal, MeasurePoint NodeId, DataType, DataType Name.
            For full project (all servers), use the Projects tab.
          </p>
          {current && (
            <p className="muted small">
              {current.tag_count} tag(s) in config
              {current.connected ? ' · OPC connected' : ' · OPC disconnected'}
            </p>
          )}

          <div className="import-export-actions">
            <button type="button" onClick={exportTags}>
              Export tags (.xlsx)
            </button>
          </div>

          <div className="import-box">
            <div className="muted">Import tags from Excel</div>
            <label className="check">
              <input type="checkbox" checked={replaceTags} onChange={(e) => setReplaceTags(e.target.checked)} />
              replace all tags on this server
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
          </div>

          {msg && <div className="good small">{msg}</div>}
        </section>
      )}
    </div>
  )
}
