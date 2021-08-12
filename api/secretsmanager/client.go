// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/secrets"
)

// Client is the api client for the Secrets facade.
type Client struct {
	facade base.FacadeCaller
}

// NewClient creates a secrets api client.
func NewClient(caller base.APICaller) *Client {
	return &Client{
		facade: base.NewFacadeCaller(caller, "SecretsManager"),
	}
}

// Create creates a new secret.
func (c *Client) Create(cfg *secrets.SecretConfig, value secrets.SecretValue) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", errors.Trace(err)
	}

	var data secrets.SecretData
	if value != nil {
		data = value.EncodedValues()
	}

	var results params.StringResults

	if err := c.facade.FacadeCall("CreateSecrets", params.CreateSecretArgs{
		Args: []params.CreateSecretArg{{
			Type:   string(cfg.Type),
			Path:   cfg.Path,
			Scope:  string(cfg.Scope),
			Params: cfg.Params,
			Data:   data,
		}},
	}, &results); err != nil {
		return "", errors.Trace(err)
	}
	if n := len(results.Results); n != 1 {
		return "", errors.Errorf("expected 1 result, got %d", n)
	}
	if err := results.Results[0].Error; err != nil {
		return "", err
	}
	return results.Results[0].Result, nil
}

// GetValue returns the value of a secret.
func (c *Client) GetValue(ID string) (secrets.SecretValue, error) {
	//TODO(wallyworld) - validate ID format

	var results params.SecretValueResults

	if err := c.facade.FacadeCall("GetSecretValues", params.GetSecretArgs{
		Args: []params.GetSecretArg{{
			ID: ID,
		}},
	}, &results); err != nil {
		return nil, errors.Trace(err)
	}
	if n := len(results.Results); n != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", n)
	}

	if err := results.Results[0].Error; err != nil {
		return nil, err
	}
	return secrets.NewSecretValue(results.Results[0].Data), nil
}
