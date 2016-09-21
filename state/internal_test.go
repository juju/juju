// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&internalStateSuite{})

// internalStateSuite manages a *State instance for tests in the state
// package (i.e. internal tests) that need it. It is similar to
// state.testing.StateSuite but is duplicated to avoid cyclic imports.
type internalStateSuite struct {
	jujutesting.MgoSuite
	testing.BaseSuite
	state *State
	owner names.UserTag
}

func (s *internalStateSuite) SetUpSuite(c *gc.C) {
	s.MgoSuite.SetUpSuite(c)
	s.BaseSuite.SetUpSuite(c)
}

func (s *internalStateSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
}

func (s *internalStateSuite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)
	s.BaseSuite.SetUpTest(c)

	s.owner = names.NewLocalUserTag("test-admin")
	// Copied from NewMongoInfo (due to import loops).
	info := &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  []string{jujutesting.MgoServer.Addr()},
			CACert: testing.CACert,
		},
	}
	modelCfg := testing.ModelConfig(c)
	controllerCfg := testing.FakeControllerConfig()
	st, err := Initialize(InitializeParams{
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: ModelArgs{
			CloudName:               "dummy",
			CloudRegion:             "dummy-region",
			Owner:                   s.owner,
			Config:                  modelCfg,
			StorageProviderRegistry: provider.CommonStorageProviders(),
		},
		CloudName: "dummy",
		Cloud: cloud.Cloud{
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
			Regions: []cloud.Region{
				cloud.Region{
					Name: "dummy-region",
				},
			},
		},
		MongoInfo:     info,
		MongoDialOpts: mongotest.DialOpts(),
		NewPolicy: func(*State) Policy {
			return internalStatePolicy{}
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.state = st
	s.AddCleanup(func(*gc.C) { s.state.Close() })
}

func (s *internalStateSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
}

type internalStatePolicy struct{}

func (internalStatePolicy) Prechecker() (Prechecker, error) {
	return nil, errors.NotImplementedf("Prechecker")
}

func (internalStatePolicy) ConfigValidator() (config.Validator, error) {
	return nil, errors.NotImplementedf("ConfigValidator")
}

func (internalStatePolicy) ConstraintsValidator() (constraints.Validator, error) {
	return nil, errors.NotImplementedf("ConstraintsValidator")
}

func (internalStatePolicy) InstanceDistributor() (instance.Distributor, error) {
	return nil, errors.NotImplementedf("InstanceDistributor")
}

func (internalStatePolicy) StorageProviderRegistry() (storage.ProviderRegistry, error) {
	return provider.CommonStorageProviders(), nil
}

func (internalStatePolicy) ProviderConfigSchemaSource() (config.ConfigSchemaSource, error) {
	return nil, errors.NotImplementedf("ConfigSchemaSource")
}
