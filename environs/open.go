// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/jujuclient"
)

// AdminUser is the initial admin user created for all controllers.
const AdminUser = "admin"

// New returns a new environment based on the provided configuration.
func New(ctx context.Context, args OpenParams, invalidator CredentialInvalidator) (Environ, error) {
	p, err := Provider(args.Cloud.Type)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return Open(ctx, p, args, invalidator)
}

// Open creates an Environ instance and errors if the provider is not for a cloud.
func Open(ctx context.Context, p EnvironProvider, args OpenParams, invalidator CredentialInvalidator) (Environ, error) {
	if envProvider, ok := p.(CloudEnvironProvider); !ok {
		return nil, errors.NotValidf("cloud environ provider %T", p)
	} else {
		return envProvider.Open(ctx, args, invalidator)
	}
}

// Destroy destroys the controller and, if successful,
// its associated configuration data from the given store.
func Destroy(
	controllerName string,
	env ControllerDestroyer,
	ctx context.Context,
	store jujuclient.ControllerStore,
) error {
	details, err := store.ControllerByName(controllerName)
	if errors.Is(err, errors.NotFound) {
		// No controller details, nothing to do.
		return nil
	} else if err != nil {
		return errors.Trace(err)
	}
	if err := env.DestroyController(ctx, details.ControllerUUID); err != nil {
		return errors.Trace(err)
	}
	err = store.RemoveController(controllerName)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	}
	return nil
}
