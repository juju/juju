// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package traceservices

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5/workertest"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/services"
)

type workerSuite struct {
	baseSuite
}

func TestWorkerSuite(t *testing.T) {
	tc.Run(t, &workerSuite{})
}

func (s *workerSuite) TestValidateConfig(c *tc.C) {
	s.setupMocks(c)

	cfg := s.getConfig()
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.DBGetter = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewTraceServices = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *workerSuite) TestWorkerServices(c *tc.C) {
	s.setupMocks(c)

	w, err := NewWorker(s.getConfig())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	srvFact, ok := w.(*servicesWorker)
	c.Assert(ok, tc.IsTrue, tc.Commentf("worker does not implement servicesWorker"))

	c.Check(srvFact.Services(), tc.Equals, s.traceService)
}

func (s *workerSuite) getConfig() Config {
	return Config{
		DBGetter: s.dbGetter,
		Logger:   s.logger,
		NewTraceServices: func(changestream.WatchableDBGetter, logger.Logger) services.TraceServices {
			return s.traceService
		},
	}
}
