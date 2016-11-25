// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations_test

import (
	"reflect"
	"time"

	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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
	var gotChange watcher.RelationUnitsChange
	changed := make(chan bool)
	s.PatchValue(&remoterelations.ObserverRelationUnitsChange,
		func(change watcher.RelationUnitsChange) error {
			gotChange = change
			changed <- true
			return nil
		})
	w := s.assertRemoteRelationsWorkers(c)
	defer workertest.CleanKill(c, w)
	s.stub.ResetCalls()

	unitsWatcher, _ := s.relationsFacade.relationsUnitsWatcher("db2:db django:db")
	unitsWatcher.changes <- watcher.RelationUnitsChange{
		Changed:  map[string]watcher.UnitSettings{"unit1": {Version: 2}},
		Departed: []string{"unit2"},
	}
	select {
	case <-changed:
	case <-time.After(coretesting.LongWait):
		c.Fatal("waiting for relation changes")
	}
	c.Assert(gotChange, jc.DeepEquals, watcher.RelationUnitsChange{
		Changed:  map[string]watcher.UnitSettings{"unit1": {Version: 2}},
		Departed: []string{"unit2"},
	})
}

func (s *remoteRelationsSuite) TestRemoteRelationsChangedError(c *gc.C) {
	s.PatchValue(&remoterelations.ObserverRelationUnitsChange,
		func(change watcher.RelationUnitsChange) error {
			return errors.New("failed")
		})
	w := s.assertRemoteRelationsWorkers(c)

	unitsWatcher, _ := s.relationsFacade.relationsUnitsWatcher("db2:db django:db")
	unitsWatcher.changes <- watcher.RelationUnitsChange{
		Changed:  map[string]watcher.UnitSettings{"unit1": {Version: 2}},
		Departed: []string{"unit2"},
	}
	err := workertest.CheckKilled(c, w)
	c.Assert(err, gc.ErrorMatches, "failed")
}
