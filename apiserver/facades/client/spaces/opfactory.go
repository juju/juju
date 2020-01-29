/*
 * // Copyright 2020 Canonical Ltd.
 * // Licensed under the AGPLv3, see LICENCE file for details.
 */

package spaces

import (
	"github.com/juju/errors"

	"github.com/juju/juju/state"
)

// OpFactory describes a source of model operations
// required by the spaces API.
type OpFactory interface {
	// NewRenameSpaceModelOp returns an operation for renaming space.
	NewRenameSpaceModelOp(fromName, toName string) (RenameSpaceModelOp, error)
}

type opFactory struct {
	st *state.State
}

func newOpFactory(st *state.State) OpFactory {
	return &opFactory{st: st}
}

// NewRenameSpaceModelOp (OpFactory) returns an operation
// for committing a branch.
func (f *opFactory) NewRenameSpaceModelOp(fromName, toName string) (RenameSpaceModelOp, error) {
	space, err := f.st.SpaceByName(fromName)
	controllerSettings := f.st.NewControllerSettings()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewRenameSpaceModelOp(controllerSettings, &renameSpaceStateShim{f.st}, space, toName), nil
}
