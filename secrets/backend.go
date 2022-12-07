// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/secrets/provider"
)

// PermissionDenied is returned when an api fails due to a permission issue.
const PermissionDenied = errors.ConstError("permission denied")

// secretsClient wraps a Juju secrets manager client.
// If a backend is specified, the secret content is managed
// by the backend instead of being stored in the Juju database.
type secretsClient struct {
	jujuAPI         jujuAPIClient
	activeBackendID string
	backends        map[string]provider.SecretsBackend
}

func getBackend(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
	p, err := provider.Provider(cfg.BackendType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return p.NewBackend(cfg)
}

// NewClient returns a new secret client configured to use the specified
// secret backend as a content backend.
func NewClient(jujuAPI jujuAPIClient) (*secretsClient, error) {
	info, err := jujuAPI.GetSecretBackendConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	backends := make(map[string]provider.SecretsBackend)
	for id, cfg := range info.Configs {
		backends[id], err = getBackend(&provider.ModelBackendConfig{
			ControllerUUID: info.ControllerUUID,
			ModelUUID:      info.ModelUUID,
			ModelName:      info.ModelName,
			BackendConfig:  cfg,
		})
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return &secretsClient{
		jujuAPI:         jujuAPI,
		activeBackendID: info.ActiveID,
		backends:        backends,
	}, nil
}

// GetContent implements Client.
func (c *secretsClient) GetContent(uri *secrets.URI, label string, refresh, peek bool) (secrets.SecretValue, error) {
	content, err := c.jujuAPI.GetContentInfo(uri, label, refresh, peek)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err = content.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	if content.ValueRef == nil {
		return content.SecretValue, nil
	}
	backend, ok := c.backends[content.ValueRef.BackendID]
	if !ok {
		return nil, errors.NotFoundf("external secret backend %q", content.ValueRef.BackendID)
	}
	return backend.GetContent(context.Background(), content.ValueRef.RevisionID)
}

// SaveContent implements Client.
func (c *secretsClient) SaveContent(uri *secrets.URI, revision int, value secrets.SecretValue) (secrets.ValueRef, error) {
	activeBackend := c.backends[c.activeBackendID]
	if activeBackend == nil {
		return secrets.ValueRef{}, errors.NotSupportedf("saving secret content to external backend")
	}
	revId, err := activeBackend.SaveContent(context.Background(), uri, revision, value)
	if err != nil {
		return secrets.ValueRef{}, errors.Trace(err)
	}
	return secrets.ValueRef{
		BackendID:  c.activeBackendID,
		RevisionID: revId,
	}, nil
}

// DeleteContent implements Client.
func (c *secretsClient) DeleteContent(ref secrets.ValueRef) error {
	backend, ok := c.backends[ref.BackendID]
	if ok {
		return errors.NotFoundf("external secret backend %q", ref.BackendID)
	}
	return backend.DeleteContent(context.Background(), ref.RevisionID)
}
