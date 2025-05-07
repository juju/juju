// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.
//go:build !windows
// +build !windows

package filetesting

import (
	"os"
	"syscall"
)

// isNotExist returns true if the error is consistent with an attempt to
// reference a file that does not exist. This works around the occasionally
// unhelpful behaviour of os.IsNotExist, which does not recognise the error
// produced when trying to read a path in which some component appears to
// reference a directory but actually references a file. For example, if
// "foo" is a file, an attempt to read "foo/bar" will generate an error that
// does not satisfy os.IsNotExist, but will satisfy filetesting.isNotExist.
func isNotExist(err error) bool {
	if os.IsNotExist(err) {
		return true
	}
	if e, ok := err.(*os.PathError); ok && e.Err == syscall.ENOTDIR {
		return true
	}
	return false
}
