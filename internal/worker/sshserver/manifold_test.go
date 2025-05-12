// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"
	"os"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	controller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/internal/featureflag"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/juju/osenv"
)

type manifoldSuite struct {
	testing.IsolationSuite

	controllerConfigService *MockControllerConfigService
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) SetUpTest(c *gc.C) {
	err := os.Setenv(osenv.JujuFeatureFlagEnvKey, featureflag.SSHJump)
	c.Assert(err, jc.ErrorIsNil)
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

func (s *manifoldSuite) TestConfigValidate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Check config as expected.

	cfg := s.newManifoldConfig(c, func(cfg *ManifoldConfig) {})
	c.Assert(cfg.Validate(), gc.IsNil)

	// Entirely missing.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.DomainServicesName = ""
		cfg.NewServerWrapperWorker = nil
		cfg.NewServerWorker = nil
		cfg.GetControllerConfigService = nil
		cfg.Logger = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing domain services name.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.DomainServicesName = ""
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing NewServerWrapperWorker.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.NewServerWrapperWorker = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing NewServerWorker.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.NewServerWorker = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing GetControllerConfigService.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.GetControllerConfigService = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing Logger.
	cfg = s.newManifoldConfig(c, func(cfg *ManifoldConfig) {
		cfg.Logger = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

}

func (s *manifoldSuite) TestManifoldStart(c *gc.C) {
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
		Logger: loggertesting.WrapCheckLog(c),
	})

	// Check the inputs are as expected
	c.Assert(manifold.Inputs, gc.DeepEquals, []string{"domain-services"})

	// Start the worker
	result, err := manifold.Start(
		context.Background(),
		dt.StubGetter(map[string]interface{}{}),
	)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, result)
	workertest.CheckAlive(c, result)

	c.Check(result, gc.NotNil)
	workertest.CleanKill(c, result)
}

func (s *manifoldSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)

	s.controllerConfigService.EXPECT().WatchControllerConfig().DoAndReturn(func() (watcher.Watcher[[]string], error) {
		return watchertest.NewMockStringsWatcher(make(<-chan []string)), nil
	}).AnyTimes()
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).DoAndReturn(
		func(ctx context.Context) (controller.Config, error) {
			return controller.Config{
				controller.SSHServerPort:               22,
				controller.SSHMaxConcurrentConnections: 10,
			}, nil
		},
	).AnyTimes()
	return ctrl
}

func (s *manifoldSuite) newManifoldConfig(c *gc.C, modifier func(cfg *ManifoldConfig)) *ManifoldConfig {
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
		Logger: loggertesting.WrapCheckLog(c),
	}

	modifier(cfg)

	return cfg
}

func (s *manifoldSuite) TestManifoldUninstall(c *gc.C) {
	// Unset feature flag
	os.Unsetenv(osenv.JujuFeatureFlagEnvKey)
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
		Logger: loggertesting.WrapCheckLog(c),
	})

	// Check the inputs are as expected
	c.Assert(manifold.Inputs, gc.DeepEquals, []string{"domain-services"})

	// Start the worker
	_, err := manifold.Start(
		context.Background(),
		dt.StubGetter(map[string]interface{}{}),
	)
	c.Assert(err, jc.ErrorIs, dependency.ErrUninstall)
}
