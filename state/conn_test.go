// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/names"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

// ConnSuite provides the infrastructure for all other
// test suites (StateSuite, CharmSuite, MachineSuite, etc).
type ConnSuite struct {
	statetesting.StateSuite
	annotations  *mgo.Collection
	charms       *mgo.Collection
	machines     *mgo.Collection
	instanceData *mgo.Collection
	relations    *mgo.Collection
	services     *mgo.Collection
	units        *mgo.Collection
	stateServers *mgo.Collection
	policy       statetesting.MockPolicy
	envTag       names.EnvironTag
}

func (cs *ConnSuite) SetUpTest(c *gc.C) {
	c.Log("SetUpTest")

	cs.policy = statetesting.MockPolicy{}
	cs.StateSuite.Policy = &cs.policy

	cs.StateSuite.SetUpTest(c)

	cs.envTag = cs.State.EnvironTag()

	jujuDB := cs.MgoSuite.Session.DB("juju")
	cs.annotations = jujuDB.C("annotations")
	cs.charms = jujuDB.C("charms")
	cs.machines = jujuDB.C("machines")
	cs.instanceData = jujuDB.C("instanceData")
	cs.relations = jujuDB.C("relations")
	cs.services = jujuDB.C("services")
	cs.units = jujuDB.C("units")
	cs.stateServers = jujuDB.C("stateServers")

	c.Log("SetUpTest done")
}

func (s *ConnSuite) AddTestingCharm(c *gc.C, name string) *state.Charm {
	return state.AddTestingCharm(c, s.State, name)
}

func (s *ConnSuite) AddTestingService(c *gc.C, name string, ch *state.Charm) *state.Service {
	return state.AddTestingService(c, s.State, name, ch, s.Owner)
}

func (s *ConnSuite) AddTestingServiceWithNetworks(c *gc.C, name string, ch *state.Charm, networks []string) *state.Service {
	return state.AddTestingServiceWithNetworks(c, s.State, name, ch, s.Owner, networks)
}

func (s *ConnSuite) AddTestingServiceWithStorage(c *gc.C, name string, ch *state.Charm, storage map[string]state.StorageConstraints) *state.Service {
	return state.AddTestingServiceWithStorage(c, s.State, name, ch, s.Owner, storage)
}

func (s *ConnSuite) AddTestingServiceWithBindings(c *gc.C, name string, ch *state.Charm, bindings map[string]string) *state.Service {
	return state.AddTestingServiceWithBindings(c, s.State, name, ch, s.Owner, bindings)
}

func (s *ConnSuite) AddSeriesCharm(c *gc.C, name, series string) *state.Charm {
	return state.AddCustomCharm(c, s.State, name, "", "", series, -1)
}

// AddConfigCharm clones a testing charm, replaces its config with
// the given YAML string and adds it to the state, using the given
// revision.
func (s *ConnSuite) AddConfigCharm(c *gc.C, name, configYaml string, revision int) *state.Charm {
	return state.AddCustomCharm(c, s.State, name, "config.yaml", configYaml, "quantal", revision)
}

// AddActionsCharm clones a testing charm, replaces its actions schema with
// the given YAML, and adds it to the state, using the given revision.
func (s *ConnSuite) AddActionsCharm(c *gc.C, name, actionsYaml string, revision int) *state.Charm {
	return state.AddCustomCharm(c, s.State, name, "actions.yaml", actionsYaml, "quantal", revision)
}

// AddMetaCharm clones a testing charm, replaces its metadata with the
// given YAML string and adds it to the state, using the given revision.
func (s *ConnSuite) AddMetaCharm(c *gc.C, name, metaYaml string, revsion int) *state.Charm {
	return state.AddCustomCharm(c, s.State, name, "metadata.yaml", metaYaml, "quantal", revsion)
}

// AddMetricsCharm clones a testing charm, replaces its metrics declaration with the
// given YAML string and adds it to the state, using the given revision.
func (s *ConnSuite) AddMetricsCharm(c *gc.C, name, metricsYaml string, revsion int) *state.Charm {
	return state.AddCustomCharm(c, s.State, name, "metrics.yaml", metricsYaml, "quantal", revsion)
}
