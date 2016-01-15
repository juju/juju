// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state/migration"
	"github.com/juju/juju/version"
)

// Import the database agnostic model representation into the database.
func (st *State) Import(description migration.Description) error {

	// At this stage, attempting to import a model with the same
	// UUID as an existing model will error.
	model := description.Model()
	envTag := model.Tag()
	_, err := st.GetEnvironment(envTag)
	if err == nil {
		// We have an existing matching environment (model).
		return errors.AlreadyExistsf("model with UUID %s", envTag.Id())
	} else if !errors.IsNotFound(err) {
		return errors.Trace(err)
	}

	// Create the environment.
	cfg, err := config.New(config.NoDefaults, model.Config())
	if err != nil {
		return errors.Trace(err)
	}
	env, envSt, err := st.NewEnvironment(cfg, model.Owner())
	if err != nil {
		return errors.Trace(err)
	}
	defer envSt.Close()

	if latest := model.LatestToolsVersion(); latest != version.Zero {
		if err := env.UpdateLatestToolsVersion(latest); err != nil {
			return errors.Trace(err)
		}
	}

	// Add machine docs...

	// NOTE: at the end of the import make sure that the mode of the model
	// is set to "imported" not "active" (or whatever we call it). This way
	// we don't start environment workers for it before the migration process
	// is complete.
	return nil
}
