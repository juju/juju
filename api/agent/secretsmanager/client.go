// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	apiservererrors "github.com/juju/juju/apiserver/errors"
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
func (c *Client) Create(cfg *secrets.SecretConfig, ownerTag names.Tag, value secrets.SecretValue) (string, error) {
	var data secrets.SecretData
	if value != nil {
		data = value.EncodedValues()
	}

	var results params.StringResults

	arg := params.CreateSecretArg{
		OwnerTag: ownerTag.String(),
		UpsertSecretArg: params.UpsertSecretArg{
			RotatePolicy: cfg.RotatePolicy,
			ExpireTime:   cfg.ExpireTime,
			Description:  cfg.Description,
			Label:        cfg.Label,
			Params:       cfg.Params,
			Data:         data,
		},
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
func (c *Client) Update(uri string, cfg *secrets.SecretConfig, value secrets.SecretValue) error {
	secretUri, err := secrets.ParseURI(uri)
	if err != nil {
		return errors.Trace(err)
	}

	var data secrets.SecretData
	if value != nil {
		data = value.EncodedValues()
		if len(data) == 0 {
			data = nil
		}
	}

	var results params.ErrorResults

	arg := params.UpdateSecretArg{
		URI: secretUri.String(),
		UpsertSecretArg: params.UpsertSecretArg{
			RotatePolicy: cfg.RotatePolicy,
			ExpireTime:   cfg.ExpireTime,
			Description:  cfg.Description,
			Label:        cfg.Label,
			Params:       cfg.Params,
			Data:         data,
		},
	}
	if err := c.facade.FacadeCall("UpdateSecrets", params.UpdateSecretArgs{
		Args: []params.UpdateSecretArg{arg},
	}, &results); err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// Remove removes the specified secret.
func (c *Client) Remove(uri string) error {
	secretUri, err := secrets.ParseURI(uri)
	if err != nil {
		return errors.Trace(err)
	}

	args := params.SecretURIArgs{
		Args: []params.SecretURIArg{{URI: secretUri.String()}},
	}
	var results params.ErrorResults
	err = c.facade.FacadeCall("RemoveSecrets", args, &results)
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

// GetValue returns the value of a secret.
func (c *Client) GetValue(uri, label string, update, peek bool) (secrets.SecretValue, error) {
	arg := params.GetSecretValueArg{
		Label:  label,
		Update: update,
		Peek:   peek,
	}
	secretUri, err := secrets.ParseURI(uri)
	if err != nil {
		return nil, errors.Trace(err)
	}
	arg.URI = secretUri.String()

	var results params.SecretValueResults

	if err := c.facade.FacadeCall("GetSecretValues", params.GetSecretValueArgs{
		Args: []params.GetSecretValueArg{arg},
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

// WatchSecretsChanges returns a watcher which serves changes to
// secrets payloads for any secrets consumed by the specified unit.
func (c *Client) WatchSecretsChanges(unitName string) (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: names.NewUnitTag(unitName).String()}},
	}
	err := c.facade.FacadeCall("WatchSecretsChanges", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, apiservererrors.RestoreError(result.Error)
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// SecretRevisionInfo holds info used to read a secret vale.
type SecretRevisionInfo struct {
	Revision int
	Label    string
}

// GetLatestSecretsRevisionInfo returns the current revision and labels for secrets consumed
// by the specified unit.
func (c *Client) GetLatestSecretsRevisionInfo(unitName string, uris []string) (map[string]SecretRevisionInfo, error) {
	var results params.SecretConsumerInfoResults
	args := params.GetSecretConsumerInfoArgs{
		ConsumerTag: names.NewUnitTag(unitName).String(),
		URIs:        uris,
	}

	err := c.facade.FacadeCall("GetLatestSecretsRevisionInfo", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != len(uris) {
		return nil, errors.Errorf("expected %d result, got %d", len(uris), len(results.Results))
	}
	info := make(map[string]SecretRevisionInfo)
	for i, latest := range results.Results {
		if err := results.Results[i].Error; err != nil {
			// If deleted or now unauthorised, do not report any info for this url.
			if err.Code == params.CodeNotFound || err.Code == params.CodeUnauthorized {
				continue
			}
			return nil, errors.Annotatef(err, "finding latest info for secret %q", uris[i])
		}
		info[uris[i]] = SecretRevisionInfo{
			Revision: latest.Revision,
			Label:    latest.Label,
		}
	}
	return info, err
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
func (c *Client) SecretRotated(uri string, when time.Time) error {
	secretUri, err := secrets.ParseURI(uri)
	if err != nil {
		return errors.Trace(err)
	}

	var results params.ErrorResults
	args := params.SecretRotatedArgs{
		Args: []params.SecretRotatedArg{{
			URI:  secretUri.String(),
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
	RelationKey     *string
	Role            secrets.SecretRole
}

// Grant grants access to the specified secret.
func (c *Client) Grant(uri string, p *SecretRevokeGrantArgs) error {
	secretUri, err := secrets.ParseURI(uri)
	if err != nil {
		return errors.Trace(err)
	}

	args := grantRevokeArgsToParams(p, secretUri)
	var results params.ErrorResults
	err = c.facade.FacadeCall("SecretsGrant", args, &results)
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

func grantRevokeArgsToParams(p *SecretRevokeGrantArgs, secretUri *secrets.URI) params.GrantRevokeSecretArgs {
	var subjectTag, scopeTag string
	if p.ApplicationName != nil {
		subjectTag = names.NewApplicationTag(*p.ApplicationName).String()
	}
	if p.UnitName != nil {
		subjectTag = names.NewUnitTag(*p.UnitName).String()
	}
	if p.RelationKey != nil {
		scopeTag = names.NewRelationTag(*p.RelationKey).String()
	} else {
		scopeTag = subjectTag
	}
	args := params.GrantRevokeSecretArgs{
		Args: []params.GrantRevokeSecretArg{{
			URI:         secretUri.String(),
			ScopeTag:    scopeTag,
			SubjectTags: []string{subjectTag},
			Role:        string(p.Role),
		}},
	}
	return args
}

// Revoke revokes access to the specified secret.
func (c *Client) Revoke(uri string, p *SecretRevokeGrantArgs) error {
	secretUri, err := secrets.ParseURI(uri)
	if err != nil {
		return errors.Trace(err)
	}

	args := grantRevokeArgsToParams(p, secretUri)
	var results params.ErrorResults
	err = c.facade.FacadeCall("SecretsRevoke", args, &results)
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
