// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package serveradmin

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.api.serveradmin")

// Client provides methods that the Juju client command uses for administration
// tasks on the Juju Server itself.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new `Client` based on an existing authenticated API
// connection.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "ServerAdmin")
	return &Client{ClientFacade: frontend, facade: backend}
}

// IdentityProvider returns the remote identity provider trusted by the Juju
// server. If an identity provider is not trusted by the server, returns nil.
func (c *Client) IdentityProvider() (*params.IdentityProviderInfo, error) {
	var result params.IdentityProviderResult
	err := c.facade.FacadeCall("IdentityProvider", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result.IdentityProvider, nil
}

// SetIdentityProvider sets the remote identity provider that should be trusted
// by the Juju server, replacing any prior trusted identity provider.
func (c *Client) SetIdentityProvider(publicKey, location string) error {
	args := params.SetIdentityProvider{
		IdentityProvider: &params.IdentityProviderInfo{
			PublicKey: publicKey,
			Location:  location,
		},
	}
	err := c.facade.FacadeCall("SetIdentityProvider", args, nil)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
