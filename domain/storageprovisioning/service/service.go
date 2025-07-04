// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/errors"
)

// Service provides the interface for provisioning storage within a model.
type Service struct {
	watcherFactory WatcherFactory

	st State
}

// State is the accumulation of all the requirements [Service] has for
// persisting and watching changes to storage instances in the model.
type State interface {
	FilesystemState
	VolumeState

	// CheckMachineIsDead checks to see if a machine is dead, returning true
	// when the life of the machine is dead.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
	// machine exists for the provided uuid.
	CheckMachineIsDead(context.Context, machine.UUID) (bool, error)

	// GetMachineNetNodeUUID retrieves the net node uuid associated with provided
	// machine.
	//
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
	// machine exists for the provided uuid.
	GetMachineNetNodeUUID(context.Context, machine.UUID) (domainnetwork.NetNodeUUID, error)

	// NamespaceForWatchMachineCloudInstance returns the change stream namespace
	// for watching machine cloud instance changes.
	NamespaceForWatchMachineCloudInstance() string
}

// WatcherFactory instances return watchers for a given namespace and UUID.
type WatcherFactory interface {
	// NewNamespaceMapperWatcher returns a new watcher that receives changes
	// from the input base watcher's db/queue. Change-log events will be emitted
	// only if the filter accepts them, and dispatching the notifications via
	// the Changes channel, once the mapper has processed them. Filtering of
	// values is done first by the filter, and then by the mapper. Based on the
	// mapper's logic a subset of them (or none) may be emitted. A filter option
	// is required, though additional filter options can be provided.
	NewNamespaceMapperWatcher(
		initialStateQuery eventsource.NamespaceQuery,
		mapper eventsource.Mapper,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)

	// NewNamespaceWatcher returns a new watcher that filters changes from the input
	// base watcher's db/queue. Change-log events will be emitted only if the filter
	// accepts them, and dispatching the notifications via the Changes channel. A
	// filter option is required, though additional filter options can be provided.
	NewNamespaceWatcher(
		initialQuery eventsource.NamespaceQuery,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)

	// NewNotifyWatcher returns a new watcher that filters changes from the input
	// base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
		filterOption eventsource.FilterOption,
		filterOptions ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
}

// NewService creates a new [Service] instance with the provided state for
// provisioning storage instances in the model.
func NewService(st State, wf WatcherFactory) *Service {
	return &Service{
		st:             st,
		watcherFactory: wf,
	}
}

// WatchMachineCloudInstance returns a watcher that fires when a machine's cloud
// instance info is changed.
//
// The following errors may be returned:
// - [machineerrors.MachineNotFound] when no machine exists for the provided
// uuid.
// - [machineerrors.MachineIsDead] when the machine is dead meaning it is about
// to go away.
func (s *Service) WatchMachineCloudInstance(
	ctx context.Context, machineUUID machine.UUID,
) (watcher.NotifyWatcher, error) {
	dead, err := s.st.CheckMachineIsDead(ctx, machineUUID)
	if err != nil {
		return nil, errors.Errorf("checking if machine is dead: %w", err)
	}

	if dead {
		return nil, errors.Errorf("machine %q is dead", machineUUID).Add(
			machineerrors.MachineIsDead,
		)
	}

	ns := s.st.NamespaceForWatchMachineCloudInstance()
	filter := eventsource.PredicateFilter(ns, changestream.All,
		eventsource.EqualsPredicate(machineUUID.String()))
	return s.watcherFactory.NewNotifyWatcher(filter)
}
