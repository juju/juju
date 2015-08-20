// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload_test

import (
	"github.com/juju/juju/workload"
	gc "gopkg.in/check.v1"
)

type workloadSuite struct{}

var _ = gc.Suite(&workloadSuite{})

func (*workloadSuite) TestComponentName(c *gc.C) {
	// Are you really sure you want to change the component name?
	c.Assert(workload.ComponentName, gc.Equals, "workloads")
}
