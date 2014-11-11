// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package serveradmin

import (
	"encoding/base64"
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/macaroon/bakery"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.serveradmin")

func init() {
	common.RegisterStandardFacade("ServerAdmin", 0, NewAPI)
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

// API implements the Juju server admin interface and is the
// concrete implementation of the api end point.
type API struct {
	state      *state.State
	authorizer common.Authorizer
}

// NewAPI returns a new API instance.
func NewAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &API{
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
		return nil, errors.Trace(err)
	}
	if len(pkBytes) != bakery.KeyLen {
		return nil, errors.Errorf("invalid public key length")
	}

	u, err := url.Parse(info.Location)
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := &state.IdentityProvider{
		Location: u.String(),
	}
	copy(result.PublicKey[:], pkBytes)
	return result, nil
}

// IdentityProvider implements the ServerAdmin interface.
func (api *API) IdentityProvider() (params.IdentityProviderResult, error) {
	info, err := api.state.StateServingInfo()
	if err != nil {
		return params.IdentityProviderResult{}, errors.Trace(err)
	}
	if info.IdentityProvider == nil {
		return params.IdentityProviderResult{}, nil
	}
	return params.IdentityProviderResult{
		IdentityProvider: newIdentityProviderParams(info.IdentityProvider),
	}, nil
}

// SetIdentityProvider implements the ServerAdmin interface.
func (api *API) SetIdentityProvider(args params.SetIdentityProvider) error {
	info, err := api.state.StateServingInfo()
	if err != nil {
		return errors.Trace(err)
	}
	if args.IdentityProvider == nil {
		info.IdentityProvider = nil
	} else {
		info.IdentityProvider, err = newIdentityProviderState(args.IdentityProvider)
		if err != nil {
			return errors.Trace(err)
		}
	}
	if info.TargetKeyPair == nil {
		info.TargetKeyPair, err = bakery.GenerateKey()
		if err != nil {
			return errors.Trace(err)
		}
	}
	return api.state.SetStateServingInfo(info)
}
