// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
)

// LocalValues contains the current controller filesystem paths.
type LocalValues struct{ DataDir, LogDir string }

// LocalConfigReader reads paths each time the worker starts.
type LocalConfigReader interface{ BackupLocalValues() (LocalValues, error) }

// ManifoldConfig configures the archive creator and its lease service.
type ManifoldConfig struct {
	LeaseManagerName        string
	DomainServicesName      string
	ObjectStoreName         string
	ControllerUUID          string
	ControllerModelUUID     string
	ControllerNodeID        string
	ControllerTag           names.Tag
	MachineInstanceIDGetter GetMachineInstanceIDFunc
	MachineBaseGetter       GetMachineBaseFunc
	LocalConfigReader       LocalConfigReader
	Clock                   clock.Clock
	Logger                  logger.Logger
	NewWorker               func(Config) (worker.Worker, error)
}

// Validate checks dependencies before the manifold starts.
func (c ManifoldConfig) Validate() error {
	if c.LeaseManagerName == "" {
		return errors.Errorf("empty LeaseManagerName %w", coreerrors.NotValid)
	}
	if c.DomainServicesName == "" {
		return errors.Errorf("empty DomainServicesName %w", coreerrors.NotValid)
	}
	if c.ObjectStoreName == "" {
		return errors.Errorf("empty ObjectStoreName %w", coreerrors.NotValid)
	}
	if c.ControllerTag == nil {
		return errors.Errorf("nil ControllerTag %w", coreerrors.NotValid)
	}
	if c.MachineInstanceIDGetter == nil {
		return errors.Errorf("nil MachineInstanceIDGetter %w", coreerrors.NotValid)
	}
	if c.MachineBaseGetter == nil {
		return errors.Errorf("nil MachineBaseGetter %w", coreerrors.NotValid)
	}
	if c.ControllerNodeID == "" {
		return errors.Errorf("empty ControllerNodeID %w", coreerrors.NotValid)
	}
	if c.LocalConfigReader == nil {
		return errors.Errorf("nil LocalConfigReader %w", coreerrors.NotValid)
	}
	if c.Clock == nil {
		return errors.Errorf("nil Clock %w", coreerrors.NotValid)
	}
	if c.Logger == nil {
		return errors.Errorf("nil Logger %w", coreerrors.NotValid)
	}
	if c.NewWorker == nil {
		return errors.Errorf("nil NewWorker %w", coreerrors.NotValid)
	}
	return nil
}

// Manifold outputs the creation function consumed by the API server.
func Manifold(c ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			c.LeaseManagerName,
			c.DomainServicesName,
			c.ObjectStoreName,
		},
		Start: func(_ context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := c.Validate(); err != nil {
				return nil, err
			}
			paths, err := c.LocalConfigReader.BackupLocalValues()
			if err != nil {
				return nil, errors.Errorf("reading backup paths: %w", err)
			}
			if paths.DataDir == "" || paths.LogDir == "" {
				return nil, errors.Errorf("empty backup paths %w", coreerrors.NotValid)
			}
			var leaseManager lease.Manager
			if err := getter.Get(c.LeaseManagerName, &leaseManager); err != nil {
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

			return c.NewWorker(Config{
				LeaseManager:        leaseManager,
				ControllerUUID:      c.ControllerUUID,
				ControllerModelUUID: c.ControllerModelUUID,
				ControllerNodeID:    c.ControllerNodeID,
				CreateArchive: archiveGenerator{
					config:         c,
					paths:          paths,
					domainServices: domainServices,
					objectStores:   objectStores,
				},
				Clock:  c.Clock,
				Logger: c.Logger,
			})
		},
		Output: output,
	}
}

func output(in worker.Worker, out any) error {
	w, ok := in.(*Worker)
	if !ok {
		return errors.Errorf("expected backup creation worker, got %T", in)
	}
	target, ok := out.(*corebackups.BackupCreator)
	if !ok {
		return errors.Errorf("expected backup creation function, got %T", out)
	}
	*target = w
	return nil
}

// archiveGenerator resolves request-scoped services after acquiring the lease.
// This keeps worker startup independent of model and object-store readiness.
type archiveGenerator struct {
	config         ManifoldConfig
	paths          LocalValues
	domainServices services.DomainServicesGetter
	objectStores   objectstore.ObjectStoreGetter
}

// Create creates a backup archive and returns its metadata, its path, and a
// cleanup that removes it.
func (g archiveGenerator) Create(ctx context.Context, notes string) (*corebackups.Metadata, string, func(), error) {
	controllerServices, err := g.domainServices.ServicesForModel(ctx, coremodel.UUID(g.config.ControllerModelUUID))
	if err != nil {
		return nil, "", nil, err
	}

	modelServicesFor := ModelServicesGetterFunc(func(ctx context.Context, modelUUID coremodel.UUID) (ModelExportDomainServices, error) {
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

	modelObjectStoreFor := ObjectStoreForModelFunc(func(ctx context.Context, modelUUID string) (ReadObjectStore, error) {
		return g.objectStores.GetObjectStore(ctx, modelUUID)
	})

	creator, err := NewCreator(CreatorConfig{
		MachineTag:                    g.config.ControllerTag,
		ControllerUUID:                g.config.ControllerUUID,
		ControllerModelUUID:           coremodel.UUID(g.config.ControllerModelUUID),
		DataDir:                       g.paths.DataDir,
		LogDir:                        g.paths.LogDir,
		MachineInstanceIDGetter:       g.config.MachineInstanceIDGetter,
		MachineBaseGetter:             g.config.MachineBaseGetter,
		ControllerExport:              controllerServices.ControllerExport(),
		ModelServicesGetter:           modelServicesFor,
		ControllerModelMachineService: controllerServices.Machine(),
		ModelConfig:                   controllerServices.Config(),
		ControllerModelLister:         controllerServices.Controller(),
		ControllerNodes:               controllerServices.ControllerNode(),
		ControllerObjectStore:         controllerStore,
		ObjectStoreForModel:           modelObjectStoreFor,
		Clock:                         g.config.Clock,
		Logger:                        g.config.Logger,
	})
	if err != nil {
		return nil, "", nil, err
	}
	return creator.Create(ctx, notes)
}

// modelExportDomainServices narrows the model services factory for the creator.
type modelExportDomainServices struct{ domainServices services.DomainServices }

func (s modelExportDomainServices) Export() ModelExportService { return s.domainServices.Export() }
