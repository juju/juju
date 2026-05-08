// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/internal/errors"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	// SetLokiEndpoint sets the Loki push API endpoint. Any previously stored
	// endpoint is replaced.
	SetLokiEndpoint(ctx context.Context, endpoint string) error

	// GetLokiEndpoint returns the configured Loki push API endpoint. If no
	// endpoint is configured, an error satisfying
	// [loggingerrors.LokiEndpointNotFound] is returned.
	GetLokiEndpoint(ctx context.Context) (string, error)

	// DeleteLokiEndpoint removes the configured Loki push API endpoint. If
	// no endpoint is configured, this is a no-op.
	DeleteLokiEndpoint(ctx context.Context) error

	// NamespaceForWatchLokiEndpoint returns the namespace identifier used
	// for watching Loki endpoint changes.
	NamespaceForWatchLokiEndpoint() string
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNotifyWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. Change-log events will be emitted
	// only if the filter accepts them, and dispatching the notifications
	// via the Changes channel. A filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
		ctx context.Context,
		summary string,
		filterOption eventsource.FilterOption,
		filterOptions ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
}

// Service defines a service for interacting with the underlying state.
type Service struct {
	st State
}

// NewService returns a new Service for interacting with the underlying state.
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// WatchableService defines a service for interacting with the underlying
// state and the ability to create watchers.
type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new Service for interacting with the
// underlying state and the ability to create watchers.
func NewWatchableService(st State, wf WatcherFactory) *WatchableService {
	return &WatchableService{
		Service: Service{
			st: st,
		},
		watcherFactory: wf,
	}
}

// SetLokiEndpoint sets the Loki push API endpoint. The endpoint must be
// non-empty; an error is returned otherwise.
func (s *Service) SetLokiEndpoint(ctx context.Context, endpoint string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if endpoint == "" {
		return errors.Errorf("empty loki endpoint").Add(coreerrors.NotValid)
	}

	return s.st.SetLokiEndpoint(ctx, endpoint)
}

// GetLokiEndpoint returns the configured Loki push API endpoint.
// If no endpoint is configured, an error satisfying
// [loggingerrors.LokiEndpointNotFound] is returned.
func (s *Service) GetLokiEndpoint(ctx context.Context) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.GetLokiEndpoint(ctx)
}

// DeleteLokiEndpoint removes the configured Loki push API endpoint.
// If no endpoint is configured, this is a no-op.
func (s *Service) DeleteLokiEndpoint(ctx context.Context) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.DeleteLokiEndpoint(ctx)
}

// WatchLokiEndpoint returns a watcher that emits notifications when the
// Loki push API endpoint configuration changes.
func (s *WatchableService) WatchLokiEndpoint(ctx context.Context) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	namespace := s.st.NamespaceForWatchLokiEndpoint()

	return s.watcherFactory.NewNotifyWatcher(
		ctx,
		"loki endpoint watcher",
		eventsource.NamespaceFilter(namespace, changestream.All),
	)
}
