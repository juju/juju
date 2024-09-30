// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"context"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/environs/config"
)

// ControllerConfigService provides the controller configuration.
type ControllerConfigService interface {
	// ControllerConfig returns the config values for the controller.
	ControllerConfig(ctx context.Context) (controller.Config, error)
	// WatchControllerConfig returns a watcher that returns keys for any
	// changes to controller config.
	WatchControllerConfig() (watcher.StringsWatcher, error)
}

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
	// Watch returns a watcher that returns keys for any changes to model
	// config.
	Watch() (watcher.StringsWatcher, error)
}

// ModelInfoService describe the service for interacting and reading the underlying
// model information.
type ModelInfoService interface {
	// GetModelInfo returns the readonly model information for the model in
	// question.
	GetModelInfo(context.Context) (model.ReadOnlyModel, error)
}

// ApplicationService describes the service for accessing application scaling info.
type ApplicationService interface {
	SetApplicationScalingState(ctx context.Context, name string, scaleTarget int, scaling bool) error
	GetApplicationScalingState(ctx context.Context, name string) (service.ScalingState, error)
	GetApplicationScale(ctx context.Context, name string) (int, error)
	GetApplicationLife(ctx context.Context, name string) (life.Value, error)
	GetUnitLife(ctx context.Context, name string) (life.Value, error)
	DestroyUnit(ctx context.Context, name string) error
	RemoveUnit(ctx context.Context, unitName string, leadershipRevoker leadership.Revoker) error
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	// It returns a MachineNotFound if the machine does not exist.
	GetMachineUUID(ctx context.Context, name coremachine.Name) (string, error)
	// InstanceID returns the cloud specific instance id for this machine.
	InstanceID(ctx context.Context, mUUID string) (string, error)
}
