// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/secrets/provider"
)

// SecretsState process access to secret state.
type SecretsState interface {
	GetSecretValue(*secrets.URI, int) (secrets.SecretValue, *secrets.ValueRef, error)
}

// secretDeletionAPIClient is a [JujuAPIClient] for secrets
// which only supports GetRevisionContentInfo used during
// the process of deleting a secret.
type secretDeletionAPIClient struct {
	backendConfigGetter BackendConfigForDeleteGetter
	secretsState        SecretsState
}

func (s secretDeletionAPIClient) getBackend(backendID string) (*provider.ModelBackendConfig, bool, error) {
	cfgInfo, err := s.backendConfigGetter(backendID)
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	cfg, ok := cfgInfo.Configs[backendID]
	if ok {
		return &provider.ModelBackendConfig{
			ControllerUUID: cfg.ControllerUUID,
			ModelUUID:      cfg.ModelUUID,
			ModelName:      cfg.ModelName,
			BackendConfig: provider.BackendConfig{
				BackendType: cfg.BackendType,
				Config:      cfg.Config,
			},
		}, backendID != cfgInfo.ActiveID, nil
	}
	return nil, false, errors.NotFoundf("secret backend %q", backendID)
}

// GetRevisionContentInfo returns info about the content of a secret revision and the backend config
// needed to make a backend deletion client.
func (s secretDeletionAPIClient) GetRevisionContentInfo(ctx context.Context, uri *secrets.URI, rev int, _ bool) (*ContentParams, *provider.ModelBackendConfig, bool, error) {
	_, valueRef, err := s.secretsState.GetSecretValue(uri, rev)
	if err != nil {
		return nil, nil, false, errors.Trace(err)
	}
	var (
		backendConfig *provider.ModelBackendConfig
		draining      bool
	)
	contentParams := ContentParams{
		// We don't expose any non external value.
		SecretValue: secrets.NewSecretValue(nil),
	}
	if valueRef != nil {
		contentParams.ValueRef = &secrets.ValueRef{
			BackendID:  valueRef.BackendID,
			RevisionID: valueRef.RevisionID,
		}
		backendConfig, draining, err = s.getBackend(valueRef.BackendID)
		if err != nil {
			return nil, nil, false, errors.Trace(err)
		}
	}
	return &contentParams, backendConfig, draining, nil
}

func (s secretDeletionAPIClient) GetContentInfo(context.Context, *secrets.URI, string, bool, bool) (*ContentParams, *provider.ModelBackendConfig, bool, error) {
	return nil, nil, false, errors.NotSupportedf("GetContentInfo")
}

func (s secretDeletionAPIClient) GetSecretBackendConfig(context.Context, *string) (*provider.ModelBackendConfigInfo, error) {
	return nil, errors.NotSupportedf("GetContentInfo")
}

func (s secretDeletionAPIClient) GetBackendConfigForDrain(context.Context, *string) (*provider.ModelBackendConfig, string, error) {
	return nil, "", errors.NotSupportedf("GetBackendConfigForDrain")
}

func (c *deleteContentClient) GetRevisionContent(context.Context, *secrets.URI, int) (secrets.SecretValue, error) {
	return nil, errors.NotSupportedf("GetRevisionContent")
}

func (c *deleteContentClient) GetBackend(context.Context, *string, bool) (provider.SecretsBackend, string, error) {
	return nil, "", errors.NotSupportedf("GetBackend")
}

// BackendConfigForDeleteGetter is a func used to get secret backend config to
// create a backend client used to delete secret content.
type BackendConfigForDeleteGetter func(backendID string) (*provider.ModelBackendConfigInfo, error)

type deleteContentClient struct {
	*secretsClient
}

// NewClientForContentDeletion creates a backend client that is solely used
// for deleting secret content.
func NewClientForContentDeletion(secretsState SecretsState, backendConfigGetter BackendConfigForDeleteGetter) *deleteContentClient {
	return &deleteContentClient{
		&secretsClient{
			jujuAPI: secretDeletionAPIClient{
				secretsState:        secretsState,
				backendConfigGetter: backendConfigGetter,
			},
		},
	}
}
