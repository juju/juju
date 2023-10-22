// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"context"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/status"
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
	Status              status.Status
	Message             string
	ID                  string
	Error               error
}

var notSupported = errors.NotSupportedf("secret backends on this juju version")

// ListSecretBackends lists the specified secret backends, or all available if no names are provided.
func (api *Client) ListSecretBackends(names []string, reveal bool) ([]SecretBackend, error) {
	if api.BestAPIVersion() < 1 {
		return nil, notSupported
	}

	var response params.ListSecretBackendsResults
	err := api.facade.FacadeCall(context.TODO(), "ListSecretBackends", params.ListSecretBackendsArgs{Names: names, Reveal: reveal}, &response)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]SecretBackend, len(response.Results))
	for i, r := range response.Results {
		var resultErr error
		if r.Error != nil {
			resultErr = r.Error
		}
		details := SecretBackend{
			Name:                r.Result.Name,
			BackendType:         r.Result.BackendType,
			TokenRotateInterval: r.Result.TokenRotateInterval,
			Config:              r.Result.Config,
			NumSecrets:          r.NumSecrets,
			Status:              status.Status(r.Status),
			Message:             r.Message,
			ID:                  r.ID,
			Error:               resultErr,
		}
		result[i] = details
	}
	return result, err
}

// CreateSecretBackend holds details for creating a secret backend.
type CreateSecretBackend struct {
	ID                  string
	Name                string
	BackendType         string
	TokenRotateInterval *time.Duration
	Config              map[string]interface{}
}

// AddSecretBackend adds the specified secret backend.
func (api *Client) AddSecretBackend(backend CreateSecretBackend) error {
	if api.BestAPIVersion() < 1 {
		return notSupported
	}

	var results params.ErrorResults
	args := params.AddSecretBackendArgs{
		Args: []params.AddSecretBackendArg{{
			ID: backend.ID,
			SecretBackend: params.SecretBackend{
				Name:                backend.Name,
				TokenRotateInterval: backend.TokenRotateInterval,
				BackendType:         backend.BackendType,
				Config:              backend.Config,
			},
		}},
	}
	err := api.facade.FacadeCall(context.TODO(), "AddSecretBackends", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// UpdateSecretBackend holds details for updating a secret backend.
type UpdateSecretBackend struct {
	Name                string
	NameChange          *string
	TokenRotateInterval *time.Duration
	Config              map[string]interface{}
	Reset               []string
}

// UpdateSecretBackend updates the specified secret backend.
func (api *Client) UpdateSecretBackend(arg UpdateSecretBackend, force bool) error {
	if api.BestAPIVersion() < 1 {
		return notSupported
	}

	var results params.ErrorResults
	args := params.UpdateSecretBackendArgs{
		Args: []params.UpdateSecretBackendArg{{
			Name:                arg.Name,
			NameChange:          arg.NameChange,
			TokenRotateInterval: arg.TokenRotateInterval,
			Config:              arg.Config,
			Reset:               arg.Reset,
			Force:               force,
		}},
	}
	err := api.facade.FacadeCall(context.TODO(), "UpdateSecretBackends", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// RemoveSecretBackend removes the specified secret backend.
func (api *Client) RemoveSecretBackend(name string, force bool) error {
	if api.BestAPIVersion() < 1 {
		return notSupported
	}

	var results params.ErrorResults
	args := params.RemoveSecretBackendArgs{
		Args: []params.RemoveSecretBackendArg{{
			Name:  name,
			Force: force,
		}},
	}
	err := api.facade.FacadeCall(context.TODO(), "RemoveSecretBackends", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return params.TranslateWellKnownError(results.OneError())
}
