// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"path"
	"path/filepath"
	"strings"

	"launchpad.net/juju-core/juju/osenv"
)

// NormalizePath replaces a leading ~ with $HOME, and removes any .. or . path
// elements.
func NormalizePath(dir string) string {
	if strings.HasPrefix(dir, "~/") {
		dir = filepath.Join(osenv.Home(), dir[2:])
	}
	return filepath.Clean(dir)
}

// JoinServerPath joins any number of path elements into a single path, adding
// a path separator (based on the current juju server OS) if necessary. The
// result is Cleaned; in particular, all empty strings are ignored.
func JoinServerPath(elem ...string) string {
	return path.Join(elem...)
}
