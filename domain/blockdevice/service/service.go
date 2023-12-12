// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/blockdevice"
	"github.com/juju/juju/domain/blockdevice/state"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	BlockDevices(ctx context.Context, machine string) ([]blockdevice.BlockDevice, error)
	GetMachineInfo(ctx context.Context, machine string) (string, int, error)
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
	machine string,
) (watcher.NotifyWatcher, error) {
	machineUUID, life, err := s.st.GetMachineInfo(ctx, machine)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if life == state.Dead {
		return nil, errors.Errorf("cannot watch block devices on dead machine %q", machine)
	}

	predicate := func(ctx context.Context, db database.TxnRunner, changes []changestream.ChangeEvent) (bool, error) {
		machineUUIDs := set.NewStrings()
		for _, ch := range changes {
			machineUUIDs.Add(ch.Changed())
		}
		s.logger.Debugf("block device changes for machines: %s", machineUUIDs.Values())
		return machineUUIDs.Contains(machineUUID), nil
	}
	baseWatcher, err := s.watcherFactory.NewValuePredicateWatcher("block_device_machine", machineUUID, changestream.All, predicate)
	if err != nil {
		return nil, errors.Annotatef(err, "watching machine block devices")
	}
	return newBlockDeviceWatcher(s.st, baseWatcher, machine)
}
