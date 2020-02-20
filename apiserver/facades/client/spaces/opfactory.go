// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"github.com/juju/errors"

	"github.com/juju/juju/state"
)

// OpFactory describes a source of model operations
// required by the spaces API.
type OpFactory interface {
	// NewRemoveSpaceModelOp returns an operation for removing a space.
	NewRemoveSpaceModelOp(fromName string) (state.ModelOperation, error)

	// NewRenameSpaceModelOp returns an operation for renaming a space.
	NewRenameSpaceModelOp(fromName, toName string) (state.ModelOperation, error)

	// NewMoveToSpaceModelOp returns an operation for updating a space with new CIDRs.
	NewMoveToSpaceModelOp(spaceID string, subnets []MoveSubnet) (MoveToSpaceModelOp, error)
}

type opFactory struct {
	st *state.State
}

func newOpFactory(st *state.State) OpFactory {
	return &opFactory{st: st}
}

// NewRemoveSpaceModelOp (OpFactory) returns an operation
// for removing a space.
func (f *opFactory) NewRemoveSpaceModelOp(fromName string) (state.ModelOperation, error) {
	space, err := f.st.SpaceByName(fromName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnets, err := space.Subnets()
	if err != nil {
		return nil, errors.Trace(err)
	}
	n := make([]RemoveSubnet, len(subnets))
	for i, subnet := range subnets {
		n[i] = subnet
	}
	return NewRemoveSpaceModelOp(space, n), nil
}

// NewRenameSpaceModelOp (OpFactory) returns an operation
// for renaming a space.
func (f *opFactory) NewRenameSpaceModelOp(fromName, toName string) (state.ModelOperation, error) {
	space, err := f.st.SpaceByName(fromName)
	controllerSettings := f.st.NewControllerSettings()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewRenameSpaceModelOp(f.st.IsController(), controllerSettings, &renameSpaceStateShim{f.st}, space, toName), nil
}

// NewMoveToSpaceModelOp (OpFactory) returns an operation
// to move a list of CIDRs to a space.
func (f *opFactory) NewMoveToSpaceModelOp(spaceName string, subnets []MoveSubnet) (MoveToSpaceModelOp, error) {
	return NewUpdateSpaceModelOp(spaceName, subnets), nil
}
