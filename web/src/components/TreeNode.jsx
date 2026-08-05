import { useState } from 'react'

export default function TreeNode({ node, depth, selectedNodeId, onSelect, loadChildren }) {
  const [open, setOpen] = useState(false)
  const [kids, setKids] = useState(null)
  const [loading, setLoading] = useState(false)

  const toggle = async (e) => {
    e.stopPropagation()
    if (node.is_leaf) {
      onSelect(node)
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
    onSelect(node)
  }

  return (
    <div className="tree-node">
      <button
        type="button"
        className={`tree-row${selectedNodeId === node.node_id ? ' selected' : ''}`}
        style={{ paddingLeft: 8 + depth * 14 }}
        onClick={toggle}
      >
        <span className="chev">{node.is_leaf ? '·' : open ? '▾' : '▸'}</span>
        <span className={`icon ${node.is_leaf ? 'var' : 'fold'}`} />
        <span className="name">{node.browse_name || node.display_name}</span>
        {loading && <span className="muted small">…</span>}
      </button>
      {open && kids && kids.map((ch) => (
        <TreeNode
          key={ch.node_id}
          node={ch}
          depth={depth + 1}
          selectedNodeId={selectedNodeId}
          onSelect={onSelect}
          loadChildren={loadChildren}
        />
      ))}
    </div>
  )
}
