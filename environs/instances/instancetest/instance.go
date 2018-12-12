// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancetest

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs/instances"
)

// MatchInstances uses DeepEquals to check the instances returned.  The lists
// are first put into a map, so the ordering of the result and expected values
// is not tested, and duplicates are ignored.
func MatchInstances(c *gc.C, result []instances.Instance, expected ...instances.Instance) {
	resultMap := make(map[instance.Id]instances.Instance)
	for _, i := range result {
		resultMap[i.Id()] = i
	}

	expectedMap := make(map[instance.Id]instances.Instance)
	for _, i := range expected {
		expectedMap[i.Id()] = i
	}
	c.Assert(resultMap, gc.DeepEquals, expectedMap)
}
