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
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/blockdevice"
	"github.com/juju/juju/internal/errors"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	// GetMachineUUIDByName returns the UUID of a machine identified by its name.
	GetMachineUUIDByName(
		ctx context.Context, name machine.Name,
	) (machine.UUID, error)

	// GetBlockDevice retrieves the info for the specified block device.
	GetBlockDevice(
		ctx context.Context, uuid blockdevice.BlockDeviceUUID,
	) (coreblockdevice.BlockDevice, error)

	// ListBlockDevices returns the BlockDevices for the specified UUIDs.
	ListBlockDevices(ctx context.Context, uuids ...string) ([]blockdevice.BlockDeviceDetails, error)

	// GetBlockDevicesForMachine returns the BlockDevices for the specified
	// machine.
	GetBlockDevicesForMachine(
		ctx context.Context, machineUUID machine.UUID,
	) (map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice, error)

	// UpdateBlockDevicesForMachine updates the block devices for the specified
	// machine.
	UpdateBlockDevicesForMachine(
		ctx context.Context, machineUUID machine.UUID,
		added map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice,
		updated map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice,
		removeable []blockdevice.BlockDeviceUUID,
	) error

	// GetBlockDevicesForAllMachines retrieves block devices for all machines.
	GetBlockDevicesForAllMachines(
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

// GetBlockDevicesForMachine returns the BlockDevices for the specified machine.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the machine uuid is not valid.
// - [machineerrors.MachineNotFound] when the machine is not found.
// - [machineerrors.MachineIsDead] when the machine is dead.
func (s *Service) GetBlockDevicesForMachine(
	ctx context.Context, machineUUID machine.UUID,
) ([]coreblockdevice.BlockDevice, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := machineUUID.Validate()
	if err != nil {
		return nil, err
	}

	blockDevices, err := s.st.GetBlockDevicesForMachine(ctx, machineUUID)
	if err != nil {
		return nil, err
	}
	return slices.Collect(maps.Values(blockDevices)), nil
}

// UpdateBlockDevicesForMachine updates the block devices for the specified
// machine.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the machine uuid is not valid.
// - [machineerrors.MachineNotFound] when the machine is not found.
// - [machineerrors.MachineIsDead] when the machine is dead.
func (s *Service) UpdateBlockDevicesForMachine(
	ctx context.Context, machineUUID machine.UUID,
	devices []coreblockdevice.BlockDevice,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := machineUUID.Validate()
	if err != nil {
		return err
	}

	existing, err := s.st.GetBlockDevicesForMachine(ctx, machineUUID)
	if err != nil {
		return err
	}

	devices = slices.Clone(devices)
	added := map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice{}
	updated := map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice{}
	removed := []blockdevice.BlockDeviceUUID{}
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
		devUUID, err := blockdevice.NewBlockDeviceUUID()
		if err != nil {
			return errors.Capture(err)
		}
		added[devUUID] = dev
	}

	return s.st.UpdateBlockDevicesForMachine(
		ctx, machineUUID, added, updated, removed)
}

// MatchOrCreateBlockDevice matches an existing block device to the provided
// block device, otherwise it creates one that matches the provided device.
// It returns the UUID of the block device.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the machine uuid is not valid.
// - [machineerrors.MachineNotFound] when the machine is not found.
// - [machineerrors.MachineIsDead] when the machine is dead.
func (s *Service) MatchOrCreateBlockDevice(
	ctx context.Context, machineUUID machine.UUID,
	device coreblockdevice.BlockDevice,
) (blockdevice.BlockDeviceUUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := machineUUID.Validate()
	if err != nil {
		return "", err
	}

	existing, err := s.st.GetBlockDevicesForMachine(ctx, machineUUID)
	if err != nil {
		return "", err
	}

	for devUUID, dev := range existing {
		if blockdevice.SameDevice(dev, device) {
			return devUUID, nil
		}
	}

	devUUID, err := blockdevice.NewBlockDeviceUUID()
	if err != nil {
		return "", nil
	}

	added := map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice{
		devUUID: device,
	}
	err = s.st.UpdateBlockDevicesForMachine(ctx, machineUUID, added, nil, nil)
	if err != nil {
		return "", err
	}

	return devUUID, nil
}

// SetBlockDevicesForMachineByName overrides all current block devices on the
// named machine.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the machine uuid is not valid.
// - [machineerrors.MachineNotFound] when the machine is not found.
// - [machineerrors.MachineIsDead] when the machine is dead.
func (s *Service) SetBlockDevicesForMachineByName(
	ctx context.Context, machineName machine.Name,
	devices []coreblockdevice.BlockDevice,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	machineUUID, err := s.st.GetMachineUUIDByName(ctx, machineName)
	if err != nil {
		return err
	}

	existing, err := s.st.GetBlockDevicesForMachine(ctx, machineUUID)
	if err != nil {
		return err
	}

	added := make(
		map[blockdevice.BlockDeviceUUID]coreblockdevice.BlockDevice,
		len(devices),
	)
	for _, dev := range devices {
		devUUID, err := blockdevice.NewBlockDeviceUUID()
		if err != nil {
			return errors.Capture(err)
		}
		added[devUUID] = dev
	}

	removed := slices.Collect(maps.Keys(existing))
	return s.st.UpdateBlockDevicesForMachine(
		ctx, machineUUID, added, nil, removed)
}

// GetBlockDevicesForAllMachines retrieves block devices for all machines.
func (s *Service) GetBlockDevicesForAllMachines(
	ctx context.Context,
) (map[machine.Name][]coreblockdevice.BlockDevice, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	blockDevices, err := s.st.GetBlockDevicesForAllMachines(ctx)
	if err != nil {
		return nil, errors.Errorf("loading all block devices: %w", err)
	}
	return blockDevices, nil
}

// GetBlockDevice retrieves a block device by uuid.
//
// The following errors may be returned:
// - [coreerrors.NotValid] when the block device uuid is not valid.
// - [blockdeviceerrors.BlockDeviceNotFound] when the block device is not found.
func (s *Service) GetBlockDevice(
	ctx context.Context, uuid blockdevice.BlockDeviceUUID,
) (coreblockdevice.BlockDevice, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	err := uuid.Validate()
	if err != nil {
		return coreblockdevice.BlockDevice{}, errors.Errorf(
			"block device uuid not valid: %w", err,
		).Add(coreerrors.NotValid)
	}

	bd, err := s.st.GetBlockDevice(ctx, uuid)
	if err != nil {
		return coreblockdevice.BlockDevice{}, errors.Capture(err)
	}

	return bd, nil
}

// ListBlockDevices returns the BlockDevices for the specified UUIDs.
func (s *Service) ListBlockDevices(
	ctx context.Context,
	bdUUIDs ...blockdevice.BlockDeviceUUID,
) ([]blockdevice.BlockDeviceDetails, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	uuids := make([]string, len(bdUUIDs))
	for i, u := range bdUUIDs {
		uuids[i] = u.String()
	}

	blockDevices, err := s.st.ListBlockDevices(ctx, uuids...)
	if err != nil {
		return nil, errors.Errorf("listing block devices: %w", err)
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

// WatchBlockDevicesForMachine returns a new NotifyWatcher watching for changes
// to block devices associated with the specified machine.
//
// The following errors may be returned:
// - [errors.NotValid] when the machine uuid is not valid.
func (s *WatchableService) WatchBlockDevicesForMachine(
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
