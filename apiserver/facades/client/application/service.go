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
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
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
	if s.StorageService == nil {
		return errors.NotValidf("empty StorageService")
	}
	if s.StubService == nil {
		return errors.NotValidf("empty StubService")
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
	AddUnits(ctx context.Context, name string, units ...applicationservice.AddUnitArg) error
	// UpdateApplicationCharm sets a new charm for the application, validating that aspects such
	// as storage are still viable with the new charm.
	UpdateApplicationCharm(ctx context.Context, name string, params applicationservice.UpdateCharmParams) error
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

	// GetCharmID returns a charm ID by name. It returns an error if the charm
	// can not be found by the name. This can also be used as a cheap way to see if
	// a charm exists without needing to load the charm metadata.
	GetCharmID(ctx context.Context, args applicationcharm.GetCharmArgs) (corecharm.ID, error)

	// GetCharmIDByApplicationName returns a charm ID by application name. It
	// returns an error if the charm can not be found by the name. This can also be
	// used as a cheap way to see if a charm exists without needing to load the
	// charm metadata.
	GetCharmIDByApplicationName(ctx context.Context, name string) (corecharm.ID, error)

	// GetCharm returns the charm using the charm ID. Calling this method will
	// return all the data associated with the charm. It is not expected to call
	// this method for all calls, instead use the move focused and specific
	// methods. That's because this method is very expensive to call. This is
	// implemented for the cases where all the charm data is needed; model
	// migration, charm export, etc.
	GetCharm(ctx context.Context, id corecharm.ID) (internalcharm.Charm, applicationcharm.CharmOrigin, error)

	// GetCharmMetadata returns the metadata for the charm using the charm ID.
	GetCharmMetadata(ctx context.Context, id corecharm.ID) (internalcharm.Meta, error)

	// GetCharmConfig returns the config for the charm using the charm ID.
	GetCharmConfig(ctx context.Context, id corecharm.ID) (internalcharm.Config, error)

	// GetCharmMetadataName returns the name for the charm using the
	// charm ID.
	GetCharmMetadataName(ctx context.Context, id corecharm.ID) (string, error)
}

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

type PortService interface {
	// GetUnitOpenedPorts returns the opened ports for a given unit uuid,
	// grouped by endpoint.
	GetUnitOpenedPorts(ctx context.Context, unitUUID unit.UUID) (network.GroupedPortRanges, error)
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
