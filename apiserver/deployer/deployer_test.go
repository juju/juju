// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"sort"
	stdtesting "testing"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/deployer"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type deployerSuite struct {
	testing.JujuConnSuite

	authorizer apiservertesting.FakeAuthorizer

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

var _ = gc.Suite(&deployerSuite{})

func (s *deployerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// The two known machines now contain the following units:
	// machine 0 (not authorized): mysql/1 (principal1)
	// machine 1 (authorized): mysql/0 (principal0), logging/0 (subordinate0)

	var err error
	s.machine0, err = s.State.AddMachine("quantal", state.JobManageEnviron, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	s.machine1, err = s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	s.service0 = s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))

	s.service1 = s.AddTestingService(c, "logging", s.AddTestingCharm(c, "logging"))
	eps, err := s.State.InferEndpoints("mysql", "logging")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	s.principal0, err = s.service0.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.principal0.AssignToMachine(s.machine1)
	c.Assert(err, jc.ErrorIsNil)

	s.principal1, err = s.service0.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.principal1.AssignToMachine(s.machine0)
	c.Assert(err, jc.ErrorIsNil)

	relUnit0, err := rel.Unit(s.principal0)
	c.Assert(err, jc.ErrorIsNil)
	err = relUnit0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.subordinate0, err = s.State.Unit("logging/0")
	c.Assert(err, jc.ErrorIsNil)

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming machine 1 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.machine1.Tag(),
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	// Create a deployer API for machine 1.
	deployer, err := deployer.NewDeployerAPI(
		s.State,
		s.resources,
		s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.deployer = deployer
}

func (s *deployerSuite) TestDeployerFailsWithNonMachineAgentUser(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = s.AdminUserTag(c)
	aDeployer, err := deployer.NewDeployerAPI(s.State, s.resources, anAuthorizer)
	c.Assert(err, gc.NotNil)
	c.Assert(aDeployer, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *deployerSuite) TestWatchUnits(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "machine-1"},
		{Tag: "machine-0"},
		{Tag: "machine-42"},
	}}
	result, err := s.deployer.WatchUnits(args)
	c.Assert(err, jc.ErrorIsNil)
	sort.Strings(result.Results[0].Changes)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Changes: []string{"logging/0", "mysql/0"}, StringsWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	c.Assert(result.Results[0].StringsWatcherId, gc.Equals, "1")
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *deployerSuite) TestSetPasswords(c *gc.C) {
	args := params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: "unit-mysql-0", Password: "xxx-12345678901234567890"},
			{Tag: "unit-mysql-1", Password: "yyy-12345678901234567890"},
			{Tag: "unit-logging-0", Password: "zzz-12345678901234567890"},
			{Tag: "unit-fake-42", Password: "abc-12345678901234567890"},
		},
	}
	results, err := s.deployer.SetPasswords(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{nil},
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})
	err = s.principal0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	changed := s.principal0.PasswordValid("xxx-12345678901234567890")
	c.Assert(changed, jc.IsTrue)
	err = s.subordinate0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	changed = s.subordinate0.PasswordValid("zzz-12345678901234567890")
	c.Assert(changed, jc.IsTrue)

	// Remove the subordinate and make sure it's detected.
	err = s.subordinate0.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.subordinate0.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.subordinate0.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	results, err = s.deployer.SetPasswords(params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: "unit-logging-0", Password: "blah-12345678901234567890"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *deployerSuite) TestLife(c *gc.C) {
	err := s.subordinate0.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.subordinate0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.subordinate0.Life(), gc.Equals, state.Dead)
	err = s.principal0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.principal0.Life(), gc.Equals, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-mysql-1"},
		{Tag: "unit-logging-0"},
		{Tag: "unit-fake-42"},
	}}
	result, err := s.deployer.Life(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Life: "alive"},
			{Error: apiservertesting.ErrUnauthorized},
			{Life: "dead"},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Remove the subordinate and make sure it's detected.
	err = s.subordinate0.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.subordinate0.Remove()
	c.Assert(err, jc.ErrorIsNil)
	err = s.subordinate0.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	result, err = s.deployer.Life(params.Entities{
		Entities: []params.Entity{
			{Tag: "unit-logging-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *deployerSuite) TestRemove(c *gc.C) {
	c.Assert(s.principal0.Life(), gc.Equals, state.Alive)
	c.Assert(s.subordinate0.Life(), gc.Equals, state.Alive)

	// Try removing alive units - should fail.
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-mysql-1"},
		{Tag: "unit-logging-0"},
		{Tag: "unit-fake-42"},
	}}
	result, err := s.deployer.Remove(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{Message: `cannot remove entity "unit-mysql-0": still alive`}},
			{apiservertesting.ErrUnauthorized},
			{&params.Error{Message: `cannot remove entity "unit-logging-0": still alive`}},
			{apiservertesting.ErrUnauthorized},
		},
	})

	err = s.principal0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.principal0.Life(), gc.Equals, state.Alive)
	err = s.subordinate0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.subordinate0.Life(), gc.Equals, state.Alive)

	// Now make the subordinate dead and try again.
	err = s.subordinate0.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.subordinate0.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.subordinate0.Life(), gc.Equals, state.Dead)

	args = params.Entities{
		Entities: []params.Entity{{Tag: "unit-logging-0"}},
	}
	result, err = s.deployer.Remove(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{nil}},
	})

	err = s.subordinate0.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)

	// Make sure the subordinate is detected as removed.
	result, err = s.deployer.Remove(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{apiservertesting.ErrUnauthorized}},
	})
}

func (s *deployerSuite) TestStateAddresses(c *gc.C) {
	err := s.machine0.SetProviderAddresses(network.NewAddress("0.1.2.3"))
	c.Assert(err, jc.ErrorIsNil)

	addresses, err := s.State.Addresses()
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.deployer.StateAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsResult{
		Result: addresses,
	})
}

func (s *deployerSuite) TestAPIAddresses(c *gc.C) {
	hostPorts := [][]network.HostPort{
		network.NewHostPorts(1234, "0.1.2.3"),
	}
	err := s.State.SetAPIHostPorts(hostPorts)
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.deployer.APIAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsResult{
		Result: []string{"0.1.2.3:1234"},
	})
}

func (s *deployerSuite) TestCACert(c *gc.C) {
	result := s.deployer.CACert()
	c.Assert(result, gc.DeepEquals, params.BytesResult{
		Result: []byte(s.State.CACert()),
	})
}
