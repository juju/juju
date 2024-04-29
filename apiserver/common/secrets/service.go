// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/secrets"
	secretservice "github.com/juju/juju/domain/secret/service"
	backendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/secrets/provider"
)

// SecretService instances provide secret service apis.
type SecretService interface {
	ListGrantedSecretsForBackend(ctx context.Context, backendID string, role secrets.SecretRole, consumers ...secretservice.SecretAccessor) ([]*secrets.SecretRevisionRef, error)
	ListCharmSecrets(context.Context, ...secretservice.CharmSecretOwner) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)
	ListUserSecrets(ctx context.Context) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error)
	ChangeSecretBackend(ctx context.Context, uri *secrets.URI, revision int, params secretservice.ChangeSecretBackendParams) error
}

// SecretBackendService instances provide secret backend service apis.
type SecretBackendService interface {
	GetRevisionsToDrain(ctx context.Context, modelUUID coremodel.UUID, revs []*secrets.SecretRevisionMetadata) ([]backendservice.RevisionInfo, error)
	GetSecretBackendConfigForAdmin(ctx context.Context, modelUUID coremodel.UUID) (*provider.ModelBackendConfigInfo, error)
}
