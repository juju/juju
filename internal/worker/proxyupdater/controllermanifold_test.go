// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"context"
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
	coretesting "github.com/juju/juju/internal/testing"
)

type controllerManifoldSuite struct {
	config ControllerManifoldConfig
}

func TestControllerManifoldSuite(t *testing.T) {
	tc.Run(t, &controllerManifoldSuite{})
}

func (s *controllerManifoldSuite) SetUpTest(c *tc.C) {
	s.config = ControllerManifoldConfig{
		DomainServicesName: "domain-services",
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
	c.Check(s.manifold().Inputs, tc.DeepEquals, []string{"domain-services"})
}

func (s *controllerManifoldSuite) TestValidate(c *tc.C) {
	c.Check(s.config.Validate(), tc.ErrorIsNil)

	s.config.DomainServicesName = ""
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)

	s.SetUpTest(c)
	s.config.WorkerFunc = nil
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)

	s.SetUpTest(c)
	s.config.GetControllerDomainServices = nil
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)

	s.SetUpTest(c)
	s.config.GetDomainServices = nil
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)

	s.SetUpTest(c)
	s.config.ExternalUpdate = nil
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)

	s.SetUpTest(c)
	s.config.InProcessUpdate = nil
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)

	s.SetUpTest(c)
	s.config.Logger = nil
	c.Check(s.config.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *controllerManifoldSuite) TestStartSuccess(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	controllerDomainServices := NewMockControllerDomainServices(ctrl)
	domainServices := NewMockDomainServices(ctrl)
	modelService := NewMockModelService(ctrl)
	modelConfigService := NewMockModelConfigService(ctrl)
	controllerNodeService := NewMockControllerNodeService(ctrl)

	modelUUID := coremodel.UUID("controller-model-uuid")
	controllerDomainServices.EXPECT().Model().Return(modelService)
	modelService.EXPECT().ControllerModel(c.Context()).Return(coremodel.Model{UUID: modelUUID}, nil)
	controllerDomainServices.EXPECT().ControllerNode().Return(controllerNodeService)
	domainServices.EXPECT().Config().Return(modelConfigService)
	modelConfig := newProxyModelConfig(c)
	modelConfigService.EXPECT().ModelConfig(c.Context()).Return(modelConfig, nil)
	controllerNodeService.EXPECT().GetAllNoProxyAPIAddressesForAgents(c.Context()).Return("10.0.0.1,10.0.0.2", nil)

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
		c.Check(getter, tc.IsNil)
		c.Check(name, tc.Equals, "domain-services")
		return controllerDomainServices, nil
	}
	s.config.GetDomainServices = func(ctx context.Context, getter dependency.Getter, name string, gotUUID coremodel.UUID) (DomainServices, error) {
		c.Check(ctx, tc.Equals, c.Context())
		c.Check(getter, tc.IsNil)
		c.Check(name, tc.Equals, "domain-services")
		c.Check(gotUUID, tc.Equals, modelUUID)
		return domainServices, nil
	}

	w, err := s.manifold().Start(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	var ready WaitReady
	err = s.manifold().Output(w, &ready)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ready.WaitReady(), tc.IsTrue)

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
