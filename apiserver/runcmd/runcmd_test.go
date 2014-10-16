// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runcmd_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/runcmd"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing"

	"github.com/juju/juju/state"
)

type clientSuite struct {
	jujutesting.JujuConnSuite

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

// End to end test is already performed by api/runcmd/client_test.go
// This is a basic sanity check test.
func (s *clientSuite) TestRun(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	machineTag := machine.Tag()
	runParams := params.RunParamsV1{
		Commands: "hostname",
		Targets:  []string{machineTag.String()},
		Context:  nil,
		Timeout:  testing.LongWait,
	}

	_, err = s.api.Run(runParams)
	c.Assert(err, gc.IsNil)
}

// End to end test is already performed by api/runcmd/client_test.go
// This is a basic sanity check test.
func (s *clientSuite) TestRunOnAllMachines(c *gc.C) {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)

	machineTag := machine.Tag()
	runParams := params.RunParamsV1{
		Commands: "hostname",
		Targets:  []string{machineTag.String()},
		Context:  nil,
		Timeout:  testing.LongWait,
	}

	_, err = s.api.RunOnAllMachines(runParams)
	c.Assert(err, gc.IsNil)
}
