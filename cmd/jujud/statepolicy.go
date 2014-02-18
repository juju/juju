// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state"
)

// environStatePolicy implements state.Policy
// in terms of environs.Environ and related
// types.
type environStatePolicy struct{}

var _ state.Policy = environStatePolicy{}

func (environStatePolicy) Prechecker(cfg *config.Config) (state.Prechecker, error) {
	env, err := environs.New(cfg)
	if err != nil {
		return nil, err
	}
	p, _ := env.(state.Prechecker)
	return p, nil
}
