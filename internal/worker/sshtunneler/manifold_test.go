// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"context"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"

	coremodel "github.com/juju/juju/core/model"
	domainssh "github.com/juju/juju/domain/ssh"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/testhelpers"
)

type manifoldSuite struct {
	testhelpers.IsolationSuite

	controllerNodeService *MockControllerNodeService
	sshModelService       *MockSSHModelService
	machineService        *MockMachineService
}

func TestManifoldSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &manifoldSuite{})
	})
}

func (s *manifoldSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerNodeService = NewMockControllerNodeService(ctrl)
	s.sshModelService = NewMockSSHModelService(ctrl)
	s.machineService = NewMockMachineService(ctrl)

	return ctrl
}

func (s *manifoldSuite) newManifoldConfig(c *tc.C, modifier func(cfg *ManifoldConfig)) *ManifoldConfig {
	cfg := &ManifoldConfig{
		DomainServicesName: "domain-services",
		Clock:              clock.WallClock,
		GetControllerNodeService: func(getter dependency.Getter, name string) (ControllerNodeService, error) {
			return s.controllerNodeService, nil
		},
		GetDomainServicesGetter: func(dependency.Getter, string) (services.DomainServicesGetter, error) {
			return stubServicesGetter{}, nil
		},
		GetSSHService: func(context.Context, services.DomainServicesGetter, coremodel.UUID) (SSHModelService, error) {
			return s.sshModelService, nil
		},
		GetMachineService: func(context.Context, services.DomainServicesGetter, coremodel.UUID) (MachineService, error) {
			return s.machineService, nil
		},
	}

	modifier(cfg)

	return cfg
}

func (s *manifoldSuite) TestConfigValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Valid config.
	cfg := s.newManifoldConfig(c, func(cfg *ManifoldConfig) {})
	c.Assert(cfg.Validate(), tc.IsNil)

	// Missing DomainServicesName.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.DomainServicesName = ""
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing Clock.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.Clock = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing GetControllerNodeService.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.GetControllerNodeService = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing GetDomainServicesGetter.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.GetDomainServicesGetter = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing GetSSHService.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.GetSSHService = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing GetMachineService.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.GetMachineService = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)
}

func (s *manifoldSuite) TestManifoldInputs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	manifold := Manifold(*s.newManifoldConfig(c, func(cfg *ManifoldConfig) {}))
	c.Assert(manifold.Inputs, tc.DeepEquals, []string{"domain-services"})
}

func (s *manifoldSuite) TestManifoldStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	manifold := Manifold(*s.newManifoldConfig(c, func(cfg *ManifoldConfig) {}))

	result, err := manifold.Start(
		c.Context(),
		dt.StubGetter(map[string]any{
			"domain-services": nil,
		}),
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.NotNil)
	workertest.CleanKill(c, result)
}

func (s *manifoldSuite) TestManifoldStartValidatesConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.GetSSHService = nil
	})
	manifold := Manifold(*cfg)

	_, err := manifold.Start(
		c.Context(),
		dt.StubGetter(map[string]any{
			"domain-services": nil,
		}),
	)
	c.Assert(errors.Is(err, errors.NotValid), tc.IsTrue)
}

func (s *manifoldSuite) TestGetSSHServiceUsesRequestModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := coremodel.UUID("8419cd78-4993-4c3a-928e-c646226beeee")

	var capturedUUID coremodel.UUID
	getter := func(_ context.Context, _ services.DomainServicesGetter, uuid coremodel.UUID) (SSHModelService, error) {
		capturedUUID = uuid
		return s.sshModelService, nil
	}

	// Simulate what tunnelerStateAdapter does: route by the provided model
	// UUID to the correct model-scoped service.
	req := domainssh.SSHConnRequest{
		TunnelID:    "some-tunnel",
		MachineName: "0",
	}

	s.sshModelService.EXPECT().InsertSSHConnRequest(gomock.Any(), req).Return(nil)

	adapter := &connRequestStateAdapter{
		domainServicesGetter: stubServicesGetter{},
		getSSHService:        getter,
	}
	err := adapter.InsertSSHConnRequest(c.Context(), modelUUID, req)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(capturedUUID, tc.Equals, modelUUID)
}
