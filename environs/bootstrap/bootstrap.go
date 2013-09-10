// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"fmt"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/provider"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
)

var logger = loggo.GetLogger("juju.environs.boostrap")

// BootstrapStorage is an interface that returns a environs.Storage that may
// be used before the bootstrap machine agent has been provisioned.
//
// This is useful for environments where the storage is managed by the machine
// agent once bootstrapped.
type BootstrapStorage interface {
	// BootstrapStorage returns an environs.Storage that may be used while
	// bootstrapping a machine.
	BootstrapStorage() (environs.Storage, error)
}

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
	// If the state file exists, it might actually have just been
	// removed by Destroy, and eventual consistency has not caught
	// up yet, so we retry to verify if that is happening.
	err := verifyBootstrapInit(environ)
	if err != nil {
		return err
	}

	// The bootstrap instance gets machine id "0".  This is not related to
	// instance ids.  Juju assigns the machine ID.
	const machineID = "0"
	logger.Infof("bootstrapping environment %q", environ.Name())
	var vers *version.Number
	if agentVersion, ok := cfg.AgentVersion(); ok {
		vers = &agentVersion
	}
	newestTools, err := tools.FindBootstrapTools(environ, vers, cfg.DefaultSeries(), cons.Arch, cfg.Development())
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
	return environ.Bootstrap(cons, newestTools, machineID)
}

// verifyBootstrapInit does the common initial check before bootstrapping, to
// confirm that the environment isn't already running, and that the storage
// works.
func verifyBootstrapInit(env environs.Environ) error {
	var err error

	storage := env.Storage()

	// If the state file exists, it might actually have just been
	// removed by Destroy, and eventual consistency has not caught
	// up yet, so we retry to verify if that is happening.
	for a := storage.ConsistencyStrategy().Start(); a.Next(); {
		if _, err = provider.LoadState(storage); err != nil {
			break
		}
	}
	if err == nil {
		return fmt.Errorf("environment is already bootstrapped")
	}
	if !errors.IsNotBootstrapped(err) {
		return fmt.Errorf("cannot query old bootstrap state: %v", err)
	}

	return environs.VerifyStorage(storage)
}

// ConfigureBootstrapMachine adds the initial machine into state.  As a part
// of this process the environmental constraints are saved as constraints used
// when bootstrapping are considered constraints for the entire environment.
func ConfigureBootstrapMachine(
	st *state.State,
	cons constraints.Value,
	datadir string,
	jobs []state.MachineJob,
	instId instance.Id,
	characteristics instance.HardwareCharacteristics,
) error {
	logger.Debugf("setting environment constraints")
	if err := st.SetEnvironConstraints(cons); err != nil {
		return err
	}

	logger.Debugf("create bootstrap machine in state")
	m, err := st.InjectMachine(&state.AddMachineParams{
		Series:                  version.Current.Series,
		Nonce:                   state.BootstrapNonce,
		Constraints:             cons,
		InstanceId:              instId,
		HardwareCharacteristics: characteristics,
		Jobs: jobs,
	})
	if err != nil {
		return err
	}
	// Read the machine agent's password and change it to
	// a new password (other agents will change their password
	// via the API connection).
	logger.Debugf("create new random password for machine %v", m.Id())
	mconf, err := agent.ReadConf(datadir, m.Tag())
	if err != nil {
		return err
	}
	newPassword, err := mconf.GenerateNewPassword()
	if err != nil {
		return err
	}
	if err := m.SetMongoPassword(newPassword); err != nil {
		return err
	}
	if err := m.SetPassword(newPassword); err != nil {
		return err
	}
	return nil
}
