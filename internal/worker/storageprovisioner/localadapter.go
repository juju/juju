// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"

	coreblockdevice "github.com/juju/juju/core/blockdevice"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainblockdevice "github.com/juju/juju/domain/blockdevice"
	blockdeviceerrors "github.com/juju/juju/domain/blockdevice/errors"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/rpc/params"
)

// storageProvisioningService is the subset of the domain storage provisioning
// service needed by the model storage adapters.
type storageProvisioningService interface {
	// Volume operations
	GetVolumeUUIDForID(ctx context.Context, volumeID string) (domainstorage.VolumeUUID, error)
	GetVolumeByID(ctx context.Context, volumeID string) (storageprovisioning.Volume, error)
	GetVolumeLife(ctx context.Context, uuid domainstorage.VolumeUUID) (domainlife.Life, error)
	GetVolumeParams(ctx context.Context, uuid domainstorage.VolumeUUID) (storageprovisioning.VolumeParams, error)
	GetVolumeRemovalParams(ctx context.Context, uuid domainstorage.VolumeUUID) (storageprovisioning.VolumeRemovalParams, error)
	SetVolumeProvisionedInfo(ctx context.Context, volumeID string, info storageprovisioning.VolumeProvisionedInfo) error

	// Filesystem operations
	GetFilesystemUUIDForID(ctx context.Context, filesystemID string) (domainstorage.FilesystemUUID, error)
	GetFilesystemForID(ctx context.Context, filesystemID string) (storageprovisioning.Filesystem, error)
	GetFilesystemLife(ctx context.Context, uuid domainstorage.FilesystemUUID) (domainlife.Life, error)
	GetFilesystemParams(ctx context.Context, uuid domainstorage.FilesystemUUID) (storageprovisioning.FilesystemParams, error)
	GetFilesystemRemovalParams(ctx context.Context, uuid domainstorage.FilesystemUUID) (storageprovisioning.FilesystemRemovalParams, error)
	SetFilesystemProvisionedInfo(ctx context.Context, filesystemID string, info storageprovisioning.FilesystemProvisionedInfo) error

	// Volume attachment operations
	GetVolumeAttachmentUUIDForVolumeIDMachine(ctx context.Context, volumeID string, machineUUID machine.UUID) (domainstorage.VolumeAttachmentUUID, error)
	GetVolumeAttachment(ctx context.Context, uuid domainstorage.VolumeAttachmentUUID) (storageprovisioning.VolumeAttachment, error)
	GetVolumeAttachmentLife(ctx context.Context, uuid domainstorage.VolumeAttachmentUUID) (domainlife.Life, error)
	GetVolumeAttachmentParams(ctx context.Context, uuid domainstorage.VolumeAttachmentUUID) (storageprovisioning.VolumeAttachmentParams, error)
	SetVolumeAttachmentProvisionedInfo(ctx context.Context, uuid domainstorage.VolumeAttachmentUUID, info storageprovisioning.VolumeAttachmentProvisionedInfo) error

	// Filesystem attachment operations
	GetFilesystemAttachmentUUIDForFilesystemIDMachine(ctx context.Context, filesystemID string, machineUUID machine.UUID) (domainstorage.FilesystemAttachmentUUID, error)
	GetFilesystemAttachmentUUIDForFilesystemIDUnit(ctx context.Context, filesystemID string, unitUUID coreunit.UUID) (domainstorage.FilesystemAttachmentUUID, error)
	GetFilesystemAttachmentForMachine(ctx context.Context, filesystemID string, machineUUID machine.UUID) (storageprovisioning.FilesystemAttachment, error)
	GetFilesystemAttachmentForUnit(ctx context.Context, filesystemID string, unitUUID coreunit.UUID) (storageprovisioning.FilesystemAttachment, error)
	GetFilesystemAttachmentLife(ctx context.Context, uuid domainstorage.FilesystemAttachmentUUID) (domainlife.Life, error)
	GetFilesystemAttachmentParams(ctx context.Context, uuid domainstorage.FilesystemAttachmentUUID) (storageprovisioning.FilesystemAttachmentParams, error)
	SetFilesystemAttachmentProvisionedInfoForMachine(ctx context.Context, filesystemID string, machineUUID machine.UUID, info storageprovisioning.FilesystemAttachmentProvisionedInfo) error
	SetFilesystemAttachmentProvisionedInfoForUnit(ctx context.Context, filesystemID string, unitUUID coreunit.UUID, info storageprovisioning.FilesystemAttachmentProvisionedInfo) error

	// Attachment ID lookups
	GetVolumeAttachmentIDs(ctx context.Context, volumeAttachmentUUIDs []string) (map[string]storageprovisioning.VolumeAttachmentID, error)
	GetFilesystemAttachmentIDs(ctx context.Context, filesystemAttachmentUUIDs []string) (map[string]storageprovisioning.FilesystemAttachmentID, error)

	// Block device for volume attachment
	GetBlockDeviceForVolumeAttachment(ctx context.Context, uuid domainstorage.VolumeAttachmentUUID) (domainblockdevice.BlockDeviceUUID, error)

	// Model resource tags
	GetStorageResourceTagsForModel(ctx context.Context) (map[string]string, error)

	// Watchers (model-scoped)
	WatchModelProvisionedVolumes(ctx context.Context) (watcher.StringsWatcher, error)
	WatchModelProvisionedFilesystems(ctx context.Context) (watcher.StringsWatcher, error)
	WatchModelProvisionedVolumeAttachments(ctx context.Context) (watcher.StringsWatcher, error)
	WatchModelProvisionedFilesystemAttachments(ctx context.Context) (watcher.StringsWatcher, error)

	// Watchers (machine-scoped)
	WatchMachineProvisionedVolumes(ctx context.Context, machineUUID machine.UUID) (watcher.StringsWatcher, error)
	WatchMachineProvisionedFilesystems(ctx context.Context, machineUUID machine.UUID) (watcher.StringsWatcher, error)
	WatchMachineProvisionedVolumeAttachments(ctx context.Context, machineUUID machine.UUID) (watcher.StringsWatcher, error)
	WatchMachineProvisionedFilesystemAttachments(ctx context.Context, machineUUID machine.UUID) (watcher.StringsWatcher, error)
	WatchVolumeAttachmentPlans(ctx context.Context, machineUUID machine.UUID) (watcher.StringsWatcher, error)

	// Volume attachment plans
	GetVolumeAttachmentPlanUUIDForVolumeIDMachine(ctx context.Context, volumeID string, machineUUID machine.UUID) (domainstorage.VolumeAttachmentPlanUUID, error)
	GetVolumeAttachmentPlan(ctx context.Context, uuid domainstorage.VolumeAttachmentPlanUUID) (storageprovisioning.VolumeAttachmentPlan, error)
	CreateVolumeAttachmentPlan(ctx context.Context, attachmentUUID domainstorage.VolumeAttachmentUUID, deviceType domainstorage.VolumeDeviceType, attrs map[string]string) (domainstorage.VolumeAttachmentPlanUUID, error)
	SetVolumeAttachmentPlanProvisionedInfo(ctx context.Context, uuid domainstorage.VolumeAttachmentPlanUUID, info storageprovisioning.VolumeAttachmentPlanProvisionedInfo) error
	SetVolumeAttachmentPlanProvisionedBlockDevice(ctx context.Context, uuid domainstorage.VolumeAttachmentPlanUUID, blockDeviceUUID domainblockdevice.BlockDeviceUUID) error
}

// machineService is the subset of the domain machine service needed by the
// model storage adapters.
type machineService interface {
	GetMachineUUID(ctx context.Context, machineName machine.Name) (machine.UUID, error)
	GetInstanceID(ctx context.Context, machineUUID machine.UUID) (instance.Id, error)
	WatchMachineCloudInstances(ctx context.Context, machineUUID machine.UUID) (watcher.NotifyWatcher, error)
}

// applicationService is the subset of the domain application service needed
// by the model storage adapters.
type applicationService interface {
	GetUnitUUID(ctx context.Context, unitName coreunit.Name) (coreunit.UUID, error)
}

// removalService is the subset of the domain removal service needed by the
// model storage adapters.
type removalService interface {
	RemoveDeadVolume(ctx context.Context, uuid domainstorage.VolumeUUID) error
	RemoveDeadFilesystem(ctx context.Context, uuid domainstorage.FilesystemUUID) error
	MarkVolumeAttachmentAsDead(ctx context.Context, uuid domainstorage.VolumeAttachmentUUID) error
	MarkFilesystemAttachmentAsDead(ctx context.Context, uuid domainstorage.FilesystemAttachmentUUID) error
	MarkVolumeAttachmentPlanAsDead(ctx context.Context, uuid domainstorage.VolumeAttachmentPlanUUID) error
}

// storageStatusService is the subset of the domain status service needed by
// the model storage adapters.
type storageStatusService interface {
	SetVolumeStatus(ctx context.Context, volumeID string, sInfo status.StatusInfo) error
	SetFilesystemStatus(ctx context.Context, filesystemID string, sInfo status.StatusInfo) error
}

// blockDeviceService is the subset of the domain block device service needed
// by the model storage adapters.
type blockDeviceService interface {
	WatchBlockDevicesForMachine(ctx context.Context, machineUUID machine.UUID) (watcher.NotifyWatcher, error)
	GetBlockDevice(ctx context.Context, uuid domainblockdevice.BlockDeviceUUID) (coreblockdevice.BlockDevice, error)
	MatchOrCreateBlockDevice(ctx context.Context, machineUUID machine.UUID, device coreblockdevice.BlockDevice) (domainblockdevice.BlockDeviceUUID, error)
}

// modelStorageAdapter implements VolumeAccessor, FilesystemAccessor,
// LifecycleManager, MachineAccessor, and StatusSetter by reading from
// domain services directly instead of through an API facade.
//
// Error handling and message wording in this adapter mirrors the server-side
// facade implementation at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go
//
// so that the storage provisioner worker observes the same error codes
// (params.CodeNotFound, params.CodeNotProvisioned, etc.) and message
// strings it would have received over the RPC API.
type modelStorageAdapter struct {
	storageSvc     storageProvisioningService
	machineSvc     machineService
	appSvc         applicationService
	removalSvc     removalService
	statusSvc      storageStatusService
	blockDeviceSvc blockDeviceService
	clock          clock.Clock
}

// errPerm mirrors apiservererrors.ErrPerm used by the facade for permission
// denied errors. The facade converts VolumeNotFound/FilesystemNotFound to
// ErrPerm in VolumeParams/FilesystemParams to avoid leaking entity
// existence to unauthorized callers. See facade:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go
//	(VolumeParams ~line 1611, FilesystemParams ~line 1811)
var errPerm = jujuerrors.ConstError("permission denied")

// errPermResult is the *params.Error form of errPerm, matching the
// CodeUnauthorized code that apiservererrors.ServerError assigns to
// apiservererrors.ErrPerm.
var errPermResult = &params.Error{
	Message: "permission denied",
	Code:    params.CodeUnauthorized,
}

// serverError converts a Go error into a *params.Error, mapping error
// codes in the same way as apiservererrors.ServerError used by the facade
// at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go
//
// Only the error code categories used by the storage provisioner facade
// are handled; all other errors receive an empty code (CodeUnknown),
// matching the default branch of apiservererrors.ServerError.
func serverError(err error) *params.Error {
	if err == nil {
		return nil
	}
	code := ""
	switch {
	case errors.Is(err, errPerm):
		code = params.CodeUnauthorized
	case errors.Is(err, coreerrors.NotFound):
		code = params.CodeNotFound
	case errors.Is(err, coreerrors.NotProvisioned):
		code = params.CodeNotProvisioned
	case errors.Is(err, coreerrors.NotValid):
		code = params.CodeNotValid
	case errors.Is(err, coreerrors.NotImplemented):
		code = params.CodeNotImplemented
	case errors.Is(err, coreerrors.NotSupported):
		code = params.CodeNotSupported
	}
	return &params.Error{
		Message: err.Error(),
		Code:    code,
	}
}

// getMachineUUID translates a machine tag into a machine UUID, mapping
// MachineNotFound to CodeNotFound. Mirrors facade helper getMachineUUID at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:3117
func (a *modelStorageAdapter) getMachineUUID(
	ctx context.Context, machineTag names.MachineTag,
) (machine.UUID, error) {
	machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(machineTag.Id()))
	if errors.Is(err, machineerrors.MachineNotFound) {
		return "", errors.Errorf(
			"machine %q not found", machineTag.Id(),
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return "", errors.Capture(err)
	}
	return machineUUID, nil
}

// getVolumeAttachmentUUID translates a volume tag + machine UUID into a
// volume attachment UUID, mapping domain sentinels to CodeNotFound.
// Mirrors facade helper getVolumeAttachmentUUID at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:2743
func (a *modelStorageAdapter) getVolumeAttachmentUUID(
	ctx context.Context, volTag names.VolumeTag, machineUUID machine.UUID,
) (domainstorage.VolumeAttachmentUUID, error) {
	rval, err := a.storageSvc.GetVolumeAttachmentUUIDForVolumeIDMachine(
		ctx, volTag.Id(), machineUUID,
	)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return "", errors.Errorf(
			"machine %q not found", machineUUID,
		).Add(coreerrors.NotFound)
	} else if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
		return "", errors.Errorf(
			"volume attachment %q on %q not found", volTag.Id(), machineUUID,
		).Add(coreerrors.NotFound)
	} else if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
		return "", errors.Errorf(
			"volume %q not found for attachment on %q", volTag.Id(), machineUUID,
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return "", errors.Errorf(
			"getting volume attachment uuid for %q on %q: %w",
			volTag.Id(), machineUUID, err,
		)
	}
	return rval, nil
}

// getVolumeAttachmentPlanUUID translates a volume tag + machine UUID into a
// volume attachment plan UUID, mapping domain sentinels to CodeNotFound.
// Mirrors facade helper getVolumeAttachmentPlanUUID at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:1027
func (a *modelStorageAdapter) getVolumeAttachmentPlanUUID(
	ctx context.Context, volumeTag names.VolumeTag, machineUUID machine.UUID,
) (domainstorage.VolumeAttachmentPlanUUID, error) {
	vapUUID, err := a.storageSvc.GetVolumeAttachmentPlanUUIDForVolumeIDMachine(
		ctx, volumeTag.Id(), machineUUID,
	)
	if errors.Is(err, storageprovisioningerrors.VolumeAttachmentPlanNotFound) {
		return "", errors.Errorf(
			"volume attachment plan %q on machine %q not found",
			volumeTag.Id(), machineUUID,
		).Add(coreerrors.NotFound)
	} else if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
		return "", errors.Errorf(
			"volume %q not found", volumeTag.Id(),
		).Add(coreerrors.NotFound)
	} else if errors.Is(err, machineerrors.MachineNotFound) {
		return "", errors.Errorf(
			"machine %q not found", machineUUID,
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return "", errors.Errorf(
			"getting volume attachment plan %q on machine %q: %v",
			volumeTag.Id(), machineUUID, err,
		)
	}
	return vapUUID, nil
}

// getFilesystemAttachmentUUID translates a filesystem tag + host tag into a
// filesystem attachment UUID, mapping domain sentinels to CodeNotFound.
// Mirrors facade helper getFilesystemAttachmentUUID at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:2680
func (a *modelStorageAdapter) getFilesystemAttachmentUUID(
	ctx context.Context, fsTag names.FilesystemTag, hostTag names.Tag,
) (domainstorage.FilesystemAttachmentUUID, error) {
	errHandler := func(err error) error {
		switch {
		case errors.Is(err, applicationerrors.UnitNotFound):
			return errors.Errorf(
				"unit %q not found", hostTag.Id(),
			).Add(coreerrors.NotFound)
		case errors.Is(err, machineerrors.MachineNotFound):
			return errors.Errorf(
				"machine %q not found", hostTag.Id(),
			).Add(coreerrors.NotFound)
		case errors.Is(err, storageprovisioningerrors.FilesystemAttachmentNotFound):
			return errors.Errorf(
				"filesystem attachment %q on %q not found", fsTag.Id(), hostTag.Id(),
			).Add(coreerrors.NotFound)
		case errors.Is(err, storageprovisioningerrors.FilesystemNotFound):
			return errors.Errorf(
				"filesystem %q not found for attachment on %q", fsTag.Id(), hostTag.Id(),
			).Add(coreerrors.NotFound)
		case err != nil:
			return errors.Errorf(
				"getting filesystem attachment UUID for %q on %q: %w",
				fsTag.Id(), hostTag.Id(), err,
			)
		}
		return nil
	}

	var rval domainstorage.FilesystemAttachmentUUID
	switch tag := hostTag.(type) {
	case names.MachineTag:
		machineUUID, err := a.getMachineUUID(ctx, tag)
		if err != nil {
			return "", errors.Capture(err)
		}
		rval, err = a.storageSvc.GetFilesystemAttachmentUUIDForFilesystemIDMachine(
			ctx, fsTag.Id(), machineUUID,
		)
		if err != nil {
			return "", errHandler(err)
		}
	case names.UnitTag:
		unitUUID, err := a.getUnitUUID(ctx, tag)
		if err != nil {
			return "", errors.Capture(err)
		}
		rval, err = a.storageSvc.GetFilesystemAttachmentUUIDForFilesystemIDUnit(
			ctx, fsTag.Id(), unitUUID,
		)
		if err != nil {
			return "", errHandler(err)
		}
	default:
		return "", errors.Errorf(
			"filesystem attachment host tag %q is not a valid", hostTag.String(),
		).Add(coreerrors.NotValid)
	}

	return rval, nil
}

// getUnitUUID translates a unit tag into a unit UUID, mapping domain
// sentinels to CodeNotFound and CodeNotValid. Mirrors facade helper
// getUnitUUID at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:2629
func (a *modelStorageAdapter) getUnitUUID(
	ctx context.Context, tag names.UnitTag,
) (coreunit.UUID, error) {
	unitName, err := coreunit.NewName(tag.Id())
	if errors.Is(err, coreunit.InvalidUnitName) {
		return "", errors.Errorf(
			"invalid unit name %q", tag.Id(),
		).Add(coreerrors.NotValid)
	} else if err != nil {
		return "", errors.Capture(err)
	}

	unitUUID, err := a.appSvc.GetUnitUUID(ctx, unitName)
	if errors.Is(err, coreunit.InvalidUnitName) {
		return "", errors.Errorf(
			"invalid unit name %q", unitName,
		).Add(coreerrors.NotValid)
	} else if errors.Is(err, applicationerrors.UnitNotFound) {
		return "", errors.Errorf(
			"unit %q not found", unitName,
		).Add(coreerrors.NotFound)
	} else if err != nil {
		return "", errors.Errorf("getting unit %q UUID: %w", unitName, err)
	}
	return unitUUID, nil
}

// VolumeAccessor implementation

func (a *modelStorageAdapter) WatchBlockDevices(ctx context.Context, m names.MachineTag) (watcher.NotifyWatcher, error) {
	machineUUID, err := a.getMachineUUID(ctx, m)
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}
	return a.blockDeviceSvc.WatchBlockDevicesForMachine(ctx, machineUUID)
}

func (a *modelStorageAdapter) WatchVolumes(ctx context.Context, scope names.Tag) (watcher.StringsWatcher, error) {
	switch scope := scope.(type) {
	case names.ModelTag:
		return a.storageSvc.WatchModelProvisionedVolumes(ctx)
	case names.MachineTag:
		machineUUID, err := a.getMachineUUID(ctx, scope)
		if err != nil {
			return nil, jujuerrors.Trace(err)
		}
		return a.storageSvc.WatchMachineProvisionedVolumes(ctx, machineUUID)
	default:
		return nil, errors.Errorf("unsupported scope %T", scope).Add(coreerrors.NotSupported)
	}
}

func (a *modelStorageAdapter) WatchVolumeAttachments(ctx context.Context, scope names.Tag) (watcher.MachineStorageIDsWatcher, error) {
	var sourceWatcher watcher.StringsWatcher
	switch scope := scope.(type) {
	case names.ModelTag:
		w, err := a.storageSvc.WatchModelProvisionedVolumeAttachments(ctx)
		if err != nil {
			return nil, jujuerrors.Trace(err)
		}
		sourceWatcher = w
	case names.MachineTag:
		machineUUID, err := a.getMachineUUID(ctx, scope)
		if err != nil {
			return nil, jujuerrors.Trace(err)
		}
		w, err := a.storageSvc.WatchMachineProvisionedVolumeAttachments(ctx, machineUUID)
		if err != nil {
			return nil, jujuerrors.Trace(err)
		}
		sourceWatcher = w
	default:
		return nil, errors.Errorf("unsupported scope %T", scope).Add(coreerrors.NotSupported)
	}
	return newAttachmentIDWatcher(sourceWatcher, func(ctx context.Context, ids ...string) ([]watcher.MachineStorageID, error) {
		attachmentIDs, err := a.storageSvc.GetVolumeAttachmentIDs(ctx, ids)
		if err != nil {
			return nil, jujuerrors.Trace(err)
		}
		var out []watcher.MachineStorageID
		for _, id := range attachmentIDs {
			if id.MachineName == nil && id.UnitName == nil {
				continue
			}
			msid := watcher.MachineStorageID{
				AttachmentTag: names.NewVolumeTag(id.VolumeID).String(),
			}
			if id.MachineName != nil {
				msid.MachineTag = names.NewMachineTag(id.MachineName.String()).String()
			} else if id.UnitName != nil {
				msid.MachineTag = names.NewUnitTag(id.UnitName.String()).String()
			}
			out = append(out, msid)
		}
		return out, nil
	})
}

func (a *modelStorageAdapter) WatchVolumeAttachmentPlans(ctx context.Context, scope names.Tag) (watcher.MachineStorageIDsWatcher, error) {
	switch scope := scope.(type) {
	case names.MachineTag:
		machineUUID, err := a.getMachineUUID(ctx, scope)
		if err != nil {
			return nil, jujuerrors.Trace(err)
		}
		w, err := a.storageSvc.WatchVolumeAttachmentPlans(ctx, machineUUID)
		if err != nil {
			return nil, jujuerrors.Trace(err)
		}
		return newAttachmentIDWatcher(w, func(ctx context.Context, volumeIDs ...string) ([]watcher.MachineStorageID, error) {
			if len(volumeIDs) == 0 {
				return nil, nil
			}
			out := make([]watcher.MachineStorageID, 0, len(volumeIDs))
			for _, volumeID := range volumeIDs {
				if !names.IsValidVolume(volumeID) {
					continue
				}
				out = append(out, watcher.MachineStorageID{
					MachineTag:    scope.String(),
					AttachmentTag: names.NewVolumeTag(volumeID).String(),
				})
			}
			return out, nil
		})
	default:
		return nil, errors.Errorf("unsupported scope %T", scope).Add(coreerrors.NotSupported)
	}
}

// Volumes returns details of volumes with the specified tags.
// Mirrors facade Volumes at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:884
func (a *modelStorageAdapter) Volumes(ctx context.Context, tags []names.VolumeTag) ([]params.VolumeResult, error) {
	results := make([]params.VolumeResult, len(tags))
	for i, tag := range tags {
		vol, err := a.storageSvc.GetVolumeByID(ctx, tag.Id())
		if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"volume %q not found", tag.Id(),
			).Add(coreerrors.NotFound))
			continue
		}
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"getting volume %q: %v", tag.Id(), err,
			))
			continue
		}
		if vol.SizeMiB == 0 {
			results[i].Error = serverError(errors.Errorf(
				"volume %q is not provisioned", tag.Id(),
			).Add(coreerrors.NotProvisioned))
			continue
		}
		results[i].Result = params.Volume{
			VolumeTag: tag.String(),
			Info: params.VolumeInfo{
				ProviderId: vol.ProviderID,
				HardwareId: vol.HardwareID,
				WWN:        vol.WWN,
				Persistent: vol.Persistent,
				SizeMiB:    vol.SizeMiB,
			},
		}
	}
	return results, nil
}

// VolumeBlockDevices returns details of block devices corresponding to the
// specified volume attachment IDs.
// Mirrors facade VolumeBlockDevices at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:1265
func (a *modelStorageAdapter) VolumeBlockDevices(ctx context.Context, ids []params.MachineStorageId) ([]params.BlockDeviceResult, error) {
	results := make([]params.BlockDeviceResult, len(ids))
	for i, id := range ids {
		volumeTag, err := names.ParseVolumeTag(id.AttachmentTag)
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"volume tag %q invalid", id.AttachmentTag,
			).Add(coreerrors.NotValid))
			continue
		}
		machineTag, err := names.ParseMachineTag(id.MachineTag)
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"machine tag %q invalid", id.MachineTag,
			).Add(coreerrors.NotValid))
			continue
		}
		machineUUID, err := a.getMachineUUID(ctx, machineTag)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		// GetVolumeAttachmentUUIDForVolumeIDMachine error mapping mirrors
		// facade lines ~1296-1314.
		va, err := a.storageSvc.GetVolumeAttachmentUUIDForVolumeIDMachine(ctx, volumeTag.Id(), machineUUID)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"volume attachment %q on machine %q not found",
				volumeTag.Id(), machineTag.Id(),
			).Add(coreerrors.NotFound))
			continue
		} else if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"volume %q not found", volumeTag.Id(),
			).Add(coreerrors.NotFound))
			continue
		} else if errors.Is(err, machineerrors.MachineNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"machine %q not found", machineTag.Id(),
			).Add(coreerrors.NotFound))
			continue
		} else if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"getting volume attachment %q on machine %q: %v",
				volumeTag.Id(), machineTag.Id(), err,
			))
			continue
		}
		// GetBlockDeviceForVolumeAttachment error mapping mirrors facade
		// lines ~1318-1333.
		bdUUID, err := a.storageSvc.GetBlockDeviceForVolumeAttachment(ctx, va)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentWithoutBlockDevice) {
			results[i].Error = serverError(errors.Errorf(
				"volume attachment %q on machine %q is not provisioned",
				volumeTag.Id(), machineTag.Id(),
			).Add(coreerrors.NotProvisioned))
			continue
		} else if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"volume attachment %q on machine %q not found",
				volumeTag.Id(), machineTag.Id(),
			).Add(coreerrors.NotFound))
			continue
		} else if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"getting volume attachment %q on machine %q: %v",
				volumeTag.Id(), machineTag.Id(), err,
			))
			continue
		}
		// GetBlockDevice error mapping mirrors facade lines ~1336-1345.
		bd, err := a.blockDeviceSvc.GetBlockDevice(ctx, bdUUID)
		if errors.Is(err, blockdeviceerrors.BlockDeviceNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"volume attachment %q on machine %q is not provisioned",
				volumeTag.Id(), machineTag.Id(),
			).Add(coreerrors.NotProvisioned))
			continue
		} else if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"getting block device %q: %v", bdUUID, err,
			))
			continue
		}
		if bd.DeviceName == "" || len(bd.DeviceLinks) == 0 {
			results[i].Error = serverError(errors.Errorf(
				"volume attachment %q on machine %q is not provisioned",
				volumeTag.Id(), machineTag.Id(),
			).Add(coreerrors.NotProvisioned))
			continue
		}
		var provenance params.BlockDeviceProvenance
		switch bd.Provenance {
		case coreblockdevice.ProviderProvenance:
			provenance = params.BlockDeviceProvenanceProvider
		case coreblockdevice.MachineProvenance:
			provenance = params.BlockDeviceProvenanceMachine
		default:
			results[i].Error = serverError(errors.Errorf(
				"unexpected provenance value: %v", bd.Provenance,
			).Add(coreerrors.NotImplemented))
			continue
		}
		results[i].Result = params.BlockDevice{
			DeviceName:     bd.DeviceName,
			DeviceLinks:    bd.DeviceLinks,
			Label:          bd.FilesystemLabel,
			UUID:           bd.FilesystemUUID,
			HardwareId:     bd.HardwareId,
			WWN:            bd.WWN,
			BusAddress:     bd.BusAddress,
			SizeMiB:        bd.SizeMiB,
			FilesystemType: bd.FilesystemType,
			InUse:          bd.InUse,
			MountPoint:     bd.MountPoint,
			SerialId:       bd.SerialId,
			Provenance:     provenance,
		}
	}
	return results, nil
}

// VolumeAttachments returns details of volume attachments with the specified
// IDs. Mirrors facade VolumeAttachments/volumeAttachments at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:1166
func (a *modelStorageAdapter) VolumeAttachments(ctx context.Context, ids []params.MachineStorageId) ([]params.VolumeAttachmentResult, error) {
	results := make([]params.VolumeAttachmentResult, len(ids))
	for i, id := range ids {
		volumeTag, err := names.ParseVolumeTag(id.AttachmentTag)
		if err != nil {
			results[i].Error = serverError(errors.New(
				"volume tag invalid",
			).Add(coreerrors.NotValid))
			continue
		}
		machineTag, err := names.ParseMachineTag(id.MachineTag)
		if err != nil {
			results[i].Error = serverError(errors.New(
				"machine tag invalid",
			).Add(coreerrors.NotValid))
			continue
		}
		machineUUID, err := a.getMachineUUID(ctx, machineTag)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		uuid, err := a.getVolumeAttachmentUUID(ctx, volumeTag, machineUUID)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		va, err := a.storageSvc.GetVolumeAttachment(ctx, uuid)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"volume attachment %q on machine %q not found",
				volumeTag.Id(), machineTag.Id(),
			).Add(coreerrors.NotFound))
			continue
		} else if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"getting volume attachment %q on machine %q: %v",
				volumeTag.Id(), machineTag.Id(), err,
			))
			continue
		}
		if va.BlockDeviceName == "" || len(va.BlockDeviceLinks) == 0 {
			results[i].Error = serverError(errors.Errorf(
				"volume %q is not provisioned", volumeTag.Id(),
			).Add(coreerrors.NotProvisioned))
			continue
		}
		results[i].Result = params.VolumeAttachment{
			VolumeTag:  volumeTag.String(),
			MachineTag: machineTag.String(),
			Info: params.VolumeAttachmentInfo{
				DeviceName: va.BlockDeviceName,
				DeviceLink: domainblockdevice.IDLink(va.BlockDeviceLinks),
				BusAddress: va.BlockDeviceBusAddress,
				ReadOnly:   va.ReadOnly,
			},
		}
	}
	return results, nil
}

// VolumeAttachmentPlans returns details of volume attachment plans with the
// specified IDs. Mirrors facade VolumeAttachmentPlans/volumeAttachmentPlan at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:1059
func (a *modelStorageAdapter) VolumeAttachmentPlans(ctx context.Context, ids []params.MachineStorageId) ([]params.VolumeAttachmentPlanResult, error) {
	results := make([]params.VolumeAttachmentPlanResult, len(ids))
	for i, id := range ids {
		volumeTag, err := names.ParseVolumeTag(id.AttachmentTag)
		if err != nil {
			results[i].Error = serverError(errors.New(
				"volume tag invalid",
			).Add(coreerrors.NotValid))
			continue
		}
		machineTag, err := names.ParseMachineTag(id.MachineTag)
		if err != nil {
			results[i].Error = serverError(errors.New(
				"machine tag invalid",
			).Add(coreerrors.NotValid))
			continue
		}
		machineUUID, err := a.getMachineUUID(ctx, machineTag)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		planUUID, err := a.getVolumeAttachmentPlanUUID(ctx, volumeTag, machineUUID)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		plan, err := a.storageSvc.GetVolumeAttachmentPlan(ctx, planUUID)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentPlanNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"volume attachment plan %q on machine %q not found",
				volumeTag.Id(), machineTag.Id(),
			).Add(coreerrors.NotFound))
			continue
		} else if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"getting volume attachment plan %q on machine %q: %v",
				volumeTag.Id(), machineTag.Id(), err,
			))
			continue
		}
		planLife, err := plan.Life.Value()
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"getting volume attachment plan %q life: %w",
				planUUID, err,
			))
			continue
		}
		// Device type conversion and error mirrors facade lines ~1142-1152.
		var deviceType storage.DeviceType
		switch plan.DeviceType {
		case domainstorage.VolumeDeviceTypeISCSI:
			deviceType = storage.DeviceTypeISCSI
		case domainstorage.VolumeDeviceTypeLocal:
			deviceType = storage.DeviceTypeLocal
		default:
			results[i].Error = serverError(errors.Errorf(
				"unknown device type %q", plan.DeviceType,
			))
			continue
		}
		results[i].Result = params.VolumeAttachmentPlan{
			VolumeTag:  volumeTag.String(),
			MachineTag: machineTag.String(),
			Life:       planLife,
			PlanInfo: params.VolumeAttachmentPlanInfo{
				DeviceType:       deviceType.String(),
				DeviceAttributes: plan.DeviceAttributes,
			},
		}
	}
	return results, nil
}

// VolumeParams returns the parameters for creating the volumes with the
// specified tags. Mirrors facade VolumeParams at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:1527
func (a *modelStorageAdapter) VolumeParams(ctx context.Context, tags []names.VolumeTag) ([]params.VolumeParamsResult, error) {
	results := make([]params.VolumeParamsResult, len(tags))
	volModelTags, err := a.storageSvc.GetStorageResourceTagsForModel(ctx)
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}
	for i, tag := range tags {
		uuid, err := a.storageSvc.GetVolumeUUIDForID(ctx, tag.Id())
		if err != nil {
			// Facade converts VolumeNotFound to ErrPerm (~line 1611)
			// to avoid leaking entity existence.
			if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
				results[i].Error = errPermResult
			} else {
				results[i].Error = serverError(err)
			}
			continue
		}
		volParams, err := a.storageSvc.GetVolumeParams(ctx, uuid)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		rval := params.VolumeParams{
			Attributes: make(map[string]any, len(volParams.Attributes)),
			VolumeTag:  tag.String(),
			Provider:   volParams.Provider,
			SizeMiB:    volParams.SizeMiB,
			Tags:       volModelTags,
		}
		for k, v := range volParams.Attributes {
			rval.Attributes[k] = v
		}
		if volParams.VolumeAttachmentUUID != nil {
			vaParams, err := a.storageSvc.GetVolumeAttachmentParams(ctx, *volParams.VolumeAttachmentUUID)
			if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
				// No attachment params yet — return rval without attachment,
				// matching facade line ~1586.
			} else if err != nil {
				results[i].Error = serverError(err)
				continue
			} else {
				rval.Attachment = &params.VolumeAttachmentParams{
					VolumeTag:  tag.String(),
					InstanceId: vaParams.MachineInstanceID,
					Provider:   vaParams.Provider,
					ProviderId: vaParams.ProviderID,
					ReadOnly:   vaParams.ReadOnly,
				}
				if vaParams.Machine != nil {
					rval.Attachment.MachineTag = names.NewMachineTag(vaParams.Machine.String()).String()
				}
			}
		}
		results[i].Result = rval
	}
	return results, nil
}

// RemoveVolumeParams returns the parameters for destroying or releasing the
// volumes with the specified tags. Mirrors facade RemoveVolumeParams at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:1627
func (a *modelStorageAdapter) RemoveVolumeParams(ctx context.Context, tags []names.VolumeTag) ([]params.RemoveVolumeParamsResult, error) {
	results := make([]params.RemoveVolumeParamsResult, len(tags))
	for i, tag := range tags {
		uuid, err := a.storageSvc.GetVolumeUUIDForID(ctx, tag.Id())
		if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"volume %q not found", tag.Id(),
			).Add(coreerrors.NotFound))
			continue
		}
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		rp, err := a.storageSvc.GetVolumeRemovalParams(ctx, uuid)
		if errors.Is(err, storageprovisioningerrors.VolumeNotDead) {
			results[i].Error = serverError(errors.Errorf(
				"volume %q is not yet dead", tag.Id(),
			))
			continue
		}
		if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"volume %q not found", tag.Id(),
			).Add(coreerrors.NotFound))
			continue
		}
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		results[i].Result = params.RemoveVolumeParams{
			Provider:   rp.Provider,
			ProviderId: rp.ProviderID,
			Destroy:    rp.Obliterate,
		}
	}
	return results, nil
}

// VolumeAttachmentParams returns the parameters for creating the volume
// attachments with the specified IDs. Mirrors facade VolumeAttachmentParams at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:1899
func (a *modelStorageAdapter) VolumeAttachmentParams(ctx context.Context, ids []params.MachineStorageId) ([]params.VolumeAttachmentParamsResult, error) {
	results := make([]params.VolumeAttachmentParamsResult, len(ids))
	for i, id := range ids {
		machineTag, err := names.ParseMachineTag(id.MachineTag)
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"parsing machine tag: %w", err,
			))
			continue
		}
		volumeTag, err := names.ParseVolumeTag(id.AttachmentTag)
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"parsing volume tag: %w", err,
			))
			continue
		}
		machineUUID, err := a.getMachineUUID(ctx, machineTag)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		attachmentUUID, err := a.getVolumeAttachmentUUID(ctx, volumeTag, machineUUID)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		volParams, err := a.storageSvc.GetVolumeAttachmentParams(ctx, attachmentUUID)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"volume attachment for volume %q and host %q not found",
				volumeTag, machineTag,
			).Add(coreerrors.NotFound))
			continue
		} else if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		results[i].Result = params.VolumeAttachmentParams{
			VolumeTag:  volumeTag.String(),
			MachineTag: machineTag.String(),
			InstanceId: volParams.MachineInstanceID,
			Provider:   volParams.Provider,
			ProviderId: volParams.ProviderID,
			ReadOnly:   volParams.ReadOnly,
		}
	}
	return results, nil
}

// SetVolumeInfo records the details of newly provisioned volumes.
// Mirrors facade SetVolumeInfo at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:2137
func (a *modelStorageAdapter) SetVolumeInfo(ctx context.Context, vols []params.Volume) ([]params.ErrorResult, error) {
	results := make([]params.ErrorResult, len(vols))
	for i, vol := range vols {
		volumeTag, err := names.ParseVolumeTag(vol.VolumeTag)
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"parsing volume tag %q: %w", vol.VolumeTag, err,
			))
			continue
		}
		if vol.Info.Pool != "" {
			results[i].Error = serverError(errors.New("pool field must not be set"))
			continue
		}
		info := storageprovisioning.VolumeProvisionedInfo{
			ProviderID: vol.Info.ProviderId,
			SizeMiB:    vol.Info.SizeMiB,
			HardwareID: vol.Info.HardwareId,
			WWN:        vol.Info.WWN,
			Persistent: vol.Info.Persistent,
		}
		err = a.storageSvc.SetVolumeProvisionedInfo(ctx, volumeTag.Id(), info)
		if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"volume %q not found", volumeTag.Id(),
			).Add(coreerrors.NotFound))
			continue
		}
		if err != nil {
			results[i].Error = serverError(err)
		}
	}
	return results, nil
}

// SetVolumeAttachmentInfo records the details of newly provisioned volume
// attachments. Mirrors facade SetVolumeAttachmentInfo/setVolumeAttachmentInfo at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:2414
func (a *modelStorageAdapter) SetVolumeAttachmentInfo(ctx context.Context, vas []params.VolumeAttachment) ([]params.ErrorResult, error) {
	results := make([]params.ErrorResult, len(vas))
	for i, va := range vas {
		machineTag, err := names.ParseMachineTag(va.MachineTag)
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"parsing machine tag: %w", err,
			))
			continue
		}
		volumeTag, err := names.ParseVolumeTag(va.VolumeTag)
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"parsing volume tag: %w", err,
			))
			continue
		}
		machineUUID, err := a.getMachineUUID(ctx, machineTag)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		// GetVolumeAttachmentUUIDForVolumeIDMachine error mapping mirrors
		// facade setVolumeAttachmentInfo ~lines 2462-2476.
		attachmentUUID, err := a.storageSvc.GetVolumeAttachmentUUIDForVolumeIDMachine(ctx, volumeTag.Id(), machineUUID)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"volume attachment %q on %q not found",
				volumeTag.Id(), machineUUID,
			).Add(coreerrors.NotFound))
			continue
		}
		if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"volume %q not found for attachment on %q",
				volumeTag.Id(), machineUUID,
			).Add(coreerrors.NotFound))
			continue
		}
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		info := storageprovisioning.VolumeAttachmentProvisionedInfo{
			ReadOnly: va.Info.ReadOnly,
		}
		if va.Info.DeviceName != "" || va.Info.DeviceLink != "" || va.Info.BusAddress != "" {
			device := coreblockdevice.BlockDevice{
				DeviceName: va.Info.DeviceName,
				BusAddress: va.Info.BusAddress,
			}
			if va.Info.DeviceLink != "" {
				device.DeviceLinks = []string{va.Info.DeviceLink}
			}
			blockDevUUID, err := a.blockDeviceSvc.MatchOrCreateBlockDevice(ctx, machineUUID, device)
			if errors.Is(err, machineerrors.MachineNotFound) {
				results[i].Error = serverError(errors.Errorf(
					"machine %q not found", machineTag.Id(),
				).Add(coreerrors.NotFound))
				continue
			} else if err != nil {
				results[i].Error = serverError(err)
				continue
			}
			info.BlockDeviceUUID = &blockDevUUID
		}
		err = a.storageSvc.SetVolumeAttachmentProvisionedInfo(ctx, attachmentUUID, info)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"volume attachment for machine %q and volume %q not found",
				machineTag.Id(), volumeTag.Id(),
			).Add(coreerrors.NotFound))
			continue
		}
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		if va.Info.PlanInfo == nil {
			continue
		}
		planUUID, err := a.getVolumeAttachmentPlanUUID(ctx, volumeTag, machineUUID)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		planInfo := storageprovisioning.VolumeAttachmentPlanProvisionedInfo{
			DeviceAttributes: va.Info.PlanInfo.DeviceAttributes,
		}
		switch va.Info.PlanInfo.DeviceType {
		case storage.DeviceTypeLocal.String():
			planInfo.DeviceType = domainstorage.VolumeDeviceTypeLocal
		case storage.DeviceTypeISCSI.String():
			planInfo.DeviceType = domainstorage.VolumeDeviceTypeISCSI
		}
		err = a.storageSvc.SetVolumeAttachmentPlanProvisionedInfo(ctx, planUUID, planInfo)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentPlanNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"volume attachment plan for machine %q and volume %q not found",
				machineTag.Id(), volumeTag.Id(),
			).Add(coreerrors.NotFound))
		} else if err != nil {
			// Wrap with context, matching facade line ~2543.
			results[i].Error = serverError(errors.Errorf(
				"setting volume attachment plan info: %w", err,
			))
		}
	}
	return results, nil
}

// CreateVolumeAttachmentPlans creates volume attachment plans.
// Mirrors facade CreateVolumeAttachmentPlans/createVolumeAttachmentPlan at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:2227
func (a *modelStorageAdapter) CreateVolumeAttachmentPlans(ctx context.Context, plans []params.VolumeAttachmentPlan) ([]params.ErrorResult, error) {
	results := make([]params.ErrorResult, len(plans))
	for i, plan := range plans {
		machineTag, err := names.ParseMachineTag(plan.MachineTag)
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"parsing machine tag: %w", err,
			))
			continue
		}
		volumeTag, err := names.ParseVolumeTag(plan.VolumeTag)
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"parsing volume tag: %w", err,
			))
			continue
		}
		// Reject block device field, matching facade line ~2250.
		if plan.BlockDevice != nil {
			results[i].Error = serverError(errors.New(
				"block device field must not be set",
			))
			continue
		}
		var deviceType domainstorage.VolumeDeviceType
		switch plan.PlanInfo.DeviceType {
		case storage.DeviceTypeISCSI.String():
			deviceType = domainstorage.VolumeDeviceTypeISCSI
		case storage.DeviceTypeLocal.String():
			deviceType = domainstorage.VolumeDeviceTypeLocal
		default:
			results[i].Error = serverError(errors.Errorf(
				"plan device type %q not valid", plan.PlanInfo.DeviceType,
			).Add(coreerrors.NotValid))
			continue
		}
		machineUUID, err := a.getMachineUUID(ctx, machineTag)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		attachmentUUID, err := a.getVolumeAttachmentUUID(ctx, volumeTag, machineUUID)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		_, err = a.storageSvc.CreateVolumeAttachmentPlan(ctx, attachmentUUID, deviceType, plan.PlanInfo.DeviceAttributes)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentPlanAlreadyExists) {
			continue
		} else if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"volume attachment for machine %q and volume %q not found",
				machineTag.Id(), volumeTag.Id(),
			).Add(coreerrors.NotFound))
		} else if err != nil {
			results[i].Error = serverError(err)
		}
	}
	return results, nil
}

// RemoveVolumeAttachmentPlan removes volume attachment plans.
// Mirrors facade RemoveVolumeAttachmentPlan/removeVolumeAttachmentPlan at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:745
func (a *modelStorageAdapter) RemoveVolumeAttachmentPlan(ctx context.Context, ids []params.MachineStorageId) ([]params.ErrorResult, error) {
	results := make([]params.ErrorResult, len(ids))
	for i, id := range ids {
		volumeTag, err := names.ParseVolumeTag(id.AttachmentTag)
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"volume tag %q invalid", id.AttachmentTag,
			).Add(coreerrors.NotValid))
			continue
		}
		machineTag, err := names.ParseMachineTag(id.MachineTag)
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"machine tag %q invalid", id.MachineTag,
			).Add(coreerrors.NotValid))
			continue
		}
		machineUUID, err := a.getMachineUUID(ctx, machineTag)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		// Plan UUID lookup: NotFound is treated as success (already removed),
		// matching facade line ~793.
		planUUID, err := a.getVolumeAttachmentPlanUUID(ctx, volumeTag, machineUUID)
		if errors.Is(err, coreerrors.NotFound) {
			continue
		} else if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		// MarkVolumeAttachmentPlanAsDead error mapping mirrors facade
		// lines ~800-809.
		err = a.removalSvc.MarkVolumeAttachmentPlanAsDead(ctx, planUUID)
		if errors.Is(err, removalerrors.EntityStillAlive) {
			results[i].Error = serverError(errors.Errorf(
				"volume %q attachment plan for machine %q is still alive",
				volumeTag.Id(), machineTag.Id(),
			))
		} else if errors.Is(err, storageprovisioningerrors.VolumeAttachmentPlanNotFound) {
			// Already removed — treat as success, matching facade line ~806.
			continue
		} else if err != nil {
			results[i].Error = serverError(err)
		}
	}
	return results, nil
}

// SetVolumeAttachmentPlanBlockInfo records block device info for volume
// attachment plans. Mirrors facade
// SetVolumeAttachmentPlanBlockInfo/setVolumeAttachmentPlanBlockInfo at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:2312
func (a *modelStorageAdapter) SetVolumeAttachmentPlanBlockInfo(ctx context.Context, plans []params.VolumeAttachmentPlan) ([]params.ErrorResult, error) {
	results := make([]params.ErrorResult, len(plans))
	for i, plan := range plans {
		machineTag, err := names.ParseMachineTag(plan.MachineTag)
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"parsing machine tag: %w", err,
			))
			continue
		}
		volumeTag, err := names.ParseVolumeTag(plan.VolumeTag)
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"parsing volume tag: %w", err,
			))
			continue
		}
		machineUUID, err := a.getMachineUUID(ctx, machineTag)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		planUUID, err := a.getVolumeAttachmentPlanUUID(ctx, volumeTag, machineUUID)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		if plan.BlockDevice == nil {
			continue
		}
		device := coreblockdevice.BlockDevice{
			DeviceName:      plan.BlockDevice.DeviceName,
			DeviceLinks:     plan.BlockDevice.DeviceLinks,
			FilesystemLabel: plan.BlockDevice.Label,
			FilesystemUUID:  plan.BlockDevice.UUID,
			HardwareId:      plan.BlockDevice.HardwareId,
			WWN:             plan.BlockDevice.WWN,
			BusAddress:      plan.BlockDevice.BusAddress,
			SizeMiB:         plan.BlockDevice.SizeMiB,
			FilesystemType:  plan.BlockDevice.FilesystemType,
			InUse:           plan.BlockDevice.InUse,
			MountPoint:      plan.BlockDevice.MountPoint,
			SerialId:        plan.BlockDevice.SerialId,
		}
		if domainblockdevice.IsEmpty(device) {
			continue
		}
		// MatchOrCreateBlockDevice MachineNotFound mapping mirrors facade
		// line ~2390.
		blockDevUUID, err := a.blockDeviceSvc.MatchOrCreateBlockDevice(ctx, machineUUID, device)
		if errors.Is(err, machineerrors.MachineNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"machine %q not found", machineTag.Id(),
			).Add(coreerrors.NotFound))
			continue
		} else if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		// SetVolumeAttachmentPlanProvisionedBlockDevice PlanNotFound mapping
		// mirrors facade line ~2400.
		err = a.storageSvc.SetVolumeAttachmentPlanProvisionedBlockDevice(ctx, planUUID, blockDevUUID)
		if errors.Is(err, storageprovisioningerrors.VolumeAttachmentPlanNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"volume attachment plan for machine %q and volume %q not found",
				machineTag.Id(), volumeTag.Id(),
			).Add(coreerrors.NotFound))
		} else if err != nil {
			results[i].Error = serverError(err)
		}
	}
	return results, nil
}

// FilesystemAccessor implementation

func (a *modelStorageAdapter) WatchFilesystems(ctx context.Context, scope names.Tag) (watcher.StringsWatcher, error) {
	switch scope := scope.(type) {
	case names.ModelTag:
		return a.storageSvc.WatchModelProvisionedFilesystems(ctx)
	case names.MachineTag:
		machineUUID, err := a.getMachineUUID(ctx, scope)
		if err != nil {
			return nil, jujuerrors.Trace(err)
		}
		return a.storageSvc.WatchMachineProvisionedFilesystems(ctx, machineUUID)
	default:
		return nil, errors.Errorf("unsupported scope %T", scope).Add(coreerrors.NotSupported)
	}
}

func (a *modelStorageAdapter) WatchFilesystemAttachments(ctx context.Context, scope names.Tag) (watcher.MachineStorageIDsWatcher, error) {
	var sourceWatcher watcher.StringsWatcher
	switch scope := scope.(type) {
	case names.ModelTag:
		w, err := a.storageSvc.WatchModelProvisionedFilesystemAttachments(ctx)
		if err != nil {
			return nil, jujuerrors.Trace(err)
		}
		sourceWatcher = w
	case names.MachineTag:
		machineUUID, err := a.getMachineUUID(ctx, scope)
		if err != nil {
			return nil, jujuerrors.Trace(err)
		}
		w, err := a.storageSvc.WatchMachineProvisionedFilesystemAttachments(ctx, machineUUID)
		if err != nil {
			return nil, jujuerrors.Trace(err)
		}
		sourceWatcher = w
	default:
		return nil, errors.Errorf("unsupported scope %T", scope).Add(coreerrors.NotSupported)
	}
	return newAttachmentIDWatcher(sourceWatcher, func(ctx context.Context, ids ...string) ([]watcher.MachineStorageID, error) {
		attachmentIDs, err := a.storageSvc.GetFilesystemAttachmentIDs(ctx, ids)
		if err != nil {
			return nil, jujuerrors.Trace(err)
		}
		var out []watcher.MachineStorageID
		for _, id := range attachmentIDs {
			if id.MachineName == nil && id.UnitName == nil {
				continue
			}
			msid := watcher.MachineStorageID{
				AttachmentTag: names.NewFilesystemTag(id.FilesystemID).String(),
			}
			if id.MachineName != nil {
				msid.MachineTag = names.NewMachineTag(id.MachineName.String()).String()
			} else if id.UnitName != nil {
				msid.MachineTag = names.NewUnitTag(id.UnitName.String()).String()
			}
			out = append(out, msid)
		}
		return out, nil
	})
}

// Filesystems returns details of filesystems with the specified tags.
// Mirrors facade Filesystems at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:951
func (a *modelStorageAdapter) Filesystems(ctx context.Context, tags []names.FilesystemTag) ([]params.FilesystemResult, error) {
	results := make([]params.FilesystemResult, len(tags))
	for i, tag := range tags {
		fs, err := a.storageSvc.GetFilesystemForID(ctx, tag.Id())
		if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"filesystem %q not found", tag.Id(),
			).Add(coreerrors.NotFound))
			continue
		}
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"getting filesystem %q: %v", tag.Id(), err,
			))
			continue
		}
		if fs.SizeMiB == 0 {
			results[i].Error = serverError(errors.Errorf(
				"filesystem %q is not provisioned", tag.Id(),
			).Add(coreerrors.NotProvisioned))
			continue
		}
		result := params.Filesystem{
			FilesystemTag: tag.String(),
			Info: params.FilesystemInfo{
				ProviderId: fs.ProviderID,
				SizeMiB:    fs.SizeMiB,
			},
		}
		if fs.BackingVolume != nil {
			// Validate backing volume ID, matching facade lines ~994-998.
			if !names.IsValidVolume(fs.BackingVolume.VolumeID) {
				results[i].Error = serverError(errors.Errorf(
					"invalid volume ID %q for filesystem %q",
					fs.BackingVolume.VolumeID, tag.Id(),
				).Add(coreerrors.NotValid))
				continue
			}
			result.VolumeTag = names.NewVolumeTag(fs.BackingVolume.VolumeID).String()
		}
		results[i].Result = result
	}
	return results, nil
}

// FilesystemAttachments returns details of filesystem attachments with the
// specified IDs. Mirrors facade FilesystemAttachments at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:1421
func (a *modelStorageAdapter) FilesystemAttachments(ctx context.Context, ids []params.MachineStorageId) ([]params.FilesystemAttachmentResult, error) {
	results := make([]params.FilesystemAttachmentResult, len(ids))
	for i, id := range ids {
		filesystemTag, err := names.ParseFilesystemTag(id.AttachmentTag)
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"parsing filesystem tag %q: %w", id.AttachmentTag, err,
			))
			continue
		}
		hostTag, err := names.ParseTag(id.MachineTag)
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"parsing host tag %q: %w", id.MachineTag, err,
			))
			continue
		}
		var fsAttachment storageprovisioning.FilesystemAttachment
		switch tag := hostTag.(type) {
		case names.MachineTag:
			machineUUID, err := a.getMachineUUID(ctx, tag)
			if err != nil {
				results[i].Error = serverError(err)
				continue
			}
			fsAttachment, err = a.storageSvc.GetFilesystemAttachmentForMachine(ctx, filesystemTag.Id(), machineUUID)
			// Error mapping mirrors facade lines ~1473-1491.
			if errors.Is(err, machineerrors.MachineNotFound) {
				results[i].Error = serverError(errors.Errorf(
					"machine %q not found", hostTag.Id(),
				).Add(coreerrors.NotFound))
				continue
			} else if errors.Is(err, storageprovisioningerrors.FilesystemAttachmentNotFound) {
				results[i].Error = serverError(errors.Errorf(
					"filesystem attachment %q on %q not found",
					filesystemTag.Id(), hostTag.Id(),
				).Add(coreerrors.NotFound))
				continue
			} else if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
				results[i].Error = serverError(errors.Errorf(
					"filesystem %q not found for attachment on %q",
					filesystemTag.Id(), hostTag.Id(),
				).Add(coreerrors.NotFound))
				continue
			} else if err != nil {
				results[i].Error = serverError(errors.Errorf(
					"getting filesystem attachment for %q on %q: %w",
					filesystemTag.Id(), hostTag.Id(), err,
				))
				continue
			}
		case names.UnitTag:
			unitUUID, err := a.getUnitUUID(ctx, tag)
			if err != nil {
				results[i].Error = serverError(err)
				continue
			}
			fsAttachment, err = a.storageSvc.GetFilesystemAttachmentForUnit(ctx, filesystemTag.Id(), unitUUID)
			// Error mapping mirrors facade lines ~1473-1491.
			if errors.Is(err, applicationerrors.UnitNotFound) {
				results[i].Error = serverError(errors.Errorf(
					"unit %q not found", hostTag.Id(),
				).Add(coreerrors.NotFound))
				continue
			} else if errors.Is(err, storageprovisioningerrors.FilesystemAttachmentNotFound) {
				results[i].Error = serverError(errors.Errorf(
					"filesystem attachment %q on %q not found",
					filesystemTag.Id(), hostTag.Id(),
				).Add(coreerrors.NotFound))
				continue
			} else if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
				results[i].Error = serverError(errors.Errorf(
					"filesystem %q not found for attachment on %q",
					filesystemTag.Id(), hostTag.Id(),
				).Add(coreerrors.NotFound))
				continue
			} else if err != nil {
				results[i].Error = serverError(errors.Errorf(
					"getting filesystem attachment for %q on %q: %w",
					filesystemTag.Id(), hostTag.Id(), err,
				))
				continue
			}
		default:
			results[i].Error = serverError(errors.Errorf(
				"filesystem attachment host tag %q", hostTag,
			).Add(coreerrors.NotValid))
			continue
		}
		if fsAttachment.MountPoint == "" {
			results[i].Error = serverError(errors.Errorf(
				"filesystem attachment %q on %q is not provisioned",
				filesystemTag.Id(), hostTag.String(),
			).Add(coreerrors.NotProvisioned))
			continue
		}
		results[i].Result = params.FilesystemAttachment{
			FilesystemTag: filesystemTag.String(),
			MachineTag:    hostTag.String(),
			Info: params.FilesystemAttachmentInfo{
				MountPoint: fsAttachment.MountPoint,
				ReadOnly:   fsAttachment.ReadOnly,
			},
		}
	}
	return results, nil
}

// FilesystemParams returns the parameters for creating the filesystems with
// the specified tags. Mirrors facade FilesystemParams at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:1742
func (a *modelStorageAdapter) FilesystemParams(ctx context.Context, tags []names.FilesystemTag) ([]params.FilesystemParamsResultV5, error) {
	results := make([]params.FilesystemParamsResultV5, len(tags))
	fsModelTags, err := a.storageSvc.GetStorageResourceTagsForModel(ctx)
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}
	for i, tag := range tags {
		uuid, err := a.storageSvc.GetFilesystemUUIDForID(ctx, tag.Id())
		if err != nil {
			// Facade converts FilesystemNotFound to ErrPerm (~line 1811)
			// to avoid leaking entity existence.
			if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
				results[i].Error = errPermResult
			} else {
				results[i].Error = serverError(err)
			}
			continue
		}
		fsParams, err := a.storageSvc.GetFilesystemParams(ctx, uuid)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		rval := params.FilesystemParamsV5{
			Attributes:    make(map[string]any, len(fsParams.Attributes)),
			FilesystemTag: tag.String(),
			Provider:      fsParams.Provider,
			ProviderId:    fsParams.ProviderID,
			SizeMiB:       fsParams.SizeMiB,
			Tags:          fsModelTags,
		}
		for k, v := range fsParams.Attributes {
			rval.Attributes[k] = v
		}
		if fsParams.BackingVolume != nil {
			rval.VolumeTag = names.NewVolumeTag(fsParams.BackingVolume.VolumeID).String()
		}
		results[i].Result = rval
	}
	return results, nil
}

// RemoveFilesystemParams returns the parameters for destroying or releasing
// the filesystems with the specified tags. Mirrors facade
// RemoveFilesystemParams at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:1827
func (a *modelStorageAdapter) RemoveFilesystemParams(ctx context.Context, tags []names.FilesystemTag) ([]params.RemoveFilesystemParamsResult, error) {
	results := make([]params.RemoveFilesystemParamsResult, len(tags))
	for i, tag := range tags {
		uuid, err := a.storageSvc.GetFilesystemUUIDForID(ctx, tag.Id())
		if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"filesystem %q not found", tag.Id(),
			).Add(coreerrors.NotFound))
			continue
		}
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		rp, err := a.storageSvc.GetFilesystemRemovalParams(ctx, uuid)
		if errors.Is(err, storageprovisioningerrors.FilesystemNotDead) {
			results[i].Error = serverError(errors.Errorf(
				"filesystem %q is not yet dead", tag.Id(),
			))
			continue
		}
		if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"filesystem %q not found", tag.Id(),
			).Add(coreerrors.NotFound))
			continue
		}
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		results[i].Result = params.RemoveFilesystemParams{
			Provider:   rp.Provider,
			ProviderId: rp.ProviderID,
			Destroy:    rp.Obliterate,
		}
	}
	return results, nil
}

// FilesystemAttachmentParams returns the parameters for creating the
// filesystem attachments with the specified IDs. Mirrors facade
// FilesystemAttachmentParams/filesystemAttachmentParams at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:2048
func (a *modelStorageAdapter) FilesystemAttachmentParams(ctx context.Context, ids []params.MachineStorageId) ([]params.FilesystemAttachmentParamsResultV5, error) {
	results := make([]params.FilesystemAttachmentParamsResultV5, len(ids))
	for i, id := range ids {
		hostTag, err := names.ParseTag(id.MachineTag)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		if hostTag.Kind() != names.MachineTagKind && hostTag.Kind() != names.UnitTagKind {
			results[i].Error = serverError(errors.Errorf(
				"filesystem attachment host tag %q not valid", hostTag,
			).Add(coreerrors.NotValid))
			continue
		}
		filesystemTag, err := names.ParseFilesystemTag(id.AttachmentTag)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		// Use getFilesystemAttachmentUUID helper which handles both
		// MachineTag and UnitTag hosts, matching facade
		// getFilesystemAttachmentUUID at line ~2680.
		attachmentUUID, err := a.getFilesystemAttachmentUUID(ctx, filesystemTag, hostTag)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		fsParams, err := a.storageSvc.GetFilesystemAttachmentParams(ctx, attachmentUUID)
		if errors.Is(err, storageprovisioningerrors.FilesystemAttachmentNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"filesystem attachment for filesystem %q and host %q not found",
				filesystemTag, hostTag,
			).Add(coreerrors.NotFound))
			continue
		}
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		// Instance ID selection mirrors facade V6 selector at line ~2054:
		// prefer MachineInstanceID, fall back to CAASInstanceID.
		instanceID := fsParams.MachineInstanceID
		if instanceID == "" {
			instanceID = fsParams.CAASInstanceID
		}
		results[i].Result = params.FilesystemAttachmentParamsV5{
			FilesystemTag:        filesystemTag.String(),
			MachineTag:           hostTag.String(),
			InstanceId:           instanceID,
			Provider:             fsParams.Provider,
			FilesystemProviderId: fsParams.FilesystemProviderID,
			// AttachmentProviderId is *string, matching V5/V6 struct fields
			// populated by the facade at line ~2118.
			AttachmentProviderId: fsParams.FilesystemAttachmentProviderID,
			MountPoint:           fsParams.MountPoint,
			ReadOnly:             fsParams.CharmStorageReadOnly,
		}
	}
	return results, nil
}

// SetFilesystemInfo records the details of newly provisioned filesystems.
// Mirrors facade SetFilesystemInfo at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:2184
func (a *modelStorageAdapter) SetFilesystemInfo(ctx context.Context, fss []params.Filesystem) ([]params.ErrorResult, error) {
	results := make([]params.ErrorResult, len(fss))
	for i, fs := range fss {
		filesystemTag, err := names.ParseFilesystemTag(fs.FilesystemTag)
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"parsing filesystem tag %q: %w", fs.FilesystemTag, err,
			))
			continue
		}
		if fs.Info.Pool != "" {
			results[i].Error = serverError(errors.New("pool field must not be set"))
			continue
		}
		info := storageprovisioning.FilesystemProvisionedInfo{
			ProviderID: fs.Info.ProviderId,
			SizeMiB:    fs.Info.SizeMiB,
		}
		err = a.storageSvc.SetFilesystemProvisionedInfo(ctx, filesystemTag.Id(), info)
		if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
			results[i].Error = serverError(errors.Errorf(
				"filesystem %q not found", filesystemTag.Id(),
			).Add(coreerrors.NotFound))
			continue
		}
		if err != nil {
			results[i].Error = serverError(err)
		}
	}
	return results, nil
}

// SetFilesystemAttachmentInfo records the details of newly provisioned
// filesystem attachments. Mirrors facade SetFilesystemAttachmentInfo at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:2550
func (a *modelStorageAdapter) SetFilesystemAttachmentInfo(ctx context.Context, fas []params.FilesystemAttachment) ([]params.ErrorResult, error) {
	results := make([]params.ErrorResult, len(fas))
	for i, fa := range fas {
		hostTag, err := names.ParseTag(fa.MachineTag)
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"parsing host tag %q: %w", fa.MachineTag, err,
			))
			continue
		}
		filesystemTag, err := names.ParseFilesystemTag(fa.FilesystemTag)
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"parsing filesystem tag %q: %w", fa.FilesystemTag, err,
			))
			continue
		}
		info := storageprovisioning.FilesystemAttachmentProvisionedInfo{
			MountPoint: fa.Info.MountPoint,
			ReadOnly:   fa.Info.ReadOnly,
		}
		switch tag := hostTag.(type) {
		case names.MachineTag:
			machineUUID, err := a.getMachineUUID(ctx, tag)
			if err != nil {
				results[i].Error = serverError(err)
				continue
			}
			err = a.storageSvc.SetFilesystemAttachmentProvisionedInfoForMachine(ctx, filesystemTag.Id(), machineUUID, info)
			// FilesystemNotFound mapping mirrors facade line ~2613.
			if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
				results[i].Error = serverError(errors.Errorf(
					"filesystem %q not found", filesystemTag.Id(),
				).Add(coreerrors.NotFound))
			} else if err != nil {
				results[i].Error = serverError(err)
			}
		case names.UnitTag:
			unitUUID, err := a.getUnitUUID(ctx, tag)
			if err != nil {
				results[i].Error = serverError(err)
				continue
			}
			err = a.storageSvc.SetFilesystemAttachmentProvisionedInfoForUnit(ctx, filesystemTag.Id(), unitUUID, info)
			// FilesystemNotFound mapping mirrors facade line ~2613.
			if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
				results[i].Error = serverError(errors.Errorf(
					"filesystem %q not found", filesystemTag.Id(),
				).Add(coreerrors.NotFound))
			} else if err != nil {
				results[i].Error = serverError(err)
			}
		default:
			// Invalid host tag message mirrors facade line ~2610.
			results[i].Error = serverError(errors.Errorf(
				"filesystem attachment host tag %q not found", tag,
			).Add(coreerrors.NotValid))
		}
	}
	return results, nil
}

// LifecycleManager implementation

// Life returns the lifecycle state of the specified entities.
// Mirrors facade Life/lifeForVolume/lifeForFilesystem at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:238
func (a *modelStorageAdapter) Life(ctx context.Context, tags []names.Tag) ([]params.LifeResult, error) {
	results := make([]params.LifeResult, len(tags))
	for i, tag := range tags {
		var l domainlife.Life
		var err error
		switch tag := tag.(type) {
		case names.VolumeTag:
			uuid, e := a.storageSvc.GetVolumeUUIDForID(ctx, tag.Id())
			if errors.Is(e, storageprovisioningerrors.VolumeNotFound) {
				results[i].Error = serverError(errors.Errorf(
					"volume not found for id %q", tag.Id(),
				).Add(coreerrors.NotFound))
				continue
			} else if e != nil {
				results[i].Error = serverError(errors.Errorf(
					"getting volume UUID for id %q: %v", tag.Id(), e,
				))
				continue
			}
			l, err = a.storageSvc.GetVolumeLife(ctx, uuid)
			if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
				results[i].Error = serverError(errors.Errorf(
					"volume not found for id %q", tag.Id(),
				).Add(coreerrors.NotFound))
				continue
			}
		case names.FilesystemTag:
			uuid, e := a.storageSvc.GetFilesystemUUIDForID(ctx, tag.Id())
			if errors.Is(e, storageprovisioningerrors.FilesystemNotFound) {
				results[i].Error = serverError(errors.Errorf(
					"filesystem not found for id %q", tag.Id(),
				).Add(coreerrors.NotFound))
				continue
			} else if e != nil {
				results[i].Error = serverError(errors.Errorf(
					"getting filesystem UUID for id %q: %v", tag.Id(), e,
				))
				continue
			}
			l, err = a.storageSvc.GetFilesystemLife(ctx, uuid)
			if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
				results[i].Error = serverError(errors.Errorf(
					"filesystem not found for id %q", tag.Id(),
				).Add(coreerrors.NotFound))
				continue
			}
		default:
			results[i].Error = serverError(errors.Errorf(
				"invalid tag %q, expected volume or filesystem", tag,
			).Add(coreerrors.NotValid))
			continue
		}
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"getting volume UUID for id %q: %v", tag, err,
			))
			continue
		}
		val, err := l.Value()
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		results[i].Life = val
	}
	return results, nil
}

// Remove removes volumes and filesystems from state.
// Mirrors facade Remove/removeVolume/removeFilesystem at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:2852
func (a *modelStorageAdapter) Remove(ctx context.Context, tags []names.Tag) ([]params.ErrorResult, error) {
	results := make([]params.ErrorResult, len(tags))
	for i, tag := range tags {
		var err error
		switch tag := tag.(type) {
		case names.VolumeTag:
			uuid, e := a.storageSvc.GetVolumeUUIDForID(ctx, tag.Id())
			if errors.Is(e, storageprovisioningerrors.VolumeNotFound) {
				// Already removed, matching facade line ~2900.
				continue
			}
			if e != nil {
				err = e
				break
			}
			err = a.removalSvc.RemoveDeadVolume(ctx, uuid)
			if errors.Is(err, storageprovisioningerrors.VolumeNotDead) {
				err = errors.Errorf("volume %q is not yet dead", tag.Id())
			} else if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
				// Already removed, matching facade line ~2912.
				err = nil
			}
		case names.FilesystemTag:
			uuid, e := a.storageSvc.GetFilesystemUUIDForID(ctx, tag.Id())
			if errors.Is(e, storageprovisioningerrors.FilesystemNotFound) {
				// Already removed, matching facade line ~2929.
				continue
			}
			if e != nil {
				err = e
				break
			}
			err = a.removalSvc.RemoveDeadFilesystem(ctx, uuid)
			if errors.Is(err, storageprovisioningerrors.FilesystemNotDead) {
				err = errors.Errorf("filesystem %q is not yet dead", tag.Id())
			} else if errors.Is(err, storageprovisioningerrors.FilesystemNotFound) {
				// Already removed, matching facade line ~2942.
				err = nil
			}
		default:
			// Invalid tag with NotValid code, matching facade line ~2876.
			results[i].Error = serverError(errors.Errorf(
				"tag %q invalid", tag,
			).Add(coreerrors.NotValid))
			continue
		}
		if err != nil {
			results[i].Error = serverError(err)
		}
	}
	return results, nil
}

// AttachmentLife returns the lifecycle state of the specified machine/entity
// attachments. Mirrors facade AttachmentLife/volumeAttachmentLife/
// filesystemAttachmentLife at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:2808
func (a *modelStorageAdapter) AttachmentLife(ctx context.Context, ids []params.MachineStorageId) ([]params.LifeResult, error) {
	results := make([]params.LifeResult, len(ids))
	for i, id := range ids {
		hostTag, err := names.ParseTag(id.MachineTag)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		attachmentTag, err := names.ParseTag(id.AttachmentTag)
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		var l domainlife.Life
		switch tag := attachmentTag.(type) {
		case names.VolumeTag:
			// Volume attachments only supported on machines, matching
			// facade volumeAttachmentLife line ~2775.
			machineTag, ok := hostTag.(names.MachineTag)
			if !ok {
				results[i].Error = serverError(errors.New(
					"volume attachments only supported on machines",
				).Add(coreerrors.NotImplemented))
				continue
			}
			machineUUID, err := a.getMachineUUID(ctx, machineTag)
			if err != nil {
				results[i].Error = serverError(err)
				continue
			}
			uuid, err := a.getVolumeAttachmentUUID(ctx, tag, machineUUID)
			if err != nil {
				results[i].Error = serverError(err)
				continue
			}
			l, err = a.storageSvc.GetVolumeAttachmentLife(ctx, uuid)
			if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
				results[i].Error = serverError(errors.Errorf(
					"volume attachment %q on %q not found",
					tag.Id(), hostTag.Id(),
				).Add(coreerrors.NotFound))
				continue
			} else if err != nil {
				results[i].Error = serverError(errors.Errorf(
					"getting volume attachment life for %q on %q: %w",
					tag.Id(), hostTag.Id(), err,
				))
				continue
			}
		case names.FilesystemTag:
			// Use getFilesystemAttachmentUUID helper for both machine
			// and unit hosts, matching facade filesystemAttachmentLife.
			fsUUID, err := a.getFilesystemAttachmentUUID(ctx, tag, hostTag)
			if err != nil {
				results[i].Error = serverError(err)
				continue
			}
			l, err = a.storageSvc.GetFilesystemAttachmentLife(ctx, fsUUID)
			if errors.Is(err, storageprovisioningerrors.FilesystemAttachmentNotFound) {
				results[i].Error = serverError(errors.Errorf(
					"filesystem attachment %q on %q not found",
					tag.Id(), hostTag.Id(),
				).Add(coreerrors.NotFound))
				continue
			} else if err != nil {
				results[i].Error = serverError(errors.Errorf(
					"getting filesystem attachment life for %q on %q: %w",
					tag.Id(), hostTag.Id(), err,
				))
				continue
			}
		default:
			// Invalid attachment tag with NotValid code, matching facade
			// line ~2836.
			results[i].Error = serverError(errors.Errorf(
				"attachment tag %q is not a valid", attachmentTag,
			).Add(coreerrors.NotValid))
			continue
		}
		val, err := l.Value()
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		results[i].Life = val
	}
	return results, nil
}

// RemoveAttachments removes the specified machine/entity attachments from
// state. Mirrors facade RemoveAttachment/removeVolumeAttachment/
// removeFilesystemAttachment at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:2953
func (a *modelStorageAdapter) RemoveAttachments(ctx context.Context, ids []params.MachineStorageId) ([]params.ErrorResult, error) {
	results := make([]params.ErrorResult, len(ids))
	for i, id := range ids {
		hostTag, err := names.ParseTag(id.MachineTag)
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"tag %q invalid", id.MachineTag,
			).Add(coreerrors.NotValid))
			continue
		}
		attachmentTag, err := names.ParseTag(id.AttachmentTag)
		if err != nil {
			results[i].Error = serverError(errors.Errorf(
				"tag %q invalid", id.AttachmentTag,
			).Add(coreerrors.NotValid))
			continue
		}
		switch tag := attachmentTag.(type) {
		case names.VolumeTag:
			machineTag, ok := hostTag.(names.MachineTag)
			if !ok {
				results[i].Error = serverError(errors.Errorf(
					"tag %q on host %q invalid", id.AttachmentTag, id.MachineTag,
				).Add(coreerrors.NotValid))
				continue
			}
			machineUUID, err := a.getMachineUUID(ctx, machineTag)
			if err != nil {
				results[i].Error = serverError(err)
				continue
			}
			// getVolumeAttachmentUUID maps domain sentinels to CodeNotFound.
			// coreerrors.NotFound is treated as success (already removed),
			// matching facade line ~3011.
			uuid, err := a.getVolumeAttachmentUUID(ctx, tag, machineUUID)
			if errors.Is(err, coreerrors.NotFound) {
				continue
			}
			if err != nil {
				results[i].Error = serverError(err)
				continue
			}
			// MarkVolumeAttachmentAsDead error mapping mirrors facade
			// lines ~3018-3027.
			err = a.removalSvc.MarkVolumeAttachmentAsDead(ctx, uuid)
			if errors.Is(err, removalerrors.EntityStillAlive) {
				results[i].Error = serverError(errors.Errorf(
					"volume %q attachment to machine %q is still alive",
					tag.Id(), machineTag.Id(),
				))
			} else if errors.Is(err, storageprovisioningerrors.VolumeAttachmentNotFound) {
				continue
			} else if err != nil {
				results[i].Error = serverError(err)
			}
		case names.FilesystemTag:
			// getFilesystemAttachmentUUID handles both machine and unit
			// hosts, mapping domain sentinels to CodeNotFound.
			// coreerrors.NotFound is treated as success (already removed),
			// matching facade line ~3038.
			fsUUID, err := a.getFilesystemAttachmentUUID(ctx, tag, hostTag)
			if errors.Is(err, coreerrors.NotFound) {
				continue
			}
			if err != nil {
				results[i].Error = serverError(err)
				continue
			}
			// MarkFilesystemAttachmentAsDead error mapping mirrors facade
			// lines ~3045-3054.
			err = a.removalSvc.MarkFilesystemAttachmentAsDead(ctx, fsUUID)
			if errors.Is(err, removalerrors.EntityStillAlive) {
				results[i].Error = serverError(errors.Errorf(
					"filesystem %q attachment to %s %q is still alive",
					tag.Id(), hostTag.Kind(), hostTag.Id(),
				))
			} else if errors.Is(err, storageprovisioningerrors.FilesystemAttachmentNotFound) {
				continue
			} else if err != nil {
				results[i].Error = serverError(err)
			}
		default:
			results[i].Error = serverError(errors.Errorf(
				"tag %q on host %q invalid", id.AttachmentTag, id.MachineTag,
			).Add(coreerrors.NotValid))
		}
	}
	return results, nil
}

// MachineAccessor implementation

func (a *modelStorageAdapter) WatchMachine(ctx context.Context, m names.MachineTag) (watcher.NotifyWatcher, error) {
	machineUUID, err := a.getMachineUUID(ctx, m)
	if err != nil {
		return nil, jujuerrors.Trace(err)
	}
	return a.machineSvc.WatchMachineCloudInstances(ctx, machineUUID)
}

// InstanceIds returns the provider specific instance ID for each machine,
// or a CodeNotProvisioned error if not set. Mirrors facade InstanceIdGetter
// at:
//
//	apiserver/common/instanceidgetter.go:50
func (a *modelStorageAdapter) InstanceIds(ctx context.Context, tags []names.MachineTag) ([]params.StringResult, error) {
	results := make([]params.StringResult, len(tags))
	for i, tag := range tags {
		machineUUID, err := a.machineSvc.GetMachineUUID(ctx, machine.Name(tag.Id()))
		if errors.Is(err, machineerrors.MachineNotFound) {
			results[i].Error = serverError(jujuerrors.NotFoundf("machine %s", tag.Id()))
			continue
		}
		if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		instanceId, err := a.machineSvc.GetInstanceID(ctx, machineUUID)
		if errors.Is(err, machineerrors.NotProvisioned) {
			results[i].Error = serverError(jujuerrors.NotProvisionedf("machine %s", tag.Id()))
			continue
		} else if err != nil {
			results[i].Error = serverError(err)
			continue
		}
		results[i].Result = string(instanceId)
	}
	return results, nil
}

// StatusSetter implementation

// SetStatus sets the status of each given storage artefact. Mirrors facade
// SetStatus at:
//
//	apiserver/facades/agent/storageprovisioner/storageprovisioner.go:3059
//
// Unlike the previous localadapter implementation which returned on the
// first error, this now processes all entities and combines errors,
// matching the facade's per-item ErrorResults + API client Combine().
func (a *modelStorageAdapter) SetStatus(ctx context.Context, args []params.EntityStatusArgs) error {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args)),
	}
	if len(args) == 0 {
		return nil
	}
	// Set Since timestamp using clock, matching facade line ~3085.
	now := a.clock.Now()
	for i, arg := range args {
		tag, err := names.ParseTag(arg.Tag)
		if err != nil {
			result.Results[i].Error = serverError(err)
			continue
		}
		sInfo := status.StatusInfo{
			Status:  status.Status(arg.Status),
			Message: arg.Info,
			Data:    arg.Data,
			Since:   &now,
		}
		var statusErr error
		switch tag := tag.(type) {
		case names.FilesystemTag:
			statusErr = a.statusSvc.SetFilesystemStatus(ctx, tag.Id(), sInfo)
			// Map FilesystemNotFound to CodeNotFound, matching facade
			// line ~3091.
			if errors.Is(statusErr, storageerrors.FilesystemNotFound) {
				statusErr = errors.Errorf(
					"filesystem %q not found", tag.Id(),
				).Add(coreerrors.NotFound)
			}
		case names.VolumeTag:
			statusErr = a.statusSvc.SetVolumeStatus(ctx, tag.Id(), sInfo)
			// Map VolumeNotFound to CodeNotFound, matching facade
			// line ~3098.
			if errors.Is(statusErr, storageerrors.VolumeNotFound) {
				statusErr = errors.Errorf(
					"volume %q not found", tag.Id(),
				).Add(coreerrors.NotFound)
			}
		default:
			// Non-volume/non-filesystem tags get ErrPerm, matching
			// facade line ~3104.
			statusErr = errPerm
		}
		result.Results[i].Error = serverError(statusErr)
	}
	return result.Combine()
}

// newAttachmentIDWatcher creates a MachineStorageIDsWatcher that wraps a
// StringsWatcher and maps domain UUID strings to MachineStorageID values.
func newAttachmentIDWatcher(
	source watcher.StringsWatcher,
	mapper func(context.Context, ...string) ([]watcher.MachineStorageID, error),
) (watcher.MachineStorageIDsWatcher, error) {
	w := &attachmentIDWatcher{
		source: source,
		mapper: mapper,
		out:    make(chan []watcher.MachineStorageID, 1),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "attachment-watcher",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{source},
	})
	return w, errors.Capture(err)
}

type attachmentIDWatcher struct {
	catacomb catacomb.Catacomb
	source   watcher.StringsWatcher
	mapper   func(context.Context, ...string) ([]watcher.MachineStorageID, error)
	out      chan []watcher.MachineStorageID
}

func (w *attachmentIDWatcher) loop() error {
	defer close(w.out)
	var (
		changes []watcher.MachineStorageID
		out     chan []watcher.MachineStorageID
		initial = true
	)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case events, ok := <-w.source.Changes():
			if !ok {
				return errors.Errorf("source watcher closed")
			}
			if !initial && len(events) == 0 {
				continue
			}
			results, err := w.mapper(w.catacomb.Context(context.Background()), events...)
			if err != nil {
				return errors.Errorf("processing changes: %v", err)
			}
			if !initial && len(results) == 0 {
				continue
			}
			if changes == nil {
				changes = results
			} else {
				changes = append(changes, results...)
			}
			out = w.out
		case out <- changes:
			changes = nil
			out = nil
			initial = false
		}
	}
}

func (w *attachmentIDWatcher) Changes() <-chan []watcher.MachineStorageID {
	return w.out
}

func (w *attachmentIDWatcher) Kill() {
	w.catacomb.Kill(nil)
}

func (w *attachmentIDWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *attachmentIDWatcher) Err() error {
	return w.catacomb.Err()
}
