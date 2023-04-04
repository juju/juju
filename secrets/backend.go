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
	jujuAPI jujuAPIClient
	backend provider.SecretsBackend
}

// NewClient returns a new secret client configured to use the specified
// secret backend as a content backend.
func NewClient(jujuAPI jujuAPIClient) (*secretsClient, error) {
	cfg, err := jujuAPI.GetSecretBackendConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	p, err := provider.Provider(cfg.BackendType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	backend, err := p.NewBackend(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &secretsClient{
		jujuAPI: jujuAPI,
		backend: backend,
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
	// We just support the juju backend for now.
	// In the future, we'll use the backend to lookup the secret content based on id.
	if content.BackendId != nil && c.backend == nil {
		return nil, errors.NotSupportedf("secret content from external backend")
	}
	if content.BackendId != nil {
		return c.backend.GetContent(context.Background(), *content.BackendId)
	}
	return content.SecretValue, nil
}

// SaveContent implements Client.
func (c *secretsClient) SaveContent(uri *secrets.URI, revision int, value secrets.SecretValue) (string, error) {
	if c.backend == nil {
		return "", errors.NotSupportedf("saving secret content to external backend")
	}
	return c.backend.SaveContent(context.Background(), uri, revision, value)
}

// DeleteContent implements Client.
func (c *secretsClient) DeleteContent(backendId string) error {
	if c.backend == nil {
		return errors.NotSupportedf("deleting secret content from external backend")
	}
	return c.backend.DeleteContent(context.Background(), backendId)
}
