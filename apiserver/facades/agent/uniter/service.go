// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/unitstate"
	"github.com/juju/juju/environs/config"
)

// ControllerConfigService provides the controller configuration for the model.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// ModelConfigService is used by the provisioner facade to get model config.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
	// Watch returns a watcher that returns keys for any changes to model
	// config.
	Watch() (watcher.StringsWatcher, error)
}

// ModelInfoService describes the service for interacting and reading the
// underlying model information.
type ModelInfoService interface {
	// GetModelInfo returns the readonly model information for the model in
	// question.
	GetModelInfo(context.Context) (model.ReadOnlyModel, error)
}

// CloudService provides access to clouds.
type CloudService interface {
	Cloud(ctx context.Context, name string) (*cloud.Cloud, error)
	WatchCloud(ctx context.Context, name string) (watcher.NotifyWatcher, error)
}

// CredentialService provides access to credentials.
type CredentialService interface {
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
	WatchCredential(ctx context.Context, key credential.Key) (watcher.NotifyWatcher, error)
}

// ApplicationService provides access to the application service.
type ApplicationService interface {
	GetApplicationLife(ctx context.Context, unitName string) (life.Value, error)
	GetUnitLife(ctx context.Context, unitName string) (life.Value, error)
	EnsureUnitDead(ctx context.Context, unitName string, leadershipRevoker leadership.Revoker) error
	DeleteUnit(ctx context.Context, unitName string) error
	DestroyUnit(ctx context.Context, unitName string) error
}

// UnitStateService describes the ability to retrieve and persist
// remote state for informing hook reconciliation.
type UnitStateService interface {
	// SetState persists the input agent state.
	SetState(context.Context, unitstate.AgentState) error
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// SpaceByName returns a space from state that matches the input name.
	// An error is returned that satisfied errors.NotFound if the space was not found
	// or an error static any problems fetching the given space.
	SpaceByName(ctx context.Context, name string) (*network.SpaceInfo, error)
	// GetAllSubnets returns all the subnets for the model.
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// RequireMachineReboot sets the machine referenced by its UUID as requiring a reboot.
	RequireMachineReboot(ctx context.Context, uuid string) error

	// ClearMachineReboot removes the reboot flag of the machine referenced by its UUID if a reboot has previously been required.
	ClearMachineReboot(ctx context.Context, uuid string) error

	// IsMachineRebootRequired checks if the machine referenced by its UUID requires a reboot.
	IsMachineRebootRequired(ctx context.Context, uuid string) (bool, error)

	// ShouldRebootOrShutdown determines whether a machine should reboot or shutdown
	ShouldRebootOrShutdown(ctx context.Context, uuid string) (coremachine.RebootAction, error)

	// GetMachineUUID returns the UUID of a machine identified by its name.
	// It returns an errors.MachineNotFound if the machine does not exist.
	GetMachineUUID(ctx context.Context, machineName coremachine.Name) (string, error)

	// LXDProfiles returns the names of the LXD profiles on the machine.
	LXDProfiles(ctx context.Context, mUUID string) ([]string, error)
}
