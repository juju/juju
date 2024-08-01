// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/user"
	userservice "github.com/juju/juju/domain/access/service"
	applicationservice "github.com/juju/juju/domain/application/service"
	storageservice "github.com/juju/juju/domain/storage/service"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/storage"
)

// ApplicationService instances save an application to dqlite state.
type ApplicationService interface {
	CreateApplication(ctx context.Context, name string, charm charm.Charm, params applicationservice.AddApplicationArgs, units ...applicationservice.AddUnitArg) (coreapplication.ID, error)
}

// BakeryConfigService describes the service used to initialise the
// maccaroon bakery config
type BakeryConfigService interface {
	InitialiseBakeryConfig(context.Context) error
}

// ControllerConfigService is the interface that is used to get the
// controller configuration.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// CloudService is the interface that is used to interact with the
// cloud.
type CloudService interface {
	Cloud(context.Context, string) (*cloud.Cloud, error)
}

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

// ModelService provides a means for interacting with the underlying models of
// this controller
type ModelService interface {
	// ControllerModel returns the representation of the model that is used for
	// running the Juju controller.
	// Should this model not have been established yet an error satisfying
	// [github.com/juju/juju/domain/model/errors.NotFound] will be returned.
	ControllerModel(context.Context) (model.Model, error)
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
	// ReloadSpaces loads spaces and subnets from the provider into state.
	ReloadSpaces(ctx context.Context) error
}

// StorageService instances save a storage pool to dqlite state.
type StorageService interface {
	CreateStoragePool(ctx context.Context, name string, providerType storage.ProviderType, attrs storageservice.PoolAttrs) error
}

// UserService is the interface that is used to add a new user to the
// database.
type UserService interface {
	// AddUser will add a new user to the database and return the UUID of the
	// user if successful. If no password is set in the incoming argument,
	// the user will be added with an activation key.
	AddUser(ctx context.Context, arg userservice.AddUserArg) (user.UUID, []byte, error)

	// GetUserByName will return the user with the given name.
	GetUserByName(ctx context.Context, name string) (user.User, error)
}
