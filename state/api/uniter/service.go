// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
)

// This module implements a subset of the interface provided by
// state.Service, as needed by the uniter API.

// TODO: Only the required calls are added as placeholders,
// the actual implementation will come in a follow-up.

// Service represents the state of a service.
type Service struct {
	st   *State
	tag  string
	life params.Life
	// TODO: Add fields.
}

// Name returns the service name.
func (s *Service) Name() string {
	_, serviceName, err := names.ParseTag(s.tag, names.ServiceTagKind)
	if err != nil {
		panic(fmt.Sprintf("%q is not a valid service tag", s.tag))
	}
	return serviceName
}

// String returns the service as a string.
func (s *Service) String() string {
	return s.Name()
}

// Watch returns a watcher for observing changes to a service.
func (s *Service) Watch() (*watcher.NotifyWatcher, error) {
	// TODO: Call Uniter.Watch(), passing the service tag as argument,
	// then start a client NotifyWatcher, like uniter.Unit.Watch()
	// does.
	panic("not implemented")
}

// WatchRelations returns a StringsWatcher that notifies of changes to
// the lifecycles of relations involving s.
func (s *Service) WatchRelations() (*watcher.StringsWatcher, error) {
	// TODO: Call Uniter.WatchServiceRelations(), passing the service
	// tag as argument, then start a client StringsWatcher, like
	// deployer.Machine.WatchUnits() does.
	panic("not implemented")
}

// Life returns the service's current life state.
func (s *Service) Life() params.Life {
	return s.life
}

// Refresh refreshes the contents of the Service from the underlying
// state.
func (s *Service) Refresh() error {
	// TODO: Call Uniter.Life(), passing the service tag as argument.
	// Update s.life accordingly after getting the result.
	panic("not implemented")
}

// CharmURL returns the service's charm URL, and whether units should
// upgrade to the charm with that URL even if they are in an error
// state.
func (s *Service) CharmURL() (curl *charm.URL, force bool) {
	// TODO: Call Uniter.CharmURL(), passing the service tag as
	// argument.
	panic("not implemented")
}
