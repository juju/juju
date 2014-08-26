// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	stdtesting "testing"

	"github.com/juju/names"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"gopkg.in/mgo.v2"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

// TestPackage integrates the tests into gotest.
func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

// ConnSuite provides the infrastructure for all other
// test suites (StateSuite, CharmSuite, MachineSuite, etc).
type ConnSuite struct {
	gitjujutesting.MgoSuite
	testing.BaseSuite
	annotations  *mgo.Collection
	charms       *mgo.Collection
	machines     *mgo.Collection
	relations    *mgo.Collection
	services     *mgo.Collection
	units        *mgo.Collection
	stateServers *mgo.Collection
	State        *state.State
	policy       statetesting.MockPolicy
	factory      *factory.Factory
	envTag       names.EnvironTag
}

func (cs *ConnSuite) SetUpSuite(c *gc.C) {
	cs.BaseSuite.SetUpSuite(c)
	cs.MgoSuite.SetUpSuite(c)
}

func (cs *ConnSuite) TearDownSuite(c *gc.C) {
	cs.MgoSuite.TearDownSuite(c)
	cs.BaseSuite.TearDownSuite(c)
}

func (cs *ConnSuite) SetUpTest(c *gc.C) {
	c.Log("SetUpTest")
	cs.BaseSuite.SetUpTest(c)
	cs.MgoSuite.SetUpTest(c)
	cs.policy = statetesting.MockPolicy{}
	cfg := testing.EnvironConfig(c)
	cs.State = TestingInitialize(c, cfg, &cs.policy)
	uuid, ok := cfg.UUID()
	c.Assert(ok, jc.IsTrue)
	cs.envTag = names.NewEnvironTag(uuid)
	cs.annotations = cs.MgoSuite.Session.DB("juju").C("annotations")
	cs.charms = cs.MgoSuite.Session.DB("juju").C("charms")
	cs.machines = cs.MgoSuite.Session.DB("juju").C("machines")
	cs.relations = cs.MgoSuite.Session.DB("juju").C("relations")
	cs.services = cs.MgoSuite.Session.DB("juju").C("services")
	cs.units = cs.MgoSuite.Session.DB("juju").C("units")
	cs.stateServers = cs.MgoSuite.Session.DB("juju").C("stateServers")
	cs.State.AddAdminUser("pass")
	cs.factory = factory.NewFactory(cs.State)
	c.Log("SetUpTest done")
}

func (cs *ConnSuite) TearDownTest(c *gc.C) {
	if cs.State != nil {
		// If setup fails, we don't have a State yet
		cs.State.Close()
	}
	cs.MgoSuite.TearDownTest(c)
	cs.BaseSuite.TearDownTest(c)
}

func (s *ConnSuite) AddTestingCharm(c *gc.C, name string) *state.Charm {
	return state.AddTestingCharm(c, s.State, name)
}

func (s *ConnSuite) AddTestingService(c *gc.C, name string, ch *state.Charm) *state.Service {
	return state.AddTestingService(c, s.State, name, ch)
}

func (s *ConnSuite) AddTestingServiceWithNetworks(c *gc.C, name string, ch *state.Charm, networks []string) *state.Service {
	return state.AddTestingServiceWithNetworks(c, s.State, name, ch, networks)
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
