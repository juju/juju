// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/mongotest"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/dummy"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&internalStateSuite{})

// internalStateSuite manages a *State instance for tests in the state
// package (i.e. internal tests) that need it. It is similar to
// state.testing.StateSuite but is duplicated to avoid cyclic imports.
type internalStateSuite struct {
	jujutesting.MgoSuite
	testing.BaseSuite
	state      *State
	owner      names.UserTag
	modelCount int
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
	s.SetInitialFeatureFlags(feature.CrossModelRelations)
	s.MgoSuite.SetUpTest(c)
	s.BaseSuite.SetUpTest(c)

	s.owner = names.NewLocalUserTag("test-admin")
	// Copied from NewMongoInfo (due to import loops).
	info := &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:      []string{jujutesting.MgoServer.Addr()},
			CACert:     testing.CACert,
			DisableTLS: !jujutesting.MgoServer.SSLEnabled(),
		},
	}
	modelCfg := testing.ModelConfig(c)
	controllerCfg := testing.FakeControllerConfig()
	st, err := Initialize(InitializeParams{
		Clock:            clock.WallClock,
		ControllerConfig: controllerCfg,
		ControllerModelArgs: ModelArgs{
			CloudName:   "dummy",
			CloudRegion: "dummy-region",
			Owner:       s.owner,
			Config:      modelCfg,
			StorageProviderRegistry: storage.ChainedProviderRegistry{
				dummy.StorageProviders(),
				provider.CommonStorageProviders(),
			},
		},
		Cloud: cloud.Cloud{
			Name:      "dummy",
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

func (s *internalStateSuite) newState(c *gc.C) *State {
	s.modelCount++
	cfg := testing.CustomModelConfig(c, testing.Attrs{
		"name": fmt.Sprintf("testmodel%d", s.modelCount),
		"uuid": utils.MustNewUUID().String(),
	})
	_, st, err := s.state.NewModel(ModelArgs{
		CloudName:   "dummy",
		CloudRegion: "dummy-region",
		Config:      cfg,
		Owner:       s.owner,
		StorageProviderRegistry: storage.ChainedProviderRegistry{
			dummy.StorageProviders(),
			provider.CommonStorageProviders(),
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { st.Close() })
	return st
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
	return storage.ChainedProviderRegistry{
		dummy.StorageProviders(),
		provider.CommonStorageProviders(),
	}, nil
}

func (internalStatePolicy) ProviderConfigSchemaSource() (config.ConfigSchemaSource, error) {
	return nil, errors.NotImplementedf("ConfigSchemaSource")
}
