// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/changestream"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
)

// ControllerKeyProvider is responsible for providing controller wide authorised
// keys that should be included as part of every machine.
type ControllerKeyProvider interface {
	// ControllerAuthorisedKeys returns controller wide authorised keys.
	ControllerAuthorisedKeys(context.Context) ([]string, error)
}

// Service provides the means for interacting with the authorised keys in a
// model.
type Service struct {
	// controllerKeyProvider is a reference to a [ControllerKeyProvider] that
	// can be used for fetching controller wide authorised keys.
	controllerKeyProvider ControllerKeyProvider

	// controllerSt is a reference to [ControllerState] for getting model
	// authorized key information.
	controllerSt ControllerState

	// st is a reference to [State] for getting model based information for
	// authorized keys.
	st State
}

// WatcherFactory describes the methods required for creating new watchers
// for key updates.
type WatcherFactory interface {
	// NewNotifyWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. Change-log events will be emitted only if
	// the filter accepts them, and dispatching the notifications via the
	// Changes channel. A filter option is required, though additional filter
	// options can be provided.
	NewNotifyWatcher(eventsource.FilterOption, ...eventsource.FilterOption) (watcher.NotifyWatcher, error)
}

// WatchableService is a normal [Service] that can also be watched for updates
// to authorised key values.
type WatchableService struct {
	// Service is the inherited service to extend upon.
	Service

	// watcherFactory is the factory to use for generating new watchers for
	// authorised key changes.
	watcherFactory WatcherFactory
}

// State provides the access layer the [Service] needs for persisting and
// retrieving a models authorised keys.
type State interface {
	// GetModelUUID returns the unique uuid for the model represented by this state.
	GetModelUUID(context.Context) (model.UUID, error)

	// CheckMachineExists check to see if the given machine exists in the model. If
	// the machine does not exist an error satisfying
	// [github.com/juju/juju/domain/machine/errors.MachineNotFound] is returned.
	CheckMachineExists(context.Context, coremachine.Name) error

	// NamespaceForWatchUserAuthentication returns the namespace used to
	// monitor user authentication changes.
	NamespaceForWatchUserAuthentication() string

	// NamespaceForWatchModelAuthorizationKeys returns the namespace used to
	// monitor authorization keys for the current model.
	NamespaceForWatchModelAuthorizationKeys() string
}

// ControllerState provides the access layer the [Service] needs for retrieving
// user based authorized key information.
type ControllerState interface {
	// GetUserAuthorizedKeysForModel is responsible for returning all of the
	// user authorized keys for a model.
	// The following errors can be expected:
	// - [modelerrors.NotFound] if the model does not exist.
	GetUserAuthorizedKeysForModel(context.Context, model.UUID) ([]string, error)
}

// NewService constructs a new [Service] for gathering up the authorised keys
// within a model.
func NewService(
	controllerKeyProvider ControllerKeyProvider,
	controllerState ControllerState,
	st State,
) *Service {
	return &Service{
		controllerSt:          controllerState,
		controllerKeyProvider: controllerKeyProvider,
		st:                    st,
	}
}

// NewWatchableService creates a new [WatchableService] for consuming changes in
// authorised keys for a model.
func NewWatchableService(
	controllerKeyProvider ControllerKeyProvider,
	controllerState ControllerState,
	st State,
	watcherFactory WatcherFactory,
) *WatchableService {
	return &WatchableService{
		Service:        *NewService(controllerKeyProvider, controllerState, st),
		watcherFactory: watcherFactory,
	}
}

// GetAuthorisedKeysForMachine is responsible for fetching the authorised keys
// that should be available on a machine. The following errors can be expected:
// - [github.com/juju/juju/core/errors.NotValid] if the machine id is not valid.
// - [github.com/juju/juju/domain/machine/errors.NotFound] if the machine does
// not exist.
func (s *Service) GetAuthorisedKeysForMachine(
	ctx context.Context,
	machineName coremachine.Name,
) ([]string, error) {
	if err := machineName.Validate(); err != nil {
		return nil, errors.Errorf(
			"validating machine name when getting authorized keys for machine: %w",
			err,
		)
	}

	err := s.st.CheckMachineExists(ctx, machineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return nil, errors.Errorf(
			"machine %q does not exist", machineName,
		).Add(machineerrors.MachineNotFound)
	} else if err != nil {
		return nil, errors.Errorf(
			"determining if machine %q exists when getting authorized keys for machine: %w",
			machineName, err,
		)
	}

	modelId, err := s.st.GetModelUUID(ctx)
	if err != nil {
		return nil, errors.Errorf(
			"getting model id when establishing authorized keys for machine %q: %w",
			machineName, err,
		)
	}

	userKeys, err := s.controllerSt.GetUserAuthorizedKeysForModel(ctx, modelId)
	if err != nil {
		return nil, errors.Errorf(
			"getting authorized keys for machine %q: %w",
			machineName, err,
		)
	}

	controllerKeys, err := s.controllerKeyProvider.ControllerAuthorisedKeys(ctx)
	if err != nil {
		return nil, errors.Errorf(
			"getting controller authorised keys for machine %q: %w",
			machineName, err,
		)
	}

	return append(userKeys, controllerKeys...), nil
}

// WatchAuthorisedKeysForMachine will watch for authorised key changes for a
// give machine name. The following errors can be expected:
// - [github.com/juju/juju/core/errors.NotValid] if the machine id is not valid.
// - [machineerrors.MachineNotFound] if no machine exists for the provided name.
func (s *WatchableService) WatchAuthorisedKeysForMachine(
	ctx context.Context,
	machineName coremachine.Name,
) (watcher.NotifyWatcher, error) {
	if err := machineName.Validate(); err != nil {
		return nil, errors.Errorf(
			"validating machine name when getting authorized keys for machine: %w",
			err,
		)
	}

	err := s.st.CheckMachineExists(ctx, machineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return nil, errors.Errorf(
			"watching authorized keys for machine %q, machine does not exist",
			machineName,
		).Add(machineerrors.MachineNotFound)
	}

	modelId, err := s.st.GetModelUUID(ctx)
	if err != nil {
		return nil, errors.Errorf(
			"getting model id for machine %q while watching authorized key changes: %w",
			machineName, err,
		)
	}

	return s.watcherFactory.NewNotifyWatcher(
		eventsource.PredicateFilter(
			s.st.NamespaceForWatchModelAuthorizationKeys(),
			changestream.All,
			eventsource.EqualsPredicate(modelId.String()),
		),
		eventsource.NamespaceFilter(
			s.st.NamespaceForWatchUserAuthentication(),
			changestream.All,
		),
	)
}

// GetInitialAuthorisedKeysForContainer returns the authorised keys to be used
// when provisioning a new container for the model.
func (s *Service) GetInitialAuthorisedKeysForContainer(ctx context.Context) ([]string, error) {
	modelId, err := s.st.GetModelUUID(ctx)
	if err != nil {
		return nil, errors.Errorf(
			"getting model id when establishing initial authorized keys for container: %w",
			err,
		)
	}

	userKeys, err := s.controllerSt.GetUserAuthorizedKeysForModel(ctx, modelId)
	if err != nil {
		return nil, errors.Errorf(
			"getting initial authorized keys for container: %w", err,
		)
	}

	controllerKeys, err := s.controllerKeyProvider.ControllerAuthorisedKeys(ctx)
	if err != nil {
		return nil, errors.Errorf(
			"getting controller authorised keys for container: %w", err,
		)
	}

	return append(userKeys, controllerKeys...), nil
}
