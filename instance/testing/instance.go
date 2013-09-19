// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
)

// MatchInstances uses DeepEquals to check the instances returned.  The lists
// are first put into a map, so the ordering of the result and expected values
// is not tested, and duplicates are ignored.
func MatchInstances(c *gc.C, result []instance.Instance, expected ...instance.Instance) {
	resultMap := make(map[instance.Id]instance.Instance)
	for _, i := range result {
		resultMap[i.Id()] = i
	}

	expectedMap := make(map[instance.Id]instance.Instance)
	for _, i := range expected {
		expectedMap[i.Id()] = i
	}
	c.Assert(resultMap, gc.DeepEquals, expectedMap)
}
