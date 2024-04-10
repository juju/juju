// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/secrets"
	secreterrors "github.com/juju/juju/domain/secret/errors"
)

// GetSecretConsumerAndLatest returns the secret consumer info for the specified unit and secret, along with
// the latest revision for the secret.
// If the unit does not exist, an error satisfying [uniterrors.NotFound] is returned.
// If the secret does not exist, an error satisfying [secreterrors.SecretNotFound] is returned.
// If there's not currently a consumer record for the secret, the latest revision is still returned,
// along with an error satisfying [secreterrors.SecretConsumerNotFound].
func (s *SecretService) GetSecretConsumerAndLatest(ctx context.Context, uri *secrets.URI, unitName string) (*secrets.SecretConsumerMetadata, int, error) {
	consumerMetadata, latestRevision, err := s.st.GetSecretConsumer(ctx, uri, unitName)

	if err != nil {
		return nil, latestRevision, errors.Trace(err)
	}
	if consumerMetadata.Label != "" {
		return consumerMetadata, latestRevision, nil
	}
	// We allow units to access the application owned secrets using the application owner label,
	// so we copy the owner label to consumer metadata.
	md, err := s.getAppOwnedOrUnitOwnedSecretMetadata(ctx, uri, unitName, "")
	if errors.Is(err, secreterrors.SecretNotFound) {
		// The secret is owned by a different application.
		return consumerMetadata, latestRevision, nil
	}
	if err != nil {
		return nil, 0, errors.Annotatef(err, "cannot get secret metadata for %q", uri)
	}
	consumerMetadata.Label = md.Label
	return consumerMetadata, latestRevision, nil
}

// GetSecretConsumer returns the secret consumer info for the specified unit and secret.
// If the unit does not exist, an error satisfying [uniterrors.NotFound] is returned.
// If the secret does not exist, an error satisfying [secreterrors.SecretNotFound] is returned.
// If there's not currently a consumer record for the secret, an error satisfying [secreterrors.SecretConsumerNotFound]
// is returned.
func (s *SecretService) GetSecretConsumer(ctx context.Context, uri *secrets.URI, unitName string) (*secrets.SecretConsumerMetadata, error) {
	result, _, err := s.GetSecretConsumerAndLatest(ctx, uri, unitName)
	return result, err
}

// SaveSecretConsumer saves the consumer metadata for the given secret and unit.
// If the unit does not exist, an error satisfying [uniterrors.NotFound] is returned.
// If the secret does not exist, an error satisfying [secreterrors.SecretNotFound] is returned.
func (s *SecretService) SaveSecretConsumer(ctx context.Context, uri *secrets.URI, unitName string, md *secrets.SecretConsumerMetadata) error {
	return s.st.SaveSecretConsumer(ctx, uri, unitName, md)
}

func (s *SecretService) GetSecretRemoteConsumer(ctx context.Context, uri *secrets.URI, unitName string) (*secrets.SecretConsumerMetadata, error) {
	return nil, secreterrors.SecretConsumerNotFound
}

func (s *SecretService) SaveSecretRemoteConsumer(ctx context.Context, uri *secrets.URI, latestRevision int, unitName string, md *secrets.SecretConsumerMetadata) error {
	return nil
}

func (s *SecretService) GetURIByConsumerLabel(ctx context.Context, label string, unitName string) (*secrets.URI, error) {
	return nil, secreterrors.SecretNotFound
}

// GetConsumedRevision returns the secret revision number for the specified consumer, possibly updating
// the label associated with the secret for the consumer.
// TODO(secrets) - test
func (s *SecretService) GetConsumedRevision(ctx context.Context, uri *secrets.URI, unitName string, refresh, peek bool, labelToUpdate *string) (int, error) {
	consumerInfo, latestRevision, err := s.GetSecretConsumerAndLatest(ctx, uri, unitName)
	if err != nil && !errors.Is(err, secreterrors.SecretConsumerNotFound) {
		return 0, errors.Trace(err)
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
			return 0, errors.Trace(err)
		}
	}
	return wantRevision, nil
}

// ListGrantedSecrets returns the secret metadata and revision metadata for any secrets
// for which the specified consumers have been granted view access.
// The count of secret and revisions in the result must match.
func (s *SecretService) ListGrantedSecrets(ctx context.Context, consumers ...SecretAccessor) ([]*secrets.SecretMetadata, [][]*secrets.SecretRevisionMetadata, error) {
	return nil, nil, nil
}
