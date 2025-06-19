// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/collections/set"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// ReloadSpaces offers a version 1 of the ReloadSpacesAPI.
type ReloadSpaces interface {
	// ReloadSpaces refreshes spaces from the substrate.
	ReloadSpaces() error
}

// BlockChecker defines the block-checking functionality required by
// the spaces facade. This is implemented by apiserver/common.BlockChecker.
type BlockChecker interface {
	ChangeAllowed() error
	RemoveAllowed() error
}

// Address is an indirection for state.Address.
type Address interface {
	SubnetCIDR() string
	ConfigMethod() network.AddressConfigType
	Value() string
}

// Unit is an indirection for state.Unit.
type Unit interface {
	Name() string
	ApplicationName() string
}

// Machine defines the methods supported by a machine used in the space context.
type Machine interface {
	AllAddresses() ([]Address, error)
	Units() ([]Unit, error)
	AllSpaces() (set.Strings, error)
}

// Constraints defines the methods supported by constraints used in the space context.
type Constraints interface {
	ID() string
	Value() constraints.Value
	ChangeSpaceNameOps(from, to string) []txn.Op
}

// Bindings describes a collection of endpoint bindings for an application.
type Bindings interface {
	// Map returns the space IDs for each bound endpoint.
	Map() map[string]string
}

// Backing describes the state methods used in this package.
type Backing interface {
	// ModelConfig returns the current model configuration.
	ModelConfig() (*config.Config, error)

	// CloudSpec returns a cloud specification.
	CloudSpec() (environscloudspec.CloudSpec, error)

	// ModelTag returns the tag of this model.
	ModelTag() names.ModelTag

	// SubnetByCIDR returns a unique subnet based on the input CIDR.
	SubnetByCIDR(cidr string) (networkingcommon.BackingSubnet, error)

	// MovingSubnet returns the subnet for the input ID,
	// suitable for moving to a new space.
	MovingSubnet(id string) (MovingSubnet, error)

	// AddSpace creates a space.
	AddSpace(name string, providerID network.Id, subnets []string, public bool) (networkingcommon.BackingSpace, error)

	// AllSpaces returns all known Juju network spaces.
	// TODO (manadart 2020-04-14): This should be removed in favour of
	// AllSpaceInfos below, reducing the reliance on networkingcommon.
	AllSpaces() ([]networkingcommon.BackingSpace, error)

	// AllSpaceInfos returns SpaceInfos for all spaces in the model.
	AllSpaceInfos() (network.SpaceInfos, error)

	// SpaceByName returns the Juju network space given by name.
	SpaceByName(name string) (networkingcommon.BackingSpace, error)

	// AllEndpointBindings loads all endpointBindings.
	AllEndpointBindings() (map[string]Bindings, error)

	// AllMachines loads all machines.
	AllMachines() ([]Machine, error)

	// ApplyOperation applies a given ModelOperation to the model.
	ApplyOperation(state.ModelOperation) error

	// ControllerConfig returns the controller config.
	ControllerConfig() (controller.Config, error)

	// AllConstraints returns all constraints in the model.
	AllConstraints() ([]Constraints, error)

	// ConstraintsBySpaceName returns constraints found by spaceName.
	ConstraintsBySpaceName(name string) ([]Constraints, error)

	// IsController returns true if this state instance
	// is for the controller model.
	IsController() bool
}
