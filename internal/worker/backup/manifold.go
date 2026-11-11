// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/providertracker"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/uuid"
)

// LocalValues contains the current controller filesystem paths.
type LocalValues struct{ DataDir, LogDir string }

// LocalConfigReader reads paths each time the worker starts.
type LocalConfigReader interface{ BackupLocalValues() (LocalValues, error) }

// ManifoldConfig configures the archive creator and its lease service.
// The source recorder wired through NewRecordSource follows the
// controller's substrate: machine controllers record the controller
// machine's base in the archive metadata, Kubernetes controllers the
// controller's provisioning facts.
type ManifoldConfig struct {
	LeaseManagerName   string
	DomainServicesName string
	ObjectStoreName    string

	// ProviderTrackerName supplies the provider factory the Kubernetes
	// source recorder resolves the running controller's provisioning
	// facts from. Only the Kubernetes wiring reads it; machine agents
	// leave it unset.
	ProviderTrackerName string
	ControllerUUID      string
	ControllerModelUUID string
	ControllerNodeID    string
	LocalConfigReader   LocalConfigReader
	Clock               clock.Clock
	Logger              logger.Logger
	NewWorker           func(Config) (worker.Worker, error)

	// NewRecordSource returns the source recorder for the controller's
	// substrate from the configuration and the resources the manifold
	// resolves at start. Machine agents wire MachineRecordSource and
	// Kubernetes agents K8sRecordSource.
	NewRecordSource func(RecordSourceArgs) (RecordSourceFunc, error)
}

// Validate checks the dependencies before the manifold starts.
func (c ManifoldConfig) Validate() error {
	if c.LeaseManagerName == "" {
		return errors.NotValidf("empty LeaseManagerName")
	}
	if c.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if c.ObjectStoreName == "" {
		return errors.NotValidf("empty ObjectStoreName")
	}
	if c.ControllerNodeID == "" {
		return errors.NotValidf("empty ControllerNodeID")
	}
	if _, err := uuid.UUIDFromString(c.ControllerUUID); err != nil {
		return errors.Annotate(err, "parsing controller UUID")
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
	if c.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if c.NewRecordSource == nil {
		return errors.NotValidf("nil NewRecordSource")
	}
	return nil
}

// Manifold outputs the creation function consumed by the API server. The
// source recorder follows the controller's substrate through the recorder
// constructor wired in the configuration: machine controllers record
// their machine base, Kubernetes controllers their provisioning facts
// from the provider tracker.
func Manifold(c ManifoldConfig) dependency.Manifold {
	// Only the Kubernetes source recorder reads the provider tracker, so
	// the manifold inputs it when the configuration names one.
	inputs := []string{c.LeaseManagerName, c.DomainServicesName, c.ObjectStoreName}
	if c.ProviderTrackerName != "" {
		inputs = append(inputs, c.ProviderTrackerName)
	}
	return dependency.Manifold{
		Inputs: inputs,
		Start: func(_ context.Context, getter dependency.Getter) (worker.Worker, error) {
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

			// The controller UUID is parsed once at start; the provider
			// config takes its typed form.
			controllerUUID, err := uuid.UUIDFromString(c.ControllerUUID)
			if err != nil {
				return nil, errors.Annotate(err, "parsing controller UUID")
			}
			// The export services resolver is built once at start: the
			// archive generator and the source recorder resolve model
			// services through it on each use.
			modelServicesFor := ModelServicesForFunc(func(ctx context.Context, modelUUID coremodel.UUID) (ModelExportDomainServices, error) {
				modelServices, err := domainServices.ServicesForModel(ctx, modelUUID)
				if err != nil {
					return nil, err
				}
				return modelExportDomainServices{modelServices}, nil
			})
			recordSource, err := c.NewRecordSource(RecordSourceArgs{
				Config:           c,
				Getter:           getter,
				DomainServices:   domainServices,
				ControllerUUID:   controllerUUID,
				ModelServicesFor: modelServicesFor,
			})
			if err != nil {
				return nil, err
			}
			return c.NewWorker(Config{
				LeaseManager:        leaseManager,
				ControllerUUID:      c.ControllerUUID,
				ControllerModelUUID: c.ControllerModelUUID,
				ControllerNodeID:    c.ControllerNodeID,
				CreateArchive: archiveGenerator{
					config:           c,
					paths:            paths,
					domainServices:   domainServices,
					objectStores:     objectStores,
					modelServicesFor: modelServicesFor,
					recordSource:     recordSource,
				},
				Clock:  c.Clock,
				Logger: c.Logger,
			})
		},
		Output: output,
	}
}

// RecordSourceArgs supplies a substrate recorder constructor with the
// manifold configuration and the resources the manifold resolved at
// start.
type RecordSourceArgs struct {
	// Config is the configuration the manifold started with.
	Config ManifoldConfig
	// Getter resolves the manifold dependencies.
	Getter dependency.Getter
	// DomainServices resolves the controller's domain services.
	DomainServices services.DomainServicesGetter
	// ControllerUUID is the parsed controller UUID.
	ControllerUUID uuid.UUID
	// ModelServicesFor resolves the export services of a model.
	ModelServicesFor ModelServicesForFunc
}

// MachineRecordSource returns the source recorder for a machine
// controller: it records the controller machine's base in the archive
// metadata, which machine recovery requires.
func MachineRecordSource(args RecordSourceArgs) (RecordSourceFunc, error) {
	return machineSourceRecorder(
		args.ModelServicesFor,
		coremodel.UUID(args.Config.ControllerModelUUID),
		args.Config.ControllerNodeID,
	), nil
}

// K8sRecordSource returns the source recorder for a Kubernetes
// controller: it records the controller's provisioning facts in the
// archive metadata, which Kubernetes recovery requires. The facts are
// read through the provider factory resolved from the provider tracker,
// so the configuration must name one.
func K8sRecordSource(args RecordSourceArgs) (RecordSourceFunc, error) {
	if args.Config.ProviderTrackerName == "" {
		return nil, errors.NotValidf("empty ProviderTrackerName")
	}
	var providerFactory providertracker.ProviderFactory
	if err := args.Getter.Get(args.Config.ProviderTrackerName, &providerFactory); err != nil {
		return nil, err
	}
	return k8sSourceRecorder(func(ctx context.Context) (*corebackups.InstanceMetadata, error) {
		// The controller model's cloud spec and config are resolved on
		// each read, so creation stays independent of model readiness
		// until a backup is actually taken.
		controllerServices, err := args.DomainServices.ServicesForModel(ctx, coremodel.UUID(args.Config.ControllerModelUUID))
		if err != nil {
			return nil, err
		}
		spec, err := controllerServices.ModelProvider().GetCloudSpec(ctx)
		if err != nil {
			return nil, err
		}
		cfg, err := controllerServices.Config().ModelConfig(ctx)
		if err != nil {
			return nil, err
		}
		provider, err := providerFactory.EphemeralProviderFromConfig(ctx, providertracker.EphemeralProviderConfig{
			ModelType: coremodel.CAAS, ModelConfig: cfg, CloudSpec: spec, ControllerUUID: args.ControllerUUID,
		})
		if err != nil {
			return nil, err
		}
		reader, ok := provider.(ControllerProvisioningReader)
		if !ok {
			return nil, internalerrors.New("provider cannot read controller provisioning").Add(coreerrors.NotSupported)
		}
		return reader.ReadControllerProvisioning(ctx, args.Config.ControllerNodeID)
	}), nil
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

// archiveGenerator resolves request-scoped services after acquiring the
// lease, then builds the archive creator with the source recorder the
// manifold injected for the controller's substrate. This keeps worker
// startup independent of model and object-store readiness.
type archiveGenerator struct {
	config           ManifoldConfig
	paths            LocalValues
	domainServices   services.DomainServicesGetter
	objectStores     objectstore.ObjectStoreGetter
	modelServicesFor ModelServicesForFunc
	recordSource     RecordSourceFunc
}

// Create creates a backup archive and returns its metadata, its path, and a
// cleanup that removes it.
func (g archiveGenerator) Create(ctx context.Context, notes string) (*corebackups.Metadata, string, func(), error) {
	controllerServices, err := g.domainServices.ServicesForModel(ctx, coremodel.UUID(g.config.ControllerModelUUID))
	if err != nil {
		return nil, "", nil, err
	}

	controllerStore, err := g.objectStores.GetObjectStore(ctx, database.ControllerNS)
	if err != nil {
		return nil, "", nil, err
	}

	modelObjectStoreFor := ObjectStoreForModelFunc(func(ctx context.Context, modelUUID string) (ReadObjectStore, error) {
		return g.objectStores.GetObjectStore(ctx, modelUUID)
	})

	creator, err := newCreator(CreatorConfig{
		ControllerUUID:        g.config.ControllerUUID,
		ControllerModelUUID:   coremodel.UUID(g.config.ControllerModelUUID),
		ControllerID:          g.config.ControllerNodeID,
		DataDir:               g.paths.DataDir,
		LogDir:                g.paths.LogDir,
		ControllerExport:      controllerServices.ControllerExport(),
		ModelServicesFor:      g.modelServicesFor,
		ModelConfig:           controllerServices.Config(),
		Controller:            controllerServices.Controller(),
		ControllerNodes:       controllerServices.ControllerNode(),
		ControllerObjectStore: controllerStore,
		ObjectStoreForModel:   modelObjectStoreFor,
		RecordSource:          g.recordSource,
		Clock:                 g.config.Clock,
		Logger:                g.config.Logger,
	})
	if err != nil {
		return nil, "", nil, err
	}
	return creator.Create(ctx, notes)
}

// modelExportDomainServices narrows the model services factory for the creator.
type modelExportDomainServices struct{ domainServices services.DomainServices }

func (s modelExportDomainServices) Export() ModelExportService { return s.domainServices.Export() }

func (s modelExportDomainServices) Machine() MachineService { return s.domainServices.Machine() }
