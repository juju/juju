// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/ssh"
	"github.com/juju/juju/internal/errors"
)

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	NewNamespaceWatcher(
		ctx context.Context,
		query eventsource.NamespaceQuery,
		summary string,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)
}

// Service provides model-scoped access to SSH connection requests.
type Service struct {
	st             State
	watcherFactory WatcherFactory
	clock          clock.Clock
}

// NewService returns a new SSH connection request service.
func NewService(state State, watcherFactory WatcherFactory, clock clock.Clock) *Service {
	return &Service{
		st:             state,
		watcherFactory: watcherFactory,
		clock:          clock,
	}
}

// InsertSSHConnRequest stores a one-shot SSH connection request for the
// current model.
func (s *Service) InsertSSHConnRequest(ctx context.Context, req ssh.SSHConnRequest) error {
	if err := s.validateRequest(req); err != nil {
		return errors.Errorf("validating ssh connection request: %w", err)
	}
	return s.st.InsertSSHConnRequest(ctx, req, s.clock.Now())
}

// GetSSHConnRequest returns a request by tunnel ID.
func (s *Service) GetSSHConnRequest(ctx context.Context, tunnelID string) (ssh.SSHConnRequest, error) {
	if tunnelID == "" {
		return ssh.SSHConnRequest{}, errors.Errorf("empty tunnel id").Add(coreerrors.NotValid)
	}
	return s.st.GetSSHConnRequest(ctx, tunnelID, s.clock.Now())
}

// WatchSSHConnRequest returns a watcher that emits changed tunnel IDs.
func (s *Service) WatchSSHConnRequest(ctx context.Context) (watcher.StringsWatcher, error) {
	if err := s.st.PruneExpiredSSHConnRequests(ctx, s.clock.Now()); err != nil {
		return nil, errors.Errorf("pruning expired ssh connection requests: %w", err)
	}

	table, stmt := s.st.InitialWatchSSHConnRequestsStatement()
	return s.watcherFactory.NewNamespaceWatcher(
		ctx,
		eventsource.InitialNamespaceChanges(stmt),
		"ssh connection request watcher",
		eventsource.NamespaceFilter(table, changestream.All),
	)
}

// RemoveSSHConnRequest deletes a request by tunnel ID.
func (s *Service) RemoveSSHConnRequest(ctx context.Context, tunnelID string) error {
	if tunnelID == "" {
		return errors.Errorf("empty tunnel id").Add(coreerrors.NotValid)
	}
	return s.st.RemoveSSHConnRequest(ctx, tunnelID)
}

func (s *Service) validateRequest(req ssh.SSHConnRequest) error {
	if req.TunnelID == "" {
		return errors.Errorf("empty tunnel id").Add(coreerrors.NotValid)
	}
	if err := coremachine.Name(req.MachineID).Validate(); err != nil {
		return errors.Errorf("validating machine id %q: %w", req.MachineID, err)
	}
	if req.Username == "" {
		return errors.Errorf("empty username").Add(coreerrors.NotValid)
	}
	if req.Password == "" {
		return errors.Errorf("empty password").Add(coreerrors.NotValid)
	}
	if req.Expires.IsZero() {
		return errors.Errorf("empty expiry").Add(coreerrors.NotValid)
	}
	return nil
}
