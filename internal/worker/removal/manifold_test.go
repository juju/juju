// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package removal

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type manifoldConfigSuite struct {
	testing.IsolationSuite

	config ManifoldConfig
}

var _ = gc.Suite(&manifoldConfigSuite{})

func (s *manifoldConfigSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.config = validConfig(c)
}

func (s *manifoldConfigSuite) TestMissingDomainServicesName(c *gc.C) {
	s.config.DomainServicesName = ""
	s.checkNotValid(c, "empty DomainServicesName not valid")
}

func (s *manifoldConfigSuite) TestMissingGetRemovalService(c *gc.C) {
	s.config.GetRemovalService = nil
	s.checkNotValid(c, "nil GetRemovalService not valid")
}

func (s *manifoldConfigSuite) TestMissingNewWorker(c *gc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *manifoldConfigSuite) TestMissingLogger(c *gc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func validConfig(c *gc.C) ManifoldConfig {
	return ManifoldConfig{
		DomainServicesName: "domain-services",
		GetRemovalService:  GetRemovalService,
		NewWorker:          func(Config) (worker.Worker, error) { return noWorker{}, nil },
		Logger:             loggertesting.WrapCheckLog(c),
	}
}

func (s *manifoldConfigSuite) checkNotValid(c *gc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(err, jc.ErrorIs, errors.NotValid)
}

type manifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestStartSuccess(c *gc.C) {
	cfg := ManifoldConfig{
		DomainServicesName: "domain-services",
		GetRemovalService:  func(dependency.Getter, string) (RemovalService, error) { return noService{}, nil },
		NewWorker:          func(Config) (worker.Worker, error) { return noWorker{}, nil },
		Logger:             loggertesting.WrapCheckLog(c),
	}

	w, err := Manifold(cfg).Start(context.Background(), noGetter{})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(w, gc.NotNil)
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
