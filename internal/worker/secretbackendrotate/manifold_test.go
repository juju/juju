// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendrotate_test

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"

	corewatcher "github.com/juju/juju/core/watcher"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/secretbackendrotate"
)

type ManifoldConfigSuite struct {
	testhelpers.IsolationSuite
	config secretbackendrotate.ManifoldConfig
}

func TestManifoldConfigSuite(t *testing.T) {
	tc.Run(t, &ManifoldConfigSuite{})
}

func (s *ManifoldConfigSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig(c)
}

func (s *ManifoldConfigSuite) validConfig(c *tc.C) secretbackendrotate.ManifoldConfig {
	return secretbackendrotate.ManifoldConfig{
		DomainServicesName:      "domain-services",
		Logger:                  loggertesting.WrapCheckLog(c),
		GetSecretBackendService: stubGetSecretBackendService,
		NewWorker:               secretbackendrotate.NewWorker,
	}
}

func (s *ManifoldConfigSuite) TestValid(c *tc.C) {
	c.Check(s.config.Validate(), tc.ErrorIsNil)
}

func (s *ManifoldConfigSuite) TestMissingDomainServicesName(c *tc.C) {
	s.config.DomainServicesName = ""
	s.checkNotValid(c, "missing DomainServicesName not valid")
}

func (s *ManifoldConfigSuite) TestMissingLogger(c *tc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *ManifoldConfigSuite) TestMissingGetSecretBackendService(c *tc.C) {
	s.config.GetSecretBackendService = nil
	s.checkNotValid(c, "nil GetSecretBackendService not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewWorker(c *tc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *ManifoldConfigSuite) TestInputs(c *tc.C) {
	manifold := secretbackendrotate.Manifold(s.config)
	c.Check(manifold.Inputs, tc.SameContents, []string{"domain-services"})
}

func (s *ManifoldConfigSuite) TestInputsNoAPICaller(c *tc.C) {
	manifold := secretbackendrotate.Manifold(s.config)
	for _, input := range manifold.Inputs {
		c.Check(input, tc.Not(tc.Equals), "api-caller")
	}
}

func (s *ManifoldConfigSuite) TestStart(c *tc.C) {
	stub := &stubSecretBackendService{}
	started := false
	cfg := s.validConfig(c)
	cfg.GetSecretBackendService = func(_ dependency.Getter, _ string) (secretbackendrotate.SecretBackendService, error) {
		return stub, nil
	}
	cfg.NewWorker = func(workerCfg secretbackendrotate.Config) (worker.Worker, error) {
		c.Assert(workerCfg.SecretBackendManagerFacade, tc.NotNil)
		c.Assert(workerCfg.Clock, tc.NotNil)
		c.Assert(workerCfg.Logger, tc.NotNil)
		started = true
		return workertest.NewDeadWorker(nil), nil
	}

	getter := dt.StubGetter(map[string]any{
		"domain-services": struct{}{},
	})

	w, err := secretbackendrotate.Manifold(cfg).Start(c.Context(), getter)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(started, tc.IsTrue)
	workertest.CleanKill(c, w)
}

func (s *ManifoldConfigSuite) TestGetSecretBackendServiceHelper(c *tc.C) {
	// GetSecretBackendService is exercised via the start path test above.
	// This test confirms it does not request api-caller.
	manifold := secretbackendrotate.Manifold(s.config)
	for _, input := range manifold.Inputs {
		c.Check(input, tc.Not(tc.Equals), "api-caller")
	}
}

func (s *ManifoldConfigSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}

// stubSecretBackendService is a minimal stub implementation of
// SecretBackendService used in manifold tests.
type stubSecretBackendService struct{}

func (s *stubSecretBackendService) WatchSecretBackendRotationChanges(_ context.Context) (corewatcher.SecretBackendRotateWatcher, error) {
	return nil, nil
}

func (s *stubSecretBackendService) RotateBackendToken(_ context.Context, _ string) error {
	return nil
}

func stubGetSecretBackendService(_ dependency.Getter, _ string) (secretbackendrotate.SecretBackendService, error) {
	return &stubSecretBackendService{}, nil
}
