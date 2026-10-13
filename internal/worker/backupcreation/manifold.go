// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupcreation

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	leaseservice "github.com/juju/juju/domain/lease/service"
	leasestate "github.com/juju/juju/domain/lease/state"
	"github.com/juju/juju/internal/services"
)

// LocalValues contains the current controller filesystem paths.
type LocalValues struct{ DataDir, LogDir string }

// LocalConfigReader reads paths each time the worker starts.
type LocalConfigReader interface{ BackupLocalValues() (LocalValues, error) }

// ManifoldConfig configures the archive creator and its lease service.
type ManifoldConfig struct {
	DBAccessorName      string
	DomainServicesName  string
	ObjectStoreName     string
	ControllerUUID      string
	ControllerModelUUID string
	ControllerTag       names.Tag
	LocalConfigReader   LocalConfigReader
	Clock               clock.Clock
	Logger              logger.Logger
	NewLeaseService     func(database.DBGetter) LeaseService
	NewWorker           func(Config) (worker.Worker, error)
}

// Manifold outputs the creation function consumed by the API server.
func Manifold(c ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{c.DBAccessorName, c.DomainServicesName, c.ObjectStoreName},
		Start:  c.start,
		Output: func(in worker.Worker, out any) error {
			w, ok := in.(*Worker)
			if !ok {
				return errors.Errorf("expected backup creation worker, got %T", in)
			}
			target, ok := out.(*corebackups.CreateFunc)
			if !ok {
				return errors.Errorf("expected backup creation function, got %T", out)
			}
			*target = w.Create
			return nil
		},
	}
}

func (c ManifoldConfig) start(_ context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	paths, err := c.LocalConfigReader.BackupLocalValues()
	if err != nil {
		return nil, errors.Annotate(err, "reading backup paths")
	}
	if paths.DataDir == "" || paths.LogDir == "" {
		return nil, errors.NotValidf("empty backup paths")
	}
	var dbGetter database.DBGetter
	if err := getter.Get(c.DBAccessorName, &dbGetter); err != nil {
		return nil, err
	}
	var domainServices services.DomainServicesGetter
	if err := getter.Get(c.DomainServicesName, &domainServices); err != nil {
		return nil, err
	}
	var objectStores objectstore.ObjectStoreGetter
	if err := getter.Get(c.ObjectStoreName, &objectStores); err != nil {
		return nil, err
	}
	generator := archiveGenerator{config: c, paths: paths, domainServices: domainServices, objectStores: objectStores}
	return c.NewWorker(Config{
		LeaseService:        c.NewLeaseService(dbGetter),
		ControllerUUID:      c.ControllerUUID,
		ControllerModelUUID: c.ControllerModelUUID,
		CreateArchive:       generator.create,
		Clock:               c.Clock,
		Logger:              c.Logger,
	})
}

// Validate checks dependencies before the manifold starts.
func (c ManifoldConfig) Validate() error {
	if c.DBAccessorName == "" {
		return errors.NotValidf("empty DBAccessorName")
	}
	if c.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if c.ObjectStoreName == "" {
		return errors.NotValidf("empty ObjectStoreName")
	}
	if c.ControllerTag == nil {
		return errors.NotValidf("nil ControllerTag")
	}
	if c.LocalConfigReader == nil {
		return errors.NotValidf("nil LocalConfigReader")
	}
	if c.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.NewLeaseService == nil {
		return errors.NotValidf("nil NewLeaseService")
	}
	if c.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// NewLeaseService constructs the controller lease service without database I/O.
func NewLeaseService(dbGetter database.DBGetter) LeaseService {
	factory := database.NewTxnRunnerFactoryForNamespace(dbGetter.GetDB, database.ControllerNS)
	return leaseservice.NewService(leasestate.NewState(factory))
}

// archiveGenerator resolves request-scoped services after acquiring the lease.
// This keeps worker startup independent of model and object-store readiness.
type archiveGenerator struct {
	config         ManifoldConfig
	paths          LocalValues
	domainServices services.DomainServicesGetter
	objectStores   objectstore.ObjectStoreGetter
}

func (g archiveGenerator) create(ctx context.Context, notes string) (*corebackups.Metadata, string, func(), error) {
	controllerServices, err := g.domainServices.ServicesForModel(ctx, coremodel.UUID(g.config.ControllerModelUUID))
	if err != nil {
		return nil, "", nil, err
	}
	modelServicesFor := ModelServicesForFunc(func(ctx context.Context, modelUUID coremodel.UUID) (ModelExportDomainServices, error) {
		modelServices, err := g.domainServices.ServicesForModel(ctx, modelUUID)
		if err != nil {
			return nil, err
		}
		return modelExportDomainServices{modelServices}, nil
	})
	controllerStore, err := g.objectStores.GetObjectStore(ctx, database.ControllerNS)
	if err != nil {
		return nil, "", nil, err
	}
	modelStoreFor := ObjectStoreForModelFunc(func(ctx context.Context, modelUUID string) (ReadObjectStore, error) {
		return g.objectStores.GetObjectStore(ctx, modelUUID)
	})
	creator, err := NewCreator(g.config.ControllerTag, g.config.ControllerUUID,
		coremodel.UUID(g.config.ControllerModelUUID), g.paths.DataDir, g.paths.LogDir,
		controllerServices.ControllerExport(), modelServicesFor,
		controllerServices.Config(), controllerServices.Controller(),
		controllerServices.ControllerNode(), controllerStore, modelStoreFor,
		g.config.Clock, g.config.Logger)
	if err != nil {
		return nil, "", nil, err
	}
	return creator.Create(ctx, notes)
}

// modelExportDomainServices narrows the model services factory for the creator.
type modelExportDomainServices struct{ domainServices services.DomainServices }

func (s modelExportDomainServices) Export() ModelExportService { return s.domainServices.Export() }
