// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providerservices

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
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
	cfg.NewProviderServices = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewProviderServicesGetter = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *workerSuite) getConfig() Config {
	return Config{
		DBGetter: s.dbGetter,
		Logger:   s.logger,
		NewProviderServices: func(coremodel.UUID, changestream.WatchableDBGetter, logger.Logger) services.ProviderServices {
			return s.providerServices
		},
		NewProviderServicesGetter: func(ProviderServicesFn, changestream.WatchableDBGetter, logger.Logger) services.ProviderServicesGetter {
			return s.providerServicesGetter
		},
	}
}

func (s *workerSuite) TestWorkerServicesGetter(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	srvFact, ok := w.(*domainServicesWorker)
	c.Assert(ok, jc.IsTrue, gc.Commentf("worker does not implement domainServicesWorker"))

	factory := srvFact.ServicesGetter()
	c.Assert(factory, gc.NotNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) newWorker(c *gc.C) worker.Worker {
	w, err := NewWorker(s.getConfig())
	c.Assert(err, jc.ErrorIsNil)
	return w
}
