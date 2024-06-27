// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/juju/core/changestream"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
)

// ControllerKeyProvider is responsible for providing controller wide authorised
// keys that should be included as part of every machine.
type ControllerKeyProvider interface {
	// ControllerKeys returns controller wide authorised keys.
	ControllerKeys(context.Context) ([]string, error)
}

// Service provides the means for interacting with the authorised keys in a
// model.
type Service struct {
	// controllerKeyProvider is a reference to a [ControllerKeyProvider] that
	// can be used for fetching controller wide authorised keys.
	controllerKeyProvider ControllerKeyProvider

	// st is a reference to [State] for getting model authorised keys.
	st State
}

// WatcherFactory describes the methods required for creating new watchers
// for key updates.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new namespace watcher for events based on
	// the input change mask.
	NewNamespaceWatcher(string, changestream.ChangeType, eventsource.NamespaceQuery) (watcher.StringsWatcher, error)
}

// WatchableService is a normal [Service] that can also be watched for updates
// to authorised key values.
type WatchableService struct {
	// Service is the inherited service to extend upon.
	Service

	st WatchableState

	// watcherFactory is the factory to use for generating new watchers for
	// authorised key changes.
	watcherFactory WatcherFactory
}

// State provides the access layer the [Service] needs for persisting and
// retrieving a models authorised keys.
type State interface {
	// AuthorisedKeysForMachine returns the set of authorised keys for a machine
	// in this model. If no machine exist for the given name a
	// [github.com/juju/juju/domain/machine/errors.NotFound] error will be
	// returned.
	AuthorisedKeysForMachine(context.Context, coremachine.Name) ([]string, error)
}

// WatchableState provides the access layer the [WatchableService] needs for
// persisting and retrieving a models authorised keys.
type WatchableState interface {
	State

	// AllPublicKeysQuery is used to get the initial state for the keys watcher.
	AllPublicKeysQuery() string
}

// NewService constructs a new [Service] for gathering up the authorised keys
// within a model.
func NewService(
	controllerKeyProvider ControllerKeyProvider,
	st State,
) *Service {
	return &Service{
		controllerKeyProvider: controllerKeyProvider,
		st:                    st,
	}
}

// NewWatchableService creates a new [WatchableService] for consuming changes in
// authorised keys for a model.
func NewWatchableService(
	controllerKeyProvider ControllerKeyProvider,
	st WatchableState,
	watcherFactory WatcherFactory,
) *WatchableService {
	return &WatchableService{
		Service: Service{
			controllerKeyProvider: controllerKeyProvider,
			st:                    st,
		},
		st:             st,
		watcherFactory: watcherFactory,
	}
}

// AuthorisedKeysForMachine is responsible for fetching the authorised keys that
// should be available on a machine. The following errors can be expected:
// - [github.com/juju/errors.NotValid] if the machine id is not valid.
// - [github.com/juju/juju/domain/machine/errors.NotFound] if the machine does
// not exist.
func (s *Service) AuthorisedKeysForMachine(
	ctx context.Context,
	machineName coremachine.Name,
) ([]string, error) {
	if err := machineName.Validate(); err != nil {
		return nil, fmt.Errorf(
			"cannot get authorised keys for machine %q: %w",
			machineName, err,
		)
	}

	machineKeys, err := s.st.AuthorisedKeysForMachine(ctx, machineName)
	if err != nil {
		return nil, fmt.Errorf(
			"gathering authorised keys for machine %q: %w",
			machineName, err,
		)
	}

	controllerKeys, err := s.controllerKeyProvider.ControllerKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"getting controller authorised keys for machine %q: %w",
			machineName, err,
		)
	}

	return append(machineKeys, controllerKeys...), nil
}

// WatchAuthorisedKeysForMachine will watch for authorised key changes for a
// give machine name. The following errors can be expected:
// - [github.com/juju/errors.NotValid] if the machine id is not valid.
func (s *WatchableService) WatchAuthorisedKeysForMachine(
	ctx context.Context,
	machineName coremachine.Name,
) (watcher.StringsWatcher, error) {
	if err := machineName.Validate(); err != nil {
		return nil, fmt.Errorf(
			"cannot watch authorised keys for machine %q: %w", machineName, err,
		)
	}

	return s.watcherFactory.NewNamespaceWatcher(
		"user_public_ssh_key",
		changestream.All,
		eventsource.InitialNamespaceChanges(s.st.AllPublicKeysQuery()),
	)
}
