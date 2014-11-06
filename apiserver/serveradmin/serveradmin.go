// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package serveradmin

import (
	"encoding/base64"
	"fmt"
	"net/url"

	"github.com/juju/loggo"
	"github.com/juju/macaroon/bakery"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.serveradmin")

func init() {
	common.RegisterStandardFacade("ServerAdmin", 0, NewServerAdminAPI)
}

// ServerAdmin defines the methods on the serveradmin API end point.
type ServerAdmin interface {

	// IdentityProvider returns the identity provider trusted by the Juju state
	// server, if any.
	IdentityProvider() (params.IdentityProviderResult, error)

	// SetIdentityProvider sets the identity provider that the Juju state
	// server should trust.
	SetIdentityProvider(args params.SetIdentityProvider) error
}

// ServerAdminAPI implements the Juju server admin interface and is the
// concrete implementation of the api end point.
type ServerAdminAPI struct {
	state      *state.State
	authorizer common.Authorizer
}

// NewServerAdminAPI returns a new ServerAdminAPI instance.
func NewServerAdminAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*ServerAdminAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &ServerAdminAPI{
		state:      st,
		authorizer: authorizer,
	}, nil
}

func newIdentityProviderParams(idp *state.IdentityProvider) *params.IdentityProviderInfo {
	return &params.IdentityProviderInfo{
		PublicKey: base64.URLEncoding.EncodeToString(idp.PublicKey[:]),
		Location:  idp.Location,
	}
}

func newIdentityProviderState(info *params.IdentityProviderInfo) (*state.IdentityProvider, error) {
	pkBytes, err := base64.URLEncoding.DecodeString(info.PublicKey)
	if err != nil {
		return nil, err
	}
	if len(pkBytes) != bakery.KeyLen {
		return nil, fmt.Errorf("invalid public key length")
	}

	u, err := url.Parse(info.Location)
	if err != nil {
		return nil, err
	}

	result := &state.IdentityProvider{
		Location: u.String(),
	}
	copy(result.PublicKey[:], pkBytes)
	return result, nil
}

func (api *ServerAdminAPI) IdentityProvider() (params.IdentityProviderResult, error) {
	info, err := api.state.StateServingInfo()
	if err != nil {
		return params.IdentityProviderResult{}, err
	}
	if info.IdentityProvider == nil {
		return params.IdentityProviderResult{}, nil
	}
	return params.IdentityProviderResult{
		IdentityProvider: newIdentityProviderParams(info.IdentityProvider),
	}, nil
}

func (api *ServerAdminAPI) SetIdentityProvider(args params.SetIdentityProvider) error {
	info, err := api.state.StateServingInfo()
	if err != nil {
		return err
	}
	if args.IdentityProvider == nil {
		info.IdentityProvider = nil
	} else {
		info.IdentityProvider, err = newIdentityProviderState(args.IdentityProvider)
		if err != nil {
			return err
		}
	}
	if info.TargetKeyPair == nil {
		info.TargetKeyPair, err = bakery.GenerateKey()
		if err != nil {
			return err
		}
	}
	return api.state.SetStateServingInfo(info)
}
