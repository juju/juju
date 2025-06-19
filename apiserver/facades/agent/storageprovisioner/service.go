// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"context"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/blockdevice"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/environs/config"
)

// ControllerConfigService provides access to the controller configuration.
type ControllerConfigService interface {
	// ControllerConfig returns the config values for the controller.
	ControllerConfig(context.Context) (controller.Config, error)
}

// ModelConfigService is the interface that the provisioner facade uses to get
// the model config.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// EnsureDeadMachine sets the provided machine's life status to Dead.
	// No error is returned if the provided machine doesn't exist, just nothing
	// gets updated.
	EnsureDeadMachine(ctx context.Context, machineName machine.Name) error
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, machineName machine.Name) (machine.UUID, error)
	// InstanceID returns the cloud specific instance id for this machine.
	InstanceID(ctx context.Context, machineUUID machine.UUID) (instance.Id, error)
	// InstanceIDAndName returns the cloud specific instance ID and display name for
	// this machine.
	InstanceIDAndName(ctx context.Context, machineUUID machine.UUID) (instance.Id, string, error)
	// HardwareCharacteristics returns the hardware characteristics of the
	// specified machine.
	HardwareCharacteristics(ctx context.Context, machineUUID machine.UUID) (*instance.HardwareCharacteristics, error)
	// GetMachineLife returns the lifecycle state of the machine with the
	// specified UUID.
	GetMachineLife(ctx context.Context, machineName machine.Name) (life.Value, error)
}

// BlockDeviceService instances can fetch and watch block devices on a machine.
type BlockDeviceService interface {
	// BlockDevices returns the block devices for a specified machine.
	BlockDevices(ctx context.Context, machineId string) ([]blockdevice.BlockDevice, error)
	// WatchBlockDevices returns a new NotifyWatcher watching for
	// changes to block devices associated with the specified machine.
	WatchBlockDevices(ctx context.Context, machineId string) (watcher.NotifyWatcher, error)
}

// StoragePoolGetter instances get a storage pool by name.
type StoragePoolGetter interface {
	// GetStoragePoolByName returns the storage pool with the specified name.
	// The following errors can be expected:
	// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
	GetStoragePoolByName(ctx context.Context, name string) (domainstorage.StoragePool, error)
}

// ApplicationService is an interface for the application domain service.
type ApplicationService interface {
	// GetUnitLife returns the life status of a unit identified by its name.
	GetUnitLife(ctx context.Context, unitName unit.Name) (life.Value, error)
	// WatchApplications returns a watcher that emits application uuids
	// when applications are added or removed.
	WatchApplications(ctx context.Context) (watcher.StringsWatcher, error)
	// GetApplicationLifeByName looks up the life of the specified application, returning
	// an error satisfying [applicationerrors.ApplicationNotFoundError] if the
	// application is not found.
	GetApplicationLifeByName(ctx context.Context, appName string) (life.Value, error)
}
