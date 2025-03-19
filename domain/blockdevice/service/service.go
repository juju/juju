// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/filesystem"
	"github.com/juju/juju/internal/errors"
)

type getWatcherFunc = func(
	namespace, changeValue string,
	changeMask changestream.ChangeType,
	mapper eventsource.Mapper,
) (watcher.NotifyWatcher, error)

// State defines an interface for interacting with the underlying state.
type State interface {
	// BlockDevices returns the block devices for a specified machine.
	BlockDevices(ctx context.Context, machineId string) ([]blockdevice.BlockDevice, error)

	// SetMachineBlockDevices updates the block devices for a specified machine.
	SetMachineBlockDevices(ctx context.Context, machineId string, devices ...blockdevice.BlockDevice) error

	// MachineBlockDevices returns all block devices in the model, keyed on
	// machine id.
	MachineBlockDevices(ctx context.Context) ([]blockdevice.MachineBlockDevice, error)

	// WatchBlockDevices returns a new NotifyWatcher watching for
	// changes to block devices associated with the specified machine.
	WatchBlockDevices(ctx context.Context, getWatcher getWatcherFunc, machineId string) (watcher.NotifyWatcher, error)
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	NewValueMapperWatcher(
		namespace, changeValue string,
		changeMask changestream.ChangeType,
		mapper eventsource.Mapper,
	) (watcher.NotifyWatcher, error)
}

// Service defines a service for interacting with the underlying state.
type Service struct {
	st     State
	logger logger.Logger
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State, logger logger.Logger) *Service {
	return &Service{
		st:     st,
		logger: logger,
	}
}

// BlockDevices returns the block devices for a specified machine.
func (s *Service) BlockDevices(ctx context.Context, machineId string) ([]blockdevice.BlockDevice, error) {
	return s.st.BlockDevices(ctx, machineId)
}

// UpdateBlockDevices updates the block devices for a specified machine.
func (s *Service) UpdateBlockDevices(ctx context.Context, machineId string, devices ...blockdevice.BlockDevice) error {
	for i := range devices {
		if devices[i].FilesystemType == "" {
			devices[i].FilesystemType = filesystem.UnspecifiedType
		}
	}
	return s.st.SetMachineBlockDevices(ctx, machineId, devices...)
}

// AllBlockDevices returns all block devices in the model, keyed on machine id.
func (s *Service) AllBlockDevices(ctx context.Context) (map[string]blockdevice.BlockDevice, error) {
	machineDevices, err := s.st.MachineBlockDevices(ctx)
	if err != nil {
		return nil, errors.Errorf("loading all block devices: %w", err)
	}
	result := make(map[string]blockdevice.BlockDevice)
	for _, md := range machineDevices {
		result[md.MachineId] = md.BlockDevice
	}
	return result, nil
}

// WatchableService defines a service for interacting with the underlying state
// and the ability to create watchers.
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new Service for interacting with the underlying
// state and the ability to create watchers.
func NewWatchableService(st State, wf WatcherFactory, logger logger.Logger) *WatchableService {
	return &WatchableService{
		Service: Service{
			st:     st,
			logger: logger,
		},
		watcherFactory: wf,
	}
}

// WatchBlockDevices returns a new NotifyWatcher watching for
// changes to block devices associated with the specified machine.
func (s *WatchableService) WatchBlockDevices(
	ctx context.Context,
	machineId string,
) (watcher.NotifyWatcher, error) {
	return s.st.WatchBlockDevices(ctx, s.watcherFactory.NewValueMapperWatcher, machineId)
}
