// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/api"
	apiuniter "github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/model"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

// uniterSuiteBase implements common testing suite for all API versions.
// It is not intended to be used directly or registered as a suite,
// but embedded.
type uniterSuiteBase struct {
	testing.JujuConnSuite

	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources
	uniter     *uniter.UniterAPI

	machine0      *state.Machine
	machine1      *state.Machine
	wpCharm       *state.Charm
	wordpress     *state.Application
	wordpressUnit *state.Unit
	mysqlCharm    *state.Charm
	mysql         *state.Application
	mysqlUnit     *state.Unit
}

func (s *uniterSuiteBase) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.setupState(c)

	s.State.StartSync()
	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming the wordpress unit has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.wordpressUnit.Tag(),
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.uniter = s.newUniterAPI(c, s.State, s.authorizer)
}

// setupState creates 2 machines, 2 services and adds a unit to each service.
func (s *uniterSuiteBase) setupState(c *gc.C) {
	s.machine0 = s.Factory.MakeMachine(c, &factory.MachineParams{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits, state.JobManageModel},
	})
	s.machine1 = s.Factory.MakeMachine(c, &factory.MachineParams{
		Series: "quantal",
		Jobs:   []state.MachineJob{state.JobHostUnits},
	})

	s.wpCharm = s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "wordpress",
		URL:  "cs:quantal/wordpress-3",
	})
	s.wordpress = s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: s.wpCharm,
	})
	s.wordpressUnit = s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.wordpress,
		Machine:     s.machine0,
	})

	s.mysqlCharm = s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "mysql",
	})
	s.mysql = s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "mysql",
		Charm: s.mysqlCharm,
	})
	s.mysqlUnit = s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.mysql,
		Machine:     s.machine1,
	})
}

func (s *uniterSuiteBase) newUniterAPI(c *gc.C, st *state.State, auth facade.Authorizer) *uniter.UniterAPI {
	uniterAPI, err := uniter.NewUniterAPI(facadetest.Context{
		Controller_:        s.Controller,
		State_:             st,
		Resources_:         s.resources,
		Auth_:              auth,
		LeadershipChecker_: s.State.LeadershipChecker(),
	})
	c.Assert(err, jc.ErrorIsNil)
	return uniterAPI
}

func (s *uniterSuiteBase) addRelation(c *gc.C, first, second string) *state.Relation {
	eps, err := s.State.InferEndpoints(first, second)
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	return rel
}

func (s *uniterSuiteBase) assertInScope(c *gc.C, relUnit *state.RelationUnit, inScope bool) {
	ok, err := relUnit.InScope()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ok, gc.Equals, inScope)
}

func (s *uniterSuiteBase) setupCAASModel(c *gc.C) (*apiuniter.State, *state.CAASModel, *state.Application, *state.Unit) {
	st := s.Factory.MakeCAASModel(c, nil)
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	s.CleanupSuite.AddCleanup(func(*gc.C) { _ = st.Close() })
	cm, err := m.CAASModel()
	c.Assert(err, jc.ErrorIsNil)

	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "gitlab", Series: "kubernetes"})
	app := f.MakeApplication(c, &factory.ApplicationParams{Name: "gitlab", Charm: ch})
	unit := f.MakeUnit(c, &factory.UnitParams{
		Application: app,
		SetCharmURL: true,
	})

	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)

	s.State.StartSync()
	s.WaitForModelWatchersIdle(c, m.UUID())

	apiInfo, err := environs.APIInfo(
		context.NewCloudCallContext(),
		s.ControllerConfig.ControllerUUID(),
		st.ModelUUID(),
		coretesting.CACert,
		s.ControllerConfig.APIPort(),
		s.Environ,
	)
	c.Assert(err, jc.ErrorIsNil)
	apiInfo.Tag = unit.Tag()
	apiInfo.Password = password
	apiState, err := api.Open(apiInfo, api.DialOpts{})
	c.Assert(err, jc.ErrorIsNil)
	s.CleanupSuite.AddCleanup(func(*gc.C) { _ = apiState.Close() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: unit.Tag(),
	}
	u, err := apiState.Uniter()
	c.Assert(err, jc.ErrorIsNil)
	return u, cm, app, unit
}

type uniterSuite struct {
	uniterSuiteBase
}

var _ = gc.Suite(&uniterSuite{})

func (s *uniterSuite) TestUniterFailsWithNonUnitAgentUser(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("9")
	_, err := uniter.NewUniterAPI(facadetest.Context{
		State_:     s.State,
		Resources_: s.resources,
		Auth_:      anAuthorizer,
	})
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *uniterSuite) TestSetStatus(c *gc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Executing,
		Message: "blah",
		Since:   &now,
	}
	err := s.wordpressUnit.SetAgentStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Executing,
		Message: "foo",
		Since:   &now,
	}
	err = s.mysqlUnit.SetAgentStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)

	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: "unit-mysql-0", Status: status.Error.String(), Info: "not really"},
			{Tag: "unit-wordpress-0", Status: status.Rebooting.String(), Info: "foobar"},
			{Tag: "unit-foo-42", Status: status.Active.String(), Info: "blah"},
		}}
	result, err := s.uniter.SetStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify mysqlUnit - no change.
	statusInfo, err := s.mysqlUnit.AgentStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Executing)
	c.Assert(statusInfo.Message, gc.Equals, "foo")
	// ...wordpressUnit is fine though.
	statusInfo, err = s.wordpressUnit.AgentStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Rebooting)
	c.Assert(statusInfo.Message, gc.Equals, "foobar")
}

func (s *uniterSuite) TestSetAgentStatus(c *gc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Executing,
		Message: "blah",
		Since:   &now,
	}
	err := s.wordpressUnit.SetAgentStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Executing,
		Message: "foo",
		Since:   &now,
	}
	err = s.mysqlUnit.SetAgentStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)

	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: "unit-mysql-0", Status: status.Error.String(), Info: "not really"},
			{Tag: "unit-wordpress-0", Status: status.Executing.String(), Info: "foobar"},
			{Tag: "unit-foo-42", Status: status.Rebooting.String(), Info: "blah"},
		}}
	result, err := s.uniter.SetAgentStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify mysqlUnit - no change.
	statusInfo, err := s.mysqlUnit.AgentStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Executing)
	c.Assert(statusInfo.Message, gc.Equals, "foo")
	// ...wordpressUnit is fine though.
	statusInfo, err = s.wordpressUnit.AgentStatus()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Executing)
	c.Assert(statusInfo.Message, gc.Equals, "foobar")
}

func (s *uniterSuite) TestSetUnitStatus(c *gc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "blah",
		Since:   &now,
	}
	err := s.wordpressUnit.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Terminated,
		Message: "foo",
		Since:   &now,
	}
	err = s.mysqlUnit.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)

	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{Tag: "unit-mysql-0", Status: status.Error.String(), Info: "not really"},
			{Tag: "unit-wordpress-0", Status: status.Terminated.String(), Info: "foobar"},
			{Tag: "unit-foo-42", Status: status.Active.String(), Info: "blah"},
		}}
	result, err := s.uniter.SetUnitStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify mysqlUnit - no change.
	statusInfo, err := s.mysqlUnit.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Terminated)
	c.Assert(statusInfo.Message, gc.Equals, "foo")
	// ...wordpressUnit is fine though.
	statusInfo, err = s.wordpressUnit.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(statusInfo.Status, gc.Equals, status.Terminated)
	c.Assert(statusInfo.Message, gc.Equals, "foobar")
}

func (s *uniterSuite) TestLife(c *gc.C) {
	// Add a relation wordpress-mysql.
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rel.Life(), gc.Equals, state.Alive)
	relStatus, err := rel.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(relStatus.Status, gc.Equals, status.Joining)

	// Make the wordpressUnit dead.
	err = s.wordpressUnit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.wordpressUnit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Dead)

	// Add another unit, so the service will stay dying when we
	// destroy it later.
	extraUnit, err := s.wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(extraUnit, gc.NotNil)

	// Make the wordpress service dying.
	err = s.wordpress.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.wordpress.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.wordpress.Life(), gc.Equals, state.Dying)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "application-mysql"},
		{Tag: "application-wordpress"},
		{Tag: "machine-0"},
		{Tag: "machine-1"},
		{Tag: "machine-42"},
		{Tag: "application-foo"},
		// TODO(dfc) these aren't valid tags any more
		// but I hope to restore this test when params.Entity takes
		// tags, not strings, which is coming soon.
		// {Tag: "just-foo"},
		{Tag: rel.Tag().String()},
		{Tag: "relation-svc1.rel1#svc2.rel2"},
		// {Tag: "relation-blah"},
	}}
	result, err := s.uniter.Life(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.LifeResults{
		Results: []params.LifeResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Life: "dead"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Life: "dying"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			// TODO(dfc) see above
			// {Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			// {Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestEnsureDead(c *gc.C) {
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Alive)
	c.Assert(s.mysqlUnit.Life(), gc.Equals, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.EnsureDead(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	err = s.wordpressUnit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Dead)
	err = s.mysqlUnit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.mysqlUnit.Life(), gc.Equals, state.Alive)

	// Try it again on a Dead unit; should work.
	args = params.Entities{
		Entities: []params.Entity{{Tag: "unit-wordpress-0"}},
	}
	result, err = s.uniter.EnsureDead(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{nil}},
	})

	// Verify Life is unchanged.
	err = s.wordpressUnit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Dead)
}

func (s *uniterSuite) TestWatch(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "application-mysql"},
		{Tag: "application-wordpress"},
		{Tag: "application-foo"},
		// TODO(dfc) these aren't valid tags any more
		// but I hope to restore this test when params.Entity takes
		// tags, not strings, which is coming soon.
		// {Tag: "just-foo"},
	}}
	result, err := s.uniter.Watch(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{NotifyWatcherId: "2"},
			{Error: apiservertesting.ErrUnauthorized},
			// see above
			// {Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 2)
	resource1 := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource1)
	resource2 := s.resources.Get("2")
	defer statetesting.AssertStop(c, resource2)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, s.State, resource1.(state.NotifyWatcher))
	wc.AssertNoChange()
	wc = statetesting.NewNotifyWatcherC(c, s.State, resource2.(state.NotifyWatcher))
	wc.AssertNoChange()
}

func (s *uniterSuite) TestPublicAddress(c *gc.C) {
	// Try first without setting an address.
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	expectErr := &params.Error{
		Code:    params.CodeNoAddressSet,
		Message: `"unit-wordpress-0" has no public address set`,
	}
	result, err := s.uniter.PublicAddress(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: expectErr},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Now set it an try again.
	err = s.machine0.SetProviderAddresses(
		network.NewScopedAddress("1.2.3.4", network.ScopePublic),
	)
	c.Assert(err, jc.ErrorIsNil)
	address, err := s.wordpressUnit.PublicAddress()
	c.Assert(address.Value, gc.Equals, "1.2.3.4")
	c.Assert(err, jc.ErrorIsNil)

	result, err = s.uniter.PublicAddress(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: "1.2.3.4"},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestPrivateAddress(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	expectErr := &params.Error{
		Code:    params.CodeNoAddressSet,
		Message: `"unit-wordpress-0" has no private address set`,
	}
	result, err := s.uniter.PrivateAddress(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: expectErr},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Now set it and try again.
	err = s.machine0.SetProviderAddresses(
		network.NewScopedAddress("1.2.3.4", network.ScopeCloudLocal),
	)
	c.Assert(err, jc.ErrorIsNil)
	address, err := s.wordpressUnit.PrivateAddress()
	c.Assert(address.Value, gc.Equals, "1.2.3.4")
	c.Assert(err, jc.ErrorIsNil)

	result, err = s.uniter.PrivateAddress(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: "1.2.3.4"},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

// TestNetworkInfoSpaceless is in uniterSuite and not uniterNetworkInfoSuite since we don't want
// all the spaces set up.
func (s *uniterSuite) TestNetworkInfoSpaceless(c *gc.C) {
	err := s.machine0.SetProviderAddresses(
		network.NewScopedAddress("1.2.3.4", network.ScopeCloudLocal),
	)
	err = s.Model.UpdateModelConfig(map[string]interface{}{config.EgressSubnets: "10.0.0.0/8"}, nil)
	c.Assert(err, jc.ErrorIsNil)

	args := params.NetworkInfoParams{
		Unit:     s.wordpressUnit.Tag().String(),
		Bindings: []string{"db"},
	}

	privateAddress, err := s.machine0.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)

	expectedInfo := params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				Addresses: []params.InterfaceAddress{
					{Address: privateAddress.Value},
				},
			},
		},
		EgressSubnets:    []string{"10.0.0.0/8"},
		IngressAddresses: []string{privateAddress.Value},
	}

	result, err := s.uniter.NetworkInfo(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, params.NetworkInfoResults{
		Results: map[string]params.NetworkInfoResult{
			"db": expectedInfo,
		},
	})
}

func (s *uniterSuite) TestAvailabilityZone(c *gc.C) {
	s.PatchValue(uniter.GetZone, func(st *state.State, tag names.Tag) (string, error) {
		return "a_zone", nil
	})

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-wordpress-0"},
	}}
	result, err := s.uniter.AvailabilityZone(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Result: "a_zone"},
		},
	})
}

func (s *uniterSuite) TestResolvedAPIV6(c *gc.C) {
	err := s.wordpressUnit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, jc.ErrorIsNil)
	mode := s.wordpressUnit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedRetryHooks)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.Resolved(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ResolvedModeResults{
		Results: []params.ResolvedModeResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Mode: params.ResolvedMode(mode)},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestClearResolved(c *gc.C) {
	err := s.wordpressUnit.SetResolved(state.ResolvedRetryHooks)
	c.Assert(err, jc.ErrorIsNil)
	mode := s.wordpressUnit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedRetryHooks)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.ClearResolved(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify wordpressUnit's resolved mode has changed.
	err = s.wordpressUnit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	mode = s.wordpressUnit.Resolved()
	c.Assert(mode, gc.Equals, state.ResolvedNone)
}

func (s *uniterSuite) TestGetPrincipal(c *gc.C) {
	// Add a subordinate to wordpressUnit.
	_, _, subordinate := s.addRelatedApplication(c, "wordpress", "logging", s.wordpressUnit)

	principal, ok := subordinate.PrincipalName()
	c.Assert(principal, gc.Equals, s.wordpressUnit.Name())
	c.Assert(ok, jc.IsTrue)

	// First try it as wordpressUnit's agent.
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: subordinate.Tag().String()},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.GetPrincipal(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringBoolResults{
		Results: []params.StringBoolResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: "", Ok: false, Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Now try as subordinate's agent.
	subAuthorizer := s.authorizer
	subAuthorizer.Tag = subordinate.Tag()
	subUniter := s.newUniterAPI(c, s.State, subAuthorizer)

	result, err = subUniter.GetPrincipal(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringBoolResults{
		Results: []params.StringBoolResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Result: "unit-wordpress-0", Ok: true, Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestHasSubordinates(c *gc.C) {
	// Try first without any subordinates for wordpressUnit.
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-logging-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.HasSubordinates(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: false},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Add two subordinates to wordpressUnit and try again.
	s.addRelatedApplication(c, "wordpress", "logging", s.wordpressUnit)
	s.addRelatedApplication(c, "wordpress", "monitoring", s.wordpressUnit)

	result, err = s.uniter.HasSubordinates(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.BoolResults{
		Results: []params.BoolResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: true},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestDestroy(c *gc.C) {
	c.Assert(s.wordpressUnit.Life(), gc.Equals, state.Alive)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.Destroy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify wordpressUnit is destroyed and removed.
	err = s.wordpressUnit.Refresh()
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *uniterSuite) TestDestroyAllSubordinates(c *gc.C) {
	// Add two subordinates to wordpressUnit.
	_, _, loggingSub := s.addRelatedApplication(c, "wordpress", "logging", s.wordpressUnit)
	_, _, monitoringSub := s.addRelatedApplication(c, "wordpress", "monitoring", s.wordpressUnit)
	c.Assert(loggingSub.Life(), gc.Equals, state.Alive)
	c.Assert(monitoringSub.Life(), gc.Equals, state.Alive)

	err := s.wordpressUnit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	subordinates := s.wordpressUnit.SubordinateNames()
	c.Assert(subordinates, gc.DeepEquals, []string{"logging/0", "monitoring/0"})

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.DestroyAllSubordinates(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify wordpressUnit's subordinates were destroyed.
	err = loggingSub.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(loggingSub.Life(), gc.Equals, state.Dying)
	err = monitoringSub.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(monitoringSub.Life(), gc.Equals, state.Dying)
}

func (s *uniterSuite) TestCharmURL(c *gc.C) {
	// Set wordpressUnit's charm URL first.
	err := s.wordpressUnit.SetCharmURL(s.wpCharm.URL())
	c.Assert(err, jc.ErrorIsNil)
	curl, ok := s.wordpressUnit.CharmURL()
	c.Assert(curl, gc.DeepEquals, s.wpCharm.URL())
	c.Assert(ok, jc.IsTrue)

	// Make sure wordpress application's charm is what we expect.
	curl, force := s.wordpress.CharmURL()
	c.Assert(curl, gc.DeepEquals, s.wpCharm.URL())
	c.Assert(force, jc.IsFalse)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "application-mysql"},
		{Tag: "application-wordpress"},
		{Tag: "application-foo"},
		// TODO(dfc) these aren't valid tags any more
		// but I hope to restore this test when params.Entity takes
		// tags, not strings, which is coming soon.
		// {Tag: "just-foo"},
	}}
	result, err := s.uniter.CharmURL(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringBoolResults{
		Results: []params.StringBoolResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: s.wpCharm.String(), Ok: ok},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Result: s.wpCharm.String(), Ok: force},
			{Error: apiservertesting.ErrUnauthorized},
			// see above
			// {Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestSetCharmURL(c *gc.C) {
	_, ok := s.wordpressUnit.CharmURL()
	c.Assert(ok, jc.IsFalse)

	args := params.EntitiesCharmURL{Entities: []params.EntityCharmURL{
		{Tag: "unit-mysql-0", CharmURL: "cs:quantal/application-42"},
		{Tag: "unit-wordpress-0", CharmURL: s.wpCharm.String()},
		{Tag: "unit-foo-42", CharmURL: "cs:quantal/foo-321"},
	}}
	result, err := s.uniter.SetCharmURL(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the charm URL was set.
	err = s.wordpressUnit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	charmURL, needsUpgrade := s.wordpressUnit.CharmURL()
	c.Assert(charmURL, gc.NotNil)
	c.Assert(charmURL.String(), gc.Equals, s.wpCharm.String())
	c.Assert(needsUpgrade, jc.IsTrue)
}

func (s *uniterSuite) TestWorkloadVersion(c *gc.C) {
	// Set wordpressUnit's workload version first.
	err := s.wordpressUnit.SetWorkloadVersion("capulet")
	c.Assert(err, jc.ErrorIsNil)
	version, err := s.wordpressUnit.WorkloadVersion()
	c.Assert(version, gc.Equals, "capulet")
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "application-wordpress"},
		{Tag: "just-foo"},
	}}

	result, err := s.uniter.WorkloadVersion(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: "capulet"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: common.ServerError(errors.New(`"application-wordpress" is not a valid unit tag`))},
			{Error: common.ServerError(errors.New(`"just-foo" is not a valid tag`))},
		},
	})
}

func (s *uniterSuite) TestSetWorkloadVersion(c *gc.C) {
	currentVersion, err := s.wordpressUnit.WorkloadVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(currentVersion, gc.Equals, "")

	args := params.EntityWorkloadVersions{Entities: []params.EntityWorkloadVersion{
		{Tag: "unit-mysql-0", WorkloadVersion: "allura"},
		{Tag: "unit-wordpress-0", WorkloadVersion: "shiro"},
		{Tag: "unit-foo-42", WorkloadVersion: "pidge"},
	}}
	result, err := s.uniter.SetWorkloadVersion(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the workload version was set.
	err = s.wordpressUnit.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	newVersion, err := s.wordpressUnit.WorkloadVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newVersion, gc.Equals, "shiro")
}

func (s *uniterSuite) TestCharmModifiedVersion(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "application-mysql"},
		{Tag: "application-wordpress"},
		{Tag: "unit-wordpress-0"},
		{Tag: "application-foo"},
	}}
	result, err := s.uniter.CharmModifiedVersion(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.IntResults{
		Results: []params.IntResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: s.wordpress.CharmModifiedVersion()},
			{Result: s.wordpress.CharmModifiedVersion()},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestOpenPorts(c *gc.C) {
	openedPorts, err := s.wordpressUnit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(openedPorts, gc.HasLen, 0)

	args := params.EntitiesPortRanges{Entities: []params.EntityPortRange{
		{Tag: "unit-mysql-0", Protocol: "tcp", FromPort: 1234, ToPort: 1400},
		{Tag: "unit-wordpress-0", Protocol: "udp", FromPort: 4321, ToPort: 5000},
		{Tag: "unit-foo-42", Protocol: "tcp", FromPort: 42, ToPort: 42},
	}}
	result, err := s.uniter.OpenPorts(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the wordpressUnit's port is opened.
	openedPorts, err = s.wordpressUnit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(openedPorts, gc.DeepEquals, []corenetwork.PortRange{
		{Protocol: "udp", FromPort: 4321, ToPort: 5000},
	})
}

func (s *uniterSuite) TestClosePorts(c *gc.C) {
	// Open port udp:4321 in advance on wordpressUnit.
	err := s.wordpressUnit.OpenPorts("udp", 4321, 5000)
	c.Assert(err, jc.ErrorIsNil)
	openedPorts, err := s.wordpressUnit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(openedPorts, gc.DeepEquals, []corenetwork.PortRange{
		{Protocol: "udp", FromPort: 4321, ToPort: 5000},
	})

	args := params.EntitiesPortRanges{Entities: []params.EntityPortRange{
		{Tag: "unit-mysql-0", Protocol: "tcp", FromPort: 1234, ToPort: 1400},
		{Tag: "unit-wordpress-0", Protocol: "udp", FromPort: 4321, ToPort: 5000},
		{Tag: "unit-foo-42", Protocol: "tcp", FromPort: 42, ToPort: 42},
	}}
	result, err := s.uniter.ClosePorts(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the wordpressUnit's port is closed.
	openedPorts, err = s.wordpressUnit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(openedPorts, gc.HasLen, 0)
}

func (s *uniterSuite) TestWatchConfigSettingsHash(c *gc.C) {
	err := s.wordpressUnit.SetCharmURL(s.wpCharm.URL())
	c.Assert(err, jc.ErrorIsNil)
	err = s.wordpress.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"blog-title": "sauceror central"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	s.State.StartSync()
	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.WatchConfigSettingsHash(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{
				StringsWatcherId: "1",
				// See core/cache/hash.go for the hash implementation.
				Changes: []string{"754ed70cf17d2df2cc6a2dcb6cbfcb569a8357b97b5708e7a7ca0409505e1d0b"},
			},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *uniterSuite) TestWatchTrustConfigSettingsHash(c *gc.C) {
	schema := environschema.Fields{
		"trust": environschema.Attr{Type: environschema.Tbool},
	}
	err := s.wordpress.UpdateApplicationConfig(coreapplication.ConfigAttributes{
		"trust": true,
	}, nil, schema, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.WatchTrustConfigSettingsHash(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{
				StringsWatcherId: "1",
				Changes:          []string{"2f1368bde39be8106dcdca15e35cc3b5f7db5b8e429806369f621a47fb938519"},
			},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *uniterSuite) TestWatchActionNotifications(c *gc.C) {
	err := s.wordpressUnit.SetCharmURL(s.wpCharm.URL())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.WatchActionNotifications(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{StringsWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, resource.(state.StringsWatcher))
	wc.AssertNoChange()

	addedAction, err := s.wordpressUnit.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(addedAction.Id())
	wc.AssertNoChange()
}

func (s *uniterSuite) TestWatchPreexistingActions(c *gc.C) {
	err := s.wordpressUnit.SetCharmURL(s.wpCharm.URL())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.resources.Count(), gc.Equals, 0)

	action1, err := s.wordpressUnit.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)
	action2, err := s.wordpressUnit.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-wordpress-0"},
	}}

	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())
	results, err := s.uniter.WatchActionNotifications(args)
	c.Assert(err, jc.ErrorIsNil)

	checkUnorderedActionIdsEqual(c, []string{action1.Id(), action2.Id()}, results)

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, resource.(state.StringsWatcher))
	wc.AssertNoChange()

	addedAction, err := s.wordpressUnit.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(addedAction.Id())
	wc.AssertNoChange()
}

func (s *uniterSuite) TestWatchActionNotificationsMalformedTag(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "ewenit-mysql-0"},
	}}
	results, err := s.uniter.WatchActionNotifications(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.NotNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.NotNil)
	c.Assert(result.Error.Message, gc.Equals, `invalid actionreceiver tag "ewenit-mysql-0"`)
}

func (s *uniterSuite) TestWatchActionNotificationsMalformedUnitName(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-01"},
	}}
	results, err := s.uniter.WatchActionNotifications(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.NotNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.NotNil)
	c.Assert(result.Error.Message, gc.Equals, `invalid actionreceiver tag "unit-mysql-01"`)
}

func (s *uniterSuite) TestWatchActionNotificationsNotUnit(c *gc.C) {
	action, err := s.mysqlUnit.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)
	args := params.Entities{Entities: []params.Entity{
		{Tag: action.Tag().String()},
	}}
	results, err := s.uniter.WatchActionNotifications(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.NotNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.NotNil)
	c.Assert(result.Error.Message, gc.Equals, `invalid actionreceiver tag "action-`+action.Id()+`"`)
}

func (s *uniterSuite) TestWatchActionNotificationsPermissionDenied(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-nonexistentgarbage-0"},
	}}
	results, err := s.uniter.WatchActionNotifications(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.NotNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.NotNil)
	c.Assert(result.Error.Message, gc.Equals, "permission denied")
}

func (s *uniterSuite) TestConfigSettings(c *gc.C) {
	err := s.wordpressUnit.SetCharmURL(s.wpCharm.URL())
	c.Assert(err, jc.ErrorIsNil)

	s.State.StartSync()
	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())

	settings, err := s.wordpressUnit.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "My Title"})

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.ConfigSettings(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ConfigSettingsResults{
		Results: []params.ConfigSettingsResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Settings: params.ConfigSettings{"blog-title": "My Title"}},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestWatchUnitRelations(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-0"},
	}}
	result, err := s.uniter.WatchUnitRelations(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results[0].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
	c.Assert(result.Results[1].StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Results[1].Changes, gc.NotNil)
	c.Assert(result.Results[1].Error, gc.IsNil)
	c.Assert(result.Results[2].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *uniterSuite) TestWatchSubordinateUnitRelations(c *gc.C) {
	// The logging charm is subordinate (and the info endpoint is scope=container).
	loggingCharm := s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "logging",
		URL:  "cs:quantal/logging-1",
	})
	loggingApp := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "logging",
		Charm: loggingCharm,
	})

	mysqlRel := s.makeSubordinateRelation(c, loggingApp, s.mysql, s.mysqlUnit)
	wpRel := s.makeSubordinateRelation(c, loggingApp, s.wordpress, s.wordpressUnit)
	mysqlLogUnit := findSubordinateUnit(c, loggingApp, s.mysqlUnit)

	subAuthorizer := s.authorizer
	subAuthorizer.Tag = mysqlLogUnit.Tag()
	uniterAPI := s.newUniterAPI(c, s.State, subAuthorizer)

	result, err := uniterAPI.WatchUnitRelations(params.Entities{
		Entities: []params.Entity{{Tag: mysqlLogUnit.Tag().String()}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Results[0].Changes, gc.NotNil)

	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, resource.(state.StringsWatcher))
	wc.AssertNoChange()

	// We get notified about the mysql relation going away but not the
	// wordpress one.
	err = mysqlRel.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(mysqlRel.Tag().Id())
	wc.AssertNoChange()

	err = wpRel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *uniterSuite) TestWatchUnitRelationsSubordinateWithGlobalEndpoint(c *gc.C) {
	// A subordinate unit should still be notified about changes to
	// relations with applications that aren't the one this unit is
	// attached to if they have global scope.
	// The logging charm is subordinate (and the info endpoint is scope=container).
	loggingCharm := s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "logging",
		URL:  "cs:quantal/logging-1",
	})
	loggingApp := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "logging",
		Charm: loggingCharm,
	})

	uiCharm := s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "logging-frontend",
		URL:  "cs:quantal/logging-frontend-1",
	})
	uiApp := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "logging-frontend",
		Charm: uiCharm,
	})

	_ = s.makeSubordinateRelation(c, loggingApp, s.mysql, s.mysqlUnit)
	mysqlLogUnit := findSubordinateUnit(c, loggingApp, s.mysqlUnit)

	subAuthorizer := s.authorizer
	subAuthorizer.Tag = mysqlLogUnit.Tag()
	uniterAPI := s.newUniterAPI(c, s.State, subAuthorizer)

	result, err := uniterAPI.WatchUnitRelations(params.Entities{
		Entities: []params.Entity{{Tag: mysqlLogUnit.Tag().String()}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Results[0].Changes, gc.NotNil)

	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	wc := statetesting.NewStringsWatcherC(c, s.State, resource.(state.StringsWatcher))
	wc.AssertNoChange()

	// Should be notified about the relation to logging frontend, since it's global scope.
	subEndpoint, err := loggingApp.Endpoint("logging-client")
	c.Assert(err, jc.ErrorIsNil)
	uiEndpoint, err := uiApp.Endpoint("logging-client")
	c.Assert(err, jc.ErrorIsNil)
	rel := s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{subEndpoint, uiEndpoint},
	})

	wc.AssertChange(rel.Tag().Id())
	wc.AssertNoChange()
}

func (s *uniterSuite) TestWatchUnitRelationsWithSubSubRelation(c *gc.C) {
	// We should be notified about relations to other subordinates
	// (since it's possible that they'll be colocated in the same
	// container).
	loggingCharm := s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "logging",
		URL:  "cs:quantal/logging-1",
	})
	loggingApp := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "logging",
		Charm: loggingCharm,
	})
	monitoringCharm := s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "monitoring",
		URL:  "cs:quantal/monitoring-1",
	})
	monitoringApp := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "monitoring",
		Charm: monitoringCharm,
	})

	s.makeSubordinateRelation(c, loggingApp, s.mysql, s.mysqlUnit)
	mysqlMonitoring := s.makeSubordinateRelation(c, monitoringApp, s.mysql, s.mysqlUnit)

	monUnit := findSubordinateUnit(c, monitoringApp, s.mysqlUnit)

	subAuthorizer := s.authorizer
	subAuthorizer.Tag = monUnit.Tag()
	uniterAPI := s.newUniterAPI(c, s.State, subAuthorizer)

	result, err := uniterAPI.WatchUnitRelations(params.Entities{
		Entities: []params.Entity{{Tag: monUnit.Tag().String()}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Results[0].Changes, gc.DeepEquals, []string{mysqlMonitoring.Tag().Id()})

	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, resource.(state.StringsWatcher))
	wc.AssertNoChange()

	// Now we relate logging and monitoring together.
	monEp, err := monitoringApp.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)

	logEp, err := loggingApp.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	rel := s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{monEp, logEp},
	})
	c.Assert(err, jc.ErrorIsNil)

	// We should be told about the new logging-monitoring relation.
	wc.AssertChange(rel.Tag().Id())
	wc.AssertNoChange()

	err = rel.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(rel.Tag().Id())
	wc.AssertNoChange()
}

func (s *uniterSuite) makeSubordinateRelation(c *gc.C, subApp, principalApp *state.Application, principalUnit *state.Unit) *state.Relation {
	subEndpoint, err := subApp.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)

	principalEndpoint, err := principalApp.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	rel := s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{subEndpoint, principalEndpoint},
	})
	c.Assert(err, jc.ErrorIsNil)
	// Trigger the creation of the subordinate unit by entering scope
	// on the principal unit.
	ru, err := rel.Unit(principalUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	return rel
}

func findSubordinateUnit(c *gc.C, subApp *state.Application, principalUnit *state.Unit) *state.Unit {
	subUnits, err := subApp.AllUnits()
	c.Assert(err, jc.ErrorIsNil)
	for _, subUnit := range subUnits {
		principal, ok := subUnit.PrincipalName()
		c.Assert(ok, jc.IsTrue)
		if principal == principalUnit.Name() {
			return subUnit
		}
	}
	c.Fatalf("couldn't find subordinate unit for %q", principalUnit.Name())
	return nil
}

func (s *uniterSuite) TestCharmArchiveSha256(c *gc.C) {
	dummyCharm := s.AddTestingCharm(c, "dummy")

	args := params.CharmURLs{URLs: []params.CharmURL{
		{URL: "something-invalid"},
		{URL: s.wpCharm.String()},
		{URL: dummyCharm.String()},
	}}
	result, err := s.uniter.CharmArchiveSha256(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: s.wpCharm.BundleSha256()},
			{Result: dummyCharm.BundleSha256()},
		},
	})
}

func (s *uniterSuite) TestCurrentModel(c *gc.C) {
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.uniter.CurrentModel()
	c.Assert(err, jc.ErrorIsNil)
	expected := params.ModelResult{
		Name: model.Name(),
		UUID: model.UUID(),
		Type: "iaas",
	}
	c.Assert(result, gc.DeepEquals, expected)
}

func (s *uniterSuite) TestActions(c *gc.C) {
	var actionTests = []struct {
		description string
		action      params.ActionResult
	}{{
		description: "A simple action.",
		action: params.ActionResult{
			Action: &params.Action{
				Name: "fakeaction",
				Parameters: map[string]interface{}{
					"outfile": "foo.txt",
				}},
		},
	}, {
		description: "An action with nested parameters.",
		action: params.ActionResult{
			Action: &params.Action{
				Name: "fakeaction",
				Parameters: map[string]interface{}{
					"outfile": "foo.bz2",
					"compression": map[string]interface{}{
						"kind":    "bzip",
						"quality": 5,
					},
				}},
		},
	}}

	for i, actionTest := range actionTests {
		c.Logf("test %d: %s", i, actionTest.description)

		a, err := s.wordpressUnit.AddAction(
			actionTest.action.Action.Name,
			actionTest.action.Action.Parameters)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(names.IsValidAction(a.Id()), gc.Equals, true)
		actionTag := names.NewActionTag(a.Id())
		c.Assert(a.ActionTag(), gc.Equals, actionTag)

		args := params.Entities{
			Entities: []params.Entity{{
				Tag: actionTag.String(),
			}},
		}
		results, err := s.uniter.Actions(args)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(results.Results, gc.HasLen, 1)

		actionsQueryResult := results.Results[0]

		c.Assert(actionsQueryResult, jc.DeepEquals, actionTest.action)
	}
}

func (s *uniterSuite) TestActionsNotPresent(c *gc.C) {
	uuid, err := utils.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: names.NewActionTag(uuid.String()).String(),
		}},
	}
	results, err := s.uniter.Actions(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results.Results, gc.HasLen, 1)
	actionsQueryResult := results.Results[0]
	c.Assert(actionsQueryResult.Error, gc.NotNil)
	c.Assert(actionsQueryResult.Error, gc.ErrorMatches, `action "[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}" not found`)
}

func (s *uniterSuite) TestActionsWrongUnit(c *gc.C) {
	// Action doesn't match unit.
	mysqlUnitAuthorizer := apiservertesting.FakeAuthorizer{
		Tag: s.mysqlUnit.Tag(),
	}
	mysqlUnitFacade := s.newUniterAPI(c, s.State, mysqlUnitAuthorizer)

	action, err := s.wordpressUnit.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: action.Tag().String(),
		}},
	}
	actions, err := mysqlUnitFacade.Actions(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(actions.Results), gc.Equals, 1)
	c.Assert(actions.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *uniterSuite) TestActionsPermissionDenied(c *gc.C) {
	action, err := s.mysqlUnit.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: action.Tag().String(),
		}},
	}
	actions, err := s.uniter.Actions(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(actions.Results), gc.Equals, 1)
	c.Assert(actions.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *uniterSuite) TestFinishActionsSuccess(c *gc.C) {
	testName := "fakeaction"
	testOutput := map[string]interface{}{"output": "completed fakeaction successfully"}

	results, err := s.wordpressUnit.CompletedActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, ([]state.Action)(nil))

	action, err := s.wordpressUnit.AddAction(testName, nil)
	c.Assert(err, jc.ErrorIsNil)

	actionResults := params.ActionExecutionResults{
		Results: []params.ActionExecutionResult{{
			ActionTag: action.ActionTag().String(),
			Status:    params.ActionCompleted,
			Results:   testOutput,
		}},
	}
	res, err := s.uniter.FinishActions(actionResults)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}})

	results, err = s.wordpressUnit.CompletedActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(results), gc.Equals, 1)
	c.Assert(results[0].Status(), gc.Equals, state.ActionCompleted)
	res2, errstr := results[0].Results()
	c.Assert(errstr, gc.Equals, "")
	c.Assert(res2, gc.DeepEquals, testOutput)
	c.Assert(results[0].Name(), gc.Equals, testName)
}

func (s *uniterSuite) TestFinishActionsFailure(c *gc.C) {
	testName := "fakeaction"
	testError := "fakeaction was a dismal failure"

	results, err := s.wordpressUnit.CompletedActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, ([]state.Action)(nil))

	action, err := s.wordpressUnit.AddAction(testName, nil)
	c.Assert(err, jc.ErrorIsNil)

	actionResults := params.ActionExecutionResults{
		Results: []params.ActionExecutionResult{{
			ActionTag: action.ActionTag().String(),
			Status:    params.ActionFailed,
			Results:   nil,
			Message:   testError,
		}},
	}
	res, err := s.uniter.FinishActions(actionResults)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{Error: nil}}})

	results, err = s.wordpressUnit.CompletedActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(results), gc.Equals, 1)
	c.Assert(results[0].Status(), gc.Equals, state.ActionFailed)
	res2, errstr := results[0].Results()
	c.Assert(errstr, gc.Equals, testError)
	c.Assert(res2, gc.DeepEquals, map[string]interface{}{})
	c.Assert(results[0].Name(), gc.Equals, testName)
}

func (s *uniterSuite) TestFinishActionsAuthAccess(c *gc.C) {
	good, err := s.wordpressUnit.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)

	bad, err := s.mysqlUnit.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)

	var tests = []struct {
		actionTag names.ActionTag
		err       error
	}{
		{actionTag: good.ActionTag(), err: nil},
		{actionTag: bad.ActionTag(), err: common.ErrPerm},
	}

	// Queue up actions from tests
	actionResults := params.ActionExecutionResults{Results: make([]params.ActionExecutionResult, len(tests))}
	for i, test := range tests {
		actionResults.Results[i] = params.ActionExecutionResult{
			ActionTag: test.actionTag.String(),
			Status:    params.ActionCompleted,
			Results:   map[string]interface{}{},
		}
	}

	// Invoke FinishActions
	res, err := s.uniter.FinishActions(actionResults)
	c.Assert(err, jc.ErrorIsNil)

	// Verify permissions errors for actions queued on different unit
	for i, result := range res.Results {
		expected := tests[i].err
		if expected != nil {
			c.Assert(result.Error, gc.NotNil)
			c.Assert(result.Error.Error(), gc.Equals, expected.Error())
		} else {
			c.Assert(result.Error, gc.IsNil)
		}
	}
}

func (s *uniterSuite) TestBeginActions(c *gc.C) {
	ten_seconds_ago := time.Now().Add(-10 * time.Second)
	good, err := s.wordpressUnit.AddAction("fakeaction", nil)
	c.Assert(err, jc.ErrorIsNil)

	running, err := s.wordpressUnit.RunningActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(running), gc.Equals, 0, gc.Commentf("expected no running actions, got %d", len(running)))

	args := params.Entities{Entities: []params.Entity{{Tag: good.ActionTag().String()}}}
	res, err := s.uniter.BeginActions(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(res.Results), gc.Equals, 1)
	c.Assert(res.Results[0].Error, gc.IsNil)

	running, err = s.wordpressUnit.RunningActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(running), gc.Equals, 1, gc.Commentf("expected one running action, got %d", len(running)))
	c.Assert(running[0].ActionTag(), gc.Equals, good.ActionTag())
	enqueued, started := running[0].Enqueued(), running[0].Started()
	c.Assert(ten_seconds_ago.Before(enqueued), jc.IsTrue, gc.Commentf("enqueued time should be after 10 seconds ago"))
	c.Assert(ten_seconds_ago.Before(started), jc.IsTrue, gc.Commentf("started time should be after 10 seconds ago"))
	c.Assert(started.After(enqueued) || started.Equal(enqueued), jc.IsTrue, gc.Commentf("started should be after or equal to enqueued time"))
}

func (s *uniterSuite) TestRelation(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	wpEp, err := rel.Endpoint("wordpress")
	c.Assert(err, jc.ErrorIsNil)

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-42", Unit: "unit-foo-0"},
		{Relation: rel.Tag().String(), Unit: "unit-wordpress-0"},
		{Relation: rel.Tag().String(), Unit: "unit-mysql-0"},
		{Relation: rel.Tag().String(), Unit: "unit-foo-0"},
		{Relation: "relation-blah", Unit: "unit-wordpress-0"},
		{Relation: "application-foo", Unit: "user-foo"},
		{Relation: "foo", Unit: "bar"},
		{Relation: "unit-wordpress-0", Unit: rel.Tag().String()},
	}}
	result, err := s.uniter.Relation(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.RelationResults{
		Results: []params.RelationResult{
			{Error: apiservertesting.ErrUnauthorized},
			{
				Id:        rel.Id(),
				Key:       rel.String(),
				Life:      params.Life(rel.Life().String()),
				Suspended: rel.Suspended(),
				Endpoint: multiwatcher.Endpoint{
					ApplicationName: wpEp.ApplicationName,
					Relation:        multiwatcher.NewCharmRelation(wpEp.Relation),
				},
				OtherApplication: s.mysql.Name(),
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestRelationById(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	c.Assert(rel.Id(), gc.Equals, 0)
	wpEp, err := rel.Endpoint("wordpress")
	c.Assert(err, jc.ErrorIsNil)

	// Add another relation to mysql application, so we can see we can't
	// get it.
	otherRel, _, _ := s.addRelatedApplication(c, "mysql", "logging", s.mysqlUnit)

	args := params.RelationIds{
		RelationIds: []int{-1, rel.Id(), otherRel.Id(), 42, 234},
	}
	result, err := s.uniter.RelationById(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.RelationResults{
		Results: []params.RelationResult{
			{Error: apiservertesting.ErrUnauthorized},
			{
				Id:        rel.Id(),
				Key:       rel.String(),
				Life:      params.Life(rel.Life().String()),
				Suspended: rel.Suspended(),
				Endpoint: multiwatcher.Endpoint{
					ApplicationName: wpEp.ApplicationName,
					Relation:        multiwatcher.NewCharmRelation(wpEp.Relation),
				},
				OtherApplication: s.mysql.Name(),
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestProviderType(c *gc.C) {
	cfg, err := s.Model.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.uniter.ProviderType()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringResult{Result: cfg.Type()})
}

func (s *uniterSuite) TestEnterScope(c *gc.C) {
	// Set wordpressUnit's private address first.
	err := s.machine0.SetProviderAddresses(
		network.NewScopedAddress("1.2.3.4", network.ScopeCloudLocal),
	)
	c.Assert(err, jc.ErrorIsNil)

	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, relUnit, false)

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-42", Unit: "unit-foo-0"},
		{Relation: rel.Tag().String(), Unit: "unit-wordpress-0"},
		{Relation: rel.Tag().String(), Unit: "unit-wordpress-0"},
		{Relation: "relation-42", Unit: "unit-wordpress-0"},
		{Relation: "relation-foo", Unit: "unit-wordpress-0"},
		{Relation: "application-wordpress", Unit: "unit-foo-0"},
		{Relation: "foo", Unit: "bar"},
		{Relation: rel.Tag().String(), Unit: "unit-mysql-0"},
		{Relation: rel.Tag().String(), Unit: "application-wordpress"},
		{Relation: rel.Tag().String(), Unit: "application-mysql"},
		{Relation: rel.Tag().String(), Unit: "user-foo"},
	}}
	result, err := s.uniter.EnterScope(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the scope changes and settings.
	s.assertInScope(c, relUnit, true)
	readSettings, err := relUnit.ReadSettings(s.wordpressUnit.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(readSettings, gc.DeepEquals, map[string]interface{}{
		"private-address": "1.2.3.4",
		"ingress-address": "1.2.3.4",
		"egress-subnets":  "1.2.3.4/32",
	})
}

func (s *uniterSuite) TestEnterScopeIgnoredForInvalidPrincipals(c *gc.C) {
	loggingCharm := s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "logging",
		URL:  "cs:quantal/logging-1",
	})
	logging := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "logging",
		Charm: loggingCharm,
	})
	mysqlRel := s.addRelation(c, "logging", "mysql")
	wpRel := s.addRelation(c, "logging", "wordpress")

	// Create logging units for each of the mysql and wp units.
	mysqlRU, err := mysqlRel.Unit(s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlRU.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	mysqlLoggingU := findSubordinateUnit(c, logging, s.mysqlUnit)
	mysqlLoggingRU, err := mysqlRel.Unit(mysqlLoggingU)
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlLoggingRU.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	wpRU, err := wpRel.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = wpRU.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	wpLoggingU := findSubordinateUnit(c, logging, s.wordpressUnit)
	_, err = wpRel.Unit(wpLoggingU)
	c.Assert(err, jc.ErrorIsNil)

	// Sanity check - a mysqlRel RU for wpLoggingU is invalid.
	ru, err := mysqlRel.Unit(wpLoggingU)
	c.Assert(err, jc.ErrorIsNil)
	valid, err := ru.Valid()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(valid, jc.IsFalse)

	subAuthorizer := s.authorizer
	subAuthorizer.Tag = wpLoggingU.Tag()
	uniterAPI := s.newUniterAPI(c, s.State, subAuthorizer)

	// Count how many relationscopes records there are beforehand.
	scopesBefore := countRelationScopes(c, s.State, mysqlRel)
	// One for each unit of mysql and the logging subordinate.
	c.Assert(scopesBefore, gc.Equals, 2)

	// Asking the API to add wpLoggingU to mysqlRel silently
	// fails. This means that we'll drop incorrect requests from
	// uniters to re-enter the relation scope after the upgrade step
	// has cleaned them up.
	// See https://bugs.launchpad.net/juju/+bug/1699050
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{{
		Relation: mysqlRel.Tag().String(),
		Unit:     wpLoggingU.Tag().String(),
	}}}
	result, err := uniterAPI.EnterScope(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})

	scopesAfter := countRelationScopes(c, s.State, mysqlRel)
	c.Assert(scopesAfter, gc.Equals, scopesBefore)
}

func countRelationScopes(c *gc.C, st *state.State, rel *state.Relation) int {
	coll := st.MongoSession().DB("juju").C("relationscopes")
	count, err := coll.Find(bson.M{"key": bson.M{
		"$regex": fmt.Sprintf(`^r#%d#`, rel.Id()),
	}}).Count()
	c.Assert(err, jc.ErrorIsNil)
	return count
}

func (s *uniterSuite) TestLeaveScope(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	settings := map[string]interface{}{
		"some": "settings",
	}
	err = relUnit.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, relUnit, true)

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-42", Unit: "unit-foo-0"},
		{Relation: rel.Tag().String(), Unit: "unit-wordpress-0"},
		{Relation: rel.Tag().String(), Unit: "unit-wordpress-0"},
		{Relation: "relation-42", Unit: "unit-wordpress-0"},
		{Relation: "relation-foo", Unit: "unit-wordpress-0"},
		{Relation: "application-wordpress", Unit: "unit-foo-0"},
		{Relation: "foo", Unit: "bar"},
		{Relation: rel.Tag().String(), Unit: "unit-mysql-0"},
		{Relation: rel.Tag().String(), Unit: "application-wordpress"},
		{Relation: rel.Tag().String(), Unit: "application-mysql"},
		{Relation: rel.Tag().String(), Unit: "user-foo"},
	}}
	result, err := s.uniter.LeaveScope(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the scope changes.
	s.assertInScope(c, relUnit, false)
	readSettings, err := relUnit.ReadSettings(s.wordpressUnit.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(readSettings, gc.DeepEquals, settings)
}

func (s *uniterSuite) TestRelationsSuspended(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	rel2 := s.addRelation(c, "wordpress", "logging")
	err = rel2.SetSuspended(true, "")
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{
		Entities: []params.Entity{
			{s.wordpressUnit.Tag().String()},
			{s.mysqlUnit.Tag().String()},
			{"unit-unknown-1"},
			{"application-wordpress"},
			{"machine-0"},
			{rel.Tag().String()},
		},
	}
	expect := params.RelationUnitStatusResults{
		Results: []params.RelationUnitStatusResult{
			{RelationResults: []params.RelationUnitStatus{{
				RelationTag: rel.Tag().String(),
				InScope:     true,
				Suspended:   false,
			}, {
				RelationTag: rel2.Tag().String(),
				InScope:     false,
				Suspended:   true,
			}},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	}
	check := func() {
		result, err := s.uniter.RelationsStatus(args)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(result, gc.DeepEquals, expect)
	}
	check()
	err = relUnit.PrepareLeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	check()
}

func (s *uniterSuite) TestSetRelationsStatusNotLeader(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	args := params.RelationStatusArgs{
		Args: []params.RelationStatusArg{
			{s.wordpressUnit.Tag().String(), rel.Id(), params.Suspended, "message"},
		},
	}
	result, err := s.uniter.SetRelationStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.ErrorMatches, `"wordpress/0" is not leader of "wordpress"`)
}

func (s *uniterSuite) TestSetRelationsStatusLeader(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	err := rel.SetStatus(status.StatusInfo{Status: status.Suspending, Message: "going, going"})
	c.Assert(err, jc.ErrorIsNil)
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	s.AddTestingApplication(c, "logging", s.AddTestingCharm(c, "logging"))
	rel2 := s.addRelation(c, "wordpress", "logging")
	err = rel2.SetSuspended(true, "")
	c.Assert(err, jc.ErrorIsNil)
	err = rel.SetStatus(status.StatusInfo{Status: status.Suspending, Message: ""})
	c.Assert(err, jc.ErrorIsNil)

	s.AddTestingApplication(c, "wp2", s.wpCharm)
	rel3 := s.addRelation(c, "wp2", "logging")
	c.Assert(err, jc.ErrorIsNil)

	args := params.RelationStatusArgs{
		Args: []params.RelationStatusArg{
			{s.wordpressUnit.Tag().String(), rel.Id(), params.Suspended, "message"},
			// This arg omits the explicit unit tag to test older servers.
			{RelationId: rel2.Id(), Status: params.Suspended, Message: "gone"},
			{s.wordpressUnit.Tag().String(), rel3.Id(), params.Broken, ""},
			{RelationId: 4},
		},
	}
	expect := params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	}
	check := func(rel *state.Relation, expectedStatus status.Status, expectedMessage string) {
		err = rel.Refresh()
		c.Assert(err, jc.ErrorIsNil)
		relStatus, err := rel.Status()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(relStatus.Status, gc.Equals, expectedStatus)
		c.Assert(relStatus.Message, gc.Equals, expectedMessage)
	}

	err = s.State.LeadershipClaimer().ClaimLeadership("wordpress", "wordpress/0", time.Minute)
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.uniter.SetRelationStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, expect)
	check(rel, status.Suspended, "message")
	check(rel2, status.Suspended, "gone")
}

func (s *uniterSuite) TestReadSettings(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	settings := map[string]interface{}{
		"some": "settings",
	}
	err = relUnit.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, relUnit, true)

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-42", Unit: "unit-foo-0"},
		{Relation: rel.Tag().String(), Unit: "unit-wordpress-0"},
		{Relation: rel.Tag().String(), Unit: "unit-mysql-0"},
		{Relation: "relation-42", Unit: "unit-wordpress-0"},
		{Relation: "relation-foo", Unit: ""},
		{Relation: "application-wordpress", Unit: "unit-foo-0"},
		{Relation: "foo", Unit: "bar"},
		{Relation: rel.Tag().String(), Unit: "unit-mysql-0"},
		{Relation: rel.Tag().String(), Unit: "application-wordpress"},
		{Relation: rel.Tag().String(), Unit: "application-mysql"},
		{Relation: rel.Tag().String(), Unit: "user-foo"},
	}}
	result, err := s.uniter.ReadSettings(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Settings: params.Settings{
				"some": "settings",
			}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestReadSettingsWithNonStringValuesFails(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	settings := map[string]interface{}{
		"other":        "things",
		"invalid-bool": false,
	}
	err = relUnit.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, relUnit, true)

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: rel.Tag().String(), Unit: "unit-wordpress-0"},
	}}
	expectErr := `unexpected relation setting "invalid-bool": expected string, got bool`
	result, err := s.uniter.ReadSettings(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Error: &params.Error{Message: expectErr}},
		},
	})
}

func (s *uniterSuite) TestReadRemoteSettings(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	settings := map[string]interface{}{
		"some": "settings",
	}
	err = relUnit.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, relUnit, true)

	// First test most of the invalid args tests and try to read the
	// (unset) remote unit settings.
	args := params.RelationUnitPairs{RelationUnitPairs: []params.RelationUnitPair{
		{Relation: "relation-42", LocalUnit: "unit-foo-0", RemoteUnit: "foo"},
		{Relation: rel.Tag().String(), LocalUnit: "unit-wordpress-0", RemoteUnit: "unit-wordpress-0"},
		{Relation: rel.Tag().String(), LocalUnit: "unit-wordpress-0", RemoteUnit: "unit-mysql-0"},
		{Relation: "relation-42", LocalUnit: "unit-wordpress-0", RemoteUnit: ""},
		{Relation: "relation-foo", LocalUnit: "", RemoteUnit: ""},
		{Relation: "application-wordpress", LocalUnit: "unit-foo-0", RemoteUnit: "user-foo"},
		{Relation: "foo", LocalUnit: "bar", RemoteUnit: "baz"},
		{Relation: rel.Tag().String(), LocalUnit: "unit-mysql-0", RemoteUnit: "unit-wordpress-0"},
		{Relation: rel.Tag().String(), LocalUnit: "application-wordpress", RemoteUnit: "application-mysql"},
		{Relation: rel.Tag().String(), LocalUnit: "application-mysql", RemoteUnit: "foo"},
		{Relation: rel.Tag().String(), LocalUnit: "user-foo", RemoteUnit: "unit-wordpress-0"},
	}}
	result, err := s.uniter.ReadRemoteSettings(args)

	// We don't set the remote unit settings on purpose to test the error.
	expectErr := `cannot read settings for unit "mysql/0" in relation "wordpress:db mysql:server": unit "mysql/0": settings`
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.NotFoundError(expectErr)},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Now leave the mysqlUnit and re-enter with new settings.
	relUnit, err = rel.Unit(s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	settings = map[string]interface{}{
		"other": "things",
	}
	err = relUnit.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, relUnit, false)
	err = relUnit.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, relUnit, true)

	// Test the remote unit settings can be read.
	args = params.RelationUnitPairs{RelationUnitPairs: []params.RelationUnitPair{{
		Relation:   rel.Tag().String(),
		LocalUnit:  "unit-wordpress-0",
		RemoteUnit: "unit-mysql-0",
	}}}
	expect := params.SettingsResults{
		Results: []params.SettingsResult{
			{Settings: params.Settings{
				"other": "things",
			}},
		},
	}
	result, err = s.uniter.ReadRemoteSettings(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, expect)

	// Now destroy the remote unit, and check its settings can still be read.
	err = s.mysqlUnit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysqlUnit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysqlUnit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	result, err = s.uniter.ReadRemoteSettings(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, expect)
}

func (s *uniterSuite) TestReadRemoteSettingsWithNonStringValuesFails(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	settings := map[string]interface{}{
		"other":        "things",
		"invalid-bool": false,
	}
	err = relUnit.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, relUnit, true)

	args := params.RelationUnitPairs{RelationUnitPairs: []params.RelationUnitPair{{
		Relation:   rel.Tag().String(),
		LocalUnit:  "unit-wordpress-0",
		RemoteUnit: "unit-mysql-0",
	}}}
	expectErr := `unexpected relation setting "invalid-bool": expected string, got bool`
	result, err := s.uniter.ReadRemoteSettings(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Error: &params.Error{Message: expectErr}},
		},
	})
}

func (s *uniterSuite) TestUpdateSettings(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	settings := map[string]interface{}{
		"some":  "settings",
		"other": "stuff",
	}
	err = relUnit.EnterScope(settings)
	s.assertInScope(c, relUnit, true)

	newSettings := params.Settings{
		"some":  "different",
		"other": "",
	}

	args := params.RelationUnitsSettings{RelationUnits: []params.RelationUnitSettings{
		{Relation: "relation-42", Unit: "unit-foo-0", Settings: nil},
		{Relation: rel.Tag().String(), Unit: "unit-wordpress-0", Settings: newSettings},
		{Relation: "relation-42", Unit: "unit-wordpress-0", Settings: nil},
		{Relation: "relation-foo", Unit: "unit-wordpress-0", Settings: nil},
		{Relation: "application-wordpress", Unit: "unit-foo-0", Settings: nil},
		{Relation: "foo", Unit: "bar", Settings: nil},
		{Relation: rel.Tag().String(), Unit: "unit-mysql-0", Settings: nil},
		{Relation: rel.Tag().String(), Unit: "application-wordpress", Settings: nil},
		{Relation: rel.Tag().String(), Unit: "application-mysql", Settings: nil},
		{Relation: rel.Tag().String(), Unit: "user-foo", Settings: nil},
	}}
	result, err := s.uniter.UpdateSettings(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the settings were saved.
	s.assertInScope(c, relUnit, true)
	readSettings, err := relUnit.ReadSettings(s.wordpressUnit.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(readSettings, gc.DeepEquals, map[string]interface{}{
		"some": "different",
	})
}

func (s *uniterSuite) TestWatchRelationUnits(c *gc.C) {
	// Add a relation between wordpress and mysql and enter scope with
	// mysqlUnit.
	rel := s.addRelation(c, "wordpress", "mysql")
	myRelUnit, err := rel.Unit(s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = myRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, myRelUnit, true)

	c.Assert(s.resources.Count(), gc.Equals, 0)

	s.WaitForModelWatchersIdle(c, s.Model.UUID())
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-42", Unit: "unit-foo-0"},
		{Relation: rel.Tag().String(), Unit: "unit-wordpress-0"},
		{Relation: rel.Tag().String(), Unit: "unit-mysql-0"},
		{Relation: "relation-42", Unit: "unit-wordpress-0"},
		{Relation: "relation-foo", Unit: ""},
		{Relation: "application-wordpress", Unit: "unit-foo-0"},
		{Relation: "foo", Unit: "bar"},
		{Relation: rel.Tag().String(), Unit: "unit-mysql-0"},
		{Relation: rel.Tag().String(), Unit: "application-wordpress"},
		{Relation: rel.Tag().String(), Unit: "application-mysql"},
		{Relation: rel.Tag().String(), Unit: "user-foo"},
	}}
	result, err := s.uniter.WatchRelationUnits(args)
	c.Assert(err, jc.ErrorIsNil)
	// UnitSettings versions are volatile, so we don't check them.
	// We just make sure the keys of the Changed field are as
	// expected.
	c.Assert(result.Results, gc.HasLen, len(args.RelationUnits))
	mysqlChanges := result.Results[1].Changes
	c.Assert(mysqlChanges, gc.NotNil)
	changed, ok := mysqlChanges.Changed["mysql/0"]
	c.Assert(ok, jc.IsTrue)
	expectChanges := params.RelationUnitsChange{
		Changed: map[string]params.UnitSettings{
			"mysql/0": {changed.Version},
		},
	}
	c.Assert(result, gc.DeepEquals, params.RelationUnitsWatchResults{
		Results: []params.RelationUnitsWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{
				RelationUnitsWatcherId: "1",
				Changes:                expectChanges,
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewRelationUnitsWatcherC(c, s.State, resource.(state.RelationUnitsWatcher))
	wc.AssertNoChange()

	// Leave scope with mysqlUnit and check it's detected.
	err = myRelUnit.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, myRelUnit, false)

	wc.AssertChange(nil, []string{"mysql/0"})
}

func (s *uniterSuite) TestAPIAddresses(c *gc.C) {
	hostPorts := [][]network.HostPort{
		network.NewHostPorts(1234, "0.1.2.3"),
	}
	err := s.State.SetAPIHostPorts(hostPorts)
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.uniter.APIAddresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsResult{
		Result: []string{"0.1.2.3:1234"},
	})
}

func (s *uniterSuite) TestWatchUnitAddressesHash(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "machine-0"},
		{Tag: "application-wordpress"},
	}}
	result, err := s.uniter.WatchUnitAddressesHash(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{
				StringsWatcherId: "1",
				// This is an empty sha256 hash.
				Changes: []string{"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *uniterSuite) TestWatchCAASUnitAddressesHash(c *gc.C) {
	_, cm, _, _ := s.setupCAASModel(c)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-gitlab-0"},
		{Tag: "unit-foo-42"},
		{Tag: "machine-0"},
		{Tag: "application-gitlab"},
	}}

	uniterAPI := s.newUniterAPI(c, cm.State(), s.authorizer)

	result, err := uniterAPI.WatchUnitAddressesHash(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{
				StringsWatcherId: "1",
				// The container doesn't have an address.
				Changes: []string{""},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *uniterSuite) TestGetMeterStatusUnauthenticated(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{{s.mysqlUnit.Tag().String()}}}
	result, err := s.uniter.GetMeterStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, "permission denied")
	c.Assert(result.Results[0].Code, gc.Equals, "")
	c.Assert(result.Results[0].Info, gc.Equals, "")
}

func (s *uniterSuite) TestGetMeterStatusBadTag(c *gc.C) {
	tags := []string{
		"user-admin",
		"unit-nosuchunit",
		"thisisnotatag",
		"machine-0",
		"model-blah",
	}
	args := params.Entities{Entities: make([]params.Entity, len(tags))}
	for i, tag := range tags {
		args.Entities[i] = params.Entity{Tag: tag}
	}
	result, err := s.uniter.GetMeterStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, len(tags))
	for i, result := range result.Results {
		c.Logf("checking result %d", i)
		c.Assert(result.Code, gc.Equals, "")
		c.Assert(result.Info, gc.Equals, "")
		c.Assert(result.Error, gc.ErrorMatches, "permission denied")
	}
}

func (s *uniterSuite) addRelatedApplication(c *gc.C, firstSvc, relatedSvc string, unit *state.Unit) (*state.Relation, *state.Application, *state.Unit) {
	relatedApplication := s.AddTestingApplication(c, relatedSvc, s.AddTestingCharm(c, relatedSvc))
	rel := s.addRelation(c, firstSvc, relatedSvc)
	relUnit, err := rel.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	relatedUnit, err := s.State.Unit(relatedSvc + "/0")
	c.Assert(err, jc.ErrorIsNil)
	return rel, relatedApplication, relatedUnit
}

func (s *uniterSuite) TestRequestReboot(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: s.machine0.Tag().String()},
		{Tag: s.machine1.Tag().String()},
		{Tag: "bogus"},
		{Tag: "nasty-tag"},
	}}
	errResult, err := s.uniter.RequestReboot(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(errResult, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		}})

	rFlag, err := s.machine0.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rFlag, jc.IsTrue)

	rFlag, err = s.machine1.GetRebootFlag()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rFlag, jc.IsFalse)
}

func checkUnorderedActionIdsEqual(c *gc.C, ids []string, results params.StringsWatchResults) {
	c.Assert(results, gc.NotNil)
	content := results.Results
	c.Assert(len(content), gc.Equals, 1)
	result := content[0]
	c.Assert(result.StringsWatcherId, gc.Equals, "1")
	obtainedIds := map[string]int{}
	expectedIds := map[string]int{}
	for _, id := range ids {
		expectedIds[id]++
	}
	// The count of each ID that has been seen.
	for _, change := range result.Changes {
		obtainedIds[change]++
	}
	c.Check(obtainedIds, jc.DeepEquals, expectedIds)
}

func (s *uniterSuite) TestStorageAttachments(c *gc.C) {
	// We need to set up a unit that has storage metadata defined.
	ch := s.AddTestingCharm(c, "storage-block")
	sCons := map[string]state.StorageConstraints{
		"data": {Pool: "", Size: 1024, Count: 1},
	}
	application := s.AddTestingApplicationWithStorage(c, "storage-block", ch, sCons)
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	assignedMachineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := s.State.Machine(assignedMachineId)
	c.Assert(err, jc.ErrorIsNil)

	volumeAttachments, err := machine.VolumeAttachments()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeAttachments, gc.HasLen, 1)

	err = machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	err = sb.SetVolumeInfo(
		volumeAttachments[0].Volume(),
		state.VolumeInfo{VolumeId: "vol-123", Size: 456},
	)
	c.Assert(err, jc.ErrorIsNil)

	err = sb.SetVolumeAttachmentInfo(
		machine.MachineTag(),
		volumeAttachments[0].Volume(),
		state.VolumeAttachmentInfo{DeviceName: "xvdf1"},
	)
	c.Assert(err, jc.ErrorIsNil)

	password, err := utils.RandomPassword()
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	st := s.OpenAPIAs(c, unit.Tag(), password)
	uniter, err := st.Uniter()
	c.Assert(err, jc.ErrorIsNil)

	attachments, err := uniter.UnitStorageAttachments(unit.UnitTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(attachments, gc.DeepEquals, []params.StorageAttachmentId{{
		StorageTag: "storage-data-0",
		UnitTag:    unit.Tag().String(),
	}})
}

func (s *uniterSuite) TestUnitStatus(c *gc.C) {
	now := time.Now()
	sInfo := status.StatusInfo{
		Status:  status.Maintenance,
		Message: "blah",
		Since:   &now,
	}
	err := s.wordpressUnit.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Terminated,
		Message: "foo",
		Since:   &now,
	}
	err = s.mysqlUnit.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: "unit-mysql-0"},
			{Tag: "unit-wordpress-0"},
			{Tag: "unit-foo-42"},
			{Tag: "machine-1"},
			{Tag: "invalid"},
		}}
	result, err := s.uniter.UnitStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	// Zero out the updated timestamps so we can easily check the results.
	for i, statusResult := range result.Results {
		r := statusResult
		if r.Status != "" {
			c.Assert(r.Since, gc.NotNil)
		}
		r.Since = nil
		result.Results[i] = r
	}
	c.Assert(result, gc.DeepEquals, params.StatusResults{
		Results: []params.StatusResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Status: status.Maintenance.String(), Info: "blah", Data: map[string]interface{}{}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ServerError(`"invalid" is not a valid tag`)},
		},
	})
}

func (s *uniterSuite) TestAssignedMachine(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "application-mysql"},
		{Tag: "application-wordpress"},
		{Tag: "machine-0"},
		{Tag: "machine-1"},
		{Tag: "machine-42"},
		{Tag: "application-foo"},
		{Tag: "relation-svc1.rel1#svc2.rel2"},
	}}
	result, err := s.uniter.AssignedMachine(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: "machine-0"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestAllMachinePorts(c *gc.C) {
	// Verify no ports are opened yet on the machine or unit.
	machinePorts, err := s.machine0.AllPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machinePorts, gc.HasLen, 0)
	unitPorts, err := s.wordpressUnit.OpenedPorts()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitPorts, gc.HasLen, 0)

	// Add another mysql unit on machine 0.
	mysqlUnit1, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlUnit1.AssignToMachine(s.machine0)
	c.Assert(err, jc.ErrorIsNil)

	// Open some ports on both units.
	err = s.wordpressUnit.OpenPorts("tcp", 100, 200)
	c.Assert(err, jc.ErrorIsNil)
	err = s.wordpressUnit.OpenPorts("udp", 10, 20)
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlUnit1.OpenPorts("tcp", 201, 250)
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlUnit1.OpenPorts("udp", 1, 8)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "machine-0"},
		{Tag: "machine-1"},
		{Tag: "unit-foo-42"},
		{Tag: "machine-42"},
		{Tag: "application-wordpress"},
	}}
	expectPorts := []params.MachinePortRange{
		{UnitTag: "unit-wordpress-0", PortRange: params.PortRange{100, 200, "tcp"}},
		{UnitTag: "unit-mysql-1", PortRange: params.PortRange{201, 250, "tcp"}},
		{UnitTag: "unit-mysql-1", PortRange: params.PortRange{1, 8, "udp"}},
		{UnitTag: "unit-wordpress-0", PortRange: params.PortRange{10, 20, "udp"}},
	}
	result, err := s.uniter.AllMachinePorts(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.MachinePortsResults{
		Results: []params.MachinePortsResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Ports: expectPorts},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestSLALevel(c *gc.C) {
	err := s.State.SetSLA("essential", "bob", []byte("creds"))
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.uniter.SLALevel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringResult{Result: "essential"})
}

func (s *uniterSuite) setupRemoteRelationScenario(c *gc.C) (names.Tag, *state.RelationUnit) {
	s.makeRemoteWordpress(c)

	// Set mysql's addresses first.
	err := s.machine1.SetProviderAddresses(
		network.NewScopedAddress("1.2.3.4", network.ScopeCloudLocal),
		network.NewScopedAddress("4.3.2.1", network.ScopePublic),
	)
	c.Assert(err, jc.ErrorIsNil)

	eps, err := s.State.InferEndpoints("mysql", "remote-wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)

	relUnit, err := rel.Unit(s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, relUnit, false)
	return rel.Tag(), relUnit
}

func (s *uniterSuite) TestPrivateAddressWithRemoteRelation(c *gc.C) {
	relTag, relUnit := s.setupRemoteRelationScenario(c)

	thisUniter := s.makeMysqlUniter(c)
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: "unit-mysql-0"},
	}}
	result, err := thisUniter.EnterScope(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})

	// Verify the scope changes and settings.
	s.assertInScope(c, relUnit, true)
	readSettings, err := relUnit.ReadSettings(s.mysqlUnit.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(readSettings, gc.DeepEquals, map[string]interface{}{
		"private-address": "4.3.2.1",
		"ingress-address": "4.3.2.1",
		"egress-subnets":  "4.3.2.1/32",
	})
}

func (s *uniterSuite) TestPrivateAddressWithRemoteRelationNoPublic(c *gc.C) {
	relTag, relUnit := s.setupRemoteRelationScenario(c)

	thisUniter := s.makeMysqlUniter(c)
	// Set mysql's addresses - no public address.
	err := s.machine1.SetProviderAddresses(
		network.NewScopedAddress("1.2.3.4", network.ScopeCloudLocal),
	)
	c.Assert(err, jc.ErrorIsNil)

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: "unit-mysql-0"},
	}}
	result, err := thisUniter.EnterScope(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})

	// Verify that we fell back to the private address.
	s.assertInScope(c, relUnit, true)
	readSettings, err := relUnit.ReadSettings(s.mysqlUnit.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(readSettings, gc.DeepEquals, map[string]interface{}{
		"private-address": "1.2.3.4",
		"ingress-address": "1.2.3.4",
		"egress-subnets":  "1.2.3.4/32",
	})
}

func (s *uniterSuite) TestRelationEgressSubnets(c *gc.C) {
	relTag, relUnit := s.setupRemoteRelationScenario(c)

	// Check model attributes are overridden by setting up a value.
	err := s.Model.UpdateModelConfig(map[string]interface{}{"egress-subnets": "192.168.0.0/16"}, nil)
	c.Assert(err, jc.ErrorIsNil)
	egress := state.NewRelationEgressNetworks(s.State)
	_, err = egress.Save(relTag.Id(), false, []string{"10.0.0.0/16", "10.1.2.0/8"})
	c.Assert(err, jc.ErrorIsNil)

	thisUniter := s.makeMysqlUniter(c)
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: "unit-mysql-0"},
	}}
	result, err := thisUniter.EnterScope(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})

	// Verify the scope changes and settings.
	s.assertInScope(c, relUnit, true)
	readSettings, err := relUnit.ReadSettings(s.mysqlUnit.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(readSettings, gc.DeepEquals, map[string]interface{}{
		"private-address": "4.3.2.1",
		"ingress-address": "4.3.2.1",
		"egress-subnets":  "10.0.0.0/16,10.1.2.0/8",
	})
}

func (s *uniterSuite) TestModelEgressSubnets(c *gc.C) {
	relTag, relUnit := s.setupRemoteRelationScenario(c)

	err := s.Model.UpdateModelConfig(map[string]interface{}{"egress-subnets": "192.168.0.0/16"}, nil)
	c.Assert(err, jc.ErrorIsNil)

	thisUniter := s.makeMysqlUniter(c)
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: "unit-mysql-0"},
	}}
	result, err := thisUniter.EnterScope(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})

	// Verify the scope changes and settings.
	s.assertInScope(c, relUnit, true)
	readSettings, err := relUnit.ReadSettings(s.mysqlUnit.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(readSettings, gc.DeepEquals, map[string]interface{}{
		"private-address": "4.3.2.1",
		"ingress-address": "4.3.2.1",
		"egress-subnets":  "192.168.0.0/16",
	})
}

func (s *uniterSuite) makeMysqlUniter(c *gc.C) *uniter.UniterAPI {
	authorizer := s.authorizer
	authorizer.Tag = s.mysqlUnit.Tag()
	return s.newUniterAPI(c, s.State, authorizer)
}

func (s *uniterSuite) makeRemoteWordpress(c *gc.C) {
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:            "remote-wordpress",
		SourceModel:     names.NewModelTag("source-model"),
		IsConsumerProxy: true,
		OfferUUID:       "offer-uuid",
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Limit:     1,
			Name:      "db",
			Role:      charm.RoleRequirer,
			Scope:     charm.ScopeGlobal,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *uniterSuite) TestV4WatchApplicationRelations(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "application-mysql"},
		{Tag: "application-wordpress"},
		{Tag: "application-foo"},
	}}
	apiV4, err := uniter.NewUniterAPIV4(facadetest.Context{
		State_:             s.State,
		Resources_:         s.resources,
		Auth_:              s.authorizer,
		LeadershipChecker_: s.State.LeadershipChecker(),
		Controller_:        s.Controller,
	})
	c.Assert(err, jc.ErrorIsNil)
	result, err := apiV4.WatchApplicationRelations(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 3)
	c.Assert(result.Results[0].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)
	c.Assert(result.Results[1].StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Results[1].Changes, gc.NotNil)
	c.Assert(result.Results[1].Error, gc.IsNil)
	c.Assert(result.Results[2].Error, gc.DeepEquals, apiservertesting.ErrUnauthorized)

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, s.State, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *uniterSuite) TestV5Relation(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	wpEp, err := rel.Endpoint("wordpress")
	c.Assert(err, jc.ErrorIsNil)

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: rel.Tag().String(), Unit: "unit-wordpress-0"},
	}}
	apiV5, err := uniter.NewUniterAPIV5(facadetest.Context{
		State_:             s.State,
		Resources_:         s.resources,
		Auth_:              s.authorizer,
		LeadershipChecker_: s.State.LeadershipChecker(),
		Controller_:        s.Controller,
	})
	c.Assert(err, jc.ErrorIsNil)
	result, err := apiV5.Relation(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.RelationResultsV5{
		Results: []params.RelationResultV5{
			{
				Id:   rel.Id(),
				Key:  rel.String(),
				Life: params.Life(rel.Life().String()),
				Endpoint: multiwatcher.Endpoint{
					ApplicationName: wpEp.ApplicationName,
					Relation:        multiwatcher.NewCharmRelation(wpEp.Relation),
				},
			},
		},
	})
}

func (s *uniterSuite) TestV5RelationById(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	wpEp, err := rel.Endpoint("wordpress")
	c.Assert(err, jc.ErrorIsNil)

	args := params.RelationIds{RelationIds: []int{rel.Id()}}
	apiV5, err := uniter.NewUniterAPIV5(facadetest.Context{
		State_:             s.State,
		Resources_:         s.resources,
		Auth_:              s.authorizer,
		LeadershipChecker_: s.State.LeadershipChecker(),
		Controller_:        s.Controller,
	})
	c.Assert(err, jc.ErrorIsNil)
	result, err := apiV5.RelationById(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.RelationResultsV5{
		Results: []params.RelationResultV5{
			{
				Id:   rel.Id(),
				Key:  rel.String(),
				Life: params.Life(rel.Life().String()),
				Endpoint: multiwatcher.Endpoint{
					ApplicationName: wpEp.ApplicationName,
					Relation:        multiwatcher.NewCharmRelation(wpEp.Relation),
				},
			},
		},
	})
}

func (s *uniterSuite) TestRefresh(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{
			{s.wordpressUnit.Tag().String()},
			{s.mysqlUnit.Tag().String()},
			{s.mysql.Tag().String()},
			{s.machine0.Tag().String()},
			{"some-word"},
		},
	}
	expect := params.UnitRefreshResults{
		Results: []params.UnitRefreshResult{
			{Life: params.Alive, Resolved: params.ResolvedNone},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	}
	results, err := s.uniter.Refresh(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, expect)
}

func (s *uniterSuite) TestRefreshNoArgs(c *gc.C) {
	results, err := s.uniter.Refresh(params.Entities{Entities: []params.Entity{}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.UnitRefreshResults{Results: []params.UnitRefreshResult{}})
}

var podSpec = `
containers:
  - name: gitlab
    image: gitlab/latest
    ports:
    - containerPort: 80
      protocol: TCP
    - containerPort: 443
    config:
      attr: foo=bar; fred=blogs
      foo: bar
`[1:]

func (s *uniterSuite) TestSetPodSpec(c *gc.C) {
	u, cm, app, _ := s.setupCAASModel(c)

	err := u.SetPodSpec(app.Name(), podSpec)
	c.Assert(err, jc.ErrorIsNil)
	spec, err := cm.PodSpec(app.ApplicationTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spec, gc.Equals, podSpec)
}

type unitMetricBatchesSuite struct {
	uniterSuiteBase
	*commontesting.ModelWatcherTest
	uniter *uniter.UniterAPI

	meteredApplication *state.Application
	meteredCharm       *state.Charm
	meteredUnit        *state.Unit
}

var _ = gc.Suite(&unitMetricBatchesSuite{})

func (s *unitMetricBatchesSuite) SetUpTest(c *gc.C) {
	s.uniterSuiteBase.SetUpTest(c)

	s.meteredCharm = s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "metered",
		URL:  "cs:quantal/metered",
	})
	s.meteredApplication = s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.meteredCharm,
	})
	s.meteredUnit = s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.meteredApplication,
		SetCharmURL: true,
	})

	meteredAuthorizer := apiservertesting.FakeAuthorizer{
		Tag: s.meteredUnit.Tag(),
	}
	s.uniter = s.newUniterAPI(c, s.State, meteredAuthorizer)

	s.ModelWatcherTest = commontesting.NewModelWatcherTest(
		s.uniter,
		s.State,
		s.resources,
	)
}

func (s *unitMetricBatchesSuite) TestAddMetricsBatch(c *gc.C) {
	metrics := []params.Metric{{Key: "pings", Value: "5", Time: time.Now().UTC()}}
	uuid := utils.MustNewUUID().String()

	result, err := s.uniter.AddMetricBatches(params.MetricBatchParams{
		Batches: []params.MetricBatchParam{{
			Tag: s.meteredUnit.Tag().String(),
			Batch: params.MetricBatch{
				UUID:     uuid,
				CharmURL: s.meteredCharm.URL().String(),
				Created:  time.Now(),
				Metrics:  metrics,
			}}}},
	)

	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{nil}},
	})
	c.Assert(err, jc.ErrorIsNil)

	batch, err := s.State.MetricBatch(uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batch.UUID(), gc.Equals, uuid)
	c.Assert(batch.CharmURL(), gc.Equals, s.meteredCharm.URL().String())
	c.Assert(batch.Unit(), gc.Equals, s.meteredUnit.Name())
	storedMetrics := batch.Metrics()
	c.Assert(storedMetrics, gc.HasLen, 1)
	c.Assert(storedMetrics[0].Key, gc.Equals, metrics[0].Key)
	c.Assert(storedMetrics[0].Value, gc.Equals, metrics[0].Value)
}

func (s *unitMetricBatchesSuite) TestAddMetricsBatchNoCharmURL(c *gc.C) {
	metrics := []params.Metric{{Key: "pings", Value: "5", Time: time.Now().UTC()}}
	uuid := utils.MustNewUUID().String()

	result, err := s.uniter.AddMetricBatches(params.MetricBatchParams{
		Batches: []params.MetricBatchParam{{
			Tag: s.meteredUnit.Tag().String(),
			Batch: params.MetricBatch{
				UUID:     uuid,
				CharmURL: s.meteredCharm.URL().String(),
				Created:  time.Now(),
				Metrics:  metrics,
			}}}})

	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{nil}},
	})
	c.Assert(err, jc.ErrorIsNil)

	batch, err := s.State.MetricBatch(uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(batch.UUID(), gc.Equals, uuid)
	c.Assert(batch.CharmURL(), gc.Equals, s.meteredCharm.URL().String())
	c.Assert(batch.Unit(), gc.Equals, s.meteredUnit.Name())
	storedMetrics := batch.Metrics()
	c.Assert(storedMetrics, gc.HasLen, 1)
	c.Assert(storedMetrics[0].Key, gc.Equals, metrics[0].Key)
	c.Assert(storedMetrics[0].Value, gc.Equals, metrics[0].Value)
}

func (s *unitMetricBatchesSuite) TestAddMetricsBatchDiffTag(c *gc.C) {
	unit2 := s.Factory.MakeUnit(c, &factory.UnitParams{Application: s.meteredApplication, SetCharmURL: true})

	metrics := []params.Metric{{Key: "pings", Value: "5", Time: time.Now().UTC()}}
	uuid := utils.MustNewUUID().String()

	tests := []struct {
		about  string
		tag    string
		expect string
	}{{
		about:  "different unit",
		tag:    unit2.Tag().String(),
		expect: "permission denied",
	}, {
		about:  "user tag",
		tag:    names.NewLocalUserTag("admin").String(),
		expect: `"user-admin" is not a valid unit tag`,
	}, {
		about:  "machine tag",
		tag:    names.NewMachineTag("0").String(),
		expect: `"machine-0" is not a valid unit tag`,
	}}

	for i, test := range tests {
		c.Logf("test %d: %s", i, test.about)
		result, err := s.uniter.AddMetricBatches(params.MetricBatchParams{
			Batches: []params.MetricBatchParam{{
				Tag: test.tag,
				Batch: params.MetricBatch{
					UUID:     uuid,
					CharmURL: "",
					Created:  time.Now(),
					Metrics:  metrics,
				}}}})

		if test.expect == "" {
			c.Assert(result.OneError(), jc.ErrorIsNil)
		} else {
			c.Assert(result.OneError(), gc.ErrorMatches, test.expect)
		}
		c.Assert(err, jc.ErrorIsNil)

		_, err = s.State.MetricBatch(uuid)
		c.Assert(err, jc.Satisfies, errors.IsNotFound)
	}
}

type uniterNetworkConfigSuite struct {
	uniterSuiteBase
	uniterv4 *uniter.UniterAPIV4
}

var _ = gc.Suite(&uniterNetworkConfigSuite{})

func (s *uniterNetworkConfigSuite) SetUpTest(c *gc.C) {
	s.uniterSuiteBase.JujuConnSuite.SetUpTest(c)

	// Add the spaces and subnets used by the test.
	subnetInfos := []corenetwork.SubnetInfo{{
		CIDR:      "8.8.0.0/16",
		SpaceName: "public",
	}, {
		CIDR:      "10.0.0.0/24",
		SpaceName: "internal",
	}}
	for _, info := range subnetInfos {
		_, err := s.State.AddSpace(info.SpaceName, "", nil, false)
		c.Assert(err, jc.ErrorIsNil)
		_, err = s.State.AddSubnet(info)
		c.Assert(err, jc.ErrorIsNil)
	}

	s.machine0 = s.addProvisionedMachineWithDevicesAndAddresses(c, 10)

	s.wpCharm = s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "wordpress-extra-bindings",
		URL:  "cs:quantal/wordpress-extra-bindings-4",
	})
	var err error
	s.wordpress, err = s.State.AddApplication(state.AddApplicationArgs{
		Name:  "wordpress",
		Charm: s.wpCharm,
		EndpointBindings: map[string]string{
			"db":        "internal", // relation name
			"admin-api": "public",   // extra-binding name
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.wordpressUnit = s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.wordpress,
		Machine:     s.machine0,
	})

	s.machine1 = s.addProvisionedMachineWithDevicesAndAddresses(c, 20)

	mysqlCharm := s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "mysql",
	})
	s.mysql = s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "mysql",
		Charm: mysqlCharm,
	})
	s.wordpressUnit = s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.wordpress,
		Machine:     s.machine0,
	})
	s.mysqlUnit = s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.mysql,
		Machine:     s.machine1,
	})

	s.State.StartSync()
	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())

	// Create the resource registry separately to track invocations to register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.setupUniterAPIForUnit(c, s.wordpressUnit)
}

func (s *uniterNetworkConfigSuite) addProvisionedMachineWithDevicesAndAddresses(c *gc.C, addrSuffix int) *state.Machine {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	devicesArgs, devicesAddrs := s.makeMachineDevicesAndAddressesArgs(addrSuffix)
	err = machine.SetInstanceInfo("i-am", "", "fake_nonce", nil, devicesArgs, devicesAddrs, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	machineAddrs, err := machine.AllAddresses()
	c.Assert(err, jc.ErrorIsNil)

	netAddrs := make([]network.Address, len(machineAddrs))
	for i, addr := range machineAddrs {
		netAddrs[i] = network.NewAddress(addr.Value())
	}
	err = machine.SetProviderAddresses(netAddrs...)
	c.Assert(err, jc.ErrorIsNil)

	return machine
}

func (s *uniterNetworkConfigSuite) makeMachineDevicesAndAddressesArgs(addrSuffix int) ([]state.LinkLayerDeviceArgs, []state.LinkLayerDeviceAddress) {
	return []state.LinkLayerDeviceArgs{{
			Name: "eth0",
			Type: state.EthernetDevice,
		}, {
			Name:       "eth0.100",
			Type:       state.VLAN_8021QDevice,
			ParentName: "eth0",
		}, {
			Name: "eth1",
			Type: state.EthernetDevice,
		}, {
			Name:       "eth1.100",
			Type:       state.VLAN_8021QDevice,
			ParentName: "eth1",
		}},
		[]state.LinkLayerDeviceAddress{{
			DeviceName:   "eth0",
			ConfigMethod: state.StaticAddress,
			CIDRAddress:  fmt.Sprintf("8.8.8.%d/16", addrSuffix),
		}, {
			DeviceName:   "eth0.100",
			ConfigMethod: state.StaticAddress,
			CIDRAddress:  fmt.Sprintf("10.0.0.%d/24", addrSuffix),
		}, {
			DeviceName:   "eth1",
			ConfigMethod: state.StaticAddress,
			CIDRAddress:  fmt.Sprintf("8.8.4.%d/16", addrSuffix),
		}, {
			DeviceName:   "eth1.100",
			ConfigMethod: state.StaticAddress,
			CIDRAddress:  fmt.Sprintf("10.0.0.%d/24", addrSuffix+1),
		}}
}

func (s *uniterNetworkConfigSuite) setupUniterAPIForUnit(c *gc.C, givenUnit *state.Unit) {
	// Create a FakeAuthorizer so we can check permissions, set up assuming the
	// given unit agent has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: givenUnit.Tag(),
	}

	var err error
	s.uniterv4, err = uniter.NewUniterAPIV4(facadetest.Context{
		State_:             s.State,
		Resources_:         s.resources,
		Auth_:              s.authorizer,
		LeadershipChecker_: s.State.LeadershipChecker(),
		Controller_:        s.Controller,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *uniterNetworkConfigSuite) TestNetworkConfigPermissions(c *gc.C) {
	s.addRelationAndAssertInScope(c)

	args := params.UnitsNetworkConfig{Args: []params.UnitNetworkConfig{
		{BindingName: "foo", UnitTag: "unit-foo-0"},
		{BindingName: "db-client", UnitTag: "invalid"},
		{BindingName: "juju-info", UnitTag: "unit-mysql-0"},
		{BindingName: "", UnitTag: s.wordpressUnit.Tag().String()},
		{BindingName: "unknown", UnitTag: s.wordpressUnit.Tag().String()},
	}}

	result, err := s.uniterv4.NetworkConfig(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.UnitNetworkConfigResults{
		Results: []params.UnitNetworkConfigResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ServerError(`"invalid" is not a valid tag`)},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ServerError(`binding name cannot be empty`)},
			{Error: apiservertesting.ServerError(`binding name "unknown" not defined by the unit's charm`)},
		},
	})
}

func (s *uniterNetworkConfigSuite) addRelationAndAssertInScope(c *gc.C) {
	// Add a relation between wordpress and mysql and enter scope with
	// mysqlUnit.
	rel := s.addRelation(c, "wordpress", "mysql")
	wpRelUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = wpRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, wpRelUnit, true)
}

func (s *uniterNetworkConfigSuite) TestNetworkConfigForExplicitlyBoundEndpoint(c *gc.C) {
	s.addRelationAndAssertInScope(c)

	args := params.UnitsNetworkConfig{Args: []params.UnitNetworkConfig{
		{BindingName: "db", UnitTag: s.wordpressUnit.Tag().String()},
		{BindingName: "admin-api", UnitTag: s.wordpressUnit.Tag().String()},
	}}

	// For the relation "wordpress:db mysql:server" we expect to see only
	// addresses bound to the "internal" space, where the "db" endpoint itself
	// is bound to.
	expectedConfigWithRelationName := []params.NetworkConfig{
		{Address: "10.0.0.10"},
		{Address: "10.0.0.11"},
	}
	// For the "admin-api" extra-binding we expect to see only addresses from
	// the "public" space.
	expectedConfigWithExtraBindingName := []params.NetworkConfig{
		{Address: "8.8.8.10"},
		{Address: "8.8.4.10"},
	}

	result, err := s.uniterv4.NetworkConfig(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.UnitNetworkConfigResults{
		Results: []params.UnitNetworkConfigResult{
			{Config: expectedConfigWithRelationName},
			{Config: expectedConfigWithExtraBindingName},
		},
	})
}

func (s *uniterNetworkConfigSuite) TestNetworkConfigForImplicitlyBoundEndpoint(c *gc.C) {
	// Since wordpressUnit has explicit binding for "db", switch the API to
	// mysqlUnit and check "mysql:server" uses the machine preferred private
	// address.
	s.setupUniterAPIForUnit(c, s.mysqlUnit)
	rel := s.addRelation(c, "mysql", "wordpress")
	mysqlRelUnit, err := rel.Unit(s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, mysqlRelUnit, true)

	args := params.UnitsNetworkConfig{Args: []params.UnitNetworkConfig{
		{BindingName: "server", UnitTag: s.mysqlUnit.Tag().String()},
	}}

	privateAddress, err := s.machine1.PrivateAddress()
	c.Assert(err, jc.ErrorIsNil)

	expectedConfig := []params.NetworkConfig{{Address: privateAddress.Value}}

	result, err := s.uniterv4.NetworkConfig(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.UnitNetworkConfigResults{
		Results: []params.UnitNetworkConfigResult{
			{Config: expectedConfig},
		},
	})
}

type uniterNetworkInfoSuite struct {
	uniterSuiteBase
	mysqlCharm *state.Charm
}

var _ = gc.Suite(&uniterNetworkInfoSuite{})

func (s *uniterNetworkInfoSuite) SetUpTest(c *gc.C) {
	s.uniterSuiteBase.JujuConnSuite.SetUpTest(c)

	// Add the spaces and subnets used by the test.
	subnetInfos := []corenetwork.SubnetInfo{{
		CIDR:      "8.8.0.0/16",
		SpaceName: "public",
	}, {
		CIDR:      "10.0.0.0/24",
		SpaceName: "internal",
	}, {
		CIDR:      "100.64.0.0/16",
		SpaceName: "wp-default",
	}, {
		CIDR:      "192.168.1.0/24",
		SpaceName: "database",
	}, {
		SpaceName: "layertwo",
	}}
	for _, info := range subnetInfos {
		_, err := s.State.AddSpace(info.SpaceName, "", nil, false)
		c.Assert(err, jc.ErrorIsNil)
		if info.CIDR != "" {
			_, err = s.State.AddSubnet(info)
			c.Assert(err, jc.ErrorIsNil)
		}
	}

	s.machine0 = s.addProvisionedMachineWithDevicesAndAddresses(c, 10)

	s.wpCharm = s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "wordpress-extra-bindings",
		URL:  "cs:quantal/wordpress-extra-bindings-4",
	})
	var err error
	s.wordpress, err = s.State.AddApplication(state.AddApplicationArgs{
		Name:  "wordpress",
		Charm: s.wpCharm,
		EndpointBindings: map[string]string{
			"db":        "internal",   // relation name
			"admin-api": "public",     // extra-binding name
			"foo-bar":   "layertwo",   // extra-binding to L2
			"":          "wp-default", // explicitly specified default space
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.wordpressUnit = s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.wordpress,
		Machine:     s.machine0,
	})

	s.machine1 = s.addProvisionedMachineWithDevicesAndAddresses(c, 20)

	s.mysqlCharm = s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: "mysql",
	})
	s.mysql = s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "mysql",
		Charm: s.mysqlCharm,
		EndpointBindings: map[string]string{
			"server": "database",
		},
	})
	s.wordpressUnit = s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.wordpress,
		Machine:     s.machine0,
	})
	s.mysqlUnit = s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.mysql,
		Machine:     s.machine1,
	})

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.setupUniterAPIForUnit(c, s.wordpressUnit)
}

func (s *uniterNetworkInfoSuite) addProvisionedMachineWithDevicesAndAddresses(c *gc.C, addrSuffix int) *state.Machine {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	devicesArgs, devicesAddrs := s.makeMachineDevicesAndAddressesArgs(addrSuffix)
	err = machine.SetInstanceInfo("i-am", "", "fake_nonce", nil, devicesArgs, devicesAddrs, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	machineAddrs, err := machine.AllAddresses()
	c.Assert(err, jc.ErrorIsNil)

	netAddrs := make([]network.Address, len(machineAddrs))
	for i, addr := range machineAddrs {
		netAddrs[i] = network.NewAddress(addr.Value())
	}
	err = machine.SetProviderAddresses(netAddrs...)
	c.Assert(err, jc.ErrorIsNil)

	return machine
}

func (s *uniterNetworkInfoSuite) makeMachineDevicesAndAddressesArgs(addrSuffix int) ([]state.LinkLayerDeviceArgs, []state.LinkLayerDeviceAddress) {
	return []state.LinkLayerDeviceArgs{{
			Name:       "eth0",
			Type:       state.EthernetDevice,
			MACAddress: fmt.Sprintf("00:11:22:33:%0.2d:50", addrSuffix),
		}, {
			Name:       "eth0.100",
			Type:       state.VLAN_8021QDevice,
			ParentName: "eth0",
			MACAddress: fmt.Sprintf("00:11:22:33:%0.2d:50", addrSuffix),
		}, {
			Name:       "eth1",
			Type:       state.EthernetDevice,
			MACAddress: fmt.Sprintf("00:11:22:33:%0.2d:51", addrSuffix),
		}, {
			Name:       "eth1.100",
			Type:       state.VLAN_8021QDevice,
			ParentName: "eth1",
			MACAddress: fmt.Sprintf("00:11:22:33:%0.2d:51", addrSuffix),
		}, {
			Name:       "eth2",
			Type:       state.EthernetDevice,
			MACAddress: fmt.Sprintf("00:11:22:33:%0.2d:52", addrSuffix),
		}, {
			Name:       "eth3",
			Type:       state.EthernetDevice,
			MACAddress: fmt.Sprintf("00:11:22:33:%0.2d:53", addrSuffix),
		}, {
			Name:       "eth4",
			Type:       state.EthernetDevice,
			MACAddress: fmt.Sprintf("00:11:22:33:%0.2d:54", addrSuffix),
		}},
		[]state.LinkLayerDeviceAddress{{
			DeviceName:   "eth0",
			ConfigMethod: state.StaticAddress,
			CIDRAddress:  fmt.Sprintf("8.8.8.%d/16", addrSuffix),
		}, {
			DeviceName:   "eth0.100",
			ConfigMethod: state.StaticAddress,
			CIDRAddress:  fmt.Sprintf("10.0.0.%d/24", addrSuffix),
		}, {
			DeviceName:   "eth1",
			ConfigMethod: state.StaticAddress,
			CIDRAddress:  fmt.Sprintf("8.8.4.%d/16", addrSuffix),
		}, {
			DeviceName:   "eth1",
			ConfigMethod: state.StaticAddress,
			CIDRAddress:  fmt.Sprintf("8.8.4.%d/16", addrSuffix+1),
		}, {
			DeviceName:   "eth1.100",
			ConfigMethod: state.StaticAddress,
			CIDRAddress:  fmt.Sprintf("10.0.0.%d/24", addrSuffix+1),
		}, {
			DeviceName:   "eth2",
			ConfigMethod: state.StaticAddress,
			CIDRAddress:  fmt.Sprintf("100.64.0.%d/16", addrSuffix),
		}, {
			DeviceName:   "eth4",
			ConfigMethod: state.StaticAddress,
			CIDRAddress:  fmt.Sprintf("192.168.1.%d/24", addrSuffix),
		}}
}

func (s *uniterNetworkInfoSuite) setupUniterAPIForUnit(c *gc.C, givenUnit *state.Unit) {
	// Create a FakeAuthorizer so we can check permissions, set up assuming the
	// given unit agent has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: givenUnit.Tag(),
	}
	s.uniter = s.newUniterAPI(c, s.State, s.authorizer)
}

func (s *uniterNetworkInfoSuite) addRelationAndAssertInScope(c *gc.C) {
	// Add a relation between wordpress and mysql and enter scope with
	// mysqlUnit.
	rel := s.addRelation(c, "wordpress", "mysql")
	wpRelUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = wpRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, wpRelUnit, true)
}

func (s *uniterNetworkInfoSuite) TestNetworkInfoPermissions(c *gc.C) {
	s.addRelationAndAssertInScope(c)
	var tests = []struct {
		Name   string
		Arg    params.NetworkInfoParams
		Result params.NetworkInfoResults
		Error  string
	}{
		{
			"Wrong unit name",
			params.NetworkInfoParams{Unit: "unit-foo-0", Bindings: []string{"foo"}},
			params.NetworkInfoResults{},
			"permission denied",
		},
		{
			"Invalid tag",
			params.NetworkInfoParams{Unit: "invalid", Bindings: []string{"db-client"}},
			params.NetworkInfoResults{},
			`"invalid" is not a valid tag`,
		},
		{
			"No access to unit",
			params.NetworkInfoParams{Unit: "unit-mysql-0", Bindings: []string{"juju-info"}},
			params.NetworkInfoResults{},
			"permission denied",
		},
		{
			"Unknown binding name",
			params.NetworkInfoParams{Unit: s.wordpressUnit.Tag().String(), Bindings: []string{"unknown"}},
			params.NetworkInfoResults{
				Results: map[string]params.NetworkInfoResult{
					"unknown": {
						Error: &params.Error{
							Message: `binding name "unknown" not defined by the unit's charm`,
						},
					},
				},
			},
			"",
		},
	}

	for _, test := range tests {
		c.Logf("Testing %s", test.Name)
		result, err := s.uniter.NetworkInfo(test.Arg)
		if test.Error != "" {
			c.Check(err, gc.ErrorMatches, test.Error)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Check(result, jc.DeepEquals, test.Result)
		}
	}
}

func (s *uniterNetworkInfoSuite) TestNetworkInfoForExplicitlyBoundEndpointAndDefaultSpace(c *gc.C) {
	s.addRelationAndAssertInScope(c)

	args := params.NetworkInfoParams{
		Unit:     s.wordpressUnit.Tag().String(),
		Bindings: []string{"db", "admin-api", "db-client"},
	}
	// For the relation "wordpress:db mysql:server" we expect to see only
	// ifaces in the "internal" space, where the "db" endpoint itself
	// is bound to.
	expectedConfigWithRelationName := params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				MACAddress:    "00:11:22:33:10:50",
				InterfaceName: "eth0.100",
				Addresses: []params.InterfaceAddress{
					{Address: "10.0.0.10", CIDR: "10.0.0.0/24"},
				},
			},
			{
				MACAddress:    "00:11:22:33:10:51",
				InterfaceName: "eth1.100",
				Addresses: []params.InterfaceAddress{
					{Address: "10.0.0.11", CIDR: "10.0.0.0/24"},
				},
			},
		},
		EgressSubnets:    []string{"10.0.0.10/32"},
		IngressAddresses: []string{"10.0.0.10", "10.0.0.11"},
	}
	// For the "admin-api" extra-binding we expect to see only interfaces from
	// the "public" space.
	expectedConfigWithExtraBindingName := params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				MACAddress:    "00:11:22:33:10:50",
				InterfaceName: "eth0",
				Addresses: []params.InterfaceAddress{
					{Address: "8.8.8.10", CIDR: "8.8.0.0/16"},
				},
			},
			{
				MACAddress:    "00:11:22:33:10:51",
				InterfaceName: "eth1",
				Addresses: []params.InterfaceAddress{
					{Address: "8.8.4.10", CIDR: "8.8.0.0/16"},
					{Address: "8.8.4.11", CIDR: "8.8.0.0/16"},
				},
			},
		},
		EgressSubnets:    []string{"8.8.8.10/32"},
		IngressAddresses: []string{"8.8.8.10", "8.8.4.10", "8.8.4.11"},
	}

	// For the "db-client" extra-binding we expect to see interfaces from default
	// "wp-default" space
	expectedConfigWithDefaultSpace := params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				MACAddress:    "00:11:22:33:10:52",
				InterfaceName: "eth2",
				Addresses: []params.InterfaceAddress{
					{Address: "100.64.0.10", CIDR: "100.64.0.0/16"},
				},
			},
		},
		EgressSubnets:    []string{"100.64.0.10/32"},
		IngressAddresses: []string{"100.64.0.10"},
	}

	result, err := s.uniter.NetworkInfo(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, params.NetworkInfoResults{
		Results: map[string]params.NetworkInfoResult{
			"db":        expectedConfigWithRelationName,
			"admin-api": expectedConfigWithExtraBindingName,
			"db-client": expectedConfigWithDefaultSpace,
		},
	})
}

func (s *uniterNetworkInfoSuite) TestNetworkInfoL2Binding(c *gc.C) {
	c.Skip("L2 not supported yet")
	s.addRelationAndAssertInScope(c)

	args := params.NetworkInfoParams{
		Unit:     s.wordpressUnit.Tag().String(),
		Bindings: []string{"foo-bar"},
	}

	expectedInfo := params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				MACAddress:    "00:11:22:33:10:50",
				InterfaceName: "eth2",
			},
		},
	}

	result, err := s.uniter.NetworkInfo(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, params.NetworkInfoResults{
		Results: map[string]params.NetworkInfoResult{
			"foo-bar": expectedInfo,
		},
	})
}

func (s *uniterNetworkInfoSuite) TestNetworkInfoForImplicitlyBoundEndpoint(c *gc.C) {
	// Since wordpressUnit has explicit binding for "db", switch the API to
	// mysqlUnit and check "mysql:server" uses the machine preferred private
	// address.
	s.setupUniterAPIForUnit(c, s.mysqlUnit)
	rel := s.addRelation(c, "mysql", "wordpress")
	mysqlRelUnit, err := rel.Unit(s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, mysqlRelUnit, true)

	args := params.NetworkInfoParams{
		Unit:     s.mysqlUnit.Tag().String(),
		Bindings: []string{"server"},
	}

	expectedInfo := params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				MACAddress:    "00:11:22:33:20:54",
				InterfaceName: "eth4",
				Addresses: []params.InterfaceAddress{
					{Address: "192.168.1.20", CIDR: "192.168.1.0/24"},
				},
			},
		},
		EgressSubnets:    []string{"192.168.1.20/32"},
		IngressAddresses: []string{"192.168.1.20"},
	}

	result, err := s.uniter.NetworkInfo(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, params.NetworkInfoResults{
		Results: map[string]params.NetworkInfoResult{
			"server": expectedInfo,
		},
	})
}

func (s *uniterNetworkInfoSuite) TestNetworkInfoUsesRelationAddressNonDefaultBinding(c *gc.C) {
	// If a network info call is made in the context of a relation, and the
	// endpoint of that relation is bound to the non default space, we
	// provide the ingress addresses as those belonging to the space.
	s.setupUniterAPIForUnit(c, s.mysqlUnit)
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		SourceModel: coretesting.ModelTag,
		Name:        "wordpress-remote",
		Endpoints:   []charm.Relation{{Name: "db", Interface: "mysql", Role: "requirer"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	rel := s.addRelation(c, "mysql", "wordpress-remote")
	mysqlRelUnit, err := rel.Unit(s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, mysqlRelUnit, true)

	// Relation specific egress subnets override model config.
	err = s.JujuConnSuite.Model.UpdateModelConfig(map[string]interface{}{config.EgressSubnets: "10.0.0.0/8"}, nil)
	c.Assert(err, jc.ErrorIsNil)
	relEgress := state.NewRelationEgressNetworks(s.State)
	_, err = relEgress.Save(rel.Tag().Id(), false, []string{"192.168.1.0/24"})
	c.Assert(err, jc.ErrorIsNil)

	relId := rel.Id()
	args := params.NetworkInfoParams{
		Unit:       s.mysqlUnit.Tag().String(),
		Bindings:   []string{"server"},
		RelationId: &relId,
	}

	expectedInfo := params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				MACAddress:    "00:11:22:33:20:54",
				InterfaceName: "eth4",
				Addresses: []params.InterfaceAddress{
					{Address: "192.168.1.20", CIDR: "192.168.1.0/24"},
				},
			},
		},
		EgressSubnets:    []string{"192.168.1.0/24"},
		IngressAddresses: []string{"192.168.1.20"},
	}

	result, err := s.uniter.NetworkInfo(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, params.NetworkInfoResults{
		Results: map[string]params.NetworkInfoResult{
			"server": expectedInfo,
		},
	})
}

func (s *uniterNetworkInfoSuite) TestNetworkInfoUsesRelationAddressDefaultBinding(c *gc.C) {
	// If a network info call is made in the context of a relation, and the
	// endpoint of that relation is not bound, or bound to the default space, we
	// provide the ingress address relevant to the relation: public for CMR.
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		SourceModel: coretesting.ModelTag,
		Name:        "wordpress-remote",
		Endpoints:   []charm.Relation{{Name: "db", Interface: "mysql", Role: "requirer"}},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Recreate mysql app without endpoint binding.
	s.mysql = s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name:  "mysql-default",
		Charm: s.mysqlCharm,
	})
	s.mysqlUnit = s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: s.mysql,
		Machine:     s.machine1,
	})
	s.setupUniterAPIForUnit(c, s.mysqlUnit)

	rel := s.addRelation(c, "mysql-default", "wordpress-remote")
	mysqlRelUnit, err := rel.Unit(s.mysqlUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, mysqlRelUnit, true)

	// Relation specific egress subnets override model config.
	err = s.JujuConnSuite.Model.UpdateModelConfig(map[string]interface{}{config.EgressSubnets: "10.0.0.0/8"}, nil)
	c.Assert(err, jc.ErrorIsNil)
	relEgress := state.NewRelationEgressNetworks(s.State)
	_, err = relEgress.Save(rel.Tag().Id(), false, []string{"192.168.1.0/24"})
	c.Assert(err, jc.ErrorIsNil)

	relId := rel.Id()
	args := params.NetworkInfoParams{
		Unit:       s.mysqlUnit.Tag().String(),
		Bindings:   []string{"server"},
		RelationId: &relId,
	}

	// Since it is a remote relation, the expected ingress address is set to the
	// machine's public address.
	expectedIngressAddress, err := s.machine1.PublicAddress()
	c.Assert(err, jc.ErrorIsNil)

	expectedInfo := params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				MACAddress:    "00:11:22:33:20:50",
				InterfaceName: "eth0.100",
				Addresses: []params.InterfaceAddress{
					{Address: "10.0.0.20", CIDR: "10.0.0.0/24"},
				},
			},
		},
		EgressSubnets:    []string{"192.168.1.0/24"},
		IngressAddresses: []string{expectedIngressAddress.Value},
	}

	result, err := s.uniter.NetworkInfo(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, params.NetworkInfoResults{
		Results: map[string]params.NetworkInfoResult{
			"server": expectedInfo,
		},
	})
}

func (s *uniterNetworkInfoSuite) TestNetworkInfoV6Results(c *gc.C) {
	s.addRelationAndAssertInScope(c)

	args := params.NetworkInfoParams{
		Unit:     s.wordpressUnit.Tag().String(),
		Bindings: []string{"db"},
	}

	expectedResult := params.NetworkInfoResultsV6{
		Results: map[string]params.NetworkInfoResultV6{
			"db": {
				Info: []params.NetworkInfo{
					{
						MACAddress:    "00:11:22:33:10:50",
						InterfaceName: "eth0.100",
						Addresses:     []params.InterfaceAddress{{Address: "10.0.0.10", CIDR: "10.0.0.0/24"}},
					}, {
						MACAddress:    "00:11:22:33:10:51",
						InterfaceName: "eth1.100",
						Addresses:     []params.InterfaceAddress{{Address: "10.0.0.11", CIDR: "10.0.0.0/24"}},
					},
				},
			},
		},
	}

	apiV6, err := uniter.NewUniterAPIV6(facadetest.Context{
		State_:             s.State,
		Resources_:         s.resources,
		Auth_:              s.authorizer,
		LeadershipChecker_: s.State.LeadershipChecker(),
		Controller_:        s.Controller,
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := apiV6.NetworkInfo(args)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result, jc.DeepEquals, expectedResult)
}

func (s *uniterSuite) TestNetworkInfoCAASModelRelation(c *gc.C) {
	_, cm, gitlab, gitlabUnit := s.setupCAASModel(c)

	st := cm.State()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "mariadb", Series: "kubernetes"})
	f.MakeApplication(c, &factory.ApplicationParams{Name: "mariadb", Charm: ch})
	eps, err := st.InferEndpoints("gitlab", "mariadb")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	wpRelUnit, err := rel.Unit(gitlabUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = wpRelUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	var updateUnits state.UpdateUnitsOperation
	addr := "10.0.0.1"
	updateUnits.Updates = []*state.UpdateUnitOperation{gitlabUnit.UpdateOperation(state.UnitUpdateProperties{
		Address: &addr,
		Ports:   &[]string{"443"},
	})}
	err = gitlab.UpdateUnits(&updateUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = gitlab.UpdateCloudService("", []network.Address{
		{Value: "192.168.1.2", Scope: network.ScopeCloudLocal},
		{Value: "54.32.1.2", Scope: network.ScopePublic},
	})
	c.Assert(err, jc.ErrorIsNil)

	relId := rel.Id()
	args := params.NetworkInfoParams{
		Unit:       gitlabUnit.Tag().String(),
		Bindings:   []string{"db"},
		RelationId: &relId,
	}

	expectedResult := params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				Addresses: []params.InterfaceAddress{},
			},
		},
		EgressSubnets:    []string{"54.32.1.2/32"},
		IngressAddresses: []string{"54.32.1.2", "192.168.1.2"},
	}

	uniterAPI := s.newUniterAPI(c, st, s.authorizer)
	result, err := uniterAPI.NetworkInfo(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results["db"], jc.DeepEquals, expectedResult)
}

func (s *uniterSuite) TestNetworkInfoCAASModelNoRelation(c *gc.C) {
	_, cm, wp, wpUnit := s.setupCAASModel(c)

	st := cm.State()
	f := factory.NewFactory(st, s.StatePool)
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "mariadb", Series: "kubernetes"})
	_ = f.MakeApplication(c, &factory.ApplicationParams{Name: "mariadb", Charm: ch})

	var updateUnits state.UpdateUnitsOperation
	addr := "10.0.0.1"
	updateUnits.Updates = []*state.UpdateUnitOperation{wpUnit.UpdateOperation(state.UnitUpdateProperties{
		Address: &addr,
		Ports:   &[]string{"443"},
	})}
	err := wp.UpdateUnits(&updateUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = wp.UpdateCloudService("", []network.Address{
		{Value: "192.168.1.2", Scope: network.ScopeCloudLocal},
		{Value: "54.32.1.2", Scope: network.ScopePublic},
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(wp.Refresh(), jc.ErrorIsNil)
	c.Assert(wpUnit.Refresh(), jc.ErrorIsNil)

	s.State.StartSync()
	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())

	args := params.NetworkInfoParams{
		Unit:     wpUnit.Tag().String(),
		Bindings: []string{"db"},
	}

	expectedResult := params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				Addresses: []params.InterfaceAddress{},
			},
		},
		EgressSubnets:    []string{"54.32.1.2/32"},
		IngressAddresses: []string{"54.32.1.2", "192.168.1.2"},
	}

	uniterAPI := s.newUniterAPI(c, st, s.authorizer)
	result, err := uniterAPI.NetworkInfo(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results["db"], jc.DeepEquals, expectedResult)
}

func (s *uniterSuite) TestGetCloudSpecDeniesAccessWhenNotTrusted(c *gc.C) {
	result, err := s.uniter.CloudSpec()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.CloudSpecResult{Error: apiservertesting.ErrUnauthorized})
}

type cloudSpecUniterSuite struct {
	uniterSuiteBase
}

var _ = gc.Suite(&cloudSpecUniterSuite{})

func (s *cloudSpecUniterSuite) SetUpTest(c *gc.C) {
	s.uniterSuiteBase.SetUpTest(c)

	// Update the application config for wordpress so that it is authorised to
	// retrieve its cloud spec.
	conf := map[string]interface{}{application.TrustConfigOptionName: true}
	fields := map[string]environschema.Attr{application.TrustConfigOptionName: {Type: environschema.Tbool}}
	defaults := map[string]interface{}{application.TrustConfigOptionName: false}
	err := s.wordpress.UpdateApplicationConfig(conf, nil, fields, defaults)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *cloudSpecUniterSuite) TestGetCloudSpecReturnsSpecWhenTrusted(c *gc.C) {
	result, err := s.uniter.CloudSpec()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Error, gc.IsNil)
	c.Assert(result.Result.Name, gc.Equals, "dummy")

	exp := map[string]string{
		"username": "dummy",
		"password": "secret",
	}
	c.Assert(result.Result.Credential.Attributes, gc.DeepEquals, exp)
}

type fakeBroker struct {
	caas.Broker
}

func (*fakeBroker) APIVersion() (string, error) {
	return "6.66", nil
}

func (s *cloudSpecUniterSuite) TestCloudAPIVersion(c *gc.C) {
	_, cm, _, _ := s.setupCAASModel(c)

	uniterAPI := s.newUniterAPI(c, cm.State(), s.authorizer)
	uniter.SetNewContainerBrokerFunc(uniterAPI, func(environs.OpenParams) (caas.Broker, error) {
		return &fakeBroker{}, nil
	})

	result, err := uniterAPI.CloudAPIVersion()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringResult{
		Result: "6.66",
	})
}

type uniterV8Suite struct {
	uniterSuiteBase
	uniterV8 *uniter.UniterAPIV8
}

var _ = gc.Suite(&uniterV8Suite{})

func (s *uniterV8Suite) SetUpTest(c *gc.C) {
	s.uniterSuiteBase.SetUpTest(c)

	uniterV8, err := uniter.NewUniterAPIV8(facadetest.Context{
		State_:             s.State,
		Resources_:         s.resources,
		Auth_:              s.authorizer,
		LeadershipChecker_: s.State.LeadershipChecker(),
		Controller_:        s.Controller,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.uniterV8 = uniterV8
}

func (s *uniterV8Suite) TestWatchConfigSettings(c *gc.C) {
	err := s.wordpressUnit.SetCharmURL(s.wpCharm.URL())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniterV8.WatchConfigSettings(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, s.State, resource.(state.NotifyWatcher))
	wc.AssertNoChange()
}

func (s *uniterV8Suite) TestWatchUnitAddresses(c *gc.C) {
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
		{Tag: "machine-0"},
		{Tag: "application-wordpress"},
	}}
	result, err := s.uniterV8.WatchUnitAddresses(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, s.State, resource.(state.NotifyWatcher))
	wc.AssertNoChange()
}

func (s *uniterV8Suite) TestWatchCAASUnitAddresses(c *gc.C) {
	_, cm, _, _ := s.setupCAASModel(c)

	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-gitlab-0"},
		{Tag: "unit-foo-42"},
		{Tag: "machine-0"},
		{Tag: "application-gitlab"},
	}}

	uniterAPI, err := uniter.NewUniterAPIV8(facadetest.Context{
		State_:             cm.State(),
		Resources_:         s.resources,
		Auth_:              s.authorizer,
		LeadershipChecker_: s.State.LeadershipChecker(),
		Controller_:        s.Controller,
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := uniterAPI.WatchUnitAddresses(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{NotifyWatcherId: "1"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer statetesting.AssertStop(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, s.State, resource.(state.NotifyWatcher))
	wc.AssertNoChange()
}

type uniterAPIErrorSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&uniterAPIErrorSuite{})

func (s *uniterAPIErrorSuite) TestGetStorageStateError(c *gc.C) {
	uniter.PatchGetStorageStateError(s, errors.New("kaboom"))

	resources := common.NewResources()
	s.AddCleanup(func(_ *gc.C) { resources.StopAll() })

	_, err := uniter.NewUniterAPI(facadetest.Context{
		State_:             s.State,
		Resources_:         resources,
		Auth_:              apiservertesting.FakeAuthorizer{Tag: names.NewUnitTag("nomatter/0")},
		LeadershipChecker_: s.State.LeadershipChecker(),
	})

	c.Assert(err, gc.ErrorMatches, "kaboom")
}
