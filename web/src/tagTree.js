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

/**
 * Short original/browse label for a leaf (e.g. rvalueout, maintenance).
 * Long composite ids stay secondary in the UI.
 */
export function tagLeafDisplayName(tag) {
  const id = String(tag.id || '').trim()
  if (!id) return ''
  const path = normalizePath(tag.path)
  const segs = path ? path.split('/') : []
  const parent = segs[segs.length - 1] || ''
  const parentKey = sanitizeSeg(parent)

  // Prefer signal after last path folder in the composite id:
  // …_oil_temp_rvalueout + path …/OIL_TEMP → rvalueout
  if (parentKey) {
    const lower = id.toLowerCase()
    const needle = `_${parentKey}_`
    const at = lower.lastIndexOf(needle)
    if (at >= 0) {
      const rest = id.slice(at + needle.length)
      if (rest) return formatBrowseHint(rest)
    }
    if (lower.startsWith(`${parentKey}_`)) {
      return formatBrowseHint(id.slice(parentKey.length + 1))
    }
    // leaf directly named like folder (rare)
    if (lower.endsWith(`_${parentKey}`) || lower === parentKey) {
      return formatBrowseHint(parent)
    }
  }

  const parts = id.split('_').filter(Boolean)
  // Avoid chopping plant Excel signals (Area/Path + short id like E2_ECE_300_CL_001)
  const longComposite =
    isOpcBrowsePath(path) || segs.length >= 3 || id.length > 48 || parts.length >= 6
  if (longComposite && parts.length >= 2) {
    const last = parts[parts.length - 1]
    if (last.length <= 2 && parts.length >= 3) {
      return formatBrowseHint(`${parts[parts.length - 2]}_${last}`)
    }
    return formatBrowseHint(last)
  }

  // Plant Excel / short signal ids: keep full id as title
  return id
}

function sanitizeSeg(s) {
  return String(s || '')
    .toLowerCase()
    .replace(/[\s.\-/]+/g, '_')
    .replace(/_+/g, '_')
    .replace(/^_|_$/g, '')
}

/** Light restore of Siemens-style / TitleCase for display. */
export function formatBrowseHint(name) {
  const s = String(name || '')
  if (!s) return s
  if (/[A-Z]/.test(s)) return s
  const low = s.toLowerCase()
  const known = [
    [/^rvalueout$/i, 'rValueOut'],
    [/^rvalue(.*)$/i, (_, m) => `rValue${m ? m.charAt(0).toUpperCase() + m.slice(1) : ''}`],
    [/^sunit$/i, 'sUnit'],
    [/^sname$/i, 'sName'],
    [/^benable$/i, 'bEnable'],
    [/^icount$/i, 'iCount'],
  ]
  for (const [re, repl] of known) {
    const m = low.match(re)
    if (!m) continue
    return typeof repl === 'function' ? repl(m[0], m[1]) : repl
  }
  return low.charAt(0).toUpperCase() + low.slice(1)
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
