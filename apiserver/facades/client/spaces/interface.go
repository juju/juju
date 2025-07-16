// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"context"

	"github.com/juju/mgo/v3/txn"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
)

// BlockChecker defines the block-checking functionality required by
// the spaces facade. This is implemented by apiserver/common.BlockChecker.
type BlockChecker interface {
	ChangeAllowed(context.Context) error
	RemoveAllowed(context.Context) error
}

// Address is an indirection for state.Address.
type Address interface {
	SubnetCIDR() string
	ConfigMethod() network.AddressConfigType
	Value() string
}

// Machine defines the methods supported by a machine used in the space context.
type Machine interface {
	Id() string
	AllAddresses() ([]Address, error)
}

// Constraints defines the methods supported by constraints used in the space context.
type Constraints interface {
	ID() string
	Value() constraints.Value
	ChangeSpaceNameOps(from, to string) []txn.Op
}

// Backing describes the state methods used in this package.
type Backing interface {
	// AllMachines loads all machines.
	AllMachines() ([]Machine, error)

	// AllConstraints returns all constraints in the model.
	AllConstraints() ([]Constraints, error)

	// ConstraintsBySpaceName returns constraints found by spaceName.
	ConstraintsBySpaceName(name string) ([]Constraints, error)

	// IsController returns true if this state instance
	// is for the controller model.
	IsController() bool
}
