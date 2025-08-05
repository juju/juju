// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/internal/errors"
)

type ProviderState interface {
	// Cloud returns the cloud with the specified name.
	Cloud(context.Context, string) (*cloud.Cloud, error)

	// WatchCloud returns a new NotifyWatcher watching for changes to the specified cloud.
	WatchCloud(
		ctx context.Context,
		getWatcher func(
			ctx context.Context,
			filter eventsource.FilterOption,
			filterOpts ...eventsource.FilterOption,
		) (watcher.NotifyWatcher, error),
		name string,
	) (watcher.NotifyWatcher, error)
}

// ProviderService provides the API for working with clouds.
// The provider service is a subset of the cloud service, and is used by the
// provider package to interact with the cloud service. By not exposing the
// full cloud service, the provider package is not able to modify the cloud
// entities, only read them.
type ProviderService struct {
	st ProviderState
}

// NewProviderService returns a new service reference wrapping the input state.
func NewProviderService(st ProviderState) *ProviderService {
	return &ProviderService{
		st: st,
	}
}

// Cloud returns the named cloud.
func (s *ProviderService) Cloud(ctx context.Context, name string) (*cloud.Cloud, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	cloud, err := s.st.Cloud(ctx, name)
	return cloud, errors.Capture(err)
}

// WatchableProviderService returns a new provider service reference wrapping
// the input state and watcher factory.
type WatchableProviderService struct {
	ProviderService
	watcherFactory WatcherFactory
}

// NewWatchableProviderService returns a new service reference wrapping the
// input state and watcher factory.
func NewWatchableProviderService(st ProviderState, watcherFactory WatcherFactory) *WatchableProviderService {
	return &WatchableProviderService{
		ProviderService: ProviderService{
			st: st,
		},
		watcherFactory: watcherFactory,
	}
}

// WatchCloud returns a watcher that observes changes to the specified cloud.
func (s *WatchableProviderService) WatchCloud(ctx context.Context, name string) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.WatchCloud(ctx, s.watcherFactory.NewNotifyWatcher, name)
}
