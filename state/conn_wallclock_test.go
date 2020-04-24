// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/testing"
)

// ConnWithWallClockSuite provides the infrastructure for all other test suites
// (StateSuite, CharmSuite, MachineSuite, etc). This should be deprecated in
// favour of ConnSuite, and tests updated to use the testing clock provided in
// ConnSuite via StateSuite.
type ConnWithWallClockSuite struct {
	statetesting.StateWithWallClockSuite
	annotations  *mgo.Collection
	charms       *mgo.Collection
	machines     *mgo.Collection
	instanceData *mgo.Collection
	relations    *mgo.Collection
	applications *mgo.Collection
	units        *mgo.Collection
	controllers  *mgo.Collection
	policy       statetesting.MockPolicy
	modelTag     names.ModelTag
}

func (cs *ConnWithWallClockSuite) SetUpTest(c *gc.C) {
	cs.policy = statetesting.MockPolicy{
		GetStorageProviderRegistry: func() (storage.ProviderRegistry, error) {
			return storage.ChainedProviderRegistry{
				dummy.StorageProviders(),
				provider.CommonStorageProviders(),
			}, nil
		},
	}
	cs.StateWithWallClockSuite.NewPolicy = func(*state.State) state.Policy {
		return &cs.policy
	}

	cs.StateWithWallClockSuite.SetUpTest(c)

	cs.modelTag = cs.Model.ModelTag()

	jujuDB := cs.MgoSuite.Session.DB("juju")
	cs.annotations = jujuDB.C("annotations")
	cs.charms = jujuDB.C("charms")
	cs.machines = jujuDB.C("machines")
	cs.instanceData = jujuDB.C("instanceData")
	cs.relations = jujuDB.C("relations")
	cs.applications = jujuDB.C("applications")
	cs.units = jujuDB.C("units")
	cs.controllers = jujuDB.C("controllers")
}

func (s *ConnWithWallClockSuite) AddTestingCharm(c *gc.C, name string) *state.Charm {
	return state.AddTestingCharm(c, s.State, name)
}

func (s *ConnWithWallClockSuite) AddTestingApplication(c *gc.C, name string, ch *state.Charm) *state.Application {
	return state.AddTestingApplication(c, s.State, name, ch)
}

func (s *ConnWithWallClockSuite) AddTestingApplicationWithStorage(c *gc.C, name string, ch *state.Charm, storage map[string]state.StorageConstraints) *state.Application {
	return state.AddTestingApplicationWithStorage(c, s.State, name, ch, storage)
}

func (s *ConnWithWallClockSuite) AddTestingApplicationWithBindings(c *gc.C, name string, ch *state.Charm, bindings map[string]string) *state.Application {
	return state.AddTestingApplicationWithBindings(c, s.State, name, ch, bindings)
}

func (s *ConnWithWallClockSuite) AddSeriesCharm(c *gc.C, name, series string) *state.Charm {
	return state.AddCustomCharm(c, s.State, name, "", "", series, -1)
}

// AddConfigCharm clones a testing charm, replaces its config with
// the given YAML string and adds it to the state, using the given
// revision.
func (s *ConnWithWallClockSuite) AddConfigCharm(c *gc.C, name, configYaml string, revision int) *state.Charm {
	return state.AddCustomCharm(c, s.State, name, "config.yaml", configYaml, "quantal", revision)
}

// AddActionsCharm clones a testing charm, replaces its actions schema with
// the given YAML, and adds it to the state, using the given revision.
func (s *ConnWithWallClockSuite) AddActionsCharm(c *gc.C, name, actionsYaml string, revision int) *state.Charm {
	return state.AddCustomCharm(c, s.State, name, "actions.yaml", actionsYaml, "quantal", revision)
}

// AddMetaCharm clones a testing charm, replaces its metadata with the
// given YAML string and adds it to the state, using the given revision.
func (s *ConnWithWallClockSuite) AddMetaCharm(c *gc.C, name, metaYaml string, revision int) *state.Charm {
	return state.AddCustomCharm(c, s.State, name, "metadata.yaml", metaYaml, "quantal", revision)
}

// AddMetricsCharm clones a testing charm, replaces its metrics declaration with the
// given YAML string and adds it to the state, using the given revision.
func (s *ConnWithWallClockSuite) AddMetricsCharm(c *gc.C, name, metricsYaml string, revision int) *state.Charm {
	return state.AddCustomCharm(c, s.State, name, "metrics.yaml", metricsYaml, "quantal", revision)
}

// NewStateForModelNamed returns an new model with the given modelName, which
// has a unique UUID, and does not need to be closed when the test completes.
func (s *ConnWithWallClockSuite) NewStateForModelNamed(c *gc.C, modelName string) *state.State {
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": modelName,
		"uuid": utils.MustNewUUID().String(),
	})
	otherOwner := names.NewLocalUserTag("test-admin")
	_, otherState, err := s.Controller.NewModel(state.ModelArgs{
		Type:                    state.ModelTypeIAAS,
		CloudName:               "dummy",
		CloudRegion:             "dummy-region",
		Config:                  cfg,
		Owner:                   otherOwner,
		StorageProviderRegistry: storage.StaticProviderRegistry{},
	})

	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { otherState.Close() })
	return otherState
}
