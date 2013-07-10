// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"os"
	"path/filepath"
	"strings"
)

// NormalizePath replaces a leading ~ with $HOME, and removes any .. or . path
// elements.
func NormalizePath(dir string) string {
	if strings.HasPrefix(dir, "~/") {
		dir = filepath.Join(os.Getenv("HOME"), dir[2:])
	}
	return filepath.Clean(dir)
}
