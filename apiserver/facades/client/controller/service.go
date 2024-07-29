// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"time"

	"github.com/juju/version/v2"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/environs/config"
)

// ControllerConfigService is the interface that wraps the ControllerConfig method.
type ControllerConfigService interface {
	// ControllerConfig returns a controller.Config
	ControllerConfig(context.Context) (controller.Config, error)
	// UpdateControllerConfig updates the controller config and has an optional
	// list of config keys to remove.
	UpdateControllerConfig(context.Context, controller.Config, []string) error
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
	ReadUserAccessLevelForTarget(ctx context.Context, subject string, target permission.ID) (permission.Access, error)
	// UpdatePermission updates the access level for a user for the controller.
	UpdatePermission(ctx context.Context, args access.UpdatePermissionArgs) error
	// LastModelLogin gets the time the specified user last connected to the
	// model.
	LastModelLogin(context.Context, string, model.UUID) (time.Time, error)
	// GetModelUsers gets all users for the model with the given ID.
	GetModelUsers(ctx context.Context, apiUser string, modelID model.UUID) ([]access.ModelUserInfo, error)
}

// ModelService provides access to currently deployed models.
type ModelService interface {
	// Model returns the model associated with the provided uuid.
	Model(ctx context.Context, uuid model.UUID) (model.Model, error)
	// ControllerModel returns the model used for housing the Juju controller.
	ControllerModel(ctx context.Context) (model.Model, error)
	// ListAllModels lists all models in the controller. If no models exist
	// then an empty slice is returned.
	ListAllModels(ctx context.Context) ([]model.Model, error)
}

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

// AgentService provides access to the Juju agent version for any model.
type AgentService interface {
	// GetModelAgentVersion returns the agent version for the provided model.
	GetModelAgentVersion(ctx context.Context, modelID model.UUID) (version.Number, error)
	// ControllerAgentVersion returns the agent version for the controller model.
	ControllerAgentVersion(ctx context.Context) (version.Number, error)
}
