// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
// +build !windows

package utils

import (
	"os"
	"syscall"
)

// ReplaceFile atomically replaces the destination file or directory
// with the source. The errors that are returned are identical to
// those returned by os.Rename.
func ReplaceFile(source, destination string) error {
	return os.Rename(source, destination)
}

// IsNotExist returns true if the error is consistent with an attempt to
// reference a file that does not exist.
func IsNotExist(err error) bool {
	if os.IsNotExist(err) {
		return true
	}
	if e, ok := err.(*os.PathError); ok && e.Err == syscall.ENOTDIR {
		return true
	}
	return false
}
