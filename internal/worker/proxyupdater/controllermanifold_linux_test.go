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
)

type controllerManifoldSuite struct {
	config ControllerManifoldConfig
}

func TestControllerManifoldSuite(t *testing.T) {
	tc.Run(t, &controllerManifoldSuite{})
}

func (s *controllerManifoldSuite) SetUpTest(c *tc.C) {
	s.config = s.newConfig(c)
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
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	oldAptProxyConfigFile := pacconfig.AptProxyConfigFile
	pacconfig.AptProxyConfigFile = filepath.Join(c.MkDir(), "juju-apt-proxy")
	defer func() { pacconfig.AptProxyConfigFile = oldAptProxyConfigFile }()

	controllerDomainServices := NewMockControllerDomainServices(ctrl)
	domainServices := NewMockDomainServices(ctrl)
	modelService := NewMockModelService(ctrl)
	modelConfigService := NewMockModelConfigService(ctrl)
	controllerNodeService := NewMockControllerNodeService(ctrl)
	proxyReadyUnlocker := NewMockProxyReadyUnlocker(ctrl)
	depGetter := NewMockGetter(ctrl)
	depGetter.EXPECT().Get("proxy-ready-gate", gomock.Any()).DoAndReturn(
		func(_ string, out any) error {
			ptr, ok := out.(*ProxyReadyUnlocker)
			c.Assert(ok, tc.IsTrue)
			*ptr = proxyReadyUnlocker
			return nil
		},
	)

	modelUUID := coremodel.UUID("controller-model-uuid")
	controllerDomainServices.EXPECT().Model().Return(modelService)
	modelService.EXPECT().ControllerModel(c.Context()).Return(coremodel.Model{UUID: modelUUID}, nil)
	controllerDomainServices.EXPECT().ControllerNode().Return(controllerNodeService)
	domainServices.EXPECT().Config().Return(modelConfigService)
	modelConfig := newProxyModelConfig(c)
	modelConfigService.EXPECT().ModelConfig(c.Context()).Return(modelConfig, nil)
	controllerNodeService.EXPECT().GetAllNoProxyAPIAddressesForAgents(c.Context()).Return("10.0.0.1,10.0.0.2", nil)
	proxyReadyUnlocker.EXPECT().Unlock()

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
		c.Check(getter, tc.Equals, depGetter)
		c.Check(name, tc.Equals, "domain-services")
		return controllerDomainServices, nil
	}
	s.config.GetDomainServices = func(ctx context.Context, getter dependency.Getter, name string, gotUUID coremodel.UUID) (DomainServices, error) {
		c.Check(ctx, tc.Equals, c.Context())
		c.Check(getter, tc.Equals, depGetter)
		c.Check(name, tc.Equals, "domain-services")
		c.Check(gotUUID, tc.Equals, modelUUID)
		return domainServices, nil
	}

	_, err := s.manifold().Start(c.Context(), depGetter)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(workerConfig.SystemdFiles, tc.DeepEquals, []string{"/etc/juju-proxy-systemd.conf"})
	c.Check(workerConfig.EnvFiles, tc.DeepEquals, []string{"/etc/juju-proxy.conf"})
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
