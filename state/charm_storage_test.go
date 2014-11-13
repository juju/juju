// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testcharms"
)

type CharmStorageSuite struct{}

var _ = gc.Suite(&CharmStorageSuite{})

func (*CharmStorageSuite) TestMetaStorage(c *gc.C) {
	chd := testcharms.Repo.CharmDir("mysql")
	meta := chd.Meta()
	c.Assert(meta, gc.NotNil)

	stored := state.StoreCharmMeta(meta)
	c.Assert(stored, gc.NotNil)

	metaRestored := stored.Convert()
	c.Assert(metaRestored, gc.DeepEquals, meta)
}

func (*CharmStorageSuite) TestNilMetaStorage(c *gc.C) {
	var meta *charm.Meta
	stored := state.StoreCharmMeta(meta)
	c.Assert(stored, gc.IsNil)
}

func (*CharmStorageSuite) TestConfigStorage(c *gc.C) {
	chd := testcharms.Repo.CharmDir("mysql")
	config := chd.Config()
	c.Assert(config, gc.NotNil)

	stored := state.StoreCharmConfig(config)
	c.Assert(stored, gc.NotNil)

	configRestored := stored.Convert()
	c.Assert(configRestored, gc.DeepEquals, config)
}

func (*CharmStorageSuite) TestNilConfigStorage(c *gc.C) {
	var config *charm.Config
	stored := state.StoreCharmConfig(config)
	c.Assert(stored, gc.IsNil)
}

func (*CharmStorageSuite) TestActionsStorage(c *gc.C) {
	chd := testcharms.Repo.CharmDir("dummy")
	actions := chd.Actions()
	c.Assert(actions, gc.NotNil)

	stored := state.StoreCharmActions(actions)
	c.Assert(stored, gc.NotNil)

	actionsRestored := stored.Convert()
	c.Assert(actionsRestored, gc.DeepEquals, actions)
}

func (*CharmStorageSuite) TestNilActionsStorage(c *gc.C) {
	var actions *charm.Actions
	stored := state.StoreCharmActions(actions)
	c.Assert(stored, gc.IsNil)
}

func (*CharmStorageSuite) TestMetricsStorage(c *gc.C) {
	chd := testcharms.Repo.CharmDir("metered")
	metrics := chd.Metrics()
	c.Assert(metrics, gc.NotNil)

	stored := state.StoreCharmMetrics(metrics)
	c.Assert(stored, gc.NotNil)

	metricsRestored := stored.Convert()
	c.Assert(metricsRestored, gc.DeepEquals, metrics)
}

func (*CharmStorageSuite) TestNilMetricsStorage(c *gc.C) {
	var metrics *charm.Metrics
	stored := state.StoreCharmMetrics(metrics)
	c.Assert(stored, gc.IsNil)
}
