// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
)

const jujuPkgPrefix = "github.com/juju/juju/"

// FindJujuCoreImports returns a sorted list of juju-core packages that are
// imported by the packageName parameter. The resulting list removes the
// common prefix "github.com/juju/juju/" leaving just the short names.
// Suites calling this MUST NOT override HOME or XDG_CACHE_HOME.
func FindJujuCoreImports(c *tc.C, packageName string) []string {
	imps, err := testing.FindImports(packageName, jujuPkgPrefix)
	c.Assert(err, tc.ErrorIsNil)
	return imps
}
