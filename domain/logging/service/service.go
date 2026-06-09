// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"net/url"

	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/logging"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// State defines an interface for interacting with the underlying state.
type State interface {
	// SetLokiConfig sets the Loki push API endpoint and CA certificate. Any
	// previously stored config is replaced. The uuid is the unique identifier
	// for the new config row.
	SetLokiConfig(ctx context.Context, uuid string, config logging.LokiConfig) error

	// GetLokiConfig returns the configured Loki push API endpoint and CA
	// certificate. If no endpoint is configured, an error satisfying
	// [loggingerrors.LokiConfigNotFound] is returned.
	GetLokiConfig(ctx context.Context) (logging.LokiConfig, error)

	// DeleteLokiConfig removes the configured Loki push API config. If no
	// config is configured, this is a no-op.
	DeleteLokiConfig(ctx context.Context) error

	// NamespaceForWatchLokiConfig returns the namespace identifier used
	// for watching Loki config changes.
	NamespaceForWatchLokiConfig() string
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

// SetLokiConfig sets the Loki push API endpoint and CA certificate. The
// endpoint must be non-empty; an error is returned otherwise.
func (s *Service) SetLokiConfig(ctx context.Context, config logging.LokiConfig) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if config.Endpoint == "" {
		return errors.Errorf("empty loki endpoint").Add(coreerrors.NotValid)
	}

	u, err := url.Parse(config.Endpoint)
	if err != nil {
		return errors.Errorf("loki endpoint %q: %w", config.Endpoint, err).Add(coreerrors.NotValid)
	}
	if u.Scheme == "" || u.Host == "" {
		return errors.Errorf("loki endpoint %q missing scheme or host", config.Endpoint).Add(coreerrors.NotValid)
	}

	id, err := uuid.NewUUID()
	if err != nil {
		return errors.Errorf("generating UUID: %w", err)
	}

	return s.st.SetLokiConfig(ctx, id.String(), config)
}

// GetLokiConfig returns the configured Loki push API endpoint and CA
// certificate. If no endpoint is configured, an error satisfying
// [loggingerrors.LokiConfigNotFound] is returned.
func (s *Service) GetLokiConfig(ctx context.Context) (logging.LokiConfig, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.GetLokiConfig(ctx)
}

// DeleteLokiConfig removes the configured Loki push API config.
// If no config is configured, this is a no-op.
func (s *Service) DeleteLokiConfig(ctx context.Context) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.DeleteLokiConfig(ctx)
}

// WatchLokiConfig returns a watcher that emits notifications when the Loki push
// API endpoint or CA certificate configuration changes.
func (s *WatchableService) WatchLokiConfig(ctx context.Context) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	namespace := s.st.NamespaceForWatchLokiConfig()

	return s.watcherFactory.NewNotifyWatcher(
		ctx,
		"loki config watcher",
		eventsource.NamespaceFilter(namespace, changestream.All),
	)
}
