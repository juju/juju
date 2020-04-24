// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
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
	controller *Controller
	pool       *StatePool
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
	s.MgoSuite.SetUpTest(c)
	s.BaseSuite.SetUpTest(c)

	s.owner = names.NewLocalUserTag("test-admin")
	modelCfg := testing.ModelConfig(c)
	controllerCfg := testing.FakeControllerConfig()
	ctlr, err := Initialize(InitializeParams{
		Clock:            testclock.NewClock(testing.NonZeroTime()),
		ControllerConfig: controllerCfg,
		ControllerModelArgs: ModelArgs{
			Type:        ModelTypeIAAS,
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
				{
					Name: "dummy-region",
				},
			},
		},
		MongoSession:  s.Session,
		AdminPassword: "dummy-secret",
		NewPolicy: func(*State) Policy {
			return internalStatePolicy{}
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.controller = ctlr
	s.pool = ctlr.StatePool()
	s.state = ctlr.SystemState()
	s.AddCleanup(func(*gc.C) {
		// Controller closes pool, pool closes all states.
		s.controller.Close()
	})
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
	_, st, err := s.controller.NewModel(ModelArgs{
		Type:        ModelTypeIAAS,
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

func (internalStatePolicy) Prechecker() (environs.InstancePrechecker, error) {
	return nil, errors.NotImplementedf("Prechecker")
}

func (internalStatePolicy) ConfigValidator() (config.Validator, error) {
	return nil, errors.NotImplementedf("ConfigValidator")
}

func (internalStatePolicy) ConstraintsValidator(context.ProviderCallContext) (constraints.Validator, error) {
	return nil, errors.NotImplementedf("ConstraintsValidator")
}

func (internalStatePolicy) InstanceDistributor() (context.Distributor, error) {
	return nil, errors.NotImplementedf("InstanceDistributor")
}

func (internalStatePolicy) StorageProviderRegistry() (storage.ProviderRegistry, error) {
	return storage.ChainedProviderRegistry{
		dummy.StorageProviders(),
		provider.CommonStorageProviders(),
	}, nil
}

func (internalStatePolicy) ProviderConfigSchemaSource(cloudName string) (config.ConfigSchemaSource, error) {
	return nil, errors.NotImplementedf("ConfigSchemaSource")
}
