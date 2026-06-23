// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"
	"os"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	domainssh "github.com/juju/juju/domain/ssh"
	"github.com/juju/juju/internal/featureflag"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/juju/osenv"
)

type manifoldSuite struct {
	testhelpers.IsolationSuite

	controllerConfigService     *MockControllerConfigService
	controllerSSHHostKeyService ControllerSSHHostKeyService
	sshService                  SSHModelService
}

func TestManifoldSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &manifoldSuite{})
	})
}

func (s *manifoldSuite) SetUpTest(c *tc.C) {
	err := os.Setenv(osenv.JujuFeatureFlagEnvKey, featureflag.SSHJump)
	c.Assert(err, tc.ErrorIsNil)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

func (s *manifoldSuite) TestConfigValidate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Check config as expected.

	cfg := s.newManifoldConfig(c, func(cfg *ManifoldConfig) {})
	c.Assert(cfg.Validate(), tc.IsNil)

	// Entirely missing.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.DomainServicesName = ""
		cfg.NewServerWrapperWorker = nil
		cfg.NewServerWorker = nil
		cfg.GetControllerConfigService = nil
		cfg.GetControllerSSHHostKeyService = nil
		cfg.GetDomainServicesGetter = nil
		cfg.GetSSHService = nil
		cfg.Logger = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing domain services name.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.DomainServicesName = ""
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing NewServerWrapperWorker.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.NewServerWrapperWorker = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing NewServerWorker.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.NewServerWorker = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing GetControllerConfigService.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.GetControllerConfigService = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing GetControllerSSHHostKeyService.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.GetControllerSSHHostKeyService = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing GetDomainServicesGetter.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.GetDomainServicesGetter = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing Logger.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.Logger = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

	// Missing GetSSHService.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.GetSSHService = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), tc.IsTrue)

}

func (s *manifoldSuite) TestManifoldStart(c *tc.C) {
	defer s.setupMocks(c).Finish()
	sshServiceCalled := false

	// Setup the manifold
	manifold := Manifold(ManifoldConfig{
		DomainServicesName:     "domain-services",
		NewServerWrapperWorker: NewServerWrapperWorker,
		NewServerWorker: func(ServerWorkerConfig) (worker.Worker, error) {
			return workertest.NewErrorWorker(nil), nil
		},
		GetControllerConfigService: func(getter dependency.Getter, name string) (ControllerConfigService, error) {
			return s.controllerConfigService, nil
		},
		GetControllerSSHHostKeyService: func(getter dependency.Getter, name string) (ControllerSSHHostKeyService, error) {
			return s.controllerSSHHostKeyService, nil
		},
		GetDomainServicesGetter: func(dependency.Getter, string) (services.DomainServicesGetter, error) {
			return stubDomainServicesGetter{}, nil
		},
		GetSSHService: func(context.Context, services.DomainServicesGetter, model.UUID) (SSHModelService, error) {
			sshServiceCalled = true
			return s.sshService, nil
		},
		Logger: loggertesting.WrapCheckLog(c),
	})

	// Check the inputs are as expected
	c.Assert(manifold.Inputs, tc.DeepEquals, []string{"domain-services"})

	// Start the worker
	result, err := manifold.Start(
		c.Context(),
		dt.StubGetter(map[string]any{}),
	)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, result)

	c.Check(result, tc.NotNil)
	c.Check(sshServiceCalled, tc.IsFalse)
	workertest.CleanKill(c, result)
}

func (s *manifoldSuite) TestSSHServiceVirtualHostKeyUsesRequestModelUUID(c *tc.C) {
	info, err := virtualhostname.NewInfoMachineTarget("8419cd78-4993-4c3a-928e-c646226beeee", "1")
	c.Assert(err, tc.ErrorIsNil)

	var resolvedModelUUID model.UUID
	sshService := sshService{
		controllerSSHHostKeyService: stubSSHService{jumpHostKey: testHostKey},
		domainServicesGetter:        stubDomainServicesGetter{},
		getSSHService: func(_ context.Context, _ services.DomainServicesGetter, modelUUID model.UUID) (SSHModelService, error) {
			resolvedModelUUID = modelUUID
			return stubSSHService{virtualHostKey: testHostKey}, nil
		},
	}

	virtualHostKey, err := sshService.VirtualHostKey(c.Context(), info)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(virtualHostKey, tc.Equals, testHostKey)
	c.Check(resolvedModelUUID, tc.Equals, info.ModelUUID())
}

func (s *manifoldSuite) TestSSHServiceInsertSSHConnRequestUsesRequestModelUUID(c *tc.C) {
	modelUUID := model.UUID("8419cd78-4993-4c3a-928e-c646226beeee")
	req := domainssh.SSHConnRequest{
		TunnelID:  "tunnel-0",
		MachineID: "1",
		ModelUUID: modelUUID,
	}

	var resolvedModelUUID model.UUID
	sshService := sshService{
		controllerSSHHostKeyService: stubSSHService{jumpHostKey: testHostKey},
		domainServicesGetter:        stubDomainServicesGetter{},
		getSSHService: func(_ context.Context, _ services.DomainServicesGetter, modelUUID model.UUID) (SSHModelService, error) {
			resolvedModelUUID = modelUUID
			return stubSSHService{virtualHostKey: testHostKey, insertConnReqErr: nil}, nil
		},
	}

	err := sshService.InsertSSHConnRequest(c.Context(), req)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(resolvedModelUUID, tc.Equals, modelUUID)
}

func (s *manifoldSuite) TestSSHServiceInsertSSHConnRequestCorrectlyCallsInsertConnRequest(c *tc.C) {
	modelUUID := model.UUID("8419cd78-4993-4c3a-928e-c646226beeee")
	req := domainssh.SSHConnRequest{
		TunnelID:  "tunnel-0",
		MachineID: "1",
		ModelUUID: modelUUID,
	}

	sshService := sshService{
		controllerSSHHostKeyService: stubSSHService{jumpHostKey: testHostKey},
		domainServicesGetter:        stubDomainServicesGetter{},
		getSSHService: func(_ context.Context, _ services.DomainServicesGetter, modelUUID model.UUID) (SSHModelService, error) {
			return stubSSHService{insertConnReqErr: errors.New("blah")}, nil
		},
	}

	// Verify this by checking the error is returned from the InsertSSHConnRequest call.
	err := sshService.InsertSSHConnRequest(c.Context(), req)
	c.Assert(err, tc.ErrorMatches, "blah")
}

func (s *manifoldSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	sshService := stubSSHService{jumpHostKey: testHostKey, virtualHostKey: testHostKey}
	s.controllerSSHHostKeyService = sshService
	s.sshService = sshService

	s.controllerConfigService.EXPECT().WatchControllerConfig(gomock.Any()).DoAndReturn(func(context.Context) (watcher.Watcher[[]string], error) {
		return watchertest.NewMockStringsWatcher(make(<-chan []string)), nil
	}).AnyTimes()
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).DoAndReturn(func(context.Context) (controller.Config, error) {
		return controller.Config{
			controller.SSHServerPort:               22,
			controller.SSHMaxConcurrentConnections: 10,
		}, nil
	}).AnyTimes()
	return ctrl
}

func (s *manifoldSuite) newManifoldConfig(c *tc.C, modifier func(cfg *ManifoldConfig)) *ManifoldConfig {
	cfg := &ManifoldConfig{
		DomainServicesName: "domain-services",
		NewServerWrapperWorker: func(ServerWrapperWorkerConfig) (worker.Worker, error) {
			return nil, nil
		},
		NewServerWorker: func(ServerWorkerConfig) (worker.Worker, error) {
			return nil, nil
		},
		GetControllerConfigService: func(getter dependency.Getter, name string) (ControllerConfigService, error) {
			return s.controllerConfigService, nil
		},
		GetControllerSSHHostKeyService: func(getter dependency.Getter, name string) (ControllerSSHHostKeyService, error) {
			return s.controllerSSHHostKeyService, nil
		},
		GetDomainServicesGetter: func(dependency.Getter, string) (services.DomainServicesGetter, error) {
			return stubDomainServicesGetter{}, nil
		},
		GetSSHService: func(context.Context, services.DomainServicesGetter, model.UUID) (SSHModelService, error) {
			return s.sshService, nil
		},
		Logger: loggertesting.WrapCheckLog(c),
	}

	modifier(cfg)

	return cfg
}

func (s *manifoldSuite) TestManifoldUninstall(c *tc.C) {
	// Unset feature flag
	_ = os.Unsetenv(osenv.JujuFeatureFlagEnvKey)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)

	defer s.setupMocks(c).Finish()

	// Setup the manifold
	manifold := Manifold(ManifoldConfig{
		DomainServicesName:     "domain-services",
		NewServerWrapperWorker: NewServerWrapperWorker,
		NewServerWorker: func(ServerWorkerConfig) (worker.Worker, error) {
			return workertest.NewErrorWorker(nil), nil
		},
		GetControllerConfigService: func(getter dependency.Getter, name string) (ControllerConfigService, error) {
			return s.controllerConfigService, nil
		},
		GetControllerSSHHostKeyService: func(getter dependency.Getter, name string) (ControllerSSHHostKeyService, error) {
			return s.controllerSSHHostKeyService, nil
		},
		GetDomainServicesGetter: func(dependency.Getter, string) (services.DomainServicesGetter, error) {
			return stubDomainServicesGetter{}, nil
		},
		GetSSHService: func(context.Context, services.DomainServicesGetter, model.UUID) (SSHModelService, error) {
			return s.sshService, nil
		},
		Logger: loggertesting.WrapCheckLog(c),
	})

	// Check the inputs are as expected
	c.Assert(manifold.Inputs, tc.DeepEquals, []string{"domain-services"})

	// Start the worker
	_, err := manifold.Start(
		c.Context(),
		dt.StubGetter(map[string]any{}),
	)
	c.Assert(err, tc.ErrorIs, dependency.ErrUninstall)
}

type stubDomainServicesGetter struct{}

func (stubDomainServicesGetter) ServicesForModel(context.Context, model.UUID) (services.DomainServices, error) {
	return nil, errors.NotImplementedf("unexpected ServicesForModel call")
}
