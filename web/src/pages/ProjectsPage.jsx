import { useState } from 'react'

export default function ProjectsPage({ devices, onError, onDevicesChanged }) {
  const [preview, setPreview] = useState(null)
  const [mode, setMode] = useState('merge')
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState('')
  const [valDevice, setValDevice] = useState('')
  const [valRows, setValRows] = useState([])
  const [valCounts, setValCounts] = useState(null)
  const [valFilter, setValFilter] = useState('')
  const [sideA, setSideA] = useState('live')
  const [sideB, setSideB] = useState('file')
  const [fileA, setFileA] = useState(null)
  const [fileB, setFileB] = useState(null)
  const [diffRows, setDiffRows] = useState([])
  const [diffOnly, setDiffOnly] = useState(true)

  const downloadProject = () => {
    window.location.href = '/api/v1/project.xlsx'
  }

  const previewFile = async (file) => {
    if (!file) return
    setBusy(true)
    setMsg('')
    onError('')
    try {
      const fd = new FormData()
      fd.append('file', file)
      const r = await fetch('/api/v1/project/preview', { method: 'POST', body: fd })
      const text = await r.text()
      if (!r.ok) throw new Error(text)
      setPreview(JSON.parse(text))
      setMsg('Preview ready — choose Apply to import')
    } catch (ex) {
      onError(String(ex.message || ex))
      setPreview(null)
    } finally {
      setBusy(false)
    }
  }

  const applyImport = async (file) => {
    if (!file) return
    if (mode === 'replace' && !window.confirm('Replace ALL servers and tags with this project?')) return
    setBusy(true)
    onError('')
    try {
      const fd = new FormData()
      fd.append('file', file)
      const r = await fetch(`/api/v1/project/import?mode=${encodeURIComponent(mode)}`, {
        method: 'POST',
        body: fd,
      })
      const text = await r.text()
      if (!r.ok) throw new Error(text)
      const data = JSON.parse(text)
      setMsg(`Imported (${data.mode}): ${data.servers} servers, ${data.tags} tags`)
      setPreview(null)
      await onDevicesChanged()
    } catch (ex) {
      onError(String(ex.message || ex))
    } finally {
      setBusy(false)
    }
  }

  const runValidate = async () => {
    setBusy(true)
    onError('')
    try {
      const r = await fetch('/api/v1/project/validate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ device_id: valDevice || undefined }),
      })
      const text = await r.text()
      if (!r.ok) throw new Error(text)
      const data = JSON.parse(text)
      setValRows(data.rows || [])
      setValCounts(data.counts || {})
      setMsg(`Validate: ${data.rows?.length || 0} tags`)
    } catch (ex) {
      onError(String(ex.message || ex))
    } finally {
      setBusy(false)
    }
  }

  const runCompare = async (asXlsx) => {
    setBusy(true)
    onError('')
    try {
      const fd = new FormData()
      if (sideA === 'file') {
        if (!fileA) throw new Error('Side A file required')
        fd.append('a', fileA)
      }
      if (sideB === 'file') {
        if (!fileB) throw new Error('Side B file required')
        fd.append('b', fileB)
      }
      const q = `?a=${encodeURIComponent(sideA)}&b=${encodeURIComponent(sideB)}`
      const url = asXlsx ? `/api/v1/project/compare.xlsx${q}` : `/api/v1/project/compare${q}`
      const r = await fetch(url, { method: 'POST', body: fd })
      if (!r.ok) throw new Error(await r.text())
      if (asXlsx) {
        const blob = await r.blob()
        const a = document.createElement('a')
        a.href = URL.createObjectURL(blob)
        a.download = 'level2-diff.xlsx'
        a.click()
        URL.revokeObjectURL(a.href)
        setMsg('Diff Excel downloaded')
      } else {
        const data = await r.json()
        setDiffRows(data.rows || [])
        setMsg(`Compare: ${data.count} difference row(s)`)
      }
    } catch (ex) {
      onError(String(ex.message || ex))
    } finally {
      setBusy(false)
    }
  }

  const filteredVal = valFilter
    ? valRows.filter((r) => r.status === valFilter)
    : valRows

  const shownDiff = diffOnly
    ? diffRows.filter((r) => r.status !== 'same')
    : diffRows

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2>Projects</h2>
          <p className="muted">Save / open Project.xlsx · validate against OPC · compare</p>
        </div>
      </div>

      <section className="panel">
        <h3>Save / Open</h3>
        <p className="hint">Project.xlsx has Servers + Tags sheets (passwords never exported).</p>
        <div className="row">
          <button type="button" onClick={downloadProject}>Download current project</button>
        </div>
        <div className="row" style={{ marginTop: 12 }}>
          <label className="check">
            Mode
            <select value={mode} onChange={(e) => setMode(e.target.value)}>
              <option value="merge">merge</option>
              <option value="replace">replace all</option>
            </select>
          </label>
          <input
            type="file"
            accept=".xlsx"
            disabled={busy}
            onChange={(e) => {
              const f = e.target.files?.[0]
              e.target.value = ''
              if (!f) return
              window.__projFile = f
              previewFile(f)
            }}
          />
          <button
            type="button"
            disabled={busy || !preview || preview.legacy}
            onClick={() => {
              const f = window.__projFile
              if (f) applyImport(f)
            }}
          >
            Apply import
          </button>
        </div>
        {preview && (
          <div className="preview-box">
            {preview.legacy ? (
              <p className="badq">Legacy plant sheet — import on Monitored tags for one server.</p>
            ) : (
              <p className="good">
                Preview: {preview.servers} servers, {preview.tags} tags
                {preview.device_ids?.length ? ` (${preview.device_ids.join(', ')})` : ''}
              </p>
            )}
            {preview.errors?.length > 0 && (
              <p className="badq small">{preview.errors.slice(0, 5).join('; ')}</p>
            )}
          </div>
        )}
        {msg && <p className="good small">{msg}</p>}
      </section>

      <section className="panel" style={{ marginTop: 12 }}>
        <h3>Validate</h3>
        <p className="hint">Check monitored tags exist on the live Address Space (Variable nodes).</p>
        <div className="row">
          <label className="server-pick">
            Server
            <select value={valDevice} onChange={(e) => setValDevice(e.target.value)}>
              <option value="">All</option>
              {devices.map((d) => (
                <option key={d.id} value={d.id}>{d.id}</option>
              ))}
            </select>
          </label>
          <button type="button" disabled={busy} onClick={runValidate}>Run validate</button>
          {valCounts && (
            <span className="muted small">
              {Object.entries(valCounts).map(([k, v]) => `${k}:${v}`).join(' · ')}
            </span>
          )}
        </div>
        {valRows.length > 0 && (
          <>
            <div className="row" style={{ marginTop: 8 }}>
              <select value={valFilter} onChange={(e) => setValFilter(e.target.value)}>
                <option value="">all statuses</option>
                {[...new Set(valRows.map((r) => r.status))].map((s) => (
                  <option key={s} value={s}>{s}</option>
                ))}
              </select>
            </div>
            <div className="table-wrap compact tall" style={{ marginTop: 8 }}>
              <table>
                <thead>
                  <tr>
                    <th>Device</th>
                    <th>Tag</th>
                    <th>NodeId</th>
                    <th>Type</th>
                    <th>Actual</th>
                    <th>Status</th>
                  </tr>
                </thead>
                <tbody>
                  {filteredVal.map((r) => (
                    <tr key={`${r.device_id}:${r.id}`} className={r.status === 'ok' ? '' : 'dim'}>
                      <td className="mono">{r.device_id}</td>
                      <td className="mono">{r.id}</td>
                      <td className="mono small">{r.node_id}</td>
                      <td>{r.datatype}</td>
                      <td className="muted">{r.actual_datatype || '—'}</td>
                      <td className={r.status === 'ok' ? 'good' : 'badq'} title={r.detail || ''}>{r.status}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
      </section>

      <section className="panel" style={{ marginTop: 12 }}>
        <h3>Compare</h3>
        <p className="hint">Side A vs Side B — live runtime or Project.xlsx files.</p>
        <div className="compare-grid">
          <div>
            <div className="muted">Side A</div>
            <select value={sideA} onChange={(e) => setSideA(e.target.value)}>
              <option value="live">Live</option>
              <option value="file">File</option>
            </select>
            {sideA === 'file' && (
              <input type="file" accept=".xlsx" onChange={(e) => setFileA(e.target.files?.[0] || null)} />
            )}
          </div>
          <div>
            <div className="muted">Side B</div>
            <select value={sideB} onChange={(e) => setSideB(e.target.value)}>
              <option value="live">Live</option>
              <option value="file">File</option>
            </select>
            {sideB === 'file' && (
              <input type="file" accept=".xlsx" onChange={(e) => setFileB(e.target.files?.[0] || null)} />
            )}
          </div>
        </div>
        <div className="row" style={{ marginTop: 10 }}>
          <button type="button" disabled={busy} onClick={() => runCompare(false)}>Compare</button>
          <button type="button" className="secondary" disabled={busy} onClick={() => runCompare(true)}>
            Export diff Excel
          </button>
          <label className="check">
            <input type="checkbox" checked={diffOnly} onChange={(e) => setDiffOnly(e.target.checked)} />
            only differences
          </label>
        </div>
        {shownDiff.length > 0 && (
          <div className="table-wrap compact tall" style={{ marginTop: 8 }}>
            <table>
              <thead>
                <tr>
                  <th>Status</th>
                  <th>Kind</th>
                  <th>Device</th>
                  <th>Id</th>
                  <th>Field</th>
                  <th>A</th>
                  <th>B</th>
                </tr>
              </thead>
              <tbody>
                {shownDiff.map((r, i) => (
                  <tr key={i}>
                    <td className="badq">{r.status}</td>
                    <td>{r.kind}</td>
                    <td className="mono small">{r.device_id || '—'}</td>
                    <td className="mono">{r.id}</td>
                    <td>{r.field || '—'}</td>
                    <td className="mono small">{r.a || '—'}</td>
                    <td className="mono small">{r.b || '—'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  )
}
