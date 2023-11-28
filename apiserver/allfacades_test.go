// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type AllFacadesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&AllFacadesSuite{})

func (s *AllFacadesSuite) TestNoPanic(c *gc.C) {
	// AllFacades will panic on error so check it by calling it.
	r := AllFacades()
	c.Assert(r, gc.NotNil)
}

func (s *AllFacadesSuite) TestFacadeVersionsInSync(c *gc.C) {
	// Ensure that there is a complete overlap between the registered
	// facade versions and the required facade versions.
	facadeVersions := requiredMigrationFacadeVersions()
	registeredFacades := AllFacades().List()

	m := make(map[string]set.Ints)
	for _, desc := range registeredFacades {
		if m[desc.Name] == nil {
			m[desc.Name] = set.NewInts()
		}
		for _, version := range desc.Versions {
			m[desc.Name].Add(version)
		}
	}

	for name, facadeVersion := range facadeVersions {
		c.Logf("checking %q", name)

		// Force the versions to be sorted.
		sort.Slice(facadeVersion, func(i, j int) bool {
			return facadeVersion[i] < facadeVersion[j]
		})

		versions, ok := m[name]
		c.Assert(ok, jc.IsTrue, gc.Commentf("facade %q not registered", name))

		// Ensure there is a complete overlap.
		c.Check(versions.Intersection(set.NewInts(facadeVersion...)).SortedValues(), gc.DeepEquals, []int(facadeVersion), gc.Commentf("facade %q versions not in sync", name))
	}
}
