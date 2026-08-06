import { useEffect, useMemo, useRef, useState } from 'react'
import { buildTagTree, collectLeaves } from '../tagTree.js'
import { formatQuality, formatSampleTime, formatValue } from '../api.js'

const TYPES = ['bool', 'int64', 'float64', 'string']

function FolderRow({
  node,
  depth,
  open,
  onToggleOpen,
  selected,
  onToggleSelect,
  onSetEnabled,
  onRemove,
}) {
  const leaves = useMemo(() => collectLeaves(node), [node])
  const checkRef = useRef(null)
  const allOn = leaves.length > 0 && leaves.every((tv) => tv.tag.enabled)
  const someOn = leaves.some((tv) => tv.tag.enabled)
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
  onRemove,
  onUpdateTag,
}) {
  const tv = node.tv
  const tag = tv.tag
  return (
    <div
      className={`tt-row leaf${tag.enabled ? '' : ' dim'}${selected.has(tag.id) ? ' selected' : ''}`}
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
          <div className="name">{node.name}</div>
          {node.name !== tag.id && (
            <div className="mono small muted">{tag.id}</div>
          )}
          <div className="mono small muted">{tag.node_id}</div>
        </div>
      </div>
      <span className="tt-meta mono">{formatValue(tv.sample)}</span>
      <span
        className={`tt-meta small ${formatQuality(tv.sample) === 'good' ? 'good' : formatQuality(tv.sample) === 'bad' ? 'badq' : 'muted'}`}
        title={formatQuality(tv.sample) === 'bad' ? 'Often a UDT/structure node — use child rValueOut (e.g. ns=4;i=4901 not 4900)' : undefined}
      >
        {formatQuality(tv.sample)}
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
        title="interval_ms"
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

function TreeBranch(props) {
  const {
    node, depth, openMap, setOpenMap, selected,
    onToggleSelect, onSetEnabled, onRemove, onUpdateTag,
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
        onRemove={onRemove}
        onUpdateTag={onUpdateTag}
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
          onRemove={onRemove}
          onUpdateTag={onUpdateTag}
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
  onRemove,
  onUpdateTag,
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
          onRemove={onRemove}
          onUpdateTag={onUpdateTag}
        />
      ))}
    </div>
  )
}
