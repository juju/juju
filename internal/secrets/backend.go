// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/secrets"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/internal/secrets/provider"
)

// PermissionDenied is returned when an api fails due to a permission issue.
const PermissionDenied = errors.ConstError("permission denied")

// secretsClient wraps a Juju secrets manager client.
// If a backend is specified, the secret content is managed
// by the backend instead of being stored in the Juju database.
type secretsClient struct {
	jujuAPI JujuAPIClient
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

// GetBackend returns the secrets backend for the specified ID and the model's current active backend ID.
func (c *secretsClient) GetBackend(backendID *string, forDrain bool) (provider.SecretsBackend, string, error) {
	if forDrain {
		cfg, activeID, err := c.jujuAPI.GetBackendConfigForDrain(backendID)
		if err != nil {
			return nil, "", errors.Trace(err)
		}
		b, err := GetBackend(cfg)
		if err != nil {
			return nil, "", errors.Trace(err)
		}
		return b, activeID, nil
	}
	info, err := c.jujuAPI.GetSecretBackendConfig(backendID)
	if err != nil {
		return nil, "", errors.Trace(err)
	}
	want := info.ActiveID
	if backendID != nil {
		want = *backendID
	}
	cfg, ok := info.Configs[want]
	if !ok {
		return nil, "", errors.Errorf("secret backend %q missing from config", want)
	}
	b, err := GetBackend(&cfg)
	return b, info.ActiveID, errors.Trace(err)
}

// GetContent implements Client.
func (c *secretsClient) GetContent(uri *secrets.URI, label string, refresh, peek bool) (secrets.SecretValue, error) {
	lastBackendID := ""
	for {
		content, backendCfg, wasDraining, err := c.jujuAPI.GetContentInfo(uri, label, refresh, peek)
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
		backend, err := GetBackend(backendCfg)
		if err != nil {
			return nil, errors.Trace(err)
		}
		val, err := backend.GetContent(context.TODO(), content.ValueRef.RevisionID)
		if err == nil || !errors.Is(err, secreterrors.SecretRevisionNotFound) || lastBackendID == backendID {
			return val, errors.Trace(err)
		}
		lastBackendID = backendID
		// Secret may have been drained to the active backend.
		if wasDraining {
			continue
		}
		return nil, errors.Trace(err)
	}
}

// GetRevisionContent implements Client.
func (c *secretsClient) GetRevisionContent(uri *secrets.URI, revision int) (secrets.SecretValue, error) {
	content, _, _, err := c.jujuAPI.GetRevisionContentInfo(uri, revision, false)
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
	backend, _, err := c.GetBackend(&backendID, false)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return backend.GetContent(context.TODO(), content.ValueRef.RevisionID)
}

// SaveContent implements Client.
func (c *secretsClient) SaveContent(uri *secrets.URI, revision int, value secrets.SecretValue) (secrets.ValueRef, error) {
	activeBackend, activeBackendID, err := c.GetBackend(nil, false)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			return secrets.ValueRef{}, errors.NotSupportedf("saving secret content to external backend")
		}
		return secrets.ValueRef{}, errors.Trace(err)
	}
	revId, err := activeBackend.SaveContent(context.TODO(), uri, revision, value)
	if err != nil {
		return secrets.ValueRef{}, errors.Trace(err)
	}
	return secrets.ValueRef{
		BackendID:  activeBackendID,
		RevisionID: revId,
	}, nil
}

// DeleteContent implements Client.
func (c *secretsClient) DeleteContent(uri *secrets.URI, revision int) error {
	lastBackendID := ""
	for {
		content, backendCfg, wasDraining, err := c.jujuAPI.GetRevisionContentInfo(uri, revision, true)
		if err != nil {
			return errors.Trace(err)
		}
		if content.ValueRef == nil {
			return nil
		}

		backendID := content.ValueRef.BackendID
		backend, err := GetBackend(backendCfg)
		if err != nil {
			return errors.Trace(err)
		}
		err = backend.DeleteContent(context.TODO(), content.ValueRef.RevisionID)
		if err == nil || !errors.Is(err, secreterrors.SecretRevisionNotFound) || lastBackendID == backendID {
			return errors.Trace(err)
		}
		lastBackendID = backendID
		// Secret may have been drained to the active backend.
		if wasDraining {
			continue
		}
		return errors.Trace(err)
	}
}

// DeleteExternalContent implements Client.
func (c *secretsClient) DeleteExternalContent(ref secrets.ValueRef) error {
	backend, _, err := c.GetBackend(&ref.BackendID, false)
	if err != nil {
		return errors.Trace(err)
	}
	err = backend.DeleteContent(context.TODO(), ref.RevisionID)
	if errors.Is(err, secreterrors.SecretRevisionNotFound) {
		return nil
	}
	return errors.Trace(err)
}
