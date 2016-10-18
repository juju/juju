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
	client *remoterelations.State
}

func (s *remoteRelationsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	conn, _ := s.OpenAPIAsNewMachine(c, state.JobManageModel)
	s.client = remoterelations.NewState(conn)
}

func (s *remoteRelationsSuite) TestWatchRemoteApplications(c *gc.C) {
	_, err := s.State.AddRemoteApplication("mysql", "local:/u/me/mysql", nil)
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

	_, err = s.State.AddRemoteApplication("db2", "local:/u/ibm/db2", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("db2")
	wc.AssertNoChange()
}

func (s *remoteRelationsSuite) TestWatchRemoteApplication(c *gc.C) {
	// Add a remote application, and watch it. It should initially have no
	// relations.
	_, err := s.State.AddRemoteApplication("mysql", "local:/u/me/mysql", []charm.Relation{{
		Interface: "mysql",
		Name:      "db",
		Role:      charm.RoleProvider,
		Scope:     charm.ScopeGlobal,
	}})
	c.Assert(err, jc.ErrorIsNil)
	w, err := s.client.WatchRemoteApplication("mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer func() {
		c.Assert(worker.Stop(w), jc.ErrorIsNil)
	}()

	assertApplicationRelationsChange(c, s.BackingState, w, watcher.ApplicationRelationsChange{})
	assertNoApplicationRelationsChange(c, s.BackingState, w)

	// Add the relation, and expect a watcher change.
	wordpress := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)

	expect := watcher.ApplicationRelationsChange{
		ChangedRelations: []watcher.RelationChange{{
			RelationId:   rel.Id(),
			Life:         "alive",
			ChangedUnits: map[string]watcher.RelationUnitChange{},
		}},
	}
	assertApplicationRelationsChange(c, s.BackingState, w, expect)
	assertNoApplicationRelationsChange(c, s.BackingState, w)

	// Add a unit of wordpress, expect a change.
	settings := map[string]interface{}{"key": "value"}
	wordpress0, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	expect.ChangedRelations[rel.Id()] = watcher.RelationChange{
		Life: "alive",
		ChangedUnits: map[string]watcher.RelationUnitChange{
			wordpress0.Name(): {
				Settings: settings,
			},
		},
	}
	assertApplicationRelationsChange(c, s.BackingState, w, expect)
	assertNoApplicationRelationsChange(c, s.BackingState, w)

	// Change the settings, expect a change.
	ruSettings, err := ru.Settings()
	c.Assert(err, jc.ErrorIsNil)
	settings["quay"] = 123
	ruSettings.Update(settings)
	_, err = ruSettings.Write()
	c.Assert(err, jc.ErrorIsNil)
	// Numeric settings values are unmarshalled as float64.
	settings["quay"] = float64(123)
	expect.ChangedRelations[rel.Id()].ChangedUnits[wordpress0.Name()] = watcher.RelationUnitChange{
		Settings: settings,
	}
	assertApplicationRelationsChange(c, s.BackingState, w, expect)
	assertNoApplicationRelationsChange(c, s.BackingState, w)
}

func assertApplicationRelationsChange(
	c *gc.C, ss statetesting.SyncStarter, w watcher.ApplicationRelationsWatcher, expect watcher.ApplicationRelationsChange,
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

func assertNoApplicationRelationsChange(c *gc.C, ss statetesting.SyncStarter, w watcher.ApplicationRelationsWatcher) {
	ss.StartSync()
	select {
	case change, ok := <-w.Changes():
		c.Errorf("unexpected change from application relations watcher: %v, %v", change, ok)
	case <-time.After(testing.ShortWait):
	}
}
