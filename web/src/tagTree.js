/** Normalize path separators and trim. */
export function normalizePath(p) {
  return String(p || '')
    .replace(/\\/g, '/')
    .replace(/\.+/g, '/')
    .split('/')
    .map((s) => s.trim())
    .filter(Boolean)
    .join('/')
}

/** True when path looks like OPC Address Space browse (not plant Area/Path). */
export function isOpcBrowsePath(path) {
  return /Objects|ServerInterfaces|DeviceSet|Root/i.test(String(path || ''))
}

/**
 * Unique tag id for DB list: parent folder segment + signal (matches expand API ids).
 * Plant Excel rows use path depth ≤2 and signal-only ids — unchanged.
 */
export function tagIdFromBrowse(folderPath, browseName, sanitizeId) {
  const base = sanitizeId(browseName || '')
  const segs = normalizePath(folderPath).split('/').filter(Boolean)
  const parent = segs[segs.length - 1] || ''
  if (!parent || !isOpcBrowsePath(folderPath)) return base
  return sanitizeId(`${parent}_${browseName}`)
}

/** Leaf label in tree table (browse name when id is parent_signal). */
export function tagLeafDisplayName(tag) {
  const id = tag.id || ''
  const path = normalizePath(tag.path)
  if (!isOpcBrowsePath(path)) return id
  const idx = id.lastIndexOf('_')
  if (idx > 0) return id.slice(idx + 1)
  return id
}

/** Full path to a variable leaf under an Address Space folder prefix. */
export function leafPathUnderPrefix(folderPrefix, browsePathOrName) {
  const rel = normalizePath(String(browsePathOrName || '').replace(/\./g, '/'))
  const prefix = normalizePath(folderPrefix)
  if (!rel) return prefix
  if (!prefix) return rel
  if (rel.startsWith(`${prefix}/`) || rel === prefix) return rel
  return normalizePath(`${prefix}/${rel}`)
}

/** Fallback folder from nsu=http://Name;i=… when tag.path empty. */
export function pathFromNodeId(nodeId) {
  const m = String(nodeId || '').match(/^nsu=https?:\/\/([^;]+)/i)
  if (m) return m[1]
  return ''
}

/**
 * Build nested tree from TagValue rows.
 * @returns {{ type: 'folder'|'leaf', key: string, name: string, children?: any[], tv?: any }[]}
 */
export function buildTagTree(tagValues) {
  const root = { type: 'folder', key: '', name: '', children: [], map: new Map() }

  for (const tv of tagValues) {
    const tag = tv.tag
    let path = normalizePath(tag.path)
    if (!path) path = normalizePath(pathFromNodeId(tag.node_id))
    const segs = path ? path.split('/') : []
    let cur = root
    let acc = ''
    for (const seg of segs) {
      acc = acc ? `${acc}/${seg}` : seg
      if (!cur.map.has(seg)) {
        const folder = { type: 'folder', key: acc, name: seg, children: [], map: new Map() }
        cur.map.set(seg, folder)
        cur.children.push(folder)
      }
      cur = cur.map.get(seg)
    }
    const leaf = {
      type: 'leaf',
      key: `tag:${tag.id}`,
      name: tagLeafDisplayName(tag),
      tv,
    }
    cur.children.push(leaf)
  }

  const strip = (node) => {
    if (node.type === 'leaf') return node
    const children = node.children.map(strip)
    return { type: 'folder', key: node.key, name: node.name, children }
  }
  return root.children.map(strip)
}

/** Collect all leaf TagValues under a tree node. */
export function collectLeaves(node) {
  if (node.type === 'leaf') return [node.tv]
  return node.children.flatMap(collectLeaves)
}
