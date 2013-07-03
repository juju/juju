// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/utils"
)

// Bootstrap bootstraps the given environment. The supplied constraints are
// used to provision the instance, and are also set within the bootstrapped
// environment.
func Bootstrap(environ Environ, cons constraints.Value) error {
	cfg := environ.Config()
	if secret := cfg.AdminSecret(); secret == "" {
		return fmt.Errorf("environment configuration has no admin-secret")
	}
	if authKeys := cfg.AuthorizedKeys(); authKeys == "" {
		// Apparently this can never happen, so it's not tested. But, one day,
		// Config will act differently (it's pretty crazy that, AFAICT, the
		// authorized-keys are optional config settings... but it's impossible
		// to actually *create* a config without them)... and when it does,
		// we'll be here to catch this problem early.
		return fmt.Errorf("environment configuration has no authorized-keys")
	}
	if _, hasCACert := cfg.CACert(); !hasCACert {
		return fmt.Errorf("environment configuration has no ca-cert")
	}
	if _, hasCAKey := cfg.CAPrivateKey(); !hasCAKey {
		return fmt.Errorf("environment configuration has no ca-private-key")
	}
	return environ.Bootstrap(cons)
}

// VerifyBootstrapInit does the common initial check inside bootstrap to
// confirm that the environment isn't already running, and that the storage
// works.
func VerifyBootstrapInit(env Environ, shortAttempt utils.AttemptStrategy) error {
	var err error

	// If the state file exists, it might actually have just been
	// removed by Destroy, and eventual consistency has not caught
	// up yet, so we retry to verify if that is happening.
	for a := shortAttempt.Start(); a.Next(); {
		_, err = LoadProviderState(env.Storage())
		if err != nil {
			break
		}
	}
	if err == nil {
		return fmt.Errorf("environment is already bootstrapped")
	}
	if !errors.IsNotFoundError(err) {
		return fmt.Errorf("cannot query old bootstrap state: %v", err)
	}

	err = VerifyStorage(env.Storage())
	if err != nil {
		return err
	}
	return nil
}
