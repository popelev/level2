package opcua

// chunkRanges splits [0,n) into half-open [start,end) slices of at most size.
// Pure helper for poll batching (and unit tests).
func chunkRanges(n, size int) [][2]int {
	if n <= 0 || size <= 0 {
		return nil
	}
	out := make([][2]int, 0, (n+size-1)/size)
	for start := 0; start < n; start += size {
		end := start + size
		if end > n {
			end = n
		}
		out = append(out, [2]int{start, end})
	}
	return out
}

// pollReadConcurrency caps parallel Read workers by batch count and configured limit.
func pollReadConcurrency(batchCount, limit int) int {
	if batchCount <= 0 {
		return 0
	}
	if limit <= 1 || batchCount == 1 {
		return 1
	}
	if limit > batchCount {
		return batchCount
	}
	return limit
}