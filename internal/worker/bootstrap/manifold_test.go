// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/dependency"
	dependencytesting "github.com/juju/worker/v5/dependency/testing"
	"go.uber.org/goleak"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
)

type manifoldSuite struct {
	baseSuite
}

func TestManifoldSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.DataDir = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.ObjectStoreName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.DomainServicesName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.BootstrapGateName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.HTTPClientName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.APIPort = 0
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.AgentPassword = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Clock = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.RequiresBootstrap = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.AgentBinaryUploader = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg.ControllerCharmDeployer = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.PopulateControllerCharm = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.ControllerUnitPassword = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.BootstrapAddressFinderGetter = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.AgentFinalizer = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		ObjectStoreName:     "object-store",
		BootstrapGateName:   "bootstrap-gate",
		DomainServicesName:  "domain-services",
		ProviderFactoryName: "provider-factory",
		HTTPClientName:      "http-client",
		DataDir:             s.dataDir,
		APIPort:             42,
		AgentPassword:       "password",
		Logger:              s.logger,
		Clock:               clock.WallClock,
		AgentBinaryUploader: func(context.Context, string, AgentBinaryStore, objectstore.ObjectStore, logger.Logger) (func(), error) {
			return func() {}, nil
		},
		ControllerCharmDeployer: func(context.Context, ControllerCharmDeployerConfig) (bootstrap.ControllerCharmDeployer, error) {
			return nil, nil
		},
		PopulateControllerCharm: func(context.Context, bootstrap.ControllerCharmDeployer) error {
			return nil
		},
		ControllerUnitPassword: func(context.Context) (string, error) {
			return "", nil
		},
		BootstrapAddressFinderGetter: func(providerFactory providertracker.ProviderFactory, namespace string) BootstrapAddressFinderFunc {
			return nil
		},
		RequiresBootstrap: func(context.Context, FlagService) (bool, error) {
			return false, nil
		},
		AgentFinalizer: func(ctx context.Context, aps AgentPasswordService, ms MachineService, sip instancecfg.StateInitializationParams, password string) error {
			return nil
		},
		StatusHistory: s.statusHistory,
	}
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"object-store":    s.objectStoreGetter,
		"bootstrap-gate":  s.bootstrapUnlocker,
		"http-client":     s.httpClientGetter,
		"domain-services": s.domainServices,
	}
	return dependencytesting.StubGetter(resources)
}

var expectedInputs = []string{
	"object-store",
	"bootstrap-gate",
	"domain-services",
	"http-client",
	"provider-factory",
}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, tc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStartAlreadyBootstrapped(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGateUnlock()

	_, err := Manifold(s.getConfig()).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIs, dependency.ErrUninstall)
}
