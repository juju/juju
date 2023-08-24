// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher"
	credentialstate "github.com/juju/juju/domain/credential/state"
)

// WatcherFactory instances return a watcher for a specified credential UUID,
type WatcherFactory interface {
	NewValueWatcher(
		namespace, uuid string, changeMask changestream.ChangeType,
	) (watcher.NotifyWatcher, error)
}

// State describes retrieval and persistence methods for credentials.
type State interface {
	// UpsertCloudCredential adds or updates a cloud credential with the given name, cloud, owner.
	UpsertCloudCredential(ctx context.Context, name, cloudName, owner string, credential cloud.Credential) error

	// CloudCredentials returns the user's cloud credentials for a given cloud,
	// keyed by credential name.
	CloudCredentials(ctx context.Context, owner, cloudName string) (map[string]cloud.Credential, error)

	// CloudCredential returns the cloud credential for the given name, cloud, owner.
	CloudCredential(ctx context.Context, name, cloudName, owner string) (cloud.Credential, error)

	// AllCloudCredentials returns all cloud credentials stored on the controller
	// for a given user.
	AllCloudCredentials(ctx context.Context, owner string) ([]credentialstate.CloudCredential, error)

	// RemoveCloudCredential removes a cloud credential with the given name, cloud, owner.
	RemoveCloudCredential(ctx context.Context, name, cloudName, owner string) error

	// WatchCredential returns a new NotifyWatcher watching for changes to the specified credential.
	WatchCredential(
		ctx context.Context,
		getWatcher func(string, string, changestream.ChangeType) (watcher.NotifyWatcher, error),
		name, cloudName, owner string,
	) (watcher.NotifyWatcher, error)
}

// Logger facilitates emitting log messages.
type Logger interface {
	Debugf(string, ...interface{})
}

// Service provides the API for working with external controllers.
type Service struct {
	st             State
	watcherFactory WatcherFactory
}

// NewService returns a new service reference wrapping the input state.
func NewService(st State, watcherFactory WatcherFactory) *Service {
	return &Service{
		st:             st,
		watcherFactory: watcherFactory,
	}
}

// CloudCredential represents a credential and the cloud it belongs to.
type CloudCredential struct {
	Credential cloud.Credential
	CloudName  string
}

// CloudCredential returns the cloud credential for the given tag.
func (s *Service) CloudCredential(ctx context.Context, tag names.CloudCredentialTag) (cloud.Credential, error) {
	return s.st.CloudCredential(ctx, tag.Name(), tag.Cloud().Id(), tag.Owner().Id())
}

// AllCloudCredentials returns all cloud credentials stored on the controller
// for a given user.
func (s *Service) AllCloudCredentials(ctx context.Context, user string) ([]CloudCredential, error) {
	creds, err := s.st.AllCloudCredentials(ctx, user)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]CloudCredential, len(creds))
	for i, c := range creds {
		result[i] = CloudCredential{Credential: c.Credential, CloudName: c.CloudName}
	}
	return result, nil
}

// CloudCredentials returns the user's cloud credentials for a given cloud,
// keyed by credential name.
func (s *Service) CloudCredentials(ctx context.Context, user, cloudName string) (map[string]cloud.Credential, error) {
	return s.st.CloudCredentials(ctx, user, cloudName)
}

// UpdateCloudCredential adds or updates a cloud credential with the given tag.
func (s *Service) UpdateCloudCredential(ctx context.Context, tag names.CloudCredentialTag, cred cloud.Credential) error {
	return s.st.UpsertCloudCredential(ctx, tag.Name(), tag.Cloud().Id(), tag.Owner().Id(), cred)
}

// RemoveCloudCredential removes a cloud credential with the given tag.
func (s *Service) RemoveCloudCredential(ctx context.Context, tag names.CloudCredentialTag) error {
	return s.st.RemoveCloudCredential(ctx, tag.Name(), tag.Cloud().Id(), tag.Owner().Id())
}

// WatchCredential returns a watcher that observes changes to the specified credential.
func (s *Service) WatchCredential(ctx context.Context, tag names.CloudCredentialTag) (watcher.NotifyWatcher, error) {
	if s.watcherFactory != nil {
		return s.st.WatchCredential(ctx, s.watcherFactory.NewValueWatcher, tag.Name(), tag.Cloud().Id(), tag.Owner().Id())
	}
	return nil, errors.NotYetAvailablef("credential watcher")
}
