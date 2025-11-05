// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainsecret "github.com/juju/juju/domain/secret"
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
	GetRemoteConsumedSecretURIsWithChangesFromOfferingSide(ctx context.Context, appUUID string, revisionUUIDs ...string) ([]string, error)

	// GetSecretRemoteConsumer returns the secret consumer info from a cross model consumer
	// for the specified unit and secret.
	GetSecretRemoteConsumer(ctx context.Context, uri *secrets.URI, unitName string) (*secrets.SecretConsumerMetadata, int, error)
	// SaveSecretRemoteConsumer saves the consumer metadata for the given secret and unit.
	SaveSecretRemoteConsumer(ctx context.Context, uri *secrets.URI, unitName string, md secrets.SecretConsumerMetadata) error
	// UpdateRemoteSecretRevision records the latest revision of the specified cross model secret.
	UpdateRemoteSecretRevision(ctx context.Context, uri *secrets.URI, latestRevision int, applicationUUID string) error

	// SaveRemoteSecretConsumer saves the consumer metadata for the given secret and unit.
	SaveRemoteSecretConsumer(ctx context.Context, uri *secrets.URI, unitName string,
		md secrets.SecretConsumerMetadata, appUUID, relUUID string) error
	// GetUnitUUID returns the unit UUID for the specified unit.
	GetUnitUUID(ctx context.Context, unitName string) (string, error)

	// GetSecretValue returns the contents - either data or value reference - of a
	// given secret revision.
	GetSecretValue(ctx context.Context, uri *secrets.URI, revision int) (secrets.SecretData, *secrets.ValueRef, error)
	// GetSecretAccess returns the access to the secret for the specified accessor.
	GetSecretAccess(ctx context.Context, uri *secrets.URI, params domainsecret.AccessParams) (string, error)
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
func (s *Service) UpdateRemoteSecretRevision(
	ctx context.Context, uri *secrets.URI, latestRevision int, applicationUUID coreapplication.UUID,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := applicationUUID.Validate(); err != nil {
		return errors.Errorf(
			"validating application uuid: %w", err).Add(applicationerrors.ApplicationUUIDNotValid)
	}

	return s.modelState.UpdateRemoteSecretRevision(ctx, uri, latestRevision, applicationUUID.String())
}

// SaveRemoteSecretConsumer saves the consumer metadata for the given remote secret and unit.
// If the unit does not exist, an error satisfying [applicationerrors.UnitNotFound] is returned.
// If the secret does not exist, an error satisfying [secreterrors.SecretNotFound] is returned.
// If the corresponding synthetic application for the relation does not exist,
// an error satisfying [crossmodelrelationerrors.RemoteApplicationNotFound] is returned.
func (s *Service) SaveRemoteSecretConsumer(ctx context.Context, uri *secrets.URI, unitName unit.Name,
	md secrets.SecretConsumerMetadata, appUUID coreapplication.UUID, relationUUID relation.UUID) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}

	unitUUID, err := s.modelState.GetUnitUUID(ctx, unitName.String())
	if err != nil {
		return errors.Capture(err)
	}

	return s.modelState.SaveRemoteSecretConsumer(ctx, uri, unitUUID, md, appUUID.String(), relationUUID.String())
}

func (s *Service) canRead(ctx context.Context, uri *secrets.URI, consumer domainsecret.AccessParams) error {
	role, err := s.modelState.GetSecretAccess(ctx, uri, consumer)
	if err != nil {
		// Typically not found error.
		return errors.Capture(err)
	}
	if !secrets.SecretRole(role).Allowed(secrets.RoleView) {
		return errors.Errorf("%q is not allowed to read this secret", consumer.SubjectID).Add(secreterrors.PermissionDenied)
	}
	return nil
}

// ProcessRemoteConsumerGetSecret returns the content of a remotely consumed secret,
// and the latest secret revision.
// The following errors may be returned:
// - [secreterrors.PermissionDenied] if the consumer does not have permission to read the secret
// - [secreterrors.SecretNotFound] if the secret does not exist
// - [secreterrors.SecretRevisionNotFound] if the secret revision does not exist
func (s *Service) ProcessRemoteConsumerGetSecret(
	ctx context.Context, uri *secrets.URI, consumer unit.Name, revision *int, peek, refresh bool,
) (secrets.SecretValue, *secrets.ValueRef, int, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// We only every grant permission to the consuming application,
	// not to the consuming unit. If this ever changes, we can check
	// for unit permission here.
	appName := consumer.Application()
	if err := s.canRead(ctx, uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     appName,
	}); err != nil {
		return nil, nil, 0, errors.Capture(err)
	}

	var (
		wantRevision   int
		latestRevision int
	)
	// Use the latest revision as the current one if --peek.
	if peek || refresh || revision == nil {
		var err error
		latestRevision, err = s.updateConsumedRevision(ctx, consumer, uri, refresh)
		if err != nil {
			return nil, nil, 0, errors.Capture(err)
		}
		wantRevision = latestRevision
	} else {
		wantRevision = *revision
	}

	data, valueRef, err := s.modelState.GetSecretValue(ctx, uri, wantRevision)
	return secrets.NewSecretValue(data), valueRef, latestRevision, err
}

func (s *Service) updateConsumedRevision(ctx context.Context, consumer unit.Name, uri *secrets.URI, refresh bool) (int, error) {
	consumerInfo, latestRevision, err := s.modelState.GetSecretRemoteConsumer(ctx, uri, consumer.String())
	if err != nil && !errors.Is(err, secreterrors.SecretConsumerNotFound) {
		return 0, errors.Capture(err)
	}
	refresh = refresh ||
		err != nil // Not found, so need to create one.

	if refresh {
		if consumerInfo == nil {
			consumerInfo = &secrets.SecretConsumerMetadata{}
		}
		consumerInfo.CurrentRevision = latestRevision
		if err := s.modelState.SaveSecretRemoteConsumer(ctx, uri, consumer.String(), *consumerInfo); err != nil {
			return 0, errors.Capture(err)
		}
	}
	return latestRevision, nil
}
