// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/assumes"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/environs/config"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/storage"
)

// Services represents all the services that the application facade requires.
type Services struct {
	ApplicationService        ApplicationService
	ExternalControllerService ExternalControllerService
	MachineService            MachineService
	ModelConfigService        ModelConfigService
	NetworkService            NetworkService
	PortService               PortService
	RelationService           RelationService
	ResourceService           ResourceService
	StorageService            StorageService
	StubService               StubService
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
	if s.PortService == nil {
		return errors.NotValidf("empty PortService")
	}
	if s.ResourceService == nil {
		return errors.NotValidf("empty ResourceService")
	}
	if s.StorageService == nil {
		return errors.NotValidf("empty StorageService")
	}
	if s.StubService == nil {
		return errors.NotValidf("empty StubService")
	}
	if s.RelationService == nil {
		return errors.NotValidf("empty RelationService")
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
	// Space returns a space from state that matches the input ID.
	// An error is returned if the space does not exist or if there was a problem
	// accessing its information.
	Space(ctx context.Context, uuid string) (*network.SpaceInfo, error)
	// SpaceByName returns a space from state that matches the input name.
	// An error is returned that satisfied errors.NotFound if the space was not found
	// or an error static any problems fetching the given space.
	SpaceByName(ctx context.Context, name string) (*network.SpaceInfo, error)
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
}

// MachineService defines the methods that the facade assumes from the Machine
// service.
type MachineService interface {
	// CreateMachine creates the specified machine.
	CreateMachine(context.Context, machine.Name) (string, error)
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name machine.Name) (string, error)
	// HardwareCharacteristics returns the hardware characteristics of the
	// specified machine.
	HardwareCharacteristics(ctx context.Context, machineUUID string) (*instance.HardwareCharacteristics, error)
}

// ApplicationService instances save an application to dqlite state.
type ApplicationService interface {
	// CreateApplication creates the specified application and units if required.
	CreateApplication(ctx context.Context, name string, charm internalcharm.Charm, origin corecharm.Origin, params applicationservice.AddApplicationArgs, units ...applicationservice.AddUnitArg) (coreapplication.ID, error)
	// AddUnits adds units to the application.
	AddUnits(ctx context.Context, storageParentDir, name string, units ...applicationservice.AddUnitArg) error
	// SetApplicationCharm sets a new charm for the application, validating that aspects such
	// as storage are still viable with the new charm.
	SetApplicationCharm(ctx context.Context, name string, params applicationservice.UpdateCharmParams) error
	// SetApplicationScale sets the application's desired scale value.
	// This is used on CAAS models.
	SetApplicationScale(ctx context.Context, name string, scale int) error
	// ChangeApplicationScale alters the existing scale by the provided change amount, returning the new amount.
	// This is used on CAAS models.
	ChangeApplicationScale(ctx context.Context, name string, scaleChange int) (int, error)

	// DestroyApplication prepares an application for removal from the model.
	DestroyApplication(ctx context.Context, name string) error

	// DeleteApplication deletes the specified application,
	// TODO(units) - remove when destroy is fully implemented.
	DeleteApplication(ctx context.Context, name string) error

	// DestroyUnit prepares a unit for removal from the model.
	DestroyUnit(context.Context, unit.Name) error

	// GetApplicationLife looks up the life of the specified application.
	GetApplicationLife(ctx context.Context, name string) (life.Value, error)

	// GetUnitLife looks up the life of the specified unit.
	GetUnitLife(context.Context, unit.Name) (life.Value, error)

	// GetUnitUUID returns the UUID for the named unit.
	GetUnitUUID(context.Context, unit.Name) (unit.UUID, error)

	// GetSupportedFeatures returns the set of features that the model makes
	// available for charms to use.
	GetSupportedFeatures(context.Context) (assumes.FeatureSet, error)

	// GetCharmLocatorByApplicationName returns a CharmLocator by application name.
	// It returns an error if the charm can not be found by the name. This can also
	// be used as a cheap way to see if a charm exists without needing to load the
	// charm metadata.
	GetCharmLocatorByApplicationName(ctx context.Context, name string) (applicationcharm.CharmLocator, error)

	// GetCharm returns the charm by name, source and revision. Calling this method
	// will return all the data associated with the charm. It is not expected to
	// call this method for all calls, instead use the move focused and specific
	// methods. That's because this method is very expensive to call. This is
	// implemented for the cases where all the charm data is needed; model
	// migration, charm export, etc.
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

	// GetApplicationAndCharmConfig returns the application and charm config for the
	// specified application ID.
	GetApplicationAndCharmConfig(ctx context.Context, appID coreapplication.ID) (applicationservice.ApplicationConfig, error)

	// SetApplicationConstraints sets the application constraints for the
	// specified application ID.
	// This method overwrites the full constraints on every call.
	// If invalid constraints are provided (e.g. invalid container type or
	// non-existing space), a [applicationerrors.InvalidApplicationConstraints]
	// error is returned.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	SetApplicationConstraints(ctx context.Context, appID coreapplication.ID, cons constraints.Value) error

	// UpdateApplicationConfig updates the application config with the specified
	// values. If the key does not exist, it is created. If the key already exists,
	// it is updated, if there is no value it is removed. With the caveat that
	// application trust will be set to false.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	// If the charm config is not valid, an error satisfying
	// [applicationerrors.InvalidApplicationConfig] is returned.
	UpdateApplicationConfig(context.Context, coreapplication.ID, map[string]string) error
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

// StubService is the interface used to interact with the stub service. A special
// service which collects temporary methods required to wire together together
// domains which are not completely implemented or wired up.
//
// TODO: Remove this dependency once units are properly assigned to machines via
// net nodes.
type StubService interface {
	// AssignUnitsToMachines assigns the given units to the given machines but setting
	// unit net node to the machine net node.
	//
	// Deprecated: AssignUnitsToMachines will become redundant once the machine and
	// application domains have become fully implemented.
	AssignUnitsToMachines(context.Context, map[string][]unit.Name) error
}

// StorageService instances get a storage pool by name.
type StorageService interface {
	// GetStoragePoolByName returns the storage pool with the specified name.
	GetStoragePoolByName(ctx context.Context, name string) (*storage.Config, error)
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
	// endpoint between applications on the model. A new relation will be created
	// between these endpoints and the details of the endpoint returned.
	//
	// If the identifiers do not uniquely specify a relation, an error will be
	// returned.
	AddRelation(ctx context.Context, ep1, ep2 string) (relation.Endpoint, relation.Endpoint, error)

	// ApplicationRelationsInfo returns all EndpointRelationData for an application.
	ApplicationRelationsInfo(ctx context.Context, applicationID coreapplication.ID) ([]relation.EndpointRelationData, error)
}
