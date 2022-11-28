// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// Client is the api client for the SecretBackends facade.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a secret backends api client.
func NewClient(caller base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(caller, "SecretBackends")
	return &Client{ClientFacade: frontend, facade: backend}
}

// SecretBackend holds details for a secret backend.
type SecretBackend struct {
	Name                string
	Backend             string
	TokenRotateInterval time.Duration
	Config              map[string]interface{}
}

// ListSecretBackends lists the available secret backends.
func (api *Client) ListSecretBackends(reveal bool) ([]SecretBackend, error) {
	var response params.ListSecretBackendsResults
	err := api.facade.FacadeCall("ListSecretBackends", params.ListSecretBackendsArgs{Reveal: reveal}, &response)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]SecretBackend, len(response.Results))
	for i, r := range response.Results {
		details := SecretBackend{
			Name:                r.Name,
			Backend:             r.Backend,
			TokenRotateInterval: r.TokenRotateInterval,
			Config:              r.Config,
		}
		result[i] = details
	}
	return result, err
}

// AddSecretBackend adds the specified secret backend.
func (api *Client) AddSecretBackend(backend SecretBackend) error {
	var results params.ErrorResults
	args := params.AddSecretBackendArgs{
		Args: []params.SecretBackend{{
			Name:                backend.Name,
			TokenRotateInterval: backend.TokenRotateInterval,
			Backend:             backend.Backend,
			Config:              backend.Config,
		}},
	}
	err := api.facade.FacadeCall("AddSecretBackends", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}
