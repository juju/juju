// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"fmt"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/provider/common"
	"launchpad.net/juju-core/version"
)

var logger = loggo.GetLogger("juju.environs.boostrap")

// Bootstrap bootstraps the given environment. The supplied constraints are
// used to provision the instance, and are also set within the bootstrapped
// environment.
func Bootstrap(environ environs.Environ, cons constraints.Value) error {
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
	// Write out the bootstrap-init file, and confirm storage is writeable.
	if err := environs.VerifyStorage(environ.Storage()); err != nil {
		return err
	}

	logger.Infof("bootstrapping environment %q", environ.Name())
	var vers *version.Number
	if agentVersion, ok := cfg.AgentVersion(); ok {
		vers = &agentVersion
	}
	params := tools.BootstrapToolsParams{
		Version: vers,
		Arch:    cons.Arch,
	}
	newestTools, err := tools.FindBootstrapTools(environ, params)
	if err != nil {
		return fmt.Errorf("cannot find bootstrap tools: %v", err)
	}

	// If agent version was not previously known, set it here using the latest compatible tools version.
	if vers == nil {
		// We probably still have a mix of versions available; discard older ones
		// and update environment configuration to use only those remaining.
		var newVersion version.Number
		newVersion, newestTools = newestTools.Newest()
		vers = &newVersion
		logger.Infof("environs: picked newest version: %s", *vers)
		cfg, err = cfg.Apply(map[string]interface{}{
			"agent-version": vers.String(),
		})
		if err == nil {
			err = environ.SetConfig(cfg)
		}
		if err != nil {
			return fmt.Errorf("failed to update environment configuration: %v", err)
		}
	}
	// ensure we have at least one valid tools
	if len(newestTools) == 0 {
		return fmt.Errorf("No bootstrap tools found")
	}
	return environ.Bootstrap(cons, newestTools)
}

// EnsureNotBootstrapped returns null if the environment is not bootstrapped,
// and an error if it is or if the function was not able to tell.
func EnsureNotBootstrapped(env environs.Environ) error {
	_, err := common.LoadState(env.Storage())
	// If there is no error loading the bootstrap state, then we are bootstrapped.
	if err == nil {
		return fmt.Errorf("environment is already bootstrapped")
	}
	if err == environs.ErrNotBootstrapped {
		return nil
	}
	return err
}
