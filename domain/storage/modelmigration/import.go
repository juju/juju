// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/description/v11"

	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	corestorage "github.com/juju/juju/core/storage"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/domain/storage/service"
	"github.com/juju/juju/domain/storage/state"
	"github.com/juju/juju/internal/errors"
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
	clock clock.Clock,
	logger logger.Logger,
) {
	coordinator.Add(&importOperation{
		storageRegistryGetter: storageRegistryGetter,
		clock:                 clock,
		logger:                logger,
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

type importOperation struct {
	modelmigration.BaseOperation

	storageRegistryGetter corestorage.ModelStorageRegistryGetter
	service               ImportService
	clock                 clock.Clock
	logger                logger.Logger
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import storage"
}

// Setup implements Operation.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(
		state.NewState(scope.ModelDB()),
		i.logger,
		i.clock,
		i.storageRegistryGetter,
	)
	return nil
}

// Execute the import on the storage pools contained in the model.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
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

	if err := i.importStorageInstances(ctx, model.Storages()); err != nil {
		return errors.Errorf("importing storage instances: %w", err)
	}

	// Filesystems and Volumes need to be handled differently for CAAS models. Until
	// this is implemented skip the import step.
	if model.Type() == coremodel.IAAS.String() {
		if err := i.importFilesystemsIAAS(ctx, model.Filesystems()); err != nil {
			return errors.Errorf("importing filesystems: %w", err)
		}

		if err := i.importVolumes(ctx, model.Volumes()); err != nil {
			return errors.Errorf("importing volumes: %w", err)
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
