// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	"reflect"

	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/remoterelations"
	"github.com/juju/juju/worker/workertest"
)

var _ = gc.Suite(&remoteRelationsSuite{})

type remoteRelationsSuite struct {
	coretesting.BaseSuite

	resources       *common.Resources
	authorizer      *apiservertesting.FakeAuthorizer
	relationsFacade *mockRelationsFacade
	config          remoterelations.Config
	stub            *jujutesting.Stub
}

func (s *remoteRelationsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = new(jujutesting.Stub)
	s.relationsFacade = newMockRelationsFacade(s.stub)
	s.config = remoterelations.Config{
		RelationsFacade: s.relationsFacade,
		NewPublisherForModelFunc: func(modelUUID string) (remoterelations.RemoteRelationChangePublisherCloser, error) {
			return s.relationsFacade, nil
		},
	}
}

func (s *remoteRelationsSuite) waitForStubCalls(c *gc.C, expected []jujutesting.StubCall) {
	var calls []jujutesting.StubCall
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		calls = s.stub.Calls()
		if reflect.DeepEqual(calls, expected) {
			return
		}
	}
	c.Fatalf("failed to see expected calls. saw: %v", calls)
}

func (s *remoteRelationsSuite) assertRemoteApplicationWorkers(c *gc.C) worker.Worker {
	// Checks that the main worker loop responds to remote application events
	// by starting relevant relation watchers.
	s.relationsFacade.remoteApplications["db2"] = newMockRemoteApplication("db2", "db2url")
	s.relationsFacade.remoteApplications["django"] = newMockRemoteApplication("django", "djangourl")
	applicationNames := []string{"db2", "django"}
	s.relationsFacade.remoteApplicationsWatcher.changes <- applicationNames

	w, err := remoterelations.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	expected := []jujutesting.StubCall{
		{"WatchRemoteApplications", nil},
		{"RemoteApplications", []interface{}{[]string{"db2", "django"}}},
		{"WatchRemoteApplicationRelations", []interface{}{"db2"}},
		{"WatchRemoteApplicationRelations", []interface{}{"django"}},
	}
	s.waitForStubCalls(c, expected)
	for _, app := range applicationNames {
		w, ok := s.relationsFacade.remoteApplicationRelationsWatcher(app)
		c.Check(ok, jc.IsTrue)
		w.CheckCalls(c, []jujutesting.StubCall{
			{"Changes", []interface{}{}},
		})
	}
	return w
}

func (s *remoteRelationsSuite) TestRemoteApplicationWorkers(c *gc.C) {
	w := s.assertRemoteApplicationWorkers(c)
	workertest.CleanKill(c, w)

	// Check that relation watchers are stopped with the worker.
	applicationNames := []string{"db2", "django"}
	for _, app := range applicationNames {
		w, ok := s.relationsFacade.remoteApplicationRelationsWatcher(app)
		c.Check(ok, jc.IsTrue)
		c.Check(w.killed(), jc.IsTrue)
	}
}

func (s *remoteRelationsSuite) TestRemoteApplicationRemoved(c *gc.C) {
	// Checks that when a remote application is removed, the relation
	// worker is killed.
	w := s.assertRemoteApplicationWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	relWatcher, _ := s.relationsFacade.removeApplication("django")
	s.relationsFacade.remoteApplicationsWatcher.changes <- []string{"django"}
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		_, ok := s.relationsFacade.remoteApplicationRelationsWatcher("django")
		if !ok {
			break
		}
	}
	c.Check(relWatcher.killed(), jc.IsTrue)
	expected := []jujutesting.StubCall{
		{"RemoteApplications", []interface{}{[]string{"django"}}},
		{"Close", nil},
	}
	s.waitForStubCalls(c, expected)
}

func (s *remoteRelationsSuite) assertRemoteRelationsWorkers(c *gc.C) worker.Worker {
	s.relationsFacade.relations["db2:db django:db"] = newMockRelation(123)
	w := s.assertRemoteApplicationWorkers(c)
	s.stub.ResetCalls()

	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("django")
	relWatcher.changes <- []string{"db2:db django:db"}

	expected := []jujutesting.StubCall{
		{"Relations", []interface{}{[]string{"db2:db django:db"}}},
		{"WatchLocalRelationUnits", []interface{}{"db2:db django:db"}},
		{"ExportEntities", []interface{}{[]names.Tag{names.NewRelationTag("db2:db django:db")}}},
	}
	s.waitForStubCalls(c, expected)

	unitWatcher, ok := s.relationsFacade.relationsUnitsWatcher("db2:db django:db")
	c.Check(ok, jc.IsTrue)
	unitWatcher.CheckCalls(c, []jujutesting.StubCall{
		{"Changes", []interface{}{}},
	})
	return w
}

func (s *remoteRelationsSuite) TestRemoteRelationsWorkers(c *gc.C) {
	w := s.assertRemoteRelationsWorkers(c)
	workertest.CleanKill(c, w)

	// Check that relation unit watchers are stopped with the worker.
	relWatcher, ok := s.relationsFacade.relationsUnitsWatchers["db2:db django:db"]
	c.Check(ok, jc.IsTrue)
	c.Check(relWatcher.killed(), jc.IsTrue)
}

func (s *remoteRelationsSuite) TestRemoteRelationsDead(c *gc.C) {
	// Checks that when a remote relation dies, the relation units
	// worker is killed.
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	unitsWatcher, _ := s.relationsFacade.updateRelationLife("db2:db django:db", params.Dead)
	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("django")
	relWatcher.changes <- []string{"db2:db django:db"}
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		_, ok := s.relationsFacade.relationsUnitsWatcher("db2:db django:db")
		if !ok {
			break
		}
	}
	c.Assert(unitsWatcher.killed(), jc.IsTrue)
	expected := []jujutesting.StubCall{
		{"Relations", []interface{}{[]string{"db2:db django:db"}}},
	}
	s.waitForStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteRelationsRemoved(c *gc.C) {
	// Checks that when a remote relation goes away, the relation units
	// worker is killed.
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	unitsWatcher, _ := s.relationsFacade.removeRelation("db2:db django:db")
	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("django")
	relWatcher.changes <- []string{"db2:db django:db"}
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		_, ok := s.relationsFacade.relationsUnitsWatcher("db2:db django:db")
		if !ok {
			break
		}
	}
	c.Assert(unitsWatcher.killed(), jc.IsTrue)
	expected := []jujutesting.StubCall{
		{"Relations", []interface{}{[]string{"db2:db django:db"}}},
	}
	s.waitForStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteRelationsChangedNotifies(c *gc.C) {
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	unitsWatcher, _ := s.relationsFacade.relationsUnitsWatcher("db2:db django:db")
	unitsWatcher.changes <- watcher.RelationUnitsChange{
		Changed:  map[string]watcher.UnitSettings{"unit/1": {Version: 2}},
		Departed: []string{"unit/2"},
	}

	expected := []jujutesting.StubCall{
		{"ExportEntities", []interface{}{
			[]names.Tag{names.NewUnitTag("unit/1"), names.NewUnitTag("unit/2")}}},
		{"RelationUnitSettings", []interface{}{
			[]params.RelationUnit{{
				Relation: "relation-db2.db#django.db",
				Unit:     "unit-unit-1"}}}},
		{"PublishLocalRelationChange", []interface{}{
			params.RemoteRelationChangeEvent{
				RelationId: params.RemoteEntityId{
					ModelUUID: "model-uuid",
					Token:     "token-db2:db django:db"},
				ChangedUnits: []params.RemoteRelationUnitChange{{
					UnitId:   params.RemoteEntityId{ModelUUID: "model-uuid", Token: "token-unit/1"},
					Settings: map[string]interface{}{"foo": "bar"},
				}},
				DepartedUnits: []params.RemoteEntityId{{
					ModelUUID: "model-uuid", Token: "token-unit/2",
				}},
			},
		}},
	}
	s.waitForStubCalls(c, expected)
}

func (s *remoteRelationsSuite) TestRemoteRelationsChangedError(c *gc.C) {
	w := s.assertRemoteRelationsWorkers(c)
	// Just in case, ensure worker is killed.
	defer workertest.CheckKill(c, w)
	s.stub.ResetCalls()

	s.stub.SetErrors(nil, errors.New("failed"))
	unitsWatcher, _ := s.relationsFacade.relationsUnitsWatcher("db2:db django:db")
	unitsWatcher.changes <- watcher.RelationUnitsChange{
		Departed: []string{"unit/1"},
	}
	err := workertest.CheckKilled(c, w)
	c.Assert(err, gc.ErrorMatches, "publishing relation change to remote model: failed")
}
