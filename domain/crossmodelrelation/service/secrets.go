// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/internal/errors"
)

// ModelSecretsState describes retrieval and persistence methods for
// cross model relations secrets related functionality in the model database.
type ModelSecretsState interface {
	// InitialWatchStatementForRemoteConsumedSecretsChangesFromOfferingSide initialises a watcher used to notify
	// of changes to consumed secrets in an offering model.
	InitialWatchStatementForRemoteConsumedSecretsChangesFromOfferingSide(appUUID string) (string, eventsource.NamespaceQuery)
	// GetRemoteConsumedSecretURIsWithChangesFromOfferingSide composes changes to consumed secrets in an offering model.
	GetRemoteConsumedSecretURIsWithChangesFromOfferingSide(ctx context.Context, appUUID string, secretIDs ...string) ([]string, error)

	// GetSecretRemoteConsumer returns the secret consumer info from a cross model consumer
	// for the specified unit and secret.
	GetSecretRemoteConsumer(ctx context.Context, uri *secrets.URI, unitName string) (*secrets.SecretConsumerMetadata, int, error)
	// SaveSecretRemoteConsumer saves the consumer metadata for the given secret and unit.
	SaveSecretRemoteConsumer(ctx context.Context, uri *secrets.URI, unitName string, md secrets.SecretConsumerMetadata) error
	// UpdateRemoteSecretRevision records the latest revision of the specified cross model secret.
	UpdateRemoteSecretRevision(ctx context.Context, uri *secrets.URI, latestRevision int) error
}

// UpdateRemoteConsumedRevision returns the latest revision for the specified secret,
// updating the tracked revision for the specified consumer if refresh is true.
// Run on the offering model when a consumer gets content for an offered secret.
func (s *Service) UpdateRemoteConsumedRevision(ctx context.Context, uri *secrets.URI, unitName unit.Name, refresh bool) (int, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return 0, errors.Errorf("validating unit name: %w", err)
	}

	consumerInfo, latestRevision, err := s.modelState.GetSecretRemoteConsumer(ctx, uri, unitName.String())
	if err != nil && !errors.Is(err, secreterrors.SecretConsumerNotFound) {
		return 0, errors.Capture(err)
	}
	refresh = refresh ||
		err != nil // Not found, so need to create one.
	if !refresh {
		return latestRevision, nil
	}

	if consumerInfo == nil {
		consumerInfo = &secrets.SecretConsumerMetadata{}
	}
	consumerInfo.CurrentRevision = latestRevision
	if err := s.modelState.SaveSecretRemoteConsumer(ctx, uri, unitName.String(), *consumerInfo); err != nil {
		return 0, errors.Capture(err)
	}
	return latestRevision, nil
}

// UpdateRemoteSecretRevision records the specified revision for the secret
// which has been consumed from a different model.
// Run on the consuming model to record that a new revision for a secret
// from the offering model is available.
func (s *Service) UpdateRemoteSecretRevision(ctx context.Context, uri *secrets.URI, latestRevision int) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.modelState.UpdateRemoteSecretRevision(ctx, uri, latestRevision)
}
