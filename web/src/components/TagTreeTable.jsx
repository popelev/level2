import { useEffect, useMemo, useRef, useState } from 'react'
import { buildTagTree, collectLeaves } from '../tagTree.js'
import { displayQuality, formatSampleTime, formatValue } from '../api.js'

const TYPES = ['bool', 'int64', 'uint', 'float64', 'string', 'datetime']

function qualityClass(label) {
  if (label === 'good') return 'good'
  if (label === 'bad' || label === 'stale') return 'badq'
  return 'muted'
}

function FolderRow({
  node,
  depth,
  open,
  onToggleOpen,
  selected,
  onToggleSelect,
  onSetEnabled,
  onSetSimulate,
  onSetWritable,
  onRemove,
}) {
  const leaves = useMemo(() => collectLeaves(node), [node])
  const checkRef = useRef(null)
  const allOn = leaves.length > 0 && leaves.every((tv) => tv.tag.enabled)
  const someOn = leaves.some((tv) => tv.tag.enabled)
  const allSim = leaves.length > 0 && leaves.every((tv) => tv.tag.simulate)
  const someSim = leaves.some((tv) => tv.tag.simulate)
  const allWritable = leaves.length > 0 && leaves.every((tv) => tv.tag.writable)
  const someWritable = leaves.some((tv) => tv.tag.writable)
  const allSel = leaves.length > 0 && leaves.every((tv) => selected.has(tv.tag.id))
  const someSel = leaves.some((tv) => selected.has(tv.tag.id))

  useEffect(() => {
    if (checkRef.current) {
      checkRef.current.indeterminate = someSel && !allSel
    }
  }, [someSel, allSel])

  return (
    <div className={`tt-row folder${allSel ? ' selected' : ''}`} style={{ ['--depth']: depth }}>
      <input
        ref={checkRef}
        type="checkbox"
        className="tree-check"
        checked={allSel}
        onChange={(e) => onToggleSelect(leaves.map((tv) => tv.tag.id), e.target.checked)}
      />
      <button type="button" className="tree-row-btn tt-tag" onClick={onToggleOpen}>
        <span className="chev">{open ? '▾' : '▸'}</span>
        <span className="name">{node.name}</span>
        <span className="muted small">({leaves.length})</span>
      </button>
      <span className="tt-meta muted">—</span>
      <span className="tt-meta muted">—</span>
      <span className="tt-meta muted">—</span>
      <span className="tt-meta muted">—</span>
      <span className="tt-meta muted">—</span>
      <span className="tt-meta muted">—</span>
      <input
        type="checkbox"
        title="Simulate all under folder"
        checked={allSim}
        ref={(el) => {
          if (el) el.indeterminate = someSim && !allSim
        }}
        onChange={(e) => onSetSimulate(leaves.map((tv) => tv.tag), e.target.checked)}
      />
      <input
        type="checkbox"
        title="Writable all under folder (Write API allow-list)"
        checked={allWritable}
        ref={(el) => {
          if (el) el.indeterminate = someWritable && !allWritable
        }}
        onChange={(e) => onSetWritable(leaves.map((tv) => tv.tag), e.target.checked)}
      />
      <input
        type="checkbox"
        title="Enable / disable all under folder"
        checked={allOn}
        ref={(el) => {
          if (el) el.indeterminate = someOn && !allOn
        }}
        onChange={(e) => onSetEnabled(leaves.map((tv) => tv.tag), e.target.checked)}
      />
      <button
        type="button"
        className="secondary small-btn"
        onClick={() => {
          if (window.confirm(`Remove ${leaves.length} tags under "${node.name}"?`)) {
            onRemove(leaves.map((tv) => tv.tag.id))
          }
        }}
      >
        Remove
      </button>
    </div>
  )
}

function LeafRow({
  node,
  depth,
  selected,
  onToggleSelect,
  onSetEnabled,
  onSetSimulate,
  onSetWritable,
  onRemove,
  onUpdateTag,
  opcConnected,
}) {
  const tv = node.tv
  const tag = tv.tag
  const qLabel = displayQuality(tv.sample, { opcConnected, simulate: !!tag.simulate })
  const staleOffline = opcConnected === false && !tag.simulate && !!tv.sample
  return (
    <div
      className={`tt-row leaf${tag.enabled ? '' : ' dim'}${selected.has(tag.id) ? ' selected' : ''}${tag.simulate ? ' sim' : ''}${staleOffline ? ' stale' : ''}`}
      style={{ ['--depth']: depth }}
    >
      <input
        type="checkbox"
        className="tree-check"
        checked={selected.has(tag.id)}
        onChange={(e) => onToggleSelect([tag.id], e.target.checked)}
      />
      <div className="tt-label tt-tag">
        <span className="chev">·</span>
        <div>
          <div className="name">
            {node.name}
            {tag.simulate ? <span className="sim-text" title="Per-tag simulation"> sim</span> : null}
          </div>
          <div className="mono small muted tt-sub">
            {node.name !== tag.id && <span className="tt-id">{tag.id}</span>}
            {node.name !== tag.id && tag.node_id ? <span className="tt-sep"> · </span> : null}
            <span className="tt-nid">{tag.node_id}</span>
          </div>
        </div>
      </div>
      <span className={`tt-meta mono${staleOffline ? ' muted' : ''}`}>{formatValue(tv.sample)}</span>
      <span
        className={`tt-meta small ${qualityClass(qLabel)}`}
        title={
          qLabel === 'stale'
            ? 'OPC disconnected — last Live value is not live PLC quality'
            : qLabel === 'bad'
              ? 'Often a UDT/structure node — use child rValueOut (e.g. ns=4;i=4901 not 4900)'
              : undefined
        }
      >
        {qLabel}
      </span>
      <span className="tt-meta mono small muted">{formatSampleTime(tv.sample, tv.updated_at)}</span>
      <select
        className="tt-select"
        value={tag.datatype || 'float64'}
        onChange={(e) => onUpdateTag({ ...tag, datatype: e.target.value })}
        title="datatype"
      >
        {TYPES.map((t) => (
          <option key={t} value={t}>{t}</option>
        ))}
      </select>
      <input
        className="tt-interval"
        type="number"
        min={100}
        step={100}
        defaultValue={tag.interval_ms || 1000}
        key={`${tag.id}-${tag.interval_ms}`}
        onBlur={(e) => {
          const v = Number(e.target.value)
          if (v > 0 && v !== tag.interval_ms) onUpdateTag({ ...tag, interval_ms: v })
        }}
        title="interval_ms (configured)"
      />
      <span
        className={`tt-meta mono small ${pollClass(tv.poll_avg_ms, tag.interval_ms)}`}
        title="Average wall-clock interval of last ≤5 OPC reads (collector)"
      >
        {formatPollAvg(tv.poll_avg_ms)}
      </span>
      <input
        type="checkbox"
        checked={!!tag.simulate}
        onChange={(e) => onSetSimulate([tag], e.target.checked)}
        title="Simulate (mock samples; OPC skipped for this tag)"
      />
      <input
        type="checkbox"
        checked={!!tag.writable}
        onChange={(e) => onSetWritable([tag], e.target.checked)}
        title="Writable (Write API allow-list; default off)"
      />
      <input
        type="checkbox"
        checked={!!tag.enabled}
        onChange={(e) => onSetEnabled([tag], e.target.checked)}
        title="Write to DB"
      />
      <button type="button" className="secondary small-btn" onClick={() => onRemove([tag.id])}>
        Remove
      </button>
    </div>
  )
}

function formatPollAvg(ms) {
  if (ms == null || ms <= 0) return '—'
  return String(Math.round(ms))
}

function pollClass(avg, configured) {
  if (avg == null || avg <= 0) return 'muted'
  const want = configured > 0 ? configured : 1000
  if (avg > want * 1.5) return 'badq'
  if (avg > want * 1.2) return 'warn'
  return 'good'
}

function TreeBranch(props) {
  const {
    node, depth, openMap, setOpenMap, selected,
    onToggleSelect, onSetEnabled, onSetSimulate, onSetWritable, onRemove, onUpdateTag, opcConnected,
  } = props
  const open = openMap[node.key] !== false
  if (node.type === 'leaf') {
    return (
      <LeafRow
        node={node}
        depth={depth}
        selected={selected}
        onToggleSelect={onToggleSelect}
        onSetEnabled={onSetEnabled}
        onSetSimulate={onSetSimulate}
        onSetWritable={onSetWritable}
        onRemove={onRemove}
        onUpdateTag={onUpdateTag}
        opcConnected={opcConnected}
      />
    )
  }
  return (
    <>
      <FolderRow
        node={node}
        depth={depth}
        open={open}
        onToggleOpen={() => setOpenMap((m) => ({ ...m, [node.key]: !open }))}
        selected={selected}
        onToggleSelect={onToggleSelect}
        onSetEnabled={onSetEnabled}
        onSetSimulate={onSetSimulate}
        onSetWritable={onSetWritable}
        onRemove={onRemove}
      />
      {open && node.children.map((ch) => (
        <TreeBranch
          key={ch.key}
          node={ch}
          depth={depth + 1}
          openMap={openMap}
          setOpenMap={setOpenMap}
          selected={selected}
          onToggleSelect={onToggleSelect}
          onSetEnabled={onSetEnabled}
          onSetSimulate={onSetSimulate}
          onSetWritable={onSetWritable}
          onRemove={onRemove}
          onUpdateTag={onUpdateTag}
          opcConnected={opcConnected}
        />
      ))}
    </>
  )
}

export default function TagTreeTable({
  tagValues,
  selected,
  onToggleSelect,
  onSetEnabled,
  onSetSimulate,
  onSetWritable,
  onRemove,
  onUpdateTag,
  opcConnected,
}) {
  const [openMap, setOpenMap] = useState({})
  const tree = useMemo(() => buildTagTree(tagValues), [tagValues])

  if (tagValues.length === 0) {
    return <p className="muted">No tags in DB write list — use Address Space or Import / Export</p>
  }

  return (
    <div className="tag-tree-table">
      <div className="tt-head">
        <span />
        <span>Tag</span>
        <span>Value</span>
        <span>Quality</span>
        <span>Time</span>
        <span>Type</span>
        <span>ms</span>
        <span title="Average of last 5 poll intervals">avg</span>
        <span title="Per-tag simulation">Sim</span>
        <span title="Write API allow-list (default off)">Writable</span>
        <span>On</span>
        <span />
      </div>
      {tree.map((n) => (
        <TreeBranch
          key={n.key}
          node={n}
          depth={0}
          openMap={openMap}
          setOpenMap={setOpenMap}
          selected={selected}
          onToggleSelect={onToggleSelect}
          onSetEnabled={onSetEnabled}
          onSetSimulate={onSetSimulate}
          onSetWritable={onSetWritable}
          onRemove={onRemove}
          onUpdateTag={onUpdateTag}
          opcConnected={opcConnected}
        />
      ))}
    </div>
  )
}
