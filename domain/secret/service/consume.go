// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	"github.com/juju/juju/internal/errors"
)

// GetSecretConsumerAndLatest returns the secret consumer info for the specified unit and secret, along with
// the latest revision for the secret.
// If the unit does not exist, an error satisfying [applicationerrors.UnitNotFound] is returned.
// If the secret does not exist, an error satisfying [secreterrors.SecretNotFound] is returned.
// If there's not currently a consumer record for the secret, the latest revision is still returned,
// along with an error satisfying [secreterrors.SecretConsumerNotFound].
func (s *SecretService) GetSecretConsumerAndLatest(ctx context.Context, uri *secrets.URI, unitName unit.Name) (_ *secrets.SecretConsumerMetadata, _ int, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	consumerMetadata, latestRevision, err := s.secretState.GetSecretConsumer(ctx, uri, unitName)
	if err != nil {
		return nil, latestRevision, errors.Capture(err)
	}
	if consumerMetadata.Label != "" {
		return consumerMetadata, latestRevision, nil
	}
	// We allow units to access the application owned secrets using the application owner label,
	// so we copy the owner label to consumer metadata.
	md, err := s.getAppOwnedOrUnitOwnedSecretMetadata(ctx, uri, unitName, "")
	if errors.Is(err, secreterrors.SecretNotFound) {
		// The secret is owned by a different application; the named unit is the consumer.
		return consumerMetadata, latestRevision, nil
	}
	if err != nil {
		return nil, 0, errors.Errorf("cannot get secret metadata for %q: %w", uri, err)
	}
	consumerMetadata.Label = md.Label
	return consumerMetadata, latestRevision, nil
}

// GetSecretConsumer returns the secret consumer info for the specified unit and secret.
// If the unit does not exist, an error satisfying [applicationerrors.UnitNotFound] is returned.
// If the secret does not exist, an error satisfying [secreterrors.SecretNotFound] is returned.
// If there's not currently a consumer record for the secret, an error satisfying [secreterrors.SecretConsumerNotFound]
// is returned.
func (s *SecretService) GetSecretConsumer(ctx context.Context, uri *secrets.URI, unitName unit.Name) (_ *secrets.SecretConsumerMetadata, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	result, _, err := s.GetSecretConsumerAndLatest(ctx, uri, unitName)
	return result, err
}

// SaveSecretConsumer saves the consumer metadata for the given secret and unit.
// If the unit does not exist, an error satisfying [applicationerrors.UnitNotFound] is returned.
// If the secret does not exist, an error satisfying [secreterrors.SecretNotFound] is returned.
func (s *SecretService) SaveSecretConsumer(ctx context.Context, uri *secrets.URI, unitName unit.Name, md *secrets.SecretConsumerMetadata) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.secretState.SaveSecretConsumer(ctx, uri, unitName, md)
}

// GetURIByConsumerLabel looks up the secret URI using the label previously registered by the specified unit,
// returning an error satisfying [secreterrors.SecretNotFound] if there's no corresponding URI.
// If the unit does not exist, an error satisfying [applicationerrors.UnitNotFound] is returned.
func (s *SecretService) GetURIByConsumerLabel(ctx context.Context, label string, unitName unit.Name) (_ *secrets.URI, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.secretState.GetURIByConsumerLabel(ctx, label, unitName)
}

// GetConsumedRevision returns the secret revision number for the specified consumer, possibly updating
// the label associated with the secret for the consumer.
func (s *SecretService) GetConsumedRevision(ctx context.Context, uri *secrets.URI, unitName unit.Name, refresh, peek bool, labelToUpdate *string) (_ int, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	consumerInfo, latestRevision, err := s.GetSecretConsumerAndLatest(ctx, uri, unitName)
	if err != nil && !errors.Is(err, secreterrors.SecretConsumerNotFound) {
		return 0, errors.Capture(err)
	}
	refresh = refresh ||
		err != nil // Not found, so need to create one.

	var wantRevision int
	if err == nil {
		wantRevision = consumerInfo.CurrentRevision
	}

	// Use the latest revision as the current one if --refresh or --peek.
	if refresh || peek {
		if consumerInfo == nil {
			consumerInfo = &secrets.SecretConsumerMetadata{}
		}
		if refresh {
			consumerInfo.CurrentRevision = latestRevision
		}
		wantRevision = latestRevision
	}
	// Save the latest consumer info if required.
	if refresh || labelToUpdate != nil {
		if labelToUpdate != nil {
			consumerInfo.Label = *labelToUpdate
		}
		if err := s.SaveSecretConsumer(ctx, uri, unitName, consumerInfo); err != nil {
			return 0, errors.Capture(err)
		}
	}
	return wantRevision, nil
}

// ListGrantedSecretsForBackend returns the secret revision info for any
// secrets from the specified backend for which the specified consumers
// have been granted the specified access.
func (s *SecretService) ListGrantedSecretsForBackend(
	ctx context.Context, backendID string, role secrets.SecretRole, consumers ...SecretAccessor,
) (_ []*secrets.SecretRevisionRef, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	accessors := make([]domainsecret.AccessParams, len(consumers))
	for i, consumer := range consumers {
		accessor := domainsecret.AccessParams{
			SubjectID: consumer.ID,
		}
		switch consumer.Kind {
		case UnitAccessor:
			accessor.SubjectTypeID = domainsecret.SubjectUnit
		case ApplicationAccessor:
			accessor.SubjectTypeID = domainsecret.SubjectApplication
		case ModelAccessor:
			accessor.SubjectTypeID = domainsecret.SubjectModel
		default:
			return nil, errors.Errorf("consumer kind %q %w", consumer.Kind, coreerrors.NotValid)
		}
		accessors[i] = accessor
	}
	return s.secretState.ListGrantedSecretsForBackend(ctx, backendID, accessors, role)
}

// UpdateRemoteConsumedRevision returns the latest revision for the specified secret,
// updating the tracked revision for the specified consumer if refresh is true.
func (s *SecretService) UpdateRemoteConsumedRevision(ctx context.Context, uri *secrets.URI, unitName unit.Name, refresh bool) (_ int, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	consumerInfo, latestRevision, err := s.secretState.GetSecretRemoteConsumer(ctx, uri, unitName)
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
		if err := s.secretState.SaveSecretRemoteConsumer(ctx, uri, unitName, consumerInfo); err != nil {
			return 0, errors.Capture(err)
		}
	}
	return latestRevision, nil
}

// UpdateRemoteSecretRevision records the specified revision for the secret
// which has been consumed from a different model.
func (s *SecretService) UpdateRemoteSecretRevision(ctx context.Context, uri *secrets.URI, latestRevision int) (err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	return s.secretState.UpdateRemoteSecretRevision(ctx, uri, latestRevision)
}
