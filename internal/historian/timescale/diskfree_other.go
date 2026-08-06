//go:build !linux && !windows

package timescale

import "fmt"

func diskFree(path string) (int64, error) {
	return 0, fmt.Errorf("disk free space not supported on this platform (%s)", path)
}
