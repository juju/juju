// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"

	"github.com/juju/names"
	"github.com/juju/utils"

	"github.com/juju/juju/environs/hackage"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/api"
	apiprovisioner "github.com/juju/juju/state/api/provisioner"
)

// TaggedPasswordChanger defines an interface for a entity with a
// Tag() and SetPassword() methods.
type TaggedPasswordChanger interface {
	SetPassword(string) error
	Tag() names.Tag
}

// AuthenticationProvider defines the single method that the provisioner
// task needs to set up authentication for a machine.
type AuthenticationProvider interface {
	SetupAuthentication(machine TaggedPasswordChanger) (*hackage.Info, *api.Info, error)
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
	stateInfo := &hackage.Info{
		Info: mongo.Info{
			Addrs:  stateAddresses,
			CACert: caCert,
		},
	}
	apiInfo := &api.Info{
		Addrs:  apiAddresses,
		CACert: caCert,
	}
	return &simpleAuth{stateInfo, apiInfo}, nil
}

type simpleAuth struct {
	stateInfo *hackage.Info
	apiInfo   *api.Info
}

func (auth *simpleAuth) SetupAuthentication(machine TaggedPasswordChanger) (*hackage.Info, *api.Info, error) {
	password, err := utils.RandomPassword()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot make password for machine %v: %v", machine, err)
	}
	if err := machine.SetPassword(password); err != nil {
		return nil, nil, fmt.Errorf("cannot set API password for machine %v: %v", machine, err)
	}
	stateInfo := *auth.stateInfo
	stateInfo.Tag = machine.Tag().String()
	stateInfo.Password = password
	apiInfo := *auth.apiInfo
	apiInfo.Tag = machine.Tag().String()
	apiInfo.Password = password
	return &stateInfo, &apiInfo, nil
}
