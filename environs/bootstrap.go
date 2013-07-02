// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"bytes"
	"fmt"
	"io/ioutil"

	"launchpad.net/goyaml"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/instance"
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

const stateFile = "provider-state"

type bootstrapState struct {
	instances []instance.Id `yaml:"state-instances"`
}

// SaveProviderState stores the instances in the state file in the defined
// storage.
func SaveProviderState(storage Storage, instances ...instance.Id) error {
	state := &bootstrapState{instances}
	data, err := goyaml.Marshal(state)
	if err != nil {
		return err
	}
	return storage.Put(stateFile, bytes.NewBuffer(data), int64(len(data)))
}

// LaveProviderState retrieves the instances from the state file in the
// defined storage.
func LoadProviderState(storage Storage) ([]instance.Id, error) {
	r, err := storage.Get(stateFile)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("error reading %q: %v", stateFile, err)
	}
	var state bootstrapState
	err = goyaml.Unmarshal(data, &state)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling %q: %v", stateFile, err)
	}
	return state.instances, nil
}
