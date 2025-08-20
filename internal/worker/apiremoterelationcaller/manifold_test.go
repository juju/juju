// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremoterelationcaller

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dependencytesting "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/errors"
)

type manifoldSuite struct {
	baseSuite
}

func TestManifoldSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig(c)
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig(c)
	cfg.DomainServicesName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		DomainServicesName: "domain-services",
		NewAPIInfoGetter: func(getter DomainServicesGetter) APIInfoGetter {
			return nil
		},
		GetDomainServicesGetterFunc: func(getter dependency.Getter, name string) (DomainServicesGetter, error) {
			return nil, nil
		},
		NewConnectionFunc: func(ctx context.Context, apiInfo api.Info) (api.Connection, error) {
			return nil, nil
		},
		NewWorker: func(config Config) (worker.Worker, error) {
			return workertest.NewErrorWorker(nil), nil
		},
		Logger: s.logger,
		Clock:  clock.WallClock,
	}
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"domain-services": struct{}{},
	}
	return dependencytesting.StubGetter(resources)
}

var expectedInputs = []string{"domain-services"}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(Manifold(s.getConfig(c)).Inputs, tc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := Manifold(s.getConfig(c)).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}
