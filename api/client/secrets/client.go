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
	Metadata  secrets.SecretMetadata
	Access    []secrets.AccessInfo
	Revisions []secrets.SecretRevisionMetadata
	Value     secrets.SecretValue
	Error     string
}

func toGrantInfo(grants []params.AccessInfo) []secrets.AccessInfo {
	result := make([]secrets.AccessInfo, len(grants))
	for i, g := range grants {
		result[i] = secrets.AccessInfo{
			Target: g.TargetTag,
			Scope:  g.ScopeTag,
			Role:   g.Role,
		}
	}
	return result
}

// ListSecrets lists the available secrets.
func (api *Client) ListSecrets(reveal bool, filter secrets.Filter) ([]SecretDetails, error) {
	arg := params.ListSecretsArgs{
		ShowSecrets: reveal,
		Filter: params.SecretsFilter{
			OwnerTag: filter.OwnerTag,
			Revision: filter.Revision,
			Label:    filter.Label,
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
				CreateTime:       r.CreateTime,
				UpdateTime:       r.UpdateTime,
			},
			Access: toGrantInfo(r.Access),
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
				Revision:    r.Revision,
				BackendName: r.BackendName,
				CreateTime:  r.CreateTime,
				UpdateTime:  r.UpdateTime,
				ExpireTime:  r.ExpireTime,
			}
		}
		if reveal && r.Value != nil {
			if r.Value.Error == nil {
				if data := secrets.NewSecretValue(r.Value.Data); !data.IsEmpty() {
					details.Value = data
				}
			} else {
				details.Error = r.Value.Error.Error()
			}
		}
		result[i] = details
	}
	return result, err
}

func (c *Client) CreateSecret(name, description string, data map[string]string) (string, error) {
	if c.BestAPIVersion() < 2 {
		return "", errors.NotSupportedf("user secrets")
	}
	var results params.StringResults
	arg := params.CreateSecretArg{
		UpsertSecretArg: params.UpsertSecretArg{
			Content: params.SecretContentParams{Data: data},
		},
	}
	if name != "" {
		arg.Label = &name
	}
	if description != "" {
		arg.Description = &description
	}

	err := c.facade.FacadeCall("CreateSecrets", params.CreateSecretArgs{Args: []params.CreateSecretArg{arg}}, &results)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return "", errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return "", params.TranslateWellKnownError(result.Error)
	}
	return result.Result, nil
}

// UpdateSecret updates an existing secret.
func (c *Client) UpdateSecret(
	uri *secrets.URI, name string, autoPrune *bool,
	newName string, description string, data map[string]string,
) error {
	if c.BestAPIVersion() < 2 {
		return errors.NotSupportedf("user secrets")
	}
	var results params.ErrorResults
	arg := params.UpdateUserSecretArg{
		AutoPrune: autoPrune,
		UpsertSecretArg: params.UpsertSecretArg{
			Content: params.SecretContentParams{Data: data},
		},
	}
	if uri == nil && name == "" {
		return errors.New("must specify either URI or name")
	}
	if uri != nil && name != "" {
		return errors.New("must specify either URI or name but not both")
	}
	if uri != nil {
		arg.URI = uri.String()
	}
	if name != "" {
		arg.ExistingLabel = name
	}
	if newName != "" {
		arg.UpsertSecretArg.Label = &newName
	}
	if description != "" {
		arg.UpsertSecretArg.Description = &description
	}
	err := c.facade.FacadeCall("UpdateSecrets", params.UpdateUserSecretArgs{Args: []params.UpdateUserSecretArg{arg}}, &results)
	if err != nil {
		return errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return params.TranslateWellKnownError(result.Error)
	}
	return nil
}

func (c *Client) RemoveSecret(uri *secrets.URI, name string, revision *int) error {
	if c.BestAPIVersion() < 2 {
		return errors.NotSupportedf("user secrets")
	}
	arg := params.DeleteSecretArg{
		URI:   uri.String(),
		Label: name,
	}
	if revision != nil {
		arg.Revisions = append(arg.Revisions, *revision)
	}

	var results params.ErrorResults
	err := c.facade.FacadeCall("RemoveSecrets", params.DeleteSecretArgs{Args: []params.DeleteSecretArg{arg}}, &results)
	if err != nil {
		return errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return params.TranslateWellKnownError(result.Error)
	}
	return nil
}

// GrantSecret grants access to a secret to the specified applications.
func (c *Client) GrantSecret(uri *secrets.URI, name string, apps []string) ([]error, error) {
	if c.BestAPIVersion() < 2 {
		return nil, errors.NotSupportedf("user secrets")
	}
	var uriString string
	if uri != nil {
		uriString = uri.String()
	}
	arg := params.GrantRevokeUserSecretArg{
		URI:          uriString,
		Label:        name,
		Applications: apps,
	}

	var results params.ErrorResults
	err := c.facade.FacadeCall("GrantSecret", arg, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(apps) {
		return nil, errors.Errorf("expected %d results, got %d", len(apps), len(results.Results))
	}
	return processErrors(results), nil
}

func processErrors(results params.ErrorResults) []error {
	errors := make([]error, len(results.Results))
	for i, result := range results.Results {
		if result.Error != nil {
			errors[i] = params.TranslateWellKnownError(result.Error)
		} else {
			errors[i] = nil
		}
	}
	return errors
}

// RevokeSecret revokes access to a secret from the specified applications.
func (c *Client) RevokeSecret(uri *secrets.URI, name string, apps []string) ([]error, error) {
	if c.BestAPIVersion() < 2 {
		return nil, errors.NotSupportedf("user secrets")
	}

	var uriString string
	if uri != nil {
		uriString = uri.String()
	}
	arg := params.GrantRevokeUserSecretArg{
		URI:          uriString,
		Label:        name,
		Applications: apps,
	}

	var results params.ErrorResults
	err := c.facade.FacadeCall("RevokeSecret", arg, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(apps) {
		return nil, errors.Errorf("expected %d results, got %d", len(apps), len(results.Results))
	}
	return processErrors(results), nil
}
