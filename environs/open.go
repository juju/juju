// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
)

// AdminUser is the initial admin user created for all controllers.
const AdminUser = "admin@local"

// New returns a new environment based on the provided configuration.
func New(config *config.Config) (Environ, error) {
	p, err := Provider(config.Type())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return p.Open(config)
}

// Destroy destroys the controller and, if successful,
// its associated configuration data from the given store.
func Destroy(
	controllerName string,
	env Environ,
	store jujuclient.ControllerStore,
) error {
	details, err := store.ControllerByName(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	if err := env.DestroyController(details.ControllerUUID); err != nil {
		return errors.Trace(err)
	}
	err = store.RemoveController(controllerName)
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	return nil
}
