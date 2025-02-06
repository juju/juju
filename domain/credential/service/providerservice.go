// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/credential"
)

// ProviderState describes retrieval and persistence methods for storage.
type ProviderState interface {
	// CloudCredential returns the cloud credential for the given name, cloud,
	// owner.
	CloudCredential(ctx context.Context, key corecredential.Key) (credential.CloudCredentialResult, error)

	// InvalidateCloudCredential marks the cloud credential for the given name,
	// cloud, owner as invalid.
	InvalidateCloudCredential(ctx context.Context, key corecredential.Key, reason string) error

	// WatchCredential returns a new NotifyWatcher watching for changes to the
	// specified credential.
	WatchCredential(
		ctx context.Context,
		getWatcher func(string, string, changestream.ChangeType) (watcher.NotifyWatcher, error),
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

// NewProviderService returns a new service reference wrapping the input state.
func NewProviderService(st ProviderState) *ProviderService {
	return &ProviderService{
		st: st,
	}
}

// CloudCredential returns the cloud credential for the given tag.
func (s *ProviderService) CloudCredential(ctx context.Context, key corecredential.Key) (cloud.Credential, error) {
	if err := key.Validate(); err != nil {
		return cloud.Credential{}, errors.Annotate(err, "invalid id getting cloud credential")
	}
	credInfo, err := s.st.CloudCredential(ctx, key)
	if err != nil {
		return cloud.Credential{}, errors.Trace(err)
	}
	cred := cloud.NewNamedCredential(credInfo.Label, cloud.AuthType(credInfo.AuthType), credInfo.Attributes, credInfo.Revoked)
	cred.Invalid = credInfo.Invalid
	cred.InvalidReason = credInfo.InvalidReason
	return cred, nil
}

// InvalidateCredential marks the cloud credential for the given name, cloud,
// owner as invalid.
func (s *ProviderService) InvalidateCredential(ctx context.Context, key corecredential.Key, reason string) error {
	if err := key.Validate(); err != nil {
		return errors.Annotatef(err, "invalid id invalidating cloud credential")
	}
	return s.st.InvalidateCloudCredential(ctx, key, reason)
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
	if err := key.Validate(); err != nil {
		return nil, errors.Annotatef(err, "invalid id watching cloud credential")
	}
	return s.st.WatchCredential(ctx, s.watcherFactory.NewValueWatcher, key)
}
