// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitless

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	coreerrors "github.com/juju/juju/core/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type manifoldSuite struct {
	testhelpers.IsolationSuite
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestValidate(c *tc.C) {
	config := validManifoldConfig(c)
	c.Check(config.Validate(), tc.ErrorIsNil)

	config.DomainServicesName = ""
	s.checkNotValid(c, config, "empty DomainServicesName not valid")

	config = validManifoldConfig(c)
	config.GetScriptletService = nil
	s.checkNotValid(c, config, "nil GetScriptletService not valid")

	config = validManifoldConfig(c)
	config.NewWorker = nil
	s.checkNotValid(c, config, "nil NewWorker not valid")

	config = validManifoldConfig(c)
	config.Clock = nil
	s.checkNotValid(c, config, "nil Clock not valid")

	config = validManifoldConfig(c)
	config.Logger = nil
	s.checkNotValid(c, config, "nil Logger not valid")
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	service := &stubScriptletService{}
	logger := loggertesting.WrapCheckLog(c)
	expectWorker := &stubWorker{}
	expectClock := clock.WallClock

	config := ManifoldConfig{
		DomainServicesName: "domain-services",
		GetScriptletService: func(_ dependency.Getter, name string) (ScriptletService, error) {
			c.Check(name, tc.Equals, "domain-services")
			return service, nil
		},
		NewWorker: func(config Config) (worker.Worker, error) {
			c.Check(config.ScriptletService, tc.Equals, service)
			c.Check(config.NewExecutor, tc.NotNil)
			c.Check(config.Clock, tc.Equals, expectClock)
			c.Check(config.MaxAllocs, tc.Equals, int64(defaultMaxAllocs))
			c.Check(config.MaxSteps, tc.Equals, int64(defaultMaxSteps))
			c.Check(config.Logger, tc.Equals, logger)
			return expectWorker, nil
		},
		Clock:  expectClock,
		Logger: logger,
	}

	w, err := Manifold(config).Start(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w, tc.Equals, expectWorker)
}

func (s *manifoldSuite) checkNotValid(c *tc.C, config ManifoldConfig, match string) {
	err := config.Validate()
	c.Check(err, tc.ErrorMatches, match)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

func validManifoldConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		DomainServicesName: "domain-services",
		GetScriptletService: func(dependency.Getter, string) (ScriptletService, error) {
			return &stubScriptletService{}, nil
		},
		NewWorker: func(Config) (worker.Worker, error) {
			return &stubWorker{}, nil
		},
		Clock:  clock.WallClock,
		Logger: loggertesting.WrapCheckLog(c),
	}
}

type stubScriptletService struct {
	ScriptletService
}

type stubWorker struct {
	worker.Worker
}
