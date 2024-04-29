// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/secrets"
	secreterrors "github.com/juju/juju/domain/secret/errors"
)

type k8sBackend struct {
	broker caas.SecretsBackend
	pinger func() error
}

// GetContent implements SecretsBackend.
func (k k8sBackend) GetContent(ctx context.Context, revisionId string) (secrets.SecretValue, error) {
	v, err := k.broker.GetJujuSecret(ctx, revisionId)
	if errors.Is(err, errors.NotFound) {
		err = fmt.Errorf("secret revision %q not found%w", revisionId, errors.Hide(secreterrors.SecretRevisionNotFound))
	}
	return v, errors.Trace(err)
}

// DeleteContent implements SecretsBackend.
func (k k8sBackend) DeleteContent(ctx context.Context, revisionId string) error {
	err := k.broker.DeleteJujuSecret(ctx, revisionId)
	if errors.Is(err, errors.NotFound) {
		err = fmt.Errorf("secret revision %q not found%w", revisionId, errors.Hide(secreterrors.SecretRevisionNotFound))
	}
	return errors.Trace(err)
}

// SaveContent implements SecretsBackend.
func (k k8sBackend) SaveContent(ctx context.Context, uri *secrets.URI, revision int, value secrets.SecretValue) (string, error) {
	return k.broker.SaveJujuSecret(ctx, uri.Name(revision), value)
}

// Ping implements SecretsBackend.
func (k k8sBackend) Ping() error {
	return k.pinger()
}
