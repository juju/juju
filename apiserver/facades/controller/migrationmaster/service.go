// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"context"

	"github.com/juju/version/v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/environs/config"
)

// UpgradeService provides a subset of the upgrade domain service methods.
type UpgradeService interface {
	// IsUpgrading returns whether the controller is currently upgrading.
	IsUpgrading(context.Context) (bool, error)
}

// ControllerConfigService provides access to the controller configuration.
type ControllerConfigService interface {
	// ControllerConfig returns the config values for the controller.
	ControllerConfig(context.Context) (controller.Config, error)
}

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

// ModelInfoService provides access to information about the model.
type ModelInfoService interface {
	// GetModelInfo returns the readonly model information for the model in
	// question.
	GetModelInfo(context.Context) (model.ModelInfo, error)
}

// ModelService provides access to currently deployed models.
type ModelService interface {
	// ControllerModel returns the model used for housing the Juju controller.
	ControllerModel(ctx context.Context) (model.Model, error)
}

// ApplicationService provides access to the application service.
type ApplicationService interface {
	// GetApplicationLife returns the life value of the application with the
	// given name.
	GetApplicationLife(ctx context.Context, name string) (life.Value, error)
}

type StatusService interface {
	// GetUnitWorkloadStatus returns the workload status of the specified unit.
	// Returns [applicationerrors.UnitNotFound] if the unit does not exist.
	GetUnitWorkloadStatus(context.Context, unit.Name) (*status.StatusInfo, error)
	// GetUnitAgentStatus returns the agent status of the specified unit.
	// Returns [applicationerrors.UnitNotFound] if the unit does not exist.
	GetUnitAgentStatus(context.Context, unit.Name) (*status.StatusInfo, error)
}

// ModelAgentService provides access to the Juju agent version for the model.
type ModelAgentService interface {
	// GetModelTargetAgentVersion returns the target agent version for the
	// entire model. The following errors can be returned:
	// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model does
	// not exist.
	GetModelTargetAgentVersion(context.Context) (version.Number, error)
}

// CredentialService provides access to credentials.
type CredentialService interface {
	// CloudCredential returns the cloud credential for the given tag.
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
}
