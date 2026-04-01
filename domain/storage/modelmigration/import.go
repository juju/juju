// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/description/v12"

	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/providertracker"
	corestorage "github.com/juju/juju/core/storage"
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

// RegisterImport registers the import operations with the given coordinator.
func RegisterImport(
	coordinator Coordinator,
	storageRegistryGetter corestorage.ModelStorageRegistryGetter,
	ephemeralProviderConfigGetter providertracker.EphemeralProviderConfigGetter,
	logger logger.Logger,
) {
	coordinator.Add(&importOperation{
		ephemeralProviderConfigGetter: ephemeralProviderConfigGetter,
		storageRegistryGetter:         storageRegistryGetter,
		logger:                        logger,
	})
}

// ImportService provides a subset of the storage domain
// service methods needed for storage pool import.
type ImportService interface {

	// ImportFilesystemsCAAS imports filesystems for CAAS models. It differs from
	// ImportFilesystemsIAAS in that it must find the persistent volume claim name
	// to be used as the attachment ProviderID.
	ImportFilesystemsCAAS(ctx context.Context, params []domainstorage.ImportFilesystemParams) error

	// ImportFilesystemsIAAS imports filesystems from the provided parameters.
	ImportFilesystemsIAAS(ctx context.Context, args []domainstorage.ImportFilesystemParams) error

	// ImportStoragePools creates new storage pools with the slice
	// of [domainstorage.UserStoragePoolParams].
	ImportStoragePools(ctx context.Context, pools []domainstorage.UserStoragePoolParams) error

	// ImportStorageInstances creates new storage instances and storage
	// unit owners if the unit name is provided.
	ImportStorageInstances(ctx context.Context, params []domainstorage.ImportStorageInstanceParams) error

	// ImportVolumes creates new volumes and storage instance volumes.
	ImportVolumes(ctx context.Context, arg []domainstorage.ImportVolumeParams) error
}

type importOperation struct {
	modelmigration.BaseOperation

	ephemeralProviderConfigGetter providertracker.EphemeralProviderConfigGetter
	storageRegistryGetter         corestorage.ModelStorageRegistryGetter

	service ImportService
	logger  logger.Logger
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import storage"
}

// Setup implements Operation.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewImportService(
		state.NewState(scope.ModelDB()),
		i.logger,
		i.storageRegistryGetter,
		providertracker.EphemeralProviderRunnerFromConfig[internalstorage.FilesystemModelMigration](
			scope.EphemeralProviderFactory(), i.ephemeralProviderConfigGetter),
	)
	return nil
}

// Execute the import on the storage pools contained in the model.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	if err := i.importStoragePools(ctx, model.StoragePools()); err != nil {
		return errors.Errorf("importing storage pools: %w", err)
	}

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

func (i *importOperation) importStorageInstances(ctx context.Context, instances []description.Storage) error {
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

func (i *importOperation) importFilesystemsIAAS(ctx context.Context, filesystems []description.Filesystem) error {
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
func (i *importOperation) importVolumesAndFilesystemsCAAS(
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

func (i *importOperation) importVolumes(ctx context.Context, volumes []description.Volume) error {
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

func (i *importOperation) importStoragePools(ctx context.Context, pools []description.StoragePool) error {
	args := transform.Slice(pools, func(in description.StoragePool) domainstorage.UserStoragePoolParams {
		return domainstorage.UserStoragePoolParams{
			Name:       in.Name(),
			Provider:   in.Provider(),
			Attributes: in.Attributes(),
		}
	})
	return i.service.ImportStoragePools(ctx, args)
}
