// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	corerelation "github.com/juju/juju/core/relation"
	coreremoteapplication "github.com/juju/juju/core/remoteapplication"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/application"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
	crossmodelrelationservice "github.com/juju/juju/domain/crossmodelrelation/service"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/domain/removal"
	"github.com/juju/juju/domain/resolve"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/environs/config"
	internalcharm "github.com/juju/juju/internal/charm"
)

// Services represents all the services that the application facade requires.
type Services struct {
	ExternalControllerService ExternalControllerService
	ApplicationService        ApplicationService
	ResolveService            ResolveService
	MachineService            MachineService
	ModelConfigService        ModelConfigService
	NetworkService            NetworkService
	PortService               PortService
	RelationService           RelationService
	RemovalService            RemovalService
	ResourceService           ResourceService
	StatusService             StatusService
	StorageService            StorageService
	CrossModelRelationService CrossModelRelationService
}

// Validate checks that all the services are set.
func (s Services) Validate() error {
	if s.ExternalControllerService == nil {
		return errors.NotValidf("empty ExternalControllerService")
	}
	if s.NetworkService == nil {
		return errors.NotValidf("empty NetworkService")
	}
	if s.ModelConfigService == nil {
		return errors.NotValidf("empty ModelConfigService")
	}
	if s.MachineService == nil {
		return errors.NotValidf("empty MachineService")
	}
	if s.ApplicationService == nil {
		return errors.NotValidf("empty ApplicationService")
	}
	if s.ResolveService == nil {
		return errors.NotValidf("empty ResolveService")
	}
	if s.PortService == nil {
		return errors.NotValidf("empty PortService")
	}
	if s.ResourceService == nil {
		return errors.NotValidf("empty ResourceService")
	}
	if s.RelationService == nil {
		return errors.NotValidf("empty RelationService")
	}
	if s.RemovalService == nil {
		return errors.NotValidf("empty RemovalService")
	}
	if s.StatusService == nil {
		return errors.NotValidf("empty StatusService")
	}
	if s.StorageService == nil {
		return errors.NotValidf("empty StorageService")
	}
	if s.CrossModelRelationService == nil {
		return errors.NotValidf("empty CrossModelRelationService")
	}
	return nil
}

// ExternalControllerService provides a subset of the external controller domain
// service methods.
type ExternalControllerService interface {
	// UpdateExternalController persists the input controller
	// record.
	UpdateExternalController(ctx context.Context, ec crossmodel.ControllerInfo) error
}

// CrossModelRelationService provides access to cross-model relations.
type CrossModelRelationService interface {
	// AddRemoteApplicationOfferer adds a new synthetic application representing
	// an offer from an external model, to this, the consuming model.
	AddRemoteApplicationOfferer(
		ctx context.Context,
		applicationName string,
		args crossmodelrelationservice.AddRemoteApplicationOffererArgs,
	) error

	// GetRemoteApplicationOffererByApplicationName returns the UUID of the remote
	// application offerer for the given application name.
	GetRemoteApplicationOffererByApplicationName(context.Context, string) (coreremoteapplication.UUID, error)
}

// CredentialService provides access to credentials.
type CredentialService interface {
	// CloudCredential returns the cloud credential for the given tag.
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
	// WatchCredential returns a watcher that observes changes to the specified
	// credential.
	WatchCredential(ctx context.Context, key credential.Key) (watcher.NotifyWatcher, error)
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)

	// GetUnitPublicAddress returns the public address for the specified unit.
	// For k8s provider, it will return the first public address of the cloud
	// service if any, the first public address of the cloud container otherwise.
	// For machines provider, it will return the first public address of the
	// machine.
	GetUnitPublicAddress(ctx context.Context, unitName unit.Name) (network.SpaceAddress, error)
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name machine.Name) (machine.UUID, error)

	// GetHardwareCharacteristics returns the hardware characteristics of the
	// specified machine.
	GetHardwareCharacteristics(ctx context.Context, machineUUID machine.UUID) (instance.HardwareCharacteristics, error)

	// GetMachineBase returns the base for the given machine.
	GetMachineBase(ctx context.Context, mName machine.Name) (base.Base, error)
}

// ApplicationService instances save an application to dqlite state.
type ApplicationService interface {
	// CreateIAASApplication creates the specified IAAS application and
	// subsequent units if supplied.
	CreateIAASApplication(context.Context, string, internalcharm.Charm, corecharm.Origin, applicationservice.AddApplicationArgs, ...applicationservice.AddIAASUnitArg) (coreapplication.UUID, error)

	// CreateCAASApplication creates the specified CAAS application and
	// subsequent units if supplied.
	CreateCAASApplication(context.Context, string, internalcharm.Charm, corecharm.Origin, applicationservice.AddApplicationArgs, ...applicationservice.AddUnitArg) (coreapplication.UUID, error)

	// AddIAASUnits adds IAAS units to the application.
	AddIAASUnits(ctx context.Context, name string, units ...applicationservice.AddIAASUnitArg) ([]unit.Name, []machine.Name, error)

	// AddCAASUnits adds CAAS units to the application.
	AddCAASUnits(ctx context.Context, name string, units ...applicationservice.AddUnitArg) ([]unit.Name, error)

	// SetApplicationCharm sets a new charm for the application, validating that
	// aspects such as storage are still viable with the new charm.
	SetApplicationCharm(ctx context.Context, appName string, locator applicationcharm.CharmLocator, params application.SetCharmParams) error

	// SetApplicationScale sets the application's desired scale value.
	// This is used on CAAS models.
	SetApplicationScale(ctx context.Context, name string, scale int) error

	// ChangeApplicationScale alters the existing scale by the provided change
	// amount, returning the new amount. This is used on CAAS models.
	ChangeApplicationScale(ctx context.Context, name string, scaleChange int) (int, error)

	// GetApplicationLife looks up the life of the specified application.
	GetApplicationLife(context.Context, coreapplication.UUID) (life.Value, error)

	// GetApplicationStorageInfo returns the storage directives set for an application,
	// keyed to the storage name. If the application does not have any storage
	// directives set then an empty result is returned.
	//
	// If the application does not exist, then a [applicationerrors.ApplicationNotFound]
	// error is returned.
	GetApplicationStorageInfo(ctx context.Context, uuid coreapplication.UUID) (application.ApplicationStorage, error)

	// GetUnitLife looks up the life of the specified unit.
	GetUnitLife(context.Context, unit.Name) (life.Value, error)

	// GetUnitUUID returns the UUID for the named unit.
	GetUnitUUID(context.Context, unit.Name) (unit.UUID, error)

	// GetUnitMachineName gets the name of the unit's machine.
	// The following errors may be returned:
	//   - [applicationerrors.UnitMachineNotAssigned] if the unit does not have a
	//     machine assigned.
	//   - [applicationerrors.UnitNotFound] if the unit cannot be found.
	//   - [applicationerrors.UnitIsDead] if the unit is dead.
	GetUnitMachineName(ctx context.Context, unitName unit.Name) (machine.Name, error)

	// GetUnitNamesForApplication returns a slice of the unit names for the given application
	// The following errors may be returned:
	// - [applicationerrors.ApplicationIsDead] if the application is dead
	// - [applicationerrors.ApplicationNotFound] if the application does not exist
	GetUnitNamesForApplication(context.Context, string) ([]unit.Name, error)

	// GetUnitWorkloadVersion returns the workload version for the given unit.
	GetUnitWorkloadVersion(ctx context.Context, unitName unit.Name) (string, error)

	// GetUnitK8sPodInfo returns information about the k8s pod for the given unit.
	// The following errors may be returned:
	// - [applicationerrors.UnitNotFound] if the unit does not exist
	GetUnitK8sPodInfo(context.Context, unit.Name) (application.K8sPodInfo, error)

	// GetSupportedFeatures returns the set of features that the model makes
	// available for charms to use.
	GetSupportedFeatures(context.Context) (assumes.FeatureSet, error)

	// GetCharmLocatorByApplicationName returns a CharmLocator by application
	// name. It returns an error if the charm can not be found by the name. This
	// can also be used as a cheap way to see if a charm exists without needing
	// to load the charm metadata.
	GetCharmLocatorByApplicationName(ctx context.Context, name string) (applicationcharm.CharmLocator, error)

	// GetCharm returns the charm by name, source and revision. Calling this
	// method will return all the data associated with the charm. It is not
	// expected to call this method for all calls, instead use the move focused
	// and specific methods. That's because this method is very expensive to
	// call. This is implemented for the cases where all the charm data is
	// needed; model migration, charm export, etc.
	GetCharm(ctx context.Context, locator applicationcharm.CharmLocator) (internalcharm.Charm, applicationcharm.CharmLocator, bool, error)

	// GetCharmMetadata returns the metadata for the charm using the charm name,
	// source and revision.
	GetCharmMetadata(ctx context.Context, locator applicationcharm.CharmLocator) (internalcharm.Meta, error)

	// GetCharmMetadataName returns the name for the charm using the
	// charm name, source and revision.
	GetCharmMetadataName(ctx context.Context, locator applicationcharm.CharmLocator) (string, error)

	// GetCharmDownloadInfo returns the download info for the charm using the
	// charm name, source and revision.
	GetCharmDownloadInfo(ctx context.Context, locator applicationcharm.CharmLocator) (*applicationcharm.DownloadInfo, error)

	// IsCharmAvailable returns whether the charm is available for use. This
	// indicates if the charm has been uploaded to the controller.
	// This will return true if the charm is available, and false otherwise.
	IsCharmAvailable(ctx context.Context, locator applicationcharm.CharmLocator) (bool, error)

	// GetApplicationUUIDByName returns an application UUID by application name. It
	// returns an error if the application can not be found by the name.
	//
	// Returns [applicationerrors.ApplicationNameNotValid] if the name is not
	// valid, and [applicationerrors.ApplicationNotFound] if the application is
	// not found.
	GetApplicationUUIDByName(ctx context.Context, name string) (coreapplication.UUID, error)

	// GetApplicationConstraints returns the application constraints for the
	// specified application UUID.
	// Empty constraints are returned if no constraints exist for the given
	// application UUID.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationConstraints(ctx context.Context, appID coreapplication.UUID) (constraints.Value, error)

	// GetApplicationCharmOrigin returns the charm origin for the specified
	// application name. If the application does not exist, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationCharmOrigin(ctx context.Context, name string) (corecharm.Origin, error)

	// GetApplicationAndCharmConfig returns the application and charm config for the
	// specified application UUID.
	GetApplicationAndCharmConfig(context.Context, coreapplication.UUID) (applicationservice.ApplicationConfig, error)

	// SetApplicationConstraints sets the application constraints for the
	// specified application UUID.
	// This method overwrites the full constraints on every call.
	// If invalid constraints are provided (e.g. invalid container type or
	// non-existing space), a [applicationerrors.InvalidApplicationConstraints]
	// error is returned.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	SetApplicationConstraints(context.Context, coreapplication.UUID, constraints.Value) error

	// UnsetApplicationConfigKeys removes the specified keys from the application
	// config. If the key does not exist, it is ignored.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	UnsetApplicationConfigKeys(context.Context, coreapplication.UUID, []string) error

	// UpdateApplicationConfig updates the application config with the specified
	// values. If the key does not exist, it is created. If the key already exists,
	// it is updated, if there is no value it is removed. With the caveat that
	// application trust will be set to false.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	// If the charm config is not valid, an error satisfying
	// [applicationerrors.InvalidApplicationConfig] is returned.
	UpdateApplicationConfig(context.Context, coreapplication.UUID, map[string]string) error

	// IsApplicationExposed returns whether the provided application is exposed or not.
	//
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	IsApplicationExposed(ctx context.Context, appName string) (bool, error)

	// IsSubordinateApplication returns true if the application is a subordinate
	// application.
	// The following errors may be returned:
	// - [appliationerrors.ApplicationNotFound] if the application does not exist
	IsSubordinateApplication(context.Context, coreapplication.UUID) (bool, error)

	// IsSubordinateApplicationByName returns true if the application is a
	// subordinate application.
	// The following errors may be returned:
	// - [appliationerrors.ApplicationNotFound] if the application does not exist
	IsSubordinateApplicationByName(context.Context, string) (bool, error)

	// GetApplicationEndpointBindings returns the mapping for each endpoint name
	// and the space ID it is bound to (or empty if unspecified). When no
	// bindings are stored for the application, defaults are returned.
	//
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationEndpointBindings(context.Context, string) (map[string]network.SpaceUUID, error)

	// GetApplicationEndpointNames returns the names of the endpoints for the given
	// application.
	// The following errors may be returned:
	//   - [applicationerrors.ApplicationNotFound] is returned if the application
	//     doesn't exist.
	GetApplicationEndpointNames(context.Context, coreapplication.UUID) ([]string, error)

	// MergeApplicationEndpointBindings merge the provided bindings into the bindings
	// for the specified application.
	// The following errors may be returned:
	// - [applicationerrors.ApplicationNotFound] if the application does not exist
	MergeApplicationEndpointBindings(ctx context.Context, appID coreapplication.UUID, bindings map[string]network.SpaceName, force bool) error

	// GetExposedEndpoints returns map where keys are endpoint names (or the ""
	// value which represents all endpoints) and values are ExposedEndpoint
	// instances that specify which sources (spaces or CIDRs) can access the
	// opened ports for each endpoint once the application is exposed.
	//
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetExposedEndpoints(ctx context.Context, appName string) (map[string]application.ExposedEndpoint, error)

	// UnsetExposeSettings removes the expose settings for the provided list of
	// endpoint names. If the resulting exposed endpoints map for the
	// application becomes empty after the settings are removed, the application
	// will be automatically unexposed.
	//
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	UnsetExposeSettings(ctx context.Context, appName string, exposedEndpoints set.Strings) error

	// MergeExposeSettings marks the application as exposed and merges the
	// provided ExposedEndpoint details into the current set of expose settings.
	// The merge operation will overwrite expose settings for each existing
	// endpoint name.
	//
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	MergeExposeSettings(ctx context.Context, appName string, exposedEndpoints map[string]application.ExposedEndpoint) error

	// ResolveApplicationConstraints resolves given application constraints, taking
	// into account the model constraints.
	ResolveApplicationConstraints(ctx context.Context, appCons constraints.Value) (constraints.Value, error)
}

type ResolveService interface {
	// ResolveUnit marks the unit as resolved. If the unit is not found, an error
	// satisfying [resolveerrors.UnitNotFound] is returned.
	ResolveUnit(context.Context, unit.Name, resolve.ResolveMode) error

	// ResolveAllUnits marks all units as resolved.
	ResolveAllUnits(context.Context, resolve.ResolveMode) error
}

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

// PortService defines the methods that the facade assumes from the Port
// service.
type PortService interface {
	// GetUnitOpenedPorts returns the opened ports for a given unit uuid,
	// grouped by endpoint.
	GetUnitOpenedPorts(ctx context.Context, unitUUID unit.UUID) (network.GroupedPortRanges, error)
}

// ResourceService defines the methods that the facade assumes from the Resource
// service.
type ResourceService interface {
	DeleteResourcesAddedBeforeApplication(ctx context.Context, resources []coreresource.UUID) error
}

// StorageService instances get a storage pool by name.
type StorageService interface {
	// GetStoragePoolUUID returns the UUID of the storage pool for the specified name.
	GetStoragePoolUUID(context.Context, string) (domainstorage.StoragePoolUUID, error)
}

// StatusService provides access to the status service.
type StatusService interface {
	// SetRemoteRelationStatus sets the status of the relation to the status
	// provided.
	SetRemoteRelationStatus(ctx context.Context, relationUUID corerelation.UUID, statusInfo status.StatusInfo) error
}

type StoragePoolService interface {
	// GetStoragePoolByUUID returns the storage pool with the UUID.
	// The following errors can be expected:
	// - [storageerrors.PoolNotFoundError] if a pool with the specified name does not exist.
	GetStoragePoolByUUID(ctx context.Context, poolUUID domainstorage.StoragePoolUUID) (domainstorage.StoragePool, error)
}

// BlockChecker defines the block-checking functionality required by
// the application facade. This is implemented by
// apiserver/common.BlockChecker.
type BlockChecker interface {
	ChangeAllowed(context.Context) error
	RemoveAllowed(context.Context) error
}

// Leadership describes the capability to read the current state of leadership.
type Leadership interface {

	// Leaders returns all application leaders in the current model.
	// TODO (manadart 2019-02-27): The return in this signature includes error
	// in order to support state.ApplicationLeaders for legacy leases.
	// When legacy leases are removed, so can the error return.
	Leaders() (map[string]string, error)
}

// RelationService defines operations for managing relations between application
// endpoints.
type RelationService interface {
	// AddRelation takes two endpoint identifiers of the form
	// <application>[:<endpoint>]. The identifiers will be used to infer two
	// endpoint between applications on the model. A new relation will be
	// created between these endpoints and the details of the endpoint returned.
	AddRelation(ctx context.Context, ep1, ep2 string, cidrs ...string) (relation.Endpoint, relation.Endpoint, error)

	// ApplicationRelationsInfo returns all EndpointRelationData for an
	// application.
	ApplicationRelationsInfo(ctx context.Context, applicationID coreapplication.UUID) ([]relation.EndpointRelationData, error)

	// GetRelationUUIDForRemoval returns the relation UUID, of the relation
	// represented in GetRelationUUIDForRemovalArgs, with the understanding
	// this relation will be removed by an end user. Peer relations cannot be
	// removed by an end user.
	GetRelationUUIDForRemoval(
		ctx context.Context,
		args relation.GetRelationUUIDForRemovalArgs,
	) (corerelation.UUID, error)

	// GetRelationUUIDByID returns the relation UUID based on the relation ID.
	GetRelationUUIDByID(ctx context.Context, relationID int) (corerelation.UUID, error)

	// GetRelationDetails returns RelationDetails for the given relation UUID.
	GetRelationDetails(
		ctx context.Context,
		relationUUID corerelation.UUID,
	) (relation.RelationDetails, error)

	// SetRemoteRelationSuspendedState sets the suspended state of the specified
	// remote relation in the local model. The relation must be a cross-model
	// relation.
	SetRemoteRelationSuspendedState(ctx context.Context, relationUUID corerelation.UUID, suspended bool, reason string) error
}

// RemovalService defines operations for removing juju entities.
type RemovalService interface {
	// RemoveApplication checks if a application with the input application UUID
	// exists. If it does, the application is guaranteed after this call to:
	// - Not be alive.
	// - Be removed or scheduled to be removed with the input force
	//   qualification.
	// - Have no units that are alive.
	// - Have all units scheduled for removal.
	// The input wait duration is the time that we will give for the normal
	// life-cycle advancement and removal to finish before forcefully removing
	// the application. This duration is ignored if the force argument is false.
	// If destroyStorage is true, the application units' storage instances will
	// be guaranteed to be not alive and to be scheduled for removal.
	// The UUID for the scheduled removal job is returned.
	RemoveApplication(
		ctx context.Context,
		appUUID coreapplication.UUID,
		destroyStorage bool,
		force bool,
		wait time.Duration,
	) (removal.UUID, error)

	// RemoveUnit checks if a unit with the input name exists.
	// If it does, the unit is guaranteed after this call to be:
	// - Not alive.
	// - Removed or scheduled to be removed with the input force qualification.
	// The input wait duration is the time that we will give for the normal
	// life-cycle advancement and removal to finish before forcefully removing
	// the unit. This duration is ignored if the force argument is false.
	// If the unit is the last one on the machine, the machine will be
	// guaranteedto not be alive and be scheduled for removal.
	// If destroyStorage is true, the unit's storage instances will be
	// guaranteed to not be alive and be scheduled for removal.
	// The UUID for the scheduled removal job is returned.
	RemoveUnit(
		ctx context.Context,
		unitUUID unit.UUID,
		destroyStorage bool,
		force bool,
		wait time.Duration,
	) (removal.UUID, error)

	// RemoveRelation checks if a relation with the input UUID exists.
	// If it does, the relation is guaranteed after this call to be:
	// - No longer alive.
	// - Removed or scheduled to be removed with the input force qualification.
	// The input wait duration is the time that we will give for the normal
	// life-cycle advancement and removal to finish before forcefully removing the
	// relation. This duration is ignored if the force argument is false.
	// The UUID for the scheduled removal job is returned.
	// [relationerrors.RelationNotFound] is returned if no such relation exists.
	RemoveRelation(
		ctx context.Context,
		relUUID corerelation.UUID,
		force bool,
		wait time.Duration,
	) (removal.UUID, error)

	// RemoveRemoteApplicationOfferer checks if a remote application with the input
	// UUID exists. If it does, the remote application is guaranteed after this
	// call to be:
	// - No longer alive.
	// - Removed or scheduled to be removed with the input force qualification.
	// The input wait duration is the time that we will give for the normal
	// life-cycle advancement and removal to finish before forcefully removing the
	// remote application. This duration is ignored if the force argument is false.
	// The UUID for the scheduled removal job is returned.
	RemoveRemoteApplicationOfferer(
		ctx context.Context,
		remoteAppOffererUUID coreremoteapplication.UUID,
		force bool,
		wait time.Duration,
	) (removal.UUID, error)

	// RemoveRelation checks if a relation with the input UUID exists.
	// If it does, the relation is guaranteed after this call to be:
	// - No longer alive.
	// - Removed or scheduled to be removed with the input force qualification.
	// The input wait duration is the time that we will give for the normal
	// life-cycle advancement and removal to finish before forcefully removing the
	// remote application. This duration is ignored if the force argument is false.
	// The UUID for the scheduled removal job is returned.
	RemoveRemoteRelation(
		ctx context.Context,
		relUUID corerelation.UUID,
		force bool,
		wait time.Duration,
	) (removal.UUID, error)
}
