// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	"github.com/juju/juju/internal/errors"
)

type VolumeState interface {
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
	GetVolumeAttachmentIDs(ctx context.Context, uuids []string) (map[string]storageprovisioning.VolumeAttachmentID, error)

	// GetVolumeAttachmentLife returns the current life value for a
	// volume attachment uuid.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storageprovisioning/errors.VolumeAttachmentNotFound]
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
		ctx context.Context, netNodeUUID domainnetwork.NetNodeUUID,
	) (map[string]domainlife.Life, error)

	// GetVolumeAttachmentPlanLifeForNetNode returns a mapping of volume
	// attachment plan volume id to the current life value for each volume
	// attachment plan. The volume id of attachment plans is returned instead of
	// the uuid because the caller for the watcher works off of this
	// information.
	GetVolumeAttachmentPlanLifeForNetNode(ctx context.Context, netNodeUUID domainnetwork.NetNodeUUID) (map[string]domainlife.Life, error)

	// GetVolumeAttachmentUUIDForUUIDNetNode returns the volume attachment uuid
	// for the supplied volume uuid which is attached to the given net node
	// uuid.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storageprovisioning/errors.VolumeNotFound]
	// when no volume exists for the supplied volume uuid.
	// - [github.com/juju/juju/domain/storageprovisioning/errors.VolumeAttachmentNotFound]
	// when no volume attachment exists for the supplied values.
	// - [networkerrors.NetNodeNotFound] when no net node exists for the
	// supplied uuid.
	GetVolumeAttachmentUUIDForVolumeNetNode(
		context.Context,
		storageprovisioning.VolumeUUID,
		domainnetwork.NetNodeUUID,
	) (storageprovisioning.VolumeAttachmentUUID, error)

	// GetVolumeLife returns the current life value for a volume uuid.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storageprovisioning/errors.VolumeNotFound]
	// when no volume exists for the provided volume uuid.
	GetVolumeLife(
		context.Context, storageprovisioning.VolumeUUID,
	) (domainlife.Life, error)

	// GetVolumeLifeForNetNode returns a mapping of volume id to current
	// life value for each machine provisioned volume that is to be
	// provisioned by the machine owning the supplied net node.
	GetVolumeLifeForNetNode(ctx context.Context, netNodeUUID domainnetwork.NetNodeUUID) (map[string]domainlife.Life, error)

	// GetVolumeUUIDForID returns the uuid for a volume with the supplied
	// id.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/storageprovisioning/errors.VolumeNotFound]
	// when no volume exists for the provided volume uuid.
	GetVolumeUUIDForID(context.Context, string) (storageprovisioning.VolumeUUID, error)

	// InitialWatchStatementMachineProvisionedVolumes returns both the
	// namespace for watching volume life changes where the volume is
	// machine provisioned and the initial query for getting the set of volumes
	// that are provisioned by the supplied machine in the model.
	//
	// Only volumes that can be provisioned by the machine connected to the
	// supplied net node will be emitted.
	InitialWatchStatementMachineProvisionedVolumes(netNodeUUID domainnetwork.NetNodeUUID) (string, eventsource.Query[map[string]domainlife.Life])

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
	InitialWatchStatementMachineProvisionedVolumeAttachments(netNodeUUID domainnetwork.NetNodeUUID) (string, eventsource.Query[map[string]domainlife.Life])

	// InitialWatchStatementModelProvisionedVolumeAttachments returns both
	// the namespace for watching volume attachment life changes where the
	// volume attachment is model provisioned and the initial query for getting
	// the set of volume attachments that are model provisioned.
	InitialWatchStatementModelProvisionedVolumeAttachments() (string, eventsource.NamespaceQuery)

	// InitialWatchStatementVolumeAttachmentPlans returns both the namespace for
	// watching volume attachment plan life changes and the initial query for
	// getting the set of volume attachment plans in the model that are
	// provisioned by the supplied machine in the model.
	InitialWatchStatementVolumeAttachmentPlans(netNodeUUID domainnetwork.NetNodeUUID) (string, eventsource.Query[map[string]domainlife.Life])
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

// GetVolumeAttachmentLife returns the current life value for a volume
// attachment uuid.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the volume attachment uuid is not valid.
// - [github.com/juju/juju/domain/storageprovisioning/errors.VolumeAttachmentNotFound]
// when no volume attachment exists for the provided uuid.
func (s *Service) GetVolumeAttachmentLife(
	ctx context.Context,
	uuid storageprovisioning.VolumeAttachmentUUID,
) (domainlife.Life, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := uuid.Validate(); err != nil {
		return 0, errors.Errorf(
			"validating volume attachment uuid: %w", err,
		).Add(coreerrors.NotValid)
	}

	life, err := s.st.GetVolumeAttachmentLife(ctx, uuid)
	if err != nil {
		return 0, errors.Capture(err)
	}
	return life, nil
}

// GetVolumeAttachmentUUIDForIDMachine returns the volume attachment
// uuid for the supplied volume id which is attached to the machine.
//
// The following errors may be returned:
// - [corestorage.InvalidStorageID] when the provided id is not valid.
// - [coreerrors.NotValid] when the provided machine uuid is not valid.
// - [storageprovisioningerrors.VolumeNotFound] when no volume exists for the
// supplied id.
// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
// attachment exists for the supplied values.
// - [machineerrors.MachineNotFound] when no machine exists for the provided
// machine uuid.
func (s *Service) GetVolumeAttachmentUUIDForIDMachine(
	ctx context.Context,
	id corestorage.ID,
	machineUUID coremachine.UUID,
) (storageprovisioning.VolumeAttachmentUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := id.Validate(); err != nil {
		return "", errors.Capture(err)
	}
	if err := machineUUID.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	netNodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, machineUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	vUUID, err := s.st.GetVolumeUUIDForID(ctx, id.String())
	if err != nil {
		return "", errors.Errorf(
			"getting volume uuid for id %q: %w", id.String(), err,
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
			"volume %q does not exist", id.String(),
		).Add(storageprovisioningerrors.VolumeNotFound)
	} else if err != nil {
		return "", errors.Capture(err)
	}

	return uuid, nil
}

// GetVolumeAttachmentUUIDForIDUnit returns the volume attachment uuid
// for the supplied volume id which is attached to the unit.
//
// The following errors may be returned:
// - [corestorage.InvalidStorageID] when the provided id is not valid.
// - [coreerrors.NotValid] when the provided unit uuid is not valid.
// - [storageprovisioningerrors.VolumeNotFound] when no volume exists for the
// supplied id.
// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
// attachment exists for the supplied values.
// - [applicationerrors.UnitNotFound] when no unit exists for the provided unit
// uuid.
func (s *Service) GetVolumeAttachmentUUIDForIDUnit(
	ctx context.Context,
	id corestorage.ID,
	unitUUID coreunit.UUID,
) (storageprovisioning.VolumeAttachmentUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := id.Validate(); err != nil {
		return "", errors.Capture(err)
	}
	if err := unitUUID.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	netNodeUUID, err := s.st.GetUnitNetNodeUUID(ctx, unitUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	vUUID, err := s.st.GetVolumeUUIDForID(ctx, id.String())
	if err != nil {
		return "", errors.Errorf(
			"getting volume uuid for id %q: %w", id.String(), err,
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
			"volume %q does not exist", id.String(),
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
// - [github.com/juju/juju/domain/storageprovisioning/errors.VolumeNotFound]
// when no volume exists for the provided volume uuid.
func (s *Service) GetVolumeLife(
	ctx context.Context,
	uuid storageprovisioning.VolumeUUID,
) (domainlife.Life, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := uuid.Validate(); err != nil {
		return 0, errors.Errorf(
			"validating volume uuid: %w", err,
		).Add(coreerrors.NotValid)
	}

	life, err := s.st.GetVolumeLife(ctx, uuid)
	if err != nil {
		return 0, errors.Capture(err)
	}
	return life, nil
}

// GetVolumeUUIDForID returns the uuid for a volume with the supplied
// id.
//
// The following errors may be returned:
// - [corestorage.InvalidStorageID] when the provided id is not valid.
// - [github.com/juju/juju/domain/storageprovisioning/errors.VolumeNotFound]
// when no volume exists for the provided volume uuid.
func (s *Service) GetVolumeUUIDForID(
	ctx context.Context, id corestorage.ID,
) (storageprovisioning.VolumeUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := id.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	uuid, err := s.st.GetVolumeUUIDForID(ctx, id.String())
	if err != nil {
		return "", errors.Capture(err)
	}

	return uuid, nil
}

// WatchModelProvisionedVolumes returns a watcher that emits volume IDs,
// whenever a model provisioned volume's life changes.
func (s *Service) WatchModelProvisionedVolumes(
	ctx context.Context,
) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	ns, initialQuery := s.st.InitialWatchStatementModelProvisionedVolumes()
	return s.watcherFactory.NewNamespaceWatcher(
		initialQuery,
		eventsource.NamespaceFilter(ns, changestream.All))
}

// WatchMachineProvisionedVolumes returns a watcher that emits volume IDs,
// whenever the given machine's provisioned volume life changes.
//
// The following errors may be returned:
// - [github.com/juju/juju/core/errors.NotValid] when the provided machine uuid
// is not valid.
// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
// machine exists for the provided machine UUUID.
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
		ns, changestream.All, eventsource.EqualsPredicate(netNodeUUID.String()),
	)

	w, err := s.watcherFactory.NewNamespaceMapperWatcher(
		initialQuery, mapper, filter)
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	ns, initialQuery := s.st.InitialWatchStatementModelProvisionedVolumeAttachments()
	return s.watcherFactory.NewNamespaceWatcher(initialQuery,
		eventsource.NamespaceFilter(ns, changestream.All))
}

// WatchMachineProvisionedVolumeAttachments returns a watcher that emits volume
// attachment UUIDs, whenever the given machine's provisioned volume
// attachment's life changes.
//
// The following errors may be returned:
// - [github.com/juju/juju/core/errors.NotValid] when the provided machine uuid
// is not valid.
// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
// machine exists for the provided machine UUUID.
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
		ns, changestream.All, eventsource.EqualsPredicate(netNodeUUID.String()),
	)

	w, err := s.watcherFactory.NewNamespaceMapperWatcher(initialQuery, mapper, filter)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return w, nil
}

// WatchVolumeAttachmentPlans returns a watcher that emits volume attachment
// plan volume ids, whenever the given machine's volume attachment plan life
// changes.
//
// The following errors may be returned:
// - [github.com/juju/juju/core/errors.NotValid] when the provided machine uuid
// is not valid.
// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
// machine exists for the provided machine UUUID.
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
		ns, changestream.All, eventsource.EqualsPredicate(netNodeUUID.String()),
	)

	w, err := s.watcherFactory.NewNamespaceMapperWatcher(initialQuery, mapper, filter)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return w, nil
}
