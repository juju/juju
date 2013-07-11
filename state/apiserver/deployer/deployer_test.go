// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	stdtesting "testing"

	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	"launchpad.net/juju-core/state/apiserver/deployer"
	apitesting "launchpad.net/juju-core/state/apiserver/testing"
	statetesting "launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type deployerSuite struct {
	testing.JujuConnSuite

	authorizer apitesting.FakeAuthorizer

	service0     *state.Service
	service1     *state.Service
	machine0     *state.Machine
	machine1     *state.Machine
	principal0   *state.Unit
	principal1   *state.Unit
	subordinate0 *state.Unit

	resources *common.Resources
	deployer  *deployer.DeployerAPI
}

var _ = Suite(&deployerSuite{})

func (s *deployerSuite) SetUpTest(c *C) {
	s.JujuConnSuite.SetUpTest(c)

	var err error
	s.machine0, err = s.State.AddMachine("series", state.JobManageState, state.JobHostUnits)
	c.Assert(err, IsNil)

	s.machine1, err = s.State.AddMachine("series", state.JobHostUnits)
	c.Assert(err, IsNil)

	s.service0, err = s.State.AddService("mysql", s.AddTestingCharm(c, "mysql"))
	c.Assert(err, IsNil)

	s.service1, err = s.State.AddService("logging", s.AddTestingCharm(c, "logging"))
	c.Assert(err, IsNil)
	eps, err := s.State.InferEndpoints([]string{"mysql", "logging"})
	c.Assert(err, IsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, IsNil)

	s.principal0, err = s.service0.AddUnit()
	c.Assert(err, IsNil)
	err = s.principal0.AssignToMachine(s.machine1)
	c.Assert(err, IsNil)

	s.principal1, err = s.service0.AddUnit()
	c.Assert(err, IsNil)
	err = s.principal1.AssignToMachine(s.machine0)
	c.Assert(err, IsNil)

	relUnit0, err := rel.Unit(s.principal0)
	c.Assert(err, IsNil)
	err = relUnit0.EnterScope(nil)
	c.Assert(err, IsNil)
	s.subordinate0, err = s.service1.Unit("logging/0")
	c.Assert(err, IsNil)

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming machine 1 has logged in.
	s.authorizer = apitesting.FakeAuthorizer{
		Tag:          state.MachineTag(s.machine1.Id()),
		LoggedIn:     true,
		Manager:      false,
		MachineAgent: true,
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()

	// Create a deployer API for machine 1.
	deployer, err := deployer.NewDeployerAPI(
		s.State,
		s.resources,
		s.authorizer,
	)
	c.Assert(err, IsNil)
	s.deployer = deployer
}

func (s *deployerSuite) TestDeployerFailsWithNonMachineAgentUser(c *C) {
	anAuthorizer := s.authorizer
	anAuthorizer.MachineAgent = false
	aDeployer, err := deployer.NewDeployerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, NotNil)
	c.Assert(aDeployer, IsNil)
	c.Assert(err, ErrorMatches, "permission denied")
}

func (s *deployerSuite) TestWatchUnits(c *C) {
	c.Assert(s.resources.Count(), Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
		{Tag: "machine-0"},
		{Tag: "machine-42"},
	}}
	result, err := s.deployer.WatchUnits(args)
	c.Assert(err, IsNil)
	c.Assert(result, DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Changes: []string{"mysql/0", "logging/0"}, StringsWatcherId: "1"},
			{Error: apitesting.UnauthorizedError},
			{Error: apitesting.UnauthorizedError},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), Equals, 1)
	c.Assert(result.Results[0].StringsWatcherId, Equals, "1")
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *deployerSuite) TestSetPasswords(c *C) {
	results, err := s.deployer.SetPasswords(params.PasswordChanges{
		Changes: []params.PasswordChange{
			{Tag: "unit-mysql-0", Password: "xxx"},
			{Tag: "unit-mysql-1", Password: "yyy"},
			{Tag: "unit-logging-0", Password: "zzz"},
			{Tag: "unit-fake-42", Password: "abc"},
		},
	})
	c.Assert(err, IsNil)
	c.Assert(results, DeepEquals, params.ErrorResults{
		Errors: []*params.Error{
			nil,
			apitesting.UnauthorizedError,
			nil,
			apitesting.UnauthorizedError,
		},
	})
	err = s.principal0.Refresh()
	c.Assert(err, IsNil)
	changed := s.principal0.PasswordValid("xxx")
	c.Assert(changed, Equals, true)
	err = s.subordinate0.Refresh()
	c.Assert(err, IsNil)
	changed = s.subordinate0.PasswordValid("zzz")
	c.Assert(changed, Equals, true)
}

func (s *deployerSuite) TestLife(c *C) {
	err := s.subordinate0.EnsureDead()
	c.Assert(err, IsNil)
	err = s.subordinate0.Refresh()
	c.Assert(err, IsNil)
	c.Assert(s.subordinate0.Life(), Equals, state.Dead)
	err = s.principal0.Refresh()
	c.Assert(err, IsNil)
	c.Assert(s.principal0.Life(), Equals, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-mysql-1"},
		{Tag: "unit-logging-0"},
		{Tag: "unit-fake-42"},
	}}
	result, err := s.deployer.Life(args)
	c.Assert(err, IsNil)
	c.Assert(result, DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: "alive"},
			{Error: apitesting.UnauthorizedError},
			{Life: "dead"},
			{Error: apitesting.UnauthorizedError},
		},
	})
}

func (s *deployerSuite) TestRemove(c *C) {
	c.Assert(s.principal0.Life(), Equals, state.Alive)
	c.Assert(s.subordinate0.Life(), Equals, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-mysql-1"},
		{Tag: "unit-logging-0"},
		{Tag: "unit-fake-42"},
	}}
	result, err := s.deployer.Remove(args)
	c.Assert(err, IsNil)
	c.Assert(result, DeepEquals, params.ErrorResults{
		Errors: []*params.Error{
			&params.Error{
				Message: "unit has subordinates",
				Code:    "unit has subordinates",
			},
			apitesting.UnauthorizedError,
			nil,
			apitesting.UnauthorizedError,
		},
	})

	err = s.principal0.Refresh()
	c.Assert(err, IsNil)
	c.Assert(s.principal0.Life(), Equals, state.Alive)
	err = s.subordinate0.Refresh()
	c.Assert(err, NotNil)
	c.Assert(errors.IsNotFoundError(err), Equals, true)
}
