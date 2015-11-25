// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/api/remoterelations"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/params"
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
	client *remoterelations.State
}

func (s *remoteRelationsSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	conn, _ := s.OpenAPIAsNewMachine(c, state.JobManageEnviron)
	s.client = remoterelations.NewState(conn)
}

func (s *remoteRelationsSuite) TestWatchRemoteServices(c *gc.C) {
	_, err := s.State.AddRemoteService("mysql", "local:/u/me/mysql", nil)
	c.Assert(err, jc.ErrorIsNil)

	w, err := s.client.WatchRemoteServices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer statetesting.AssertStop(c, w)

	wc := statetesting.NewStringsWatcherC(c, s.BackingState, w)
	wc.AssertChangeInSingleEvent("mysql")
	wc.AssertNoChange()

	_, err = s.State.AddRemoteService("db2", "local:/u/ibm/db2", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("db2")
	wc.AssertNoChange()
}

func (s *remoteRelationsSuite) TestWatchRemoteService(c *gc.C) {
	// Add a remote service, and watch it. It should initially have no
	// relations.
	_, err := s.State.AddRemoteService("mysql", "local:/u/me/mysql", []charm.Relation{{
		Interface: "mysql",
		Name:      "db",
		Role:      charm.RoleProvider,
		Scope:     charm.ScopeGlobal,
	}})
	c.Assert(err, jc.ErrorIsNil)
	w, err := s.client.WatchRemoteService("mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer statetesting.AssertStop(c, w)
	assertServiceRelationsChange(c, s.BackingState, w, params.ServiceRelationsChange{})
	assertNoServiceRelationsChange(c, s.BackingState, w)

	// Add the relation, and expect a watcher change.
	wordpress := s.Factory.MakeService(c, &factory.ServiceParams{
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	eps, err := s.State.InferEndpoints("wordpress", "mysql")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps[0], eps[1])
	c.Assert(err, jc.ErrorIsNil)

	expect := params.ServiceRelationsChange{
		ChangedRelations: []params.RelationChange{{
			RelationId: rel.Id(),
			Life:       params.Alive,
		}},
	}
	assertServiceRelationsChange(c, s.BackingState, w, expect)
	assertNoServiceRelationsChange(c, s.BackingState, w)

	// Add a unit of wordpress, expect a change.
	settings := map[string]interface{}{"key": "value"}
	wordpress0, err := wordpress.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(settings)
	c.Assert(err, jc.ErrorIsNil)
	expect.ChangedRelations[rel.Id()] = params.RelationChange{
		Life: params.Alive,
		ChangedUnits: map[string]params.RelationUnitChange{
			wordpress0.Name(): params.RelationUnitChange{
				Settings: settings,
			},
		},
	}
	assertServiceRelationsChange(c, s.BackingState, w, expect)
	assertNoServiceRelationsChange(c, s.BackingState, w)

	// Change the settings, expect a change.
	ruSettings, err := ru.Settings()
	c.Assert(err, jc.ErrorIsNil)
	settings["quay"] = 123
	ruSettings.Update(settings)
	_, err = ruSettings.Write()
	c.Assert(err, jc.ErrorIsNil)
	expect.ChangedRelations[rel.Id()].ChangedUnits[wordpress0.Name()] = params.RelationUnitChange{
		Settings: settings,
	}
	assertServiceRelationsChange(c, s.BackingState, w, expect)
	assertNoServiceRelationsChange(c, s.BackingState, w)
}

func assertServiceRelationsChange(
	c *gc.C, ss statetesting.SyncStarter, w apiserver.ServiceRelationsWatcher, change params.ServiceRelationsChange,
) {
	ss.StartSync()
	select {
	case change, ok := <-w.Changes():
		c.Assert(ok, jc.IsTrue)
		c.Assert(change, jc.DeepEquals, change)
	case <-time.After(testing.LongWait):
		c.Errorf("timed out waiting for service relations change")
	}
}

func assertNoServiceRelationsChange(c *gc.C, ss statetesting.SyncStarter, w apiserver.ServiceRelationsWatcher) {
	ss.StartSync()
	select {
	case change, ok := <-w.Changes():
		c.Errorf("unexpected change from service relations watcher: %v, %v", change, ok)
	case <-time.After(testing.ShortWait):
	}
}
