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

const jujuPkgPrefix = "launchpad.net/juju-core/"

// FindJujuCoreImports returns a sorted list of juju-core packages that are
// imported by the packageName parameter.  The resulting list removes the
// common prefix "launchpad.net/juju-core/" leaving just the short names.
func FindJujuCoreImports(c *gc.C, packageName string) []string {
	var result []string
	allpkgs := make(map[string]bool)

	findJujuCoreImports(c, packageName, allpkgs)
	for name := range allpkgs {
		result = append(result, name[len(jujuPkgPrefix):])
	}
	sort.Strings(result)
	return result
}

// findJujuCoreImports recursively adds all imported packages of given
// package (packageName) to allpkgs map.
func findJujuCoreImports(c *gc.C, packageName string, allpkgs map[string]bool) {

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

	for _, name := range imports {
		if strings.HasPrefix(name, jujuPkgPrefix) {
			allpkgs[name] = true
			findJujuCoreImports(c, name, allpkgs)
		}
	}

}
