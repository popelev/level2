import { useEffect, useState } from 'react'

const emptyForm = {
  id: '',
  endpoint: 'opc.tcp://10.14.10.16:4840',
  username: '',
  password: '',
  security: 'None',
  poll_concurrency: 4,
}

export default function ServersPage({ devices, onChanged, onError }) {
  const [showForm, setShowForm] = useState(false)
  const [editForm, setEditForm] = useState(emptyForm)
  const [selectedId, setSelectedId] = useState('')

  useEffect(() => {
    if (!selectedId && devices[0]) setSelectedId(devices[0].id)
  }, [devices, selectedId])

  const openCreate = () => {
    setEditForm({ ...emptyForm })
    setShowForm(true)
  }

  const openEdit = (d) => {
    setEditForm({
      id: d.id,
      endpoint: d.endpoint,
      username: d.username || '',
      password: '',
      security: d.security || 'None',
      poll_concurrency: d.poll_concurrency > 0 ? d.poll_concurrency : 4,
    })
    setSelectedId(d.id)
    setShowForm(true)
  }

  const saveDevice = async (e) => {
    e.preventDefault()
    onError('')
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
      setShowForm(false)
      setSelectedId(editForm.id)
      await onChanged()
    } catch (ex) {
      onError(String(ex.message || ex))
    }
  }

  const removeDevice = async () => {
    if (!selectedId) return
    if (!window.confirm(`Remove server "${selectedId}"?`)) return
    onError('')
    try {
      const r = await fetch(`/api/v1/devices/${encodeURIComponent(selectedId)}`, { method: 'DELETE' })
      if (!r.ok && r.status !== 204) throw new Error(await r.text())
      setSelectedId('')
      setShowForm(false)
      await onChanged()
    } catch (ex) {
      onError(String(ex.message || ex))
    }
  }

  const selected = devices.find((d) => d.id === selectedId)

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2>Servers</h2>
          <p className="muted">Add, edit and remove OPC UA servers (UaExpert Project)</p>
        </div>
        <div className="row tight">
          <button type="button" onClick={openCreate}>+ Server</button>
          <button type="button" className="secondary" onClick={removeDevice} disabled={!selectedId}>
            Remove
          </button>
        </div>
      </div>

      <div className="servers-layout">
        <section className="panel">
          <div className="proj-label">Servers</div>
          <div className="project-tree">
            {devices.map((d) => (
              <button
                key={d.id}
                type="button"
                className={`server-row${d.id === selectedId ? ' selected' : ''}`}
                onClick={() => setSelectedId(d.id)}
                onDoubleClick={() => openEdit(d)}
              >
                <span className={`dot ${d.connected ? 'on' : 'off'}`} />
                <span className="mono">{d.id}</span>
                <span className="mono small muted">{d.endpoint}</span>
                <span className="muted small">{d.tag_count} monitored tags</span>
              </button>
            ))}
            {devices.length === 0 && <p className="muted">No servers yet</p>}
          </div>
        </section>

        <section className="panel">
          {selected && !showForm && (
            <div className="device-detail">
              <h3>{selected.id}</h3>
              <div className="detail-grid">
                <div><span className="muted">endpoint</span><div className="mono">{selected.endpoint}</div></div>
                <div><span className="muted">security</span><div>{selected.security}</div></div>
                <div><span className="muted">username</span><div>{selected.username || '—'}</div></div>
                <div><span className="muted">connected</span><div className={selected.connected ? 'good' : 'badq'}>{String(selected.connected)}</div></div>
                <div><span className="muted">monitored tags</span><div>{selected.tag_count}</div></div>
                <div>
                  <span className="muted">parallel reads</span>
                  <div>{selected.poll_concurrency > 0 ? selected.poll_concurrency : 4}</div>
                </div>
              </div>
              <div className="row">
                <button type="button" onClick={() => openEdit(selected)}>Edit</button>
                <button
                  type="button"
                  className="secondary"
                  onClick={() => {
                    location.hash = `#/monitor?device=${encodeURIComponent(selected.id)}`
                  }}
                >
                  Open in Monitor
                </button>
              </div>
            </div>
          )}
          {!selected && !showForm && <p className="muted">Select a server or create one</p>}
          {showForm && (
            <form className="device-form" onSubmit={saveDevice}>
              <h3>{devices.some((d) => d.id === editForm.id) ? 'Edit server' : 'Add server'}</h3>
              <label>
                Id
                <input
                  required
                  value={editForm.id}
                  onChange={(e) => setEditForm({ ...editForm, id: e.target.value })}
                  disabled={devices.some((d) => d.id === editForm.id)}
                />
              </label>
              <label>
                Endpoint
                <input
                  required
                  value={editForm.endpoint}
                  onChange={(e) => setEditForm({ ...editForm, endpoint: e.target.value })}
                />
              </label>
              <label>
                Username
                <input
                  value={editForm.username}
                  onChange={(e) => setEditForm({ ...editForm, username: e.target.value })}
                />
              </label>
              <label>
                Password
                <input
                  type="password"
                  value={editForm.password}
                  onChange={(e) => setEditForm({ ...editForm, password: e.target.value })}
                  placeholder={devices.some((d) => d.id === editForm.id) ? 'leave empty to keep' : ''}
                />
              </label>
              {devices.some((d) => d.id === editForm.id) && (
                <p className="hint">Empty password keeps the existing secret.</p>
              )}
              <label>
                Security
                <select
                  value={editForm.security}
                  onChange={(e) => setEditForm({ ...editForm, security: e.target.value })}
                >
                  <option>None</option>
                  <option>Sign</option>
                  <option>SignAndEncrypt</option>
                </select>
              </label>
              <label>
                Parallel reads
                <input
                  type="number"
                  min={1}
                  max={16}
                  required
                  value={editForm.poll_concurrency}
                  onChange={(e) => setEditForm({
                    ...editForm,
                    poll_concurrency: Math.min(16, Math.max(1, Number(e.target.value) || 1)),
                  })}
                />
              </label>
              <p className="hint">
                Concurrent OPC Read batches (each <=100 nodes) for this server. Raise to shorten poll cycles on large tag sets; lower if the PLC drops the session.
              </p>
              <div className="row">
                <button type="submit">Save</button>
                <button type="button" className="secondary" onClick={() => setShowForm(false)}>Cancel</button>
              </div>
            </form>
          )}
        </section>
      </div>
    </div>
  )
}
