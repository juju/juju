// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/workload"
)

type workloadSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&workloadSuite{})

func (*workloadSuite) TestComponentName(c *gc.C) {
	// Are you really sure you want to change the component name?
	c.Assert(workload.ComponentName, gc.Equals, "workloads")
}
