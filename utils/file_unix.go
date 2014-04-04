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
// reference a file that does not exist. This works around the occasionally
// unhelpful behaviour of os.IsNotExist, which does not recognise the error
// produced when trying to read a path in which some component appears to
// reference a directory but actually references a file. For example, if
// "foo" is a file, an attempt to read "foo/bar" will generate an error that
// does not satisfy os.IsNotExist, but will satisfy utils.IsNotExist.
func IsNotExist(err error) bool {
	if os.IsNotExist(err) {
		return true
	}
	if e, ok := err.(*os.PathError); ok && e.Err == syscall.ENOTDIR {
		return true
	}
	return false
}
