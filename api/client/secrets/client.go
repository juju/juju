// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
)

// Client is the api client for the Secrets facade.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a secrets api client.
func NewClient(caller base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(caller, "Secrets")
	return &Client{ClientFacade: frontend, facade: backend}
}

// SecretDetails holds a secret metadata and value.
type SecretDetails struct {
	Metadata secrets.SecretMetadata
	Value    secrets.SecretValue
	Error    string
}

// ListSecrets lists the available secrets.
func (api *Client) ListSecrets(showSecrets bool) ([]SecretDetails, error) {
	arg := params.ListSecretsArgs{
		ShowSecrets: showSecrets,
	}
	var response params.ListSecretResults
	err := api.facade.FacadeCall("ListSecrets", arg, &response)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]SecretDetails, len(response.Results))
	for i, r := range response.Results {
		details := SecretDetails{
			Metadata: secrets.SecretMetadata{
				Version:        r.Version,
				OwnerTag:       r.OwnerTag,
				Provider:       r.Provider,
				ProviderID:     r.ProviderID,
				Description:    r.Description,
				Label:          r.Label,
				RotatePolicy:   secrets.RotatePolicy(r.RotatePolicy),
				NextRotateTime: r.NextRotateTime,
				ExpireTime:     r.ExpireTime,
				Revision:       r.Revision,
				CreateTime:     r.CreateTime,
				UpdateTime:     r.UpdateTime,
			},
		}
		uri, err := secrets.ParseURI(r.URI)
		if err == nil {
			details.Metadata.URI = uri
		} else {
			details.Error = err.Error()
		}
		if showSecrets && r.Value != nil {
			if r.Value.Error == nil {
				details.Value = secrets.NewSecretValue(r.Value.Data)
			} else {
				details.Error = r.Value.Error.Error()
			}
		}
		result[i] = details
	}
	return result, err
}
