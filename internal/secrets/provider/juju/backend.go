// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	coresecrets "github.com/juju/juju/core/secrets"
	secreterrors "github.com/juju/juju/domain/secret/errors"
)

// jujuBackend is a dummy backend which returns
// NotFound or NotSupported as needed.
type jujuBackend struct{}

// GetContent implements SecretsBackend.
func (k jujuBackend) GetContent(ctx context.Context, revisionId string) (coresecrets.SecretValue, error) {
	return nil, fmt.Errorf("secret revision %s not found%w", revisionId, errors.Hide(secreterrors.SecretRevisionNotFound))
}

// DeleteContent implements SecretsBackend.
func (k jujuBackend) DeleteContent(ctx context.Context, revisionId string) error {
	return fmt.Errorf("secret revision %s not found%w", revisionId, errors.Hide(secreterrors.SecretRevisionNotFound))
}

// SaveContent implements SecretsBackend.
func (k jujuBackend) SaveContent(ctx context.Context, uri *coresecrets.URI, revision int, value coresecrets.SecretValue) (string, error) {
	return "", errors.NotSupportedf("saving content to internal backend")
}

// Ping implements SecretsBackend.
func (k jujuBackend) Ping() error {
	return nil
}
