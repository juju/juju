// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/state/migration"
)

// Export the representation of the environment into the database agnostic
// model description.
func (st *State) Export(env names.EnvironTag) (migration.Description, error) {
	var model migration.Description
	return model, errors.NotImplementedf("State.Export")
}

// Import the database agnostic model representation into the database.
func (st *State) Import(description migration.Description) error {

	// NOTE: at the end of the import make sure that the mode of the model
	// is set to "imported" not "active" (or whatever we call it). This way
	// we don't start environment workers for it before the migration process
	// is complete.
	return errors.NotImplementedf("State.Import")
}
