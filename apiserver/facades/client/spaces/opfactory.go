// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/state"
)

// OpFactory describes a source of model operations
// required by the spaces API.
type OpFactory interface {
	// NewRemoveSpaceModelOp returns an operation for removing a space.
	NewRemoveSpaceModelOp(fromName string) (state.ModelOperation, error)

	// NewRenameSpaceModelOp returns an operation for renaming a space.
	NewRenameSpaceModelOp(spaceName, toName string) (state.ModelOperation, error)

	// TODO (manadart 2020-03-19): Rename the methods above for consistency.
	// We know they are are ModelOps; we can be brief without redundancy.
	// NewMoveSubnetsOp returns an operation for updating a space with new CIDRs.
	NewMoveSubnetsOp(spaceID string, subnets []MovingSubnet) (MoveSubnetsOp, error)
}

type opFactory struct {
	st *state.State
}

func newOpFactory(st *state.State) OpFactory {
	return &opFactory{st: st}
}

// NewRemoveSpaceModelOp (OpFactory) returns an operation
// for removing a space.
func (f *opFactory) NewRemoveSpaceModelOp(spaceName string) (state.ModelOperation, error) {
	space, err := f.st.SpaceByName(spaceName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnets, err := space.Subnets()
	if err != nil {
		return nil, errors.Trace(err)
	}
	n := make([]Subnet, len(subnets))
	for i, subnet := range subnets {
		n[i] = subnet
	}
	return NewRemoveSpaceModelOp(space, n), nil
}

// NewRenameSpaceModelOp (OpFactory) returns an operation
// for renaming a space.
func (f *opFactory) NewRenameSpaceModelOp(fromName, toName string) (state.ModelOperation, error) {
	space, err := f.st.SpaceByName(fromName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewRenameSpaceModelOp(
		f.st.IsController(), f.st.NewControllerSettings(), &renameSpaceState{f.st}, space, toName), nil
}

// NewMoveSubnetsOp (OpFactory) returns an operation
// to move a list of subnets to a space.
func (f *opFactory) NewMoveSubnetsOp(spaceName string, subnets []MovingSubnet) (MoveSubnetsOp, error) {
	space, err := f.st.SpaceByName(spaceName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewMoveSubnetsOp(networkingcommon.NewSpaceShim(space), subnets), nil
}
