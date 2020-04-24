// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"time"

	"github.com/juju/charm/v7"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/remoterelations"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

// TODO(axw) this suite should be re-written as end-to-end tests using the
// remote relations worker when it is ready.

type remoteRelationsSuite struct {
	jujutesting.JujuConnSuite
	client *remoterelations.Client
}

func (s *remoteRelationsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	conn, _ := s.OpenAPIAsNewMachine(c, state.JobManageModel)
	s.client = remoterelations.NewClient(conn)
}

func (s *remoteRelationsSuite) TestWatchRemoteApplications(c *gc.C) {
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql",
		SourceModel: testing.ModelTag,
	})
	c.Assert(err, jc.ErrorIsNil)

	w, err := s.client.WatchRemoteApplications()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer func() {
		c.Assert(worker.Stop(w), jc.ErrorIsNil)
	}()

	wc := watchertest.NewStringsWatcherC(c, w, s.BackingState.StartSync)
	defer wc.AssertStops()

	wc.AssertChangeInSingleEvent("mysql")
	wc.AssertNoChange()

	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "db2",
		SourceModel: testing.ModelTag,
	})
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("db2")
	wc.AssertNoChange()
}

func (s *remoteRelationsSuite) TestWatchRemoteApplicationRelations(c *gc.C) {
	// Add a remote application, and watch it. It should initially have no
	// relations.
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql",
		SourceModel: testing.ModelTag,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	w, err := s.client.WatchRemoteApplicationRelations("mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer func() {
		c.Assert(worker.Stop(w), jc.ErrorIsNil)
	}()

	assertRemoteRelationsChange(c, s.BackingState, w, []string{})
	assertNoRemoteRelationsChange(c, s.BackingState, w)

	// Add the relation, and expect a watcher change.
	s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)

	assertRemoteRelationsChange(c, s.BackingState, w, []string{rel.String()})
	assertNoRemoteRelationsChange(c, s.BackingState, w)
}

func assertRemoteRelationsChange(
	c *gc.C, ss statetesting.SyncStarter, w watcher.StringsWatcher, expect []string,
) {
	ss.StartSync()
	select {
	case change, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(change, jc.DeepEquals, expect)
	case <-time.After(testing.LongWait):
		c.Errorf("timed out waiting for application relations change")
	}
}

func assertNoRemoteRelationsChange(c *gc.C, ss statetesting.SyncStarter, w watcher.StringsWatcher) {
	ss.StartSync()
	select {
	case change, ok := <-w.Changes():
		c.Errorf("unexpected change from application relations watcher: %v, %v", change, ok)
	case <-time.After(testing.ShortWait):
	}
}

func (s *remoteRelationsSuite) TestWatchLocalRelationChanges(c *gc.C) {
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql",
		SourceModel: testing.ModelTag,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	wordpress := s.AddTestingApplication(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)
	remoteEntities := s.State.RemoteEntities()
	relToken, err := remoteEntities.ExportLocalEntity(rel.Tag())
	c.Assert(err, jc.ErrorIsNil)
	appToken, err := remoteEntities.ExportLocalEntity(wordpress.Tag())
	c.Assert(err, jc.ErrorIsNil)
	s.WaitForModelWatchersIdle(c, s.State.ModelUUID())

	w, err := s.client.WatchLocalRelationChanges(rel.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer func() {
		c.Assert(worker.Stop(w), jc.ErrorIsNil)
	}()

	assertRelationUnitsChange(c, s.BackingState, w, params.RemoteRelationChangeEvent{
		RelationToken:    relToken,
		ApplicationToken: appToken,
	})
	assertNoRelationUnitsChange(c, s.BackingState, w)

	// Add a unit of wordpress, expect a change.
	settings := map[string]interface{}{"key": "value"}
	wordpress0, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	expect := params.RemoteRelationChangeEvent{
		RelationToken:    relToken,
		ApplicationToken: appToken,
		ChangedUnits: []params.RemoteRelationUnitChange{{
			UnitId: 0,
			Settings: map[string]interface{}{
				"key": "value",
			},
		}},
	}
	assertRelationUnitsChange(c, s.BackingState, w, expect)
	assertNoRelationUnitsChange(c, s.BackingState, w)

	// Change the settings, expect a change.
	ruSettings, err := ru.Settings()
	c.Assert(err, jc.ErrorIsNil)
	settings["quay"] = 123
	ruSettings.Update(settings)
	_, err = ruSettings.Write()
	c.Assert(err, jc.ErrorIsNil)
	// Numeric settings values are unmarshalled as float64.
	settings["quay"] = float64(123)
	expect = params.RemoteRelationChangeEvent{
		RelationToken:    relToken,
		ApplicationToken: appToken,
		ChangedUnits: []params.RemoteRelationUnitChange{{
			UnitId: 0,
			Settings: map[string]interface{}{
				"key":  "value",
				"quay": float64(123),
			},
		}},
	}
	assertRelationUnitsChange(c, s.BackingState, w, expect)
	assertNoRelationUnitsChange(c, s.BackingState, w)

	// Remove a unit of wordpress, expect a change.
	err = ru.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	expect = params.RemoteRelationChangeEvent{
		RelationToken:    relToken,
		ApplicationToken: appToken,
		DepartedUnits:    []int{0},
	}
	assertRelationUnitsChange(c, s.BackingState, w, expect)
	assertNoRelationUnitsChange(c, s.BackingState, w)
}

func assertRelationUnitsChange(
	c *gc.C, ss statetesting.SyncStarter, w apiwatcher.RemoteRelationWatcher, expect params.RemoteRelationChangeEvent,
) {
	ss.StartSync()
	select {
	case change, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(change, jc.DeepEquals, expect)
	case <-time.After(testing.LongWait):
		c.Errorf("timed out waiting for relations unit change")
	}
}

func assertNoRelationUnitsChange(c *gc.C, ss statetesting.SyncStarter, w apiwatcher.RemoteRelationWatcher) {
	ss.StartSync()
	select {
	case change, ok := <-w.Changes():
		c.Errorf("unexpected change from relations units watcher: %v, %v", change, ok)
	case <-time.After(testing.ShortWait):
	}
}

func (s *remoteRelationsSuite) TestWatchRemoteRelations(c *gc.C) {
	w, err := s.client.WatchRemoteRelations()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer func() {
		c.Assert(worker.Stop(w), jc.ErrorIsNil)
	}()

	assertRemoteRelationsChange(c, s.BackingState, w, []string{})
	assertNoRemoteRelationsChange(c, s.BackingState, w)

	// Add a relation, and expect a watcher change.
	_, err = s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql",
		SourceModel: testing.ModelTag,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)

	assertRemoteRelationsChange(c, s.BackingState, w, []string{rel.String()})
	assertNoRemoteRelationsChange(c, s.BackingState, w)
}
