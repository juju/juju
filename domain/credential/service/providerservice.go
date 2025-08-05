// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/internal/errors"
)

// ProviderState describes retrieval and persistence methods for storage.
type ProviderState interface {
	// CloudCredential returns the cloud credential for the given key.
	CloudCredential(ctx context.Context, key corecredential.Key) (credential.CloudCredentialResult, error)

	// CredentialUUIDForKey finds and returns the uuid for the cloud credential
	// identified by key. If no credential is found then an error of
	// [github.com/juju/juju/domain/credential/errors.NotFound] is returned.
	CredentialUUIDForKey(context.Context, corecredential.Key) (corecredential.UUID, error)

	// InvalidateCloudCredential marks a cloud credential for the provided uuid as
	// invalid.
	// The following errors can be expected:
	// - [github.com/juju/juju/domain/credential/errors.NotFound] when no
	// credential is found for the given uuid.
	InvalidateCloudCredential(ctx context.Context, uuid corecredential.UUID, reason string) error

	// WatchCredential returns a new NotifyWatcher watching for changes to the specified credential.
	WatchCredential(
		ctx context.Context,
		getWatcher func(
			ctx context.Context,
			filter eventsource.FilterOption,
			filterOpts ...eventsource.FilterOption,
		) (watcher.NotifyWatcher, error),
		key corecredential.Key,
	) (watcher.NotifyWatcher, error)
}

// ProviderService provides the API for working with credentials.
// The provider service is a subset of the credentials service, and is used by
// the provider package to interact with the credentials service. By not
// exposing the full credentials service, the provider package is not able to
// modify the credentials entities, only read them.
type ProviderService struct {
	st ProviderState
}

// CloudCredential returns the cloud credential for the given tag.
func (s *ProviderService) CloudCredential(ctx context.Context, key corecredential.Key) (cloud.Credential, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := key.Validate(); err != nil {
		return cloud.Credential{}, errors.Errorf("invalid id getting cloud credential: %w", err)
	}
	credInfo, err := s.st.CloudCredential(ctx, key)
	if err != nil {
		return cloud.Credential{}, errors.Capture(err)
	}
	cred := cloud.NewNamedCredential(credInfo.Label, cloud.AuthType(credInfo.AuthType), credInfo.Attributes, credInfo.Revoked)
	cred.Invalid = credInfo.Invalid
	cred.InvalidReason = credInfo.InvalidReason
	return cred, nil
}

// InvalidateCredential marks the cloud credential for the given key as invalid.
// The following errors can be expected:
// - [github.com/juju/juju/domain/credential/errors.NotFound] when the
// credential specified by key does not exist.
func (s *ProviderService) InvalidateCredential(ctx context.Context, key corecredential.Key, reason string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := key.Validate(); err != nil {
		return errors.Errorf("invalidating cloud credential with invalid key: %w", err)
	}
	uuid, err := s.st.CredentialUUIDForKey(ctx, key)
	if err != nil {
		return errors.Errorf("getting credential uuid for key %q: %w", key, err)
	}
	return s.st.InvalidateCloudCredential(ctx, uuid, reason)
}

// WatchableProviderService provides the API for working with credentials and
// the ability to create watchers.
type WatchableProviderService struct {
	ProviderService
	watcherFactory WatcherFactory
}

// NewWatchableProviderService returns a new service reference wrapping the
// input state.
func NewWatchableProviderService(st ProviderState, watcherFactory WatcherFactory) *WatchableProviderService {
	return &WatchableProviderService{
		ProviderService: ProviderService{
			st: st,
		},
		watcherFactory: watcherFactory,
	}
}

// WatchCredential returns a watcher that observes changes to the specified
// credential.
func (s *WatchableProviderService) WatchCredential(ctx context.Context, key corecredential.Key) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := key.Validate(); err != nil {
		return nil, errors.Errorf("watching cloud credential with invalid key: %w", err)
	}
	return s.st.WatchCredential(ctx, s.watcherFactory.NewNotifyWatcher, key)
}
