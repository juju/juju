// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"fmt"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/apiserver/provisioner"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type provisionerSuite struct {
	testing.JujuConnSuite

	machines []*state.Machine

	authorizer  apiservertesting.FakeAuthorizer
	resources   *common.Resources
	provisioner *provisioner.ProvisionerAPI
}

var _ = gc.Suite(&provisionerSuite{})

func (s *provisionerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	var err error
	// Create 3 machines for the tests.
	for i := 0; i < 3; i++ {
		machine, err := s.State.AddMachine("series", state.JobHostUnits)
		c.Check(err, gc.IsNil)
		s.machines = append(s.machines, machine)
	}

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming machine 0 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:          names.MachineTag(s.machines[0].Id()),
		LoggedIn:     true,
		Manager:      true,
		MachineAgent: true,
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()

	// Create a provisioner API for the machine.
	s.provisioner, err = provisioner.NewProvisionerAPI(
		s.State,
		s.resources,
		s.authorizer,
	)
	c.Assert(err, gc.IsNil)
}

func (s *provisionerSuite) TestProvisionerFailsWithNonMachineAgentNonManagerUser(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.MachineAgent = false
	anAuthorizer.Manager = true
	// Works with an environment manager, which is not a machine agent.
	aDeployer, err := provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.IsNil)
	c.Assert(aDeployer, gc.NotNil)

	// But fails with neither a machine agent or an environment manager.
	anAuthorizer.Manager = false
	aDeployer, err = provisioner.NewProvisionerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.NotNil)
	c.Assert(aDeployer, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *provisionerSuite) TestSetPasswords(c *gc.C) {
	args := params.PasswordChanges{
		Changes: []params.PasswordChange{
			{Tag: s.machines[0].Tag(), Password: "xxx0"},
			{Tag: s.machines[1].Tag(), Password: "xxx1"},
			{Tag: s.machines[2].Tag(), Password: "xxx2"},
			{Tag: "unit-foo-0", Password: "zzz"},
			{Tag: "service-bar", Password: "abc"},
		},
	}
	results, err := s.provisioner.SetPasswords(args)
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{nil},
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the changes to both machines succeeded.
	for i, machine := range s.machines {
		c.Logf("trying %q password", machine.Tag())
		err = machine.Refresh()
		c.Assert(err, gc.IsNil)
		changed := machine.PasswordValid(fmt.Sprintf("xxx%d", i))
		c.Assert(changed, jc.IsTrue)
	}
}

func (s *provisionerSuite) TestLife(c *gc.C) {
	err := s.machines[1].EnsureDead()
	c.Assert(err, gc.IsNil)
	err = s.machines[1].Refresh()
	c.Assert(err, gc.IsNil)
	c.Assert(s.machines[0].Life(), gc.Equals, state.Alive)
	c.Assert(s.machines[1].Life(), gc.Equals, state.Dead)
	c.Assert(s.machines[2].Life(), gc.Equals, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machines[0].Tag()},
		{Tag: s.machines[1].Tag()},
		{Tag: s.machines[2].Tag()},
		{Tag: "unit-foo-0"},
		{Tag: "service-bar"},
	}}
	result, err := s.provisioner.Life(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: "alive"},
			{Life: "dead"},
			{Life: "alive"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Remove the subordinate and make sure it's detected.
	err = s.machines[1].Remove()
	c.Assert(err, gc.IsNil)
	err = s.machines[1].Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFoundError)

	result, err = s.provisioner.Life(params.Entities{
		Entities: []params.Entity{
			{Tag: s.machines[1].Tag()},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Error: apiservertesting.NotFoundError("machine 1")},
		},
	})
}

// func (s *deployerSuite) TestRemove(c *gc.C) {
// 	c.Assert(s.principal0.Life(), gc.Equals, state.Alive)
// 	c.Assert(s.subordinate0.Life(), gc.Equals, state.Alive)

// 	// Try removing alive units - should fail.
// 	args := params.Entities{Entities: []params.Entity{
// 		{Tag: "unit-mysql-0"},
// 		{Tag: "unit-mysql-1"},
// 		{Tag: "unit-logging-0"},
// 		{Tag: "unit-fake-42"},
// 	}}
// 	result, err := s.deployer.Remove(args)
// 	c.Assert(err, gc.IsNil)
// 	c.Assert(result, gc.DeepEquals, params.ErrorResults{
// 		Results: []params.ErrorResult{
// 			{&params.Error{Message: `cannot remove entity "unit-mysql-0": still alive`}},
// 			{apiservertesting.ErrUnauthorized},
// 			{&params.Error{Message: `cannot remove entity "unit-logging-0": still alive`}},
// 			{apiservertesting.ErrUnauthorized},
// 		},
// 	})

// 	err = s.principal0.Refresh()
// 	c.Assert(err, gc.IsNil)
// 	c.Assert(s.principal0.Life(), gc.Equals, state.Alive)
// 	err = s.subordinate0.Refresh()
// 	c.Assert(err, gc.IsNil)
// 	c.Assert(s.subordinate0.Life(), gc.Equals, state.Alive)

// 	// Now make the subordinate dead and try again.
// 	err = s.subordinate0.EnsureDead()
// 	c.Assert(err, gc.IsNil)
// 	err = s.subordinate0.Refresh()
// 	c.Assert(err, gc.IsNil)
// 	c.Assert(s.subordinate0.Life(), gc.Equals, state.Dead)

// 	args = params.Entities{
// 		Entities: []params.Entity{{Tag: "unit-logging-0"}},
// 	}
// 	result, err = s.deployer.Remove(args)
// 	c.Assert(err, gc.IsNil)
// 	c.Assert(result, gc.DeepEquals, params.ErrorResults{
// 		Results: []params.ErrorResult{{nil}},
// 	})

// 	err = s.subordinate0.Refresh()
// 	c.Assert(errors.IsNotFoundError(err), gc.Equals, true)

// 	// Make sure the subordinate is detected as removed.
// 	result, err = s.deployer.Remove(args)
// 	c.Assert(err, gc.IsNil)
// 	c.Assert(result, gc.DeepEquals, params.ErrorResults{
// 		Results: []params.ErrorResult{{apiservertesting.ErrUnauthorized}},
// 	})
// }
