// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"io"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/user"
	userservice "github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/domain/application"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/relation"
	storageservice "github.com/juju/juju/domain/storage/service"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/storage"
)

// AgentPasswordService provides access to agent password management.
type AgentPasswordService interface {
	// SetUnitPassword sets the password for the given unit. If the unit does
	// not exist, an error satisfying [applicationerrors.UnitNotFound] is
	// returned.
	SetUnitPassword(ctx context.Context, unitName unit.Name, password string) error
}

// AgentBinaryStore is responsible for persisting agent binary's into a long
// term store for later retrival.
type AgentBinaryStore interface {
	// AddAgentBinaryWithSHA256 adds a new agent binary to the object store and saves its
	// metadata to the database.
	AddAgentBinaryWithSHA256(
		_ context.Context,
		data io.Reader,
		varions coreagentbinary.Version,
		size int64,
		sha256 string,
	) error
}

// ApplicationService instances save an application to dqlite state.
type ApplicationService interface {
	// CreateApplication creates a new application with the given name and
	// charm.
	CreateApplication(
		context.Context, string, charm.Charm, corecharm.Origin,
		applicationservice.AddApplicationArgs, relation.CreatePeerRelationsFunc,
	...applicationservice.AddUnitArg,
	) (coreapplication.ID, error)

	// ResolveControllerCharmDownload resolves the controller charm download
	// slot.
	ResolveControllerCharmDownload(
		ctx context.Context,
		resolve application.ResolveControllerCharmDownload,
	) (application.ResolvedControllerCharmDownload, error)

	// UpdateApplication updates the application with the given name.
	UpdateCAASUnit(ctx context.Context, unitName unit.Name, params applicationservice.UpdateCAASUnitParams) error
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

// KeyManagerService provides access to the authorised keys for individual users
// of a model.
type KeyManagerService interface {
	// AddPublicKeysForUser is responsible for adding public keys to a user in
	// this model. If no keys are supplied then no operation will take place.
	AddPublicKeysForUser(context.Context, user.UUID, ...string) error
}

// ModelConfigService provides access to the model configuration.
type ModelConfigService interface {
	// ModelConfig returns the current config for the model.
	ModelConfig(context.Context) (*config.Config, error)
}

// MachineService provides access to the machine domain. Used here to set
// the machine cloud instance data.
type MachineService interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name machine.Name) (string, error)
	// SetMachineCloudInstance sets an entry in the machine cloud instance table
	// along with the instance tags and the link to a lxd profile if any.
	SetMachineCloudInstance(
		ctx context.Context,
		machineUUID string,
		instanceID instance.Id,
		displayName string,
		hardwareCharacteristics *instance.HardwareCharacteristics,
	) error
	// InstanceIDAndName returns the cloud specific instance ID and display name for
	// this machine.
	InstanceIDAndName(ctx context.Context, machineUUID string) (instance.Id, string, error)
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

// RelationService defines operations for managing relations between application
// endpoints.
type RelationService interface {
	// CreatePeerRelations creates the peer relations for an application.
	CreatePeerRelations(ctx context.Context, appId coreapplication.ID) error
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

	// AddExternalUser adds a new external user to the database and does not set a
	// password or activation key.
	// The following error types are possible from this function:
	//   - accesserrors.UserNameNotValid: When the username supplied is not
	//     valid.
	//   - accesserrors.UserAlreadyExists: If a user with the supplied name
	//     already exists.
	//   - accesserrors.CreatorUUIDNotFound: If the creator supplied for the
	//     user does not exist.
	AddExternalUser(ctx context.Context, name user.Name, displayName string, creatorUUID user.UUID) error

	// GetUserByName will return the user with the given name.
	GetUserByName(ctx context.Context, name user.Name) (user.User, error)
}
