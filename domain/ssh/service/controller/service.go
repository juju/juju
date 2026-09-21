// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/internal/errors"
)

// Service provides controller-scoped SSH host key workflows.
type Service struct {
	state State
}

// NewService returns a new controller SSH service.
func NewService(state State) *Service {
	return &Service{state: state}
}

// GetSSHServerPort returns the port the controller SSH jump server listens on.
// If no port has been set (e.g. before the controller charm has pushed a
// value), the default SSH server port is returned.
func (s *Service) GetSSHServerPort(ctx context.Context) (int, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	port, err := s.state.GetSSHServerPort(ctx)
	if errors.Is(err, coreerrors.NotFound) {
		return controller.DefaultSSHServerPort, nil
	}
	if err != nil {
		return 0, errors.Errorf("getting controller SSH server port: %w", err)
	}
	return port, nil
}

// SetSSHServerPort sets the port the controller SSH jump server listens on.
func (s *Service) SetSSHServerPort(ctx context.Context, port int) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.state.SetSSHServerPort(ctx, port); err != nil {
		return errors.Errorf("setting controller SSH server port: %w", err)
	}
	return nil
}

// SSHServerHostKey returns the controller jump host key.
func (s *Service) SSHServerHostKey(ctx context.Context) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	key, err := s.state.GetSSHServerHostKey(ctx)
	if err != nil {
		return "", errors.Errorf("getting controller SSH server host key: %w", err)
	}
	return key, nil
}

// SSHServerHostPublicKey returns the marshalled public host key of the
// controller SSH jump server. The public key is derived once at bootstrap and
// stored alongside the private key, so this method simply reads the stored
// value and never handles private key material.
func (s *Service) SSHServerHostPublicKey(ctx context.Context) ([]byte, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	key, err := s.state.GetSSHServerHostPublicKey(ctx)
	if err != nil {
		return nil, errors.Errorf("getting controller SSH server host public key: %w", err)
	}
	return key, nil
}

// GetPublicKeysForUser returns all public SSH keys registered for a user.
func (s *Service) GetPublicKeysForUser(ctx context.Context, username user.Name) ([]coressh.PublicKey, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	keys, err := s.state.GetPublicKeysForUser(ctx, username)
	if err != nil {
		return nil, errors.Errorf("getting public SSH keys for user %q: %w", username, err)
	}
	return keys, nil
}

// PublicKeyInModel reports whether key is authorized for username in modelUUID.
func (s *Service) PublicKeyInModel(ctx context.Context, modelUUID coremodel.UUID, username user.Name, key gossh.PublicKey) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := modelUUID.Validate(); err != nil {
		return false, errors.Errorf("validating model UUID %q: %w", modelUUID, err)
	}

	fingerprint := gossh.FingerprintSHA256(key)
	found, err := s.state.MatchesPublicKeyInModelForUser(ctx, modelUUID.String(), username.Name(), fingerprint)
	if err != nil {
		return false, errors.Errorf("checking public SSH key for user %q: %w", username, err)
	}
	return found, nil
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNotifyWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue.
	NewNotifyWatcher(
		ctx context.Context,
		summary string,
		filterOption eventsource.FilterOption,
		filterOptions ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
}

// WatchableService provides controller-scoped SSH workflows plus the ability
// to watch for changes to the SSH server configuration.
type WatchableService struct {
	*Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new watchable controller SSH service.
func NewWatchableService(state State, watcherFactory WatcherFactory) *WatchableService {
	return &WatchableService{
		Service:        NewService(state),
		watcherFactory: watcherFactory,
	}
}

// WatchSSHServerPort returns a watcher that notifies when the controller SSH
// server port changes.
func (s *WatchableService) WatchSSHServerPort(ctx context.Context) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.watcherFactory.NewNotifyWatcher(
		ctx,
		"controller SSH server port watcher",
		eventsource.NamespaceFilter(s.state.NamespaceForWatchSSHServerPort(), changestream.All),
	)
}
