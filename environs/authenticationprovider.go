// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
	apiprovisioner "launchpad.net/juju-core/state/api/provisioner"
	"launchpad.net/juju-core/utils"
)

// TaggedPasswordChanger defines an interface for a entity with a
// Tag() and SetPassword() methods.
type TaggedPasswordChanger interface {
	SetPassword(string) error
	Tag() string
}

// AuthenticationProvider defines the single method that the provisioner
// task needs to set up authentication for a machine.
type AuthenticationProvider interface {
	SetupAuthentication(machine TaggedPasswordChanger) (*state.Info, *api.Info, error)
}

// NewEnvironAuthenticator gets the state and api info once from the environ.
func NewEnvironAuthenticator(environ Environ) (AuthenticationProvider, error) {
	stateInfo, apiInfo, err := environ.StateInfo()
	if err != nil {
		return nil, err
	}
	return &simpleAuth{stateInfo, apiInfo}, nil
}

// NewAPIAuthenticator gets the state and api info once from the
// provisioner API.
func NewAPIAuthenticator(st *apiprovisioner.State) (AuthenticationProvider, error) {
	stateAddresses, err := st.StateAddresses()
	if err != nil {
		return nil, err
	}
	apiAddresses, err := st.APIAddresses()
	if err != nil {
		return nil, err
	}
	caCert, err := st.CACert()
	if err != nil {
		return nil, err
	}
	stateInfo := &state.Info{
		Addrs:  stateAddresses,
		CACert: caCert,
	}
	apiInfo := &api.Info{
		Addrs:  apiAddresses,
		CACert: caCert,
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
