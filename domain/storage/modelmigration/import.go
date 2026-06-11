// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/description/v12"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/providertracker"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storage/service"
	"github.com/juju/juju/domain/storage/state"
	"github.com/juju/juju/internal/errors"
	internalstorage "github.com/juju/juju/internal/storage"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
}

// RegisterImportStoragePools registers the storage pool import operation with
// the given coordinator. Storage pools must be imported before applications so
// that application storage directives are able to resolve their pools.
func RegisterImportStoragePools(
	coordinator Coordinator,
	ephemeralProviderConfigGetter providertracker.EphemeralProviderConfigGetter,
	logger logger.Logger,
) {
	coordinator.Add(&importStoragePoolOperation{
		baseImportOperation: baseImportOperation{
			ephemeralProviderConfigGetter: ephemeralProviderConfigGetter,
			logger:                        logger,
		},
	})
}

// RegisterImportStorage registers the storage import operation with the given
// coordinator. This imports storage instances, volumes and filesystems and
// must run after applications (for units) and storage pools are imported.
func RegisterImportStorage(
	coordinator Coordinator,
	ephemeralProviderConfigGetter providertracker.EphemeralProviderConfigGetter,
	logger logger.Logger,
) {
	coordinator.Add(&importStorageOperation{
		baseImportOperation: baseImportOperation{
			ephemeralProviderConfigGetter: ephemeralProviderConfigGetter,
			logger:                        logger,
		},
	})
}

// ImportService provides a subset of the storage domain
// service methods needed for storage pool import.
type ImportService interface {

	// GetStoragePoolsToImport resolves the full set of storage pools to create during
	// model import.
	GetStoragePoolsToImport(ctx context.Context, userPools []description.StoragePool) (
		[]domainstorage.ImportStoragePoolParams,
		[]domainstorage.RecommendedStoragePoolParams,
		error,
	)
	// ImportFilesystemsCAAS imports filesystems for CAAS models. It differs from
	// ImportFilesystemsIAAS in that it must find the persistent volume claim name
	// to be used as the attachment ProviderID.
	ImportFilesystemsCAAS(ctx context.Context, params []domainstorage.ImportFilesystemParams) error

	// ImportFilesystemsIAAS imports filesystems from the provided parameters.
	ImportFilesystemsIAAS(ctx context.Context, args []domainstorage.ImportFilesystemParams) error

	// ImportStoragePools creates new storage pools with the slice
	// of [domainstorage.ImportStoragePoolParams].
	ImportStoragePools(ctx context.Context, pools []domainstorage.ImportStoragePoolParams) error

	// ImportStorageInstances creates new storage instances and storage
	// unit owners if the unit name is provided.
	ImportStorageInstances(ctx context.Context, params []domainstorage.ImportStorageInstanceParams) error

	// ImportVolumes creates new volumes and storage instance volumes.
	ImportVolumes(ctx context.Context, arg []domainstorage.ImportVolumeParams) error

	// SetRecommendedStoragePools persists the set of recommended storage pools
	// that are to be used for a model.
	SetRecommendedStoragePools(ctx context.Context, pools []domainstorage.RecommendedStoragePoolParams) error
}

// ephemeralStorageRegistryGetter creates a storage registry from an ephemeral
// provider. This is used during model import where the model doesn't yet
// have a running provider tracker. The ephemeral provider is constructed
// from the model description's cloud, credentials and model type, ensuring
// the registry matches the model being imported rather than the controller.
type ephemeralStorageRegistryGetter struct {
	factory      providertracker.EphemeralProviderFactory
	configGetter providertracker.EphemeralProviderConfigGetter
}

// GetStorageRegistry returns a storage provider registry by creating an
// ephemeral provider and type-asserting it to ProviderRegistry.
func (g *ephemeralStorageRegistryGetter) GetStorageRegistry(
	ctx context.Context,
) (internalstorage.ProviderRegistry, error) {
	cfg, err := g.configGetter.GetEphemeralProviderConfig(ctx)
	if err != nil {
		return nil, errors.Errorf(
			"getting ephemeral provider config for storage registry: %w", err,
		)
	}
	provider, err := g.factory.EphemeralProviderFromConfig(ctx, cfg)
	if err != nil {
		return nil, errors.Errorf(
			"creating ephemeral provider for storage registry: %w", err,
		)
	}
	registry, ok := provider.(internalstorage.ProviderRegistry)
	if !ok {
		return nil, errors.Errorf(
			"provider type %T does not implement storage.ProviderRegistry",
			provider,
		).Add(coreerrors.NotSupported)
	}
	return registry, nil
}

type baseImportOperation struct {
	modelmigration.BaseOperation

	ephemeralProviderConfigGetter providertracker.EphemeralProviderConfigGetter

	service ImportService
	logger  logger.Logger
}

// setup creates the storage service used by the import operations.
func (i *baseImportOperation) setup(scope modelmigration.Scope) error {
	// Create a storage registry getter from the ephemeral provider.
	// This ensures the registry matches the model being imported, rather than matching
	// by the controller model's cloud type. For example, this is useful when importing
	// a CAAS model to an IAAS controller.
	registryGetter := &ephemeralStorageRegistryGetter{
		factory:      scope.EphemeralProviderFactory(),
		configGetter: i.ephemeralProviderConfigGetter,
	}

	i.service = service.NewImportService(
		state.NewState(scope.ModelDB()),
		i.logger,
		registryGetter,
		providertracker.EphemeralProviderRunnerFromConfig[internalstorage.FilesystemModelMigration](
			scope.EphemeralProviderFactory(), i.ephemeralProviderConfigGetter),
	)
	return nil
}

// importStoragePoolOperation imports the storage pools contained in the model.
// It runs before applications are imported so that application storage
// directives can resolve their pools.
type importStoragePoolOperation struct {
	baseImportOperation
}

// Name returns the name of this operation.
func (i *importStoragePoolOperation) Name() string {
	return "import storage pools"
}

// Setup implements Operation.
func (i *importStoragePoolOperation) Setup(scope modelmigration.Scope) error {
	return i.setup(scope)
}

// Execute the import on the storage pools contained in the model.
func (i *importStoragePoolOperation) Execute(ctx context.Context, model description.Model) error {
	// TODO: Combine the storage pool import calls into a single service call,
	// and stop passing through a description entity into the service layer.
	// This must be done ASAP
	poolsToImport, recommendedPools, err := i.service.GetStoragePoolsToImport(ctx, model.StoragePools())
	if err != nil {
		return errors.Errorf("getting pools to import: %w", err)
	}

	err = i.service.ImportStoragePools(ctx, poolsToImport)
	if err != nil {
		return errors.Errorf("importing storage pools %+v: %w", poolsToImport, err)
	}

	err = i.service.SetRecommendedStoragePools(ctx, recommendedPools)
	if err != nil {
		return errors.Errorf("setting recommended storage pools: %w", err)
	}

	return nil
}

// importStorageOperation imports the storage instances, volumes and filesystems
// contained in the model. It runs after applications (for units) and storage
// pools have been imported.
type importStorageOperation struct {
	baseImportOperation
}

// Name returns the name of this operation.
func (i *importStorageOperation) Name() string {
	return "import storage"
}

// Setup implements Operation.
func (i *importStorageOperation) Setup(scope modelmigration.Scope) error {
	return i.setup(scope)
}

// Execute the import on the storage instances, volumes and filesystems
// contained in the model.
func (i *importStorageOperation) Execute(ctx context.Context, model description.Model) error {
	if err := i.importStorageInstances(ctx, model.Storages()); err != nil {
		return errors.Errorf("importing storage instances: %w", err)
	}

	switch model.Type() {
	case coremodel.CAAS.String():
		err := i.importVolumesAndFilesystemsCAAS(ctx, model.Filesystems(), model.Volumes())
		if err != nil {
			return errors.Errorf("importing CAAS storage: %w", err)
		}
	case coremodel.IAAS.String():
		if err := i.importFilesystemsIAAS(ctx, model.Filesystems()); err != nil {
			return errors.Errorf("importing filesystems: %w", err)
		}

		if err := i.importVolumes(ctx, model.Volumes()); err != nil {
			return errors.Errorf("setting volumes: %w", err)

		}
	}
	return nil
}

func (i *importStorageOperation) importStorageInstances(ctx context.Context, instances []description.Storage) error {
	if len(instances) == 0 {
		return nil
	}

	args, err := transform.SliceOrErr(instances, func(in description.Storage) (domainstorage.ImportStorageInstanceParams, error) {
		if err := in.Validate(); err != nil {
			return domainstorage.ImportStorageInstanceParams{}, err
		}
		owner, _ := in.UnitOwner()
		var pool string
		var size uint64
		constraints, ok := in.Constraints()
		if ok {
			pool = constraints.Pool
			size = constraints.Size
		}
		return domainstorage.ImportStorageInstanceParams{
			StorageName:       in.Name(),
			StorageKind:       in.Kind(),
			StorageInstanceID: in.ID(),
			UnitName:          owner,
			RequestedSizeMiB:  size,
			PoolName:          pool,
			AttachedUnitNames: in.Attachments(),
		}, nil
	})

	if err != nil {
		return err
	}

	return i.service.ImportStorageInstances(ctx, args)
}

func (i *importStorageOperation) importFilesystemsIAAS(ctx context.Context, filesystems []description.Filesystem) error {
	if len(filesystems) == 0 {
		return nil
	}

	args := transform.Slice(filesystems, func(in description.Filesystem) domainstorage.ImportFilesystemParams {
		return domainstorage.ImportFilesystemParams{
			ID:                in.ID(),
			SizeInMiB:         in.Size(),
			ProviderID:        in.FilesystemID(),
			PoolName:          in.Pool(),
			StorageInstanceID: in.Storage(),
			Attachments: transform.Slice(
				in.Attachments(),
				func(a description.FilesystemAttachment) domainstorage.ImportFilesystemAttachmentsParams {
					hostMachine, _ := a.HostMachine()
					hostUnit, _ := a.HostUnit()
					return domainstorage.ImportFilesystemAttachmentsParams{
						HostMachineName: hostMachine,
						HostUnitName:    hostUnit,
						MountPoint:      a.MountPoint(),
						ReadOnly:        a.ReadOnly(),
					}
				},
			),
		}
	})

	return i.service.ImportFilesystemsIAAS(ctx, args)
}

// importVolumesAndFilesystemsCAAS imports CAAS storage a filesystems and
// filesystem attachments. In previous version of juju these were volumes
// and filesystems.
func (i *importStorageOperation) importVolumesAndFilesystemsCAAS(
	ctx context.Context,
	filesystems []description.Filesystem,
	volumes []description.Volume) error {
	if len(filesystems) == 0 && len(volumes) == 0 {
		return nil
	}
	if (len(filesystems) == 0) != (len(volumes) == 0) {
		return errors.Errorf("volumes and filesystems must both exist or not exist: %d volumes,"+
			" %d filesystems", len(volumes), len(filesystems))
	}

	convertedFS := transform.SliceToMap(volumes, func(in description.Volume) (
		string, domainstorage.ImportFilesystemParams) {
		return in.ID(), domainstorage.ImportFilesystemParams{
			SizeInMiB:         in.Size(),
			ProviderID:        in.VolumeID(),
			PoolName:          in.Pool(),
			StorageInstanceID: in.Storage(),
		}
	})

	params, err := transform.SliceOrErr(
		filesystems,
		func(in description.Filesystem) (domainstorage.ImportFilesystemParams, error,
		) {
			fs, ok := convertedFS[in.Volume()]
			if !ok {
				return domainstorage.ImportFilesystemParams{},
					errors.Errorf("could not find volume %q for filesystem %q", in.Volume(), in.ID())
			}

			attachments, err := getCAASFilesystemAttachment(in.FilesystemID(), in.Attachments())
			if err != nil {
				return domainstorage.ImportFilesystemParams{},
					errors.Errorf("%q: %w", in.ID(), err)
			}

			fs.ID = in.ID()
			fs.Attachments = attachments
			return fs, nil
		})
	if err != nil {
		return errors.Errorf("converting CAAS filesystem attachments for import: %w", err)
	}

	return i.service.ImportFilesystemsCAAS(ctx, params)
}

func getCAASFilesystemAttachment(
	filesystemID string,
	attachments []description.FilesystemAttachment,
) ([]domainstorage.ImportFilesystemAttachmentsParams, error) {
	if len(attachments) > 1 {
		// Attachments are being converted to filesystems during import,
		// a storage instance is only allowed one filesystem per the DDL.
		return nil, errors.Errorf("filesystem has more than one attachment")
	}
	if len(attachments) != 1 {
		return nil, nil
	}
	attachment := attachments[0]

	hostUnit, _ := attachment.HostUnit()
	return []domainstorage.ImportFilesystemAttachmentsParams{
		{
			HostUnitName: hostUnit,
			MountPoint:   attachment.MountPoint(),
			ReadOnly:     attachment.ReadOnly(),
			ProviderID:   filesystemID,
		},
	}, nil
}

func (i *importStorageOperation) importVolumes(ctx context.Context, volumes []description.Volume) error {
	if len(volumes) == 0 {
		return nil
	}

	args := make([]domainstorage.ImportVolumeParams, len(volumes))
	for k, volume := range volumes {
		attachments := volume.Attachments()
		plans := volume.AttachmentPlans()
		vol := domainstorage.ImportVolumeParams{
			ID:                volume.ID(),
			StorageInstanceID: volume.Storage(),
			Provisioned:       volume.Provisioned(),
			SizeMiB:           volume.Size(),
			Pool:              volume.Pool(),
			HardwareID:        volume.HardwareID(),
			WWN:               volume.WWN(),
			ProviderID:        volume.VolumeID(),
			Persistent:        volume.Persistent(),
			Attachments:       make([]domainstorage.ImportVolumeAttachmentParams, len(attachments)),
			AttachmentPlans:   make([]domainstorage.ImportVolumeAttachmentPlanParams, len(plans)),
		}

		for j, attach := range attachments {
			// Volumes can only be attached to machines. Import of CAAS storage
			// will be handled in a separate step.
			machineID, _ := attach.HostMachine()
			vol.Attachments[j] = domainstorage.ImportVolumeAttachmentParams{
				HostMachineName: machineID,
				Provisioned:     attach.Provisioned(),
				ReadOnly:        attach.ReadOnly(),
				DeviceName:      attach.DeviceName(),
				DeviceLink:      attach.DeviceLink(),
				BusAddress:      attach.BusAddress(),
			}
		}

		for p, plan := range plans {
			var (
				deviceType       string
				deviceAttributes map[string]string
			)
			if info := plan.VolumePlanInfo(); info != nil {
				deviceType = info.DeviceType()
				deviceAttributes = info.DeviceAttributes()
			}
			vol.AttachmentPlans[p] = domainstorage.ImportVolumeAttachmentPlanParams{
				HostMachineName:  plan.Machine(),
				DeviceType:       deviceType,
				DeviceAttributes: deviceAttributes,
			}
		}
		args[k] = vol
	}

	return i.service.ImportVolumes(ctx, args)
}
