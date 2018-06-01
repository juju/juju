// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"os"
	"runtime"
)

func getDirPerm() os.FileMode {
	// File permissions on windows yield different results then on Linux
	// For example an os.FileMode of 0100 on a directory on Windows,
	// will yield -r-xr-xr-x. os.FileMode of 0755 will yield -rwxrwxrwx
	if runtime.GOOS == "windows" {
		return os.FileMode(0777)
	}
	return os.FileMode(dirPerm)
}

var (
	DirPerm = getDirPerm()
)

const (
	ToolsFile      = toolsFile
	GUIArchiveFile = guiArchiveFile
)
