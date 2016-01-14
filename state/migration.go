// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state/migration"
	"github.com/juju/juju/version"
)

// Export the representation of the environment into the database agnostic
// model description.
func (st *State) Export(env names.EnvironTag) (migration.Description, error) {
	envState := st
	if st.EnvironTag() != env {
		s, err := st.ForEnviron(env)
		if err != nil {
			return nil, errors.Trace(err)
		}
		envState = s
		defer envState.Close()
	}

	environment, err := envState.Environment()
	if err != nil {
		return nil, errors.Trace(err)
	}
	settings, err := envState.readAllSettings()
	if err != nil {
		return nil, errors.Trace(err)
	}

	envConfig, found := settings[environGlobalKey]
	if !found {
		return nil, errors.New("missing environ config")
	}

	args := migration.ModelArgs{
		Owner:              environment.Owner(),
		Config:             envConfig.Settings,
		LatestToolsVersion: environment.LatestToolsVersion(),
	}

	result := migration.NewDescription(args)

	// Add machines...

	return result, nil
}

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

func (st *State) readAllSettings() (map[string]settingsDoc, error) {
	settings, closer := st.getCollection(settingsC)
	defer closer()

	var docs []settingsDoc
	if err := settings.Find(nil).All(&docs); err != nil {
		return nil, errors.Trace(err)
	}

	result := make(map[string]settingsDoc)
	for _, doc := range docs {
		key := st.localID(doc.DocID)
		result[key] = doc
	}
	return result, nil
}
