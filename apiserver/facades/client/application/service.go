// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/assumes"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/storage"
)

// ExternalControllerService provides a subset of the external controller domain
// service methods.
type ExternalControllerService interface {
	// UpdateExternalController persists the input controller
	// record.
	UpdateExternalController(ctx context.Context, ec crossmodel.ControllerInfo) error
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
	CreateApplication(ctx context.Context, name string, charm charm.Charm, origin corecharm.Origin, params applicationservice.AddApplicationArgs, units ...applicationservice.AddUnitArg) (coreapplication.ID, error)
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

// StoragePoolGetter instances get a storage pool by name.
type StoragePoolGetter interface {
	// GetStoragePoolByName returns the storage pool with the specified name.
	GetStoragePoolByName(ctx context.Context, name string) (*storage.Config, error)
}
