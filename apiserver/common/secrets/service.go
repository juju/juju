// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/juju/core/secrets"
	secretservice "github.com/juju/juju/domain/secret/service"
)

// SecretService instances provide secret service apis.
type SecretService interface {
	GetSecretAccess(ctx context.Context, uri *secrets.URI, consumer secretservice.SecretAccessor) (secrets.SecretRole, error)
	ListGrantedSecrets(ctx context.Context, consumers ...secretservice.SecretAccessor) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)

	ListCharmSecrets(context.Context, ...secretservice.CharmSecretOwner) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)
	ListUserSecrets(ctx context.Context) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)
	ChangeSecretBackend(ctx context.Context, uri *secrets.URI, revision int, params secretservice.ChangeSecretBackendParams) error
}

// SecretConsumer instances provide secret service apis for managing access.
type SecretConsumer interface {
	GetSecretAccess(ctx context.Context, uri *secrets.URI, consumer secretservice.SecretAccessor) (secrets.SecretRole, error)
}
