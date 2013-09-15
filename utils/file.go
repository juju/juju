// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
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
