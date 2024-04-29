// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	coresecrets "github.com/juju/juju/core/secrets"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/rpc/params"
)

// Client is the api client for accessing secret backend related facades.
type Client struct {
	facade base.FacadeCaller
}

// NewClient creates a secrets api client.
func NewClient(facade base.FacadeCaller) *Client {
	return &Client{facade: facade}
}

// GetSecretBackendConfig fetches the config needed to make a secret backend client.
// If backendID is nil, fetch the current active backend config.
func (c *Client) GetSecretBackendConfig(backendID *string) (*provider.ModelBackendConfigInfo, error) {
	var results params.SecretBackendConfigResults

	args := params.SecretBackendArgs{}
	if backendID != nil {
		args.BackendIDs = []string{*backendID}
	}
	err := c.facade.FacadeCall(context.TODO(), "GetSecretBackendConfigs", args, &results)
	if err != nil && !errors.Is(params.TranslateWellKnownError(err), secretbackenderrors.NotFound) {
		return nil, errors.Trace(err)
	}
	if err != nil || len(results.Results) == 0 {
		msg := "active secret backend"
		if backendID != nil {
			msg = fmt.Sprintf("external secret backend id %q", *backendID)
		}
		return nil, fmt.Errorf("%s%w", msg, errors.Hide(secretbackenderrors.NotFound))
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	info := &provider.ModelBackendConfigInfo{
		ActiveID: results.ActiveID,
		Configs:  make(map[string]provider.ModelBackendConfig),
	}
	for id, cfg := range results.Results {
		info.Configs[id] = provider.ModelBackendConfig{
			ControllerUUID: cfg.ControllerUUID,
			ModelUUID:      cfg.ModelUUID,
			ModelName:      cfg.ModelName,
			BackendConfig: provider.BackendConfig{
				BackendType: cfg.Config.BackendType,
				Config:      cfg.Config.Params,
			},
		}
	}
	return info, nil
}

// GetBackendConfigForDrain fetches the config needed to make a secret backend client for the drain worker.
func (c *Client) GetBackendConfigForDrain(backendID *string) (*provider.ModelBackendConfig, string, error) {
	var result params.SecretBackendConfigResults
	arg := params.SecretBackendArgs{ForDrain: true}
	if backendID != nil {
		arg.BackendIDs = []string{*backendID}
	}
	err := c.facade.FacadeCall(context.TODO(), "GetSecretBackendConfigs", arg, &result)
	if err != nil && !errors.Is(params.TranslateWellKnownError(err), secretbackenderrors.NotFound) {
		return nil, "", errors.Trace(err)
	}
	if len(result.Results) == 0 {
		return nil, "", errors.NotFoundf("no secret backends available")
	}

	for _, cfg := range result.Results {
		return &provider.ModelBackendConfig{
			ControllerUUID: cfg.ControllerUUID,
			ModelUUID:      cfg.ModelUUID,
			ModelName:      cfg.ModelName,
			BackendConfig: provider.BackendConfig{
				BackendType: cfg.Config.BackendType,
				Config:      cfg.Config.Params,
			},
		}, result.ActiveID, nil
	}
	if backendID != nil {
		return nil, "", errors.NotFoundf("secret backend %q", *backendID)
	}
	return nil, "", errors.NotFoundf("active secret backend")
}

// GetContentInfo returns info about the content of a secret.
func (c *Client) GetContentInfo(uri *coresecrets.URI, label string, refresh, peek bool) (*secrets.ContentParams, *provider.ModelBackendConfig, bool, error) {
	arg := params.GetSecretContentArg{
		Label:   label,
		Refresh: refresh,
		Peek:    peek,
	}
	if uri != nil {
		arg.URI = uri.String()
	}

	var results params.SecretContentResults

	if err := c.facade.FacadeCall(
		context.TODO(),
		"GetSecretContentInfo", params.GetSecretContentArgs{Args: []params.GetSecretContentArg{arg}}, &results,
	); err != nil {
		return nil, nil, false, errors.Trace(err)
	}
	return c.processSecretContentResults(results)
}

func (c *Client) processSecretContentResults(results params.SecretContentResults) (*secrets.ContentParams, *provider.ModelBackendConfig, bool, error) {
	if n := len(results.Results); n != 1 {
		return nil, nil, false, errors.Errorf("expected 1 result, got %d", n)
	}

	if err := results.Results[0].Error; err != nil {
		return nil, nil, false, apiservererrors.RestoreError(err)
	}
	content := &secrets.ContentParams{}
	var (
		backendConfig *provider.ModelBackendConfig
		draining      bool
	)
	result := results.Results[0]
	contentParams := results.Results[0].Content
	if contentParams.ValueRef != nil {
		content.ValueRef = &coresecrets.ValueRef{
			BackendID:  contentParams.ValueRef.BackendID,
			RevisionID: contentParams.ValueRef.RevisionID,
		}
		if result.BackendConfig == nil {
			return nil, nil, false, errors.Errorf("missing secret backend info for %q", content.ValueRef)
		}
		backendConfig = &provider.ModelBackendConfig{
			ControllerUUID: result.BackendConfig.ControllerUUID,
			ModelUUID:      result.BackendConfig.ModelUUID,
			ModelName:      result.BackendConfig.ModelName,
			BackendConfig: provider.BackendConfig{
				BackendType: result.BackendConfig.Config.BackendType,
				Config:      result.BackendConfig.Config.Params,
			},
		}
		draining = result.BackendConfig.Draining
	}
	if len(contentParams.Data) > 0 {
		content.SecretValue = coresecrets.NewSecretValue(contentParams.Data)
	}
	return content, backendConfig, draining, nil
}

// GetRevisionContentInfo returns info about the content of a secret revision.
// If pendingDelete is true, the revision is marked for deletion.
func (c *Client) GetRevisionContentInfo(uri *coresecrets.URI, revision int, pendingDelete bool) (*secrets.ContentParams, *provider.ModelBackendConfig, bool, error) {
	arg := params.SecretRevisionArg{
		URI:           uri.String(),
		Revisions:     []int{revision},
		PendingDelete: pendingDelete,
	}

	var results params.SecretContentResults

	if err := c.facade.FacadeCall(
		context.TODO(),
		"GetSecretRevisionContentInfo", arg, &results,
	); err != nil {
		return nil, nil, false, errors.Trace(err)
	}
	return c.processSecretContentResults(results)
}
