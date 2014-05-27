// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"labix.org/v2/mgo"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
)

// TestPackage integrates the tests into gotest.
func TestPackage(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

// ConnSuite provides the infrastructure for all other
// test suites (StateSuite, CharmSuite, MachineSuite, etc).
type ConnSuite struct {
	testing.MgoSuite
	testing.BaseSuite
	annotations  *mgo.Collection
	charms       *mgo.Collection
	machines     *mgo.Collection
	relations    *mgo.Collection
	services     *mgo.Collection
	units        *mgo.Collection
	stateServers *mgo.Collection
	State        *state.State
	policy       mockPolicy
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
	cs.BaseSuite.SetUpTest(c)
	cs.MgoSuite.SetUpTest(c)
	cs.policy = mockPolicy{}
	cs.State = state.TestingInitialize(c, nil, &cs.policy)
	cs.annotations = cs.MgoSuite.Session.DB("juju").C("annotations")
	cs.charms = cs.MgoSuite.Session.DB("juju").C("charms")
	cs.machines = cs.MgoSuite.Session.DB("juju").C("machines")
	cs.relations = cs.MgoSuite.Session.DB("juju").C("relations")
	cs.services = cs.MgoSuite.Session.DB("juju").C("services")
	cs.units = cs.MgoSuite.Session.DB("juju").C("units")
	cs.stateServers = cs.MgoSuite.Session.DB("juju").C("stateServers")
	cs.State.AddUser(state.AdminUser, "pass")
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

func (s *ConnSuite) AddTestingServiceWithNetworks(c *gc.C, name string, ch *state.Charm, includeNetworks, excludeNetworks []string) *state.Service {
	return state.AddTestingServiceWithNetworks(c, s.State, name, ch, includeNetworks, excludeNetworks)
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

// AddMetaCharm clones a testing charm, replaces its metadata with the
// given YAML string and adds it to the state, using the given revision.
func (s *ConnSuite) AddMetaCharm(c *gc.C, name, metaYaml string, revsion int) *state.Charm {
	return state.AddCustomCharm(c, s.State, name, "metadata.yaml", metaYaml, "quantal", revsion)
}

type mockPolicy struct {
	getPrechecker           func(*config.Config) (state.Prechecker, error)
	getConfigValidator      func(string) (state.ConfigValidator, error)
	getEnvironCapability    func(*config.Config) (state.EnvironCapability, error)
	getConstraintsValidator func(*config.Config) (constraints.Validator, error)
	getInstanceDistributor  func(*config.Config) (state.InstanceDistributor, error)
}

func (p *mockPolicy) Prechecker(cfg *config.Config) (state.Prechecker, error) {
	if p.getPrechecker != nil {
		return p.getPrechecker(cfg)
	}
	return nil, errors.NotImplementedf("Prechecker")
}

func (p *mockPolicy) ConfigValidator(providerType string) (state.ConfigValidator, error) {
	if p.getConfigValidator != nil {
		return p.getConfigValidator(providerType)
	}
	return nil, errors.NotImplementedf("ConfigValidator")
}

func (p *mockPolicy) EnvironCapability(cfg *config.Config) (state.EnvironCapability, error) {
	if p.getEnvironCapability != nil {
		return p.getEnvironCapability(cfg)
	}
	return nil, errors.NotImplementedf("EnvironCapability")
}

func (p *mockPolicy) ConstraintsValidator(cfg *config.Config) (constraints.Validator, error) {
	if p.getConstraintsValidator != nil {
		return p.getConstraintsValidator(cfg)
	}
	return nil, errors.NewNotImplemented(nil, "ConstraintsValidator")
}

func (p *mockPolicy) InstanceDistributor(cfg *config.Config) (state.InstanceDistributor, error) {
	if p.getInstanceDistributor != nil {
		return p.getInstanceDistributor(cfg)
	}
	return nil, errors.NewNotImplemented(nil, "InstanceDistributor")
}
