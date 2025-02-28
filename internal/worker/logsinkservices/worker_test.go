// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsinkservices

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/services"
)

type workerSuite struct {
	baseSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.DBGetter = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewLogSinkServices = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *workerSuite) getConfig() Config {
	return Config{
		DBGetter: s.dbGetter,
		Logger:   s.logger,
		NewLogSinkServices: func(changestream.WatchableDBGetter, logger.Logger) services.LogSinkServices {
			return s.logSinkServices
		},
	}
}

func (s *workerSuite) TestWorkerServices(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	srvFact, ok := w.(*servicesWorker)
	c.Assert(ok, jc.IsTrue, gc.Commentf("worker does not implement servicesWorker"))

	services := srvFact.Services()
	c.Assert(services, gc.NotNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) newWorker(c *gc.C) worker.Worker {
	w, err := NewWorker(s.getConfig())
	c.Assert(err, jc.ErrorIsNil)
	return w
}
