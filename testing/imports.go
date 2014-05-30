// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/testing"
	gc "launchpad.net/gocheck"
)

const jujuPkgPrefix = "launchpad.net/juju-core/"

// FindJujuCoreImports returns a sorted list of juju-core packages that are
// imported by the packageName parameter.  The resulting list removes the
// common prefix "launchpad.net/juju-core/" leaving just the short names.
func FindJujuCoreImports(c *gc.C, packageName string) []string {
	imps, err := testing.FindImports(packageName, jujuPkgPrefix)
	c.Assert(err, gc.IsNil)
	return imps
}
