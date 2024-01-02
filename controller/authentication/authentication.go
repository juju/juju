// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/api"
	apiprovisioner "github.com/juju/juju/api/agent/provisioner"
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
	SetupAuthentication(machine TaggedPasswordChanger) (*api.Info, error)
}

// NewAPIAuthenticator gets the state and api info once from the
// provisioner API.
func NewAPIAuthenticator(st *apiprovisioner.State) (AuthenticationProvider, error) {
	apiAddresses, err := st.APIAddresses()
	if err != nil {
		return nil, errors.Trace(err)
	}
	caCert, err := st.CACert()
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelUUID, err := st.ModelUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	apiInfo := &api.Info{
		Addrs:    apiAddresses,
		CACert:   caCert,
		ModelTag: names.NewModelTag(modelUUID),
	}
	return &simpleAuth{apiInfo}, nil
}

// SetupAuthentication generates a random password for the given machine,
// recording it via the machine's SetPassword method, and updates the
// info arguments with the tag and password.
func SetupAuthentication(
	machine TaggedPasswordChanger,
	apiInfo *api.Info,
) (*api.Info, error) {
	auth := simpleAuth{apiInfo}
	return auth.SetupAuthentication(machine)
}

type simpleAuth struct {
	apiInfo *api.Info
}

// SetupAuthentication implements AuthenticationProvider.
func (auth *simpleAuth) SetupAuthentication(machine TaggedPasswordChanger) (*api.Info, error) {
	password, err := utils.RandomPassword()
	if err != nil {
		return nil, fmt.Errorf("cannot make password for machine %v: %v", machine, err)
	}
	if err := machine.SetPassword(password); err != nil {
		return nil, fmt.Errorf("cannot set API password for machine %v: %v", machine, err)
	}
	var apiInfo *api.Info
	if auth.apiInfo != nil {
		apiInfoCopy := *auth.apiInfo
		apiInfo = &apiInfoCopy
		apiInfo.Tag = machine.Tag()
		apiInfo.Password = password
	}
	return apiInfo, nil
}
