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
 * Unique tag id for DB list from Address Space path.
 * Plant Excel rows use path depth ≤2 and signal-only ids — unchanged.
 * OPC browse paths use ALL folder segments + signal (last-segment-only collided
 * across sibling units and dropped ~half of bulk writes via upsert overwrite).
 */
export function tagIdFromBrowse(folderPath, browseName, sanitizeId) {
  const base = sanitizeId(browseName || '')
  const segs = normalizePath(folderPath).split('/').filter(Boolean)
  if (!segs.length || !isOpcBrowsePath(folderPath)) return base
  const raw = [...segs, browseName || ''].filter(Boolean).join('_')
  return sanitizeId(raw)
    .toLowerCase()
    .replace(/[.\-/]+/g, '_')
    .replace(/_+/g, '_')
    .replace(/^_|_$/g, '')
}

/** Short stable suffix from OPC NodeId for id disambiguation. */
export function nodeIdSuffix(nodeId) {
  const s = String(nodeId || '')
  const m = s.match(/;([isgb]=.+)$/i) || s.match(/([isgb]=\S+)$/i)
  const raw = (m ? m[1] : s).replace(/[^a-zA-Z0-9]+/g, '_').replace(/^_|_$/g, '')
  return raw.slice(-24) || 'n'
}

/**
 * Build unique tag payloads for bulk upsert. Skips duplicate node_ids;
 * renames id (nodeId suffix) when two different nodes would share an id.
 * @returns {{ tags: object[], skippedDuplicates: number, renamed: number }}
 */
export function allocateUniqueTagIds(items, sanitizeId) {
  const usedIds = new Map() // id -> node_id
  const seenNodes = new Set()
  const tags = []
  let skippedDuplicates = 0
  let renamed = 0

  for (const item of items) {
    if (!item?.node_id) continue
    if (seenNodes.has(item.node_id)) {
      skippedDuplicates++
      continue
    }
    seenNodes.add(item.node_id)

    const folderPath = normalizePath(
      item.path && String(item.path).includes('/')
        ? String(item.path).slice(0, String(item.path).lastIndexOf('/'))
        : '',
    )
    // Prefer full browse path for uniqueness; expand ids are relative to the
    // checked folder and can collide when several folders are selected.
    let id = tagIdFromBrowse(folderPath, item.browse_name || '', sanitizeId)
    if (!id && item.tag_id) {
      id = sanitizeId(String(item.tag_id))
        .toLowerCase()
        .replace(/[.\-/]+/g, '_')
        .replace(/_+/g, '_')
        .replace(/^_|_$/g, '')
    }
    if (!id) id = `tag_${nodeIdSuffix(item.node_id)}`

    if (usedIds.has(id) && usedIds.get(id) !== item.node_id) {
      const suf = nodeIdSuffix(item.node_id)
      let candidate = sanitizeId(`${id}_${suf}`)
        .toLowerCase()
        .replace(/_+/g, '_')
      let n = 2
      while (usedIds.has(candidate) && usedIds.get(candidate) !== item.node_id) {
        candidate = sanitizeId(`${id}_${suf}_${n}`).toLowerCase().replace(/_+/g, '_')
        n++
      }
      id = candidate
      renamed++
    }
    usedIds.set(id, item.node_id)
    tags.push({
      id,
      node_id: item.node_id,
      path: folderPath,
      datatype: item.datatype || 'float64',
      enabled: true,
      interval_ms: item.interval_ms || 1000,
    })
  }
  return { tags, skippedDuplicates, renamed }
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
