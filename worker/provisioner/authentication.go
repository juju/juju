// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"fmt"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/utils"
)

type TaggedPasswordChanger interface {
	SetPassword(string) error
	Tag() string
}

// AuthenticationProvider defines the single method that the provisioner
// task needs to set up authentication for a machine.
type AuthenticationProvider interface {
	SetupAuthentication(machine TaggedPasswordChanger) (*state.Info, *api.Info, error)
}

// NewSimpleAuthenticator gets the state and api info once from the environ.
func NewSimpleAuthenticator(environ environs.Environ) (AuthenticationProvider, error) {
	stateInfo, apiInfo, err := environ.StateInfo()
	if err != nil {
		return nil, err
	}
	return &simpleAuth{stateInfo, apiInfo}, nil
}

type simpleAuth struct {
	stateInfo *state.Info
	apiInfo   *api.Info
}

func (auth *simpleAuth) SetupAuthentication(machine TaggedPasswordChanger) (*state.Info, *api.Info, error) {
	password, err := utils.RandomPassword()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot make password for machine %v: %v", machine, err)
	}
	if err := machine.SetPassword(password); err != nil {
		return nil, nil, fmt.Errorf("cannot set API password for machine %v: %v", machine, err)
	}
	stateInfo := *auth.stateInfo
	stateInfo.Tag = machine.Tag()
	stateInfo.Password = password
	apiInfo := *auth.apiInfo
	apiInfo.Tag = machine.Tag()
	apiInfo.Password = password
	return &stateInfo, &apiInfo, nil
}
