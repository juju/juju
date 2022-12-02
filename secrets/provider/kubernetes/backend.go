// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/secrets"
)

type k8sBackend struct {
	broker caas.SecretsBackend
}

// GetContent implements SecretsBackend.
func (k k8sBackend) GetContent(ctx context.Context, backendId string) (secrets.SecretValue, error) {
	return k.broker.GetJujuSecret(ctx, backendId)
}

// DeleteContent implements SecretsBackend.
func (k k8sBackend) DeleteContent(ctx context.Context, backendId string) error {
	return k.broker.DeleteJujuSecret(ctx, backendId)
}

// SaveContent implements SecretsBackend.
func (k k8sBackend) SaveContent(ctx context.Context, uri *secrets.URI, revision int, value secrets.SecretValue) (string, error) {
	return k.broker.SaveJujuSecret(ctx, uri.Name(revision), value)
}
