// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v7"
	"github.com/juju/charm/v7/hooks"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/relation"
	"github.com/juju/juju/worker/uniter/relation/mocks"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
	"github.com/juju/juju/worker/uniter/runner/context"
)

/*
TODO(wallyworld)
DO NOT COPY THE METHODOLOGY USED IN THE relationResolverSuite.
We want to write unit tests without resorting to JujuConnSuite.
However, the current api/uniter code uses structs instead of
interfaces for its component model, and it's not possible to
implement a stub uniter api at the model level due to the way
the domain objects reference each other.

The best we can do for now is to stub out the facade caller and
return curated values for each API call.
*/

type relationResolverSuite struct {
	coretesting.BaseSuite

	charmDir              string
	leadershipContextFunc relation.LeadershipContextFunc
}

var (
	_ = gc.Suite(&relationResolverSuite{})
	_ = gc.Suite(&relationCreatedResolverSuite{})
	_ = gc.Suite(&mockRelationResolverSuite{})
)

type apiCall struct {
	request string
	args    interface{}
	result  interface{}
	err     error
}

func uniterAPICall(request string, args, result interface{}, err error) apiCall {
	return apiCall{
		request: request,
		args:    args,
		result:  result,
		err:     err,
	}
}

func mockAPICaller(c *gc.C, callNumber *int32, apiCalls ...apiCall) apitesting.APICallerFunc {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		switch objType {
		case "NotifyWatcher":
			return nil
		case "Uniter":
			index := int(atomic.AddInt32(callNumber, 1)) - 1
			c.Check(index <= len(apiCalls), jc.IsTrue, gc.Commentf("index = %d; len(apiCalls) = %d", index, len(apiCalls)))
			call := apiCalls[index]
			c.Logf("request %d, %s", index, request)
			c.Check(version, gc.Equals, 0)
			c.Check(id, gc.Equals, "")
			c.Check(request, gc.Equals, call.request)
			c.Check(arg, jc.DeepEquals, call.args)
			if call.err != nil {
				return common.ServerError(call.err)
			}
			testing.PatchValue(result, call.result)
		default:
			c.Fail()
		}
		return nil
	})
	return apiCaller
}

type stubLeadershipContext struct {
	context.LeadershipContext
	isLeader bool
}

func (stub *stubLeadershipContext) IsLeader() (bool, error) {
	return stub.isLeader, nil
}

var minimalMetadata = `
name: wordpress
summary: "test"
description: "test"
requires:
  mysql: db
`[1:]

func (s *relationResolverSuite) SetUpTest(c *gc.C) {
	s.charmDir = filepath.Join(c.MkDir(), "charm")
	err := os.MkdirAll(s.charmDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(s.charmDir, "metadata.yaml"), []byte(minimalMetadata), 0755)
	c.Assert(err, jc.ErrorIsNil)
	s.leadershipContextFunc = func(accessor context.LeadershipSettingsAccessor, tracker leadership.Tracker, unitName string) context.LeadershipContext {
		return &stubLeadershipContext{isLeader: true}
	}
}

func assertNumCalls(c *gc.C, numCalls *int32, expected int32) {
	v := atomic.LoadInt32(numCalls)
	c.Assert(v, gc.Equals, expected)
}

func (s *relationResolverSuite) setupRelations(c *gc.C) relation.RelationStateTracker {
	unitTag := names.NewUnitTag("wordpress/0")
	abort := make(chan struct{})

	var numCalls int32
	unitEntity := params.Entities{Entities: []params.Entity{{Tag: "unit-wordpress-0"}}}
	unitStateResults := params.UnitStateResults{Results: []params.UnitStateResult{{}}}
	apiCaller := mockAPICaller(c, &numCalls,
		uniterAPICall("Refresh", unitEntity, params.UnitRefreshResults{Results: []params.UnitRefreshResult{{Life: life.Alive, Resolved: params.ResolvedNone}}}, nil),
		uniterAPICall("GetPrincipal", unitEntity, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "", Ok: false}}}, nil),
		uniterAPICall("RelationsStatus", unitEntity, params.RelationUnitStatusResults{Results: []params.RelationUnitStatusResult{{RelationResults: []params.RelationUnitStatus{}}}}, nil),
		uniterAPICall("State", unitEntity, unitStateResults, nil),
	)
	st := uniter.NewState(apiCaller, unitTag)
	u, err := st.Unit(unitTag)
	c.Assert(err, jc.ErrorIsNil)
	r, err := relation.NewRelationStateTracker(
		relation.RelationStateTrackerConfig{
			State:                st,
			Unit:                 u,
			CharmDir:             s.charmDir,
			NewLeadershipContext: s.leadershipContextFunc,
			Abort:                abort,
		})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, &numCalls, 4)
	return r
}

func (s *relationResolverSuite) TestNewRelationsNoRelations(c *gc.C) {
	r := s.setupRelations(c)
	//No relations created.
	c.Assert(r.GetInfo(), gc.HasLen, 0)
}

func (s *relationResolverSuite) assertNewRelationsWithExistingRelations(c *gc.C, isLeader bool) {
	unitTag := names.NewUnitTag("wordpress/0")
	abort := make(chan struct{})
	s.leadershipContextFunc = func(accessor context.LeadershipSettingsAccessor, tracker leadership.Tracker, unitName string) context.LeadershipContext {
		return &stubLeadershipContext{isLeader: isLeader}
	}

	var numCalls int32
	unitEntity := params.Entities{Entities: []params.Entity{{Tag: "unit-wordpress-0"}}}
	relationUnits := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-wordpress.db#mysql.db", Unit: "unit-wordpress-0"},
	}}
	relationResults := params.RelationResults{
		Results: []params.RelationResult{
			{
				Id:   1,
				Key:  "wordpress:db mysql:db",
				Life: life.Alive,
				Endpoint: params.Endpoint{
					ApplicationName: "wordpress",
					Relation:        params.CharmRelation{Name: "mysql", Role: string(charm.RoleProvider), Interface: "db"},
				}},
		},
	}
	relationStatus := params.RelationStatusArgs{Args: []params.RelationStatusArg{{
		UnitTag:    "unit-wordpress-0",
		RelationId: 1,
		Status:     params.Joined,
	}}}
	unitSetStateArgs := params.SetUnitStateArgs{
		Args: []params.SetUnitStateArg{{
			Tag:           "unit-wordpress-0",
			RelationState: &map[int]string{1: "id: 1\n"},
		},
		}}
	unitStateResults := params.UnitStateResults{Results: []params.UnitStateResult{{}}}

	apiCalls := []apiCall{
		uniterAPICall("Refresh", unitEntity, params.UnitRefreshResults{Results: []params.UnitRefreshResult{{Life: life.Alive, Resolved: params.ResolvedNone}}}, nil),
		uniterAPICall("GetPrincipal", unitEntity, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "", Ok: false}}}, nil),
		uniterAPICall("RelationsStatus", unitEntity, params.RelationUnitStatusResults{Results: []params.RelationUnitStatusResult{
			{RelationResults: []params.RelationUnitStatus{{RelationTag: "relation-wordpress:db mysql:db", InScope: true}}}}}, nil),
		uniterAPICall("State", unitEntity, unitStateResults, nil),
		uniterAPICall("Relation", relationUnits, relationResults, nil),
		uniterAPICall("Relation", relationUnits, relationResults, nil),
		uniterAPICall("Watch", unitEntity, params.NotifyWatchResults{Results: []params.NotifyWatchResult{{NotifyWatcherId: "1"}}}, nil),
		uniterAPICall("SetState", unitSetStateArgs, noErrorResult, nil),
		uniterAPICall("EnterScope", relationUnits, params.ErrorResults{Results: []params.ErrorResult{{}}}, nil),
	}
	if isLeader {
		apiCalls = append(apiCalls,
			uniterAPICall("SetRelationStatus", relationStatus, noErrorResult, nil),
		)
	}
	apiCaller := mockAPICaller(c, &numCalls, apiCalls...)
	st := uniter.NewState(apiCaller, unitTag)
	u, err := st.Unit(unitTag)
	c.Assert(err, jc.ErrorIsNil)
	r, err := relation.NewRelationStateTracker(
		relation.RelationStateTrackerConfig{
			State:                st,
			Unit:                 u,
			CharmDir:             s.charmDir,
			NewLeadershipContext: s.leadershipContextFunc,
			Abort:                abort,
		})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, &numCalls, int32(len(apiCalls)))

	info := r.GetInfo()
	c.Assert(info, gc.HasLen, 1)
	oneInfo := info[1]
	c.Assert(oneInfo.RelationUnit.Relation().Tag(), gc.Equals, names.NewRelationTag("wordpress:db mysql:db"))
	c.Assert(oneInfo.RelationUnit.Endpoint(), jc.DeepEquals, uniter.Endpoint{
		Relation: charm.Relation{Name: "mysql", Role: "provider", Interface: "db", Optional: false, Limit: 0, Scope: ""},
	})
	c.Assert(oneInfo.MemberNames, gc.HasLen, 0)
}

func (s *relationResolverSuite) TestNewRelationsWithExistingRelationsLeader(c *gc.C) {
	s.assertNewRelationsWithExistingRelations(c, true)
}

func (s *relationResolverSuite) TestNewRelationsWithExistingRelationsNotLeader(c *gc.C) {
	s.assertNewRelationsWithExistingRelations(c, false)
}

func (s *relationResolverSuite) TestNextOpNothing(c *gc.C) {
	unitTag := names.NewUnitTag("wordpress/0")
	abort := make(chan struct{})

	var numCalls int32
	unitEntity := params.Entities{Entities: []params.Entity{{Tag: "unit-wordpress-0"}}}
	unitStateResults := params.UnitStateResults{Results: []params.UnitStateResult{{}}}
	apiCaller := mockAPICaller(c, &numCalls,
		uniterAPICall("Refresh", unitEntity, params.UnitRefreshResults{Results: []params.UnitRefreshResult{{Life: life.Alive, Resolved: params.ResolvedNone}}}, nil),
		uniterAPICall("GetPrincipal", unitEntity, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "", Ok: false}}}, nil),
		uniterAPICall("RelationsStatus", unitEntity, params.RelationUnitStatusResults{Results: []params.RelationUnitStatusResult{{RelationResults: []params.RelationUnitStatus{}}}}, nil),
		uniterAPICall("State", unitEntity, unitStateResults, nil),
	)
	st := uniter.NewState(apiCaller, unitTag)
	u, err := st.Unit(unitTag)
	c.Assert(err, jc.ErrorIsNil)
	r, err := relation.NewRelationStateTracker(
		relation.RelationStateTrackerConfig{
			State:                st,
			Unit:                 u,
			CharmDir:             s.charmDir,
			NewLeadershipContext: s.leadershipContextFunc,
			Abort:                abort,
		})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, &numCalls, 4)

	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{}
	relationsResolver := relation.NewRelationResolver(r, nil)
	_, err = relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(errors.Cause(err), gc.Equals, resolver.ErrNoOperation)
}

func relationJoinedAPICalls() []apiCall {
	apiCalls := relationJoinedAPICalls2SetState()
	unitSetStateArgs3 := params.SetUnitStateArgs{
		Args: []params.SetUnitStateArg{{
			Tag:           "unit-wordpress-0",
			RelationState: &map[int]string{1: "id: 1\nmembers:\n  wordpress/0: 0\n"},
		},
		}}
	return append(apiCalls, uniterAPICall("SetState", unitSetStateArgs3, noErrorResult, nil))
}

func relationJoinedAPICalls2SetState() []apiCall {
	unitEntity := params.Entities{Entities: []params.Entity{{Tag: "unit-wordpress-0"}}}
	relationResults := params.RelationResults{
		Results: []params.RelationResult{
			{
				Id:   1,
				Key:  "wordpress:db mysql:db",
				Life: life.Alive,
				Endpoint: params.Endpoint{
					ApplicationName: "wordpress",
					Relation:        params.CharmRelation{Name: "mysql", Role: string(charm.RoleRequirer), Interface: "db", Scope: "global"},
				}},
		},
	}
	relationUnits := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-wordpress.db#mysql.db", Unit: "unit-wordpress-0"},
	}}
	relationStatus := params.RelationStatusArgs{Args: []params.RelationStatusArg{{
		UnitTag:    "unit-wordpress-0",
		RelationId: 1,
		Status:     params.Joined,
	}}}
	unitSetStateArgs := params.SetUnitStateArgs{
		Args: []params.SetUnitStateArg{{
			Tag:           "unit-wordpress-0",
			RelationState: &map[int]string{1: "id: 1\n"},
		},
		}}
	unitSetStateArgs2 := params.SetUnitStateArgs{
		Args: []params.SetUnitStateArg{{
			Tag:           "unit-wordpress-0",
			RelationState: &map[int]string{1: "id: 1\nmembers:\n  wordpress/0: 1\nchanged-pending: wordpress/0\n"},
		},
		}}

	unitStateResults := params.UnitStateResults{Results: []params.UnitStateResult{{}}}
	apiCalls := []apiCall{
		uniterAPICall("Refresh", unitEntity, params.UnitRefreshResults{Results: []params.UnitRefreshResult{{Life: life.Alive, Resolved: params.ResolvedNone}}}, nil),
		uniterAPICall("GetPrincipal", unitEntity, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "", Ok: false}}}, nil),
		uniterAPICall("RelationsStatus", unitEntity, params.RelationUnitStatusResults{Results: []params.RelationUnitStatusResult{{RelationResults: []params.RelationUnitStatus{}}}}, nil),
		uniterAPICall("State", unitEntity, unitStateResults, nil),
		uniterAPICall("RelationById", params.RelationIds{RelationIds: []int{1}}, relationResults, nil),
		uniterAPICall("Relation", relationUnits, relationResults, nil),
		//uniterAPICall("State", unitEntity, unitStateResults, nil),
		uniterAPICall("Relation", relationUnits, relationResults, nil),
		uniterAPICall("Watch", unitEntity, params.NotifyWatchResults{Results: []params.NotifyWatchResult{{NotifyWatcherId: "1"}}}, nil),
		uniterAPICall("SetState", unitSetStateArgs, noErrorResult, nil),
		uniterAPICall("EnterScope", relationUnits, params.ErrorResults{Results: []params.ErrorResult{{}}}, nil),
		uniterAPICall("SetRelationStatus", relationStatus, noErrorResult, nil),
		uniterAPICall("SetState", unitSetStateArgs2, noErrorResult, nil),
	}
	return apiCalls
}

func relationJoinedAndDepartedAPICalls() []apiCall {
	apiCalls := relationJoinedAndDepartedAPICallsNoState()
	unitEntity := params.Entities{Entities: []params.Entity{{Tag: "unit-wordpress-0"}}}
	unitStateResults := params.UnitStateResults{Results: []params.UnitStateResult{{
		RelationState: map[int]string{1: "id: 1\n", 74: ""},
	}}}
	return append(apiCalls, uniterAPICall("State", unitEntity, unitStateResults, nil))
}

func relationJoinedAndDepartedAPICallsEmptyState() []apiCall {
	apiCalls := relationJoinedAndDepartedAPICallsNoState()
	unitEntity := params.Entities{Entities: []params.Entity{{Tag: "unit-wordpress-0"}}}
	unitStateResults := params.UnitStateResults{Results: []params.UnitStateResult{{
		RelationState: map[int]string{},
	}}}
	return append(apiCalls, uniterAPICall("State", unitEntity, unitStateResults, nil))
}

func relationJoinedAndDepartedAPICallsNoState() []apiCall {
	apiCalls := relationJoinedAPICalls()

	// Resolver calls Refresh to check the life for the local unit and Life
	// to check the app life before emitting a relation-departed hook
	refreshReq := params.Entities{Entities: []params.Entity{{Tag: "unit-wordpress-0"}}}
	refreshRes := params.UnitRefreshResults{
		Results: []params.UnitRefreshResult{
			{Life: life.Alive},
		},
	}

	lifeReq := params.Entities{Entities: []params.Entity{{Tag: "application-wordpress"}}}
	lifeRes := params.LifeResults{
		Results: []params.LifeResult{
			{Life: life.Alive},
		},
	}

	unitSetStateArgs := params.SetUnitStateArgs{
		Args: []params.SetUnitStateArg{{
			Tag:           "unit-wordpress-0",
			RelationState: &map[int]string{1: "id: 1\n"},
		},
		}}

	return append(apiCalls,
		uniterAPICall("Refresh", refreshReq, refreshRes, nil),
		uniterAPICall("Life", lifeReq, lifeRes, nil),
		uniterAPICall("SetState", unitSetStateArgs, noErrorResult, nil),
	)
}

func (s *relationResolverSuite) assertHookRelationJoined(c *gc.C, numCalls *int32, apiCalls ...apiCall) relation.RelationStateTracker {
	unitTag := names.NewUnitTag("wordpress/0")
	abort := make(chan struct{})

	apiCaller := mockAPICaller(c, numCalls, apiCalls...)
	st := uniter.NewState(apiCaller, unitTag)
	u, err := st.Unit(unitTag)
	c.Assert(err, jc.ErrorIsNil)
	r, err := relation.NewRelationStateTracker(
		relation.RelationStateTrackerConfig{
			State:                st,
			Unit:                 u,
			CharmDir:             s.charmDir,
			NewLeadershipContext: s.leadershipContextFunc,
			Abort:                abort,
		})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, numCalls, 4)

	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life:      life.Alive,
				Suspended: false,
				Members: map[string]int64{
					"wordpress/0": 1,
				},
				ApplicationMembers: map[string]int64{
					"wordpress": 0,
				},
			},
		},
	}
	relationsResolver := relation.NewRelationResolver(r, nil)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, numCalls, 11)
	c.Assert(op.String(), gc.Equals, "run hook relation-joined on unit wordpress/0 with relation 1")

	_, err = r.PrepareHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CommitHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	return r
}

func (s *relationResolverSuite) TestHookRelationJoined(c *gc.C) {
	var numCalls int32
	s.assertHookRelationJoined(c, &numCalls, relationJoinedAPICalls()...)
}

func (s *relationResolverSuite) assertHookRelationChanged(
	c *gc.C, r relation.RelationStateTracker,
	remoteRelationSnapshot remotestate.RelationSnapshot,
	numCalls *int32,
) {
	numCallsBefore := *numCalls
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: remoteRelationSnapshot,
		},
	}
	relationsResolver := relation.NewRelationResolver(r, nil)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, numCalls, numCallsBefore)
	c.Assert(op.String(), gc.Equals, "run hook relation-changed on unit wordpress/0 with relation 1")

	// Commit the operation so we save local state for any next operation.
	_, err = r.PrepareHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CommitHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationResolverSuite) TestHookRelationChanged(c *gc.C) {
	var numCalls int32
	apiCalls := relationJoinedAPICalls()
	unitSetStateArgs := params.SetUnitStateArgs{
		Args: []params.SetUnitStateArg{{
			Tag:           "unit-wordpress-0",
			RelationState: &map[int]string{1: "id: 1\nmembers:\n  wordpress/0: 2\n"},
		},
		}}
	unitSetStateArgs2 := params.SetUnitStateArgs{
		Args: []params.SetUnitStateArg{{
			Tag:           "unit-wordpress-0",
			RelationState: &map[int]string{1: "id: 1\nmembers:\n  wordpress/0: 1\n"},
		},
		}}
	apiCalls = append(apiCalls,
		uniterAPICall("SetState", unitSetStateArgs, noErrorResult, nil),
		uniterAPICall("SetState", unitSetStateArgs2, noErrorResult, nil),
	)
	r := s.assertHookRelationJoined(c, &numCalls, apiCalls...)

	// There will be an initial relation-changed regardless of
	// members, due to the "changed pending" local persistent
	// state.
	s.assertHookRelationChanged(c, r, remotestate.RelationSnapshot{
		Life:      life.Alive,
		Suspended: false,
	}, &numCalls)

	// wordpress starts at 1, changing to 2 should trigger a
	// relation-changed hook.
	s.assertHookRelationChanged(c, r, remotestate.RelationSnapshot{
		Life:      life.Alive,
		Suspended: false,
		Members: map[string]int64{
			"wordpress/0": 2,
		},
	}, &numCalls)

	// NOTE(axw) this is a test for the temporary to fix lp:1495542.
	//
	// wordpress is at 2, changing to 1 should trigger a
	// relation-changed hook. This is to cater for the scenario
	// where the relation settings document is removed and
	// recreated, thus resetting the txn-revno.
	s.assertHookRelationChanged(c, r, remotestate.RelationSnapshot{
		Life: life.Alive,
		Members: map[string]int64{
			"wordpress/0": 1,
		},
	}, &numCalls)
}

func (s *relationResolverSuite) TestHookRelationChangedApplication(c *gc.C) {
	var numCalls int32
	apiCalls := relationJoinedAPICalls()
	r := s.assertHookRelationJoined(c, &numCalls, apiCalls...)

	// There will be an initial relation-changed regardless of
	// members, due to the "changed pending" local persistent
	// state.
	s.assertHookRelationChanged(c, r, remotestate.RelationSnapshot{
		Life:      life.Alive,
		Suspended: false,
	}, &numCalls)

	// wordpress app starts at 0, changing to 1 should trigger a
	// relation-changed hook for the app. We also leave wordpress/0 at 1 so that
	// it doesn't trigger relation-departed or relation-changed.
	numCallsBefore := numCalls
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life:      life.Alive,
				Suspended: false,
				Members: map[string]int64{
					"wordpress/0": 1,
				},
				ApplicationMembers: map[string]int64{
					"wordpress": 1,
				},
			},
		},
	}
	relationsResolver := relation.NewRelationResolver(r, nil)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	// No new calls
	assertNumCalls(c, &numCalls, numCallsBefore)
	c.Assert(op.String(), gc.Equals, "run hook relation-changed on app wordpress with relation 1")
}

func (s *relationResolverSuite) TestHookRelationChangedSuspended(c *gc.C) {
	var numCalls int32
	apiCalls := relationJoinedAndDepartedAPICalls()
	r := s.assertHookRelationJoined(c, &numCalls, apiCalls...)

	// There will be an initial relation-changed regardless of
	// members, due to the "changed pending" local persistent
	// state.
	s.assertHookRelationChanged(c, r, remotestate.RelationSnapshot{
		Life:      life.Alive,
		Suspended: true,
	}, &numCalls)
	c.Assert(r.GetInfo()[1].RelationUnit.Relation().Suspended(), jc.IsTrue)

	numCallsBefore := numCalls

	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life:      life.Alive,
				Suspended: true,
			},
		},
	}

	relationsResolver := relation.NewRelationResolver(r, nil)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, &numCalls, numCallsBefore+2) // Refresh/Life calls made by the resolver prior to emitting a RelationDeparted hook
	c.Assert(op.String(), gc.Equals, "run hook relation-departed on unit wordpress/0 with relation 1")
}

func (s *relationResolverSuite) assertHookRelationDeparted(c *gc.C, numCalls *int32, apiCalls ...apiCall) relation.RelationStateTracker {
	r := s.assertHookRelationJoined(c, numCalls, apiCalls...)
	s.assertHookRelationChanged(c, r, remotestate.RelationSnapshot{
		Life:      life.Alive,
		Suspended: false,
	}, numCalls)
	numCallsBefore := *numCalls

	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life: life.Dying,
				Members: map[string]int64{
					"wordpress/0": 1,
				},
			},
		},
	}
	relationsResolver := relation.NewRelationResolver(r, nil)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, numCalls, numCallsBefore+2) // Refresh/Life calls made by the resolver prior to emitting a RelationDeparted hook
	c.Assert(op.String(), gc.Equals, "run hook relation-departed on unit wordpress/0 with relation 1")

	// Commit the operation so we save local state for any next operation.
	_, err = r.PrepareHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CommitHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	return r
}

func (s *relationResolverSuite) TestHookRelationDeparted(c *gc.C) {
	var numCalls int32
	apiCalls := relationJoinedAndDepartedAPICalls()

	s.assertHookRelationDeparted(c, &numCalls, apiCalls...)
}

func (s *relationResolverSuite) TestHookRelationBroken(c *gc.C) {
	var numCalls int32
	apiCalls := relationJoinedAndDepartedAPICalls()

	r := s.assertHookRelationDeparted(c, &numCalls, apiCalls...)

	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life: life.Dying,
			},
		},
	}
	relationsResolver := relation.NewRelationResolver(r, nil)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, &numCalls, 16)
	c.Assert(op.String(), gc.Equals, "run hook relation-broken with relation 1")
}

func (s *relationResolverSuite) TestHookRelationBrokenWhenSuspended(c *gc.C) {
	var numCalls int32
	apiCalls := relationJoinedAndDepartedAPICalls()

	r := s.assertHookRelationDeparted(c, &numCalls, apiCalls...)

	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life:      life.Alive,
				Suspended: true,
			},
		},
	}
	relationsResolver := relation.NewRelationResolver(r, nil)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, &numCalls, 16)
	c.Assert(op.String(), gc.Equals, "run hook relation-broken with relation 1")
}

func (s *relationResolverSuite) TestHookRelationBrokenOnlyOnce(c *gc.C) {
	var numCalls int32
	apiCalls := relationJoinedAndDepartedAPICallsNoState()
	relationUnits := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-wordpress.db#mysql.db", Unit: "unit-wordpress-0"},
	}}
	unitSetStateArgs3 := params.SetUnitStateArgs{
		Args: []params.SetUnitStateArg{{
			Tag:           "unit-wordpress-0",
			RelationState: &map[int]string{},
		}}}
	apiCalls = append(apiCalls,
		uniterAPICall("LeaveScope", relationUnits, params.ErrorResults{Results: []params.ErrorResult{{}}}, nil),
		uniterAPICall("SetState", unitSetStateArgs3, noErrorResult, nil),
	)
	r := s.assertHookRelationDeparted(c, &numCalls, apiCalls...)

	// Setup above received and ran CommitHook for:
	// relation-joined
	// relation-changed
	// relation-departed

	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life:      life.Alive,
				Suspended: true,
			},
		},
	}
	// Get RelationBroken once.
	relationsResolver := relation.NewRelationResolver(r, nil)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)

	// Commit the RelationBroken, so the NextOp will do the correct thing.
	mockOp, ok := op.(*mockOperation)
	c.Assert(ok, jc.IsTrue)
	c.Assert(mockOp.hookInfo.Kind, gc.Equals, hooks.RelationBroken)
	err = r.CommitHook(mockOp.hookInfo)

	op, err = relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(errors.Cause(err), gc.Equals, resolver.ErrNoOperation)
}

func (s *relationResolverSuite) TestCommitHook(c *gc.C) {
	var numCalls int32
	apiCalls := relationJoinedAPICalls2SetState()
	relationUnits := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-wordpress.db#mysql.db", Unit: "unit-wordpress-0"},
	}}
	unitSetStateArgs := params.SetUnitStateArgs{
		Args: []params.SetUnitStateArg{{
			Tag:           "unit-wordpress-0",
			RelationState: &map[int]string{1: "id: 1\nmembers:\n  wordpress/0: 2\n"},
		}}}
	unitSetStateArgs2 := params.SetUnitStateArgs{
		Args: []params.SetUnitStateArg{{
			Tag:           "unit-wordpress-0",
			RelationState: &map[int]string{1: "id: 1\n"},
		}}}
	// ops.Remove() via die()
	unitSetStateArgs3 := params.SetUnitStateArgs{
		Args: []params.SetUnitStateArg{{
			Tag:           "unit-wordpress-0",
			RelationState: &map[int]string{1: ""},
		}}}
	apiCalls = append(apiCalls,
		uniterAPICall("SetState", unitSetStateArgs, noErrorResult, nil),
		uniterAPICall("SetState", unitSetStateArgs2, noErrorResult, nil),
		uniterAPICall("LeaveScope", relationUnits, params.ErrorResults{Results: []params.ErrorResult{{}}}, nil),
		uniterAPICall("SetState", unitSetStateArgs3, noErrorResult, nil),
	)
	r := s.assertHookRelationJoined(c, &numCalls, apiCalls...)

	err := r.CommitHook(hook.Info{
		Kind:              hooks.RelationChanged,
		RemoteUnit:        "wordpress/0",
		RemoteApplication: "wordpress",
		RelationId:        1,
		ChangeVersion:     2,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = r.CommitHook(hook.Info{
		Kind:              hooks.RelationDeparted,
		RemoteUnit:        "wordpress/0",
		RemoteApplication: "wordpress",
		RelationId:        1,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationResolverSuite) TestImplicitRelationNoHooks(c *gc.C) {
	unitTag := names.NewUnitTag("wordpress/0")
	abort := make(chan struct{})

	unitEntity := params.Entities{Entities: []params.Entity{{Tag: "unit-wordpress-0"}}}
	relationResults := params.RelationResults{
		Results: []params.RelationResult{
			{
				Id:   1,
				Key:  "wordpress:juju-info juju-info:juju-info",
				Life: life.Alive,
				Endpoint: params.Endpoint{
					ApplicationName: "wordpress",
					Relation:        params.CharmRelation{Name: "juju-info", Role: string(charm.RoleProvider), Interface: "juju-info", Scope: "global"},
				}},
		},
	}
	relationUnits := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-wordpress.juju-info#juju-info.juju-info", Unit: "unit-wordpress-0"},
	}}
	relationStatus := params.RelationStatusArgs{Args: []params.RelationStatusArg{{
		UnitTag:    "unit-wordpress-0",
		RelationId: 1,
		Status:     params.Joined,
	}}}
	unitSetStateArgs := params.SetUnitStateArgs{
		Args: []params.SetUnitStateArg{{
			Tag:           "unit-wordpress-0",
			RelationState: &map[int]string{1: "id: 1\n"},
		},
		}}
	// ReadStateDir
	unitStateResults := params.UnitStateResults{Results: []params.UnitStateResult{{}}}

	apiCalls := []apiCall{
		uniterAPICall("Refresh", unitEntity, params.UnitRefreshResults{Results: []params.UnitRefreshResult{{Life: life.Alive, Resolved: params.ResolvedNone}}}, nil),
		uniterAPICall("GetPrincipal", unitEntity, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "", Ok: false}}}, nil),
		uniterAPICall("RelationsStatus", unitEntity, params.RelationUnitStatusResults{Results: []params.RelationUnitStatusResult{{RelationResults: []params.RelationUnitStatus{}}}}, nil),
		uniterAPICall("State", unitEntity, unitStateResults, nil),
		uniterAPICall("RelationById", params.RelationIds{RelationIds: []int{1}}, relationResults, nil),
		uniterAPICall("Relation", relationUnits, relationResults, nil),
		//uniterAPICall("State", unitEntity, unitStateResults, nil),
		uniterAPICall("Relation", relationUnits, relationResults, nil),
		uniterAPICall("Watch", unitEntity, params.NotifyWatchResults{Results: []params.NotifyWatchResult{{NotifyWatcherId: "1"}}}, nil),
		uniterAPICall("SetState", unitSetStateArgs, noErrorResult, nil),
		uniterAPICall("EnterScope", relationUnits, params.ErrorResults{Results: []params.ErrorResult{{}}}, nil),
		uniterAPICall("SetRelationStatus", relationStatus, noErrorResult, nil),
	}

	var numCalls int32
	apiCaller := mockAPICaller(c, &numCalls, apiCalls...)
	st := uniter.NewState(apiCaller, unitTag)
	u, err := st.Unit(unitTag)
	c.Assert(err, jc.ErrorIsNil)
	r, err := relation.NewRelationStateTracker(
		relation.RelationStateTrackerConfig{
			State:                st,
			Unit:                 u,
			CharmDir:             s.charmDir,
			NewLeadershipContext: s.leadershipContextFunc,
			Abort:                abort,
		})
	c.Assert(err, jc.ErrorIsNil)

	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life: life.Alive,
				Members: map[string]int64{
					"wordpress": 1,
				},
			},
		},
	}
	relationsResolver := relation.NewRelationResolver(r, nil)
	_, err = relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(errors.Cause(err), gc.Equals, resolver.ErrNoOperation)
}

var (
	noErrorResult  = params.ErrorResults{Results: []params.ErrorResult{{}}}
	nrpeUnitTag    = names.NewUnitTag("nrpe/0")
	nrpeUnitEntity = params.Entities{Entities: []params.Entity{{Tag: nrpeUnitTag.String()}}}
)

func subSubRelationAPICalls() []apiCall {
	relationStatusResults := params.RelationUnitStatusResults{Results: []params.RelationUnitStatusResult{{
		RelationResults: []params.RelationUnitStatus{{
			RelationTag: "relation-wordpress:juju-info nrpe:general-info",
			InScope:     true,
		}, {
			RelationTag: "relation-ntp:nrpe-external-master nrpe:external-master",
			InScope:     true,
		},
		}}}}
	relationUnits1 := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-wordpress.juju-info#nrpe.general-info", Unit: "unit-nrpe-0"},
	}}
	relationResults1 := params.RelationResults{
		Results: []params.RelationResult{{
			Id:               1,
			Key:              "wordpress:juju-info nrpe:general-info",
			Life:             life.Alive,
			OtherApplication: "wordpress",
			Endpoint: params.Endpoint{
				ApplicationName: "nrpe",
				Relation: params.CharmRelation{
					Name:      "general-info",
					Role:      string(charm.RoleRequirer),
					Interface: "juju-info",
					Scope:     "container",
				},
			},
		}},
	}
	relationUnits2 := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-ntp.nrpe-external-master#nrpe.external-master", Unit: "unit-nrpe-0"},
	}}
	relationResults2 := params.RelationResults{
		Results: []params.RelationResult{{
			Id:               2,
			Key:              "ntp:nrpe-external-master nrpe:external-master",
			Life:             life.Alive,
			OtherApplication: "ntp",
			Endpoint: params.Endpoint{
				ApplicationName: "nrpe",
				Relation: params.CharmRelation{
					Name:      "external-master",
					Role:      string(charm.RoleRequirer),
					Interface: "nrpe-external-master",
					Scope:     "container",
				},
			},
		}},
	}
	relationStatus1 := params.RelationStatusArgs{Args: []params.RelationStatusArg{{
		UnitTag:    "unit-nrpe-0",
		RelationId: 1,
		Status:     params.Joined,
	}}}
	relationStatus2 := params.RelationStatusArgs{Args: []params.RelationStatusArg{{
		UnitTag:    "unit-nrpe-0",
		RelationId: 2,
		Status:     params.Joined,
	}}}

	unitSetStateArgs1 := params.SetUnitStateArgs{
		Args: []params.SetUnitStateArg{{
			Tag:           "unit-nrpe-0",
			RelationState: &map[int]string{1: "id: 1\n"},
		},
		}}
	unitSetStateArgs2 := params.SetUnitStateArgs{
		Args: []params.SetUnitStateArg{{
			Tag:           "unit-nrpe-0",
			RelationState: &map[int]string{1: "id: 1\n", 2: "id: 2\n"},
		},
		}}
	unitStateResults := params.UnitStateResults{Results: []params.UnitStateResult{{}}}

	return []apiCall{
		uniterAPICall("Refresh", nrpeUnitEntity, params.UnitRefreshResults{Results: []params.UnitRefreshResult{{Life: life.Alive, Resolved: params.ResolvedNone}}}, nil),
		uniterAPICall("GetPrincipal", nrpeUnitEntity, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "unit-wordpress-0", Ok: true}}}, nil),
		uniterAPICall("RelationsStatus", nrpeUnitEntity, relationStatusResults, nil),
		uniterAPICall("State", nrpeUnitEntity, unitStateResults, nil),
		uniterAPICall("Relation", relationUnits1, relationResults1, nil),
		uniterAPICall("Relation", relationUnits2, relationResults2, nil),
		uniterAPICall("Relation", relationUnits1, relationResults1, nil),
		uniterAPICall("Watch", nrpeUnitEntity, params.NotifyWatchResults{Results: []params.NotifyWatchResult{{NotifyWatcherId: "1"}}}, nil),
		uniterAPICall("SetState", unitSetStateArgs1, noErrorResult, nil),
		uniterAPICall("EnterScope", relationUnits1, noErrorResult, nil),
		uniterAPICall("SetRelationStatus", relationStatus1, noErrorResult, nil),
		uniterAPICall("Relation", relationUnits2, relationResults2, nil),
		uniterAPICall("Watch", nrpeUnitEntity, params.NotifyWatchResults{Results: []params.NotifyWatchResult{{NotifyWatcherId: "2"}}}, nil),
		uniterAPICall("SetState", unitSetStateArgs2, noErrorResult, nil),
		uniterAPICall("EnterScope", relationUnits2, noErrorResult, nil),
		uniterAPICall("SetRelationStatus", relationStatus2, noErrorResult, nil),
	}
}

func (s *relationResolverSuite) TestSubSubPrincipalRelationDyingDestroysUnit(c *gc.C) {
	// When two subordinate units are related on a principal unit's
	// machine, the sub-sub relation shouldn't keep them alive if the
	// relation to the principal dies.
	var numCalls int32
	apiCalls := subSubRelationAPICalls()
	callsBeforeDestroy := int32(len(apiCalls))

	// This should only be called once the relation to the
	// principal app is destroyed.
	apiCalls = append(apiCalls, uniterAPICall("Destroy", nrpeUnitEntity, noErrorResult, nil))
	//unitStateResults := params.UnitStateResults{Results: []params.UnitStateResult{{
	//	RelationState: map[int]string{2: "id: 2\n"},
	//}}}
	//apiCalls = append(apiCalls, uniterAPICall("State", nrpeUnitEntity, unitStateResults, nil))
	apiCaller := mockAPICaller(c, &numCalls, apiCalls...)

	st := uniter.NewState(apiCaller, nrpeUnitTag)
	u, err := st.Unit(nrpeUnitTag)
	c.Assert(err, jc.ErrorIsNil)
	r, err := relation.NewRelationStateTracker(
		relation.RelationStateTrackerConfig{
			State:                st,
			Unit:                 u,
			CharmDir:             s.charmDir,
			NewLeadershipContext: s.leadershipContextFunc,
			Abort:                make(chan struct{}),
		})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, &numCalls, callsBeforeDestroy)

	// So now we have a relations object with two relations, one to
	// wordpress and one to ntp. We want to ensure that if the
	// relation to wordpress changes to Dying, the unit is destroyed,
	// even if the ntp relation is still going strong.
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}

	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life: life.Dying,
				Members: map[string]int64{
					"wordpress/0": 1,
				},
			},
			2: {
				Life: life.Alive,
				Members: map[string]int64{
					"ntp/0": 1,
				},
			},
		},
	}

	relationResolver := relation.NewRelationResolver(r, nil)
	_, err = relationResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)

	// Check that we've made the destroy unit call.
	//
	// TODO: Fix this test...
	// This test intermittently makes either 17 or 18
	// calls.  Number 17 is destroy, so ensure we've
	// called at least that.
	c.Assert(atomic.LoadInt32(&numCalls), jc.GreaterThan, 16)
}

func (s *relationResolverSuite) TestSubSubOtherRelationDyingNotDestroyed(c *gc.C) {
	var numCalls int32
	apiCalls := subSubRelationAPICalls()
	// Sanity check: there shouldn't be a destroy at the end.
	c.Assert(apiCalls[len(apiCalls)-1].request, gc.Not(gc.Equals), "Destroy")

	//unitStateResults := params.UnitStateResults{Results: []params.UnitStateResult{{
	//	RelationState: map[int]string{2: "id: 2\n"},
	//}}}
	//apiCalls = append(apiCalls, uniterAPICall("State", nrpeUnitEntity, unitStateResults, nil))

	apiCaller := mockAPICaller(c, &numCalls, apiCalls...)

	st := uniter.NewState(apiCaller, nrpeUnitTag)
	u, err := st.Unit(nrpeUnitTag)
	c.Assert(err, jc.ErrorIsNil)
	r, err := relation.NewRelationStateTracker(
		relation.RelationStateTrackerConfig{
			State:                st,
			Unit:                 u,
			CharmDir:             s.charmDir,
			NewLeadershipContext: s.leadershipContextFunc,
			Abort:                make(chan struct{}),
		})
	c.Assert(err, jc.ErrorIsNil)

	// TODO: Fix this test...
	// This test intermittently makes either 16 or 17
	// calls.  Number 16 is destroy, so ensure we've
	// called at least that.
	c.Assert(atomic.LoadInt32(&numCalls), jc.GreaterThan, 15)

	// So now we have a relations object with two relations, one to
	// wordpress and one to ntp. We want to ensure that if the
	// relation to ntp changes to Dying, the unit isn't destroyed,
	// since it's kept alive by the principal relation.
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}

	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life: life.Alive,
				Members: map[string]int64{
					"wordpress/0": 1,
				},
			},
			2: {
				Life: life.Dying,
				Members: map[string]int64{
					"ntp/0": 1,
				},
			},
		},
	}

	relationResolver := relation.NewRelationResolver(r, nil)
	_, err = relationResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)

	// Check that we didn't try to make a destroy call (the apiCaller
	// should panic in that case anyway).
	// TODO: Fix this test...
	// This test intermittently makes either 16 or 17
	// calls.  Number 16 is destroy, so ensure we've
	// called at least that.
	c.Assert(atomic.LoadInt32(&numCalls), jc.GreaterThan, 15)
}

func principalWithSubordinateAPICalls() []apiCall {
	relationStatusResults := params.RelationUnitStatusResults{Results: []params.RelationUnitStatusResult{{
		RelationResults: []params.RelationUnitStatus{{
			RelationTag: "relation-wordpress:juju-info nrpe:general-info",
			InScope:     true,
		},
		}}}}
	relationUnits1 := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-wordpress.juju-info#nrpe.general-info", Unit: "unit-nrpe-0"},
	}}
	relationResults1 := params.RelationResults{
		Results: []params.RelationResult{{
			Id:               1,
			Key:              "wordpress:juju-info nrpe:general-info",
			Life:             life.Alive,
			OtherApplication: "wordpress",
			Endpoint: params.Endpoint{
				ApplicationName: "nrpe",
				Relation: params.CharmRelation{
					Name:      "general-info",
					Role:      string(charm.RoleRequirer),
					Interface: "juju-info",
					Scope:     "container",
				},
			},
		}},
	}
	relationStatus1 := params.RelationStatusArgs{Args: []params.RelationStatusArg{{
		UnitTag:    "unit-nrpe-0",
		RelationId: 1,
		Status:     params.Joined,
	}}}

	unitSetStateArgs := params.SetUnitStateArgs{
		Args: []params.SetUnitStateArg{{
			Tag:           "unit-nrpe-0",
			RelationState: &map[int]string{1: "id: 1\n"},
		},
		}}
	unitStateResults := params.UnitStateResults{Results: []params.UnitStateResult{{}}}

	return []apiCall{
		uniterAPICall("Refresh", nrpeUnitEntity, params.UnitRefreshResults{Results: []params.UnitRefreshResult{{Life: life.Alive, Resolved: params.ResolvedNone}}}, nil),
		uniterAPICall("GetPrincipal", nrpeUnitEntity, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "unit-wordpress-0", Ok: true}}}, nil),
		uniterAPICall("RelationsStatus", nrpeUnitEntity, relationStatusResults, nil),
		uniterAPICall("State", nrpeUnitEntity, unitStateResults, nil),
		uniterAPICall("Relation", relationUnits1, relationResults1, nil),
		uniterAPICall("Relation", relationUnits1, relationResults1, nil),
		uniterAPICall("Watch", nrpeUnitEntity, params.NotifyWatchResults{Results: []params.NotifyWatchResult{{NotifyWatcherId: "1"}}}, nil),
		uniterAPICall("SetState", unitSetStateArgs, noErrorResult, nil),
		uniterAPICall("EnterScope", relationUnits1, noErrorResult, nil),
		uniterAPICall("SetRelationStatus", relationStatus1, noErrorResult, nil),
	}
}

func (s *relationResolverSuite) TestPrincipalDyingDestroysSubordinates(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var numCalls int32
	apiCalls := principalWithSubordinateAPICalls()
	callsBeforeDestroy := int32(len(apiCalls))
	callsAfterDestroy := callsBeforeDestroy + 1
	// This should only be called after we queue the subordinate for destruction
	apiCalls = append(apiCalls, uniterAPICall("Destroy", nrpeUnitEntity, noErrorResult, nil))
	//unitStateResults := params.UnitStateResults{Results: []params.UnitStateResult{{
	//	RelationState: map[int]string{1: "id: 1\n", 73: ""},
	//}}}
	//apiCalls = append(apiCalls, uniterAPICall("State", nrpeUnitEntity, unitStateResults, nil))
	apiCaller := mockAPICaller(c, &numCalls, apiCalls...)

	st := uniter.NewState(apiCaller, nrpeUnitTag)
	u, err := st.Unit(nrpeUnitTag)
	c.Assert(err, jc.ErrorIsNil)
	r, err := relation.NewRelationStateTracker(
		relation.RelationStateTrackerConfig{
			State:                st,
			Unit:                 u,
			CharmDir:             s.charmDir,
			NewLeadershipContext: s.leadershipContextFunc,
			Abort:                make(chan struct{}),
		})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, &numCalls, callsBeforeDestroy)

	// So now we have a relation between a principal (wordpress) and a
	// subordinate (nrpe). If the wordpress unit is being destroyed,
	// the subordinate must be also queued for destruction.
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}

	remoteState := remotestate.Snapshot{
		Life: life.Dying,
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life: life.Alive,
				Members: map[string]int64{
					"nrpe/0": 1,
				},
			},
		},
	}

	destroyer := mocks.NewMockSubordinateDestroyer(ctrl)
	destroyer.EXPECT().DestroyAllSubordinates().Return(nil)
	relationResolver := relation.NewRelationResolver(r, destroyer)
	_, err = relationResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)

	// Check that we've made the destroy unit call.
	assertNumCalls(c, &numCalls, callsAfterDestroy)
}

type relationCreatedResolverSuite struct{}

func (s *relationCreatedResolverSuite) TestCreatedRelationResolverForRelationInScope(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	r := mocks.NewMockRelationStateTracker(ctrl)

	localState := resolver.LocalState{
		State: operation.State{
			// relation-created hooks can only fire after the charm is installed
			Installed: true,
			Kind:      operation.Continue,
		},
	}

	remoteState := remotestate.Snapshot{
		Life: life.Alive,
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life:      life.Alive,
				Suspended: false,
				Members: map[string]int64{
					"wordpress/0": 1,
				},
				ApplicationMembers: map[string]int64{
					"wordpress": 0,
				},
			},
		},
	}

	gomock.InOrder(
		r.EXPECT().SynchronizeScopes(remoteState).Return(nil),
		r.EXPECT().IsImplicit(1).Return(false, nil),
		// Since the relation was already in scope when the state tracker
		// was initialized, RelationCreated will return true as we will
		// only enter scope *after* the relation-created hook fires.
		r.EXPECT().RelationCreated(1).Return(true),
	)

	createdRelationsResolver := relation.NewCreatedRelationResolver(r)
	_, err := createdRelationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, gc.Equals, resolver.ErrNoOperation, gc.Commentf("unexpected hook from created relations resolver for already joined relation"))
}

func (s *relationCreatedResolverSuite) TestCreatedRelationResolverFordRelationNotInScope(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	r := mocks.NewMockRelationStateTracker(ctrl)

	localState := resolver.LocalState{
		State: operation.State{
			// relation-created hooks can only fire after the charm is installed
			Installed: true,
			Kind:      operation.Continue,
		},
	}

	remoteState := remotestate.Snapshot{
		Life: life.Alive,
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life:      life.Alive,
				Suspended: false,
				Members: map[string]int64{
					"wordpress/0": 1,
				},
				ApplicationMembers: map[string]int64{
					"wordpress": 0,
				},
			},
		},
	}

	gomock.InOrder(
		r.EXPECT().SynchronizeScopes(remoteState).Return(nil),
		r.EXPECT().IsImplicit(1).Return(false, nil),
		// Since the relation is not in scope, RelationCreated will
		// return false
		r.EXPECT().RelationCreated(1).Return(false),
		r.EXPECT().RemoteApplication(1).Return("mysql"),
	)

	createdRelationsResolver := relation.NewCreatedRelationResolver(r)
	op, err := createdRelationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op, gc.DeepEquals, &mockOperation{
		hookInfo: hook.Info{
			Kind:              hooks.RelationCreated,
			RelationId:        1,
			RemoteApplication: "mysql",
		},
	})
}

type mockRelationResolverSuite struct {
	mockRelStTracker *mocks.MockRelationStateTracker
	mockSupDestroyer *mocks.MockSubordinateDestroyer
}

func (s *mockRelationResolverSuite) TestNextOpNothing(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSyncScopesEmpty()

	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{}

	relationsResolver := relation.NewRelationResolver(s.mockRelStTracker, s.mockSupDestroyer)
	_, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(errors.Cause(err), gc.Equals, resolver.ErrNoOperation)
}

func (s *mockRelationResolverSuite) TestHookRelationJoined(c *gc.C) {
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life:      life.Alive,
				Suspended: false,
				Members: map[string]int64{
					"wordpress/0": 1,
				},
				ApplicationMembers: map[string]int64{
					"wordpress": 0,
				},
			},
		},
	}

	defer s.setupMocks(c).Finish()
	s.expectSyncScopes(remoteState)
	s.expectIsKnown(1)
	s.expectIsImplicitFalse(1)
	s.expectStateUnknown(1)
	s.expectIsPeerRelationFalse(1)

	relationsResolver := relation.NewRelationResolver(s.mockRelStTracker, s.mockSupDestroyer)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run hook relation-joined on unit wordpress/0 with relation 1")
}

func (s *mockRelationResolverSuite) TestHookRelationChangedApplication(c *gc.C) {
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life:      life.Alive,
				Suspended: false,
				Members: map[string]int64{
					"wordpress/0": 1,
				},
				ApplicationMembers: map[string]int64{
					"wordpress": 1,
				},
			},
		},
	}
	relationState := relation.State{
		RelationId: 1,
		Members: map[string]int64{
			"wordpress/0": 0,
		},
		ApplicationMembers: map[string]int64{
			"wordpress": 0,
		},
		ChangedPending: "",
	}
	defer s.setupMocks(c).Finish()
	s.expectSyncScopes(remoteState)
	s.expectIsKnown(1)
	s.expectIsImplicitFalse(1)
	s.expectState(relationState)
	s.expectIsPeerRelationFalse(1)

	relationsResolver := relation.NewRelationResolver(s.mockRelStTracker, s.mockSupDestroyer)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run hook relation-changed on app wordpress with relation 1")
}

func (s *mockRelationResolverSuite) TestHookRelationChangedSuspended(c *gc.C) {
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life:      life.Alive,
				Suspended: true,
			},
		},
	}
	relationState := relation.State{
		RelationId: 1,
		Members: map[string]int64{
			"wordpress/0": 0,
		},
		ApplicationMembers: map[string]int64{
			"wordpress": 0,
		},
		ChangedPending: "",
	}
	defer s.setupMocks(c).Finish()
	s.expectSyncScopes(remoteState)
	s.expectIsKnown(1)
	s.expectIsImplicitFalse(1)
	s.expectState(relationState)
	s.expectLocalUnitAndApplicationLife()

	relationsResolver := relation.NewRelationResolver(s.mockRelStTracker, s.mockSupDestroyer)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run hook relation-departed on unit wordpress/0 with relation 1")
}

func (s *mockRelationResolverSuite) TestHookRelationDeparted(c *gc.C) {
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life:      life.Alive,
				Suspended: true,
			},
		},
	}
	relationState := relation.State{
		RelationId: 1,
		Members: map[string]int64{
			"wordpress/0": 0,
		},
		ApplicationMembers: map[string]int64{
			"wordpress": 0,
		},
		ChangedPending: "",
	}
	defer s.setupMocks(c).Finish()
	s.expectSyncScopes(remoteState)
	s.expectIsKnown(1)
	s.expectIsImplicitFalse(1)
	s.expectState(relationState)
	s.expectLocalUnitAndApplicationLife()

	relationsResolver := relation.NewRelationResolver(s.mockRelStTracker, s.mockSupDestroyer)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run hook relation-departed on unit wordpress/0 with relation 1")
}

func (s *mockRelationResolverSuite) TestHookRelationBroken(c *gc.C) {
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life: life.Dying,
			},
		},
	}
	relationState := relation.State{
		RelationId:         1,
		Members:            map[string]int64{},
		ApplicationMembers: map[string]int64{},
		ChangedPending:     "",
	}
	defer s.setupMocks(c).Finish()
	s.expectSyncScopes(remoteState)
	s.expectIsKnown(1)
	s.expectIsImplicitFalse(1)
	s.expectState(relationState)
	s.expectIsPeerRelationFalse(1)
	s.expectStateFound(1)
	s.expectRemoteApplication(1, "")

	relationsResolver := relation.NewRelationResolver(s.mockRelStTracker, s.mockSupDestroyer)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run hook relation-broken with relation 1")
}

func (s *mockRelationResolverSuite) TestHookRelationBrokenWhenSuspended(c *gc.C) {
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life:      life.Alive,
				Suspended: true,
			},
		},
	}
	relationState := relation.State{
		RelationId:         1,
		Members:            map[string]int64{},
		ApplicationMembers: map[string]int64{},
		ChangedPending:     "",
	}
	defer s.setupMocks(c).Finish()
	s.expectSyncScopes(remoteState)
	s.expectIsKnown(1)
	s.expectIsImplicitFalse(1)
	s.expectState(relationState)
	s.expectIsPeerRelationFalse(1)
	s.expectStateFound(1)
	s.expectRemoteApplication(1, "")

	relationsResolver := relation.NewRelationResolver(s.mockRelStTracker, s.mockSupDestroyer)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run hook relation-broken with relation 1")
}

func (s *mockRelationResolverSuite) TestHookRelationBrokenOnlyOnce(c *gc.C) {
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life: life.Dying,
			},
		},
	}
	relationState := relation.State{
		RelationId:         1,
		Members:            map[string]int64{},
		ApplicationMembers: map[string]int64{},
		ChangedPending:     "",
	}
	defer s.setupMocks(c).Finish()
	s.expectSyncScopes(remoteState)
	s.expectIsKnown(1)
	s.expectIsImplicitFalse(1)
	s.expectState(relationState)
	s.expectIsPeerRelationFalse(1)
	s.expectStateFoundFalse(1)

	relationsResolver := relation.NewRelationResolver(s.mockRelStTracker, s.mockSupDestroyer)
	_, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(errors.Cause(err), gc.Equals, resolver.ErrNoOperation)
}

func (s *mockRelationResolverSuite) TestImplicitRelationNoHooks(c *gc.C) {
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life: life.Alive,
				Members: map[string]int64{
					"wordpress": 1,
				},
			},
		},
	}
	defer s.setupMocks(c).Finish()
	s.expectSyncScopes(remoteState)
	s.expectIsKnown(1)
	s.expectIsImplicit(1)

	relationsResolver := relation.NewRelationResolver(s.mockRelStTracker, s.mockSupDestroyer)
	_, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(errors.Cause(err), gc.Equals, resolver.ErrNoOperation)
}

func (s *mockRelationResolverSuite) TestPrincipalDyingDestroysSubordinates(c *gc.C) {
	// So now we have a relation between a principal (wordpress) and a
	// subordinate (nrpe). If the wordpress unit is being destroyed,
	// the subordinate must be also queued for destruction.
	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Life: life.Dying,
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life: life.Alive,
				Members: map[string]int64{
					"nrpe/0": 1,
				},
			},
		},
	}
	relationState := relation.State{
		RelationId:         1,
		Members:            map[string]int64{},
		ApplicationMembers: map[string]int64{},
		ChangedPending:     "",
	}
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()
	s.expectSyncScopes(remoteState)
	s.expectIsKnown(1)
	s.expectIsImplicitFalse(1)
	s.expectState(relationState)
	s.expectIsPeerRelationFalse(1)
	s.expectHasContainerScope(1)
	s.expectStateFound(1)
	s.expectRemoteApplication(1, "")
	destroyer := mocks.NewMockSubordinateDestroyer(ctrl)
	destroyer.EXPECT().DestroyAllSubordinates().Return(nil)

	relationsResolver := relation.NewRelationResolver(s.mockRelStTracker, destroyer)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), gc.Equals, "run hook relation-broken with relation 1")
}

func (s *mockRelationResolverSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockRelStTracker = mocks.NewMockRelationStateTracker(ctrl)
	s.mockSupDestroyer = mocks.NewMockSubordinateDestroyer(ctrl)
	return ctrl
}

func (s *mockRelationResolverSuite) expectSyncScopesEmpty() {
	exp := s.mockRelStTracker.EXPECT()
	exp.SynchronizeScopes(remotestate.Snapshot{}).Return(nil)
}

func (s *mockRelationResolverSuite) expectSyncScopes(snapshot remotestate.Snapshot) {
	exp := s.mockRelStTracker.EXPECT()
	exp.SynchronizeScopes(snapshot).Return(nil)
}

func (s *mockRelationResolverSuite) expectIsKnown(id int) {
	exp := s.mockRelStTracker.EXPECT()
	exp.IsKnown(id).Return(true).AnyTimes()
}

func (s *mockRelationResolverSuite) expectIsImplicit(id int) {
	exp := s.mockRelStTracker.EXPECT()
	exp.IsImplicit(id).Return(true, nil).AnyTimes()
}

func (s *mockRelationResolverSuite) expectIsImplicitFalse(id int) {
	exp := s.mockRelStTracker.EXPECT()
	exp.IsImplicit(id).Return(false, nil).AnyTimes()
}

func (s *mockRelationResolverSuite) expectStateUnknown(id int) {
	exp := s.mockRelStTracker.EXPECT()
	exp.State(id).Return(nil, errors.Errorf("unknown relation: %d", id))
}

func (s *mockRelationResolverSuite) expectState(st relation.State) {
	exp := s.mockRelStTracker.EXPECT()
	exp.State(st.RelationId).Return(&st, nil)
}

func (s *mockRelationResolverSuite) expectStateMaybe(st relation.State) {
	exp := s.mockRelStTracker.EXPECT()
	exp.State(st.RelationId).Return(&st, nil).AnyTimes()
}

func (s *mockRelationResolverSuite) expectIsPeerRelationFalse(id int) {
	exp := s.mockRelStTracker.EXPECT()
	exp.IsPeerRelation(id).Return(false, nil)
}

func (s *mockRelationResolverSuite) expectLocalUnitAndApplicationLife() {
	exp := s.mockRelStTracker.EXPECT()
	exp.LocalUnitAndApplicationLife().Return(life.Alive, life.Alive, nil)
}

func (s *mockRelationResolverSuite) expectStateFound(id int) {
	exp := s.mockRelStTracker.EXPECT()
	exp.StateFound(id).Return(true)
}

func (s *mockRelationResolverSuite) expectStateFoundFalse(id int) {
	exp := s.mockRelStTracker.EXPECT()
	exp.StateFound(id).Return(false)
}

func (s *mockRelationResolverSuite) expectRemoteApplication(id int, app string) {
	exp := s.mockRelStTracker.EXPECT()
	exp.RemoteApplication(id).Return(app)
}

func (s *mockRelationResolverSuite) expectHasContainerScope(id int) {
	exp := s.mockRelStTracker.EXPECT()
	exp.HasContainerScope(id).Return(true, nil)
}
