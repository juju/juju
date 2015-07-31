// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process_test

import (
	"github.com/juju/juju/process"
	gc "gopkg.in/check.v1"
)

type processSuite struct{}

var _ = gc.Suite(&processSuite{})

func (*processSuite) TestComponentName(c *gc.C) {
	// Are you really sure you want to change the component name?
	c.Assert(process.ComponentName, gc.Equals, "workload-processes")
}
