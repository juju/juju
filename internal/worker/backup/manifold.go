// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
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
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/environs/cloudspec"
	environsconfig "github.com/juju/juju/environs/config"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/uuid"
)

// LocalValues contains the current controller filesystem paths.
type LocalValues struct{ DataDir, LogDir string }

// LocalConfigReader reads paths each time the worker starts.
type LocalConfigReader interface{ BackupLocalValues() (LocalValues, error) }

// ManifoldConfig configures the archive creator and its lease service.
type ManifoldConfig struct {
	LeaseManagerName    string
	DomainServicesName  string
	ObjectStoreName     string
	ProviderTrackerName string
	ControllerUUID      string
	ControllerModelUUID string
	ControllerNodeID    string
	// ControllerTag identifies the agent running the controller: a
	// machine tag on machine controllers, a controller-agent tag on
	// Kubernetes controllers. It selects the creator the manifold wires.
	ControllerTag     names.Tag
	LocalConfigReader LocalConfigReader
	Clock             clock.Clock
	Logger            logger.Logger
	NewWorker         func(Config) (worker.Worker, error)
	// NewMachineCreator returns the archive creator for a machine
	// controller, which records the controller machine's base. It is
	// wired when ControllerTag is a machine tag.
	NewMachineCreator CreatorConstructor
	// NewK8sCreator returns the archive creator for a Kubernetes
	// controller, which records the controller's provisioning facts. It
	// is wired, with the provisioning read resolved from the provider
	// tracker, when ControllerTag is a controller-agent tag.
	NewK8sCreator K8sCreatorConstructor
}

// Validate checks dependencies before the manifold starts.
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
	if c.ControllerTag == nil {
		return errors.NotValidf("nil ControllerTag")
	}
	switch c.ControllerTag.(type) {
	case names.MachineTag, names.ControllerAgentTag:
	default:
		return errors.NotValidf("ControllerTag kind %T", c.ControllerTag)
	}
	if !isMachineControllerTag(c.ControllerTag) && c.ProviderTrackerName == "" {
		return errors.NotValidf("empty ProviderTrackerName")
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
	if c.NewMachineCreator == nil {
		return errors.NotValidf("nil NewMachineCreator")
	}
	if c.NewK8sCreator == nil {
		return errors.NotValidf("nil NewK8sCreator")
	}
	return nil
}

// isMachineControllerTag reports whether tag identifies a machine
// controller. Kubernetes controllers use a controller-agent tag instead.
func isMachineControllerTag(tag names.Tag) bool {
	_, ok := tag.(names.MachineTag)
	return ok
}

// Manifold outputs the creation function consumed by the API server. The
// creator follows the controller's substrate: machine controllers record
// their machine base, Kubernetes controllers record their provisioning
// facts from the provider tracker.
func Manifold(c ManifoldConfig) dependency.Manifold {
	machineController := isMachineControllerTag(c.ControllerTag)
	inputs := []string{
		c.LeaseManagerName,
		c.DomainServicesName,
		c.ObjectStoreName,
	}
	// Only the Kubernetes creator reads the provider tracker: machine
	// controllers do not depend on it.
	if !machineController {
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

			// The manifold, not the archive generator, knows the creator
			// to build: the machine creator, or the Kubernetes creator
			// with the provisioning read resolved from the provider
			// tracker.
			newCreator := c.NewMachineCreator
			if !machineController {
				var providerFactory providertracker.ProviderFactory
				if err := getter.Get(c.ProviderTrackerName, &providerFactory); err != nil {
					return nil, err
				}
				readProvisioning := k8sProvisioningReader(domainServices, providerFactory, controllerUUID, c)
				newCreator = func(cfg CreatorConfig) (corebackups.BackupCreator, error) {
					return c.NewK8sCreator(K8sCreatorConfig{
						CreatorConfig:          cfg,
						ControllerProvisioning: readProvisioning,
					})
				}
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
					newCreator:     newCreator,
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

// archiveGenerator resolves request-scoped services after acquiring the
// lease, then delegates archive creation to the creator the manifold
// injected. This keeps worker startup independent of model and
// object-store readiness.
type archiveGenerator struct {
	config         ManifoldConfig
	paths          LocalValues
	domainServices services.DomainServicesGetter
	objectStores   objectstore.ObjectStoreGetter
	newCreator     CreatorConstructor
}

// cloudSpecProvider provides the cloud spec for the controller model.
type cloudSpecProvider interface {
	GetCloudSpec(context.Context) (cloudspec.CloudSpec, error)
}

// modelConfigProvider provides the controller model config.
type modelConfigProvider interface {
	ModelConfig(context.Context) (*environsconfig.Config, error)
}

// Create creates a backup archive and returns its metadata, its path, and a
// cleanup that removes it.
func (g archiveGenerator) Create(ctx context.Context, notes string) (*corebackups.Metadata, string, func(), error) {
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

	modelObjectStoreFor := ObjectStoreForModelFunc(func(ctx context.Context, modelUUID string) (ReadObjectStore, error) {
		return g.objectStores.GetObjectStore(ctx, modelUUID)
	})

	creator, err := g.newCreator(CreatorConfig{
		ControllerUUID:        g.config.ControllerUUID,
		ControllerModelUUID:   coremodel.UUID(g.config.ControllerModelUUID),
		ControllerID:          g.config.ControllerNodeID,
		DataDir:               g.paths.DataDir,
		LogDir:                g.paths.LogDir,
		ControllerExport:      controllerServices.ControllerExport(),
		ModelServicesFor:      modelServicesFor,
		ModelConfig:           controllerServices.Config(),
		Controller:            controllerServices.Controller(),
		ControllerNodes:       controllerServices.ControllerNode(),
		ControllerObjectStore: controllerStore,
		ObjectStoreForModel:   modelObjectStoreFor,
		Clock:                 g.config.Clock,
		Logger:                g.config.Logger,
	})
	if err != nil {
		return nil, "", nil, err
	}
	return creator.Create(ctx, notes)
}

// k8sProvisioningReader returns the provisioning read the Kubernetes
// creator records in the archive metadata. The controller model's cloud
// spec and config are resolved on each read, so creation stays
// independent of model readiness until a backup is actually taken.
func k8sProvisioningReader(
	domainServices services.DomainServicesGetter,
	providerFactory providertracker.ProviderFactory,
	controllerUUID uuid.UUID,
	config ManifoldConfig,
) func(context.Context) (*corebackups.InstanceMetadata, error) {
	return func(ctx context.Context) (*corebackups.InstanceMetadata, error) {
		controllerServices, err := domainServices.ServicesForModel(ctx, coremodel.UUID(config.ControllerModelUUID))
		if err != nil {
			return nil, err
		}
		return readControllerProvisioning(ctx,
			controllerServices.ModelProvider(), controllerServices.Config(),
			providerFactory, controllerUUID, config.ControllerNodeID)
	}
}

// readControllerProvisioning reads the running controller's provisioning
// facts from the provider. It is only used on Kubernetes controllers.
func readControllerProvisioning(
	ctx context.Context,
	modelProvider cloudSpecProvider,
	configProvider modelConfigProvider,
	providerFactory providertracker.ProviderFactory,
	controllerUUID uuid.UUID,
	controllerNodeID string,
) (*corebackups.InstanceMetadata, error) {
	spec, err := modelProvider.GetCloudSpec(ctx)
	if err != nil {
		return nil, err
	}
	cfg, err := configProvider.ModelConfig(ctx)
	if err != nil {
		return nil, err
	}
	provider, err := providerFactory.EphemeralProviderFromConfig(ctx, providertracker.EphemeralProviderConfig{
		ModelType: coremodel.CAAS, ModelConfig: cfg, CloudSpec: spec, ControllerUUID: controllerUUID,
	})
	if err != nil {
		return nil, err
	}
	reader, ok := provider.(ControllerProvisioningReader)
	if !ok {
		return nil, internalerrors.New("provider cannot read controller provisioning").Add(coreerrors.NotSupported)
	}
	return reader.ReadControllerProvisioning(ctx, controllerNodeID)
}

// modelExportDomainServices narrows the model services factory for the creator.
type modelExportDomainServices struct{ domainServices services.DomainServices }

func (s modelExportDomainServices) Export() ModelExportService { return s.domainServices.Export() }

func (s modelExportDomainServices) Machine() MachineService { return s.domainServices.Machine() }
