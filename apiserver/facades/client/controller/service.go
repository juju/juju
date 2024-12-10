// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"time"

	corecontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/domain/blockcommand"
	domainmodel "github.com/juju/juju/domain/model"
	"github.com/juju/juju/internal/proxy"
)

// ControllerConfigService is the interface that wraps the ControllerConfig method.
type ControllerConfigService interface {
	// ControllerConfig returns a controller.Config
	ControllerConfig(context.Context) (corecontroller.Config, error)
	// UpdateControllerConfig updates the controller config and has an optional
	// list of config keys to remove.
	UpdateControllerConfig(context.Context, corecontroller.Config, []string) error
}

// UpgradeService provides a subset of the upgrade domain service methods.
type UpgradeService interface {
	// IsUpgrading returns whether the controller is currently upgrading.
	IsUpgrading(context.Context) (bool, error)
}

// ControllerAccessService provides a subset of the Access domain for use.
type ControllerAccessService interface {
	// ReadUserAccessLevelForTarget returns the access level for the provided
	// subject (user) for controller.
	ReadUserAccessLevelForTarget(ctx context.Context, subject user.Name, target permission.ID) (permission.Access, error)
	// UpdatePermission updates the access level for a user for the controller.
	UpdatePermission(ctx context.Context, args access.UpdatePermissionArgs) error
	// LastModelLogin gets the time the specified user last connected to the
	// model.
	LastModelLogin(context.Context, user.Name, coremodel.UUID) (time.Time, error)
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// EnsureDeadMachine sets the provided machine's life status to Dead.
	// No error is returned if the provided machine doesn't exist, just nothing
	// gets updated.
	EnsureDeadMachine(ctx context.Context, machineName machine.Name) error
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name machine.Name) (string, error)
	// InstanceID returns the cloud specific instance id for this machine.
	InstanceID(ctx context.Context, mUUID string) (instance.Id, error)
	// InstanceIDAndName returns the cloud specific instance ID and display name for
	// this machine.
	InstanceIDAndName(ctx context.Context, machineUUID string) (instance.Id, string, error)
	// HardwareCharacteristics returns the hardware characteristics of the
	// specified machine.
	HardwareCharacteristics(ctx context.Context, machineUUID string) (*instance.HardwareCharacteristics, error)
}

// ModelService provides access to information about running Juju agents.
type ModelService interface {
	// Model returns the model associated with the provided uuid.
	Model(ctx context.Context, uuid coremodel.UUID) (coremodel.Model, error)
	// ControllerModel returns the model used for housing the Juju controller.
	ControllerModel(ctx context.Context) (coremodel.Model, error)
	// GetModelUsers will retrieve basic information about users with permissions on
	// the given model UUID.
	GetModelUsers(ctx context.Context, modelUUID coremodel.UUID) ([]coremodel.ModelUserInfo, error)
}

// ModelInfoService defines domain service methods for managing a model.
type ModelInfoService interface {
	// Status returns the current status of the model.
	// The following error types can be expected to be returned:
	// - [modelerrors.NotFound]: When the model does not exist.
	Status(context.Context) (domainmodel.StatusInfo, error)
}

type ApplicationService interface {
	GetApplicationLife(ctx context.Context, name string) (life.Value, error)
}

// ProxyService provides access to the proxy service.
type ProxyService interface {
	// GetProxyToApplication returns the proxy information for the application
	// with the given port.
	GetProxyToApplication(ctx context.Context, appName, remotePort string) (proxy.Proxier, error)
}

// BlockCommandService defines methods for interacting with block commands.
type BlockCommandService interface {
	// GetBlockSwitchedOn returns the optional block message if it is switched
	// on for the given type.
	GetBlockSwitchedOn(ctx context.Context, t blockcommand.BlockType) (string, error)

	// GetBlocks returns all the blocks that are currently in place.
	GetBlocks(ctx context.Context) ([]blockcommand.Block, error)

	// RemoveAllBlocks removes all the blocks that are currently in place.
	RemoveAllBlocks(ctx context.Context) error
}
