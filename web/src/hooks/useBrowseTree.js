import { startTransition, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ROOT_ID, expandTags, getJSON } from '../api.js'
import { leafPathUnderPrefix } from '../tagTree.js'

export function formatExpandProgress(ev, folderName, folderIdx, folderTotal) {
  const prefix =
    folderIdx && folderTotal
      ? `${folderName} (${folderIdx}/${folderTotal}): `
      : folderName
        ? `${folderName}: `
        : ''
  if (!ev || !ev.phase) return `${prefix}Loading tags…`
  if (ev.phase === 'browse') {
    return `${prefix}Browsing address space… (${ev.done ?? 0} leaves)`
  }
  if (ev.phase === 'datatype') {
    return `${prefix}Reading datatypes ${ev.done ?? 0}/${ev.total ?? 0}…`
  }
  return `${prefix}Loading tags…`
}

export function leafName(expanded) {
  if (expanded.browse_path) {
    const parts = String(expanded.browse_path).split('.')
    return parts[parts.length - 1] || expanded.id
  }
  if (expanded.id && expanded.id.includes('.')) {
    const parts = expanded.id.split('.')
    return parts[parts.length - 1]
  }
  return expanded.id
}

/**
 * OPC UA browse tree + checkbox selection (folder-aware) for Address Space.
 * UI behavior matches the former inline MonitorPage logic.
 */
export default function useBrowseTree({ deviceId, onError, setMsg }) {
  const [selectedNode, setSelectedNode] = useState(null)
  const [selectedPath, setSelectedPath] = useState('')
  const [rootKids, setRootKids] = useState([])
  const [treeKey, setTreeKey] = useState(0)
  const [addrChecked, setAddrChecked] = useState(() => new Map())
  const [excludedIds, setExcludedIds] = useState(() => new Set())
  const [expandingId, setExpandingId] = useState('')
  const [expandGen, setExpandGen] = useState(0)
  const expandCacheRef = useRef(new Map())
  const pendingLeafRef = useRef(null)
  const leafFlushTimer = useRef(0)
  const expandAbortRef = useRef(0)

  const loadChildren = useCallback(async (nodeId) => {
    if (!deviceId) return []
    return getJSON(
      `/api/v1/browse?device_id=${encodeURIComponent(deviceId)}&node_id=${encodeURIComponent(nodeId)}`,
    )
  }, [deviceId])

  const reloadTree = useCallback(async () => {
    if (!deviceId) {
      setRootKids([])
      return
    }
    const kids = await loadChildren(ROOT_ID)
    setRootKids(kids)
    setTreeKey((k) => k + 1)
    setSelectedNode({ node_id: ROOT_ID, browse_name: 'Root', is_leaf: false })
    setSelectedPath('')
    setAddrChecked(new Map())
    setExcludedIds(new Set())
    expandCacheRef.current = new Map()
    expandAbortRef.current += 1
    setExpandGen((g) => g + 1)
  }, [deviceId, loadChildren])

  const resetSelectionCaches = useCallback(() => {
    setAddrChecked(new Map())
    setExcludedIds(new Set())
    expandCacheRef.current = new Map()
    expandAbortRef.current += 1
    setExpandGen((g) => g + 1)
  }, [])

  useEffect(() => () => {
    if (leafFlushTimer.current) window.clearTimeout(leafFlushTimer.current)
  }, [])

  const folderPaths = useMemo(() => {
    const out = []
    for (const v of addrChecked.values()) {
      if (v.folder) out.push(v.path || '')
    }
    return out
  }, [addrChecked])

  const isUnderCheckedFolder = useCallback((path) => {
    for (const fp of folderPaths) {
      if (path === fp || (fp && String(path || '').startsWith(`${fp}/`))) return true
    }
    return false
  }, [folderPaths])

  const isAddrChecked = useCallback((nodeId, path) => {
    if (excludedIds.has(nodeId)) return false
    if (addrChecked.has(nodeId)) return true
    return isUnderCheckedFolder(path)
  }, [addrChecked, excludedIds, isUnderCheckedFolder])

  const collectLeavesFromSelection = useCallback(async (onProgress) => {
    const leaves = []
    const folders = []
    for (const item of addrChecked.values()) {
      if (item.folder) folders.push(item)
      else if (!excludedIds.has(item.node_id)) leaves.push(item)
    }
    for (let i = 0; i < folders.length; i++) {
      const f = folders[i]
      onProgress?.(`Expanding ${f.browse_name} (${i + 1}/${folders.length})…`)
      let expanded = expandCacheRef.current.get(f.node_id)
      if (!expanded) {
        expanded = await expandTags(deviceId, f.node_id, f.browse_name, (ev) => {
          onProgress?.(formatExpandProgress(ev, f.browse_name, i + 1, folders.length))
        })
        expandCacheRef.current.set(f.node_id, expanded)
        setExpandGen((g) => g + 1)
      }
      for (const t of expanded) {
        if (excludedIds.has(t.node_id)) continue
        const name = leafName(t)
        const rel = t.browse_path || name
        const leafPath = leafPathUnderPrefix(f.path, rel)
        leaves.push({
          browse_name: name,
          node_id: t.node_id,
          path: leafPath,
          datatype: t.datatype || '',
          tag_id: t.id,
        })
      }
    }
    return leaves
  }, [addrChecked, excludedIds, deviceId])

  const flushLeafChecks = useCallback(() => {
    leafFlushTimer.current = 0
    const pending = pendingLeafRef.current
    if (!pending?.size) return
    pendingLeafRef.current = null
    startTransition(() => {
      setAddrChecked((prev) => {
        const next = new Map(prev)
        for (const [nodeId, op] of pending) {
          if (op === null) next.delete(nodeId)
          else next.set(nodeId, op)
        }
        return next
      })
      setExcludedIds((prev) => {
        let changed = false
        const next = new Set(prev)
        for (const [nodeId, op] of pending) {
          // Leaf queue never adds exclusions (folder-covered unchecks do that
          // directly). Checking a leaf clears a prior exclusion.
          if (op !== null && next.delete(nodeId)) changed = true
        }
        return changed ? next : prev
      })
    })
  }, [])

  const queueLeafCheck = useCallback((nodeId, value) => {
    if (!pendingLeafRef.current) pendingLeafRef.current = new Map()
    pendingLeafRef.current.set(nodeId, value)
    if (leafFlushTimer.current) return
    leafFlushTimer.current = window.setTimeout(flushLeafChecks, 40)
  }, [flushLeafChecks])

  const removeUnderPath = (prev, folderPath, folderNodeId) => {
    const next = new Map(prev)
    next.delete(folderNodeId)
    const prefix = folderPath ? `${folderPath}/` : ''
    for (const [k, v] of prev) {
      if (k === folderNodeId) continue
      if (v.path === folderPath || (prefix && String(v.path || '').startsWith(prefix))) {
        next.delete(k)
      }
    }
    return next
  }

  const prefetchFolderExpand = async (node, path, token) => {
    setExpandingId(node.node_id)
    setMsg(`Loading tags under ${node.browse_name}…`)
    try {
      let expanded = expandCacheRef.current.get(node.node_id)
      if (!expanded) {
        expanded = await expandTags(deviceId, node.node_id, node.browse_name, (ev) => {
          if (token !== expandAbortRef.current) return
          setMsg(formatExpandProgress(ev, node.browse_name))
        })
        if (token !== expandAbortRef.current) return
        expandCacheRef.current.set(node.node_id, expanded)
        setExpandGen((g) => g + 1)
      }
      if (token !== expandAbortRef.current) return
      setMsg(`Selected ${expanded.length} leaf tag(s) under ${node.browse_name}`)
    } catch (ex) {
      if (token !== expandAbortRef.current) return
      onError(String(ex.message || ex))
      setMsg('')
    } finally {
      if (token === expandAbortRef.current) setExpandingId('')
    }
  }

  const onToggleAddrCheck = async (node, path, checked) => {
    onError('')
    if (node.is_leaf) {
      if (checked) {
        if (isUnderCheckedFolder(path)) {
          // Covered by ancestor folder — clear exclusion only.
          setExcludedIds((prev) => {
            if (!prev.has(node.node_id)) return prev
            const next = new Set(prev)
            next.delete(node.node_id)
            return next
          })
          return
        }
        queueLeafCheck(node.node_id, {
          browse_name: node.browse_name,
          node_id: node.node_id,
          path,
          datatype: node.datatype || '',
        })
      } else if (isUnderCheckedFolder(path)) {
        setExcludedIds((prev) => {
          const next = new Set(prev)
          next.add(node.node_id)
          return next
        })
      } else {
        queueLeafCheck(node.node_id, null)
      }
      return
    }

    if (!checked) {
      expandAbortRef.current += 1
      const cached = expandCacheRef.current.get(node.node_id)
      startTransition(() => {
        setAddrChecked((prev) => {
          if (cached?.length) {
            const next = new Map(prev)
            next.delete(node.node_id)
            for (const t of cached) next.delete(t.node_id)
            return next
          }
          return removeUnderPath(prev, path, node.node_id)
        })
        setExcludedIds((prev) => {
          if (!prev.size) return prev
          const next = new Set(prev)
          if (cached?.length) {
            for (const t of cached) next.delete(t.node_id)
          }
          return next
        })
      })
      setExpandingId('')
      setMsg('')
      return
    }

    // Instant folder select — do not wait for OPC expand or put thousands of
    // leaves into React state. Prefetch expand into cache for count / write.
    const token = ++expandAbortRef.current
    startTransition(() => {
      setAddrChecked((prev) => {
        const next = new Map(prev)
        next.set(node.node_id, {
          browse_name: node.browse_name,
          node_id: node.node_id,
          path,
          folder: true,
        })
        return next
      })
    })
    void prefetchFolderExpand(node, path, token)
  }

  const selectionSummary = useMemo(() => {
    let leafN = 0
    let folderN = 0
    let pendingFolders = 0
    const seen = new Set()
    for (const [id, v] of addrChecked) {
      if (v.folder) {
        folderN++
        const cached = expandCacheRef.current.get(id)
        if (!cached) {
          pendingFolders++
          continue
        }
        for (const t of cached) {
          if (excludedIds.has(t.node_id) || seen.has(t.node_id)) continue
          seen.add(t.node_id)
          leafN++
        }
      } else if (!excludedIds.has(id) && !seen.has(id)) {
        seen.add(id)
        leafN++
      }
    }
    return { leafN, folderN, pendingFolders }
    // expandGen bumps when cache fills so count refreshes without putting leaves in state
  }, [addrChecked, excludedIds, expandGen])

  const hasSelection = addrChecked.size > 0
  const selLabel = selectionSummary.pendingFolders
    ? `${selectionSummary.folderN} folder(s) loading…`
    : `${selectionSummary.leafN} selected`

  const clearSelection = useCallback(({ clearMsg = true } = {}) => {
    expandAbortRef.current += 1
    setAddrChecked(new Map())
    setExcludedIds(new Set())
    setExpandingId('')
    if (clearMsg) setMsg('')
  }, [setMsg])

  return {
    selectedNode,
    setSelectedNode,
    selectedPath,
    setSelectedPath,
    rootKids,
    treeKey,
    expandingId,
    loadChildren,
    reloadTree,
    resetSelectionCaches,
    isAddrChecked,
    onToggleAddrCheck,
    collectLeavesFromSelection,
    hasSelection,
    selLabel,
    clearSelection,
    addrChecked,
  }
}
