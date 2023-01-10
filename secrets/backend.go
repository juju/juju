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
	jujuAPI         JujuAPIClient
	activeBackendID string
	backends        map[string]provider.SecretsBackend
}

// For testing.
var (
	GetBackend = getBackend
)

func getBackend(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
	p, err := provider.Provider(cfg.BackendType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return p.NewBackend(cfg)
}

// NewClient returns a new secret client configured to use the specified
// secret backend as a content backend.
func NewClient(jujuAPI JujuAPIClient) (*secretsClient, error) {
	c := &secretsClient{
		jujuAPI: jujuAPI,
	}
	return c, nil
}

func (c *secretsClient) init() error {
	info, err := c.jujuAPI.GetSecretBackendConfig()
	if err != nil {
		return errors.Trace(err)
	}
	backends := make(map[string]provider.SecretsBackend)
	for id, cfg := range info.Configs {
		backends[id], err = GetBackend(&provider.ModelBackendConfig{
			ControllerUUID: info.ControllerUUID,
			ModelUUID:      info.ModelUUID,
			ModelName:      info.ModelName,
			BackendConfig:  cfg,
		})
		if err != nil {
			return errors.Trace(err)
		}
	}
	c.activeBackendID = info.ActiveID
	c.backends = backends
	return nil
}

// GetContent implements Client.
func (c *secretsClient) GetContent(uri *secrets.URI, label string, refresh, peek bool) (secrets.SecretValue, error) {
	if err := c.init(); err != nil {
		return nil, errors.Trace(err)
	}
	lastBackendID := ""
	for {
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

		backendID := content.ValueRef.BackendID
		backend, ok := c.backends[backendID]
		if !ok {
			return nil, errors.NotFoundf("external secret backend %q", backendID)
		}
		val, err := backend.GetContent(context.TODO(), content.ValueRef.RevisionID)
		if err == nil || !errors.Is(err, errors.NotFound) || lastBackendID == backendID {
			return val, errors.Trace(err)
		}
		lastBackendID = backendID
		// Secret may have been drained to the active backend.
		if backendID != c.activeBackendID {
			continue
		}
		// The active backend may have changed.
		if initErr := c.init(); initErr != nil {
			return nil, errors.Trace(initErr)
		}
		if c.activeBackendID == backendID {
			return nil, errors.Trace(err)
		}
	}
}

// SaveContent implements Client.
func (c *secretsClient) SaveContent(uri *secrets.URI, revision int, value secrets.SecretValue) (secrets.ValueRef, error) {
	if err := c.init(); err != nil {
		return secrets.ValueRef{}, errors.Trace(err)
	}
	activeBackend := c.backends[c.activeBackendID]
	if activeBackend == nil {
		return secrets.ValueRef{}, errors.NotSupportedf("saving secret content to external backend")
	}
	revId, err := activeBackend.SaveContent(context.TODO(), uri, revision, value)
	if err != nil {
		return secrets.ValueRef{}, errors.Trace(err)
	}
	return secrets.ValueRef{
		BackendID:  c.activeBackendID,
		RevisionID: revId,
	}, nil
}

// DeleteContent implements Client.
func (c *secretsClient) DeleteContent(uri *secrets.URI, revision int) error {
	if err := c.init(); err != nil {
		return errors.Trace(err)
	}
	lastBackendID := ""
	for {
		content, err := c.jujuAPI.GetRevisionContentInfo(uri, revision, true)
		if err != nil {
			return errors.Trace(err)
		}
		if err = content.Validate(); err != nil {
			return errors.Trace(err)
		}
		if content.ValueRef == nil {
			return nil
		}

		backendID := content.ValueRef.BackendID
		backend, ok := c.backends[backendID]
		if !ok {
			return errors.NotFoundf("external secret backend %q", backendID)
		}
		err = backend.DeleteContent(context.TODO(), content.ValueRef.RevisionID)
		if err == nil || !errors.Is(err, errors.NotFound) || lastBackendID == backendID {
			return errors.Trace(err)
		}
		lastBackendID = backendID
		// Secret may have been drained to the active backend.
		if backendID != c.activeBackendID {
			continue
		}
		// The active backend may have changed.
		if initErr := c.init(); initErr != nil {
			return errors.Trace(initErr)
		}
		if c.activeBackendID == backendID {
			return errors.Trace(err)
		}
	}
}

// DeleteExternalContent implements Client.
func (c *secretsClient) DeleteExternalContent(ref secrets.ValueRef) error {
	backend, ok := c.backends[ref.BackendID]
	if !ok {
		return errors.NotFoundf("external secret backend %q", ref.BackendID)
	}
	err := backend.DeleteContent(context.TODO(), ref.RevisionID)
	if errors.Is(err, errors.NotFound) {
		return nil
	}
	return errors.Trace(err)
}
