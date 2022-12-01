// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiservererrors "github.com/juju/juju/apiserver/errors"
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
	BackendType         string
	TokenRotateInterval *time.Duration
	Config              map[string]interface{}
	NumSecrets          int
	Error               error
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
			Name:                r.Result.Name,
			BackendType:         r.Result.BackendType,
			TokenRotateInterval: r.Result.TokenRotateInterval,
			Config:              r.Result.Config,
			NumSecrets:          r.NumSecrets,
			Error:               apiservererrors.RestoreError(r.Error),
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
			BackendType:         backend.BackendType,
			Config:              backend.Config,
		}},
	}
	err := api.facade.FacadeCall("AddSecretBackends", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}
