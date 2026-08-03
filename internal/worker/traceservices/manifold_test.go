// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package traceservices

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	dt "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/services"
)

type manifoldSuite struct {
	baseSuite
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	s.setupMocks(c)

	cfg := s.getConfig()
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.ChangeStreamName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewTraceServices = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	s.setupMocks(c)

	c.Assert(Manifold(s.getConfig()).Inputs, tc.SameContents, []string{"changestream"})
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	s.setupMocks(c)

	getter := map[string]any{
		"changestream": s.dbGetter,
	}

	w, err := Manifold(s.getConfig()).Start(c.Context(), dt.StubGetter(getter))
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	workertest.CheckAlive(c, w)
}

func (s *manifoldSuite) TestOutputTraceServices(c *tc.C) {
	s.setupMocks(c)

	w, err := NewWorker(s.workerConfig())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	var services services.TraceServices
	err = ManifoldConfig{}.output(w, &services)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(services, tc.Equals, s.traceService)
}

func (s *manifoldSuite) TestOutputInvalid(c *tc.C) {
	s.setupMocks(c)

	w, err := NewWorker(s.workerConfig())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	var factory struct{}
	err = ManifoldConfig{}.output(w, &factory)
	c.Assert(err, tc.ErrorMatches, `unsupported output type .*`)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		ChangeStreamName: "changestream",
		Logger:           s.logger,
		NewWorker:        NewWorker,
		NewTraceServices: func(changestream.WatchableDBGetter, logger.Logger) services.TraceServices {
			return s.traceService
		},
	}
}

func (s *manifoldSuite) workerConfig() Config {
	return Config{
		DBGetter: s.dbGetter,
		Logger:   s.logger,
		NewTraceServices: func(changestream.WatchableDBGetter, logger.Logger) services.TraceServices {
			return s.traceService
		},
	}
}

var _ worker.Worker = (*servicesWorker)(nil)
