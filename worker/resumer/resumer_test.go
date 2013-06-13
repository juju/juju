// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resumer_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/resumer"
	stdtesting "testing"
)

func TestPackage(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type ResumerSuite struct {
	testing.JujuConnSuite
}

var _ = Suite(&ResumerSuite{})

func (s *ResumerSuite) TestRunStop(c *C) {
	rr := resumer.NewResumer(s.State)

	c.Assert(rr.Stop(), IsNil)
}
