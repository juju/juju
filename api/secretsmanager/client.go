// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
)

// Client is the api client for the SecretsManager facade.
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
func (c *Client) Create(cfg *secrets.SecretConfig, secretType secrets.SecretType, value secrets.SecretValue) (string, error) {
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
			Type:           string(secretType),
			Path:           cfg.Path,
			RotateInterval: cfg.RotateInterval,
			Params:         cfg.Params,
			Data:           data,
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

// Update updates an existing secret value and/or config like rotate interval.
func (c *Client) Update(URL *secrets.URL, cfg *secrets.SecretConfig, value secrets.SecretValue) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", errors.Trace(err)
	}

	var data secrets.SecretData
	if value != nil {
		data = value.EncodedValues()
		if len(data) == 0 {
			data = nil
		}
	}

	var results params.StringResults

	if err := c.facade.FacadeCall("UpdateSecrets", params.UpdateSecretArgs{
		Args: []params.UpdateSecretArg{{
			URL:            URL.ID(),
			RotateInterval: cfg.RotateInterval,
			Params:         cfg.Params,
			Data:           data,
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

// WatchSecretsRotationChanges returns a watcher which serves changes to
// secrets rotation config for any secrets managed by the specified owner.
func (c *Client) WatchSecretsRotationChanges(ownerTag string) (watcher.SecretRotationWatcher, error) {
	var results params.SecretRotationWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: ownerTag}},
	}
	err := c.facade.FacadeCall("WatchSecretsRotationChanges", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewSecretsRotationWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}
