import { useCallback, useState } from 'react'
import {
  deleteDeviceTag,
  postTagsSimulate,
  postTagsWritable,
  putDeviceTag,
} from '../api.js'

/**
 * Tag simulate / writable flag mutations (per-tag + bulk) for DB write list.
 * Behavior matches the former inline DbWriteListPage helpers.
 */
export default function useTagFlags({
  deviceId,
  tags,
  setTags,
  selectedIds,
  onError,
  refreshTags,
}) {
  const [bulkBusy, setBulkBusy] = useState('')
  const [msg, setMsg] = useState('')

  const setTagsSimulate = useCallback(async (tagList, simulate) => {
    if (!deviceId || !tagList?.length) return
    onError('')
    setTags((prev) =>
      prev.map((tv) =>
        tagList.some((t) => t.id === tv.tag.id)
          ? { ...tv, tag: { ...tv.tag, simulate } }
          : tv,
      ),
    )
    try {
      await postTagsSimulate(deviceId, {
        simulate,
        tag_ids: tagList.map((t) => t.id),
      })
      await refreshTags(deviceId)
    } catch (ex) {
      onError(String(ex.message || ex))
      await refreshTags(deviceId).catch(() => {})
    }
  }, [deviceId, onError, refreshTags, setTags])

  const setTagsWritable = useCallback(async (tagList, writable) => {
    if (!deviceId || !tagList?.length) return
    onError('')
    setTags((prev) =>
      prev.map((tv) =>
        tagList.some((t) => t.id === tv.tag.id)
          ? { ...tv, tag: { ...tv.tag, writable } }
          : tv,
      ),
    )
    try {
      await postTagsWritable(deviceId, {
        writable,
        tag_ids: tagList.map((t) => t.id),
      })
      await refreshTags(deviceId)
    } catch (ex) {
      onError(String(ex.message || ex))
      await refreshTags(deviceId).catch(() => {})
    }
  }, [deviceId, onError, refreshTags, setTags])

  const bulkSimulate = useCallback(async (simulate, mode) => {
    if (!deviceId) return
    const selectedList = tags.filter((t) => selectedIds.has(t.tag.id)).map((t) => t.tag)
    const target = mode === 'selected' ? selectedList : tags.map((t) => t.tag)
    if (!target.length) return
    const label = mode === 'selected'
      ? `${simulate ? 'Enable' : 'Disable'} simulation for ${target.length} selected tag(s)?`
      : `${simulate ? 'Enable' : 'Disable'} simulation for all ${target.length} tag(s) on this server?`
    if (!window.confirm(label)) return
    const busyKey = `${simulate ? 'sim-on' : 'sim-off'}-${mode}`
    setBulkBusy(busyKey)
    onError('')
    setMsg('')
    try {
      const body = mode === 'selected'
        ? { simulate, tag_ids: target.map((t) => t.id) }
        : { simulate, all: true }
      const data = await postTagsSimulate(deviceId, body)
      setMsg(`${simulate ? 'Simulation on' : 'Simulation off'}: ${data.updated} tag(s)`)
      await refreshTags(deviceId)
    } catch (ex) {
      onError(String(ex.message || ex))
    } finally {
      setBulkBusy('')
    }
  }, [deviceId, tags, selectedIds, onError, refreshTags])

  const bulkWritable = useCallback(async (writable, mode) => {
    if (!deviceId) return
    const selectedList = tags.filter((t) => selectedIds.has(t.tag.id)).map((t) => t.tag)
    const target = mode === 'selected' ? selectedList : tags.map((t) => t.tag)
    if (!target.length) return
    const label = mode === 'selected'
      ? `${writable ? 'Enable' : 'Disable'} writable for ${target.length} selected tag(s)?`
      : `${writable ? 'Enable' : 'Disable'} writable for all ${target.length} tag(s) on this server?`
    if (!window.confirm(label)) return
    const busyKey = `${writable ? 'wr-on' : 'wr-off'}-${mode}`
    setBulkBusy(busyKey)
    onError('')
    setMsg('')
    try {
      const body = mode === 'selected'
        ? { writable, tag_ids: target.map((t) => t.id) }
        : { writable, all: true }
      const data = await postTagsWritable(deviceId, body)
      setMsg(`${writable ? 'Writable on' : 'Writable off'}: ${data.updated} tag(s)`)
      await refreshTags(deviceId)
    } catch (ex) {
      onError(String(ex.message || ex))
    } finally {
      setBulkBusy('')
    }
  }, [deviceId, tags, selectedIds, onError, refreshTags])

  const setTagsEnabled = useCallback(async (tagList, enabled) => {
    if (!deviceId || !tagList?.length) return
    onError('')
    try {
      for (const tag of tagList) {
        await putDeviceTag(deviceId, { ...tag, enabled })
      }
      await refreshTags(deviceId)
    } catch (ex) {
      onError(String(ex.message || ex))
    }
  }, [deviceId, onError, refreshTags])

  const unmonitorTags = useCallback(async (tagIds, { onRemoved } = {}) => {
    if (!deviceId || !tagIds?.length) return
    onError('')
    try {
      for (const tagId of tagIds) {
        await deleteDeviceTag(deviceId, tagId)
      }
      onRemoved?.(tagIds)
      await refreshTags(deviceId)
    } catch (ex) {
      onError(String(ex.message || ex))
      throw ex // callers that need onDevicesChanged only on success
    }
  }, [deviceId, onError, refreshTags])

  return {
    bulkBusy,
    setBulkBusy,
    msg,
    setMsg,
    setTagsSimulate,
    setTagsWritable,
    bulkSimulate,
    bulkWritable,
    setTagsEnabled,
    unmonitorTags,
  }
}
