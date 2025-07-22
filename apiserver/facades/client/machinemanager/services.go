// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"context"
	"time"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/instance"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/blockcommand"
	domainmachine "github.com/juju/juju/domain/machine"
	machineservice "github.com/juju/juju/domain/machine/service"
	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
)

type Services struct {
	AgentBinaryService      AgentBinaryService
	AgentPasswordService    AgentPasswordService
	ApplicationService      ApplicationService
	BlockCommandService     BlockCommandService
	CloudService            CloudService
	ControllerConfigService ControllerConfigService
	ControllerNodeService   ControllerNodeService
	KeyUpdaterService       KeyUpdaterService
	MachineService          MachineService
	StatusService           StatusService
	ModelConfigService      ModelConfigService
	NetworkService          NetworkService
	RemovalService          RemovalService
}

// ControllerConfigService defines a method for getting the controller config.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// ControllerNodeService represents a way to get controller api addresses.
type ControllerNodeService interface {
	// GetAllAPIAddressesForAgents returns a string of api
	// addresses available for agents ordered to prefer local-cloud scoped
	// addresses and IPv4 over IPv6 for each machine.
	GetAllAPIAddressesForAgents(ctx context.Context) ([]string, error)
}

// KeyUpdaterService is responsible for returning information about the ssh keys
// for a machine within a model.
type KeyUpdaterService interface {
	// GetAuthorisedKeysForMachine returns the authorized keys that should be
	// allowed to access the given machine.
	GetAuthorisedKeysForMachine(context.Context, coremachine.Name) ([]string, error)
}

// ModelConfigService is responsible for providing an accessor to the models
// config.
type ModelConfigService interface {
	// ModelConfig provides the currently set model config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

// Authorizer checks to see if an operation can be performed.
type Authorizer interface {
	// CanRead checks to see if a read is possible. Returns an error if a read
	// is not possible.
	CanRead(context.Context) error

	// CanWrite checks to see if a write is possible. Returns an error if a
	// write is not possible.
	CanWrite(context.Context) error

	// AuthClient returns true if the entity is an external user.
	AuthClient() bool
}

// MachineService is the interface that is used to interact with the machines.
type MachineService interface {
	// AddMachine creates the net node and machines if required, depending
	// on the placement.
	// It returns the net node UUID for the machine and a list of child
	// machine names that were created as part of the placement.
	AddMachine(context.Context, domainmachine.AddMachineArgs) (machineservice.AddMachineResults, error)

	// DeleteMachine deletes a machine with the given name.
	DeleteMachine(context.Context, coremachine.Name) error

	// AllMachineNames returns the names of all machines in the model.
	AllMachineNames(context.Context) ([]coremachine.Name, error)

	// GetInstanceTypesFetcher returns the instance types fetcher.
	GetInstanceTypesFetcher(context.Context) (environs.InstanceTypesFetcher, error)

	// ShouldKeepInstance reports whether a machine, when removed from Juju, should cause
	// the corresponding cloud instance to be stopped.
	// It returns a NotFound if the given machine doesn't exist.
	ShouldKeepInstance(context.Context, coremachine.Name) (bool, error)

	// SetKeepInstance sets whether the machine cloud instance will be retained
	// when the machine is removed from Juju. This is only relevant if an instance
	// exists.
	// It returns a NotFound if the given machine doesn't exist.
	SetKeepInstance(ctx context.Context, machineName coremachine.Name, keep bool) error

	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(context.Context, coremachine.Name) (coremachine.UUID, error)

	// GetHardwareCharacteristics returns the hardware characteristics of the
	// specified machine.
	GetHardwareCharacteristics(context.Context, coremachine.UUID) (*instance.HardwareCharacteristics, error)

	// GetMachineContainers returns the names of the machines which have as parent
	// the specified machine.
	GetMachineContainers(context.Context, coremachine.Name) ([]coremachine.Name, error)

	// GetMachineBase returns the base for the given machine.
	GetMachineBase(context.Context, coremachine.Name) (base.Base, error)
}

// StatusService defines the methods that the facade assumes from the Status
// service.
type StatusService interface {
	// GetInstanceStatus returns the cloud specific instance status for this
	// machine.
	GetInstanceStatus(context.Context, coremachine.Name) (status.StatusInfo, error)

	// SetInstanceStatus sets the cloud specific instance status for this machine.
	SetInstanceStatus(context.Context, coremachine.Name, status.StatusInfo) error
}

// ApplicationService is the interface that is used to interact with
// applications and units.
type ApplicationService interface {
	// GetUnitNamesOnMachine returns a slice of the unit names on the given machine.
	// The following errors may be returned:
	// - [applicationerrors.MachineNotFound] if the machine does not exist
	GetUnitNamesOnMachine(context.Context, coremachine.Name) ([]coreunit.Name, error)
}

// CharmhubClient represents a way for querying the charmhub api for information
// about the application charm.
type CharmhubClient interface {
	Refresh(ctx context.Context, config charmhub.RefreshConfig) ([]transport.RefreshResponse, error)
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
}

// BlockCommandService defines methods for interacting with block commands.
type BlockCommandService interface {
	// GetBlockSwitchedOn returns the optional block message if it is switched
	// on for the given type.
	GetBlockSwitchedOn(ctx context.Context, t blockcommand.BlockType) (string, error)

	// GetBlocks returns all the blocks that are currently in place.
	GetBlocks(ctx context.Context) ([]blockcommand.Block, error)
}

// CloudService provides access to clouds.
type CloudService interface {
	// Cloud returns the named cloud.
	Cloud(ctx context.Context, name string) (*cloud.Cloud, error)
}

// RemovalService provides access to the removal service.
type RemovalService interface {
	// RemoveMachine checks if a machine with the input name exists.
	// If it does, the machine is guaranteed after this call to be:
	// - No longer alive.
	// - Removed or scheduled to be removed with the input force qualification.
	// The input wait duration is the time that we will give for the normal
	// life-cycle advancement and removal to finish before forcefully removing the
	// machine. This duration is ignored if the force argument is false.
	// The UUID for the scheduled removal job is returned.
	RemoveMachine(
		ctx context.Context,
		machineUUID coremachine.UUID,
		force bool,
		wait time.Duration,
	) (removal.UUID, error)
}
