// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupsweeper

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/core/model"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type manifoldSuite struct {
	baseSuite
}

func TestManifoldSuite(t *testing.T) {
	testhelpers.PrintGoroutineLeaks(t, func(t *testing.T) {
		tc.Run(t, &manifoldSuite{})
	})
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig(c)
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig(c)
	cfg.DomainServicesName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.ControllerModelUUID = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Clock = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.GetModelConfigService = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	manifold := Manifold(s.getConfig(c))
	c.Check(manifold.Inputs, tc.DeepEquals, []string{"domain-services"})
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	var gotUUID model.UUID
	var gotCfg WorkerConfig
	cfg := s.getConfig(c)
	cfg.GetModelConfigService = func(getter dependency.Getter, name string, controllerModelUUID model.UUID) (ModelConfigService, error) {
		gotUUID = controllerModelUUID
		return s.modelConfig, nil
	}
	cfg.NewWorker = func(wcfg WorkerConfig) (worker.Worker, error) {
		gotCfg = wcfg
		return workertest.NewForeverWorker(nil), nil
	}

	w, err := Manifold(cfg).Start(c.Context(), dt.StubGetter(map[string]any{"domain-services": nil}))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w, tc.NotNil)

	// The controller model UUID from the config is what the domain
	// services getter is queried with, and the resolved model config
	// service reaches the worker.
	c.Check(gotUUID.String(), tc.Equals, cfg.ControllerModelUUID)
	c.Check(gotCfg.ModelConfigService, tc.NotNil)
	c.Check(gotCfg.Clock, tc.NotNil)
	c.Check(gotCfg.Logger, tc.NotNil)
}

func (s *manifoldSuite) getConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		DomainServicesName:  "domain-services",
		ControllerModelUUID: "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		Clock:               s.clock,
		Logger:              loggertesting.WrapCheckLog(c),
		NewWorker: func(WorkerConfig) (worker.Worker, error) {
			return nil, nil
		},
		GetModelConfigService: func(getter dependency.Getter, name string, controllerModelUUID model.UUID) (ModelConfigService, error) {
			return nil, nil
		},
	}
}
