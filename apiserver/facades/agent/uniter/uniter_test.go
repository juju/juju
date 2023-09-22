// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/charm/v11"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/worker/v3/workertest"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/environschema.v1"

	apiuniter "github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/uniter"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/config"
	_ "github.com/juju/juju/internal/secrets/provider/all"
	jujujujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

const allEndpoints = ""

type uniterSuite struct {
	uniterSuiteBase
}

var _ = gc.Suite(&uniterSuite{})

func (s *uniterSuite) TestUniterFailsWithNonUnitAgentUser(c *gc.C) {
	anAuthorizer := s.authorizer
	anAuthorizer.Tag = names.NewMachineTag("9")
	context := s.facadeContext(c)
	context.Auth_ = anAuthorizer
	_, err := uniter.NewUniterAPI(context)
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
	result, err := s.uniter.SetStatus(context.Background(), args)
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
	result, err := s.uniter.SetAgentStatus(context.Background(), args)
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
	result, err := s.uniter.SetUnitStatus(context.Background(), args)
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
	result, err := s.uniter.Life(context.Background(), args)
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
	result, err := s.uniter.EnsureDead(context.Background(), args)
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
	result, err = s.uniter.EnsureDead(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{nil}},
	})
	c.Assert(s.leadershipRevoker.revoked.Contains(s.wordpressUnit.Name()), jc.IsTrue)

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
	result, err := s.uniter.Watch(context.Background(), args)
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
	defer workertest.CleanKill(c, resource1)
	resource2 := s.resources.Get("2")
	defer workertest.CleanKill(c, resource2)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewNotifyWatcherC(c, resource1.(state.NotifyWatcher))
	wc.AssertNoChange()
	wc = statetesting.NewNotifyWatcherC(c, resource2.(state.NotifyWatcher))
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
	result, err := s.uniter.PublicAddress(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: expectErr},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Now set it an try again.
	st := s.ControllerModel(c).State()
	controllerConfig, err := st.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine0.SetProviderAddresses(
		controllerConfig,
		network.NewSpaceAddress("1.2.3.4", network.WithScope(network.ScopePublic)),
	)
	c.Assert(err, jc.ErrorIsNil)
	address, err := s.wordpressUnit.PublicAddress()
	c.Assert(address.Value, gc.Equals, "1.2.3.4")
	c.Assert(err, jc.ErrorIsNil)

	result, err = s.uniter.PublicAddress(context.Background(), args)
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
	result, err := s.uniter.PrivateAddress(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: expectErr},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Now set it and try again.
	st := s.ControllerModel(c).State()
	controllerConfig, err := st.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine0.SetProviderAddresses(
		controllerConfig,
		network.NewSpaceAddress("1.2.3.4", network.WithScope(network.ScopeCloudLocal)),
	)
	c.Assert(err, jc.ErrorIsNil)
	address, err := s.wordpressUnit.PrivateAddress()
	c.Assert(address.Value, gc.Equals, "1.2.3.4")
	c.Assert(err, jc.ErrorIsNil)

	result, err = s.uniter.PrivateAddress(context.Background(), args)
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
	st := s.ControllerModel(c).State()
	controllerConfig, err := st.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine0.SetProviderAddresses(
		controllerConfig,
		network.NewSpaceAddress("1.2.3.4", network.WithScope(network.ScopeCloudLocal)),
	)
	c.Assert(err, jc.ErrorIsNil)

	err = s.ControllerModel(c).UpdateModelConfig(map[string]interface{}{config.EgressSubnets: "10.0.0.0/8"}, nil)
	c.Assert(err, jc.ErrorIsNil)

	args := params.NetworkInfoParams{
		Unit:      s.wordpressUnit.Tag().String(),
		Endpoints: []string{"db", "juju-info"},
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

	result, err := s.uniter.NetworkInfo(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result, jc.DeepEquals, params.NetworkInfoResults{
		Results: map[string]params.NetworkInfoResult{
			"db":        expectedInfo,
			"juju-info": expectedInfo,
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
	result, err := s.uniter.AvailabilityZone(context.Background(), args)
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
	result, err := s.uniter.Resolved(context.Background(), args)
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
	result, err := s.uniter.ClearResolved(context.Background(), args)
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
	result, err := s.uniter.GetPrincipal(context.Background(), args)
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
	subUniter := s.newUniterAPI(c, s.ControllerModel(c).State(), subAuthorizer)

	result, err = subUniter.GetPrincipal(context.Background(), args)
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
	result, err := s.uniter.HasSubordinates(context.Background(), args)
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

	result, err = s.uniter.HasSubordinates(context.Background(), args)
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
	result, err := s.uniter.Destroy(context.Background(), args)
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
	c.Assert(err, jc.ErrorIs, errors.NotFound)
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
	result, err := s.uniter.DestroyAllSubordinates(context.Background(), args)
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
	curl := s.wordpressUnit.CharmURL()
	c.Assert(curl, gc.NotNil)
	c.Assert(*curl, gc.Equals, s.wpCharm.URL())

	// Make sure wordpress application's charm is what we expect.
	curlStr, force := s.wordpress.CharmURL()
	c.Assert(*curlStr, gc.Equals, s.wpCharm.URL())
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
	result, err := s.uniter.CharmURL(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringBoolResults{
		Results: []params.StringBoolResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: s.wpCharm.URL(), Ok: true},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Result: s.wpCharm.URL(), Ok: force},
			{Error: apiservertesting.ErrUnauthorized},
			// see above
			// {Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestSetCharmURL(c *gc.C) {
	charmURL := s.wordpressUnit.CharmURL()
	c.Assert(charmURL, gc.IsNil)

	args := params.EntitiesCharmURL{Entities: []params.EntityCharmURL{
		{Tag: "unit-mysql-0", CharmURL: "ch:amd64/quantal/application-42"},
		{Tag: "unit-wordpress-0", CharmURL: s.wpCharm.URL()},
		{Tag: "unit-foo-42", CharmURL: "ch:amd64/quantal/foo-321"},
	}}
	result, err := s.uniter.SetCharmURL(context.Background(), args)
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

	charmURL = s.wordpressUnit.CharmURL()
	c.Assert(charmURL, gc.NotNil)
	c.Assert(*charmURL, gc.Equals, s.wpCharm.URL())
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

	result, err := s.uniter.WorkloadVersion(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringResults{
		Results: []params.StringResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Result: "capulet"},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservererrors.ServerError(errors.New(`"application-wordpress" is not a valid unit tag`))},
			{Error: apiservererrors.ServerError(errors.New(`"just-foo" is not a valid tag`))},
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
	result, err := s.uniter.SetWorkloadVersion(context.Background(), args)
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
	result, err := s.uniter.CharmModifiedVersion(context.Background(), args)
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

func (s *uniterSuite) TestWatchConfigSettingsHash(c *gc.C) {
	err := s.wordpressUnit.SetCharmURL(s.wpCharm.URL())
	c.Assert(err, jc.ErrorIsNil)
	err = s.wordpress.UpdateCharmConfig(model.GenerationMaster, charm.Settings{"blog-title": "sauceror central"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.WatchConfigSettingsHash(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{
				StringsWatcherId: "1",
				Changes:          []string{"7579d9a32a0af2e5459c21b9a6ada743db4ed33662f5230d3ca8283518268746"},
			},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done.
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *uniterSuite) TestWatchTrustConfigSettingsHash(c *gc.C) {
	schema := environschema.Fields{
		"trust": environschema.Attr{Type: environschema.Tbool},
	}
	err := s.wordpress.UpdateApplicationConfig(coreconfig.ConfigAttributes{
		"trust": true,
	}, nil, schema, nil)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.resources.Count(), gc.Equals, 0)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.WatchTrustConfigSettingsHash(context.Background(), args)
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
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *uniterSuite) TestLogActionMessage(c *gc.C) {
	operationID, err := s.ControllerModel(c).EnqueueOperation("a test", 1)
	c.Assert(err, jc.ErrorIsNil)
	anAction, err := s.ControllerModel(c).AddAction(s.wordpressUnit, operationID, "fakeaction", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(anAction.Messages(), gc.HasLen, 0)
	_, err = anAction.Begin()
	c.Assert(err, jc.ErrorIsNil)

	wrongAction, err := s.ControllerModel(c).AddAction(s.mysqlUnit, operationID, "fakeaction", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	args := params.ActionMessageParams{Messages: []params.EntityString{
		{Tag: anAction.Tag().String(), Value: "hello"},
		{Tag: wrongAction.Tag().String(), Value: "world"},
		{Tag: "foo-42", Value: "mars"},
	}}
	result, err := s.uniter.LogActionsMessages(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: &params.Error{Message: `"foo-42" is not a valid tag`}},
		},
	})
	anAction, err = s.ControllerModel(c).Action(anAction.Id())
	c.Assert(err, jc.ErrorIsNil)
	messages := anAction.Messages()
	c.Assert(messages, gc.HasLen, 1)
	c.Assert(messages[0].Message(), gc.Equals, "hello")
	c.Assert(messages[0].Timestamp(), gc.NotNil)
}

func (s *uniterSuite) TestLogActionMessageAborting(c *gc.C) {
	operationID, err := s.ControllerModel(c).EnqueueOperation("a test", 1)
	c.Assert(err, jc.ErrorIsNil)
	anAction, err := s.ControllerModel(c).AddAction(s.wordpressUnit, operationID, "fakeaction", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(anAction.Messages(), gc.HasLen, 0)
	_, err = anAction.Begin()
	c.Assert(err, jc.ErrorIsNil)

	_, err = anAction.Cancel()
	c.Assert(err, jc.ErrorIsNil)

	args := params.ActionMessageParams{Messages: []params.EntityString{
		{Tag: anAction.Tag().String(), Value: "hello"},
	}}
	result, err := s.uniter.LogActionsMessages(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{},
		},
	})
	anAction, err = s.ControllerModel(c).Action(anAction.Id())
	c.Assert(err, jc.ErrorIsNil)
	messages := anAction.Messages()
	c.Assert(messages, gc.HasLen, 1)
	c.Assert(messages[0].Message(), gc.Equals, "hello")
	c.Assert(messages[0].Timestamp(), gc.NotNil)
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
	result, err := s.uniter.WatchActionNotifications(context.Background(), args)
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
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()

	operationID, err := s.ControllerModel(c).EnqueueOperation("a test", 1)
	c.Assert(err, jc.ErrorIsNil)
	addedAction, err := s.ControllerModel(c).AddAction(s.wordpressUnit, operationID, "fakeaction", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(addedAction.Id())

	_, err = addedAction.Begin()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()

	_, err = addedAction.Cancel()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(addedAction.Id())
	wc.AssertNoChange()
}

func (s *uniterSuite) TestWatchPreexistingActions(c *gc.C) {
	err := s.wordpressUnit.SetCharmURL(s.wpCharm.URL())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.resources.Count(), gc.Equals, 0)

	operationID, err := s.ControllerModel(c).EnqueueOperation("a test", 1)
	c.Assert(err, jc.ErrorIsNil)
	action1, err := s.ControllerModel(c).AddAction(s.wordpressUnit, operationID, "fakeaction", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	action2, err := s.ControllerModel(c).AddAction(s.wordpressUnit, operationID, "fakeaction", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-wordpress-0"},
	}}

	results, err := s.uniter.WatchActionNotifications(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	checkUnorderedActionIdsEqual(c, []string{action1.Id(), action2.Id()}, results)

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()

	addedAction, err := s.ControllerModel(c).AddAction(s.wordpressUnit, operationID, "fakeaction", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChange(addedAction.Id())
	wc.AssertNoChange()
}

func (s *uniterSuite) TestWatchActionNotificationsMalformedTag(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{
		{Tag: "ewenit-mysql-0"},
	}}
	results, err := s.uniter.WatchActionNotifications(context.Background(), args)
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
	results, err := s.uniter.WatchActionNotifications(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.NotNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.NotNil)
	c.Assert(result.Error.Message, gc.Equals, `invalid actionreceiver tag "unit-mysql-01"`)
}

func (s *uniterSuite) TestWatchActionNotificationsNotUnit(c *gc.C) {
	operationID, err := s.ControllerModel(c).EnqueueOperation("a test", 1)
	c.Assert(err, jc.ErrorIsNil)
	action, err := s.ControllerModel(c).AddAction(s.mysqlUnit, operationID, "fakeaction", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	args := params.Entities{Entities: []params.Entity{
		{Tag: action.Tag().String()},
	}}
	results, err := s.uniter.WatchActionNotifications(context.Background(), args)
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
	results, err := s.uniter.WatchActionNotifications(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.NotNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	result := results.Results[0]
	c.Assert(result.Error, gc.NotNil)
	c.Assert(result.Error.Message, gc.Equals, "permission denied")
}

func (s *uniterSuite) TestConfigSettings(c *gc.C) {
	res, err := s.uniter.SetCharmURL(context.Background(), params.EntitiesCharmURL{
		Entities: []params.EntityCharmURL{
			{
				Tag:      s.wordpressUnit.Tag().String(),
				CharmURL: s.wpCharm.URL(),
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.OneError(), jc.ErrorIsNil)

	c.Assert(s.wordpressUnit.Refresh(), jc.ErrorIsNil)
	settings, err := s.wordpressUnit.ConfigSettings()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(settings, gc.DeepEquals, charm.Settings{"blog-title": "My Title"})

	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "unit-wordpress-0"},
		{Tag: "unit-foo-42"},
	}}
	result, err := s.uniter.ConfigSettings(context.Background(), args)
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
	result, err := s.uniter.WatchUnitRelations(context.Background(), args)
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
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *uniterSuite) TestWatchSubordinateUnitRelations(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	// The logging charm is subordinate (and the info endpoint is scope=container).
	loggingCharm := f.MakeCharm(c, &factory.CharmParams{
		Name: "logging",
		URL:  "ch:amd64/quantal/logging-1",
	})
	loggingApp := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "logging",
		Charm: loggingCharm,
	})

	mysqlRel := s.makeSubordinateRelation(c, loggingApp, s.mysql, s.mysqlUnit)
	wpRel := s.makeSubordinateRelation(c, loggingApp, s.wordpress, s.wordpressUnit)
	mysqlLogUnit := findSubordinateUnit(c, loggingApp, s.mysqlUnit)

	subAuthorizer := s.authorizer
	subAuthorizer.Tag = mysqlLogUnit.Tag()
	uniterAPI := s.newUniterAPI(c, s.ControllerModel(c).State(), subAuthorizer)

	result, err := uniterAPI.WatchUnitRelations(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: mysqlLogUnit.Tag().String()}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Results[0].Changes, gc.NotNil)

	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()

	// We get notified about the mysql relation going away but not the
	// wordpress one.
	err = mysqlRel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlRel.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	wc.AssertChange(mysqlRel.Tag().Id())
	wc.AssertNoChange()

	err = wpRel.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	err = wpRel.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertNoChange()
}

func (s *uniterSuite) TestWatchUnitRelationsSubordinateWithGlobalEndpoint(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	// A subordinate unit should still be notified about changes to
	// relations with applications that aren't the one this unit is
	// attached to if they have global scope.
	// The logging charm is subordinate (and the info endpoint is scope=container).
	loggingCharm := f.MakeCharm(c, &factory.CharmParams{
		Name: "logging",
		URL:  "ch:amd64/quantal/logging-1",
	})
	loggingApp := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "logging",
		Charm: loggingCharm,
	})

	uiCharm := f.MakeCharm(c, &factory.CharmParams{
		Name: "logging-frontend",
		URL:  "ch:amd64/quantal/logging-frontend-1",
	})
	uiApp := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "logging-frontend",
		Charm: uiCharm,
	})

	_ = s.makeSubordinateRelation(c, loggingApp, s.mysql, s.mysqlUnit)
	mysqlLogUnit := findSubordinateUnit(c, loggingApp, s.mysqlUnit)

	subAuthorizer := s.authorizer
	subAuthorizer.Tag = mysqlLogUnit.Tag()
	uniterAPI := s.newUniterAPI(c, s.ControllerModel(c).State(), subAuthorizer)

	result, err := uniterAPI.WatchUnitRelations(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: mysqlLogUnit.Tag().String()}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Results[0].Changes, gc.NotNil)

	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer workertest.CleanKill(c, resource)

	wc := statetesting.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()

	// Should be notified about the relation to logging frontend, since it's global scope.
	subEndpoint, err := loggingApp.Endpoint("logging-client")
	c.Assert(err, jc.ErrorIsNil)
	uiEndpoint, err := uiApp.Endpoint("logging-client")
	c.Assert(err, jc.ErrorIsNil)
	rel := f.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{subEndpoint, uiEndpoint},
	})

	wc.AssertChange(rel.Tag().Id())
	wc.AssertNoChange()
}

func (s *uniterSuite) TestWatchUnitRelationsWithSubSubRelation(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	// We should be notified about relations to other subordinates
	// (since it's possible that they'll be colocated in the same
	// container).
	loggingCharm := f.MakeCharm(c, &factory.CharmParams{
		Name: "logging",
		URL:  "ch:amd64/quantal/logging-1",
	})
	loggingApp := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "logging",
		Charm: loggingCharm,
	})
	monitoringCharm := f.MakeCharm(c, &factory.CharmParams{
		Name: "monitoring",
		URL:  "ch:amd64/quantal/monitoring-1",
	})
	monitoringApp := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "monitoring",
		Charm: monitoringCharm,
	})

	s.makeSubordinateRelation(c, loggingApp, s.mysql, s.mysqlUnit)
	mysqlMonitoring := s.makeSubordinateRelation(c, monitoringApp, s.mysql, s.mysqlUnit)

	monUnit := findSubordinateUnit(c, monitoringApp, s.mysqlUnit)

	subAuthorizer := s.authorizer
	subAuthorizer.Tag = monUnit.Tag()
	uniterAPI := s.newUniterAPI(c, s.ControllerModel(c).State(), subAuthorizer)

	result, err := uniterAPI.WatchUnitRelations(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: monUnit.Tag().String()}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(result.Results[0].StringsWatcherId, gc.Equals, "1")
	c.Assert(result.Results[0].Changes, gc.DeepEquals, []string{mysqlMonitoring.Tag().Id()})

	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()

	// Now we relate logging and monitoring together.
	monEp, err := monitoringApp.Endpoint("info")
	c.Assert(err, jc.ErrorIsNil)

	logEp, err := loggingApp.Endpoint("juju-info")
	c.Assert(err, jc.ErrorIsNil)
	rel := f.MakeRelation(c, &factory.RelationParams{
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

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	rel := f.MakeRelation(c, &factory.RelationParams{
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
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	dummyCharm := f.MakeCharm(c, &factory.CharmParams{Name: "dummy"})

	args := params.CharmURLs{URLs: []params.CharmURL{
		{URL: "something-invalid"},
		{URL: s.wpCharm.URL()},
		{URL: dummyCharm.URL()},
	}}
	result, err := s.uniter.CharmArchiveSha256(context.Background(), args)
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
	model := s.ControllerModel(c)
	result, err := s.uniter.CurrentModel(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	expected := params.ModelResult{
		Name: model.Name(),
		UUID: model.UUID(),
		Type: "iaas",
	}
	c.Assert(result, gc.DeepEquals, expected)
}

func (s *uniterSuite) TestActions(c *gc.C) {
	parallel := false
	executionGroup := "group"
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
				},
				Parallel:       &parallel,
				ExecutionGroup: &executionGroup,
			},
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
				},
				Parallel:       &parallel,
				ExecutionGroup: &executionGroup,
			},
		},
	}}

	for i, actionTest := range actionTests {
		c.Logf("test %d: %s", i, actionTest.description)

		operationID, err := s.ControllerModel(c).EnqueueOperation("a test", 1)
		c.Assert(err, jc.ErrorIsNil)
		a, err := s.ControllerModel(c).AddAction(s.wordpressUnit,
			operationID,
			actionTest.action.Action.Name,
			actionTest.action.Action.Parameters, &parallel, &executionGroup)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(names.IsValidAction(a.Id()), gc.Equals, true)
		actionTag := names.NewActionTag(a.Id())
		c.Assert(a.ActionTag(), gc.Equals, actionTag)

		args := params.Entities{
			Entities: []params.Entity{{
				Tag: actionTag.String(),
			}},
		}
		results, err := s.uniter.Actions(context.Background(), args)
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
	results, err := s.uniter.Actions(context.Background(), args)
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
	mysqlUnitFacade := s.newUniterAPI(c, s.ControllerModel(c).State(), mysqlUnitAuthorizer)

	operationID, err := s.ControllerModel(c).EnqueueOperation("a test", 1)
	c.Assert(err, jc.ErrorIsNil)
	action, err := s.ControllerModel(c).AddAction(s.wordpressUnit, operationID, "fakeaction", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: action.Tag().String(),
		}},
	}
	actions, err := mysqlUnitFacade.Actions(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(actions.Results), gc.Equals, 1)
	c.Assert(actions.Results[0].Error, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *uniterSuite) TestActionsPermissionDenied(c *gc.C) {
	operationID, err := s.ControllerModel(c).EnqueueOperation("a test", 1)
	c.Assert(err, jc.ErrorIsNil)
	action, err := s.ControllerModel(c).AddAction(s.mysqlUnit, operationID, "fakeaction", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	args := params.Entities{
		Entities: []params.Entity{{
			Tag: action.Tag().String(),
		}},
	}
	actions, err := s.uniter.Actions(context.Background(), args)
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

	operationID, err := s.ControllerModel(c).EnqueueOperation("a test", 1)
	c.Assert(err, jc.ErrorIsNil)
	action, err := s.ControllerModel(c).AddAction(s.wordpressUnit, operationID, testName, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	actionResults := params.ActionExecutionResults{
		Results: []params.ActionExecutionResult{{
			ActionTag: action.ActionTag().String(),
			Status:    params.ActionCompleted,
			Results:   testOutput,
		}},
	}
	res, err := s.uniter.FinishActions(context.Background(), actionResults)
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

	operationID, err := s.ControllerModel(c).EnqueueOperation("a test", 1)
	c.Assert(err, jc.ErrorIsNil)
	action, err := s.ControllerModel(c).AddAction(s.wordpressUnit, operationID, testName, nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	actionResults := params.ActionExecutionResults{
		Results: []params.ActionExecutionResult{{
			ActionTag: action.ActionTag().String(),
			Status:    params.ActionFailed,
			Results:   nil,
			Message:   testError,
		}},
	}
	res, err := s.uniter.FinishActions(context.Background(), actionResults)
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
	operationID, err := s.ControllerModel(c).EnqueueOperation("a test", 2)
	c.Assert(err, jc.ErrorIsNil)
	good, err := s.ControllerModel(c).AddAction(s.wordpressUnit, operationID, "fakeaction", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	bad, err := s.ControllerModel(c).AddAction(s.mysqlUnit, operationID, "fakeaction", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	var tests = []struct {
		actionTag names.ActionTag
		err       error
	}{
		{actionTag: good.ActionTag(), err: nil},
		{actionTag: bad.ActionTag(), err: apiservererrors.ErrPerm},
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
	res, err := s.uniter.FinishActions(context.Background(), actionResults)
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
	operationID, err := s.ControllerModel(c).EnqueueOperation("a test", 1)
	c.Assert(err, jc.ErrorIsNil)
	good, err := s.ControllerModel(c).AddAction(s.wordpressUnit, operationID, "fakeaction", nil, nil, nil)
	c.Assert(err, jc.ErrorIsNil)

	running, err := s.wordpressUnit.RunningActions()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(running), gc.Equals, 0, gc.Commentf("expected no running actions, got %d", len(running)))

	args := params.Entities{Entities: []params.Entity{{Tag: good.ActionTag().String()}}}
	res, err := s.uniter.BeginActions(context.Background(), args)
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
	result, err := s.uniter.Relation(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.RelationResults{
		Results: []params.RelationResult{
			{Error: apiservertesting.ErrUnauthorized},
			{
				Id:        rel.Id(),
				Key:       rel.String(),
				Life:      life.Value(rel.Life().String()),
				Suspended: rel.Suspended(),
				Endpoint: params.Endpoint{
					ApplicationName: wpEp.ApplicationName,
					Relation:        params.NewCharmRelation(wpEp.Relation),
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
	result, err := s.uniter.RelationById(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.RelationResults{
		Results: []params.RelationResult{
			{Error: apiservertesting.ErrUnauthorized},
			{
				Id:        rel.Id(),
				Key:       rel.String(),
				Life:      life.Value(rel.Life().String()),
				Suspended: rel.Suspended(),
				Endpoint: params.Endpoint{
					ApplicationName: wpEp.ApplicationName,
					Relation:        params.NewCharmRelation(wpEp.Relation),
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
	cfg, err := s.ControllerModel(c).ModelConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.uniter.ProviderType(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringResult{Result: cfg.Type()})
}

func (s *uniterSuite) TestEnterScope(c *gc.C) {
	// Set wordpressUnit's private address first.
	st := s.ControllerModel(c).State()
	controllerConfig, err := st.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine0.SetProviderAddresses(
		controllerConfig,
		network.NewSpaceAddress("1.2.3.4", network.WithScope(network.ScopeCloudLocal)),
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
	result, err := s.uniter.EnterScope(context.Background(), args)
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
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	loggingCharm := f.MakeCharm(c, &factory.CharmParams{
		Name: "logging",
		URL:  "ch:amd64/quantal/logging-1",
	})
	logging := f.MakeApplication(c, &factory.ApplicationParams{
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
	st := s.ControllerModel(c).State()
	uniterAPI := s.newUniterAPI(c, st, subAuthorizer)

	// Count how many relationscopes records there are beforehand.
	scopesBefore := countRelationScopes(c, st, mysqlRel)
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
	result, err := uniterAPI.EnterScope(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})

	scopesAfter := countRelationScopes(c, st, mysqlRel)
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
	result, err := s.uniter.LeaveScope(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{apiservertesting.ErrUnauthorized},
			{nil},
			{nil},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{apiservertesting.ErrUnauthorized},
			{&params.Error{Message: `"bar" is not a valid tag`}},
			{apiservertesting.ErrUnauthorized},
			{&params.Error{Message: `"application-wordpress" is not a valid unit tag`}},
			{&params.Error{Message: `"application-mysql" is not a valid unit tag`}},
			{&params.Error{Message: `"user-foo" is not a valid unit tag`}},
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

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "logging",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "logging"}),
	})

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
		result, err := s.uniter.RelationsStatus(context.Background(), args)
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

	s.leadershipChecker.isLeader = false
	args := params.RelationStatusArgs{
		Args: []params.RelationStatusArg{
			{s.wordpressUnit.Tag().String(), rel.Id(), params.Suspended, "message"},
		},
	}
	result, err := s.uniter.SetRelationStatus(context.Background(), args)
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

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "logging",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "logging"}),
	})
	rel2 := s.addRelation(c, "wordpress", "logging")
	err = rel2.SetSuspended(true, "")
	c.Assert(err, jc.ErrorIsNil)
	err = rel.SetStatus(status.StatusInfo{Status: status.Suspending, Message: ""})
	c.Assert(err, jc.ErrorIsNil)

	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wp2",
		Charm: s.wpCharm,
	})
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

	s.leadershipChecker.isLeader = true

	result, err := s.uniter.SetRelationStatus(context.Background(), args)
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

	err = rel.UpdateApplicationSettings("wordpress", &token{isLeader: true}, map[string]interface{}{
		"wanda": "firebaugh",
	})
	c.Assert(err, jc.ErrorIsNil)

	s.leadershipChecker.isLeader = true

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
	result, err := s.uniter.ReadSettings(context.Background(), args)
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
			{Settings: params.Settings{
				"wanda": "firebaugh",
			}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestReadSettingsForApplicationWhenNotLeader(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	settings := map[string]interface{}{
		"some": "settings",
	}
	err = relUnit.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, relUnit, true)

	err = rel.UpdateApplicationSettings("wordpress", &token{isLeader: true}, map[string]interface{}{
		"wanda": "firebaugh",
	})
	c.Assert(err, jc.ErrorIsNil)

	s.leadershipChecker.isLeader = false

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: rel.Tag().String(), Unit: "application-wordpress"},
	}}
	result, err := s.uniter.ReadSettings(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestReadSettingsForApplicationInPeerRelation(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	riak := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "riak",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "riak"}),
	})
	ep, err := riak.Endpoint("ring")
	c.Assert(err, jc.ErrorIsNil)
	st := s.ControllerModel(c).State()
	rel, err := st.EndpointsRelation(ep)
	c.Assert(err, jc.ErrorIsNil)

	err = rel.UpdateApplicationSettings("riak", &fakeToken{}, map[string]interface{}{
		"deerhoof": "little hollywood",
	})
	c.Assert(err, jc.ErrorIsNil)

	riakUnit := f.MakeUnit(c, &factory.UnitParams{
		Application: riak,
		Machine:     s.machine0,
	})

	relUnit, err := rel.Unit(riakUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	auth := apiservertesting.FakeAuthorizer{Tag: riakUnit.Tag()}
	uniter := s.newUniterAPI(c, st, auth)

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{{
		Relation: rel.Tag().String(),
		Unit:     "application-riak",
	}}}
	result, err := uniter.ReadSettings(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Settings: params.Settings{
				"deerhoof": "little hollywood",
			}},
		},
	})
}

func (s *uniterSuite) TestReadLocalApplicationSettingsWhenNotLeader(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	settings := map[string]interface{}{
		"some": "settings",
	}
	err = relUnit.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, relUnit, true)

	// This is a unit that doesn't exist.
	err = rel.UpdateApplicationSettings("wordpress", &token{isLeader: true}, map[string]interface{}{
		"wanda": "firebaugh",
	})
	c.Assert(err, jc.ErrorIsNil)

	arg := params.RelationUnit{
		Relation: rel.Tag().String(),
		Unit:     "unit-wordpress-1",
	}
	_, err = s.uniter.ReadLocalApplicationSettings(context.Background(), arg)
	c.Assert(errors.Cause(err), gc.Equals, apiservererrors.ErrPerm)
}

func (s *uniterSuite) TestReadLocalApplicationSettingsForAnotherApplicationAsAnOperator(c *gc.C) {
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	settings := map[string]interface{}{
		"some": "settings",
	}
	err = relUnit.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, relUnit, true)

	// The agent has logged in as the "riak" application; this simulates a k8s operator.
	auth := apiservertesting.FakeAuthorizer{Tag: names.NewApplicationTag("application-riak-k8s")}
	uniter := s.newUniterAPI(c, s.ControllerModel(c).State(), auth)

	// As the operator for riak, try to read the application data on behalf
	// of another application unit; the facade should reject this request
	// with a permission error as the inferred app from the unit name below
	// does not match our login credentials.
	arg := params.RelationUnit{
		Relation: rel.Tag().String(),
		Unit:     "unit-wordpress-0",
	}
	_, err = uniter.ReadLocalApplicationSettings(context.Background(), arg)
	c.Assert(errors.Cause(err), gc.Equals, apiservererrors.ErrPerm, gc.Commentf("expected ErrPerm due to mismatch in logged in app and inferred app from provided unit name"))
}

func (s *uniterSuite) TestReadLocalApplicationSettingsInPeerRelation(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	riak := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "riak",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "riak"}),
	})
	ep, err := riak.Endpoint("ring")
	c.Assert(err, jc.ErrorIsNil)
	st := s.ControllerModel(c).State()
	rel, err := st.EndpointsRelation(ep)
	c.Assert(err, jc.ErrorIsNil)

	err = rel.UpdateApplicationSettings("riak", &fakeToken{}, map[string]interface{}{
		"deerhoof": "little hollywood",
	})
	c.Assert(err, jc.ErrorIsNil)

	riakUnit := f.MakeUnit(c, &factory.UnitParams{
		Application: riak,
		Machine:     s.machine0,
	})

	relUnit, err := rel.Unit(riakUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	auth := apiservertesting.FakeAuthorizer{Tag: riakUnit.Tag()}
	uniter := s.newUniterAPI(c, st, auth)

	arg := params.RelationUnit{
		Relation: rel.Tag().String(),
		Unit:     "unit-riak-0",
	}
	result, err := uniter.ReadLocalApplicationSettings(context.Background(), arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.SettingsResult{
		Settings: params.Settings{
			"deerhoof": "little hollywood",
		},
	})
}

func (s *uniterSuite) TestReadLocalApplicationSettingsInPeerRelationAsAnOperator(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	riak := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "riak",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "riak"}),
	})
	ep, err := riak.Endpoint("ring")
	c.Assert(err, jc.ErrorIsNil)
	st := s.ControllerModel(c).State()
	rel, err := st.EndpointsRelation(ep)
	c.Assert(err, jc.ErrorIsNil)

	err = rel.UpdateApplicationSettings("riak", &fakeToken{}, map[string]interface{}{
		"deerhoof": "little hollywood",
	})
	c.Assert(err, jc.ErrorIsNil)

	riakUnit := f.MakeUnit(c, &factory.UnitParams{
		Application: riak,
		Machine:     s.machine0,
	})

	relUnit, err := rel.Unit(riakUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	// The agent has logged in as the application; this simulates a k8s operator.
	auth := apiservertesting.FakeAuthorizer{Tag: riak.Tag()}
	uniter := s.newUniterAPI(c, st, auth)

	arg := params.RelationUnit{
		Relation: rel.Tag().String(),
		Unit:     "unit-riak-0",
	}
	result, err := uniter.ReadLocalApplicationSettings(context.Background(), arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.SettingsResult{
		Settings: params.Settings{
			"deerhoof": "little hollywood",
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
	result, err := s.uniter.ReadSettings(context.Background(), args)
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
		{Relation: rel.Tag().String(), LocalUnit: "unit-wordpress-0", RemoteUnit: "application-mysql"},
		{Relation: "relation-42", LocalUnit: "unit-wordpress-0", RemoteUnit: ""},
		{Relation: "relation-foo", LocalUnit: "", RemoteUnit: ""},
		{Relation: "application-wordpress", LocalUnit: "unit-foo-0", RemoteUnit: "user-foo"},
		{Relation: "foo", LocalUnit: "bar", RemoteUnit: "baz"},
		{Relation: rel.Tag().String(), LocalUnit: "unit-mysql-0", RemoteUnit: "unit-wordpress-0"},
		{Relation: rel.Tag().String(), LocalUnit: "application-wordpress", RemoteUnit: "application-mysql"},
		{Relation: rel.Tag().String(), LocalUnit: "application-mysql", RemoteUnit: "foo"},
		{Relation: rel.Tag().String(), LocalUnit: "user-foo", RemoteUnit: "unit-wordpress-0"},
	}}
	result, err := s.uniter.ReadRemoteSettings(context.Background(), args)

	// We don't set the remote unit settings on purpose
	// to test the error.
	expectErr := `cannot read settings for unit "mysql/0" in relation "wordpress:db mysql:server": unit "mysql/0": settings`

	// The application settings are always initialised to empty when
	// the relation is created.
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("%s", pretty.Sprint(result))
	c.Assert(result, jc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.NotFoundError(expectErr)},
			{Settings: params.Settings{}},
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
	result, err = s.uniter.ReadRemoteSettings(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, expect)

	// Now destroy the remote unit, and check its settings can still be read.
	err = s.mysqlUnit.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysqlUnit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.mysqlUnit.Remove()
	c.Assert(err, jc.ErrorIsNil)
	result, err = s.uniter.ReadRemoteSettings(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, expect)
}

func (s *uniterSuite) TestReadRemoteSettingsForApplication(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "logging",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "logging"}),
	})
	rel := s.addRelation(c, "wordpress", "mysql")
	relUnit, err := rel.Unit(s.wordpressUnit)
	c.Assert(err, jc.ErrorIsNil)
	settings := map[string]interface{}{
		"some": "settings",
	}
	err = relUnit.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, relUnit, true)

	// Set some application settings for mysql and check that we can
	// see them.
	err = rel.UpdateApplicationSettings("mysql", &fakeToken{}, map[string]interface{}{
		"problem thinker": "fireproof",
	})
	c.Assert(err, jc.ErrorIsNil)

	args := params.RelationUnitPairs{RelationUnitPairs: []params.RelationUnitPair{{
		Relation:   rel.Tag().String(),
		LocalUnit:  "unit-wordpress-0",
		RemoteUnit: "application-mysql",
	}, {
		Relation:   rel.Tag().String(),
		LocalUnit:  "unit-wordpress-0",
		RemoteUnit: "application-wordpress",
	}, {
		Relation:   rel.Tag().String(),
		LocalUnit:  "unit-wordpress-0",
		RemoteUnit: "application-logging",
	}}}
	expect := params.SettingsResults{
		Results: []params.SettingsResult{
			{Settings: params.Settings{
				"problem thinker": "fireproof",
			}},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	}
	result, err := s.uniter.ReadRemoteSettings(context.Background(), args)
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
	result, err := s.uniter.ReadRemoteSettings(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Error: &params.Error{Message: expectErr}},
		},
	})
}

func (s *uniterSuite) TestReadRemoteApplicationSettingsForPeerRelation(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	riak := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "riak",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "riak"}),
	})
	ep, err := riak.Endpoint("ring")
	c.Assert(err, jc.ErrorIsNil)
	st := s.ControllerModel(c).State()
	rel, err := st.EndpointsRelation(ep)
	c.Assert(err, jc.ErrorIsNil)

	err = rel.UpdateApplicationSettings("riak", &fakeToken{}, map[string]interface{}{
		"black midi": "ducter",
	})
	c.Assert(err, jc.ErrorIsNil)

	riakUnit := f.MakeUnit(c, &factory.UnitParams{
		Application: riak,
		Machine:     s.machine0,
	})

	relUnit, err := rel.Unit(riakUnit)
	c.Assert(err, jc.ErrorIsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	auth := apiservertesting.FakeAuthorizer{Tag: riakUnit.Tag()}
	uniter := s.newUniterAPI(c, st, auth)

	args := params.RelationUnitPairs{RelationUnitPairs: []params.RelationUnitPair{{
		Relation:   rel.Tag().String(),
		LocalUnit:  "unit-riak-0",
		RemoteUnit: "application-riak",
	}}}
	result, err := uniter.ReadRemoteSettings(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Settings: params.Settings{
				"black midi": "ducter",
			}},
		},
	})
}

func (s *uniterSuite) TestReadRemoteSettingsForCAASApplicationInPeerRelationSidecar(c *gc.C) {
	_, cm, app, unit := s.setupCAASModel(c)
	c.Assert(s.resources.Count(), gc.Equals, 0)

	ep, err := app.Endpoint("ring")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := cm.State().EndpointsRelation(ep)
	c.Assert(err, jc.ErrorIsNil)

	unit2, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)

	relUnit, err := rel.Unit(unit2)
	c.Assert(err, jc.ErrorIsNil)
	err = relUnit.EnterScope(map[string]interface{}{
		"black midi": "ducter",
	})
	c.Assert(err, jc.ErrorIsNil)

	uniterAPI := s.newUniterAPI(c, cm.State(), s.authorizer)
	args := params.RelationUnitPairs{RelationUnitPairs: []params.RelationUnitPair{{
		Relation:   rel.Tag().String(),
		LocalUnit:  unit.Tag().String(),
		RemoteUnit: unit2.Tag().String(),
	}}}
	result, err := uniterAPI.ReadRemoteSettings(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.SettingsResults{
		Results: []params.SettingsResult{
			{Settings: params.Settings{
				"black midi": "ducter",
			}},
		},
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
	result, err := s.uniter.WatchRelationUnits(context.Background(), args)
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
		AppChanged: map[string]int64{
			"mysql": 0,
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
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	w, ok := resource.(common.RelationUnitsWatcher)
	c.Assert(ok, gc.Equals, true)
	select {
	case actual, ok := <-w.Changes():
		c.Fatalf("watcher sent unexpected change: (%v, %v)", actual, ok)
	case <-time.After(coretesting.ShortWait):
	}

	// Leave scope with mysqlUnit and check it's detected.
	err = myRelUnit.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	s.assertInScope(c, myRelUnit, false)

	s.assertRUWChange(c, w, nil, nil, []string{"mysql/0"})
	// TODO(jam): 2019-10-21 this test is getting a bit unweildy, but maybe we
	//  should test that changing application data triggers a change here
}

func (s *uniterSuite) assertRUWChange(c *gc.C, w common.RelationUnitsWatcher, changed []string, appChanged []string, departed []string) {
	// Cloned from state/testing.RelationUnitsWatcherC - we can't use
	// that anymore since the change type is different between the
	// state and apiserver watchers. Hacked out the code to maintain
	// state between events, since it's not needed for this test.

	// Get all items in changed in a map for easy lookup.
	changedNames := set.NewStrings(changed...)
	appChangedNames := set.NewStrings(appChanged...)
	timeout := time.After(coretesting.LongWait)
	select {
	case actual, ok := <-w.Changes():
		c.Logf("Watcher.Changes() => %# v", actual)
		c.Assert(ok, jc.IsTrue)
		c.Check(actual.Changed, gc.HasLen, len(changed))
		c.Check(actual.AppChanged, gc.HasLen, len(appChanged))
		// Because the versions can change, we only need to make sure
		// the keys match, not the contents (UnitSettings == txnRevno).
		for k := range actual.Changed {
			c.Check(changedNames.Contains(k), jc.IsTrue)
		}
		for k := range actual.AppChanged {
			c.Check(appChangedNames.Contains(k), jc.IsTrue)
		}
		c.Check(actual.Departed, jc.SameContents, departed)
	case <-timeout:
		c.Fatalf("watcher did not send change")
	}
}

func (s *uniterSuite) TestAPIAddresses(c *gc.C) {
	hostPorts := []network.SpaceHostPorts{
		network.NewSpaceHostPorts(1234, "0.1.2.3"),
	}
	st := s.ControllerModel(c).State()
	controllerConfig, err := st.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)
	err = st.SetAPIHostPorts(controllerConfig, hostPorts)
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.uniter.APIAddresses(context.Background())
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
	result, err := s.uniter.WatchUnitAddressesHash(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{Error: apiservertesting.ErrUnauthorized},
			{
				StringsWatcherId: "1",
				// The unit's machine has no network addresses
				// so the expected hash only contains the
				// sorted endpoint to space ID bindings for the
				// wordpress application.
				Changes: []string{"6048d9d417c851eddf006fa5b5435549313ee3046cf45a8223f47244d8c73e03"},
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})

	// Verify the resource was registered and stop when done
	c.Assert(s.resources.Count(), gc.Equals, 1)
	resource := s.resources.Get("1")
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, resource.(state.StringsWatcher))
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

	result, err := uniterAPI.WatchUnitAddressesHash(context.Background(), args)
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
	defer workertest.CleanKill(c, resource)

	// Check that the Watch has consumed the initial event ("returned" in
	// the Watch call)
	wc := statetesting.NewStringsWatcherC(c, resource.(state.StringsWatcher))
	wc.AssertNoChange()
}

func (s *uniterSuite) TestGetMeterStatusUnauthenticated(c *gc.C) {
	args := params.Entities{Entities: []params.Entity{{s.mysqlUnit.Tag().String()}}}
	result, err := s.uniter.GetMeterStatus(context.Background(), args)
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
	result, err := s.uniter.GetMeterStatus(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, len(tags))
	for i, result := range result.Results {
		c.Logf("checking result %d", i)
		c.Assert(result.Code, gc.Equals, "")
		c.Assert(result.Info, gc.Equals, "")
		c.Assert(result.Error, gc.ErrorMatches, "permission denied")
	}
}

func (s *uniterSuite) addRelatedApplication(c *gc.C, firstSvc, relatedApp string, unit *state.Unit) (*state.Relation, *state.Application, *state.Unit) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	relatedApplication := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  relatedApp,
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: relatedApp}),
	})
	rel := s.addRelation(c, firstSvc, relatedApp)
	relUnit, err := rel.Unit(unit)
	c.Assert(err, jc.ErrorIsNil)
	err = relUnit.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)
	relatedUnit, err := s.ControllerModel(c).State().Unit(relatedApp + "/0")
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
	errResult, err := s.uniter.RequestReboot(context.Background(), args)
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
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	// We need to set up a unit that has storage metadata defined.
	sCons := map[string]state.StorageConstraints{
		"data": {Pool: "", Size: 1024, Count: 1},
	}
	application := f.MakeApplication(c, &factory.ApplicationParams{
		Name:    "storage-block",
		Charm:   f.MakeCharm(c, &factory.CharmParams{Name: "storage-block"}),
		Storage: sCons,
	})
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	st := s.ControllerModel(c).State()
	err = st.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	assignedMachineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := st.Machine(assignedMachineId)
	c.Assert(err, jc.ErrorIsNil)

	volumeAttachments, err := machine.VolumeAttachments()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(volumeAttachments, gc.HasLen, 1)

	err = machine.SetProvisioned("inst-id", "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)

	sb, err := state.NewStorageBackend(st)
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
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	api := s.OpenModelAPIAs(c, s.ControllerModelUUID(), unit.Tag(), password, "nonce")
	uniter, err := apiuniter.NewFromConnection(api)
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
	result, err := s.uniter.UnitStatus(context.Background(), args)
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
	result, err := s.uniter.AssignedMachine(context.Background(), args)
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

func (s *uniterSuite) TestOpenedMachinePortRangesByEndpoint(c *gc.C) {
	// Verify no ports are opened yet on the machine (or unit).
	machinePortRanges, err := s.machine0.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machinePortRanges.UniquePortRanges(), gc.HasLen, 0)

	// Add another mysql unit on machine 0.
	mysqlUnit1, err := s.mysql.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = mysqlUnit1.AssignToMachine(s.machine0)
	c.Assert(err, jc.ErrorIsNil)

	// Open some ports on both units using different endpoints.
	wpPortRanges := machinePortRanges.ForUnit(s.wordpressUnit.Name())
	wpPortRanges.Open(allEndpoints, network.MustParsePortRange("100-200/tcp"))
	wpPortRanges.Open("monitoring-port", network.MustParsePortRange("10-20/udp"))

	msPortRanges := machinePortRanges.ForUnit(mysqlUnit1.Name())
	msPortRanges.Open("server", network.MustParsePortRange("3306/tcp"))

	c.Assert(s.ControllerModel(c).State().ApplyOperation(machinePortRanges.Changes()), jc.ErrorIsNil)

	// Get the open port ranges
	args := params.Entities{Entities: []params.Entity{
		{Tag: "unit-mysql-0"},
		{Tag: "machine-0"},
		{Tag: "machine-1"},
		{Tag: "unit-foo-42"},
		{Tag: "machine-42"},
		{Tag: "application-wordpress"},
	}}
	expectPortRanges := map[string][]params.OpenUnitPortRangesByEndpoint{
		"unit-mysql-1": {
			{
				Endpoint:   "server",
				PortRanges: []params.PortRange{{3306, 3306, "tcp"}},
			},
		},
		"unit-wordpress-0": {
			{
				Endpoint:   "",
				PortRanges: []params.PortRange{{100, 200, "tcp"}},
			},
			{
				Endpoint:   "monitoring-port",
				PortRanges: []params.PortRange{{10, 20, "udp"}},
			},
		},
	}
	result, err := s.uniter.OpenedMachinePortRangesByEndpoint(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.OpenPortRangesByEndpointResults{
		Results: []params.OpenPortRangesByEndpointResult{
			{Error: apiservertesting.ErrUnauthorized},
			{
				UnitPortRanges: expectPortRanges,
			},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (s *uniterSuite) TestSLALevel(c *gc.C) {
	err := s.ControllerModel(c).State().SetSLA("essential", "bob", []byte("creds"))
	c.Assert(err, jc.ErrorIsNil)

	result, err := s.uniter.SLALevel(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringResult{Result: "essential"})
}

func (s *uniterSuite) setupRemoteRelationScenario(c *gc.C) (names.Tag, *state.RelationUnit) {
	s.makeRemoteWordpress(c)

	st := s.ControllerModel(c).State()
	controllerConfig, err := st.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)

	// Set mysql's addresses first.
	err = s.machine1.SetProviderAddresses(
		controllerConfig,
		network.NewSpaceAddress("1.2.3.4", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("4.3.2.1", network.WithScope(network.ScopePublic)),
	)
	c.Assert(err, jc.ErrorIsNil)

	eps, err := st.InferEndpoints("mysql", "remote-wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := st.AddRelation(eps...)
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
	result, err := thisUniter.EnterScope(context.Background(), args)
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

	st := s.ControllerModel(c).State()
	controllerConfig, err := st.ControllerConfig()
	c.Assert(err, jc.ErrorIsNil)

	// Set mysql's addresses - no public address.
	err = s.machine1.SetProviderAddresses(
		controllerConfig,
		network.NewSpaceAddress("1.2.3.4", network.WithScope(network.ScopeCloudLocal)),
	)
	c.Assert(err, jc.ErrorIsNil)

	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: "unit-mysql-0"},
	}}
	result, err := thisUniter.EnterScope(context.Background(), args)
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
	err := s.ControllerModel(c).UpdateModelConfig(map[string]interface{}{"egress-subnets": "192.168.0.0/16"}, nil)
	c.Assert(err, jc.ErrorIsNil)

	egress := state.NewRelationEgressNetworks(s.ControllerModel(c).State())
	_, err = egress.Save(relTag.Id(), false, []string{"10.0.0.0/16", "10.1.2.0/8"})
	c.Assert(err, jc.ErrorIsNil)

	thisUniter := s.makeMysqlUniter(c)
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: "unit-mysql-0"},
	}}

	result, err := thisUniter.EnterScope(context.Background(), args)
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

	err := s.ControllerModel(c).UpdateModelConfig(map[string]interface{}{"egress-subnets": "192.168.0.0/16"}, nil)
	c.Assert(err, jc.ErrorIsNil)

	thisUniter := s.makeMysqlUniter(c)
	args := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: relTag.String(), Unit: "unit-mysql-0"},
	}}
	result, err := thisUniter.EnterScope(context.Background(), args)
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
	return s.newUniterAPI(c, s.ControllerModel(c).State(), authorizer)
}

func (s *uniterSuite) makeRemoteWordpress(c *gc.C) {
	_, err := s.ControllerModel(c).State().AddRemoteApplication(state.AddRemoteApplicationParams{
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
			{Life: life.Alive, Resolved: params.ResolvedNone},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	}
	results, err := s.uniter.Refresh(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, expect)
}

func (s *uniterSuite) TestRefreshNoArgs(c *gc.C) {
	results, err := s.uniter.Refresh(context.Background(), params.Entities{Entities: []params.Entity{}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.UnitRefreshResults{Results: []params.UnitRefreshResult{}})
}

func (s *uniterSuite) TestOpenedApplicationPortRangesByEndpoint(c *gc.C) {
	_, cm, app, unit := s.setupCAASModel(c)
	st := cm.State()

	appPortRanges, err := app.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appPortRanges.UniquePortRanges(), gc.HasLen, 0)

	portRanges, err := unit.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)

	// Open some ports using different endpoints.
	portRanges.Open(allEndpoints, network.MustParsePortRange("1000/tcp"))
	portRanges.Open("db", network.MustParsePortRange("1111/udp"))

	c.Assert(st.ApplyOperation(portRanges.Changes()), jc.ErrorIsNil)

	// Get the open port ranges
	arg := params.Entity{Tag: "application-gitlab"}
	expectPortRanges := []params.ApplicationOpenedPorts{
		{
			Endpoint:   "",
			PortRanges: []params.PortRange{{FromPort: 1000, ToPort: 1000, Protocol: "tcp"}},
		},
		{
			Endpoint:   "db",
			PortRanges: []params.PortRange{{FromPort: 1111, ToPort: 1111, Protocol: "udp"}},
		},
	}

	uniterAPI := s.newUniterAPI(c, st, s.authorizer)

	result, err := uniterAPI.OpenedApplicationPortRangesByEndpoint(context.Background(), arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ApplicationOpenedPortsResults{
		Results: []params.ApplicationOpenedPortsResult{
			{ApplicationPortRanges: expectPortRanges},
		},
	})
}

func (s *uniterSuite) TestOpenedPortRangesByEndpoint(c *gc.C) {
	_, cm, app, unit := s.setupCAASModel(c)
	st := cm.State()

	appPortRanges, err := app.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appPortRanges.UniquePortRanges(), gc.HasLen, 0)

	portRanges, err := unit.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)

	// Open some ports using different endpoints.
	portRanges.Open(allEndpoints, network.MustParsePortRange("1000/tcp"))
	portRanges.Open("db", network.MustParsePortRange("1111/udp"))

	c.Assert(st.ApplyOperation(portRanges.Changes()), jc.ErrorIsNil)

	// Get the open port ranges
	expectPortRanges := []params.OpenUnitPortRangesByEndpoint{
		{
			Endpoint:   "",
			PortRanges: []params.PortRange{{FromPort: 1000, ToPort: 1000, Protocol: "tcp"}},
		},
		{
			Endpoint:   "db",
			PortRanges: []params.PortRange{{FromPort: 1111, ToPort: 1111, Protocol: "udp"}},
		},
	}

	uniterAPI := s.newUniterAPI(c, st, s.authorizer)

	result, err := uniterAPI.OpenedPortRangesByEndpoint(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.OpenPortRangesByEndpointResults{
		Results: []params.OpenPortRangesByEndpointResult{
			{
				UnitPortRanges: map[string][]params.OpenUnitPortRangesByEndpoint{
					"unit-gitlab-0": expectPortRanges,
				},
			},
		},
	})
}

func (s *uniterSuite) TestCommitHookChangesWithSecrets(c *gc.C) {
	s.addRelatedApplication(c, "wordpress", "logging", s.wordpressUnit)
	s.leadershipChecker.isLeader = true
	st := s.ControllerModel(c).State()
	store := state.NewSecrets(st)
	uri2 := secrets.NewURI()
	_, err := store.CreateSecret(uri2, state.CreateSecretParams{
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &token{isLeader: true},
			Data:        map[string]string{"foo2": "bar"},
		},
		Owner: s.wordpress.Tag(),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = st.GrantSecretAccess(uri2, state.SecretAccessParams{
		LeaderToken: &token{isLeader: true},
		Scope:       s.wordpress.Tag(),
		Subject:     s.wordpress.Tag(),
		Role:        secrets.RoleManage,
	})
	c.Assert(err, jc.ErrorIsNil)
	uri3 := secrets.NewURI()
	_, err = store.CreateSecret(uri3, state.CreateSecretParams{
		UpdateSecretParams: state.UpdateSecretParams{
			LeaderToken: &token{isLeader: true},
			Data:        map[string]string{"foo3": "bar"},
		},
		Owner: s.wordpress.Tag(),
	})
	c.Assert(err, jc.ErrorIsNil)
	err = st.GrantSecretAccess(uri3, state.SecretAccessParams{
		LeaderToken: &token{isLeader: true},
		Scope:       s.wordpress.Tag(),
		Subject:     s.wordpress.Tag(),
		Role:        secrets.RoleManage,
	})
	c.Assert(err, jc.ErrorIsNil)

	b := apiuniter.NewCommitHookParamsBuilder(s.wordpressUnit.UnitTag())
	uri := secrets.NewURI()
	b.AddSecretCreates([]apiuniter.SecretCreateArg{{
		SecretUpsertArg: apiuniter.SecretUpsertArg{
			URI:   uri,
			Label: ptr("foobar"),
			Value: secrets.NewSecretValue(map[string]string{"foo": "bar"}),
		},
		OwnerTag: s.wordpress.Tag(),
	}})
	b.AddSecretUpdates([]apiuniter.SecretUpsertArg{{
		URI:          uri,
		RotatePolicy: ptr(secrets.RotateDaily),
		Description:  ptr("a secret"),
		Label:        ptr("foobar"),
		Value:        secrets.NewSecretValue(map[string]string{"foo": "bar2"}),
	}})
	b.AddSecretDeletes([]apiuniter.SecretDeleteArg{{URI: uri3, Revision: ptr(1)}})
	b.AddSecretGrants([]apiuniter.SecretGrantRevokeArgs{{
		URI:             uri,
		ApplicationName: ptr(s.mysql.Name()),
		Role:            secrets.RoleView,
	}, {
		URI:             uri2,
		ApplicationName: ptr(s.mysql.Name()),
		Role:            secrets.RoleView,
	}})
	b.AddSecretRevokes([]apiuniter.SecretGrantRevokeArgs{{
		URI:             uri2,
		ApplicationName: ptr(s.mysql.Name()),
	}})
	req, _ := b.Build()

	result, err := s.uniter.CommitHookChanges(context.Background(), req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	// Verify state
	_, err = store.GetSecret(uri3)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
	md, err := store.GetSecret(uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(md.Description, gc.Equals, "a secret")
	c.Assert(md.Label, gc.Equals, "foobar")
	c.Assert(md.RotatePolicy, gc.Equals, secrets.RotateDaily)
	val, _, err := store.GetSecretValue(uri, 2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(val.EncodedValues(), jc.DeepEquals, map[string]string{"foo": "bar2"})
	access, err := st.SecretAccess(uri, s.mysql.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, secrets.RoleView)
	access, err = st.SecretAccess(uri2, s.mysql.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(access, gc.Equals, secrets.RoleNone)
}

func (s *uniterSuite) TestCommitHookChangesWithStorage(c *gc.C) {
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	// We need to set up a unit that has storage metadata defined.
	application := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "storage-block2",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "storage-block2"}),
	})
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	st := s.ControllerModel(c).State()
	err = st.AssignUnit(unit, state.AssignCleanEmpty)
	c.Assert(err, jc.ErrorIsNil)
	assignedMachineId, err := unit.AssignedMachineId()
	c.Assert(err, jc.ErrorIsNil)
	machine, err := st.Machine(assignedMachineId)
	c.Assert(err, jc.ErrorIsNil)
	oldVolumeAttachments, err := machine.VolumeAttachments()
	c.Assert(err, jc.ErrorIsNil)

	stCount := uint64(1)
	b := apiuniter.NewCommitHookParamsBuilder(unit.UnitTag())
	b.UpdateNetworkInfo()
	b.OpenPortRange(allEndpoints, network.MustParsePortRange("80-81/tcp"))
	b.OpenPortRange(allEndpoints, network.MustParsePortRange("7337/tcp")) // same port closed below; this should be a no-op
	b.ClosePortRange(allEndpoints, network.MustParsePortRange("7337/tcp"))
	b.UpdateCharmState(map[string]string{"charm-key": "charm-value"})
	b.AddStorage(map[string][]params.StorageConstraints{
		"multi1to10": {{Count: &stCount}},
	})
	req, _ := b.Build()

	// Test-suite uses an older API version. Create a new one and override
	// authorizer to allow access to the unit we just created.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: unit.Tag(),
	}
	api, err := uniter.NewUniterAPI(s.facadeContext(c))
	c.Assert(err, jc.ErrorIsNil)

	result, err := api.CommitHookChanges(context.Background(), req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	// Verify state
	unitPortRanges, err := unit.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitPortRanges.UniquePortRanges(), jc.DeepEquals, []network.PortRange{{Protocol: "tcp", FromPort: 80, ToPort: 81}})

	unitState, err := unit.State()
	c.Assert(err, jc.ErrorIsNil)
	charmState, _ := unitState.CharmState()
	c.Assert(charmState, jc.DeepEquals, map[string]string{"charm-key": "charm-value"}, gc.Commentf("state doc not updated"))

	newVolumeAttachments, err := machine.VolumeAttachments()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newVolumeAttachments, gc.HasLen, len(oldVolumeAttachments)+1, gc.Commentf("expected an additional instance of block storage to be added"))
}

func (s *uniterSuite) TestCommitHookChangesWithPortsSidecarApplication(c *gc.C) {
	_, cm, app, unit := s.setupCAASModel(c)

	b := apiuniter.NewCommitHookParamsBuilder(unit.UnitTag())
	b.UpdateNetworkInfo()
	b.UpdateCharmState(map[string]string{"charm-key": "charm-value"})

	b.OpenPortRange("db", network.MustParsePortRange("80/tcp"))
	b.OpenPortRange("db", network.MustParsePortRange("7337/tcp")) // same port closed below; this should be a no-op
	b.ClosePortRange("db", network.MustParsePortRange("7337/tcp"))
	req, _ := b.Build()

	s.authorizer = apiservertesting.FakeAuthorizer{Tag: unit.Tag()}
	uniterAPI, err := uniter.NewUniterAPI(facadetest.Context{
		State_:             cm.State(),
		StatePool_:         s.StatePool(),
		Resources_:         s.resources,
		Auth_:              s.authorizer,
		LeadershipChecker_: s.leadershipChecker,
		ServiceFactory_:    s.ServiceFactory(jujujujutesting.DefaultModelUUID),
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := uniterAPI.CommitHookChanges(context.Background(), req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	appPortRanges, err := app.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(appPortRanges.UniquePortRanges(), jc.DeepEquals, []network.PortRange{{Protocol: "tcp", FromPort: 80, ToPort: 80}})

	portRanges, err := unit.OpenedPortRanges()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(portRanges.ByEndpoint(), jc.DeepEquals, network.GroupedPortRanges{
		"db": []network.PortRange{network.MustParsePortRange("80/tcp")},
	})
}

func (s *uniterNetworkInfoSuite) TestCommitHookChangesCAAS(c *gc.C) {
	_, cm, _, unit := s.setupCAASModel(c)

	s.leadershipChecker.isLeader = true

	b := apiuniter.NewCommitHookParamsBuilder(unit.UnitTag())
	b.UpdateNetworkInfo()
	b.UpdateCharmState(map[string]string{"charm-key": "charm-value"})

	req, _ := b.Build()

	s.st = cm.State()
	s.authorizer = apiservertesting.FakeAuthorizer{Tag: unit.Tag()}
	uniterAPI := s.newUniterAPI(c, s.st, s.authorizer)

	result, err := uniterAPI.CommitHookChanges(context.Background(), req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{Error: nil},
		},
	})

	// Verify expected unit state
	unitState, err := unit.State()
	c.Assert(err, jc.ErrorIsNil)
	charmState, _ := unitState.CharmState()
	c.Assert(charmState, jc.DeepEquals, map[string]string{"charm-key": "charm-value"}, gc.Commentf("state doc not updated"))
}

func (s *uniterSuite) TestNetworkInfoCAASModelRelation(c *gc.C) {
	_, cm, gitlab, gitlabUnit := s.setupCAASModel(c)

	f, release := s.NewFactory(c, cm.UUID())
	defer release()

	st := cm.State()
	ch := f.MakeCharm(c, &factory.CharmParams{Name: "mariadb-k8s", Series: "focal"})
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

	err = gitlab.UpdateCloudService("", []network.SpaceAddress{
		network.NewSpaceAddress("192.168.1.2", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("54.32.1.2", network.WithScope(network.ScopePublic)),
	})
	c.Assert(err, jc.ErrorIsNil)

	relId := rel.Id()
	args := params.NetworkInfoParams{
		Unit:       gitlabUnit.Tag().String(),
		Endpoints:  []string{"db"},
		RelationId: &relId,
	}

	expectedResult := params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				Addresses: []params.InterfaceAddress{
					{Address: "10.0.0.1"},
				},
			},
		},
		EgressSubnets:    []string{"54.32.1.2/32"},
		IngressAddresses: []string{"54.32.1.2", "192.168.1.2"},
	}

	uniterAPI := s.newUniterAPI(c, st, s.authorizer)
	result, err := uniterAPI.NetworkInfo(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results["db"], jc.DeepEquals, expectedResult)
}

func (s *uniterSuite) TestNetworkInfoCAASModelNoRelation(c *gc.C) {
	_, cm, wp, wpUnit := s.setupCAASModel(c)

	f, release := s.NewFactory(c, cm.UUID())
	defer release()

	ch := f.MakeCharm(c, &factory.CharmParams{Name: "mariadb-k8s", Series: "focal"})
	_ = f.MakeApplication(c, &factory.ApplicationParams{Name: "mariadb", Charm: ch})

	var updateUnits state.UpdateUnitsOperation
	addr := "10.0.0.1"
	updateUnits.Updates = []*state.UpdateUnitOperation{wpUnit.UpdateOperation(state.UnitUpdateProperties{
		Address: &addr,
		Ports:   &[]string{"443"},
	})}
	err := wp.UpdateUnits(&updateUnits)
	c.Assert(err, jc.ErrorIsNil)

	err = wp.UpdateCloudService("", []network.SpaceAddress{
		network.NewSpaceAddress("192.168.1.2", network.WithScope(network.ScopeCloudLocal)),
		network.NewSpaceAddress("54.32.1.2", network.WithScope(network.ScopePublic)),
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(wp.Refresh(), jc.ErrorIsNil)
	c.Assert(wpUnit.Refresh(), jc.ErrorIsNil)

	args := params.NetworkInfoParams{
		Unit:      wpUnit.Tag().String(),
		Endpoints: []string{"db"},
	}

	expectedResult := params.NetworkInfoResult{
		Info: []params.NetworkInfo{
			{
				Addresses: []params.InterfaceAddress{
					{Address: "10.0.0.1"},
				},
			},
		},
		EgressSubnets:    []string{"54.32.1.2/32"},
		IngressAddresses: []string{"54.32.1.2", "192.168.1.2"},
	}

	uniterAPI := s.newUniterAPI(c, cm.State(), s.authorizer)
	result, err := uniterAPI.NetworkInfo(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Results["db"], jc.DeepEquals, expectedResult)
}

func (s *uniterSuite) TestGetCloudSpecDeniesAccessWhenNotTrusted(c *gc.C) {
	result, err := s.uniter.CloudSpec(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.CloudSpecResult{Error: apiservertesting.ErrUnauthorized})
}
