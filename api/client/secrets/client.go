// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	secretsservice "github.com/juju/juju/secrets"
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
	Metadata  secrets.SecretMetadata
	Revisions []secrets.SecretRevisionMetadata
	Value     secrets.SecretValue
	Error     string
}

// ListSecrets lists the available secrets.
func (api *Client) ListSecrets(reveal bool, filter secretsservice.Filter) ([]SecretDetails, error) {
	arg := params.ListSecretsArgs{
		ShowSecrets: reveal,
		Filter: params.SecretsFilter{
			OwnerTag: filter.OwnerTag,
			Revision: filter.Revision,
		},
	}
	if filter.URI != nil {
		uri := filter.URI.String()
		arg.Filter.URI = &uri
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
				Version:          r.Version,
				OwnerTag:         r.OwnerTag,
				RotatePolicy:     secrets.RotatePolicy(r.RotatePolicy),
				NextRotateTime:   r.NextRotateTime,
				LatestRevision:   r.LatestRevision,
				LatestExpireTime: r.LatestExpireTime,
				Description:      r.Description,
				Label:            r.Label,
				ProviderID:       r.ProviderID,
				CreateTime:       r.CreateTime,
				UpdateTime:       r.UpdateTime,
			},
		}
		uri, err := secrets.ParseURI(r.URI)
		if err == nil {
			details.Metadata.URI = uri
		} else {
			details.Error = err.Error()
		}
		details.Revisions = make([]secrets.SecretRevisionMetadata, len(r.Revisions))
		for i, r := range r.Revisions {
			details.Revisions[i] = secrets.SecretRevisionMetadata{
				Revision:   r.Revision,
				CreateTime: r.CreateTime,
				UpdateTime: r.UpdateTime,
				ExpireTime: r.ExpireTime,
			}
		}
		if reveal && r.Value != nil {
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
