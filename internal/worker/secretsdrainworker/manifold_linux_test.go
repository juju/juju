//go:build dqlite

// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrainworker_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	dt "github.com/juju/worker/v5/dependency/testing"

	secretservice "github.com/juju/juju/domain/secret/service"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/secretsdrainworker"
)

type ModelManifoldSuite struct {
	testhelpers.IsolationSuite
	config secretsdrainworker.ModelManifoldConfig
}

func TestModelManifoldSuite(t *testing.T) {
	tc.Run(t, &ModelManifoldSuite{})
}

func (s *ModelManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validModelConfig(c)
}

func (s *ModelManifoldSuite) validModelConfig(c *tc.C) secretsdrainworker.ModelManifoldConfig {
	return secretsdrainworker.ModelManifoldConfig{
		DomainServicesName: "domain-services",
		ModelUUID:          "mock-model-uuid",
		Logger:             loggertesting.WrapCheckLog(c),
		NewWorker: func(config secretsdrainworker.Config) (worker.Worker, error) {
			return nil, nil
		},
	}
}

func (s *ModelManifoldSuite) TestValid(c *tc.C) {
	c.Check(s.config.Validate(), tc.ErrorIsNil)
}

func (s *ModelManifoldSuite) TestMissingDomainServicesName(c *tc.C) {
	s.config.DomainServicesName = ""
	s.checkNotValid(c, "empty DomainServicesName not valid")
}

func (s *ModelManifoldSuite) TestMissingModelUUID(c *tc.C) {
	s.config.ModelUUID = ""
	s.checkNotValid(c, "empty ModelUUID not valid")
}

func (s *ModelManifoldSuite) TestMissingLogger(c *tc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *ModelManifoldSuite) TestMissingNewWorker(c *tc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *ModelManifoldSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}

func (s *ModelManifoldSuite) TestInputs(c *tc.C) {
	manifold := secretsdrainworker.ModelManifold(s.config)
	c.Check(manifold.Inputs, tc.DeepEquals, []string{"domain-services"})
}

func (s *ModelManifoldSuite) TestStart(c *tc.C) {
	called := false
	s.config.NewWorker = func(config secretsdrainworker.Config) (worker.Worker, error) {
		called = true
		c.Check(config.SecretsDrainFacade, tc.NotNil)
		c.Check(config.Logger, tc.NotNil)
		c.Check(config.SecretsBackendGetter, tc.NotNil)
		c.Check(config.LeadershipTrackerFunc, tc.NotNil)
		return nil, nil
	}
	manifold := secretsdrainworker.ModelManifold(s.config)
	w, err := manifold.Start(c.Context(), dt.StubGetter(map[string]any{
		"domain-services": &stubDomainServices{},
	}))
	c.Assert(w, tc.IsNil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(called, tc.IsTrue)
}

// stubDomainServices is a minimal stub that satisfies services.DomainServices
// for tests that only need Secret() and SecretBackend() methods.
type stubDomainServices struct {
	services.ControllerDomainServices
	services.ModelDomainServices
}

func (s *stubDomainServices) Secret() *secretservice.WatchableService {
	return nil
}

func (s *stubDomainServices) SecretBackend() *secretbackendservice.WatchableService {
	return nil
}
