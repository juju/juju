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
	// NewRemoveSpaceOp returns an operation for removing a space.
	NewRemoveSpaceOp(fromName string) (state.ModelOperation, error)

	// NewRenameSpaceOp returns an operation for renaming a space.
	NewRenameSpaceOp(spaceName, toName string) (state.ModelOperation, error)

	// NewMoveSubnetsOp returns an operation for updating a space with new CIDRs.
	NewMoveSubnetsOp(spaceID string, subnets []MovingSubnet) (MoveSubnetsOp, error)
}

type opFactory struct {
	st                      *state.State
	controllerConfigService ControllerConfigService
}

func newOpFactory(st *state.State, controllerConfigService ControllerConfigService) OpFactory {
	return &opFactory{
		st:                      st,
		controllerConfigService: controllerConfigService,
	}
}

// NewRemoveSpaceOp (OpFactory) returns an operation
// for removing a space.
func (f *opFactory) NewRemoveSpaceOp(spaceName string) (state.ModelOperation, error) {
	space, err := f.st.SpaceByName(spaceName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewRemoveSpaceOp(space), nil
}

// NewRenameSpaceOp (OpFactory) returns an operation
// for renaming a space.
func (f *opFactory) NewRenameSpaceOp(fromName, toName string) (state.ModelOperation, error) {
	space, err := f.st.SpaceByName(fromName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewRenameSpaceOp(
		f.st.IsController(),
		f.st.NewControllerSettings(),
		&renameSpaceState{State: f.st},
		f.controllerConfigService,
		space,
		toName,
	), nil
}

// NewMoveSubnetsOp (OpFactory) returns an operation
// to move a list of subnets to a space.
func (f *opFactory) NewMoveSubnetsOp(spaceName string, subnets []MovingSubnet) (MoveSubnetsOp, error) {
	space, err := f.st.SpaceByName(spaceName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewMoveSubnetsOp(f.st, space.Id(), subnets), nil
}
