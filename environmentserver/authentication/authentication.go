// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"

	"github.com/juju/juju/api"
	apiprovisioner "github.com/juju/juju/api/provisioner"
	"github.com/juju/juju/mongo"
)

// TaggedPasswordChanger defines an interface for a entity with a
// Tag() and SetPassword() methods.
type TaggedPasswordChanger interface {
	SetPassword(string) error
	Tag() names.Tag
}

// NewAuthenticator returns a simpleAuth populated with connectionInfo and apiInfo
func NewAuthenticator(connectionInfo *mongo.MongoInfo, apiInfo *api.Info) AuthenticationProvider {
	return &simpleAuth{
		stateInfo: connectionInfo,
		apiInfo:   apiInfo,
	}
}

// AuthenticationProvider defines the single method that the provisioner
// task needs to set up authentication for a machine.
type AuthenticationProvider interface {
	SetupAuthentication(machine TaggedPasswordChanger) (*mongo.MongoInfo, *api.Info, error)
}

// NewAPIAuthenticator gets the state and api info once from the
// provisioner API.
func NewAPIAuthenticator(st *apiprovisioner.State) (AuthenticationProvider, error) {
	stateAddresses, err := st.StateAddresses()
	if err != nil {
		return nil, errors.Trace(err)
	}
	apiAddresses, err := st.APIAddresses()
	if err != nil {
		return nil, errors.Trace(err)
	}
	caCert, err := st.CACert()
	if err != nil {
		return nil, errors.Trace(err)
	}
	envUUID, err := st.EnvironUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	stateInfo := &mongo.MongoInfo{
		Info: mongo.Info{
			Addrs:  stateAddresses,
			CACert: caCert,
		},
	}
	apiInfo := &api.Info{
		Addrs:      apiAddresses,
		CACert:     caCert,
		EnvironTag: names.NewEnvironTag(envUUID),
	}
	return &simpleAuth{stateInfo, apiInfo}, nil
}

type simpleAuth struct {
	stateInfo *mongo.MongoInfo
	apiInfo   *api.Info
}

func (auth *simpleAuth) SetupAuthentication(machine TaggedPasswordChanger) (*mongo.MongoInfo, *api.Info, error) {
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
