// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/secrets/provider"
)

// secretsClient wraps a Juju secrets manager client.
// If a store is specified, the secret content is managed
// by the store instead of being stored in the Juju database.
type secretsClient struct {
	jujuAPI jujuAPIClient
	store   provider.SecretsStore
}

// NewClient returns a new secret client configured to use the specified
// secret store as a content backend.
func NewClient(jujuAPI jujuAPIClient, cfg *provider.StoreConfig) (*secretsClient, error) {
	p, err := provider.Provider(cfg.StoreType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	store, err := p.NewStore(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &secretsClient{
		jujuAPI: jujuAPI,
		store:   store,
	}, nil
}

// CreateSecretURIs implements Client.
func (c *secretsClient) CreateSecretURIs(count int) ([]*secrets.URI, error) {
	return c.jujuAPI.CreateSecretURIs(count)
}

// GetConsumerSecretsRevisionInfo implements Client.
func (c *secretsClient) GetConsumerSecretsRevisionInfo(unitName string, secretURIs []string) (map[string]secrets.SecretRevisionInfo, error) {
	return c.jujuAPI.GetConsumerSecretsRevisionInfo(unitName, secretURIs)
}

// GetContent implements Client.
func (c *secretsClient) GetContent(uri *secrets.URI, label string, update, peek bool) (secrets.SecretValue, error) {
	content, err := c.jujuAPI.GetContentInfo(uri, label, update, peek)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err = content.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	// We just support the juju backend for now.
	// In the future, we'll use the store to lookup the secret content based on id.
	if content.ProviderId != nil && c.store == nil {
		return nil, errors.NotSupportedf("secret content from external store")
	}
	if content.ProviderId != nil {
		return c.store.GetContent(context.Background(), *content.ProviderId)
	}
	return content.SecretValue, nil
}

// SaveContent implements Client.
func (c *secretsClient) SaveContent(uri *secrets.URI, revision int, value secrets.SecretValue) (string, error) {
	if c.store == nil {
		return "", errors.NotSupportedf("saving secret content to external store")
	}
	return c.store.SaveContent(context.Background(), uri, revision, value)
}

// DeleteContent implements Client.
func (c *secretsClient) DeleteContent(providerId string) error {
	if c.store == nil {
		return errors.NotSupportedf("deleting secret content from external store")
	}
	return c.store.DeleteContent(context.Background(), providerId)
}

// SecretMetadata implements Client.
func (c *secretsClient) SecretMetadata(filter secrets.Filter) ([]secrets.SecretOwnerMetadata, error) {
	return c.jujuAPI.SecretMetadata(filter)
}

// WatchSecretsChanges implements Client.
func (c *secretsClient) WatchSecretsChanges(unitName string) (watcher.StringsWatcher, error) {
	return c.jujuAPI.WatchSecretsChanges(unitName)
}

// SecretRotated implements Client.
func (c *secretsClient) SecretRotated(uri string, oldRevision int) error {
	return c.jujuAPI.SecretRotated(uri, oldRevision)
}
