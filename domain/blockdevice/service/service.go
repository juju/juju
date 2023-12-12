// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/blockdevice"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	BlockDevices(ctx context.Context, machine string) ([]blockdevice.BlockDevice, error)
	GetMachineInfo(ctx context.Context, machineId string) (string, domain.Life, error)
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
	machineUUID, life, err := s.st.GetMachineInfo(ctx, machineId)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if life == domain.Dead {
		return nil, errors.Errorf("cannot watch block devices on dead machine %q", machineId)
	}

	predicate := func(ctx context.Context, db database.TxnRunner, changes []changestream.ChangeEvent) (bool, error) {
		for _, ch := range changes {
			if ch.Changed() == machineUUID {
				return true, nil
			}
		}
		return false, nil
	}
	baseWatcher, err := s.watcherFactory.NewValuePredicateWatcher("block_device_machine", machineUUID, changestream.All, predicate)
	if err != nil {
		return nil, errors.Annotatef(err, "watching machine %q block devices", machineId)
	}
	return newBlockDeviceWatcher(s.st, baseWatcher, machineId)
}
