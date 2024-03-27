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

	CreateSecret(context.Context, *secrets.URI, secretservice.CreateSecretParams) (*secrets.SecretMetadata, error)
	UpdateSecret(context.Context, *secrets.URI, secretservice.UpdateSecretParams) (*secrets.SecretMetadata, error)

	// View and fetch secrets.

	GetSecret(ctx context.Context, uri *secrets.URI) (*secrets.SecretMetadata, error)
	GetUserSecretByLabel(ctx context.Context, label string) (*secrets.SecretMetadata, error)
	GetSecretValue(context.Context, *secrets.URI, int) (secrets.SecretValue, *secrets.ValueRef, error)
	ListSecrets(ctx context.Context, uri *secrets.URI,
		revisions domainsecret.Revisions,
		labels domainsecret.Labels, appOwners domainsecret.ApplicationOwners,
		unitOwners domainsecret.UnitOwners, modelOwners domainsecret.ModelOwners,
	) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)

	// Delete secrets.

	DeleteUserSecret(ctx context.Context, uri *secrets.URI, revisions []int) error

	// Grant/revoke secret access.

	GetSecretGrants(ctx context.Context, uri *secrets.URI, role secrets.SecretRole) ([]secrets.AccessInfo, error)
	GrantSecretAccess(ctx context.Context, uri *secrets.URI, p secretservice.SecretAccessParams) error
	RevokeSecretAccess(ctx context.Context, uri *secrets.URI, p secretservice.SecretAccessParams) error
}
