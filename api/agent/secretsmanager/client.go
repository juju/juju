// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"strings"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
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

	arg := params.CreateSecretArg{
		Type:   string(secretType),
		Path:   cfg.Path,
		Params: cfg.Params,
		Data:   data,
	}
	if cfg.Status != nil {
		arg.Status = string(*cfg.Status)
	}
	if cfg.RotateInterval != nil {
		arg.RotateInterval = *cfg.RotateInterval
	}
	if cfg.Description != nil {
		arg.Description = *cfg.Description
	}
	if cfg.Tags != nil {
		arg.Tags = *cfg.Tags
	}
	if err := c.facade.FacadeCall("CreateSecrets", params.CreateSecretArgs{
		Args: []params.CreateSecretArg{arg},
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
func (c *Client) Update(url string, cfg *secrets.SecretConfig, value secrets.SecretValue) (string, error) {
	secretUrl, err := secrets.ParseURL(url)
	if err != nil {
		return "", errors.Trace(err)
	}
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

	arg := params.UpdateSecretArg{
		URL:            secretUrl.ID(),
		RotateInterval: cfg.RotateInterval,
		Description:    cfg.Description,
		Tags:           cfg.Tags,
		Params:         cfg.Params,
		Data:           data,
	}
	if cfg.Status != nil {
		statusStr := string(*cfg.Status)
		arg.Status = &statusStr
	}
	if err := c.facade.FacadeCall("UpdateSecrets", params.UpdateSecretArgs{
		Args: []params.UpdateSecretArg{arg},
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
func (c *Client) GetValue(urlOrId string) (secrets.SecretValue, error) {
	arg := params.GetSecretArg{}
	if strings.HasPrefix(urlOrId, secrets.SecretScheme+"://") {
		secretUrl, err := secrets.ParseURL(urlOrId)
		if err != nil {
			return nil, errors.Trace(err)
		}
		arg.URL = secretUrl.ID()
	} else {
		arg.ID = urlOrId
	}

	var results params.SecretValueResults

	if err := c.facade.FacadeCall("GetSecretValues", params.GetSecretArgs{
		Args: []params.GetSecretArg{arg},
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

// SecretRotated records when a secret was last rotated.
func (c *Client) SecretRotated(url string, when time.Time) error {
	secretUrl, err := secrets.ParseURL(url)
	if err != nil {
		return errors.Trace(err)
	}

	var results params.ErrorResults
	args := params.SecretRotatedArgs{
		Args: []params.SecretRotatedArg{{
			URL:  secretUrl.ID(),
			When: when,
		}},
	}
	err = c.facade.FacadeCall("SecretsRotated", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// SecretRevokeGrantArgs holds the args used to grant or revoke access to a secret.
// To grant access, specify one of ApplicationName or UnitName, plus optionally RelationId.
// To revoke access, specify one of ApplicationName or UnitName.
type SecretRevokeGrantArgs struct {
	ApplicationName *string
	UnitName        *string
	RelationId      *int
	Role            secrets.SecretRole
}

// Grant grants access to the specified secret.
func (c *Client) Grant(url string, args *SecretRevokeGrantArgs) error {
	return errors.NotImplementedf("Grant")
}

// Revoke revokes access to the specified secret.
func (c *Client) Revoke(url string, args *SecretRevokeGrantArgs) error {
	return errors.NotImplementedf("Revoke")
}
