// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"maps"
	"slices"

	coreblockdevice "github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/blockdevice"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name machine.Name) (machine.UUID, error)

	// BlockDevices returns the BlockDevices for the specified machine.
	BlockDevices(
		ctx context.Context, machineUUID machine.UUID,
	) (map[string]coreblockdevice.BlockDevice, error)

	// UpdateMachineBlockDevices updates the block devices for the specified
	// machine.
	UpdateMachineBlockDevices(
		ctx context.Context, machineUUID machine.UUID,
		added map[string]coreblockdevice.BlockDevice,
		updated map[string]coreblockdevice.BlockDevice,
		removeable []string,
	) error

	// MachineBlockDevices retrieves block devices for all machines.
	MachineBlockDevices(
		ctx context.Context,
	) (map[machine.Name][]coreblockdevice.BlockDevice, error)

	// NamespaceForBlockDevices returns the change stream namespace for watching
	// block devices.
	NamespaceForWatchBlockDevices() string
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNotifyWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
		ctx context.Context,
		summary string,
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
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

// BlockDevices returns the BlockDevices for the specified machine.
//
// The following errors may be returned:
// - [errors.NotValid] when the machine uuid is not valid.
// - [machineerrors.MachineNotFound] when the machine is not found.
// - [machineerrors.MachineIsDead] when the machine is dead.
func (s *Service) BlockDevices(
	ctx context.Context, machineUUID machine.UUID,
) ([]coreblockdevice.BlockDevice, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := machineUUID.Validate()
	if err != nil {
		return nil, err
	}

	blockDevices, err := s.st.BlockDevices(ctx, machineUUID)
	if err != nil {
		return nil, err
	}
	return slices.Collect(maps.Values(blockDevices)), nil
}

// UpdateMachineBlockDevices updates the block devices for the specified
// machine.
//
// The following errors may be returned:
// - [errors.NotValid] when the machine uuid is not valid.
// - [machineerrors.MachineNotFound] when the machine is not found.
// - [machineerrors.MachineIsDead] when the machine is dead.
func (s *Service) UpdateBlockDevices(
	ctx context.Context, machineUUID machine.UUID,
	devices []coreblockdevice.BlockDevice,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := machineUUID.Validate()
	if err != nil {
		return err
	}

	existing, err := s.st.BlockDevices(ctx, machineUUID)
	if err != nil {
		return err
	}

	devices = slices.Clone(devices)
	added := map[string]coreblockdevice.BlockDevice{}
	updated := map[string]coreblockdevice.BlockDevice{}
	removed := []string{}
updated:
	for devUUID, dev := range existing {
		for i, candidate := range devices {
			if blockdevice.SameDevice(dev, candidate) {
				updated[devUUID] = candidate
				devices = slices.Delete(devices, i, i+1)
				continue updated
			}
		}
		removed = append(removed, devUUID)
	}
	for _, dev := range devices {
		devUUID, err := uuid.NewUUID()
		if err != nil {
			return errors.Capture(err)
		}
		added[devUUID.String()] = dev
	}

	return s.st.UpdateMachineBlockDevices(
		ctx, machineUUID, added, updated, removed)
}

// SetBlockDevices overrides all current block devices on the named machine.
//
// The following errors may be returned:
// - [errors.NotValid] when the machine uuid is not valid.
// - [machineerrors.MachineNotFound] when the machine is not found.
// - [machineerrors.MachineIsDead] when the machine is dead.
func (s *Service) SetBlockDevices(
	ctx context.Context, machineName machine.Name,
	devices []coreblockdevice.BlockDevice,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	machineUUID, err := s.st.GetMachineUUID(ctx, machineName)
	if err != nil {
		return err
	}

	existing, err := s.st.BlockDevices(ctx, machineUUID)
	if err != nil {
		return err
	}

	added := make(map[string]coreblockdevice.BlockDevice, len(devices))
	for _, dev := range devices {
		devUUID, err := uuid.NewUUID()
		if err != nil {
			return errors.Capture(err)
		}
		added[devUUID.String()] = dev
	}

	removed := slices.Collect(maps.Keys(existing))
	return s.st.UpdateMachineBlockDevices(
		ctx, machineUUID, added, nil, removed)
}

// AllBlockDevices retrieves block devices for all machines.
func (s *Service) AllBlockDevices(
	ctx context.Context,
) (map[machine.Name][]coreblockdevice.BlockDevice, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	blockDevices, err := s.st.MachineBlockDevices(ctx)
	if err != nil {
		return nil, errors.Errorf("loading all block devices: %w", err)
	}
	return blockDevices, nil
}

// WatchableService defines a service for interacting with the underlying state
// and the ability to create watchers.
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new Service for interacting with the underlying
// state and the ability to create watchers.
func NewWatchableService(
	st State, wf WatcherFactory, logger logger.Logger,
) *WatchableService {
	return &WatchableService{
		Service: Service{
			st:     st,
			logger: logger,
		},
		watcherFactory: wf,
	}
}

// WatchBlockDevices returns a new NotifyWatcher watching for changes to block
// devices associated with the specified machine.
//
// The following errors may be returned:
// - [errors.NotValid] when the machine uuid is not valid.
func (s *WatchableService) WatchBlockDevices(
	ctx context.Context,
	machineUUID machine.UUID,
) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := machineUUID.Validate()
	if err != nil {
		return nil, err
	}

	w, err := s.watcherFactory.NewNotifyWatcher(
		ctx,
		fmt.Sprintf("block devices watcher for machine %q", machineUUID),
		eventsource.PredicateFilter(
			s.st.NamespaceForWatchBlockDevices(),
			changestream.All,
			eventsource.EqualsPredicate(machineUUID.String()),
		),
	)
	if err != nil {
		return nil, errors.Errorf(
			"watching block devices for machine %q: %w",
			machineUUID, err,
		)
	}
	return w, nil
}
