// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runcmd_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/runcmd"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type clientSuite struct {
	testing.JujuConnSuite

	resources  *common.Resources
	authoriser apiservertesting.FakeAuthorizer
	api        *runcmd.RunCommandAPI
}

var _ = gc.Suite(&clientSuite{})

func (s *clientSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag:            s.AdminUserTag(c),
		EnvironManager: true,
	}

	var err error
	s.api, err = runcmd.NewRunCommandAPI(s.State, s.resources, s.authoriser)
	c.Assert(err, gc.IsNil)
}

func (s *clientSuite) TestRun(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	machineTag := machine.Tag()
	runCommands := runcmd.RunCommands{
		Commands: "hostname",
		Targets:  []string{machineTag.String()},
		Context:  nil,
		Timeout:  1,
	}

	_, err = s.api.Run([]runcmd.RunCommands{runCommands})
	c.Assert(err, gc.IsNil)
}
