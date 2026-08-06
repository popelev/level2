//go:build linux

package timescale

import "golang.org/x/sys/unix"

func diskSpace(path string) (free, total int64, err error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, 0, err
	}
	bsize := int64(st.Bsize)
	// Bavail is free blocks for unprivileged users.
	free = int64(st.Bavail) * bsize
	total = int64(st.Blocks) * bsize
	return free, total, nil
}

func diskFree(path string) (int64, error) {
	free, _, err := diskSpace(path)
	return free, err
}
