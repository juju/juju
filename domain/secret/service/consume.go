// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/secrets"
	secreterrors "github.com/juju/juju/domain/secret/errors"
)

func (s *SecretService) GetSecretConsumer(ctx context.Context, uri *secrets.URI, consumer SecretConsumer) (*secrets.SecretConsumerMetadata, error) {
	//TODO implement me
	panic("implement me")
	/*
		//if consumerMetadata.Label != "" {
		//	return consumerMetadata, nil
		//}
		// We allow units to access the application owned secrets using the application owner label,
		// so we copy the owner label to consumer metadata.
		//md, err := s.getAppOwnedOrUnitOwnedSecretMetadata(uri, "")
		//if errors.Is(err, errors.NotFound) {
		//	// The secret is owned by a different application.
		//	return consumerMetadata, nil
		//}
		//if err != nil {
		//	return nil, errors.Annotatef(err, "cannot get secret metadata for %q", uri)
		//}
		//consumerMetadata.Label = md.Label

	*/
}

func (s *SecretService) GetURIByConsumerLabel(ctx context.Context, label string, consumer SecretConsumer) (*secrets.URI, error) {
	//TODO implement me
	panic("implement me")
}

func (s *SecretService) SaveSecretConsumer(ctx context.Context, uri *secrets.URI, consumer SecretConsumer, md *secrets.SecretConsumerMetadata) error {
	//TODO implement me
	panic("implement me")
}

// TODO(secrets) - document and test
func (s *SecretService) GetConsumedRevision(ctx context.Context, uri *secrets.URI, consumer SecretConsumer, refresh, peek bool, labelToUpdate *string) (int, error) {
	consumerInfo, err := s.GetSecretConsumer(ctx, uri, consumer)
	if err != nil && !errors.Is(err, secreterrors.SecretNotFound) {
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
		md, err := s.GetSecret(ctx, uri)
		if err != nil {
			return 0, errors.Trace(err)
		}
		if consumerInfo == nil {
			consumerInfo = &secrets.SecretConsumerMetadata{}
		}
		consumerInfo.LatestRevision = md.LatestRevision
		if refresh {
			consumerInfo.CurrentRevision = md.LatestRevision
		}
		wantRevision = md.LatestRevision
	}
	// Save the latest consumer info if required.
	if refresh || labelToUpdate != nil {
		if labelToUpdate != nil {
			consumerInfo.Label = *labelToUpdate
		}
		if err := s.SaveSecretConsumer(ctx, uri, consumer, consumerInfo); err != nil {
			return 0, errors.Trace(err)
		}
	}
	return wantRevision, nil
}
