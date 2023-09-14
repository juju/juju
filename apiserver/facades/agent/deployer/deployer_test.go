// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"context"
	"sort"
	stdtesting "testing"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/deployer"
	"github.com/juju/juju/apiserver/facades/agent/deployer/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type mockLeadershipRevoker struct {
	revoked set.Strings
}

func (s *mockLeadershipRevoker) RevokeLeadership(applicationId, unitId string) error {
	s.revoked.Add(unitId)
	return nil
}

type deployerSuite struct {
	testing.ApiServerSuite

	authorizer apiservertesting.FakeAuthorizer

	service0     *state.Application
	machine0     *state.Machine
	machine1     *state.Machine
	principal0   *state.Unit
	principal1   *state.Unit
	subordinate0 *state.Unit

	resources *common.Resources
	deployer  *deployer.DeployerAPI
	revoker   *mockLeadershipRevoker

	controllerConfigGetter *mocks.MockControllerConfigGetter
}

var _ = gc.Suite(&deployerSuite{})

func (s *deployerSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	ctrl := gomock.NewController(c)
	s.controllerConfigGetter = mocks.NewMockControllerConfigGetter(ctrl)
	s.controllerConfigGetter.EXPECT().ControllerConfig(gomock.Any()).Return(s.ControllerConfigAttrs, nil).AnyTimes()

	// The two known machines now contain the following units:
	// machine 0 (not authorized): mysql/1 (principal1)
	// machine 1 (authorized): mysql/0 (principal0), logging/0 (subordinate0)

	st := s.ControllerModel(c).State()
	var err error
	s.machine0, err = st.AddMachine(state.UbuntuBase("12.10"), state.JobManageModel, state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	s.machine1, err = st.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	s.service0 = f.MakeApplication(c, nil)

	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "logging",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "logging"}),
	})
	eps, err := st.InferEndpoints("mysql", "logging")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	s.principal0, err = s.service0.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.principal0.AssignToMachine(s.machine1)
	c.Assert(err, jc.ErrorIsNil)

	s.principal1, err = s.service0.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.principal1.AssignToMachine(s.machine0)
	c.Assert(err, jc.ErrorIsNil)

	relUnit0, err := rel.Unit(s.principal0)
	c.Assert(err, jc.ErrorIsNil)
	err = relUnit0.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.subordinate0, err = st.Unit("logging/0")
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

	s.revoker = &mockLeadershipRevoker{revoked: set.NewStrings()}
	// Create a deployer API for machine 1.

	deployer, err := deployer.NewDeployerAPI(
		s.controllerConfigGetter,
		s.authorizer,
		st,
		s.resources,
		s.revoker,
		st,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.deployer = deployer
}

func (s *deployerSuite) TestDeployerFailsWithNonMachineAgentUser(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = testing.AdminUser
	aDeployer, err := deployer.NewDeployerFacade(
		facadetest.Context{
			Auth_:              anAuthorizer,
			LeadershipRevoker_: s.revoker,
			Resources_:         s.resources,
			State_:             s.ControllerModel(c).State(),
		},
	)
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
	result, err := s.deployer.WatchUnits(context.Background(), args)
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
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, resource.(state.StringsWatcher))
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
	results, err := s.deployer.SetPasswords(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
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
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	results, err = s.deployer.SetPasswords(context.Background(), params.EntityPasswords{
		Changes: []params.EntityPassword{
			{Tag: "unit-logging-0", Password: "blah-12345678901234567890"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: apiservertesting.ErrUnauthorized},
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
	result, err := s.deployer.Life(context.Background(), args)
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
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	result, err = s.deployer.Life(context.Background(), params.Entities{
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
	result, err := s.deployer.Remove(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: &params.Error{Message: `cannot remove entity "unit-mysql-0": still alive`}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: &params.Error{Message: `cannot remove entity "unit-logging-0": still alive`}},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
	c.Assert(s.revoker.revoked.IsEmpty(), jc.IsTrue)

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
	result, err = s.deployer.Remove(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})
	c.Assert(s.revoker.revoked.Contains("logging/0"), jc.IsTrue)

	err = s.subordinate0.Refresh()
	c.Assert(err, jc.ErrorIs, errors.NotFound)

	// Make sure the subordinate is detected as removed.
	result, err = s.deployer.Remove(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: apiservertesting.ErrUnauthorized}},
	})
}

func (s *deployerSuite) TestConnectionInfo(c *gc.C) {
	s.controllerConfigGetter.EXPECT().ControllerConfig(gomock.Any()).Return(s.ControllerConfigAttrs, nil).AnyTimes()

	st := s.ControllerModel(c).State()
	controllerConfig, err := s.controllerConfigGetter.ControllerConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	err = s.machine0.SetProviderAddresses(controllerConfig,
		network.NewSpaceAddress("0.1.2.3", network.WithScope(network.ScopePublic)),
		network.NewSpaceAddress("1.2.3.4", network.WithScope(network.ScopeCloudLocal)),
	)
	c.Assert(err, jc.ErrorIsNil)

	// Default host port scope is public, so change the cloud-local one
	hostPorts := network.NewSpaceHostPorts(1234, "0.1.2.3", "1.2.3.4")
	hostPorts[1].Scope = network.ScopeCloudLocal

	err = st.SetAPIHostPorts(controllerConfig, []network.SpaceHostPorts{hostPorts})
	c.Assert(err, jc.ErrorIsNil)

	expected := params.DeployerConnectionValues{
		APIAddresses: []string{"1.2.3.4:1234", "0.1.2.3:1234"},
	}

	result, err := s.deployer.ConnectionInfo(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, expected)
}

func (s *deployerSuite) TestSetStatus(c *gc.C) {
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: "unit-mysql-0", Status: "blocked", Info: "waiting", Data: map[string]interface{}{"foo": "bar"}},
			{Tag: "unit-mysql-1", Status: "blocked", Info: "waiting", Data: map[string]interface{}{"foo": "bar"}},
			{Tag: "unit-fake-42", Status: "blocked", Info: "waiting", Data: map[string]interface{}{"foo": "bar"}},
		},
	}
	results, err := s.deployer.SetStatus(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
	sInfo, err := s.principal0.Status()
	c.Assert(err, jc.ErrorIsNil)
	sInfo.Since = nil
	c.Assert(sInfo, jc.DeepEquals, status.StatusInfo{
		Status:  status.Blocked,
		Message: "waiting",
		Data:    map[string]interface{}{"foo": "bar"},
	})
}
