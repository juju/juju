// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
)

// WatchableService provides the API for working with external controllers
// and the ability to create watchers.
type WatchableService struct {
	ProviderService
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new watchable service reference wrapping the
// input state and provider.
func NewWatchableService(st State,
	providerWithNetworking providertracker.ProviderGetter[ProviderWithNetworking],
	providerWithZones providertracker.ProviderGetter[ProviderWithZones],
	watcherFactory WatcherFactory, logger logger.Logger) *WatchableService {
	return &WatchableService{
		ProviderService: ProviderService{
			Service: Service{
				st:     st,
				logger: logger,
			},
			providerWithNetworking: providerWithNetworking,
			providerWithZones:      providerWithZones,
		},
		watcherFactory: watcherFactory,
	}
}

// WatchSubnets returns a watcher that observes changes to subnets and their
// association (fan underlays), filtered based on the provided list of subnets
// to watch.
func (s *WatchableService) WatchSubnets(ctx context.Context, subnetUUIDsToWatch set.Strings) (watcher.StringsWatcher, error) {
	filter := subnetUUIDsFilter(subnetUUIDsToWatch)

	return s.watcherFactory.NewNamespaceMapperWatcher(
		s.st.AllSubnetsQuery,
		eventsource.FilterEvents(filter),
		eventsource.NamespaceFilter(s.st.NamespaceForWatchSubnet(), changestream.All),
	)
}

// subnetUUIDsFilter filters the returned subnet UUIDs from the changelog
// according to the user-provided list of subnet UUIDs.
// To keep the compatibility with legacy watchers, if the input set of subnets
// is empty then no filtering is applied.
func subnetUUIDsFilter(subnetUUIDsToWatch set.Strings) func(changestream.ChangeEvent) bool {
	if subnetUUIDsToWatch.IsEmpty() {
		return func(_ changestream.ChangeEvent) bool { return true }
	}
	return func(event changestream.ChangeEvent) bool {
		return subnetUUIDsToWatch.Contains(event.Changed())
	}
}
