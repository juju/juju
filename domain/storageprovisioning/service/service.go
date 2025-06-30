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
)

type Service struct {
	watcherFactory WatcherFactory

	st State
}

type State interface {
	FilesystemState
	VolumeState

	GetMachineNetNodeUUID(ctx context.Context, machineUUID machine.UUID) (string, error)

	NamespaceForWatchMachineCloudInstance() string
}

// WatcherFactory instances return watchers for a given namespace and UUID.
type WatcherFactory interface {
	// NewNotifyWatcher returns a new watcher that filters changes from the input
	// base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
		filterOption eventsource.FilterOption,
		filterOptions ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)

	// NewNamespaceWatcher returns a new watcher that filters changes from the input
	// base watcher's db/queue. Change-log events will be emitted only if the filter
	// accepts them, and dispatching the notifications via the Changes channel. A
	// filter option is required, though additional filter options can be provided.
	NewNamespaceWatcher(
		initialQuery eventsource.NamespaceQuery,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)

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
}

func NewService(st State, wf WatcherFactory) *Service {
	return &Service{
		st:             st,
		watcherFactory: wf,
	}
}

// WatchMachineCloudInstance returns a watcher that fires when a machine's cloud
// instance info is changed.
func (s *Service) WatchMachineCloudInstance(
	ctx context.Context, machineUUID machine.UUID,
) (watcher.NotifyWatcher, error) {
	ns := s.st.NamespaceForWatchMachineCloudInstance()
	filter := eventsource.PredicateFilter(ns, changestream.All,
		eventsource.EqualsPredicate(machineUUID.String()))
	return s.watcherFactory.NewNotifyWatcher(filter)
}

func MakeLifeGetterMapperBits(eventsource.Query[map[string]life.Life], func(context.Context) (map[string]life.Life, error)) (eventsource.NamespaceQuery, eventsource.Mapper) {
	return nil, nil
}
