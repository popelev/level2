import { useEffect, useMemo, useRef, useState } from 'react'
import { buildTagTree, collectLeaves } from '../tagTree.js'
import { formatValue } from '../api.js'

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
    <div className={`tt-row folder${allSel ? ' selected' : ''}`} style={{ paddingLeft: 8 + depth * 14 }}>
      <input
        ref={checkRef}
        type="checkbox"
        className="tree-check"
        checked={allSel}
        onChange={(e) => onToggleSelect(leaves.map((tv) => tv.tag.id), e.target.checked)}
      />
      <button type="button" className="tree-row-btn" onClick={onToggleOpen}>
        <span className="chev">{open ? '▾' : '▸'}</span>
        <span className="icon fold" />
        <span className="name">{node.name}</span>
        <span className="muted small">({leaves.length})</span>
      </button>
      <span className="tt-val muted">—</span>
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
}) {
  const tv = node.tv
  const tag = tv.tag
  return (
    <div className={`tt-row leaf${tag.enabled ? '' : ' dim'}${selected.has(tag.id) ? ' selected' : ''}`} style={{ paddingLeft: 8 + depth * 14 }}>
      <input
        type="checkbox"
        className="tree-check"
        checked={selected.has(tag.id)}
        onChange={(e) => onToggleSelect([tag.id], e.target.checked)}
      />
      <div className="tt-label">
        <span className="chev">·</span>
        <span className="icon var" />
        <div>
          <div className="name">{tag.id}</div>
          <div className="mono small muted">{tag.node_id}</div>
        </div>
      </div>
      <span className="tt-val mono">{formatValue(tv.sample)}</span>
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

function TreeBranch({
  node,
  depth,
  openMap,
  setOpenMap,
  selected,
  onToggleSelect,
  onSetEnabled,
  onRemove,
}) {
  const open = openMap[node.key] !== false // default open
  if (node.type === 'leaf') {
    return (
      <LeafRow
        node={node}
        depth={depth}
        selected={selected}
        onToggleSelect={onToggleSelect}
        onSetEnabled={onSetEnabled}
        onRemove={onRemove}
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
}) {
  const [openMap, setOpenMap] = useState({})
  const tree = useMemo(() => buildTagTree(tagValues), [tagValues])

  if (tagValues.length === 0) {
    return <p className="muted">No monitored tags</p>
  }

  return (
    <div className="tag-tree-table">
      <div className="tt-head">
        <span className="tt-col-tag">Tag</span>
        <span className="tt-col-val">Value</span>
        <span className="tt-col-on">On</span>
        <span className="tt-col-act" />
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
        />
      ))}
    </div>
  )
}
