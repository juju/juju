// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domainservices

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/storage"
	domainservices "github.com/juju/juju/domain/services"
	"github.com/juju/juju/internal/services"
)

type manifoldSuite struct {
	baseSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.DBAccessorName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.ProviderFactoryName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.ObjectStoreName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.ChangeStreamName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.StorageRegistryName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.HTTPClientName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.LeaseManagerName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.LogSinkName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewWorker = nil
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

	cfg = s.getConfig()
	cfg.Clock = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.httpClientGetter.EXPECT().GetHTTPClient(gomock.Any(), corehttp.SSHImporterPurpose).Return(s.httpClient, nil)

	getter := map[string]any{
		"dbaccessor":      s.dbDeleter,
		"changestream":    s.dbGetter,
		"providerfactory": s.providerFactory,
		"objectstore":     s.objectStoreGetter,
		"storageregistry": s.storageRegistryGetter,
		"httpclient":      s.httpClientGetter,
		"leasemanager":    s.leaseManager,
		"logsink":         s.loggerContextGetter,
	}

	manifold := Manifold(ManifoldConfig{
		DBAccessorName:              "dbaccessor",
		ChangeStreamName:            "changestream",
		ProviderFactoryName:         "providerfactory",
		ObjectStoreName:             "objectstore",
		StorageRegistryName:         "storageregistry",
		HTTPClientName:              "httpclient",
		LeaseManagerName:            "leasemanager",
		LogSinkName:                 "logsink",
		Logger:                      s.logger,
		NewWorker:                   NewWorker,
		NewDomainServicesGetter:     NewDomainServicesGetter,
		NewControllerDomainServices: NewControllerDomainServices,
		NewModelDomainServices:      NewProviderTrackerModelDomainServices,
		Clock:                       s.clock,
	})
	w, err := manifold.Start(context.Background(), dt.StubGetter(getter))
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	workertest.CheckAlive(c, w)
}

func (s *manifoldSuite) TestOutputControllerDomainServices(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, err := NewWorker(Config{
		DBDeleter:                   s.dbDeleter,
		DBGetter:                    s.dbGetter,
		Logger:                      s.logger,
		LoggerContextGetter:         s.loggerContextGetter,
		ProviderFactory:             s.providerFactory,
		ObjectStoreGetter:           s.objectStoreGetter,
		StorageRegistryGetter:       s.storageRegistryGetter,
		PublicKeyImporter:           s.publicKeyImporter,
		LeaseManager:                s.leaseManager,
		NewDomainServicesGetter:     NewDomainServicesGetter,
		NewControllerDomainServices: NewControllerDomainServices,
		NewModelDomainServices:      NewProviderTrackerModelDomainServices,
		Clock:                       s.clock,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	manifold := ManifoldConfig{}

	var factory services.ControllerDomainServices
	err = manifold.output(w, &factory)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *manifoldSuite) TestOutputDomainServicesGetter(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, err := NewWorker(Config{
		DBDeleter:                   s.dbDeleter,
		DBGetter:                    s.dbGetter,
		Logger:                      s.logger,
		LoggerContextGetter:         s.loggerContextGetter,
		ProviderFactory:             s.providerFactory,
		ObjectStoreGetter:           s.objectStoreGetter,
		StorageRegistryGetter:       s.storageRegistryGetter,
		PublicKeyImporter:           s.publicKeyImporter,
		LeaseManager:                s.leaseManager,
		NewDomainServicesGetter:     NewDomainServicesGetter,
		NewControllerDomainServices: NewControllerDomainServices,
		NewModelDomainServices:      NewProviderTrackerModelDomainServices,
		Clock:                       s.clock,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	manifold := ManifoldConfig{}

	var factory services.DomainServicesGetter
	err = manifold.output(w, &factory)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *manifoldSuite) TestOutputInvalid(c *gc.C) {
	defer s.setupMocks(c).Finish()

	w, err := NewWorker(Config{
		DBDeleter:                   s.dbDeleter,
		DBGetter:                    s.dbGetter,
		Logger:                      s.logger,
		LoggerContextGetter:         s.loggerContextGetter,
		ProviderFactory:             s.providerFactory,
		ObjectStoreGetter:           s.objectStoreGetter,
		StorageRegistryGetter:       s.storageRegistryGetter,
		PublicKeyImporter:           s.publicKeyImporter,
		LeaseManager:                s.leaseManager,
		NewDomainServicesGetter:     NewDomainServicesGetter,
		NewControllerDomainServices: NewControllerDomainServices,
		NewModelDomainServices:      NewProviderTrackerModelDomainServices,
		Clock:                       s.clock,
	})
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	manifold := ManifoldConfig{}

	var factory struct{}
	err = manifold.output(w, &factory)
	c.Assert(err, gc.ErrorMatches, `unsupported output type .*`)
}

func (s *manifoldSuite) TestNewControllerDomainServices(c *gc.C) {
	factory := NewControllerDomainServices(s.dbGetter, s.dbDeleter, s.modelObjectStoreGetter, s.clock, s.logger)
	c.Assert(factory, gc.NotNil)
}

func (s *manifoldSuite) TestNewModelDomainServices(c *gc.C) {
	factory := NewModelDomainServices(
		"model",
		s.dbGetter,
		s.modelObjectStoreGetter,
		s.modelStorageRegistryGetter,
		s.publicKeyImporter,
		s.modelLeaseManagerGetter,
		s.clock,
		s.logger,
	)
	c.Assert(factory, gc.NotNil)
}

func (s *manifoldSuite) TestNewDomainServicesGetter(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.loggerContextGetter.EXPECT().GetLoggerContext(gomock.Any(), coremodel.UUID("model")).Return(s.loggerContext, nil)
	s.loggerContext.EXPECT().GetLogger("juju.services").Return(s.logger)

	ctrlFactory := NewControllerDomainServices(s.dbGetter, s.dbDeleter, s.modelObjectStoreGetter, s.clock, s.logger)
	factory := NewDomainServicesGetter(
		ctrlFactory,
		s.dbGetter,
		NewProviderTrackerModelDomainServices,
		nil,
		s.objectStoreGetter,
		s.storageRegistryGetter,
		s.publicKeyImporter,
		s.leaseManager,
		s.clock,
		s.loggerContextGetter,
	)
	c.Assert(factory, gc.NotNil)

	modelFactory, err := factory.ServicesForModel(context.Background(), "model")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(modelFactory, gc.NotNil)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		DBAccessorName:      "dbaccessor",
		ChangeStreamName:    "changestream",
		ProviderFactoryName: "providerfactory",
		ObjectStoreName:     "objectstore",
		StorageRegistryName: "storageregistry",
		HTTPClientName:      "httpclient",
		LeaseManagerName:    "leasemanager",
		LogSinkName:         "logsink",
		Clock:               s.clock,
		Logger:              s.logger,
		NewWorker: func(Config) (worker.Worker, error) {
			return nil, nil
		},
		NewDomainServicesGetter:     noopDomainServicesGetter,
		NewControllerDomainServices: noopControllerDomainServices,
		NewModelDomainServices:      noopModelDomainServices,
	}
}

func noopDomainServicesGetter(
	services.ControllerDomainServices,
	changestream.WatchableDBGetter,
	ModelDomainServicesFn,
	providertracker.ProviderFactory,
	objectstore.ObjectStoreGetter,
	storage.StorageRegistryGetter,
	domainservices.PublicKeyImporter,
	lease.Manager,
	clock.Clock,
	logger.LoggerContextGetter,
) services.DomainServicesGetter {
	return nil
}

func noopControllerDomainServices(
	changestream.WatchableDBGetter,
	coredatabase.DBDeleter,
	objectstore.ModelObjectStoreGetter,
	clock.Clock,
	logger.Logger,
) services.ControllerDomainServices {
	return nil
}

func noopModelDomainServices(
	coremodel.UUID,
	changestream.WatchableDBGetter,
	providertracker.ProviderFactory,
	objectstore.ModelObjectStoreGetter,
	storage.ModelStorageRegistryGetter,
	domainservices.PublicKeyImporter,
	lease.ModelLeaseManagerGetter,
	clock.Clock,
	logger.Logger,
) services.ModelDomainServices {
	return nil
}
