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
	// NewRenameSpaceOp returns an operation for renaming a space.
	NewRenameSpaceOp(spaceName, toName string) (state.ModelOperation, error)
}

type opFactory struct {
	st *state.State
}

func newOpFactory(st *state.State) OpFactory {
	return &opFactory{st: st}
}

// NewRenameSpaceOp (OpFactory) returns an operation
// for renaming a space.
func (f *opFactory) NewRenameSpaceOp(fromName, toName string) (state.ModelOperation, error) {
	space, err := f.st.SpaceByName(fromName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewRenameSpaceOp(
		f.st.IsController(), f.st.NewControllerSettings(), &renameSpaceState{f.st}, space, toName), nil
}
