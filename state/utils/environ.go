// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
)

// GetEnvironment returns the environs.Environ ("provider") associated
// with the environment.
func GetEnvironment(st *state.State) (environs.Environ, error) {
	envcfg, err := st.EnvironConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	env, err := environs.New(envcfg)
	return env, errors.Trace(err)
}
