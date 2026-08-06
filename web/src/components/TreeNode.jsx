import { useEffect, useRef, useState } from 'react'

export default function TreeNode({
  node,
  depth,
  pathPrefix = '',
  selectedNodeId,
  onSelect,
  loadChildren,
  isChecked,
  onToggleCheck,
  expandingId,
  monitoredIds,
}) {
  const [open, setOpen] = useState(false)
  const [kids, setKids] = useState(null)
  const [loading, setLoading] = useState(false)
  const checkRef = useRef(null)

  const path = pathPrefix ? `${pathPrefix}/${node.browse_name}` : (node.browse_name || '')
  const inDB = monitoredIds?.has(node.node_id)
  const checked = !!isChecked?.(node.node_id, path)
  const expanding = expandingId === node.node_id

  useEffect(() => {
    if (!checkRef.current || node.is_leaf) return
    checkRef.current.indeterminate = false
  }, [checked, node.is_leaf])

  const toggleOpen = async (e) => {
    e.stopPropagation()
    if (node.is_leaf) {
      onSelect?.(node, path)
      return
    }
    if (!open && kids == null) {
      setLoading(true)
      try {
        const rows = await loadChildren(node.node_id)
        setKids(rows)
      } finally {
        setLoading(false)
      }
    }
    setOpen((v) => !v)
    onSelect?.(node, path)
  }

  const onCheck = (e) => {
    e.stopPropagation()
    onToggleCheck?.(node, path, e.target.checked)
  }

  return (
    <div className="tree-node">
      <div
        className={`tree-row${selectedNodeId === node.node_id ? ' selected' : ''}${inDB ? ' in-db' : ''}`}
        style={{ paddingLeft: 8 + depth * 14 }}
      >
        <input
          ref={checkRef}
          type="checkbox"
          className="tree-check"
          checked={checked}
          disabled={expanding}
          onChange={onCheck}
          onClick={(e) => e.stopPropagation()}
          title={node.is_leaf ? 'Select tag' : 'Select all variables under folder'}
        />
        <button type="button" className="tree-row-btn" onClick={toggleOpen}>
          <span className="chev">{node.is_leaf ? '·' : open ? '▾' : '▸'}</span>
          <span className="name">{node.browse_name || node.display_name}</span>
          {inDB && node.is_leaf && <span className="badge-in-db">in DB</span>}
          {(loading || expanding) && <span className="muted small">…</span>}
        </button>
      </div>
      {open && kids && kids.map((ch) => (
        <TreeNode
          key={ch.node_id}
          node={ch}
          depth={depth + 1}
          pathPrefix={path}
          selectedNodeId={selectedNodeId}
          onSelect={onSelect}
          loadChildren={loadChildren}
          isChecked={isChecked}
          onToggleCheck={onToggleCheck}
          expandingId={expandingId}
          monitoredIds={monitoredIds}
        />
      ))}
    </div>
  )
}
