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
	// NewRenameSpaceModelOp returns an operation for renaming a space.
	NewRenameSpaceModelOp(fromName, toName string) (state.ModelOperation, error)
}

type opFactory struct {
	st *state.State
}

func newOpFactory(st *state.State) OpFactory {
	return &opFactory{st: st}
}

// NewRenameSpaceModelOp (OpFactory) returns an operation
// for renaming a space.
func (f *opFactory) NewRenameSpaceModelOp(fromName, toName string) (state.ModelOperation, error) {
	space, err := f.st.SpaceByName(fromName)
	controllerSettings := f.st.NewControllerSettings()
	if err != nil {
		return nil, errors.Trace(err)
	}
	model, err := f.st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewRenameSpaceModelOp(model.IsControllerModel(), controllerSettings, &renameSpaceStateShim{f.st}, space, toName), nil
}
