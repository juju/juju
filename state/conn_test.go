// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
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
	controllers  *mgo.Collection
	policy       statetesting.MockPolicy
	modelTag     names.ModelTag
}

func (cs *ConnSuite) SetUpTest(c *gc.C) {
	c.Log("SetUpTest")

	cs.policy = statetesting.MockPolicy{
		GetStorageProviderRegistry: func() (storage.ProviderRegistry, error) {
			return dummy.StorageProviders(), nil
		},
	}
	cs.StateSuite.NewPolicy = func(*state.State) state.Policy {
		return &cs.policy
	}

	cs.StateSuite.SetUpTest(c)

	cs.modelTag = cs.State.ModelTag()

	jujuDB := cs.MgoSuite.Session.DB("juju")
	cs.annotations = jujuDB.C("annotations")
	cs.charms = jujuDB.C("charms")
	cs.machines = jujuDB.C("machines")
	cs.instanceData = jujuDB.C("instanceData")
	cs.relations = jujuDB.C("relations")
	cs.services = jujuDB.C("applications")
	cs.units = jujuDB.C("units")
	cs.controllers = jujuDB.C("controllers")

	c.Log("SetUpTest done")
}

func (s *ConnSuite) AddTestingCharm(c *gc.C, name string) *state.Charm {
	return state.AddTestingCharm(c, s.State, name)
}

func (s *ConnSuite) AddTestingService(c *gc.C, name string, ch *state.Charm) *state.Application {
	return state.AddTestingService(c, s.State, name, ch)
}

func (s *ConnSuite) AddTestingServiceWithStorage(c *gc.C, name string, ch *state.Charm, storage map[string]state.StorageConstraints) *state.Application {
	return state.AddTestingServiceWithStorage(c, s.State, name, ch, storage)
}

func (s *ConnSuite) AddTestingServiceWithBindings(c *gc.C, name string, ch *state.Charm, bindings map[string]string) *state.Application {
	return state.AddTestingServiceWithBindings(c, s.State, name, ch, bindings)
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
func (s *ConnSuite) AddMetaCharm(c *gc.C, name, metaYaml string, revision int) *state.Charm {
	return state.AddCustomCharm(c, s.State, name, "metadata.yaml", metaYaml, "quantal", revision)
}

// AddMetricsCharm clones a testing charm, replaces its metrics declaration with the
// given YAML string and adds it to the state, using the given revision.
func (s *ConnSuite) AddMetricsCharm(c *gc.C, name, metricsYaml string, revision int) *state.Charm {
	return state.AddCustomCharm(c, s.State, name, "metrics.yaml", metricsYaml, "quantal", revision)
}

// NewStateForModelNamed returns an new model with the given modelName, which
// has a unique UUID, and does not need to be closed when the test completes.
func (s *ConnSuite) NewStateForModelNamed(c *gc.C, modelName string) *state.State {
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": modelName,
		"uuid": utils.MustNewUUID().String(),
	})
	otherOwner := names.NewLocalUserTag("test-admin")
	_, otherState, err := s.State.NewModel(state.ModelArgs{
		CloudName: "dummy", CloudRegion: "dummy-region", Config: cfg, Owner: otherOwner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})

	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { otherState.Close() })
	return otherState
}
