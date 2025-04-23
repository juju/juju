// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/dependency"
	dependencytesting "github.com/juju/worker/v4/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/bootstrap"
)

type manifoldSuite struct {
	baseSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.AgentName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.StateName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.ObjectStoreName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.DomainServicesName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.BootstrapGateName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.StorageRegistryName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.HTTPClientName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Clock = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.RequiresBootstrap = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.AgentBinaryUploader = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg.ControllerCharmDeployer = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.PopulateControllerCharm = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.ControllerUnitPassword = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		AgentName:           "agent",
		ObjectStoreName:     "object-store",
		StateName:           "state",
		BootstrapGateName:   "bootstrap-gate",
		DomainServicesName:  "domain-services",
		ProviderFactoryName: "provider-factory",
		HTTPClientName:      "http-client",
		StorageRegistryName: "storage-registry",
		Logger:              s.logger,
		Clock:               clock.WallClock,
		AgentBinaryUploader: func(context.Context, string, BinaryAgentStorageService, AgentBinaryStore, objectstore.ObjectStore, logger.Logger) (func(), error) {
			return func() {}, nil
		},
		ControllerCharmDeployer: func(ControllerCharmDeployerConfig) (bootstrap.ControllerCharmDeployer, error) {
			return nil, nil
		},
		PopulateControllerCharm: func(context.Context, bootstrap.ControllerCharmDeployer) error {
			return nil
		},
		ControllerUnitPassword: func(context.Context) (string, error) {
			return "", nil
		},
		RequiresBootstrap: func(context.Context, FlagService) (bool, error) {
			return false, nil
		},
	}
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"agent":            s.agent,
		"state":            s.stateTracker,
		"object-store":     s.objectStoreGetter,
		"bootstrap-gate":   s.bootstrapUnlocker,
		"http-client":      s.httpClientGetter,
		"domain-services":  s.domainServices,
		"storage-registry": s.storageRegistryGetter,
	}
	return dependencytesting.StubGetter(resources)
}

var expectedInputs = []string{
	"agent",
	"state",
	"object-store",
	"bootstrap-gate",
	"domain-services",
	"http-client",
	"provider-factory",
	"storage-registry",
}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStartAlreadyBootstrapped(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGateUnlock()
	s.expectAgentConfig()

	_, err := Manifold(s.getConfig()).Start(context.Background(), s.newGetter())
	c.Assert(err, jc.ErrorIs, dependency.ErrUninstall)
}
