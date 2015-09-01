// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/juju/charm.v5/hooks"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/multiwatcher"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/hook"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/relation"
	"github.com/juju/juju/worker/uniter/remotestate"
	"github.com/juju/juju/worker/uniter/resolver"
)

type relationsSuite struct {
	coretesting.BaseSuite

	stateDir     string
	relationsDir string
}

var _ = gc.Suite(&relationsSuite{})

type apiCall struct {
	facadeName string
	version    int
	id         string
	request    string
	args       interface{}
	result     interface{}
	err        error
}

func uniterApiCall(request string, args, result interface{}, err error) apiCall {
	return apiCall{
		facadeName: "Uniter",
		version:    2,
		request:    request,
		args:       args,
		result:     result,
		err:        err,
	}
}

func watcherApiCall(requst string, args, result interface{}, err error) apiCall {
	return apiCall{
		facadeName: "NotifyWatcher",
		version:    0,
		id:         "1",
		request:    requst,
		args:       args,
		result:     result,
		err:        err,
	}
}

func mockAPICaller(c *gc.C, callNumber *int, apiCalls ...apiCall) apitesting.APICallerFunc {
	apiCaller := apitesting.APICallerFunc(func(objType string, version int, id, request string, arg, result interface{}) error {
		index := *callNumber
		c.Logf("request %d, %s", index, request)
		c.Assert(index < len(apiCalls), jc.IsTrue)
		call := apiCalls[index]
		*callNumber += 1
		c.Check(objType, gc.Equals, call.facadeName)
		c.Check(version, gc.Equals, call.version)
		c.Check(id, gc.Equals, call.id)
		c.Check(request, gc.Equals, call.request)
		c.Check(arg, jc.DeepEquals, call.args)
		if call.err != nil {
			return common.ServerError(call.err)
		}
		testing.PatchValue(result, call.result)
		return nil
	})
	return apiCaller
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
}

func (s *relationsSuite) setupRelations(c *gc.C) relation.Relations {
	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	numCalls := 0
	unitEntity := params.Entities{Entities: []params.Entity{params.Entity{Tag: "unit-mysql-0"}}}
	apiCaller := mockAPICaller(c, &numCalls,
		uniterApiCall("Life", unitEntity, params.LifeResults{Results: []params.LifeResult{{Life: params.Alive}}}, nil),
		uniterApiCall("JoinedRelations", unitEntity, params.StringsResults{Results: []params.StringsResult{{Result: []string{}}}}, nil),
	)
	st := uniter.NewState(apiCaller, unitTag)
	r, err := relation.NewRelations(st, unitTag, s.stateDir, s.relationsDir, abort)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(numCalls, gc.Equals, 2)
	return r
}

func (s *relationsSuite) TestNewRelationsNoRelations(c *gc.C) {
	r := s.setupRelations(c)
	//No relations created.
	c.Assert(r.GetInfo(), gc.HasLen, 0)
}

func (s *relationsSuite) TestNewRelationsWithExistingRelations(c *gc.C) {
	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	numCalls := 0
	unitEntity := params.Entities{Entities: []params.Entity{params.Entity{Tag: "unit-mysql-0"}}}
	relationUnits := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-wordpress.db#mysql.db", Unit: "unit-mysql-0"},
	}}
	relationResults := params.RelationResults{
		Results: []params.RelationResult{
			{
				Id:   1,
				Key:  "wordpress:db mysql:db",
				Life: params.Alive,
				Endpoint: multiwatcher.Endpoint{
					ServiceName: "wordpress",
					Relation:    charm.Relation{Name: "mysql", Role: charm.RoleRequirer, Interface: "db"},
				}},
		},
	}

	apiCaller := mockAPICaller(c, &numCalls,
		uniterApiCall("Life", unitEntity, params.LifeResults{Results: []params.LifeResult{{Life: params.Alive}}}, nil),
		uniterApiCall("JoinedRelations", unitEntity, params.StringsResults{Results: []params.StringsResult{{Result: []string{"relation-wordpress:db mysql:db"}}}}, nil),
		uniterApiCall("Relation", relationUnits, relationResults, nil),
		uniterApiCall("Relation", relationUnits, relationResults, nil),
		uniterApiCall("Watch", unitEntity, params.NotifyWatchResults{Results: []params.NotifyWatchResult{{NotifyWatcherId: "1"}}}, nil),
		uniterApiCall("EnterScope", relationUnits, params.ErrorResults{Results: []params.ErrorResult{{}}}, nil),
		watcherApiCall("Stop", nil, params.ErrorResults{Results: []params.ErrorResult{{}}}, nil),
		watcherApiCall("Next", nil, nil, errors.NewNotFound(nil, "watcher")),
	)
	st := uniter.NewState(apiCaller, unitTag)
	r, err := relation.NewRelations(st, unitTag, s.stateDir, s.relationsDir, abort)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(numCalls, gc.Equals, 8)

	info := r.GetInfo()
	c.Assert(info, gc.HasLen, 1)
	oneInfo := info[1]
	c.Assert(oneInfo.RelationUnit.Relation().Tag(), gc.Equals, names.NewRelationTag("wordpress:db mysql:db"))
	c.Assert(oneInfo.RelationUnit.Endpoint(), jc.DeepEquals, uniter.Endpoint{
		Relation: charm.Relation{Name: "mysql", Role: "requirer", Interface: "db", Optional: false, Limit: 0, Scope: ""},
	})
	c.Assert(oneInfo.MemberNames, gc.HasLen, 0)
}

func (s *relationsSuite) TestNextOpNothing(c *gc.C) {
	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	numCalls := 0
	unitEntity := params.Entities{Entities: []params.Entity{params.Entity{Tag: "unit-mysql-0"}}}
	apiCaller := mockAPICaller(c, &numCalls,
		uniterApiCall("Life", unitEntity, params.LifeResults{Results: []params.LifeResult{{Life: params.Alive}}}, nil),
		uniterApiCall("JoinedRelations", unitEntity, params.StringsResults{Results: []params.StringsResult{{Result: []string{}}}}, nil),
		uniterApiCall("GetPrincipal", unitEntity, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "", Ok: false}}}, nil),
	)
	st := uniter.NewState(apiCaller, unitTag)
	r, err := relation.NewRelations(st, unitTag, s.stateDir, s.relationsDir, abort)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(numCalls, gc.Equals, 2)

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

func relationJoinedApiCalls() []apiCall {
	unitEntity := params.Entities{Entities: []params.Entity{params.Entity{Tag: "unit-mysql-0"}}}
	relationResults := params.RelationResults{
		Results: []params.RelationResult{
			{
				Id:   1,
				Key:  "wordpress:db mysql:db",
				Life: params.Alive,
				Endpoint: multiwatcher.Endpoint{
					ServiceName: "wordpress",
					Relation:    charm.Relation{Name: "mysql", Role: charm.RoleRequirer, Interface: "db", Scope: "global"},
				}},
		},
	}
	relationUnits := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-wordpress.db#mysql.db", Unit: "unit-mysql-0"},
	}}
	apiCalls := []apiCall{
		uniterApiCall("Life", unitEntity, params.LifeResults{Results: []params.LifeResult{{Life: params.Alive}}}, nil),
		uniterApiCall("JoinedRelations", unitEntity, params.StringsResults{Results: []params.StringsResult{{Result: []string{}}}}, nil),
		uniterApiCall("RelationById", params.RelationIds{RelationIds: []int{1}}, relationResults, nil),
		uniterApiCall("Relation", relationUnits, relationResults, nil),
		uniterApiCall("Relation", relationUnits, relationResults, nil),
		uniterApiCall("Watch", unitEntity, params.NotifyWatchResults{Results: []params.NotifyWatchResult{{NotifyWatcherId: "1"}}}, nil),
		uniterApiCall("EnterScope", relationUnits, params.ErrorResults{Results: []params.ErrorResult{{}}}, nil),
		watcherApiCall("Stop", nil, params.ErrorResults{Results: []params.ErrorResult{{}}}, nil),
		watcherApiCall("Next", nil, nil, errors.NewNotFound(nil, "watcher")),
		uniterApiCall("GetPrincipal", unitEntity, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "", Ok: false}}}, nil),
	}
	return apiCalls
}

func (s *relationsSuite) assertHookRelationJoined(c *gc.C, numCalls *int, apiCalls ...apiCall) relation.Relations {
	unitTag := names.NewUnitTag("mysql/0")
	abort := make(chan struct{})

	apiCaller := mockAPICaller(c, numCalls, apiCalls...)
	st := uniter.NewState(apiCaller, unitTag)
	r, err := relation.NewRelations(st, unitTag, s.stateDir, s.relationsDir, abort)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*numCalls, gc.Equals, 2)

	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: remotestate.RelationSnapshot{
				Life: params.Alive,
				Members: map[string]int64{
					"wordpress": 1,
				},
			},
		},
	}
	relationsResolver := relation.NewRelationsResolver(r)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*numCalls, gc.Equals, 10)
	c.Assert(op.String(), gc.Equals, "run hook relation-joined on unit with relation 1")

	// Commit the operation so we save local state for any next operation.
	_, err = r.PrepareHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CommitHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	return r
}

func (s *relationsSuite) TestHookRelationJoined(c *gc.C) {
	numCalls := 0
	s.assertHookRelationJoined(c, &numCalls, relationJoinedApiCalls()...)
}

func (s *relationsSuite) assertHookRelationChanged(c *gc.C, numCalls *int, apiCalls ...apiCall) relation.Relations {
	r := s.assertHookRelationJoined(c, numCalls, apiCalls...)

	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: remotestate.RelationSnapshot{
				Life: params.Alive,
			},
		},
	}
	relationsResolver := relation.NewRelationsResolver(r)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*numCalls, gc.Equals, 11)
	c.Assert(op.String(), gc.Equals, "run hook relation-changed on unit with relation 1")

	// Commit the operation so we save local state for any next operation.
	_, err = r.PrepareHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CommitHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	return r
}

func getPrincipalApiCalls(numCalls int) []apiCall {
	unitEntity := params.Entities{Entities: []params.Entity{params.Entity{Tag: "unit-mysql-0"}}}
	result := make([]apiCall, numCalls)
	for i := 0; i < numCalls; i++ {
		result[i] = uniterApiCall("GetPrincipal", unitEntity, params.StringBoolResults{Results: []params.StringBoolResult{{Result: "", Ok: false}}}, nil)
	}
	return result
}

func (s *relationsSuite) TestHookRelationChanged(c *gc.C) {
	numCalls := 0
	apiCalls := relationJoinedApiCalls()

	apiCalls = append(apiCalls, getPrincipalApiCalls(1)...)
	s.assertHookRelationChanged(c, &numCalls, apiCalls...)
}

func (s *relationsSuite) assertHookRelationDeparted(c *gc.C, numCalls *int, apiCalls ...apiCall) relation.Relations {
	r := s.assertHookRelationChanged(c, numCalls, apiCalls...)

	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: remotestate.RelationSnapshot{
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
	c.Assert(*numCalls, gc.Equals, 12)
	c.Assert(op.String(), gc.Equals, "run hook relation-departed on unit with relation 1")

	// Commit the operation so we save local state for any next operation.
	_, err = r.PrepareHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	err = r.CommitHook(op.(*mockOperation).hookInfo)
	c.Assert(err, jc.ErrorIsNil)
	return r
}

func (s *relationsSuite) TestHookRelationDeparted(c *gc.C) {
	numCalls := 0
	apiCalls := relationJoinedApiCalls()

	apiCalls = append(apiCalls, getPrincipalApiCalls(2)...)
	s.assertHookRelationDeparted(c, &numCalls, apiCalls...)
}

func (s *relationsSuite) TestHookRelationBroken(c *gc.C) {
	numCalls := 0
	apiCalls := relationJoinedApiCalls()

	apiCalls = append(apiCalls, getPrincipalApiCalls(3)...)
	r := s.assertHookRelationDeparted(c, &numCalls, apiCalls...)

	localState := resolver.LocalState{
		State: operation.State{
			Kind: operation.Continue,
		},
	}
	remoteState := remotestate.Snapshot{
		Relations: map[int]remotestate.RelationSnapshot{
			1: remotestate.RelationSnapshot{
				Life: params.Dying,
			},
		},
	}
	relationsResolver := relation.NewRelationsResolver(r)
	op, err := relationsResolver.NextOp(localState, remoteState, &mockOperations{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(numCalls, gc.Equals, 13)
	c.Assert(op.String(), gc.Equals, "run hook relation-broken on unit with relation 1")
}

func (s *relationsSuite) TestCommitHook(c *gc.C) {
	numCalls := 0
	apiCalls := relationJoinedApiCalls()
	relationUnits := params.RelationUnits{RelationUnits: []params.RelationUnit{
		{Relation: "relation-wordpress.db#mysql.db", Unit: "unit-mysql-0"},
	}}
	apiCalls = append(apiCalls,
		uniterApiCall("LeaveScope", relationUnits, params.ErrorResults{Results: []params.ErrorResult{{}}}, nil),
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
