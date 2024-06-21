// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	secretservice "github.com/juju/juju/domain/secret/service"
	backendservice "github.com/juju/juju/domain/secretbackend/service"
)

// SecretService instances provide secret service apis.
type SecretService interface {
	ListCharmSecretsToDrain(
		ctx context.Context, owners ...secretservice.CharmSecretOwner,
	) ([]*secrets.SecretMetadataForDrain, error)
	ListUserSecretsToDrain(ctx context.Context) ([]*secrets.SecretMetadataForDrain, error)
	ChangeSecretBackend(ctx context.Context, uri *secrets.URI, revision int, params secretservice.ChangeSecretBackendParams) error
}

// SecretBackendGetter instances provide a method to get the secret backend the model.
type SecretBackendGetter interface {
	GetModelSecretBackendID(ctx context.Context, modelUUID coremodel.UUID) (string, error)
}

// SecretBackendService instances provide secret backend service apis.
type SecretBackendService interface {
	GetModelSecretBackendID(ctx context.Context, modelUUID coremodel.UUID) (string, error)
	GetRevisionsToDrain(ctx context.Context, modelUUID coremodel.UUID, revs []secrets.SecretExternalRevision) ([]backendservice.RevisionInfo, error)
	WatchModelSecretBackendChanged(ctx context.Context, modelUUID coremodel.UUID) (watcher.NotifyWatcher, error)
}
