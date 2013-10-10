// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"os"
	"syscall"
)

const (
	movefile_replace_existing = 0x1
	movefile_write_through    = 0x8
)

//sys moveFileEx(lpExistingFileName *uint16, lpNewFileName *uint16, dwFlags uint32) (err error) = MoveFileExW

// Replace atomically replaces the destination file or directory with the source.
func Replace(source, destination string) error {
	src, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return &os.LinkError{"replace", source, destination, err}
	}
	dest, err := syscall.UTF16PtrFromString(destination)
	if err != nil {
		return &os.LinkError{"replace", source, destination, err}
	}
	if err := moveFileEx(src, dest, movefile_replace_existing|movefile_write_through); err != nil {
		return &os.LinkError{"replace", source, destination, err}
	}
	return nil
}
