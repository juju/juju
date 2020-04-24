// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
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

// AuthenticationProvider defines the single method that the provisioner
// task needs to set up authentication for a machine.
type AuthenticationProvider interface {
	SetupAuthentication(machine TaggedPasswordChanger) (*mongo.MongoInfo, *api.Info, error)
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
	var stateInfo *mongo.MongoInfo
	stateAddresses, err := st.StateAddresses()
	// We may not have stateAddresses (we don't on K8s), but the common case is that we don't need them
	//  (we only need them for controller machines).
	if err == nil {
		stateInfo = &mongo.MongoInfo{
			Info: mongo.Info{
				Addrs:  stateAddresses,
				CACert: caCert,
			},
		}
	}
	apiInfo := &api.Info{
		Addrs:    apiAddresses,
		CACert:   caCert,
		ModelTag: names.NewModelTag(modelUUID),
	}
	return &simpleAuth{stateInfo, apiInfo}, nil
}

// SetupAuthentication generates a random password for the given machine,
// recording it via the machine's SetPassword method, and updates the
// info arguments with the tag and password.
func SetupAuthentication(
	machine TaggedPasswordChanger,
	stateInfo *mongo.MongoInfo,
	apiInfo *api.Info,
) (*mongo.MongoInfo, *api.Info, error) {
	auth := simpleAuth{stateInfo, apiInfo}
	return auth.SetupAuthentication(machine)
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
	var stateInfo *mongo.MongoInfo
	if auth.stateInfo != nil {
		stateInfoCopy := *auth.stateInfo
		stateInfo = &stateInfoCopy
		stateInfo.Tag = machine.Tag()
		stateInfo.Password = password
	}
	var apiInfo *api.Info
	if auth.apiInfo != nil {
		apiInfoCopy := *auth.apiInfo
		apiInfo = &apiInfoCopy
		apiInfo.Tag = machine.Tag()
		apiInfo.Password = password
	}
	return stateInfo, apiInfo, nil
}
