// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state/migration"
	"github.com/juju/juju/version"
)

// Import the database agnostic model representation into the database.
func (st *State) Import(description migration.Description) (_ *Environment, _ *State, err error) {

	// At this stage, attempting to import a model with the same
	// UUID as an existing model will error.
	model := description.Model()
	envTag := model.Tag()
	_, err = st.GetEnvironment(envTag)
	if err == nil {
		// We have an existing matching environment (model).
		return nil, nil, errors.AlreadyExistsf("model with UUID %s", envTag.Id())
	} else if !errors.IsNotFound(err) {
		return nil, nil, errors.Trace(err)
	}

	// Create the environment.
	cfg, err := config.New(config.NoDefaults, model.Config())
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	env, envSt, err := st.NewEnvironment(cfg, model.Owner())
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	defer func() {
		if err != nil {
			envSt.Close()
		}
	}()

	if latest := model.LatestToolsVersion(); latest != version.Zero {
		if err := env.UpdateLatestToolsVersion(latest); err != nil {
			return nil, nil, errors.Trace(err)
		}
	}

	// I would have loved to use import, but that is a reserved word.
	restore := importer{
		st:          envSt,
		environment: env,
		model:       model,
	}
	if err := restore.environmentUsers(); err != nil {
		return nil, nil, errors.Trace(err)
	}

	// Add machine docs...

	// NOTE: at the end of the import make sure that the mode of the model
	// is set to "imported" not "active" (or whatever we call it). This way
	// we don't start environment workers for it before the migration process
	// is complete.
	return env, envSt, nil
}

type importer struct {
	st          *State
	environment *Environment
	model       migration.Model
}

func (i *importer) environmentUsers() error {
	// The user that was auto-added when we created the environment will have
	// the wrong DateCreated, so we remove it, and add in all the users we
	// know about. It is also possible that the owner of the environment no
	// longer has access to the environment due to changes over time.
	if err := i.st.RemoveEnvironmentUser(i.model.Owner()); err != nil {
		return errors.Trace(err)
	}

	users := i.model.Users()
	envuuid := i.environment.UUID()
	var ops []txn.Op
	for _, user := range users {
		ops = append(ops, createEnvUserOp(
			envuuid,
			user.Name(),
			user.CreatedBy(),
			user.DisplayName(),
			user.DateCreated(),
			user.ReadOnly()))
	}
	if err := i.st.runTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	// Now set their last connection times.
	for _, user := range users {
		lastConnection := user.LastConnection()
		if lastConnection.IsZero() {
			continue
		}
		envUser, err := i.st.EnvironmentUser(user.Name())
		if err != nil {
			return errors.Trace(err)
		}
		err = envUser.updateLastConnection(lastConnection)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
