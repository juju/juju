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
	"github.com/juju/juju/internal/errors"
)

type VolumeState interface {
	InitialWatchStatementModelProvisionedVolumes() (string, eventsource.NamespaceQuery)
	InitialWatchStatementMachineProvisionedVolumes(netNodeUUID string) (string, eventsource.Query[map[string]life.Life])

	InitialWatchStatementModelProvisionedVolumeAttachments() (string, eventsource.NamespaceQuery)
	InitialWatchStatementMachineProvisionedVolumeAttachments(netNodeUUID string) (string, eventsource.Query[map[string]life.Life])

	InitialWatchStatementVolumeAttachmentPlans(netNodeUUID string) (string, eventsource.Query[map[string]life.Life])

	GetVolumeLifeForNetNode(ctx context.Context, netNodeUUID string) (map[string]life.Life, error)
	GetVolumeAttachmentLifeForNetNode(ctx context.Context, netNodeUUID string) (map[string]life.Life, error)
	GetVolumeAttachmentPlanLifeForNetNode(ctx context.Context, netNodeUUID string) (map[string]life.Life, error)
}

// WatchModelProvisionedVolumes returns a watcher that emits filesystem IDs,
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
// plan UUIDs, whenever a volume attachment plan's life changes for the given
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
