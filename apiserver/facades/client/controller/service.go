// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"time"

	"github.com/juju/juju/cloud"
	corecontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/domain/blockcommand"
	"github.com/juju/juju/domain/relation"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
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

// ControllerNodeService represents a way to get controller api addresses.
type ControllerNodeService interface {
	// GetAllAPIAddressesForAgents returns a string of api
	// addresses available for agents ordered to prefer local-cloud scoped
	// addresses and IPv4 over IPv6 for each machine.
	GetAllAPIAddressesForAgents(ctx context.Context) ([]string, error)
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

// ModelService provides access to information about running Juju agents.
type ModelService interface {
	// Model returns the model associated with the provided uuid.
	Model(ctx context.Context, uuid coremodel.UUID) (coremodel.Model, error)
	// CheckModelExistsByName checks if a model exists within the controller.
	// True or false is returned indicating of the model exists.
	CheckModelExists(ctx context.Context, modelUUID coremodel.UUID) (bool, error)
	// ControllerModel returns the model used for housing the Juju controller.
	ControllerModel(ctx context.Context) (coremodel.Model, error)
	// GetModelUsers will retrieve basic information about users with permissions on
	// the given model UUID.
	GetModelUsers(ctx context.Context, modelUUID coremodel.UUID) ([]coremodel.ModelUserInfo, error)
	// ListAllModels returns a slice of all models in the controller. If no models
	// exist an empty slice is returned.
	ListAllModels(ctx context.Context) ([]coremodel.Model, error)
	// ListModelUUIDs returns a list of all model UUIDs in the controller.
	ListModelUUIDs(context.Context) ([]coremodel.UUID, error)
}

// ModelInfoService defines domain service methods for managing a model.
type ModelInfoService interface {
	// IsControllerModel returns true if the model is the controller model.
	// The following errors may be returned:
	// - [github.com/juju/juju/domain/model/errors.NotFound] When the model does not exist.
	IsControllerModel(context.Context) (bool, error)
	// HasValidCredential returns true if the model has a valid credential.
	// The following errors may be returned:
	// - [modelerrors.NotFound] when the model no longer exists.
	HasValidCredential(context.Context) (bool, error)
}

// ApplicationService provides access to the application service.
type ApplicationService interface {
	// CheckAllApplicationsAndUnitsAreAlive checks that all applications and units
	// in the model are alive, returning an error if any are not.
	CheckAllApplicationsAndUnitsAreAlive(ctx context.Context) error

	// GetUnitNamesForApplication returns a slice of the unit names for the given application
	GetUnitNamesForApplication(ctx context.Context, appName string) ([]unit.Name, error)
}

// RelationService provides access to the relation service.
type RelationService interface {
	// GetAllRelationDetails return RelationDetailResults for all relations
	// for the current model.
	GetAllRelationDetails(ctx context.Context) ([]relation.RelationDetailsResult, error)

	// RelationUnitInScopeByID returns a boolean to indicate whether the given
	// unit is in scopen of a given relation
	RelationUnitInScopeByID(ctx context.Context, relationID int, unitName unit.Name) (bool,
		error)
}

type StatusService interface {
	// CheckUnitStatusesReadyForMigration returns true is the statuses of all units
	// in the model indicate they can be migrated.
	CheckUnitStatusesReadyForMigration(context.Context) error

	// CheckMachineStatusesReadyForMigration returns an error if the statuses of any
	// machines in the model indicate they cannot be migrated.
	CheckMachineStatusesReadyForMigration(context.Context) error

	// GetApplicationAndUnitModelStatuses returns the application name and unit
	// count for each model for the model status request.
	GetApplicationAndUnitModelStatuses(ctx context.Context) (map[string]int, error)

	// GetModelStatusInfo returns information about the current model for the
	// purpose of reporting its status.
	// The following error types can be expected to be returned:
	// - [github.com/juju/juju/domain/model/errors.NotFound]: When the model does not exist.
	GetModelStatusInfo(context.Context) (domainstatus.ModelStatusInfo, error)

	// GetAllMachineStatuses returns all the machine statuses for the model, indexed
	// by machine name.
	GetAllMachineStatuses(context.Context) (map[machine.Name]status.StatusInfo, error)
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

// CredentialService provides access to credentials.
type CredentialService interface {
	// CloudCredential returns the cloud credential for the given key.
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
}

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(ctx context.Context) (*config.Config, error)
}

// ModelProviderService providers access to the model provider service.
type ModelProviderService interface {
	// GetCloudSpec returns the cloud spec for the model.
	GetCloudSpec(ctx context.Context) (cloudspec.CloudSpec, error)
}

// ModelAgentService provides access to the Juju agent version for the model.
type ModelAgentService interface {
	// GetMachinesNotAtTargetAgentVersion reports all of the machines in the model that
	// are currently not at the desired target version. This also returns machines
	// that have no reported agent version set. If all units are up to the
	// target version or no units exist in the model a zero length slice is
	// returned.
	GetMachinesNotAtTargetAgentVersion(context.Context) ([]machine.Name, error)

	// GetModelTargetAgentVersion returns the target agent version for the
	// entire model. The following errors can be returned:
	// - [github.com/juju/juju/domain/modelagent/errors.NotFound] When the model
	// does not exist.
	GetModelTargetAgentVersion(context.Context) (semversion.Number, error)

	// GetUnitsNotAtTargetAgentVersion reports all of the units in the model that
	// are currently not at the desired target agent version. This also returns
	// units that have no reported agent version set. If all units are up to the
	// target version or no units exist in the model a zero length slice is
	// returned.
	GetUnitsNotAtTargetAgentVersion(context.Context) ([]unit.Name, error)
}
