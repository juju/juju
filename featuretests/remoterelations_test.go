// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/api/remoterelations"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/watcher/watchertest"
	"github.com/juju/juju/worker"
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
		URL:         "local:/u/me/mysql",
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
		URL:         "local:/u/me/db2",
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
		URL:         "local:/u/me/mysql",
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

func (s *remoteRelationsSuite) TestWatchLocalRelationUnits(c *gc.C) {
	_, err := s.State.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        "mysql",
		URL:         "local:/u/me/mysql",
		SourceModel: testing.ModelTag,
		Endpoints: []charm.Relation{{
			Interface: "mysql",
			Name:      "db",
			Role:      charm.RoleProvider,
			Scope:     charm.ScopeGlobal,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)
	w, err := s.client.WatchLocalRelationUnits(rel.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer func() {
		c.Assert(worker.Stop(w), jc.ErrorIsNil)
	}()

	assertRelationUnitsChange(c, s.BackingState, w, watcher.RelationUnitsChange{})
	assertNoRelationUnitsChange(c, s.BackingState, w)

	// Add a unit of wordpress, expect a change.
	settings := map[string]interface{}{"key": "value"}
	wordpress0, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	expect := watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{"wordpress/0": {Version: 0}},
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
	expect = watcher.RelationUnitsChange{
		Changed: map[string]watcher.UnitSettings{"wordpress/0": {Version: 1}},
	}
	assertRelationUnitsChange(c, s.BackingState, w, expect)
	assertNoRelationUnitsChange(c, s.BackingState, w)

	// Remove a unit of wordpress, expect a change.
	err = ru.LeaveScope()
	c.Assert(err, jc.ErrorIsNil)
	expect = watcher.RelationUnitsChange{
		Departed: []string{"wordpress/0"},
	}
	assertRelationUnitsChange(c, s.BackingState, w, expect)
	assertNoRelationUnitsChange(c, s.BackingState, w)
}

func assertRelationUnitsChange(
	c *gc.C, ss statetesting.SyncStarter, w watcher.RelationUnitsWatcher, expect watcher.RelationUnitsChange,
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

func assertNoRelationUnitsChange(c *gc.C, ss statetesting.SyncStarter, w watcher.RelationUnitsWatcher) {
	ss.StartSync()
	select {
	case change, ok := <-w.Changes():
		c.Errorf("unexpected change from relations units watcher: %v, %v", change, ok)
	case <-time.After(testing.ShortWait):
	}
}
