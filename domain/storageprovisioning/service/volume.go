// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
)

type VolumeState interface {
	// GetVolumeAttachmentIDs returns the
	// [storageprovisioning.VolumeAttachmentID] information for each
	// volume attachment uuid supplied. If a uuid does not exist or isn't
	// attached to either a machine or a unit then it will not exist in the
	// result.
	GetVolumeAttachmentIDs(ctx context.Context, uuids []string) (map[string]storageprovisioning.VolumeAttachmentID, error)

	// GetVolumeAttachmentLifeForNetNode returns a mapping of volume
	// attachment uuid to the current life value for each machine provisioned
	// volume attachment that is to be provisioned by the machine owning the
	// supplied net node.
	GetVolumeAttachmentLifeForNetNode(ctx context.Context, netNodeUUID string) (map[string]life.Life, error)

	// GetVolumeAttachmentPlanLifeForNetNode returns a mapping of volume
	// attachment plan volume id to the current life value for each volume
	// attachment plan. The volume id of attachment plans is returned instead of
	// the uuid because the caller for the watcher works off of this
	// information.
	GetVolumeAttachmentPlanLifeForNetNode(ctx context.Context, netNodeUUID string) (map[string]life.Life, error)

	// GetVolumeLifeForNetNode returns a mapping of volume id to current
	// life value for each machine provisioned volume that is to be
	// provisioned by the machine owning the supplied net node.
	GetVolumeLifeForNetNode(ctx context.Context, netNodeUUID string) (map[string]life.Life, error)

	// InitialWatchStatementMachineProvisionedVolumes returns both the
	// namespace for watching volume life changes where the volume is
	// machine provisioned. On top of this the initial query for getting all
	// volumes in the model that are machine provisioned is returned.
	//
	// Only volumes that can be provisioned by the machine connected to the
	// supplied net node will be emitted.
	InitialWatchStatementMachineProvisionedVolumes(netNodeUUID string) (string, eventsource.Query[map[string]life.Life])

	// InitialWatchStatementModelProvisionedVolumes returns both the
	// namespace for watching volume life changes where the volume is
	// model provisioned. On top of this the initial query for getting all
	// volumes in the model that are model provisioned is returned.
	InitialWatchStatementModelProvisionedVolumes() (string, eventsource.NamespaceQuery)

	// InitialWatchStatementMachineProvisionedVolumeAttachments returns
	// both the namespace for watching volume attachment life changes where
	// the volume attachment is machine provisioned. On top of this the
	// initial query for getting all volume attachments in the model that
	// are machine provisioned is returned.
	//
	// Only volume attachments that can be provisioned by the machine
	// connected to the supplied net node will be emitted.
	InitialWatchStatementMachineProvisionedVolumeAttachments(netNodeUUID string) (string, eventsource.Query[map[string]life.Life])

	// InitialWatchStatementModelProvisionedVolumeAttachments returns both
	// the namespace for watching volume attachment life changes where the
	// volume attachment is model provisioned. On top of this the initial
	// query for getting all volume attachments in the model that are model
	// provisioned is returned.
	InitialWatchStatementModelProvisionedVolumeAttachments() (string, eventsource.NamespaceQuery)

	// InitialWatchStatementVolumeAttachmentPlans returns both the namespace for
	// watching volume attachment plan life changes. On top of this the initial
	// query for getting all volume attachment plan volume ids in the model that
	// are for the given net node uuid.
	InitialWatchStatementVolumeAttachmentPlans(netNodeUUID string) (string, eventsource.Query[map[string]life.Life])
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
	return s.st.GetVolumeAttachmentIDs(ctx, uuids)
}

// WatchModelProvisionedVolumes returns a watcher that emits volume IDs,
// whenever a model provisioned volume's life changes.
func (s *Service) WatchModelProvisionedVolumes(
	ctx context.Context,
) (watcher.StringsWatcher, error) {
	ns, initialQuery := s.st.InitialWatchStatementModelProvisionedVolumes()
	return s.watcherFactory.NewNamespaceWatcher(
		initialQuery,
		eventsource.NamespaceFilter(ns, changestream.All))
}

// WatchMachineProvisionedVolumes returns a watcher that emits volume IDs,
// whenever a machine provisioned volume's life changes for the given machine.
//
// The following errors may be returned:
// - [github.com/juju/juju/core/errors.NotValid] when the provided machine uuid
// is not valid.
// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
// machine exists for the provided machine UUUID.
func (s *Service) WatchMachineProvisionedVolumes(
	ctx context.Context, machineUUID machine.UUID,
) (watcher.StringsWatcher, error) {
	if err := machineUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	netNodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, machineUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	lifeGetter := func(ctx context.Context) (map[string]life.Life, error) {
		return s.st.GetVolumeLifeForNetNode(ctx, netNodeUUID)
	}

	ns, initialLifeQuery := s.st.InitialWatchStatementMachineProvisionedVolumes(netNodeUUID)
	initialQuery, mapper := MakeEntityLifePrerequisites(initialLifeQuery, lifeGetter)
	filter := eventsource.PredicateFilter(ns, changestream.All, eventsource.EqualsPredicate(netNodeUUID))

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
	ns, initialQuery := s.st.InitialWatchStatementModelProvisionedVolumeAttachments()
	return s.watcherFactory.NewNamespaceWatcher(initialQuery,
		eventsource.NamespaceFilter(ns, changestream.All))
}

// WatchMachineProvisionedVolumeAttachments returns a watcher that emits volume
// attachment UUIDs, whenever a machine provisioned volume attachment's life
// changes for the given machine.
//
// The following errors may be returned:
// - [github.com/juju/juju/core/errors.NotValid] when the provided machine uuid
// is not valid.
// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
// machine exists for the provided machine UUUID.
func (s *Service) WatchMachineProvisionedVolumeAttachments(
	ctx context.Context, machineUUID machine.UUID,
) (watcher.StringsWatcher, error) {
	if err := machineUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	netNodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, machineUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	lifeGetter := func(ctx context.Context) (map[string]life.Life, error) {
		return s.st.GetVolumeAttachmentLifeForNetNode(ctx, netNodeUUID)
	}

	ns, initialLifeQuery := s.st.InitialWatchStatementMachineProvisionedVolumeAttachments(netNodeUUID)
	initialQuery, mapper := MakeEntityLifePrerequisites(initialLifeQuery, lifeGetter)
	filter := eventsource.PredicateFilter(ns, changestream.All, eventsource.EqualsPredicate(netNodeUUID))

	w, err := s.watcherFactory.NewNamespaceMapperWatcher(initialQuery, mapper, filter)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return w, nil
}

// WatchVolumeAttachmentPlans returns a watcher that emits volume attachment
// plan volume ids, whenever a volume attachment plan's life changes for the given
// machine.
//
// The following errors may be returned:
// - [github.com/juju/juju/core/errors.NotValid] when the provided machine uuid
// is not valid.
// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
// machine exists for the provided machine UUUID.
func (s *Service) WatchVolumeAttachmentPlans(
	ctx context.Context, machineUUID machine.UUID,
) (watcher.StringsWatcher, error) {
	if err := machineUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	netNodeUUID, err := s.st.GetMachineNetNodeUUID(ctx, machineUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	lifeGetter := func(ctx context.Context) (map[string]life.Life, error) {
		return s.st.GetVolumeAttachmentPlanLifeForNetNode(ctx, netNodeUUID)
	}

	ns, initialLifeQuery := s.st.InitialWatchStatementVolumeAttachmentPlans(netNodeUUID)
	initialQuery, mapper := MakeEntityLifePrerequisites(initialLifeQuery, lifeGetter)
	filter := eventsource.PredicateFilter(ns, changestream.All, eventsource.EqualsPredicate(netNodeUUID))

	w, err := s.watcherFactory.NewNamespaceMapperWatcher(initialQuery, mapper, filter)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return w, nil
}
