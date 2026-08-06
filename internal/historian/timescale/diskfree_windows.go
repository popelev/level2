//go:build windows

package timescale

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func diskSpace(path string) (free, total int64, err error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(p, &freeBytesAvailable, &totalNumberOfBytes, &totalNumberOfFreeBytes); err != nil {
		return 0, 0, fmt.Errorf("GetDiskFreeSpaceEx: %w", err)
	}
	return int64(freeBytesAvailable), int64(totalNumberOfBytes), nil
}

func diskFree(path string) (int64, error) {
	free, _, err := diskSpace(path)
	return free, err
}
