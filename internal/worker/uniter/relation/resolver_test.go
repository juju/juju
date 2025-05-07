// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	stdcontext "context"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	apiuniter "github.com/juju/juju/api/agent/uniter"
	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/life"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/hooks"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
	uniterapi "github.com/juju/juju/internal/worker/uniter/api"
	"github.com/juju/juju/internal/worker/uniter/hook"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/relation"
	"github.com/juju/juju/internal/worker/uniter/relation/mocks"
	"github.com/juju/juju/internal/worker/uniter/remotestate"
	"github.com/juju/juju/internal/worker/uniter/resolver"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/rpc/params"
)

type relationResolverSuite struct {
	coretesting.BaseSuite

	charmDir          string
	leadershipContext context.LeadershipContext
}

var (
	_ = tc.Suite(&relationResolverSuite{})
	_ = tc.Suite(&relationCreatedResolverSuite{})
	_ = tc.Suite(&mockRelationResolverSuite{})
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

func mockAPICaller(c *tc.C, callNumber *int32, apiCalls ...apiCall) apitesting.APICallerFunc {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		switch objType {
		case "NotifyWatcher":
			return nil
		case "Uniter":
			index := int(atomic.AddInt32(callNumber, 1)) - 1
			c.Check(index <= len(apiCalls), jc.IsTrue, tc.Commentf("index = %d; len(apiCalls) = %d", index, len(apiCalls)))
			call := apiCalls[index]
			c.Logf("request %d, %s", index, request)
			c.Check(version, tc.Equals, 0)
			c.Check(id, tc.Equals, "")
			c.Check(request, tc.Equals, call.request)
			c.Check(arg, jc.DeepEquals, call.args)
			if call.err != nil {
				return apiservererrors.ServerError(call.err)
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

func (s *relationResolverSuite) SetUpTest(c *tc.C) {
	s.charmDir = filepath.Join(c.MkDir(), "charm")
	err := os.MkdirAll(s.charmDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(s.charmDir, "metadata.yaml"), []byte(minimalMetadata), 0755)
	c.Assert(err, jc.ErrorIsNil)
	s.leadershipContext = &stubLeadershipContext{isLeader: true}
}

func assertNumCalls(c *tc.C, numCalls *int32, expected int32) {
	v := atomic.LoadInt32(numCalls)
	c.Assert(v, tc.Equals, expected)
}

func (s *relationResolverSuite) newRelationStateTracker(c *tc.C, apiCaller base.APICaller, unitTag names.UnitTag) relation.RelationStateTracker {
	abort := make(chan struct{})
	client := apiuniter.NewClient(apiCaller, unitTag)
	u, err := client.Unit(stdcontext.Background(), unitTag)
	c.Assert(err, jc.ErrorIsNil)
	r, err := relation.NewRelationStateTracker(stdcontext.Background(),
		relation.RelationStateTrackerConfig{
			Client:            uniterapi.UniterClientShim{Client: client},
			Unit:              uniterapi.UnitShim{Unit: u},
			Logger:            loggertesting.WrapCheckLog(c),
			CharmDir:          s.charmDir,
			LeadershipContext: s.leadershipContext,
			Abort:             abort,
		})
	c.Assert(err, jc.ErrorIsNil)
	return r
}

func (s *relationResolverSuite) setupRelations(c *tc.C) relation.RelationStateTracker {
	unitTag := names.NewUnitTag("wordpress/0")

	var numCalls int32
	unitEntity := params.Entities{Entities: []params.Entity{{Tag: "unit-wordpress-0"}}}
	unitStateResults := params.UnitStateResults{Results: []params.UnitStateResult{{}}}
	apiCaller := mockAPICaller(c, &numCalls,
		uniterAPICall("Refresh", unitEntity, params.UnitRefreshResults{Results: []params.UnitRefreshResult{{Life: life.Alive, Resolved: params.ResolvedNone}}}, nil),
		uniterAPICall("GetPrincipal", unitEntity, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "", Ok: false}}}, nil),
		uniterAPICall("State", unitEntity, unitStateResults, nil),
		uniterAPICall("RelationsStatus", unitEntity, params.RelationUnitStatusResults{Results: []params.RelationUnitStatusResult{{RelationResults: []params.RelationUnitStatus{}}}}, nil),
	)
	r := s.newRelationStateTracker(c, apiCaller, unitTag)
	assertNumCalls(c, &numCalls, 4)
	return r
}

func (s *relationResolverSuite) TestNewRelationsNoRelations(c *tc.C) {
	r := s.setupRelations(c)
	//No relations created.
	c.Assert(r.GetInfo(), tc.HasLen, 0)
}

func (s *relationResolverSuite) assertNewRelationsWithExistingRelations(c *tc.C, isLeader bool) {
	unitTag := names.NewUnitTag("wordpress/0")
	s.leadershipContext = &stubLeadershipContext{isLeader: isLeader}

	var numCalls int32
	unitEntitySingleton := params.Entities{Entities: []params.Entity{{Tag: "unit-wordpress-0"}}}
	unitEntity := params.Entity{Tag: "unit-wordpress-0"}
	relationUnits := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-wordpress.db#mysql.db", Unit: "unit-wordpress-0"},
	}}
	relationResults := params.RelationResultsV2{
		Results: []params.RelationResultV2{
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
		uniterAPICall("Refresh", unitEntitySingleton, params.UnitRefreshResults{Results: []params.UnitRefreshResult{{Life: life.Alive, Resolved: params.ResolvedNone}}}, nil),
		uniterAPICall("GetPrincipal", unitEntitySingleton, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "", Ok: false}}}, nil),
		uniterAPICall("State", unitEntitySingleton, unitStateResults, nil),
		uniterAPICall("RelationsStatus", unitEntitySingleton, params.RelationUnitStatusResults{Results: []params.RelationUnitStatusResult{
			{RelationResults: []params.RelationUnitStatus{{RelationTag: "relation-wordpress:db mysql:db", InScope: true}}}}}, nil),
		uniterAPICall("Relation", relationUnits, relationResults, nil),
		uniterAPICall("Relation", relationUnits, relationResults, nil),
		uniterAPICall("WatchUnit", unitEntity, params.NotifyWatchResult{NotifyWatcherId: "1"}, nil),
		uniterAPICall("SetState", unitSetStateArgs, noErrorResult, nil),
		uniterAPICall("EnterScope", relationUnits, params.ErrorResults{Results: []params.ErrorResult{{}}}, nil),
	}
	if isLeader {
		apiCalls = append(apiCalls,
			uniterAPICall("SetRelationStatus", relationStatus, noErrorResult, nil),
		)
	}
	apiCaller := mockAPICaller(c, &numCalls, apiCalls...)
	r := s.newRelationStateTracker(c, apiCaller, unitTag)
	assertNumCalls(c, &numCalls, int32(len(apiCalls)))

	info := r.GetInfo()
	c.Assert(info, tc.HasLen, 1)
	oneInfo := info[1]
	c.Assert(oneInfo.RelationUnit.Relation().Tag(), tc.Equals, names.NewRelationTag("wordpress:db mysql:db"))
	c.Assert(oneInfo.RelationUnit.Endpoint(), jc.DeepEquals, apiuniter.Endpoint{
		Relation: charm.Relation{Name: "mysql", Role: "provider", Interface: "db", Optional: false, Limit: 0, Scope: ""},
	})
	c.Assert(oneInfo.MemberNames, tc.HasLen, 0)
}

func (s *relationResolverSuite) TestNewRelationsWithExistingRelationsLeader(c *tc.C) {
	s.assertNewRelationsWithExistingRelations(c, true)
}

func (s *relationResolverSuite) TestNewRelationsWithExistingRelationsNotLeader(c *tc.C) {
	s.assertNewRelationsWithExistingRelations(c, false)
}

func (s *relationResolverSuite) newRelationResolver(c *tc.C, stateTracker relation.RelationStateTracker, subordinateDestroyer relation.SubordinateDestroyer) resolver.Resolver {
	return relation.NewRelationResolver(stateTracker, subordinateDestroyer, loggertesting.WrapCheckLog(c))
}

func (s *relationResolverSuite) TestNextOpNothing(c *tc.C) {
	unitTag := names.NewUnitTag("wordpress/0")

	var numCalls int32
	unitEntity := params.Entities{Entities: []params.Entity{{Tag: "unit-wordpress-0"}}}
	unitStateResults := params.UnitStateResults{Results: []params.UnitStateResult{{}}}
	apiCaller := mockAPICaller(c, &numCalls,
		uniterAPICall("Refresh", unitEntity, params.UnitRefreshResults{Results: []params.UnitRefreshResult{{Life: life.Alive, Resolved: params.ResolvedNone}}}, nil),
		uniterAPICall("GetPrincipal", unitEntity, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "", Ok: false}}}, nil),
		uniterAPICall("State", unitEntity, unitStateResults, nil),
		uniterAPICall("RelationsStatus", unitEntity, params.RelationUnitStatusResults{Results: []params.RelationUnitStatusResult{{RelationResults: []params.RelationUnitStatus{}}}}, nil),
	)
	r := s.newRelationStateTracker(c, apiCaller, unitTag)
	assertNumCalls(c, &numCalls, 4)

	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{}
	relationsResolver := s.newRelationResolver(c, r, nil)
	_, err := relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(errors.Cause(err), tc.Equals, resolver.ErrNoOperation)
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
	unitEntitySingleton := params.Entities{Entities: []params.Entity{{Tag: "unit-wordpress-0"}}}
	unitEntity := params.Entity{Tag: "unit-wordpress-0"}
	relationResults := params.RelationResultsV2{
		Results: []params.RelationResultV2{
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
		uniterAPICall("Refresh", unitEntitySingleton, params.UnitRefreshResults{Results: []params.UnitRefreshResult{{Life: life.Alive, Resolved: params.ResolvedNone}}}, nil),
		uniterAPICall("GetPrincipal", unitEntitySingleton, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "", Ok: false}}}, nil),
		uniterAPICall("State", unitEntitySingleton, unitStateResults, nil),
		uniterAPICall("RelationsStatus", unitEntitySingleton, params.RelationUnitStatusResults{Results: []params.RelationUnitStatusResult{{RelationResults: []params.RelationUnitStatus{}}}}, nil),
		uniterAPICall("RelationById", params.RelationIds{RelationIds: []int{1}}, relationResults, nil),
		uniterAPICall("Relation", relationUnits, relationResults, nil),
		//uniterAPICall("State", unitEntitySingleton, unitStateResults, nil),
		uniterAPICall("Relation", relationUnits, relationResults, nil),
		uniterAPICall("WatchUnit", unitEntity, params.NotifyWatchResult{NotifyWatcherId: "1"}, nil),
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

func (s *relationResolverSuite) assertHookRelationJoined(c *tc.C, numCalls *int32, apiCalls ...apiCall) relation.RelationStateTracker {
	unitTag := names.NewUnitTag("wordpress/0")

	apiCaller := mockAPICaller(c, numCalls, apiCalls...)
	r := s.newRelationStateTracker(c, apiCaller, unitTag)
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
	relationsResolver := s.newRelationResolver(c, r, nil)
	op, err := relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, numCalls, 11)
	c.Assert(op.String(), tc.Equals, "run hook relation-joined on unit wordpress/0 with relation 1")

	_, err = r.PrepareHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CommitHook(stdcontext.Background(), op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	return r
}

func (s *relationResolverSuite) TestHookRelationJoined(c *tc.C) {
	var numCalls int32
	s.assertHookRelationJoined(c, &numCalls, relationJoinedAPICalls()...)
}

func (s *relationResolverSuite) assertHookRelationChanged(
	c *tc.C, r relation.RelationStateTracker,
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
	relationsResolver := s.newRelationResolver(c, r, nil)
	op, err := relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, numCalls, numCallsBefore)
	c.Assert(op.String(), tc.Equals, "run hook relation-changed on unit wordpress/0 with relation 1")

	// Commit the operation so we save local state for any next operation.
	_, err = r.PrepareHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CommitHook(stdcontext.Background(), op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationResolverSuite) TestHookRelationChanged(c *tc.C) {
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

func (s *relationResolverSuite) TestHookRelationChangedApplication(c *tc.C) {
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
	relationsResolver := s.newRelationResolver(c, r, nil)
	op, err := relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	// No new calls
	assertNumCalls(c, &numCalls, numCallsBefore)
	c.Assert(op.String(), tc.Equals, "run hook relation-changed on app wordpress with relation 1")
}

func (s *relationResolverSuite) TestHookRelationChangedSuspended(c *tc.C) {
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

	relationsResolver := s.newRelationResolver(c, r, nil)
	op, err := relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, &numCalls, numCallsBefore+2) // Refresh/Life calls made by the resolver prior to emitting a RelationDeparted hook
	c.Assert(op.String(), tc.Equals, "run hook relation-departed on unit wordpress/0 with relation 1")
}

func (s *relationResolverSuite) assertHookRelationDeparted(c *tc.C, numCalls *int32, apiCalls ...apiCall) relation.RelationStateTracker {
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
	relationsResolver := s.newRelationResolver(c, r, nil)
	op, err := relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, numCalls, numCallsBefore+2) // Refresh/Life calls made by the resolver prior to emitting a RelationDeparted hook
	c.Assert(op.String(), tc.Equals, "run hook relation-departed on unit wordpress/0 with relation 1")

	// Commit the operation so we save local state for any next operation.
	_, err = r.PrepareHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CommitHook(stdcontext.Background(), op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	return r
}

func (s *relationResolverSuite) TestHookRelationDeparted(c *tc.C) {
	var numCalls int32
	apiCalls := relationJoinedAndDepartedAPICalls()

	s.assertHookRelationDeparted(c, &numCalls, apiCalls...)
}

func (s *relationResolverSuite) TestHookRelationBroken(c *tc.C) {
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
	relationsResolver := s.newRelationResolver(c, r, nil)
	op, err := relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, &numCalls, 16)
	c.Assert(op.String(), tc.Equals, "run hook relation-broken with relation 1")
}

func (s *relationResolverSuite) TestHookRelationBrokenWhenSuspended(c *tc.C) {
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
	relationsResolver := s.newRelationResolver(c, r, nil)
	op, err := relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, &numCalls, 16)
	c.Assert(op.String(), tc.Equals, "run hook relation-broken with relation 1")
}

func (s *relationResolverSuite) TestHookRelationBrokenOnlyOnce(c *tc.C) {
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
	relationsResolver := s.newRelationResolver(c, r, nil)
	op, err := relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)

	// Commit the RelationBroken, so the NextOp will do the correct thing.
	mockOp, ok := op.(*mockOperation)
	c.Assert(ok, jc.IsTrue)
	c.Assert(mockOp.hookInfo.Kind, tc.Equals, hooks.RelationBroken)
	err = r.CommitHook(stdcontext.Background(), mockOp.hookInfo)
	c.Assert(err, jc.ErrorIsNil)

	_, err = relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(errors.Cause(err), tc.Equals, resolver.ErrNoOperation)
}

func (s *relationResolverSuite) TestCommitHook(c *tc.C) {
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

	err := r.CommitHook(stdcontext.Background(), hook.Info{
		Kind:              hooks.RelationChanged,
		RemoteUnit:        "wordpress/0",
		RemoteApplication: "wordpress",
		RelationId:        1,
		ChangeVersion:     2,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = r.CommitHook(stdcontext.Background(), hook.Info{
		Kind:              hooks.RelationDeparted,
		RemoteUnit:        "wordpress/0",
		RemoteApplication: "wordpress",
		RelationId:        1,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationResolverSuite) TestImplicitRelationNoHooks(c *tc.C) {
	unitTag := names.NewUnitTag("wordpress/0")

	unitEntitySingleton := params.Entities{Entities: []params.Entity{{Tag: "unit-wordpress-0"}}}
	unitEntity := params.Entity{Tag: "unit-wordpress-0"}

	relationResults := params.RelationResultsV2{
		Results: []params.RelationResultV2{
			{
				Id:   1,
				Key:  "wordpress:juju-info juju-info:juju-info",
				Life: life.Alive,
				Endpoint: params.Endpoint{
					ApplicationName: "wordpress",
					Relation:        params.CharmRelation{Name: corerelation.JujuInfo, Role: string(charm.RoleProvider), Interface: corerelation.JujuInfo, Scope: "global"},
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
		uniterAPICall("Refresh", unitEntitySingleton, params.UnitRefreshResults{Results: []params.UnitRefreshResult{{Life: life.Alive, Resolved: params.ResolvedNone}}}, nil),
		uniterAPICall("GetPrincipal", unitEntitySingleton, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "", Ok: false}}}, nil),
		uniterAPICall("State", unitEntitySingleton, unitStateResults, nil),
		uniterAPICall("RelationsStatus", unitEntitySingleton, params.RelationUnitStatusResults{Results: []params.RelationUnitStatusResult{{RelationResults: []params.RelationUnitStatus{}}}}, nil),
		uniterAPICall("RelationById", params.RelationIds{RelationIds: []int{1}}, relationResults, nil),
		uniterAPICall("Relation", relationUnits, relationResults, nil),
		uniterAPICall("Relation", relationUnits, relationResults, nil),
		uniterAPICall("WatchUnit", unitEntity, params.NotifyWatchResult{NotifyWatcherId: "1"}, nil),
		uniterAPICall("SetState", unitSetStateArgs, noErrorResult, nil),
		uniterAPICall("EnterScope", relationUnits, params.ErrorResults{Results: []params.ErrorResult{{}}}, nil),
		uniterAPICall("SetRelationStatus", relationStatus, noErrorResult, nil),
	}

	var numCalls int32
	apiCaller := mockAPICaller(c, &numCalls, apiCalls...)
	r := s.newRelationStateTracker(c, apiCaller, unitTag)

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
	relationsResolver := s.newRelationResolver(c, r, nil)
	_, err := relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(errors.Cause(err), tc.Equals, resolver.ErrNoOperation)
}

var (
	noErrorResult           = params.ErrorResults{Results: []params.ErrorResult{{}}}
	nrpeUnitTag             = names.NewUnitTag("nrpe/0")
	nrpeUnitEntitySingleton = params.Entities{Entities: []params.Entity{{Tag: nrpeUnitTag.String()}}}
	nrpeUnitEntity          = params.Entity{Tag: nrpeUnitTag.String()}
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
	relationResults1 := params.RelationResultsV2{
		Results: []params.RelationResultV2{{
			Id:   1,
			Key:  "wordpress:juju-info nrpe:general-info",
			Life: life.Alive,
			OtherApplication: params.RelatedApplicationDetails{
				ModelUUID:       coretesting.ModelTag.Id(),
				ApplicationName: "wordpress",
			},
			Endpoint: params.Endpoint{
				ApplicationName: "nrpe",
				Relation: params.CharmRelation{
					Name:      "general-info",
					Role:      string(charm.RoleRequirer),
					Interface: corerelation.JujuInfo,
					Scope:     "container",
				},
			},
		}},
	}
	relationUnits2 := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-ntp.nrpe-external-master#nrpe.external-master", Unit: "unit-nrpe-0"},
	}}
	relationResults2 := params.RelationResultsV2{
		Results: []params.RelationResultV2{{
			Id:   2,
			Key:  "ntp:nrpe-external-master nrpe:external-master",
			Life: life.Alive,
			OtherApplication: params.RelatedApplicationDetails{
				ModelUUID:       coretesting.ModelTag.Id(),
				ApplicationName: "ntp",
			},
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
		uniterAPICall("Refresh", nrpeUnitEntitySingleton, params.UnitRefreshResults{Results: []params.UnitRefreshResult{{Life: life.Alive, Resolved: params.ResolvedNone}}}, nil),
		uniterAPICall("GetPrincipal", nrpeUnitEntitySingleton, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "unit-wordpress-0", Ok: true}}}, nil),
		uniterAPICall("State", nrpeUnitEntitySingleton, unitStateResults, nil),
		uniterAPICall("RelationsStatus", nrpeUnitEntitySingleton, relationStatusResults, nil),
		uniterAPICall("Relation", relationUnits1, relationResults1, nil),
		uniterAPICall("Relation", relationUnits2, relationResults2, nil),
		uniterAPICall("Relation", relationUnits1, relationResults1, nil),
		uniterAPICall("WatchUnit", nrpeUnitEntity, params.NotifyWatchResult{NotifyWatcherId: "1"}, nil),
		uniterAPICall("SetState", unitSetStateArgs1, noErrorResult, nil),
		uniterAPICall("EnterScope", relationUnits1, noErrorResult, nil),
		uniterAPICall("SetRelationStatus", relationStatus1, noErrorResult, nil),
		uniterAPICall("Relation", relationUnits2, relationResults2, nil),
		uniterAPICall("WatchUnit", nrpeUnitEntity, params.NotifyWatchResult{NotifyWatcherId: "2"}, nil),
		uniterAPICall("SetState", unitSetStateArgs2, noErrorResult, nil),
		uniterAPICall("EnterScope", relationUnits2, noErrorResult, nil),
		uniterAPICall("SetRelationStatus", relationStatus2, noErrorResult, nil),
	}
}

func (s *relationResolverSuite) TestSubSubPrincipalRelationDyingDestroysUnit(c *tc.C) {
	// When two subordinate units are related on a principal unit's
	// machine, the sub-sub relation shouldn't keep them alive if the
	// relation to the principal dies.
	var numCalls int32
	apiCalls := subSubRelationAPICalls()
	callsBeforeDestroy := int32(len(apiCalls))

	// This should only be called once the relation to the
	// principal app is destroyed.
	apiCalls = append(apiCalls, uniterAPICall("Destroy", nrpeUnitEntitySingleton, noErrorResult, nil))
	//unitStateResults := params.UnitStateResults{Results: []params.UnitStateResult{{
	//	RelationState: map[int]string{2: "id: 2\n"},
	//}}}
	//apiCalls = append(apiCalls, uniterAPICall("State", nrpeUnitEntitySingleton, unitStateResults, nil))
	apiCaller := mockAPICaller(c, &numCalls, apiCalls...)

	r := s.newRelationStateTracker(c, apiCaller, nrpeUnitTag)
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

	relationResolver := s.newRelationResolver(c, r, nil)
	_, err := relationResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)

	// Check that we've made the destroy unit call.
	//
	// TODO: Fix this test...
	// This test intermittently makes either 17 or 18
	// calls.  Number 17 is destroy, so ensure we've
	// called at least that.
	c.Assert(atomic.LoadInt32(&numCalls), jc.GreaterThan, 16)
}

func (s *relationResolverSuite) TestSubSubOtherRelationDyingNotDestroyed(c *tc.C) {
	var numCalls int32
	apiCalls := subSubRelationAPICalls()
	// Sanity check: there shouldn't be a destroy at the end.
	c.Assert(apiCalls[len(apiCalls)-1].request, tc.Not(tc.Equals), "Destroy")

	//unitStateResults := params.UnitStateResults{Results: []params.UnitStateResult{{
	//	RelationState: map[int]string{2: "id: 2\n"},
	//}}}
	//apiCalls = append(apiCalls, uniterAPICall("State", nrpeUnitEntitySingleton, unitStateResults, nil))

	apiCaller := mockAPICaller(c, &numCalls, apiCalls...)

	r := s.newRelationStateTracker(c, apiCaller, nrpeUnitTag)

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

	relationResolver := s.newRelationResolver(c, r, nil)
	_, err := relationResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
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
	relationResults1 := params.RelationResultsV2{
		Results: []params.RelationResultV2{{
			Id:   1,
			Key:  "wordpress:juju-info nrpe:general-info",
			Life: life.Alive,
			OtherApplication: params.RelatedApplicationDetails{
				ModelUUID:       coretesting.ModelTag.Id(),
				ApplicationName: "wordpress",
			},
			Endpoint: params.Endpoint{
				ApplicationName: "nrpe",
				Relation: params.CharmRelation{
					Name:      "general-info",
					Role:      string(charm.RoleRequirer),
					Interface: corerelation.JujuInfo,
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
		uniterAPICall("Refresh", nrpeUnitEntitySingleton, params.UnitRefreshResults{Results: []params.UnitRefreshResult{{Life: life.Alive, Resolved: params.ResolvedNone}}}, nil),
		uniterAPICall("GetPrincipal", nrpeUnitEntitySingleton, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "unit-wordpress-0", Ok: true}}}, nil),
		uniterAPICall("State", nrpeUnitEntitySingleton, unitStateResults, nil),
		uniterAPICall("RelationsStatus", nrpeUnitEntitySingleton, relationStatusResults, nil),
		uniterAPICall("Relation", relationUnits1, relationResults1, nil),
		uniterAPICall("Relation", relationUnits1, relationResults1, nil),
		uniterAPICall("WatchUnit", nrpeUnitEntity, params.NotifyWatchResult{NotifyWatcherId: "1"}, nil),
		uniterAPICall("SetState", unitSetStateArgs, noErrorResult, nil),
		uniterAPICall("EnterScope", relationUnits1, noErrorResult, nil),
		uniterAPICall("SetRelationStatus", relationStatus1, noErrorResult, nil),
	}
}

func (s *relationResolverSuite) TestPrincipalDyingDestroysSubordinates(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var numCalls int32
	apiCalls := principalWithSubordinateAPICalls()
	callsBeforeDestroy := int32(len(apiCalls))
	callsAfterDestroy := callsBeforeDestroy + 1
	// This should only be called after we queue the subordinate for destruction
	apiCalls = append(apiCalls, uniterAPICall("Destroy", nrpeUnitEntitySingleton, noErrorResult, nil))
	//unitStateResults := params.UnitStateResults{Results: []params.UnitStateResult{{
	//	RelationState: map[int]string{1: "id: 1\n", 73: ""},
	//}}}
	//apiCalls = append(apiCalls, uniterAPICall("State", nrpeUnitEntitySingleton, unitStateResults, nil))
	apiCaller := mockAPICaller(c, &numCalls, apiCalls...)

	r := s.newRelationStateTracker(c, apiCaller, nrpeUnitTag)
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
	destroyer.EXPECT().DestroyAllSubordinates(gomock.Any()).Return(nil)
	relationResolver := s.newRelationResolver(c, r, destroyer)
	_, err := relationResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)

	// Check that we've made the destroy unit call.
	assertNumCalls(c, &numCalls, callsAfterDestroy)
}

type relationCreatedResolverSuite struct{}

func (s *relationCreatedResolverSuite) TestCreatedRelationResolverForRelationInScope(c *tc.C) {
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
		r.EXPECT().SynchronizeScopes(gomock.Any(), remoteState).Return(nil),
		r.EXPECT().IsImplicit(1).Return(false, nil),
		// Since the relation was already in scope when the state tracker
		// was initialized, RelationCreated will return true as we will
		// only enter scope *after* the relation-created hook fires.
		r.EXPECT().RelationCreated(1).Return(true),
	)

	createdRelationsResolver := relation.NewCreatedRelationResolver(r, loggertesting.WrapCheckLog(c))
	_, err := createdRelationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, tc.Equals, resolver.ErrNoOperation, tc.Commentf("unexpected hook from created relations resolver for already joined relation"))
}

func (s *relationCreatedResolverSuite) TestCreatedRelationResolverFordRelationNotInScope(c *tc.C) {
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
		r.EXPECT().SynchronizeScopes(stdcontext.Background(), remoteState).Return(nil),
		r.EXPECT().IsImplicit(1).Return(false, nil),
		// Since the relation is not in scope, RelationCreated will
		// return false
		r.EXPECT().RelationCreated(1).Return(false),
		r.EXPECT().RemoteApplication(1).Return("mysql"),
	)

	createdRelationsResolver := relation.NewCreatedRelationResolver(r, loggertesting.WrapCheckLog(c))
	op, err := createdRelationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op, tc.DeepEquals, &mockOperation{
		hookInfo: hook.Info{
			Kind:              hooks.RelationCreated,
			RelationId:        1,
			RemoteApplication: "mysql",
		},
	})
}

// This is a regression test for LP1906706
func (s *relationCreatedResolverSuite) TestCreatedRelationsResolverWithPendingHook(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	r := mocks.NewMockRelationStateTracker(ctrl)

	localState := resolver.LocalState{
		State: operation.State{
			Installed: true,
			Kind:      operation.RunHook,
			Step:      operation.Pending,
		},
	}
	remoteState := remotestate.Snapshot{
		Life: life.Alive,
	}

	createdRelationsResolver := relation.NewCreatedRelationResolver(r, loggertesting.WrapCheckLog(c))
	_, err := createdRelationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(errors.Cause(err), tc.Equals, resolver.ErrNoOperation, tc.Commentf("expected to get ErrNoOperation when a RunHook operation is pending"))
}

type mockRelationResolverSuite struct {
	mockRelStTracker *mocks.MockRelationStateTracker
	mockSupDestroyer *mocks.MockSubordinateDestroyer
}

func (s *mockRelationResolverSuite) newRelationResolver(c *tc.C, stateTracker relation.RelationStateTracker, subordinateDestroyer relation.SubordinateDestroyer) resolver.Resolver {
	return relation.NewRelationResolver(stateTracker, subordinateDestroyer, loggertesting.WrapCheckLog(c))
}

func (s *mockRelationResolverSuite) TestNextOpNothing(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.expectSyncScopesEmpty()

	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{}

	relationsResolver := s.newRelationResolver(c, s.mockRelStTracker, s.mockSupDestroyer)
	_, err := relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(errors.Cause(err), tc.Equals, resolver.ErrNoOperation)
}

func (s *mockRelationResolverSuite) TestHookRelationJoined(c *tc.C) {
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

	s.mockRelStTracker.EXPECT().IsPeerRelation(1).Return(false, nil).Times(2)

	relationsResolver := s.newRelationResolver(c, s.mockRelStTracker, s.mockSupDestroyer)
	op, err := relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run hook relation-joined on unit wordpress/0 with relation 1")
}

func (s *mockRelationResolverSuite) TestHookRelationChangedApplication(c *tc.C) {
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

	s.mockRelStTracker.EXPECT().IsPeerRelation(1).Return(false, nil).Times(2)

	relationsResolver := s.newRelationResolver(c, s.mockRelStTracker, s.mockSupDestroyer)
	op, err := relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run hook relation-changed on app wordpress with relation 1")
}

func (s *mockRelationResolverSuite) TestHookRelationChangedSuspended(c *tc.C) {
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

	s.mockRelStTracker.EXPECT().IsPeerRelation(1).Return(false, nil)

	relationsResolver := s.newRelationResolver(c, s.mockRelStTracker, s.mockSupDestroyer)
	op, err := relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run hook relation-departed on unit wordpress/0 with relation 1")
}

func (s *mockRelationResolverSuite) TestHookRelationDeparted(c *tc.C) {
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

	s.mockRelStTracker.EXPECT().IsPeerRelation(1).Return(false, nil)

	relationsResolver := s.newRelationResolver(c, s.mockRelStTracker, s.mockSupDestroyer)
	op, err := relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run hook relation-departed on unit wordpress/0 with relation 1")
}

func (s *mockRelationResolverSuite) TestHookRelationBroken(c *tc.C) {
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
			2: {
				Life: life.Dying,
			},
		},
	}

	defer s.setupMocks(c).Finish()

	s.expectSyncScopes(remoteState)

	relationState1 := relation.State{
		RelationId:         1,
		Members:            map[string]int64{},
		ApplicationMembers: map[string]int64{},
		ChangedPending:     "",
	}
	s.expectIsKnown(1)
	s.expectIsImplicitFalse(1)
	s.expectState(relationState1)
	s.expectStateFound(1)
	s.expectRemoteApplication(1, "")
	s.mockRelStTracker.EXPECT().IsPeerRelation(1).Return(false, nil).Times(2)

	// The `Relations` map in the remote state snapshot (as with all Go maps)
	// has random iteration order. This will sometimes be called
	// (relation ID 2 first) and sometimes not (ID 1 first). The test here is
	// that in all cases, the next operation is for ID 1 (non-peer) - it is
	// always enqueued ahead of ID 2, which is a peer relation.
	s.mockRelStTracker.EXPECT().IsPeerRelation(2).Return(true, nil).MaxTimes(1)

	relationsResolver := s.newRelationResolver(c, s.mockRelStTracker, s.mockSupDestroyer)
	op, err := relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run hook relation-broken with relation 1")
}

func (s *mockRelationResolverSuite) TestHookRelationBrokenWhenSuspended(c *tc.C) {
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
	s.expectStateFound(1)
	s.expectRemoteApplication(1, "")

	s.mockRelStTracker.EXPECT().IsPeerRelation(1).Return(false, nil).Times(2)

	relationsResolver := s.newRelationResolver(c, s.mockRelStTracker, s.mockSupDestroyer)
	op, err := relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run hook relation-broken with relation 1")
}

func (s *mockRelationResolverSuite) TestHookRelationBrokenOnlyOnce(c *tc.C) {
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
	s.expectStateFoundFalse(1)

	s.mockRelStTracker.EXPECT().IsPeerRelation(1).Return(false, nil).Times(2)

	relationsResolver := s.newRelationResolver(c, s.mockRelStTracker, s.mockSupDestroyer)
	_, err := relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(errors.Cause(err), tc.Equals, resolver.ErrNoOperation)
}

func (s *mockRelationResolverSuite) TestImplicitRelationNoHooks(c *tc.C) {
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

	s.mockRelStTracker.EXPECT().IsPeerRelation(1).Return(false, nil)

	relationsResolver := s.newRelationResolver(c, s.mockRelStTracker, s.mockSupDestroyer)
	_, err := relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(errors.Cause(err), tc.Equals, resolver.ErrNoOperation)
}

func (s *mockRelationResolverSuite) TestPrincipalDyingDestroysSubordinates(c *tc.C) {
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
	s.expectHasContainerScope(1)
	s.expectStateFound(1)
	s.expectRemoteApplication(1, "")
	destroyer := mocks.NewMockSubordinateDestroyer(ctrl)
	destroyer.EXPECT().DestroyAllSubordinates(gomock.Any()).Return(nil)

	s.mockRelStTracker.EXPECT().IsPeerRelation(1).Return(false, nil).Times(2)

	relationsResolver := s.newRelationResolver(c, s.mockRelStTracker, destroyer)
	op, err := relationsResolver.NextOp(stdcontext.Background(), localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.String(), tc.Equals, "run hook relation-broken with relation 1")
}

func (s *mockRelationResolverSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockRelStTracker = mocks.NewMockRelationStateTracker(ctrl)
	s.mockSupDestroyer = mocks.NewMockSubordinateDestroyer(ctrl)
	return ctrl
}

func (s *mockRelationResolverSuite) expectSyncScopesEmpty() {
	exp := s.mockRelStTracker.EXPECT()
	exp.SynchronizeScopes(stdcontext.Background(), remotestate.Snapshot{}).Return(nil)
}

func (s *mockRelationResolverSuite) expectSyncScopes(snapshot remotestate.Snapshot) {
	exp := s.mockRelStTracker.EXPECT()
	exp.SynchronizeScopes(stdcontext.Background(), snapshot).Return(nil)
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
	exp.State(id).Return(nil, errors.NotFoundf("relation: %d", id))
}

func (s *mockRelationResolverSuite) expectState(st relation.State) {
	exp := s.mockRelStTracker.EXPECT()
	exp.State(st.RelationId).Return(&st, nil)
}

func (s *mockRelationResolverSuite) expectLocalUnitAndApplicationLife() {
	exp := s.mockRelStTracker.EXPECT()
	exp.LocalUnitAndApplicationLife(gomock.Any()).Return(life.Alive, life.Alive, nil)
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
