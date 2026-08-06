//go:build !linux && !windows

package timescale

import "fmt"

func diskSpace(path string) (free, total int64, err error) {
	return 0, 0, fmt.Errorf("disk free space not supported on this platform (%s)", path)
}

func diskFree(path string) (int64, error) {
	free, _, err := diskSpace(path)
	return free, err
}
