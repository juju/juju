// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build linux

package backups

import (
	"golang.org/x/sys/unix"

	"github.com/juju/juju/internal/errors"
)

// diskFree returns the number of bytes available to unprivileged users
// on the file system hosting dir.
func diskFree(dir string) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(dir, &stat); err != nil {
		return 0, errors.Capture(err)
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

// diskTotal returns the total size in bytes of the file system hosting
// dir.
func diskTotal(dir string) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(dir, &stat); err != nil {
		return 0, errors.Capture(err)
	}
	return stat.Blocks * uint64(stat.Bsize), nil
}
