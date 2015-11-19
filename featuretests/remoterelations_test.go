// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featuretests

import (
	"time"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/remoterelations"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

// TODO(axw) this suite should be re-written as end-to-end tests using the
// remote relations worker when it is ready.

type remoteRelationsSuite struct {
	statetesting.StateSuite

	resources  *common.Resources
	authorizer *apiservertesting.FakeAuthorizer
}

func (s *remoteRelationsSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(*gc.C) { s.resources.StopAll() })
	s.authorizer = &apiservertesting.FakeAuthorizer{
		Tag:            names.NewMachineTag("0"),
		EnvironManager: true,
	}
}

func (s *remoteRelationsSuite) TestWatchRemoteServices(c *gc.C) {
	// TODO(axw) when we add the api client, use JujuConnSuite and rewrite
	// this test to use it.
	serverFacade, err := remoterelations.NewStateRemoteRelationsAPI(
		s.State, s.resources, s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)

	result, err := serverFacade.WatchRemoteServices()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.StringsWatchResult{
		StringsWatcherId: "1",
		Changes:          []string{},
	})
	w := s.resources.Get("1").(state.StringsWatcher)
	wc := statetesting.NewStringsWatcherC(c, s.State, w)
	wc.AssertNoChange()

	_, err = s.State.AddRemoteService("db2", nil)
	c.Assert(err, jc.ErrorIsNil)
	wc.AssertChangeInSingleEvent("db2")
}

func (s *remoteRelationsSuite) TestWatchRemoteService(c *gc.C) {
	// TODO(axw) when we add the api client, use JujuConnSuite and rewrite
	// this test to use it.
	serverFacade, err := remoterelations.NewStateRemoteRelationsAPI(
		s.State, s.resources, s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)

	// Add a remote service, and watch it. It should initially have no
	// relations.
	_, err = s.State.AddRemoteService("mysql", []charm.Relation{{
		Interface: "mysql",
		Name:      "db",
		Role:      charm.RoleProvider,
		Scope:     charm.ScopeGlobal,
	}})
	c.Assert(err, jc.ErrorIsNil)
	results, err := serverFacade.WatchRemoteService(params.Entities{[]params.Entity{
		{Tag: "service-mysql"},
	}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ServiceRelationsWatchResult{{
		ServiceRelationsWatcherId: "1",
		Changes:                   &params.ServiceRelationsChange{},
	}})
	w := s.resources.Get("1").(apiserver.ServiceRelationsWatcher)
	assertNoServiceRelationsChange(c, s.State, w)

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
		ChangedRelations: map[int]params.RelationChange{
			rel.Id(): params.RelationChange{
				Life: params.Alive,
			},
		},
	}
	assertServiceRelationsChange(c, s.State, w, expect)
	assertNoServiceRelationsChange(c, s.State, w)

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
	assertServiceRelationsChange(c, s.State, w, expect)
	assertNoServiceRelationsChange(c, s.State, w)

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
	assertServiceRelationsChange(c, s.State, w, expect)
	assertNoServiceRelationsChange(c, s.State, w)
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
