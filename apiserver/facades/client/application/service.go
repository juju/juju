// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"context"

	"github.com/juju/version/v2"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
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

// MachineService instances save a machine to dqlite state.
type MachineService interface {
	// CreateMachine creates the specified machine.
	CreateMachine(context.Context, machine.Name) (string, error)
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

	// DestroyApplication prepares an application for removal from the model
	// returning an error  satisfying [applicationerrors.ApplicationNotFoundError]
	// if the application doesn't exist.
	DestroyApplication(ctx context.Context, name string) error
}

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

// ModelAgentService provides access to the Juju agent version for the model.
type ModelAgentService interface {
	// GetModelAgentVersion returns the agent version for the current model.
	GetModelAgentVersion(ctx context.Context) (version.Number, error)
}

// StoragePoolGetter instances get a storage pool by name.
type StoragePoolGetter interface {
	// GetStoragePoolByName returns the storage pool with the specified name.
	GetStoragePoolByName(ctx context.Context, name string) (*storage.Config, error)
}
