// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package removal

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type manifoldConfigSuite struct {
	testhelpers.IsolationSuite

	config ManifoldConfig
}

var _ = tc.Suite(&manifoldConfigSuite{})

func (s *manifoldConfigSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.config = validConfig(c)
}

func (s *manifoldConfigSuite) TestMissingDomainServicesName(c *tc.C) {
	s.config.DomainServicesName = ""
	s.checkNotValid(c, "empty DomainServicesName not valid")
}

func (s *manifoldConfigSuite) TestMissingGetRemovalService(c *tc.C) {
	s.config.GetRemovalService = nil
	s.checkNotValid(c, "nil GetRemovalService not valid")
}

func (s *manifoldConfigSuite) TestMissingNewWorker(c *tc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *manifoldConfigSuite) TestMissingClock(c *tc.C) {
	s.config.Clock = nil
	s.checkNotValid(c, "nil Clock not valid")
}

func (s *manifoldConfigSuite) TestMissingLogger(c *tc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func validConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		DomainServicesName: "domain-services",
		GetRemovalService:  GetRemovalService,
		NewWorker:          func(Config) (worker.Worker, error) { return noWorker{}, nil },
		Clock:              clock.WallClock,
		Logger:             loggertesting.WrapCheckLog(c),
	}
}

func (s *manifoldConfigSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}

type manifoldSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestStartSuccess(c *tc.C) {
	cfg := ManifoldConfig{
		DomainServicesName: "domain-services",
		GetRemovalService:  func(dependency.Getter, string) (RemovalService, error) { return noService{}, nil },
		NewWorker: func(cfg Config) (worker.Worker, error) {
			if err := cfg.Validate(); err != nil {
				return nil, err
			}
			return noWorker{}, nil
		},
		Clock:  clock.WallClock,
		Logger: loggertesting.WrapCheckLog(c),
	}

	w, err := Manifold(cfg).Start(context.Background(), noGetter{})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w, tc.NotNil)
}

type noGetter struct {
	dependency.Getter
}

type noService struct {
	RemovalService
}

type noWorker struct {
	worker.Worker
}
