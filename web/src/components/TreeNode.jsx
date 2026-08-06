import { useEffect, useRef, useState } from 'react'

export default function TreeNode({
  node,
  depth,
  pathPrefix = '',
  selectedNodeId,
  onSelect,
  loadChildren,
  checkedIds,
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
  const checked = checkedIds?.has(node.node_id)
  const expanding = expandingId === node.node_id

  useEffect(() => {
    if (!checkRef.current || node.is_leaf) return
    // Folder indeterminate is computed by parent via checkedIds of descendants —
    // for folders we only show checked when explicitly in checkedIds (folder key).
    checkRef.current.indeterminate = !!checkedIds?.has(`partial:${node.node_id}`)
  }, [checkedIds, node.is_leaf, node.node_id])

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
          checked={!!checked && !checkedIds?.has(`partial:${node.node_id}`)}
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
          checkedIds={checkedIds}
          onToggleCheck={onToggleCheck}
          expandingId={expandingId}
          monitoredIds={monitoredIds}
        />
      ))}
    </div>
  )
}
