// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"fmt"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/state"
)

// GetEnvironStorage creates an Environ from the config in state and
// returns its storage interface.
func GetEnvironStorage(st *state.State) (storage.Storage, error) {
	envConfig, err := st.EnvironConfig()
	if err != nil {
		return nil, fmt.Errorf("cannot get environment config: %v", err)
	}
	env, err := environs.New(envConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot access environment: %v", err)
	}
	return env.Storage(), nil
}
