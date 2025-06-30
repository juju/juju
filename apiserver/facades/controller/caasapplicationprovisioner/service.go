// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"context"
	"time"

	"github.com/juju/juju/controller"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/application"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/environs/config"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/resource"
)

type Services struct {
	ApplicationService      ApplicationService
	ControllerConfigService ControllerConfigService
	ControllerNodeService   ControllerNodeService
	ModelConfigService      ModelConfigService
	ModelInfoService        ModelInfoService
	StatusService           StatusService
	RemovalService          RemovalService
}

// ControllerConfigService provides the controller configuration.
type ControllerConfigService interface {
	// ControllerConfig returns the config values for the controller.
	ControllerConfig(ctx context.Context) (controller.Config, error)
	// WatchControllerConfig returns a watcher that returns keys for any
	// changes to controller config.
	WatchControllerConfig(context.Context) (watcher.StringsWatcher, error)
}

// ControllerNodeService represents a way to get controller api addresses.
type ControllerNodeService interface {
	// GetAllAPIAddressesForAgentsInPreferredOrder returns a string of api
	// addresses available for agents ordered to prefer local-cloud scoped
	// addresses and IPv4 over IPv6 for each machine.
	GetAllAPIAddressesForAgentsInPreferredOrder(ctx context.Context) ([]string, error)
	// WatchControllerAPIAddresses returns a watcher that observes changes to the
	// controller ip addresses.
	WatchControllerAPIAddresses(context.Context) (watcher.NotifyWatcher, error)
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
	GetModelInfo(context.Context) (model.ModelInfo, error)
}

// ApplicationService describes the service for accessing application scaling info.
type ApplicationService interface {
	GetApplicationScale(ctx context.Context, name string) (int, error)
	// GetCharmLocatorByApplicationName returns a CharmLocator by application name.
	// It returns an error if the charm can not be found by the name. This can also
	// be used as a cheap way to see if a charm exists without needing to load the
	// charm metadata.
	GetCharmLocatorByApplicationName(ctx context.Context, name string) (applicationcharm.CharmLocator, error)
	// GetCharmMetadataStorage returns the storage specification for the charm using
	// the charm name, source and revision.
	GetCharmMetadataStorage(ctx context.Context, locator applicationcharm.CharmLocator) (map[string]internalcharm.Storage, error)
	// GetCharmMetadataResources returns the specifications for the resources for the
	// charm using the charm name, source and revision.
	GetCharmMetadataResources(ctx context.Context, locator applicationcharm.CharmLocator) (map[string]resource.Meta, error)
	// IsCharmAvailable returns whether the charm is available for use. This
	// indicates if the charm has been uploaded to the controller.
	// This will return true if the charm is available, and false otherwise.
	IsCharmAvailable(ctx context.Context, locator applicationcharm.CharmLocator) (bool, error)

	// UpdateCAASUnit updates the specified CAAS unit
	UpdateCAASUnit(context.Context, unit.Name, service.UpdateCAASUnitParams) error

	// GetApplicationIDByName returns an application ID by application name. It
	// returns an error if the application can not be found by the name.
	//
	// Returns [applicationerrors.ApplicationNameNotValid] if the name is not valid,
	// and [applicationerrors.ApplicationNotFound] if the application is not found.
	GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error)

	// GetApplicationConstraints returns the application constraints for the
	// specified application ID.
	// Empty constraints are returned if no constraints exist for the given
	// application ID.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationConstraints(ctx context.Context, appID coreapplication.ID) (constraints.Value, error)

	// WatchApplication returns a NotifyWatcher for changes to the application.
	WatchApplication(ctx context.Context, name string) (watcher.NotifyWatcher, error)

	// GetDeviceConstraints returns the device constraints for an application.
	//
	// If the application is dead, [applicationerrors.ApplicationIsDead] is returned.
	// If the application is not found, [applicationerrors.ApplicationNotFound]
	// is returned.
	GetDeviceConstraints(ctx context.Context, name string) (map[string]devices.Constraints, error)

	// GetUnitUUID returns the UUID for the named unit.
	//
	// The following errors may be returned:
	// - [applicationerrors.UnitNotFound] if the unit doesn't exist.
	GetUnitUUID(ctx context.Context, unitName unit.Name) (unit.UUID, error)

	// WatchApplication returns a watcher that emits application uuids when
	// applications are added or removed.
	WatchApplications(ctx context.Context) (watcher.StringsWatcher, error)

	// GetApplicationTrustSetting returns the application trust setting.
	// The following errors may be returned:
	// - [applicationerrors.ApplicationNotFound] if the application doesn't exist
	GetApplicationTrustSetting(ctx context.Context, appName string) (bool, error)

	// GetCharmModifiedVersion looks up the charm modified version of the given
	// application.
	GetCharmModifiedVersion(ctx context.Context, id coreapplication.ID) (int, error)

	// GetUnitNamesForApplication returns a slice of the unit names for the given application
	// The following errors may be returned:
	// - [applicationerrors.ApplicationIsDead] if the application is dead
	// - [applicationerrors.ApplicationNotFound] if the application does not exist
	GetUnitNamesForApplication(ctx context.Context, appName string) ([]unit.Name, error)

	// GetApplicationCharmOrigin returns the charm origin for the specified
	// application name. If the application does not exist, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationCharmOrigin(ctx context.Context, name string) (application.CharmOrigin, error)

	// GetApplicationLifeByName looks up the life of the specified application, returning
	// an error satisfying [applicationerrors.ApplicationNotFoundError] if the
	// application is not found.
	GetApplicationLifeByName(ctx context.Context, appName string) (life.Value, error)
}

// RemovalService defines operations for removing juju entities.
type RemovalService interface {
	// RemoveUnit checks if a unit with the input name exists.
	// If it does, the unit is guaranteed after this call to be:
	// - No longer alive.
	// - Removed or scheduled to be removed with the input force qualification.
	// The input wait duration is the time that we will give for the normal
	// life-cycle advancement and removal to finish before forcefully removing the
	// unit. This duration is ignored if the force argument is false.
	// The UUID for the scheduled removal job is returned.
	RemoveUnit(ctx context.Context, unitUUID unit.UUID, force bool, wait time.Duration) (removal.UUID, error)

	// MarkUnitAsDead marks the unit as dead. It will not remove the unit as
	// that is a separate operation. This will advance the unit's life to dead
	// and will not allow it to be transitioned back to alive.
	// Returns an error if the unit does not exist.
	MarkUnitAsDead(ctx context.Context, unitUUID unit.UUID) error
}

type StatusService interface {
	// GetUnitWorkloadStatusesForApplication returns the workload statuses of all
	// units in the specified application, indexed by unit name, returning an error
	// satisfying [statuserrors.ApplicationNotFound] if the application doesn't
	// exist.
	GetUnitWorkloadStatusesForApplication(context.Context, coreapplication.ID) (map[unit.Name]status.StatusInfo, error)

	// SetApplicationStatus saves the given application status, overwriting any
	// current status data. If returns an error satisfying
	// [statuserrors.ApplicationNotFound] if the application doesn't exist.
	SetApplicationStatus(context.Context, string, status.StatusInfo) error
}
