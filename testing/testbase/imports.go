// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testbase

import (
	"go/build"
	"path/filepath"
	"sort"
	"strings"

	gc "launchpad.net/gocheck"
)

// FindJujuCoreImports returns a sorted list of juju-core packages that are
// imported by the packageName parameter.  The resulting list removes the
// common prefix "launchpad.net/juju-core/" leaving just the short names.
func FindJujuCoreImports(c *gc.C, packageName string) []string {
	var imports []string

	for _, root := range build.Default.SrcDirs() {
		fullpath := filepath.Join(root, packageName)
		pkg, err := build.ImportDir(fullpath, 0)
		if err == nil {
			imports = pkg.Imports
			break
		}
	}
	if imports == nil {
		c.Fatalf(packageName + " not found")
	}

	var result []string
	const prefix = "launchpad.net/juju-core/"
	for _, name := range imports {
		if strings.HasPrefix(name, prefix) {
			result = append(result, name[len(prefix):])
		}
	}
	sort.Strings(result)
	return result
}
