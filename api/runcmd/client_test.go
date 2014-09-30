// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runcmd_test

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/api/runcmd"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type clientSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&clientSuite{})

func (s *clientSuite) TestClientRunCommand(c *gc.C) {
	_, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)

	result := runcmd.NewClient(s.APIState)
	c.Assert(result, gc.NotNil)
}

func (s *clientSuite) TestClientRunCommandVersion(c *gc.C) {
	client := runcmd.NewClient(s.APIState)
	c.Assert(client.BestAPIVersion(), gc.Equals, 1)
}
