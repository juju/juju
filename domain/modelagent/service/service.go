// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/version/v2"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
)

type ModelState interface {
	// CheckApplicationExists check to see if the given machine exists in the
	// model. If the machine does not exist an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	CheckApplicationExists(context.Context, string) error

	// CheckMachineExists check to see if the given machine exists in the model. If
	// the machine does not exist an error satisfying
	// [machineerrors.MachineNotFound] is returned.
	CheckMachineExists(context.Context, machine.Name) error

	// GetModelUUID returns the unique uuid for the model represented by this
	// state.
	GetModelUUID(context.Context) (model.UUID, error)

	// CheckUnitExists check to see if the given unit exists in the model. If
	// the unit does not exist an error satisfying
	// [applicationerrors.UnitNotFound] is returned.
	CheckUnitExists(context.Context, string) error
}

// State provides the state methods needed by the modelagent service.
type State interface {
	// GetModelTargetAgentVersion is responsible for returning the target
	// version the model is currently targeting for agents.
	GetModelTargetAgentVersion(context.Context, model.UUID) (version.Number, error)
}

// Service is a modelagent service which can be used to get the running Juju
// agent version for any given model.
type Service struct {
	st State
}

// WatcherFactory provides a factory for constructing new watchers.
type WatcherFactory interface {
	// NewValueWatcher returns a watcher for a particular change value in a
	// namespace, based on the input change mask.
	NewValueWatcher(
		string,
		string,
		changestream.ChangeType,
	) (watcher.NotifyWatcher, error)
}

// NewService returns a new modelagent service.
func NewService(st State) *Service {
	return &Service{st: st}
}

// ModelService is a modelagent service which can be used to get the running
// Juju agent version for the current model.
type ModelService struct {
	*Service
	st             ModelState
	watcherFactory WatcherFactory
}

// NewModelService returns a new [ModelService].
func NewModelService(modelSt ModelState, st State) *ModelService {
	return &ModelService{
		Service: NewService(st),
		st:      modelSt,
	}
}

// GetApplicationTargetAgentVersion reports the target agent version that should be
// being run on the provided machine identified by name. The following errors
// are possible:
// - [applicationerrors.ApplicationNotFound]
// - [github.com/juju/juju/domain/model/errors.NotFound]
func (s *ModelService) GetApplicationTargetAgentVersion(
	ctx context.Context,
	applicationName string,
) (version.Number, error) {
	err := s.st.CheckApplicationExists(ctx, applicationName)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return version.Zero, errors.Errorf(
			"application %q does not exist", applicationName,
		).Add(applicationerrors.ApplicationNotFound)
	} else if err != nil {
		return version.Zero, errors.Errorf(
			"checking if application %q exists when getting target agent version: %w",
			applicationName, err,
		)
	}

	modelUUID, err := s.st.GetModelUUID(ctx)
	if err != nil {
		return version.Zero, errors.Errorf(
			"getting application %q model uuid: %w", applicationName, err,
		)
	}

	return s.Service.st.GetModelTargetAgentVersion(ctx, modelUUID)
}

// GetMachineTargetAgentVersion reports the target agent version that should be
// being run on the provided machine identified by name. The following errors
// are possible:
// - [machineerrors.MachineNotFound]
// - [github.com/juju/juju/domain/model/errors.NotFound]
func (s *ModelService) GetMachineTargetAgentVersion(
	ctx context.Context,
	machineName machine.Name,
) (version.Number, error) {
	err := s.st.CheckMachineExists(ctx, machineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return version.Zero, errors.Errorf(
			"machine %q does not exist", machineName,
		).Add(machineerrors.MachineNotFound)
	} else if err != nil {
		return version.Zero, errors.Errorf(
			"checking if machine %q exists when getting target agent version: %w",
			machineName, err,
		)
	}

	modelUUID, err := s.st.GetModelUUID(ctx)
	if err != nil {
		return version.Zero, errors.Errorf(
			"getting machine %q model uuid: %w", machineName, err,
		)
	}

	return s.Service.st.GetModelTargetAgentVersion(ctx, modelUUID)
}

// GetModelTargetAgentVersion returns the agent version for the specified model.
// The following errors can be returned:
//   - [errors.NotValid] if the model ID is not valid;
//   - [github.com/juju/juju/domain/model/errors.NotFound] if no model exists
//     for the provided ID.
func (s *Service) GetModelTargetAgentVersion(
	ctx context.Context,
	modelID model.UUID,
) (version.Number, error) {
	if err := modelID.Validate(); err != nil {
		return version.Zero, errors.Errorf("validating model ID: %w", err)
	}
	return s.st.GetModelTargetAgentVersion(ctx, modelID)
}

// GetModelTargetAgentVersion returns the target agent version for the entire
// model. The following errors can be returned:
// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model does
// not exist.
func (s *ModelService) GetModelTargetAgentVersion(
	ctx context.Context,
) (version.Number, error) {
	modelUUID, err := s.st.GetModelUUID(ctx)
	if err != nil {
		return version.Zero, errors.Errorf("getting the current model's uuid: %w", err)
	}

	return s.Service.GetModelTargetAgentVersion(ctx, modelUUID)
}

// GetUnitTargetAgentVersion reports the target agent version that should be
// being run on the provided unit identified by name. The following errors
// are possible:
// - [applicationerrors.UnitNotFound] - When the unit in question does not exist.
// - [github.com/juju/juju/domain/model/errors.NotFound] - When the model the
// unit belongs to no longer exists.
func (s *ModelService) GetUnitTargetAgentVersion(
	ctx context.Context,
	unitName string,
) (version.Number, error) {
	err := s.st.CheckUnitExists(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return version.Zero, errors.Errorf(
			"unit %q does not exist", unitName,
		).Add(applicationerrors.UnitNotFound)
	} else if err != nil {
		return version.Zero, errors.Errorf(
			"checking if unit %q exists when getting target agent version: %w",
			unitName, err,
		)
	}

	modelUUID, err := s.st.GetModelUUID(ctx)
	if err != nil {
		return version.Zero, errors.Errorf(
			"getting unit %q model uuid: %w", unitName, err,
		)
	}

	return s.Service.st.GetModelTargetAgentVersion(ctx, modelUUID)
}

// WatchApplicationTargetAgentVersion is responsible for watching the target
// agent version for an application and reporting when there has been a change
// via a [watcher.NotifyWatcher]. The following errors can be expected:
// - [applicationerrors.ApplicationNotFound] - When the application does not
// exist.
// - [modelerrors.NotFound] - When the model of the machine no longer exists.
func (s *ModelService) WatchApplicationTargetAgentVersion(
	ctx context.Context,
	applicationName string,
) (watcher.NotifyWatcher, error) {
	err := s.st.CheckApplicationExists(ctx, applicationName)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return nil, errors.Errorf(
			"application %q does not exist", applicationName,
		).Add(applicationerrors.ApplicationNotFound)
	} else if err != nil {
		return nil, errors.Errorf(
			"checking if application %q exists when watching target agent version: %w",
			applicationName, err,
		)
	}

	w, err := s.WatchModelTargetAgentVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"getting watcher for application %q model target agent version: %w",
			applicationName, err,
		)
	}
	return w, nil
}

// WatchMachineTargetAgentVersion is responsible for watching the target agent
// version for machine and reporting when there has been a change via a
// [watcher.NotifyWatcher]. The following errors can be expected:
// - [machineerrors.NotFound] - When no machine exists for the provided name.
// - [modelerrors.NotFound] - When the model of the machine no longer exists.
func (s *ModelService) WatchMachineTargetAgentVersion(
	ctx context.Context,
	machineName machine.Name,
) (watcher.NotifyWatcher, error) {
	err := s.st.CheckMachineExists(ctx, machineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return nil, errors.Errorf(
			"machine %q does not exist", machineName,
		).Add(machineerrors.MachineNotFound)
	} else if err != nil {
		return nil, errors.Errorf(
			"checking if machine %q exists when watching target agent version: %w",
			machineName, err,
		)
	}

	w, err := s.WatchModelTargetAgentVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"getting watcher for machine %q model target agent version: %w",
			machineName, err,
		)
	}
	return w, nil
}

// WatchModelTargetAgentVersion is responsible for watching the target agent
// version of this model and reporting when a change has happened in the
// version.
func (s *ModelService) WatchModelTargetAgentVersion(
	ctx context.Context,
) (watcher.NotifyWatcher, error) {
	modelUUID, err := s.st.GetModelUUID(ctx)
	if err != nil {
		return nil, errors.Errorf(
			"getting model uuid: %w", err,
		)
	}

	w, err := s.watcherFactory.NewValueWatcher(
		"model_agent", modelUUID.String(), changestream.All,
	)
	if err != nil {
		return nil, errors.Errorf(
			"creating watcher for model %q target agent version: %w",
			modelUUID, err,
		)
	}

	return w, nil
}

// WatchUnitTargetAgentVersion is responsible for watching the target agent
// version for unit and reporting when there has been a change via a
// [watcher.NotifyWatcher]. The following errors can be expected:
// - [applicationerrors.NotFound] - When no unit exists for the provided name.
// - [modelerrors.NotFound] - When the model of the unit no longer exists.
func (s *ModelService) WatchUnitTargetAgentVersion(
	ctx context.Context,
	unitName string,
) (watcher.NotifyWatcher, error) {
	err := s.st.CheckUnitExists(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return nil, errors.Errorf(
			"unit %q does not exist", unitName,
		).Add(machineerrors.MachineNotFound)
	} else if err != nil {
		return nil, errors.Errorf(
			"checking if unit %q exists when watching target agent version: %w",
			unitName, err,
		)
	}

	w, err := s.WatchModelTargetAgentVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"getting watcher for unit %q model target agent version: %w",
			unitName, err,
		)
	}
	return w, nil
}
