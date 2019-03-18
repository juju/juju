// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charm.v6/hooks"
	"gopkg.in/juju/names.v2"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/state/multiwatcher"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/relation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
	"github.com/juju/juju/worker/uniter/runner/context"
)

/*
TODO(wallyworld)
DO NOT COPY THE METHODOLOGY USED IN THESE TESTS.
We want to write unit tests without resorting to JujuConnSuite.
However, the current api/uniter code uses structs instead of
interfaces for its component model, and it's not possible to
implement a stub uniter api at the model level due to the way
the domain objects reference each other.

The best we can do for now is to stub out the facade caller and
return curated values for each API call.
*/

type relationsSuite struct {
	coretesting.BaseSuite

	stateDir              string
	relationsDir          string
	leadershipContextFunc relation.LeadershipContextFunc
}

var _ = gc.Suite(&relationsSuite{})

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
			c.Check(index < len(apiCalls), jc.IsTrue)
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

func (s *relationsSuite) SetUpTest(c *gc.C) {
	s.stateDir = filepath.Join(c.MkDir(), "charm")
	err := os.MkdirAll(s.stateDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(s.stateDir, "metadata.yaml"), []byte(minimalMetadata), 0755)
	c.Assert(err, jc.ErrorIsNil)
	s.relationsDir = filepath.Join(c.MkDir(), "relations")
	s.leadershipContextFunc = func(accessor context.LeadershipSettingsAccessor, tracker leadership.Tracker, unitName string) context.LeadershipContext {
		return &stubLeadershipContext{isLeader: true}
	}
}

func assertNumCalls(c *gc.C, numCalls *int32, expected int32) {
	v := atomic.LoadInt32(numCalls)
	c.Assert(v, gc.Equals, expected)
}

func (s *relationsSuite) setupRelations(c *gc.C) relation.Relations {
	unitTag := names.NewUnitTag("wordpress/0")
	abort := make(chan struct{})

	var numCalls int32
	unitEntity := params.Entities{Entities: []params.Entity{{Tag: "unit-wordpress-0"}}}
	apiCaller := mockAPICaller(c, &numCalls,
		uniterAPICall("Refresh", unitEntity, params.UnitRefreshResults{Results: []params.UnitRefreshResult{{Life: params.Alive, Resolved: params.ResolvedNone}}}, nil),
		uniterAPICall("GetPrincipal", unitEntity, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "", Ok: false}}}, nil),
		uniterAPICall("RelationsStatus", unitEntity, params.RelationUnitStatusResults{Results: []params.RelationUnitStatusResult{{RelationResults: []params.RelationUnitStatus{}}}}, nil),
	)
	st := uniter.NewState(apiCaller, unitTag)
	r, err := relation.NewRelations(
		relation.RelationsConfig{
			State:                st,
			UnitTag:              unitTag,
			CharmDir:             s.stateDir,
			RelationsDir:         s.relationsDir,
			NewLeadershipContext: s.leadershipContextFunc,
			Abort:                abort,
		})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, &numCalls, 3)
	return r
}

func (s *relationsSuite) TestNewRelationsNoRelations(c *gc.C) {
	r := s.setupRelations(c)
	//No relations created.
	c.Assert(r.GetInfo(), gc.HasLen, 0)
}

func (s *relationsSuite) assertNewRelationsWithExistingRelations(c *gc.C, isLeader bool) {
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
				Life: params.Alive,
				Endpoint: multiwatcher.Endpoint{
					ApplicationName: "wordpress",
					Relation:        multiwatcher.CharmRelation{Name: "mysql", Role: string(charm.RoleProvider), Interface: "db"},
				}},
		},
	}
	relationStatus := params.RelationStatusArgs{Args: []params.RelationStatusArg{{
		UnitTag:    "unit-wordpress-0",
		RelationId: 1,
		Status:     params.Joined,
	}}}

	apiCalls := []apiCall{
		uniterAPICall("Refresh", unitEntity, params.UnitRefreshResults{Results: []params.UnitRefreshResult{{Life: params.Alive, Resolved: params.ResolvedNone}}}, nil),
		uniterAPICall("GetPrincipal", unitEntity, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "", Ok: false}}}, nil),
		uniterAPICall("RelationsStatus", unitEntity, params.RelationUnitStatusResults{Results: []params.RelationUnitStatusResult{
			{RelationResults: []params.RelationUnitStatus{{RelationTag: "relation-wordpress:db mysql:db", InScope: true}}}}}, nil),
		uniterAPICall("Relation", relationUnits, relationResults, nil),
		uniterAPICall("Relation", relationUnits, relationResults, nil),
		uniterAPICall("Watch", unitEntity, params.NotifyWatchResults{Results: []params.NotifyWatchResult{{NotifyWatcherId: "1"}}}, nil),
		uniterAPICall("EnterScope", relationUnits, params.ErrorResults{Results: []params.ErrorResult{{}}}, nil),
	}
	if isLeader {
		apiCalls = append(apiCalls,
			uniterAPICall("SetRelationStatus", relationStatus, noErrorResult, nil),
		)
	}
	apiCaller := mockAPICaller(c, &numCalls, apiCalls...)
	st := uniter.NewState(apiCaller, unitTag)
	r, err := relation.NewRelations(
		relation.RelationsConfig{
			State:                st,
			UnitTag:              unitTag,
			CharmDir:             s.stateDir,
			RelationsDir:         s.relationsDir,
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

func (s *relationsSuite) TestNewRelationsWithExistingRelationsLeader(c *gc.C) {
	s.assertNewRelationsWithExistingRelations(c, true)
}

func (s *relationsSuite) TestNewRelationsWithExistingRelationsNotLeader(c *gc.C) {
	s.assertNewRelationsWithExistingRelations(c, false)
}

func (s *relationsSuite) TestNextOpNothing(c *gc.C) {
	unitTag := names.NewUnitTag("wordpress/0")
	abort := make(chan struct{})

	var numCalls int32
	unitEntity := params.Entities{Entities: []params.Entity{{Tag: "unit-wordpress-0"}}}
	apiCaller := mockAPICaller(c, &numCalls,
		uniterAPICall("Refresh", unitEntity, params.UnitRefreshResults{Results: []params.UnitRefreshResult{{Life: params.Alive, Resolved: params.ResolvedNone}}}, nil),
		uniterAPICall("GetPrincipal", unitEntity, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "", Ok: false}}}, nil),
		uniterAPICall("RelationsStatus", unitEntity, params.RelationUnitStatusResults{Results: []params.RelationUnitStatusResult{{RelationResults: []params.RelationUnitStatus{}}}}, nil),
	)
	st := uniter.NewState(apiCaller, unitTag)
	r, err := relation.NewRelations(
		relation.RelationsConfig{
			State:                st,
			UnitTag:              unitTag,
			CharmDir:             s.stateDir,
			RelationsDir:         s.relationsDir,
			NewLeadershipContext: s.leadershipContextFunc,
			Abort:                abort,
		})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, &numCalls, 3)

	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{}
	relationsResolver := relation.NewRelationsResolver(r)
	_, err = relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(errors.Cause(err), gc.Equals, resolver.ErrNoOperation)
}

func relationJoinedAPICalls() []apiCall {
	unitEntity := params.Entities{Entities: []params.Entity{{Tag: "unit-wordpress-0"}}}
	relationResults := params.RelationResults{
		Results: []params.RelationResult{
			{
				Id:   1,
				Key:  "wordpress:db mysql:db",
				Life: params.Alive,
				Endpoint: multiwatcher.Endpoint{
					ApplicationName: "wordpress",
					Relation:        multiwatcher.CharmRelation{Name: "mysql", Role: string(charm.RoleRequirer), Interface: "db", Scope: "global"},
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
	apiCalls := []apiCall{
		uniterAPICall("Refresh", unitEntity, params.UnitRefreshResults{Results: []params.UnitRefreshResult{{Life: params.Alive, Resolved: params.ResolvedNone}}}, nil),
		uniterAPICall("GetPrincipal", unitEntity, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "", Ok: false}}}, nil),
		uniterAPICall("RelationsStatus", unitEntity, params.RelationUnitStatusResults{Results: []params.RelationUnitStatusResult{{RelationResults: []params.RelationUnitStatus{}}}}, nil),
		uniterAPICall("RelationById", params.RelationIds{RelationIds: []int{1}}, relationResults, nil),
		uniterAPICall("Relation", relationUnits, relationResults, nil),
		uniterAPICall("Relation", relationUnits, relationResults, nil),
		uniterAPICall("Watch", unitEntity, params.NotifyWatchResults{Results: []params.NotifyWatchResult{{NotifyWatcherId: "1"}}}, nil),
		uniterAPICall("EnterScope", relationUnits, params.ErrorResults{Results: []params.ErrorResult{{}}}, nil),
		uniterAPICall("SetRelationStatus", relationStatus, noErrorResult, nil),
	}
	return apiCalls
}

func (s *relationsSuite) assertHookRelationJoined(c *gc.C, numCalls *int32, apiCalls ...apiCall) relation.Relations {
	unitTag := names.NewUnitTag("wordpress/0")
	abort := make(chan struct{})

	apiCaller := mockAPICaller(c, numCalls, apiCalls...)
	st := uniter.NewState(apiCaller, unitTag)
	r, err := relation.NewRelations(
		relation.RelationsConfig{
			State:                st,
			UnitTag:              unitTag,
			CharmDir:             s.stateDir,
			RelationsDir:         s.relationsDir,
			NewLeadershipContext: s.leadershipContextFunc,
			Abort:                abort,
		})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, numCalls, 3)

	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life:      params.Alive,
				Suspended: false,
				Members: map[string]int64{
					"wordpress": 1,
				},
			},
		},
	}
	relationsResolver := relation.NewRelationsResolver(r)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, numCalls, 9)
	c.Assert(op.String(), gc.Equals, "run hook relation-joined on unit with relation 1")

	// Commit the operation so we save local state for any next operation.
	_, err = r.PrepareHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CommitHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	return r
}

func (s *relationsSuite) TestHookRelationJoined(c *gc.C) {
	var numCalls int32
	s.assertHookRelationJoined(c, &numCalls, relationJoinedAPICalls()...)
}

func (s *relationsSuite) assertHookRelationChanged(
	c *gc.C, r relation.Relations,
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
	relationsResolver := relation.NewRelationsResolver(r)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, numCalls, numCallsBefore)
	c.Assert(op.String(), gc.Equals, "run hook relation-changed on unit with relation 1")

	// Commit the operation so we save local state for any next operation.
	_, err = r.PrepareHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CommitHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *relationsSuite) TestHookRelationChanged(c *gc.C) {
	var numCalls int32
	apiCalls := relationJoinedAPICalls()
	r := s.assertHookRelationJoined(c, &numCalls, apiCalls...)

	// There will be an initial relation-changed regardless of
	// members, due to the "changed pending" local persistent
	// state.
	s.assertHookRelationChanged(c, r, remotestate.RelationSnapshot{
		Life:      params.Alive,
		Suspended: false,
	}, &numCalls)

	// wordpress starts at 1, changing to 2 should trigger a
	// relation-changed hook.
	s.assertHookRelationChanged(c, r, remotestate.RelationSnapshot{
		Life:      params.Alive,
		Suspended: false,
		Members: map[string]int64{
			"wordpress": 2,
		},
	}, &numCalls)

	// NOTE(axw) this is a test for the temporary to fix lp:1495542.
	//
	// wordpress is at 2, changing to 1 should trigger a
	// relation-changed hook. This is to cater for the scenario
	// where the relation settings document is removed and
	// recreated, thus resetting the txn-revno.
	s.assertHookRelationChanged(c, r, remotestate.RelationSnapshot{
		Life: params.Alive,
		Members: map[string]int64{
			"wordpress": 1,
		},
	}, &numCalls)
}

func (s *relationsSuite) TestHookRelationChangedSuspended(c *gc.C) {
	var numCalls int32
	apiCalls := relationJoinedAPICalls()
	r := s.assertHookRelationJoined(c, &numCalls, apiCalls...)

	// There will be an initial relation-changed regardless of
	// members, due to the "changed pending" local persistent
	// state.
	s.assertHookRelationChanged(c, r, remotestate.RelationSnapshot{
		Life:      params.Alive,
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
				Life:      params.Alive,
				Suspended: true,
			},
		},
	}

	relationsResolver := relation.NewRelationsResolver(r)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, &numCalls, numCallsBefore)
	c.Assert(op.String(), gc.Equals, "run hook relation-departed on unit with relation 1")
}

func (s *relationsSuite) assertHookRelationDeparted(c *gc.C, numCalls *int32, apiCalls ...apiCall) relation.Relations {
	r := s.assertHookRelationJoined(c, numCalls, apiCalls...)
	s.assertHookRelationChanged(c, r, remotestate.RelationSnapshot{
		Life:      params.Alive,
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
				Life: params.Dying,
				Members: map[string]int64{
					"wordpress": 1,
				},
			},
		},
	}
	relationsResolver := relation.NewRelationsResolver(r)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, numCalls, numCallsBefore)
	c.Assert(op.String(), gc.Equals, "run hook relation-departed on unit with relation 1")

	// Commit the operation so we save local state for any next operation.
	_, err = r.PrepareHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CommitHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	return r
}

func (s *relationsSuite) TestHookRelationDeparted(c *gc.C) {
	var numCalls int32
	apiCalls := relationJoinedAPICalls()

	s.assertHookRelationDeparted(c, &numCalls, apiCalls...)
}

func (s *relationsSuite) TestHookRelationBroken(c *gc.C) {
	var numCalls int32
	apiCalls := relationJoinedAPICalls()

	r := s.assertHookRelationDeparted(c, &numCalls, apiCalls...)

	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life: params.Dying,
			},
		},
	}
	relationsResolver := relation.NewRelationsResolver(r)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, &numCalls, 9)
	c.Assert(op.String(), gc.Equals, "run hook relation-broken on unit with relation 1")
}

func (s *relationsSuite) TestHookRelationBrokenWhenSuspended(c *gc.C) {
	var numCalls int32
	apiCalls := relationJoinedAPICalls()

	r := s.assertHookRelationDeparted(c, &numCalls, apiCalls...)

	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life:      params.Alive,
				Suspended: true,
			},
		},
	}
	relationsResolver := relation.NewRelationsResolver(r)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, &numCalls, 9)
	c.Assert(op.String(), gc.Equals, "run hook relation-broken on unit with relation 1")
}

func (s *relationsSuite) TestHookRelationBrokenOnlyOnce(c *gc.C) {
	var numCalls int32
	apiCalls := relationJoinedAPICalls()
	relationUnits := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-wordpress.db#mysql.db", Unit: "unit-wordpress-0"},
	}}
	apiCalls = append(apiCalls,
		uniterAPICall("LeaveScope", relationUnits, params.ErrorResults{Results: []params.ErrorResult{{}}}, nil),
	)

	r := s.assertHookRelationDeparted(c, &numCalls, apiCalls...)

	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: {
				Life:      params.Alive,
				Suspended: true,
			},
		},
	}
	relationsResolver := relation.NewRelationsResolver(r)

	// Remove the state directory to check that the hook is not run again.
	err := os.RemoveAll(s.relationsDir)
	c.Assert(err, jc.ErrorIsNil)
	_, err = relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(errors.Cause(err), gc.Equals, resolver.ErrNoOperation)
}

func (s *relationsSuite) TestCommitHook(c *gc.C) {
	var numCalls int32
	apiCalls := relationJoinedAPICalls()
	relationUnits := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-wordpress.db#mysql.db", Unit: "unit-wordpress-0"},
	}}
	apiCalls = append(apiCalls,
		uniterAPICall("LeaveScope", relationUnits, params.ErrorResults{Results: []params.ErrorResult{{}}}, nil),
	)
	stateFile := filepath.Join(s.relationsDir, "1", "wordpress")
	c.Assert(stateFile, jc.DoesNotExist)
	r := s.assertHookRelationJoined(c, &numCalls, apiCalls...)

	data, err := ioutil.ReadFile(stateFile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "change-version: 1\nchanged-pending: true\n")

	err = r.CommitHook(hook.Info{
		Kind:          hooks.RelationChanged,
		RemoteUnit:    "wordpress",
		RelationId:    1,
		ChangeVersion: 2,
	})
	c.Assert(err, jc.ErrorIsNil)
	data, err = ioutil.ReadFile(stateFile)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "change-version: 2\n")

	err = r.CommitHook(hook.Info{
		Kind:       hooks.RelationDeparted,
		RemoteUnit: "wordpress",
		RelationId: 1,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(stateFile, jc.DoesNotExist)
}

func (s *relationsSuite) TestImplicitRelationNoHooks(c *gc.C) {
	unitTag := names.NewUnitTag("wordpress/0")
	abort := make(chan struct{})

	unitEntity := params.Entities{Entities: []params.Entity{{Tag: "unit-wordpress-0"}}}
	relationResults := params.RelationResults{
		Results: []params.RelationResult{
			{
				Id:   1,
				Key:  "wordpress:juju-info juju-info:juju-info",
				Life: params.Alive,
				Endpoint: multiwatcher.Endpoint{
					ApplicationName: "wordpress",
					Relation:        multiwatcher.CharmRelation{Name: "juju-info", Role: string(charm.RoleProvider), Interface: "juju-info", Scope: "global"},
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
	apiCalls := []apiCall{
		uniterAPICall("Refresh", unitEntity, params.UnitRefreshResults{Results: []params.UnitRefreshResult{{Life: params.Alive, Resolved: params.ResolvedNone}}}, nil),
		uniterAPICall("GetPrincipal", unitEntity, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "", Ok: false}}}, nil),
		uniterAPICall("RelationsStatus", unitEntity, params.RelationUnitStatusResults{Results: []params.RelationUnitStatusResult{{RelationResults: []params.RelationUnitStatus{}}}}, nil),
		uniterAPICall("RelationById", params.RelationIds{RelationIds: []int{1}}, relationResults, nil),
		uniterAPICall("Relation", relationUnits, relationResults, nil),
		uniterAPICall("Relation", relationUnits, relationResults, nil),
		uniterAPICall("Watch", unitEntity, params.NotifyWatchResults{Results: []params.NotifyWatchResult{{NotifyWatcherId: "1"}}}, nil),
		uniterAPICall("EnterScope", relationUnits, params.ErrorResults{Results: []params.ErrorResult{{}}}, nil),
		uniterAPICall("SetRelationStatus", relationStatus, noErrorResult, nil),
	}

	var numCalls int32
	apiCaller := mockAPICaller(c, &numCalls, apiCalls...)
	st := uniter.NewState(apiCaller, unitTag)
	r, err := relation.NewRelations(
		relation.RelationsConfig{
			State:                st,
			UnitTag:              unitTag,
			CharmDir:             s.stateDir,
			RelationsDir:         s.relationsDir,
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
				Life: params.Alive,
				Members: map[string]int64{
					"wordpress": 1,
				},
			},
		},
	}
	relationsResolver := relation.NewRelationsResolver(r)
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
			Life:             params.Alive,
			OtherApplication: "wordpress",
			Endpoint: multiwatcher.Endpoint{
				ApplicationName: "nrpe",
				Relation: multiwatcher.CharmRelation{
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
			Life:             params.Alive,
			OtherApplication: "ntp",
			Endpoint: multiwatcher.Endpoint{
				ApplicationName: "nrpe",
				Relation: multiwatcher.CharmRelation{
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

	return []apiCall{
		uniterAPICall("Refresh", nrpeUnitEntity, params.UnitRefreshResults{Results: []params.UnitRefreshResult{{Life: params.Alive, Resolved: params.ResolvedNone}}}, nil),
		uniterAPICall("GetPrincipal", nrpeUnitEntity, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "unit-wordpress-0", Ok: true}}}, nil),
		uniterAPICall("RelationsStatus", nrpeUnitEntity, relationStatusResults, nil),
		uniterAPICall("Relation", relationUnits1, relationResults1, nil),
		uniterAPICall("Relation", relationUnits2, relationResults2, nil),
		uniterAPICall("Relation", relationUnits1, relationResults1, nil),
		uniterAPICall("Watch", nrpeUnitEntity, params.NotifyWatchResults{Results: []params.NotifyWatchResult{{NotifyWatcherId: "1"}}}, nil),
		uniterAPICall("EnterScope", relationUnits1, noErrorResult, nil),
		uniterAPICall("SetRelationStatus", relationStatus1, noErrorResult, nil),
		uniterAPICall("Relation", relationUnits2, relationResults2, nil),
		uniterAPICall("Watch", nrpeUnitEntity, params.NotifyWatchResults{Results: []params.NotifyWatchResult{{NotifyWatcherId: "2"}}}, nil),
		uniterAPICall("EnterScope", relationUnits2, noErrorResult, nil),
		uniterAPICall("SetRelationStatus", relationStatus2, noErrorResult, nil),
	}
}

func (s *relationsSuite) TestSubSubPrincipalRelationDyingDestroysUnit(c *gc.C) {
	// When two subordinate units are related on a principal unit's
	// machine, the sub-sub relation shouldn't keep them alive if the
	// relation to the principal dies.
	var numCalls int32
	apiCalls := subSubRelationAPICalls()
	callsBeforeDestroy := int32(len(apiCalls))
	callsAfterDestroy := callsBeforeDestroy + 1
	// This should only be called once the relation to the
	// principal app is destroyed.
	apiCalls = append(apiCalls, uniterAPICall("Destroy", nrpeUnitEntity, noErrorResult, nil))
	apiCaller := mockAPICaller(c, &numCalls, apiCalls...)

	st := uniter.NewState(apiCaller, nrpeUnitTag)
	r, err := relation.NewRelations(
		relation.RelationsConfig{
			State:                st,
			UnitTag:              nrpeUnitTag,
			CharmDir:             s.stateDir,
			RelationsDir:         s.relationsDir,
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
				Life: params.Dying,
				Members: map[string]int64{
					"wordpress/0": 1,
				},
			},
			2: {
				Life: params.Alive,
				Members: map[string]int64{
					"ntp/0": 1,
				},
			},
		},
	}

	rr := relation.NewRelationsResolver(r)
	_, err = rr.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)

	// Check that we've made the destroy unit call.
	assertNumCalls(c, &numCalls, callsAfterDestroy)
}

func (s *relationsSuite) TestSubSubOtherRelationDyingNotDestroyed(c *gc.C) {
	var numCalls int32
	apiCalls := subSubRelationAPICalls()
	// Sanity check: there shouldn't be a destroy at the end.
	c.Assert(apiCalls[len(apiCalls)-1].request, gc.Not(gc.Equals), "Destroy")

	expectedCalls := int32(len(apiCalls))
	apiCaller := mockAPICaller(c, &numCalls, apiCalls...)

	st := uniter.NewState(apiCaller, nrpeUnitTag)
	r, err := relation.NewRelations(
		relation.RelationsConfig{
			State:                st,
			UnitTag:              nrpeUnitTag,
			CharmDir:             s.stateDir,
			RelationsDir:         s.relationsDir,
			NewLeadershipContext: s.leadershipContextFunc,
			Abort:                make(chan struct{}),
		})
	c.Assert(err, jc.ErrorIsNil)
	assertNumCalls(c, &numCalls, expectedCalls)

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
				Life: params.Alive,
				Members: map[string]int64{
					"wordpress/0": 1,
				},
			},
			2: {
				Life: params.Dying,
				Members: map[string]int64{
					"ntp/0": 1,
				},
			},
		},
	}

	rr := relation.NewRelationsResolver(r)
	_, err = rr.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)

	// Check that we didn't try to make a destroy call (the apiCaller
	// should panic in that case anyway).
	assertNumCalls(c, &numCalls, expectedCalls)
}
