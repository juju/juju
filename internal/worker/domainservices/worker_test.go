// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domainservices

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/providertracker"
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
	cfg.DBDeleter = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.DBGetter = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.ProviderFactory = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.ObjectStoreGetter = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewDomainServicesGetter = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewControllerDomainServices = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewModelDomainServices = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *workerSuite) getConfig() Config {
	return Config{
		DBGetter:          s.dbGetter,
		DBDeleter:         s.dbDeleter,
		ProviderFactory:   s.providerFactory,
		ObjectStoreGetter: s.objectStoreGetter,
		Logger:            s.logger,
		NewDomainServicesGetter: func(services.ControllerDomainServices, changestream.WatchableDBGetter, logger.Logger, ModelDomainServicesFn, providertracker.ProviderFactory, objectstore.ObjectStoreGetter) services.DomainServicesGetter {
			return s.domainServicesGetter
		},
		NewControllerDomainServices: func(changestream.WatchableDBGetter, coredatabase.DBDeleter, logger.Logger) services.ControllerDomainServices {
			return s.controllerDomainServices
		},
		NewModelDomainServices: func(coremodel.UUID, changestream.WatchableDBGetter, providertracker.ProviderFactory, objectstore.ModelObjectStoreGetter, logger.Logger) services.ModelDomainServices {
			return s.modelDomainServices
		},
	}
}

func (s *workerSuite) TestWorkerControllerServices(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	srvFact, ok := w.(*domainServicesWorker)
	c.Assert(ok, jc.IsTrue, gc.Commentf("worker does not implement domainServicesWorker"))

	factory := srvFact.ControllerServices()
	c.Assert(factory, gc.NotNil)

	workertest.CleanKill(c, w)
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
