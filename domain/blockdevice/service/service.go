// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/blockdevice"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	BlockDevices(ctx context.Context, machine string) ([]blockdevice.BlockDevice, error)
	WatchBlockDevices(
		ctx context.Context,
		getWatcher func(
			namespace, changeValue string,
			changeMask changestream.ChangeType,
			predicate eventsource.Predicate,
		) (watcher.NotifyWatcher, error),
		machineId string,
	) (watcher.NotifyWatcher, error)
}

// Logger facilitates emitting log messages.
type Logger interface {
	Debugf(string, ...interface{})
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	NewValuePredicateWatcher(
		namespace, changeValue string,
		changeMask changestream.ChangeType,
		predicate eventsource.Predicate,
	) (watcher.NotifyWatcher, error)
}

// Service defines a service for interacting with the underlying state.
type Service struct {
	st             State
	watcherFactory WatcherFactory
	logger         Logger
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State, wf WatcherFactory, logger Logger) *Service {
	return &Service{
		st:             st,
		watcherFactory: wf,
		logger:         logger,
	}
}

// WatchBlockDevices returns a new NotifyWatcher watching for
// changes to block devices associated with the specified machine.
func (s *Service) WatchBlockDevices(
	ctx context.Context,
	machineId string,
) (watcher.NotifyWatcher, error) {
	return s.st.WatchBlockDevices(ctx, s.watcherFactory.NewValuePredicateWatcher, machineId)
}
