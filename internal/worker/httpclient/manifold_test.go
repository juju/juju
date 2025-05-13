// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpclient

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dependencytesting "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"

	corehttp "github.com/juju/juju/core/http"
	internalhttp "github.com/juju/juju/internal/http"
)

type manifoldSuite struct {
	baseSuite
}

var _ = tc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg.NewHTTPClient = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg.NewHTTPClientWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg.Clock = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		NewHTTPClient: func(corehttp.Purpose, ...internalhttp.Option) *internalhttp.Client {
			return nil
		},
		NewHTTPClientWorker: func(c *internalhttp.Client) (worker.Worker, error) {
			return nil, nil
		},
		Clock:  s.clock,
		Logger: s.logger,
	}
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{}
	return dependencytesting.StubGetter(resources)
}

var expectedInputs = []string{}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, tc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := Manifold(s.getConfig()).Start(context.Background(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}
