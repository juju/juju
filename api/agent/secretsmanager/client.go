// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
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
func (c *Client) Create(cfg *secrets.SecretConfig, ownerTag names.Tag, value secrets.SecretValue) (*secrets.URI, error) {
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
		return nil, errors.Trace(err)
	}
	if n := len(results.Results); n != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", n)
	}
	if err := results.Results[0].Error; err != nil {
		return nil, errors.Trace(err)
	}
	uri, err := secrets.ParseURI(results.Results[0].Result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return uri, nil
}

// GetValue returns the value of a secret.
func (c *Client) GetValue(uri *secrets.URI, label string, update, peek bool) (secrets.SecretValue, error) {
	arg := params.GetSecretValueArg{
		Label:  label,
		Update: update,
		Peek:   peek,
	}
	arg.URI = uri.String()

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
	LatestRevision int
	Label          string
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
			LatestRevision: latest.Revision,
			Label:          latest.Label,
		}
	}
	return info, err
}

// SecretMetadata returns metadata for the caller's secrets.
func (c *Client) SecretMetadata() ([]secrets.SecretMetadata, error) {
	var results params.ListSecretResults
	err := c.facade.FacadeCall("GetSecretMetadata", nil, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result []secrets.SecretMetadata
	for _, info := range results.Results {
		uri, err := secrets.ParseURI(info.URI)
		if err != nil {
			return nil, errors.NotValidf("secret URI %q", info.URI)
		}
		result = append(result, secrets.SecretMetadata{
			URI:              uri,
			Description:      info.Description,
			Label:            info.Label,
			RotatePolicy:     secrets.RotatePolicy(info.RotatePolicy),
			LatestRevision:   info.LatestRevision,
			LatestExpireTime: info.LatestExpireTime,
			NextRotateTime:   info.NextRotateTime,
		})
	}
	return result, nil
}

// WatchSecretsRotationChanges returns a watcher which serves changes to
// secrets rotation config for any secrets managed by the specified owner.
func (c *Client) WatchSecretsRotationChanges(ownerTag string) (watcher.SecretTriggerWatcher, error) {
	var results params.SecretTriggerWatchResults
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

// SecretRotated records the outcome of rotating a secret.
func (c *Client) SecretRotated(uri string, oldRevision int) error {
	secretUri, err := secrets.ParseURI(uri)
	if err != nil {
		return errors.Trace(err)
	}

	var results params.ErrorResults
	args := params.SecretRotatedArgs{
		Args: []params.SecretRotatedArg{{
			URI:              secretUri.String(),
			OriginalRevision: oldRevision,
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
