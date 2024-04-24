// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/juju/core/secrets"
	domainsecret "github.com/juju/juju/domain/secret"
	secretservice "github.com/juju/juju/domain/secret/service"
)

// SecretService instances provide secret apis.
type SecretService interface {
	// Create and update secrets.

	CreateSecret(context.Context, *secrets.URI, secretservice.CreateSecretParams) error
	UpdateSecret(context.Context, *secrets.URI, secretservice.UpdateSecretParams) error

	// View and fetch secrets.

	GetSecret(ctx context.Context, uri *secrets.URI) (*secrets.SecretMetadata, error)
	GetUserSecretURIByLabel(ctx context.Context, label string) (*secrets.URI, error)
	GetSecretValue(context.Context, *secrets.URI, int, secretservice.SecretAccessor) (secrets.SecretValue, *secrets.ValueRef, error)
	ListSecrets(ctx context.Context, uri *secrets.URI,
		revision *int,
		labels domainsecret.Labels,
	) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)
	ListCharmSecrets(ctx context.Context, owners ...secretservice.CharmSecretOwner) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)

	// Delete secrets.

	DeleteSecret(ctx context.Context, uri *secrets.URI, params secretservice.DeleteSecretParams) error

	// Grant/revoke secret access.

	GetSecretGrants(ctx context.Context, uri *secrets.URI, role secrets.SecretRole) ([]secretservice.SecretAccess, error)
	GrantSecretAccess(ctx context.Context, uri *secrets.URI, p secretservice.SecretAccessParams) error
	RevokeSecretAccess(ctx context.Context, uri *secrets.URI, p secretservice.SecretAccessParams) error
}
