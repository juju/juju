// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/worker/sshserver"
)

type manifoldSuite struct {
}

var _ = gc.Suite(&manifoldSuite{})

func newManifoldConfig(c *gc.C, modifier func(cfg *sshserver.ManifoldConfig)) *sshserver.ManifoldConfig {
	cfg := &sshserver.ManifoldConfig{
		DomainServicesName:     "domain-services",
		NewServerWrapperWorker: func(sshserver.ServerWrapperWorkerConfig) (worker.Worker, error) { return nil, nil },
		NewServerWorker:        func(sshserver.ServerWorkerConfig) (worker.Worker, error) { return nil, nil },
		GetControllerConfigService: func(getter dependency.Getter, name string) (sshserver.ControllerConfigService, error) {
			return nil, nil
		},
		Logger: loggertesting.WrapCheckLog(c),
	}

	modifier(cfg)

	return cfg
}

func (s *manifoldSuite) TestConfigValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	// Check config as expected.

	cfg := newManifoldConfig(c, func(cfg *sshserver.ManifoldConfig) {})
	c.Assert(cfg.Validate(), gc.IsNil)

	// Entirely missing.
	cfg = newManifoldConfig(c, func(cfg *sshserver.ManifoldConfig) {
		cfg.DomainServicesName = ""
		cfg.NewServerWrapperWorker = nil
		cfg.NewServerWorker = nil
		cfg.GetControllerConfigService = nil
		cfg.Logger = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing domain services name.
	cfg = newManifoldConfig(c, func(cfg *sshserver.ManifoldConfig) {
		cfg.DomainServicesName = ""
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing NewServerWrapperWorker.
	cfg = newManifoldConfig(c, func(cfg *sshserver.ManifoldConfig) {
		cfg.NewServerWrapperWorker = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing NewServerWorker.
	cfg = newManifoldConfig(c, func(cfg *sshserver.ManifoldConfig) {
		cfg.NewServerWorker = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing GetControllerConfigService.
	cfg = newManifoldConfig(c, func(cfg *sshserver.ManifoldConfig) {
		cfg.GetControllerConfigService = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Missing Logger.
	cfg = newManifoldConfig(c, func(cfg *sshserver.ManifoldConfig) {
		cfg.Logger = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

}

func (s *manifoldSuite) TestManifoldStart(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	controllerConfigService := sshserver.NewMockControllerConfigService(ctrl)
	w := workertest.NewErrorWorker(nil)
	// Setup the manifold
	manifold := sshserver.Manifold(sshserver.ManifoldConfig{
		DomainServicesName:     "domain-services",
		NewServerWrapperWorker: sshserver.NewServerWrapperWorker,
		NewServerWorker:        func(sshserver.ServerWorkerConfig) (worker.Worker, error) { return w, nil },
		GetControllerConfigService: func(getter dependency.Getter, name string) (sshserver.ControllerConfigService, error) {
			return controllerConfigService, nil
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
	c.Assert(result, gc.NotNil)
	workertest.CleanKill(c, w)
}
