// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/mgo/v3"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	dummystorage "github.com/juju/juju/storage/provider/dummy"
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
	applications *mgo.Collection
	units        *mgo.Collection
	controllers  *mgo.Collection
	sshconnreqs  *mgo.Collection
	policy       statetesting.MockPolicy
	modelTag     names.ModelTag
}

func (s *ConnSuite) SetUpTest(c *gc.C) {
	s.policy = statetesting.MockPolicy{
		GetStorageProviderRegistry: func() (storage.ProviderRegistry, error) {
			return storage.ChainedProviderRegistry{
				dummystorage.StorageProviders(),
				provider.CommonStorageProviders(),
			}, nil
		},
	}
	s.StateSuite.NewPolicy = func(*state.State) state.Policy {
		return &s.policy
	}

	s.StateSuite.SetUpTest(c)

	s.modelTag = s.Model.ModelTag()

	jujuDB := s.MgoSuite.Session.DB("juju")
	s.annotations = jujuDB.C("annotations")
	s.charms = jujuDB.C("charms")
	s.machines = jujuDB.C("machines")
	s.instanceData = jujuDB.C("instanceData")
	s.relations = jujuDB.C("relations")
	s.applications = jujuDB.C("applications")
	s.units = jujuDB.C("units")
	s.controllers = jujuDB.C("controllers")
	s.sshconnreqs = jujuDB.C(state.SSHConnRequestsC)
}

func (s *ConnSuite) AddTestingCharm(c *gc.C, name string) *state.Charm {
	return state.AddTestingCharm(c, s.State, name)
}

func (s *ConnSuite) AddTestingCharmWithSeries(c *gc.C, name string, series string) *state.Charm {
	return state.AddTestingCharmWithSeries(c, s.State, name, series)
}

func (s *ConnSuite) AddTestingApplication(c *gc.C, name string, ch *state.Charm) *state.Application {
	return state.AddTestingApplication(c, s.State, name, ch)
}

func (s *ConnSuite) AddTestingApplicationForBase(c *gc.C, base state.Base, name string, ch *state.Charm) *state.Application {
	return state.AddTestingApplicationForBase(c, s.State, base, name, ch)
}

func (s *ConnSuite) AddTestingApplicationWithNumUnits(c *gc.C, numUnits int, name string, ch *state.Charm) *state.Application {
	return state.AddTestingApplicationWithNumUnits(c, s.State, numUnits, name, ch)
}

func (s *ConnSuite) AddTestingApplicationWithStorage(c *gc.C, name string, ch *state.Charm, storage map[string]state.StorageConstraints) *state.Application {
	return state.AddTestingApplicationWithStorage(c, s.State, name, ch, storage)
}

func (s *ConnSuite) AddTestingApplicationWithDevices(c *gc.C, name string, ch *state.Charm, devs map[string]state.DeviceConstraints) *state.Application {
	return state.AddTestingApplicationWithDevices(c, s.State, name, ch, devs)
}

func (s *ConnSuite) AddTestingApplicationWithBindings(c *gc.C, name string, ch *state.Charm, bindings map[string]string) *state.Application {
	return state.AddTestingApplicationWithBindings(c, s.State, name, ch, bindings)
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

// AddManifestCharm clones a testing charm, replaces its manifest schema with
// the given YAML, and adds it to the state, using the given revision.
func (s *ConnSuite) AddManifestCharm(c *gc.C, name, manifestYaml string, revision int) *state.Charm {
	return state.AddCustomCharm(c, s.State, name, "manifest.yaml", manifestYaml, "quantal", revision)
}

// AddLXDProfileCharm clones a testing charm, replaces its lxd profile config with
// the given YAML, and adds it to the state, using the given revision.
func (s *ConnSuite) AddLXDProfileCharm(c *gc.C, name, lxdProfileYaml string, revision int) *state.Charm {
	return state.AddCustomCharm(c, s.State, name, "lxd-profile.yaml", lxdProfileYaml, "quantal", revision)
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

// SetJujuManagementSpace mimics a controller having been configured with a
// management space name. This is the space that constrains the set of
// addresses that agents should use for controller communication.
func (s *ConnSuite) SetJujuManagementSpace(c *gc.C, space string) {
	controllerSettings, err := s.State.ReadSettings(state.ControllersC, "controllerSettings")
	c.Assert(err, jc.ErrorIsNil)
	controllerSettings.Set(controller.JujuManagementSpace, space)
	_, err = controllerSettings.Write()
	c.Assert(err, jc.ErrorIsNil)
}
