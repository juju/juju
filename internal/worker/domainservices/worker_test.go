// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domainservices

import (
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/storage"
	domainservices "github.com/juju/juju/domain/services"
	"github.com/juju/juju/internal/services"
)

type workerSuite struct {
	baseSuite
}

func TestWorkerSuite(t *stdtesting.T) { tc.Run(t, &workerSuite{}) }
func (s *workerSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig(c)
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.DBDeleter = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.DBGetter = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.ProviderFactory = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.ObjectStoreGetter = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.StorageRegistryGetter = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.LeaseManager = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewDomainServicesGetter = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewControllerDomainServices = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewModelDomainServices = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.LogDir = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Clock = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.PublicKeyImporter = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.LoggerContextGetter = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *workerSuite) getConfig(c *tc.C) Config {
	return Config{
		DBGetter:              s.dbGetter,
		DBDeleter:             s.dbDeleter,
		ProviderFactory:       s.providerFactory,
		ObjectStoreGetter:     s.objectStoreGetter,
		StorageRegistryGetter: s.storageRegistryGetter,
		PublicKeyImporter:     s.publicKeyImporter,
		LeaseManager:          s.leaseManager,
		LogDir:                c.MkDir(),
		Clock:                 s.clock,
		Logger:                s.logger,
		LoggerContextGetter:   s.loggerContextGetter,
		NewDomainServicesGetter: func(
			services.ControllerDomainServices,
			changestream.WatchableDBGetter,
			ModelDomainServicesFn,
			providertracker.ProviderFactory,
			objectstore.ObjectStoreGetter,
			storage.StorageRegistryGetter,
			domainservices.PublicKeyImporter,
			lease.Manager,
			string,
			clock.Clock,
			logger.LoggerContextGetter,
		) services.DomainServicesGetter {
			return s.domainServicesGetter
		},
		NewControllerDomainServices: func(
			changestream.WatchableDBGetter,
			coredatabase.DBDeleter,
			objectstore.NamespacedObjectStoreGetter,
			clock.Clock,
			logger.Logger,
		) services.ControllerDomainServices {
			return s.controllerDomainServices
		},
		NewModelDomainServices: func(
			coremodel.UUID,
			changestream.WatchableDBGetter,
			providertracker.ProviderFactory,
			objectstore.ModelObjectStoreGetter,
			storage.ModelStorageRegistryGetter,
			domainservices.PublicKeyImporter,
			lease.ModelLeaseManagerGetter,
			string,
			clock.Clock,
			logger.Logger,
		) services.ModelDomainServices {
			return s.modelDomainServices
		},
	}
}

func (s *workerSuite) TestWorkerControllerServices(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	srvFact, ok := w.(*domainServicesWorker)
	c.Assert(ok, tc.IsTrue, tc.Commentf("worker does not implement domainServicesWorker"))

	factory := srvFact.ControllerServices()
	c.Assert(factory, tc.NotNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) TestWorkerServicesGetter(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w := s.newWorker(c)
	defer workertest.CleanKill(c, w)

	srvFact, ok := w.(*domainServicesWorker)
	c.Assert(ok, tc.IsTrue, tc.Commentf("worker does not implement domainServicesWorker"))

	factory := srvFact.ServicesGetter()
	c.Assert(factory, tc.NotNil)

	workertest.CleanKill(c, w)
}

func (s *workerSuite) newWorker(c *tc.C) worker.Worker {
	w, err := NewWorker(s.getConfig(c))
	c.Assert(err, tc.ErrorIsNil)
	return w
}
