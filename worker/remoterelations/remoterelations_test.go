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
	s.relationsFacade.remoteApplications["mysql"] = newMockRemoteApplication("mysql", "mysqlurl")
	applicationNames := []string{"db2", "mysql"}
	s.relationsFacade.remoteApplicationsWatcher.changes <- applicationNames

	w, err := remoterelations.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	expected := []jujutesting.StubCall{
		{"WatchRemoteApplications", nil},
		{"RemoteApplications", []interface{}{[]string{"db2", "mysql"}}},
		{"ExportEntities", []interface{}{[]names.Tag{names.NewApplicationTag("db2")}}},
		{"WatchRemoteApplicationRelations", []interface{}{"db2"}},
		{"ExportEntities", []interface{}{[]names.Tag{names.NewApplicationTag("mysql")}}},
		{"WatchRemoteApplicationRelations", []interface{}{"mysql"}},
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
	applicationNames := []string{"db2", "mysql"}
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

	relWatcher, _ := s.relationsFacade.removeApplication("mysql")
	s.relationsFacade.remoteApplicationsWatcher.changes <- []string{"mysql"}
	for a := coretesting.LongAttempt.Start(); a.Next(); {
		_, ok := s.relationsFacade.remoteApplicationRelationsWatcher("mysql")
		if !ok {
			break
		}
	}
	c.Check(relWatcher.killed(), jc.IsTrue)
	expected := []jujutesting.StubCall{
		{"RemoteApplications", []interface{}{[]string{"mysql"}}},
		{"Close", nil},
	}
	s.waitForStubCalls(c, expected)
}

func (s *remoteRelationsSuite) assertRemoteRelationsWorkers(c *gc.C) worker.Worker {
	s.relationsFacade.relations["db2:db django:db"] = newMockRelation(123)
	w := s.assertRemoteApplicationWorkers(c)
	s.stub.ResetCalls()

	s.relationsFacade.relationsEndpoints["db2:db django:db"] = &relationEndpointInfo{
		localApplicationName: "django",
		localEndpoint: params.RemoteEndpoint{
			Name:      "db2",
			Role:      "requires",
			Interface: "db2",
			Limit:     1,
			Scope:     "global",
		},
		remoteEndpointName: "data",
	}

	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("db2")
	relWatcher.changes <- []string{"db2:db django:db"}

	expected := []jujutesting.StubCall{
		{"Relations", []interface{}{[]string{"db2:db django:db"}}},
		{"ExportEntities", []interface{}{[]names.Tag{names.NewRelationTag("db2:db django:db")}}},
		{"RegisterRemoteRelation", []interface{}{params.RegisterRemoteRelation{
			ApplicationId: params.RemoteEntityId{ModelUUID: "model-uuid", Token: "token-db2"},
			RelationId:    params.RemoteEntityId{ModelUUID: "model-uuid", Token: "token-db2:db django:db"},
			RemoteEndpoint: params.RemoteEndpoint{
				Name:      "db2",
				Role:      "requires",
				Interface: "db2",
				Limit:     1,
				Scope:     "global",
			},
			OfferedApplicationName: "db2",
			LocalEndpointName:      "data",
		}}},
		{"WatchLocalRelationUnits", []interface{}{"db2:db django:db"}},
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
	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("db2")
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
	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("db2")
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
		{"RelationUnitSettings", []interface{}{
			[]params.RelationUnit{{
				Relation: "relation-db2.db#django.db",
				Unit:     "unit-unit-1"}}}},
		{"PublishLocalRelationChange", []interface{}{
			params.RemoteRelationChangeEvent{
				Life: params.Alive,
				ApplicationId: params.RemoteEntityId{
					ModelUUID: "model-uuid",
					Token:     "token-db2"},
				RelationId: params.RemoteEntityId{
					ModelUUID: "model-uuid",
					Token:     "token-db2:db django:db"},
				ChangedUnits: []params.RemoteRelationUnitChange{{
					UnitId:   1,
					Settings: map[string]interface{}{"foo": "bar"},
				}},
				DepartedUnits: []int{2},
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

	s.stub.SetErrors(errors.New("failed"))
	unitsWatcher, _ := s.relationsFacade.relationsUnitsWatcher("db2:db django:db")
	unitsWatcher.changes <- watcher.RelationUnitsChange{
		Departed: []string{"unit/1"},
	}
	err := workertest.CheckKilled(c, w)
	c.Assert(err, gc.ErrorMatches, "publishing relation change to remote model: failed")
}

func (s *remoteRelationsSuite) TestRegisteredApplicationNotRegistered(c *gc.C) {
	s.relationsFacade.relations["db2:db django:db"] = newMockRelation(123)
	db2app := newMockRemoteApplication("db2", "db2url")
	db2app.registered = true
	s.relationsFacade.remoteApplications["db2"] = db2app
	applicationNames := []string{"db2"}
	s.relationsFacade.remoteApplicationsWatcher.changes <- applicationNames

	w, err := remoterelations.New(s.config)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	expected := []jujutesting.StubCall{
		{"WatchRemoteApplications", nil},
		{"RemoteApplications", []interface{}{[]string{"db2"}}},
		{"ExportEntities", []interface{}{[]names.Tag{names.NewApplicationTag("db2")}}},
		{"WatchRemoteApplicationRelations", []interface{}{"db2"}},
	}
	s.waitForStubCalls(c, expected)
	s.stub.ResetCalls()

	s.relationsFacade.relationsEndpoints["db2:db django:db"] = &relationEndpointInfo{
		localApplicationName: "django",
		localEndpoint: params.RemoteEndpoint{
			Name:      "db2",
			Role:      "requires",
			Interface: "db2",
			Limit:     1,
			Scope:     "global",
		},
		remoteEndpointName: "data",
	}

	relWatcher, _ := s.relationsFacade.remoteApplicationRelationsWatcher("db2")
	relWatcher.changes <- []string{"db2:db django:db"}

	expected = []jujutesting.StubCall{
		{"Relations", []interface{}{[]string{"db2:db django:db"}}},
		{"GetToken", []interface{}{"remote-model-uuid", names.NewRelationTag("db2:db django:db")}},
		{"WatchLocalRelationUnits", []interface{}{"db2:db django:db"}},
	}
	s.waitForStubCalls(c, expected)
}
