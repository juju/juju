// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payload_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/payload"
)

type componentSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&componentSuite{})

func (*componentSuite) TestComponentName(c *gc.C) {
	// Are you really sure you want to change the component name?
	c.Assert(payload.ComponentName, gc.Equals, "payloads")
}
