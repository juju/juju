//go:build dqlite && linux

// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/proxy"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/packaging/commands"
	pacconfig "github.com/juju/juju/internal/packaging/config"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/gate"
)

var _ API = domainProxySource{}

type controllerManifoldSuite struct {
	config ControllerManifoldConfig

	controllerDomainServices *MockControllerDomainServices
	domainServices           *MockDomainServices
	modelService             *MockModelService
	modelConfigService       *MockModelConfigService
	controllerNodeService    *MockControllerNodeService
	proxyReadyUnlocker       *MockUnlocker
	depGetter                *MockGetter
}

func TestControllerManifoldSuite(t *testing.T) {
	tc.Run(t, &controllerManifoldSuite{})
}

func (s *controllerManifoldSuite) SetUpTest(c *tc.C) {
	s.config = s.newConfig(c)
}

func (s *controllerManifoldSuite) setupMocks(c *tc.C) *gomock.Controller {
	controller := gomock.NewController(c)

	s.controllerDomainServices = NewMockControllerDomainServices(controller)
	s.domainServices = NewMockDomainServices(controller)
	s.modelService = NewMockModelService(controller)
	s.modelConfigService = NewMockModelConfigService(controller)
	s.controllerNodeService = NewMockControllerNodeService(controller)
	s.proxyReadyUnlocker = NewMockUnlocker(controller)
	s.depGetter = NewMockGetter(controller)

	c.Cleanup(func() {
		s.controllerDomainServices = nil
		s.domainServices = nil
		s.modelService = nil
		s.modelConfigService = nil
		s.controllerNodeService = nil
		s.proxyReadyUnlocker = nil
		s.depGetter = nil
	})

	return controller
}

func (s *controllerManifoldSuite) newConfig(c *tc.C) ControllerManifoldConfig {
	return ControllerManifoldConfig{
		DomainServicesName: "domain-services",
		ProxyReadyGateName: "proxy-ready-gate",
		Logger:             logger.GetLogger("test"),
		WorkerFunc: func(cfg Config) (worker.Worker, error) {
			return &dummyWorker{config: cfg}, nil
		},
		GetControllerDomainServices: func(dependency.Getter, string) (ControllerDomainServices, error) {
			c.Fatalf("unexpected GetControllerDomainServices call")
			return nil, nil
		},
		GetDomainServices: func(context.Context, dependency.Getter, string, coremodel.UUID) (DomainServices, error) {
			c.Fatalf("unexpected GetDomainServices call")
			return nil, nil
		},
		SupportLegacyValues: true,
		ExternalUpdate:      makeUpdateFunc("external"),
		InProcessUpdate:     makeUpdateFunc("in-process"),
	}
}

func (s *controllerManifoldSuite) manifold() dependency.Manifold {
	return ControllerManifold(s.config)
}

func (s *controllerManifoldSuite) TestInputs(c *tc.C) {
	c.Check(s.manifold().Inputs, tc.DeepEquals, []string{"domain-services", "proxy-ready-gate"})
}

func (s *controllerManifoldSuite) TestValidate(c *tc.C) {
	c.Check(s.config.Validate(), tc.ErrorIsNil)

	s.config.DomainServicesName = ""
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)

	s.config = s.newConfig(c)
	s.config.ProxyReadyGateName = ""
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)

	s.config = s.newConfig(c)
	s.config.WorkerFunc = nil
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)

	s.config = s.newConfig(c)
	s.config.GetControllerDomainServices = nil
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)

	s.config = s.newConfig(c)
	s.config.GetDomainServices = nil
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)

	s.config = s.newConfig(c)
	s.config.ExternalUpdate = nil
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)

	s.config = s.newConfig(c)
	s.config.InProcessUpdate = nil
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)

	s.config = s.newConfig(c)
	s.config.Logger = nil
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *controllerManifoldSuite) TestStartSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Patch the apt proxy config file to a temp directory so tests don't
	// write to the real filesystem.
	pacconfig.AptProxyConfigFile = filepath.Join(c.MkDir(), "juju-apt-proxy")

	s.depGetter.EXPECT().Get("proxy-ready-gate", gomock.Any()).DoAndReturn(
		func(_ string, out any) error {
			ptr, ok := out.(*gate.Unlocker)
			c.Assert(ok, tc.IsTrue)
			*ptr = s.proxyReadyUnlocker
			return nil
		},
	)

	modelUUID := coremodel.UUID("controller-model-uuid")
	s.controllerDomainServices.EXPECT().Model().Return(s.modelService)
	s.modelService.EXPECT().ControllerModel(c.Context()).Return(coremodel.Model{UUID: modelUUID}, nil)
	s.controllerDomainServices.EXPECT().ControllerNode().Return(s.controllerNodeService)
	s.domainServices.EXPECT().Config().Return(s.modelConfigService)

	modelConfig := newProxyModelConfig(c)
	s.modelConfigService.EXPECT().ModelConfig(c.Context()).Return(modelConfig, nil)
	s.controllerNodeService.EXPECT().GetAllNoProxyAPIAddressesForAgents(c.Context()).Return("10.0.0.1,10.0.0.2", nil)
	s.proxyReadyUnlocker.EXPECT().Unlock()

	var initialSettings proxy.Settings
	var workerConfig Config
	s.config.InProcessUpdate = func(settings proxy.Settings) error {
		initialSettings = settings
		return nil
	}
	s.config.WorkerFunc = func(cfg Config) (worker.Worker, error) {
		workerConfig = cfg
		return &dummyWorker{config: cfg}, nil
	}

	s.config.GetControllerDomainServices = func(getter dependency.Getter, name string) (ControllerDomainServices, error) {
		c.Check(getter, tc.Equals, s.depGetter)
		c.Check(name, tc.Equals, "domain-services")
		return s.controllerDomainServices, nil
	}
	s.config.GetDomainServices = func(ctx context.Context, getter dependency.Getter, name string, gotUUID coremodel.UUID) (DomainServices, error) {
		c.Check(ctx, tc.Equals, c.Context())
		c.Check(getter, tc.Equals, s.depGetter)
		c.Check(name, tc.Equals, "domain-services")
		c.Check(gotUUID, tc.Equals, modelUUID)
		return s.domainServices, nil
	}

	_, err := s.manifold().Start(c.Context(), s.depGetter)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(workerConfig.SystemdFiles, tc.DeepEquals, defaultSystemdFiles)
	c.Check(workerConfig.EnvFiles, tc.DeepEquals, defaultEnvFiles)
	c.Check(workerConfig.SupportLegacyValues, tc.IsTrue)
	c.Check(workerConfig.API, tc.NotNil)
	c.Check(workerConfig.ExternalUpdate(proxy.Settings{}), tc.ErrorMatches, "external")
	c.Check(initialSettings, tc.Equals, proxy.Settings{
		Http:        "http://proxy.internal:3128",
		Https:       "http://proxy.internal:3128",
		NoProxy:     "localhost",
		AutoNoProxy: "10.0.0.1,10.0.0.2",
	})
	aptProxyConfig, err := os.ReadFile(pacconfig.AptProxyConfigFile)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(aptProxyConfig), tc.Equals,
		commands.NewAptPackageCommander().ProxyConfigContents(modelConfig.AptProxySettings())+"\n")
}

func newProxyModelConfig(c *tc.C) *config.Config {
	attrs := coretesting.FakeConfig()
	attrs[config.JujuHTTPProxyKey] = "http://proxy.internal:3128"
	attrs[config.JujuHTTPSProxyKey] = "http://proxy.internal:3128"
	attrs[config.JujuNoProxyKey] = "localhost"
	modelConfig, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, tc.ErrorIsNil)
	return modelConfig
}
