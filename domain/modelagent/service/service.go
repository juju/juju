// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/version/v2"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
)

type State interface {
	// CheckMachineExists check to see if the given machine exists in the model. If
	// the machine does not exist an error satisfying
	// [machineerrors.MachineNotFound] is returned.
	CheckMachineExists(context.Context, machine.Name) error

	// CheckUnitExists check to see if the given unit exists in the model. If
	// the unit does not exist an error satisfying
	// [applicationerrors.UnitNotFound] is returned.
	CheckUnitExists(context.Context, string) error

	// GetTargetAgentVersion returns the target agent version for this model.
	GetTargetAgentVersion(context.Context) (version.Number, error)

	// NamespaceForWatchAgentVersion returns the namespace identifier
	// to watch for the agent version.
	NamespaceForWatchAgentVersion() string
}

// WatcherFactory provides a factory for constructing new watchers.
type WatcherFactory interface {
	// NewNamespaceNotifyWatcher returns a new namespace notify watcher
	// for events based on the input change mask.
	NewNamespaceNotifyWatcher(namespace string, changeMask changestream.ChangeType) (watcher.NotifyWatcher, error)
}

// Service is used to get the target Juju agent version for the current model.
type Service struct {
	st             State
	watcherFactory WatcherFactory
}

// NewService returns a new [Service].
func NewService(st State, watcherFactory WatcherFactory) *Service {
	return &Service{
		st:             st,
		watcherFactory: watcherFactory,
	}
}

// GetMachineTargetAgentVersion reports the target agent version that should be
// running on the provided machine identified by name. The following errors are
// possible:
// - [machineerrors.MachineNotFound]
// - [github.com/juju/juju/domain/model/errors.AgentVersionNotFound]
func (s *Service) GetMachineTargetAgentVersion(
	ctx context.Context,
	machineName machine.Name,
) (version.Number, error) {
	err := s.st.CheckMachineExists(ctx, machineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return version.Zero, errors.Errorf("machine %q does not exist", machineName).Add(machineerrors.MachineNotFound)
	} else if err != nil {
		return version.Zero, errors.Errorf(
			"checking if machine %q exists when getting target agent version: %w",
			machineName, err,
		)
	}

	return s.st.GetTargetAgentVersion(ctx)
}

// GetUnitTargetAgentVersion reports the target agent version that should be
// being run on the provided unit identified by name. The following errors
// are possible:
// - [applicationerrors.UnitNotFound] - When the unit in question does not exist.
//   - [github.com/juju/juju/domain/model/errors.AgentVersionFound] if no
//     agent version record exists.
func (s *Service) GetUnitTargetAgentVersion(
	ctx context.Context,
	unitName string,
) (version.Number, error) {
	err := s.st.CheckUnitExists(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return version.Zero, errors.Errorf("unit %q does not exist", unitName).Add(applicationerrors.UnitNotFound)
	} else if err != nil {
		return version.Zero, errors.Errorf(
			"checking if unit %q exists when getting target agent version: %w",
			unitName, err,
		)
	}

	return s.st.GetTargetAgentVersion(ctx)
}

// GetModelTargetAgentVersion returns the agent version for the specified model.
// The following errors can be returned:
//   - [errors.NotValid] if the model ID is not valid;
//   - [github.com/juju/juju/domain/model/errors.AgentVersionFound] if no
//     agent version record exists.
func (s *Service) GetModelTargetAgentVersion(ctx context.Context) (version.Number, error) {
	return s.st.GetTargetAgentVersion(ctx)
}

// WatchMachineTargetAgentVersion is responsible for watching the target agent
// version for machine and reporting when there has been a change via a
// [watcher.NotifyWatcher]. The following errors can be expected:
// - [machineerrors.NotFound] - When no machine exists for the provided name.
// - [modelerrors.AgentVersionNotFound] - When there is no target version found.
func (s *Service) WatchMachineTargetAgentVersion(
	ctx context.Context,
	machineName machine.Name,
) (watcher.NotifyWatcher, error) {
	err := s.st.CheckMachineExists(ctx, machineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return nil, errors.Errorf("machine %q does not exist", machineName).Add(machineerrors.MachineNotFound)
	} else if err != nil {
		return nil, errors.Errorf(
			"checking if machine %q exists when watching target agent version: %w", machineName, err)
	}

	w, err := s.WatchModelTargetAgentVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting watcher for machine %q model target agent version: %w", machineName, err)
	}
	return w, nil
}

// WatchUnitTargetAgentVersion is responsible for watching the target agent
// version for unit and reporting when there has been a change via a
// [watcher.NotifyWatcher]. The following errors can be expected:
// - [applicationerrors.UnitNotFound] - When no unit exists for the provided name.
// - [modelerrors.AgentVersionNotFound] - When there is no target version found.
func (s *Service) WatchUnitTargetAgentVersion(
	ctx context.Context,
	unitName string,
) (watcher.NotifyWatcher, error) {
	err := s.st.CheckUnitExists(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return nil, errors.Errorf("unit %q does not exist", unitName).Add(applicationerrors.UnitNotFound)
	} else if err != nil {
		return nil, errors.Errorf("checking if unit %q exists when watching target agent version: %w", unitName, err)
	}

	w, err := s.WatchModelTargetAgentVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting watcher for unit %q model target agent version: %w", unitName, err)
	}
	return w, nil
}

// WatchModelTargetAgentVersion is responsible for watching the target agent
// version of this model and reporting when a change has happened in the
// version.
func (s *Service) WatchModelTargetAgentVersion(ctx context.Context) (watcher.NotifyWatcher, error) {
	w, err := s.watcherFactory.NewNamespaceNotifyWatcher(s.st.NamespaceForWatchAgentVersion(), changestream.All)
	if err != nil {
		return nil, errors.Errorf("creating watcher for agent version: %w", err)
	}
	return w, nil
}
