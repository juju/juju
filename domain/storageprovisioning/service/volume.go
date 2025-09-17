// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	coreblockdevice "github.com/juju/juju/core/blockdevice"
	corechangestream "github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/blockdevice"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	"github.com/juju/juju/internal/errors"
)

type VolumeState interface {
	// GetVolume returns the volume information for the specified volume uuid.
	GetVolume(
		context.Context, storageprovisioning.VolumeUUID,
	) (storageprovisioning.Volume, error)

	// GetVolumeAttachmentIDs returns the
	// [storageprovisioning.VolumeAttachmentID] information for each
	// volume attachment uuid supplied. If a uuid does not exist or isn't
	// attached to either a machine or a unit then it will not exist in the
	// result.
	//
	// It is not considered an error if a volume attachment uuid no longer
	// exists as it is expected the caller has already satisfied this requirement
	// themselves.
	//
	// This function exists to help keep supporting storage provisioning facades
	// that have a very week data model about what a volume attachment is
	// attached to.
	//
	// All returned values will have either the machine name or unit name value
	// filled out in the [storageprovisioning.VolumeAttachmentID] struct.
	GetVolumeAttachmentIDs(
		ctx context.Context, uuids []string,
	) (map[string]storageprovisioning.VolumeAttachmentID, error)

	// GetVolumeAttachmentLife returns the current life value for a
	// volume attachment uuid.
	//
	// The following errors may be returned:
	// - [storageprovisioningerrors.VolumeAttachmentNotFound]
	// when no volume attachment exists for the provided uuid.
	GetVolumeAttachmentLife(
		context.Context,
		storageprovisioning.VolumeAttachmentUUID,
	) (domainlife.Life, error)

	// GetVolumeAttachmentLifeForNetNode returns a mapping of volume
	// attachment uuid to the current life value for each machine provisioned
	// volume attachment that is to be provisioned by the machine owning the
	// supplied net node.
	GetVolumeAttachmentLifeForNetNode(
		context.Context, domainnetwork.NetNodeUUID,
	) (map[string]domainlife.Life, error)

	// GetVolumeAttachmentPlanLifeForNetNode returns a mapping of volume
	// attachment plan volume ID to the current life value for each volume
	// attachment plan. The volume ID of attachment plans is returned instead of
	// the uuid because the caller for the watcher works off of this
	// information.
	GetVolumeAttachmentPlanLifeForNetNode(
		context.Context, domainnetwork.NetNodeUUID,
	) (map[string]domainlife.Life, error)

	// GetVolumeAttachmentUUIDForVolumeNetNode returns the volume attachment uuid
	// for the supplied volume uuid which is attached to the given net node
	// uuid.
	//
	// The following errors may be returned:
	// - [storageprovisioningerrors.VolumeNotFound]
	// when no volume exists for the supplied volume uuid.
	// - [storageprovisioningerrors.VolumeAttachmentNotFound]
	// when no volume attachment exists for the supplied values.
	// - [networkerrors.NetNodeNotFound] when no net node exists for the
	// supplied uuid.
	GetVolumeAttachmentUUIDForVolumeNetNode(
		context.Context,
		storageprovisioning.VolumeUUID,
		domainnetwork.NetNodeUUID,
	) (storageprovisioning.VolumeAttachmentUUID, error)

	// GetVolumeAttachmentPlanUUIDForVolumeNetNode returns the volume attachment
	// uuid for the supplied volume uuid which is attached to the given net node
	// uuid.
	GetVolumeAttachmentPlanUUIDForVolumeNetNode(
		context.Context,
		storageprovisioning.VolumeUUID,
		domainnetwork.NetNodeUUID,
	) (storageprovisioning.VolumeAttachmentPlanUUID, error)

	// GetVolumeAttachment returns the volume attachment for the supplied volume
	// attachment uuid.
	GetVolumeAttachment(
		context.Context,
		storageprovisioning.VolumeAttachmentUUID,
	) (storageprovisioning.VolumeAttachment, error)

	// GetVolumeLife returns the current life value for a volume uuid.
	//
	// The following errors may be returned:
	// - [storageprovisioningerrors.VolumeNotFound]
	// when no volume exists for the provided volume uuid.
	GetVolumeLife(
		context.Context, storageprovisioning.VolumeUUID,
	) (domainlife.Life, error)

	// GetVolumeLifeForNetNode returns a mapping of volume ID to current
	// life value for each machine provisioned volume that is to be
	// provisioned by the machine owning the supplied net node.
	GetVolumeLifeForNetNode(
		context.Context, domainnetwork.NetNodeUUID,
	) (map[string]domainlife.Life, error)

	// GetVolumeUUIDForID returns the uuid for a volume with the supplied
	// id.
	//
	// The following errors may be returned:
	// - [storageprovisioningerrors.VolumeNotFound]
	// when no volume exists for the provided volume uuid.
	GetVolumeUUIDForID(
		context.Context, string,
	) (storageprovisioning.VolumeUUID, error)

	// GetVolumeParams returns the volume params for the supplied uuid.
	//
	// The following errors may be returned:
	// - [storageprovisioningerrors.VolumeNotFound] when no volume exists for
	// the uuid.
	GetVolumeParams(
		context.Context, storageprovisioning.VolumeUUID,
	) (storageprovisioning.VolumeParams, error)

	// GetVolumeAttachmentParams retrieves the attachment params for the given
	// volume attachment.
	//
	// The following errors may be returned:
	// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
	// attachment exists for the supplied uuid.
	GetVolumeAttachmentParams(
		context.Context, storageprovisioning.VolumeAttachmentUUID,
	) (storageprovisioning.VolumeAttachmentParams, error)

	// SetVolumeProvisionedInfo sets the provisioned information for the given
	// volume.
	SetVolumeProvisionedInfo(
		context.Context, storageprovisioning.VolumeUUID,
		storageprovisioning.VolumeProvisionedInfo,
	) error

	// SetVolumeAttachmentProvisionedInfo sets on the provided volume the
	// information about the provisioned volume attachment.
	SetVolumeAttachmentProvisionedInfo(
		context.Context,
		storageprovisioning.VolumeAttachmentUUID,
		storageprovisioning.VolumeAttachmentProvisionedInfo,
	) error

	// InitialWatchStatementMachineProvisionedVolumes returns both the
	// namespace for watching volume life changes where the volume is
	// machine provisioned and the initial query for getting the set of volumes
	// that are provisioned by the supplied machine in the model.
	//
	// Only volumes that can be provisioned by the machine connected to the
	// supplied net node will be emitted.
	InitialWatchStatementMachineProvisionedVolumes(
		domainnetwork.NetNodeUUID,
	) (string, eventsource.Query[map[string]domainlife.Life])

	// InitialWatchStatementModelProvisionedVolumes returns both the
	// namespace for watching volume life changes where the volume is
	// model provisioned and the initial query for getting the set of volumes
	// that are model provisioned.
	InitialWatchStatementModelProvisionedVolumes() (string, eventsource.NamespaceQuery)

	// InitialWatchStatementMachineProvisionedVolumeAttachments returns
	// both the namespace for watching volume attachment life changes where
	// the volume attachment is machine provisioned and the initial query for
	// getting the set of volume attachments in the model that are provisioned
	// by the supplied machine in the model.
	//
	// Only volume attachments that can be provisioned by the machine
	// connected to the supplied net node will be emitted.
	InitialWatchStatementMachineProvisionedVolumeAttachments(
		domainnetwork.NetNodeUUID,
	) (string, eventsource.Query[map[string]domainlife.Life])

	// InitialWatchStatementModelProvisionedVolumeAttachments returns both
	// the namespace for watching volume attachment life changes where the
	// volume attachment is model provisioned and the initial query for getting
	// the set of volume attachments that are model provisioned.
	InitialWatchStatementModelProvisionedVolumeAttachments() (string, eventsource.NamespaceQuery)

	// InitialWatchStatementVolumeAttachmentPlans returns both the namespace for
	// watching volume attachment plan life changes and the initial query for
	// getting the set of volume attachment plans in the model that are
	// provisioned by the supplied machine in the model.
	InitialWatchStatementVolumeAttachmentPlans(
		domainnetwork.NetNodeUUID,
	) (string, eventsource.Query[map[string]domainlife.Life])
}

// GetVolumeAttachmentIDs returns the
// [storageprovisioning.VolumeAttachmentID] information for each of the
// supplied volume attachment uuids. If a volume attachment does exist
// for a supplied uuid or if a volume attachment is not attached to either a
// machine or unit then this uuid will be left out of the final result.
//
// It is not considered an error if a volume attachment uuid no longer
// exists as it is expected the caller has already satisfied this requirement
// themselves.
//
// This function exists to help keep supporting storage provisioning facades
// that have a very week data model about what a volume attachment is
// attached to.
//
// All returned values will have either the machine name or unit name value
// filled out in the [storageprovisioning.VolumeAttachmentID] struct.
func (s *Service) GetVolumeAttachmentIDs(
	ctx context.Context, uuids []string,
) (map[string]storageprovisioning.VolumeAttachmentID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.GetVolumeAttachmentIDs(ctx, uuids)
}

// GetVolumeParams returns the volume params for the supplied UUID.
// The following errors may be returned:
// - [coreerrors.NotValid] when the supplied volume attachment UUID is not
// valid.
// - [storageprovisioningerrors.VolumeNotFound] when no volume
// attachment exists for the supplied values.
func (s *Service) GetVolumeParams(
	ctx context.Context, uuid storageprovisioning.VolumeUUID,
) (storageprovisioning.VolumeParams, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := uuid.Validate(); err != nil {
		return storageprovisioning.VolumeParams{}, errors.New(
			"volume uuid is not valid",
		).Add(coreerrors.NotValid)
	}

	return s.st.GetVolumeParams(ctx, uuid)
}

// GetVolumeAttachmentParams retrieves the attachment parameters for a given
// volume attachment.
// The following errors may be returned:
// - [coreerrors.NotValid] when the supplied volume attachment UUID is not
// valid.
// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
// attachment exists for the supplied values.
func (s *Service) GetVolumeAttachmentParams(
	ctx context.Context,
	volumeAttachmentUUID storageprovisioning.VolumeAttachmentUUID,
) (storageprovisioning.VolumeAttachmentParams, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := volumeAttachmentUUID.Validate(); err != nil {
		return storageprovisioning.VolumeAttachmentParams{}, errors.New(
			"volume attachment uuid is not valid",
		).Add(coreerrors.NotValid)
	}

	return s.st.GetVolumeAttachmentParams(ctx, volumeAttachmentUUID)
}

// GetVolumeAttachmentLife returns the current life value for a volume
// attachment uuid.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the volume attachment uuid is not valid.
// - [storageprovisioningerrors.VolumeAttachmentNotFound]
// when no volume attachment exists for the provided uuid.
func (s *Service) GetVolumeAttachmentLife(
	ctx context.Context,
	uuid storageprovisioning.VolumeAttachmentUUID,
) (domainlife.Life, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := uuid.Validate(); err != nil {
		return -1, errors.Errorf(
			"validating volume attachment uuid: %w", err,
		).Add(coreerrors.NotValid)
	}

	life, err := s.st.GetVolumeAttachmentLife(ctx, uuid)
	if err != nil {
		return -1, errors.Capture(err)
	}
	return life, nil
}

// GetVolumeAttachment returns information about a volume attachment.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the volume attachment uuid is not valid.
// - [storageprovisioningerrors.VolumeAttachmentNotFound]
// when no volume attachment exists for the provided uuid.
func (s *Service) GetVolumeAttachment(
	ctx context.Context,
	uuid storageprovisioning.VolumeAttachmentUUID,
) (storageprovisioning.VolumeAttachment, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := uuid.Validate(); err != nil {
		return storageprovisioning.VolumeAttachment{}, errors.Errorf(
			"validating volume attachment uuid: %w", err,
		).Add(coreerrors.NotValid)
	}

	va, err := s.st.GetVolumeAttachment(ctx, uuid)
	if err != nil {
		return storageprovisioning.VolumeAttachment{}, errors.Capture(err)
	}
	return va, nil
}

// GetVolumeAttachmentPlanUUIDForVolumeIDMachine returns the volume attachment
// plan uuid for the supplied volume ID which is attached to the machine.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the provided machine uuid is not valid.
// - [storageprovisioningerrors.VolumeNotFound] when no volume exists for the
// supplied id.
// - [storageprovisioningerrors.VolumeAttachmentPlanNotFound] when no volume
// attachment plan exists for the supplied values.
// - [machineerrors.MachineNotFound] when no machine exists for the provided
// machine uuid.
func (s *Service) GetVolumeAttachmentPlanUUIDForVolumeIDMachine(
	ctx context.Context,
	volumeID string,
	machineUUID coremachine.UUID,
) (storageprovisioning.VolumeAttachmentPlanUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineUUID.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	netNodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, machineUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	volumeUUID, err := s.st.GetVolumeUUIDForID(ctx, volumeID)
	if err != nil {
		return "", errors.Capture(err)
	}

	vapUUID, err := s.st.GetVolumeAttachmentPlanUUIDForVolumeNetNode(
		ctx, volumeUUID, netNodeUUID)
	if errors.Is(err, networkerrors.NetNodeNotFound) {
		return "", errors.Errorf(
			"machine %q does not exist", machineUUID.String(),
		).Add(machineerrors.MachineNotFound)
	} else if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
		return "", errors.Errorf(
			"volume %q does not exist", volumeID,
		).Add(storageprovisioningerrors.VolumeNotFound)
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return vapUUID, nil
}

// GetVolumeAttachmentUUIDForVolumeIDMachine returns the volume attachment
// uuid for the supplied volume ID which is attached to the machine.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the provided machine uuid is not valid.
// - [storageprovisioningerrors.VolumeNotFound] when no volume exists for the
// supplied id.
// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
// attachment exists for the supplied values.
// - [machineerrors.MachineNotFound] when no machine exists for the provided
// machine uuid.
func (s *Service) GetVolumeAttachmentUUIDForVolumeIDMachine(
	ctx context.Context,
	volumeID string,
	machineUUID coremachine.UUID,
) (storageprovisioning.VolumeAttachmentUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineUUID.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	netNodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, machineUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	vUUID, err := s.st.GetVolumeUUIDForID(ctx, volumeID)
	if err != nil {
		return "", errors.Errorf(
			"getting volume uuid for id %q: %w", volumeID, err,
		)
	}

	uuid, err := s.st.GetVolumeAttachmentUUIDForVolumeNetNode(
		ctx, vUUID, netNodeUUID,
	)
	if errors.Is(err, networkerrors.NetNodeNotFound) {
		return "", errors.Errorf(
			"machine %q does not exist", machineUUID.String(),
		).Add(machineerrors.MachineNotFound)
	} else if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
		return "", errors.Errorf(
			"volume %q does not exist", volumeID,
		).Add(storageprovisioningerrors.VolumeNotFound)
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return uuid, nil
}

// GetVolumeAttachmentUUIDForVolumeIDUnit returns the volume attachment uuid
// for the supplied volume ID which is attached to the unit.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the provided unit uuid is not valid.
// - [storageprovisioningerrors.VolumeNotFound] when no volume exists for the
// supplied id.
// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
// attachment exists for the supplied values.
// - [applicationerrors.UnitNotFound] when no unit exists for the provided unit
// uuid.
func (s *Service) GetVolumeAttachmentUUIDForVolumeIDUnit(
	ctx context.Context,
	volumeID string,
	unitUUID coreunit.UUID,
) (storageprovisioning.VolumeAttachmentUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitUUID.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	netNodeUUID, err := s.st.GetUnitNetNodeUUID(ctx, unitUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	vUUID, err := s.st.GetVolumeUUIDForID(ctx, volumeID)
	if err != nil {
		return "", errors.Errorf(
			"getting volume uuid for id %q: %w", volumeID, err,
		)
	}

	uuid, err := s.st.GetVolumeAttachmentUUIDForVolumeNetNode(
		ctx, vUUID, netNodeUUID,
	)
	if errors.Is(err, networkerrors.NetNodeNotFound) {
		return "", errors.Errorf(
			"unit %q does not exist", unitUUID.String(),
		).Add(applicationerrors.UnitNotFound)
	} else if errors.Is(err, storageprovisioningerrors.VolumeNotFound) {
		return "", errors.Errorf(
			"volume %q does not exist", volumeID,
		).Add(storageprovisioningerrors.VolumeNotFound)
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return uuid, nil
}

// GetVolumeLife returns the current life value for a volume uuid.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the volume uuid is not valid.
// - [storageprovisioningerrors.VolumeNotFound] when no volume exists for the
// provided volume uuid.
func (s *Service) GetVolumeLife(
	ctx context.Context,
	uuid storageprovisioning.VolumeUUID,
) (domainlife.Life, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := uuid.Validate(); err != nil {
		return -1, errors.Errorf(
			"validating volume uuid: %w", err,
		).Add(coreerrors.NotValid)
	}

	life, err := s.st.GetVolumeLife(ctx, uuid)
	if err != nil {
		return -1, errors.Capture(err)
	}
	return life, nil
}

// GetVolumeUUIDForID returns the uuid for a volume with the supplied
// id.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeNotFound] when no volume exists for the
// provided volume uuid.
func (s *Service) GetVolumeUUIDForID(
	ctx context.Context, volumeID string,
) (storageprovisioning.VolumeUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	uuid, err := s.st.GetVolumeUUIDForID(ctx, volumeID)
	if err != nil {
		return "", errors.Capture(err)
	}

	return uuid, nil
}

// GetVolumeByID retrieves the [storageprovisioning.Volume] for the given
// volume ID.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeNotFound] when no volume exists for the
// provided volume uuid.
func (s *Service) GetVolumeByID(
	ctx context.Context, volumeID string,
) (storageprovisioning.Volume, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	uuid, err := s.st.GetVolumeUUIDForID(ctx, volumeID)
	if err != nil {
		return storageprovisioning.Volume{}, errors.Capture(err)
	}

	volume, err := s.st.GetVolume(ctx, uuid)
	if err != nil {
		return storageprovisioning.Volume{}, errors.Capture(err)
	}

	return volume, nil
}

// GetBlockDeviceForVolumeAttachment returns information about the block
// device set for the specified volume attachment.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the volume attachment uuid is not valid.
// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
// attachment exists for the provided uuid.
// - [storageprovisioningerrors.VolumeAttachmentWithoutBlockDevice] when the
// volume attachment does not yet have a block device.
func (s *Service) GetBlockDeviceForVolumeAttachment(
	ctx context.Context, uuid storageprovisioning.VolumeAttachmentUUID,
) (coreblockdevice.BlockDevice, error) {
	return coreblockdevice.BlockDevice{}, nil
}

// WatchModelProvisionedVolumes returns a watcher that emits volume IDs,
// whenever a model provisioned volume's life changes.
func (s *Service) WatchModelProvisionedVolumes(
	ctx context.Context,
) (watcher.StringsWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	ns, initialQuery := s.st.InitialWatchStatementModelProvisionedVolumes()
	return s.watcherFactory.NewNamespaceWatcher(
		ctx,
		initialQuery,
		"model provisioned volume watcher",
		eventsource.NamespaceFilter(ns, corechangestream.All))
}

// WatchMachineProvisionedVolumes returns a watcher that emits volume IDs,
// whenever the given machine's provisioned volume life changes.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the provided machine uuid is not valid.
// - [machineerrors.MachineNotFound] when no machine exists for the provided
// machine UUUID.
func (s *Service) WatchMachineProvisionedVolumes(
	ctx context.Context, machineUUID coremachine.UUID,
) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	netNodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, machineUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	lifeGetter := func(ctx context.Context) (map[string]domainlife.Life, error) {
		return s.st.GetVolumeLifeForNetNode(ctx, netNodeUUID)
	}

	ns, initialLifeQuery := s.st.InitialWatchStatementMachineProvisionedVolumes(netNodeUUID)
	initialQuery, mapper := makeEntityLifePrerequisites(initialLifeQuery, lifeGetter)
	filter := eventsource.PredicateFilter(
		ns, corechangestream.All, eventsource.EqualsPredicate(netNodeUUID.String()),
	)

	w, err := s.watcherFactory.NewNamespaceMapperWatcher(
		ctx,
		initialQuery,
		fmt.Sprintf("machine provisioned volume watcher for %q", machineUUID),
		mapper, filter)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return w, nil
}

// WatchModelProvisionedVolumeAttachments returns a watcher that emits volume
// attachment UUIDs, whenever a model provisioned volume attachment's life
// changes.
func (s *Service) WatchModelProvisionedVolumeAttachments(
	ctx context.Context,
) (watcher.StringsWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	ns, initialQuery := s.st.InitialWatchStatementModelProvisionedVolumeAttachments()
	return s.watcherFactory.NewNamespaceWatcher(
		ctx,
		initialQuery,
		"model provisioned volume attachment watcher",
		eventsource.NamespaceFilter(ns, corechangestream.All),
	)
}

// WatchMachineProvisionedVolumeAttachments returns a watcher that emits volume
// attachment UUIDs, whenever the given machine's provisioned volume
// attachment's life changes.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the provided machine uuid is not valid.
// - [machineerrors.MachineNotFound] when no machine exists for the provided
// machine UUUID.
func (s *Service) WatchMachineProvisionedVolumeAttachments(
	ctx context.Context, machineUUID coremachine.UUID,
) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	netNodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, machineUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	lifeGetter := func(ctx context.Context) (map[string]domainlife.Life, error) {
		return s.st.GetVolumeAttachmentLifeForNetNode(ctx, netNodeUUID)
	}

	ns, initialLifeQuery := s.st.InitialWatchStatementMachineProvisionedVolumeAttachments(netNodeUUID)
	initialQuery, mapper := makeEntityLifePrerequisites(initialLifeQuery, lifeGetter)
	filter := eventsource.PredicateFilter(
		ns, corechangestream.All, eventsource.EqualsPredicate(netNodeUUID.String()),
	)

	w, err := s.watcherFactory.NewNamespaceMapperWatcher(
		ctx,
		initialQuery,
		fmt.Sprintf("machine provisioned volume attachment watcher for %q", machineUUID),
		mapper, filter)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return w, nil
}

// WatchVolumeAttachmentPlans returns a watcher that emits volume attachment
// plan volume IDs, whenever the given machine's volume attachment plan life
// changes.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the provided machine uuid is not valid.
// - [machineerrors.MachineNotFound] when no machine exists for the provided
// machine UUUID.
func (s *Service) WatchVolumeAttachmentPlans(
	ctx context.Context, machineUUID coremachine.UUID,
) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	netNodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, machineUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	lifeGetter := func(ctx context.Context) (map[string]domainlife.Life, error) {
		return s.st.GetVolumeAttachmentPlanLifeForNetNode(ctx, netNodeUUID)
	}

	ns, initialLifeQuery := s.st.InitialWatchStatementVolumeAttachmentPlans(netNodeUUID)
	initialQuery, mapper := makeEntityLifePrerequisites(initialLifeQuery, lifeGetter)
	filter := eventsource.PredicateFilter(
		ns, corechangestream.All, eventsource.EqualsPredicate(netNodeUUID.String()),
	)

	w, err := s.watcherFactory.NewNamespaceMapperWatcher(
		ctx,
		initialQuery,
		fmt.Sprintf("volume attachment plan watcher for %q", machineUUID),
		mapper, filter)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return w, nil
}

// SetVolumeProvisionedInfo sets on the provided volume the information about
// the provisioned volume.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeNotFound] when no volume exists for the
// provided volume id.
func (s *Service) SetVolumeProvisionedInfo(
	ctx context.Context,
	volumeID string,
	info storageprovisioning.VolumeProvisionedInfo,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	uuid, err := s.st.GetVolumeUUIDForID(ctx, volumeID)
	if err != nil {
		return errors.Capture(err)
	}

	err = s.st.SetVolumeProvisionedInfo(ctx, uuid, info)
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// SetVolumeAttachmentProvisionedInfo sets on the provided volume the information
// about the provisioned volume attachment.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the provided volume attachment uuid or block
// device uuid is not valid.
// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
// attachment exists for the provided volume attachment uuid.
// - [blockdeviceerrors.BlockDeviceNotFound] when no block device exists for a
// given block device uuid.
func (s *Service) SetVolumeAttachmentProvisionedInfo(
	ctx context.Context,
	volumeAttachmentUUID storageprovisioning.VolumeAttachmentUUID,
	info storageprovisioning.VolumeAttachmentProvisionedInfo,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := volumeAttachmentUUID.Validate()
	if err != nil {
		return errors.Errorf(
			"validating volume attachment uuid: %w", err,
		).Add(coreerrors.NotValid)
	}

	if info.BlockDeviceUUID != nil {
		err = info.BlockDeviceUUID.Validate()
		if err != nil {
			return errors.Errorf(
				"validating block device uuid: %w", err,
			).Add(coreerrors.NotValid)
		}
	}

	err = s.st.SetVolumeAttachmentProvisionedInfo(
		ctx, volumeAttachmentUUID, info)
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// GetVolumeAttachmentPlan gets the volume attachment plan for the provided
// uuid.
func (s *Service) GetVolumeAttachmentPlan(
	ctx context.Context, uuid storageprovisioning.VolumeAttachmentPlanUUID,
) (storageprovisioning.VolumeAttachmentPlan, error) {
	return storageprovisioning.VolumeAttachmentPlan{}, errors.New("GetVolumeAttachmentPlan not implemented")
}

// CreateVolumeAttachmentPlan creates a volume attachment plan for the
// provided volume attachment uuid. Returned is the new uuid for the volume
// attachment plan in the model.
func (s *Service) CreateVolumeAttachmentPlan(
	ctx context.Context,
	attachmentUUID storageprovisioning.VolumeAttachmentUUID,
	deviceType storageprovisioning.PlanDeviceType,
	attrs map[string]string,
) (storageprovisioning.VolumeAttachmentPlanUUID, error) {
	return "", errors.New("CreateVolumeAttachmentPlan not implemented")
}

// SetVolumeAttachmentPlanProvisionedInfo sets on the provided volume attachment
// plan information.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeAttachmentPlanNotFound] when no volume
// attachment plan exists for the provided uuid.
func (s *Service) SetVolumeAttachmentPlanProvisionedInfo(
	ctx context.Context,
	uuid storageprovisioning.VolumeAttachmentPlanUUID,
	info storageprovisioning.VolumeAttachmentPlanProvisionedInfo,
) error {
	return errors.New("SetVolumeAttachmentPlanProvisionedInfo not implemented")
}

// SetVolumeAttachmentPlanProvisionedBlockDevice sets on the provided
// volume attachment plan the information about the provisioned block device.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the provided volume attachment plan uuid or
// block device uuid is not valid.
// - [storageprovisioningerrors.VolumeAttachmentNotPlanFound] when no volume
// attachment plan exists for the provided uuid.
// - [blockdeviceerrors.BlockDeviceNotFound] when no block device exists for the
// provided block device uuid.
func (s *Service) SetVolumeAttachmentPlanProvisionedBlockDevice(
	ctx context.Context,
	uuid storageprovisioning.VolumeAttachmentPlanUUID,
	blockDeviceUUID blockdevice.BlockDeviceUUID,
) error {
	return errors.New("SetVolumeAttachmentPlanProvisionedBlockDevice not implemented")
}
