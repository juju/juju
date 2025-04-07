// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
)

type State interface {
	// GetMachineRunningAgentBinaryVersion returns the running machine agent
	// binary version for the given machine uuid.
	// The following errors can be expected:
	// - [machineerrors.MachineNotFound] when the machine being asked for does
	// not exist.
	// - [github.com/juju/juju/domain/modelagent/errors.AgentVersionNotFound]
	// when no running agent version has been set for the given machine.
	GetMachineRunningAgentBinaryVersion(context.Context, string) (coreagentbinary.Version, error)

	// GetMachineTargetAgentVersion returns the target agent version for the specified machine.
	// The following error types can be expected:
	// - [github.com/juju/juju/domain/model/errors.AgentVersionNotFound] when
	// the agent version does not exist.
	GetMachineTargetAgentVersion(context.Context, string) (coreagentbinary.Version, error)

	// GetMachineUUIDByName returns the UUID of a machine identified by its
	// name. If no machine exists for this name an error satisfying
	// [machineerroros.MachineNotFound] is returned.
	GetMachineUUIDByName(context.Context, machine.Name) (string, error)

	// GetModelTargetAgentVersion returns the target agent version for this model.
	GetModelTargetAgentVersion(context.Context) (semversion.Number, error)

	// GetUnitRunningAgentBinaryVersion returns the running unit agent binary
	// version for the given unit uuid.
	// The following errors can be expected:
	// - [applicationerrors.UnitNotFound] when the unit in question does not
	// exist.
	// - [github.com/juju/juju/domain/modelagent/errors.AgentVersionNotFound] when no
	// running agent version has been reported for the given machine.
	GetUnitRunningAgentBinaryVersion(context.Context, coreunit.UUID) (coreagentbinary.Version, error)

	// GetUnitTargetAgentVersion returns the target agent version for the specified unit.
	// The following error types can be expected:
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound] when the
	// unit does not exist.
	// - [github.com/juju/juju/domain/model/errors.AgentVersionNotFound] when
	// the agent version does not exist.
	GetUnitTargetAgentVersion(context.Context, coreunit.UUID) (coreagentbinary.Version, error)

	// GetUnitUUIDByName returns the UUID for the named unit, returning an
	// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't
	// exist.
	GetUnitUUIDByName(context.Context, coreunit.Name) (coreunit.UUID, error)

	// NamespaceForWatchAgentVersion returns the namespace identifier
	// to watch for the agent version.
	NamespaceForWatchAgentVersion() string

	// SetMachineRunningAgentBinaryVersion sets the running agent version for
	// the machine.
	// The following errors can be expected:
	// - [machineerrors.MachineNotFound] if the machine does not exist.
	// - [machineerrors.MachineIsDead] if the machine is dead.
	// - [github.com/juju/juju/core/errors.NotSupported] if the architecture is
	// not known to the database.
	SetMachineRunningAgentBinaryVersion(context.Context, string, coreagentbinary.Version) error

	// SetUnitRunningAgentBinaryVersion sets the running agent version for the unit.
	// The following error types can be expected:
	// - [applicationerrors.UnitNotFound] - when the unit does not exist.
	// - [applicationerrors.UnitIsDead] - when the unit is dead.
	// - [github.com/juju/juju/core/errors.NotSupported] if the architecture is
	// not known to the database.
	SetUnitRunningAgentBinaryVersion(context.Context, coreunit.UUID, coreagentbinary.Version) error
}

// WatcherFactory provides a factory for constructing new watchers.
type WatcherFactory interface {
	// NewNotifyWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
}

// Service is used to get the target Juju agent version for the current model.
type Service struct {
	st State
}

// WatchableService extends Service to provide further interactions with state
// to watch for agent version changes within the model.
type WatchableService struct {
	// Service is the composed Service that is being extended with watching
	// capabilities.
	Service
	watcherFactory WatcherFactory
}

// NewService returns a new [Service].
func NewService(st State) *Service {
	return &Service{
		st: st,
	}
}

// NewWatchableService returns a new [WatchableService].
func NewWatchableService(st State, watcherFactory WatcherFactory) *WatchableService {
	return &WatchableService{
		Service: Service{
			st: st,
		},
		watcherFactory: watcherFactory,
	}
}

// GetMachineReportedAgentVersion returns the agent binary version that was last
// reported to be running by the agent on the machine.
// The following errors are possible:
// - [machineerrors.MachineNotFound] when the machine being asked for does not
// exist.
// - [github.com/juju/juju/domain/model/errors.AgentVersionNotFound] when no
// agent version has been reported for the given machine.
func (s *Service) GetMachineReportedAgentVersion(
	ctx context.Context,
	machineName machine.Name,
) (coreagentbinary.Version, error) {
	uuid, err := s.st.GetMachineUUIDByName(ctx, machineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return coreagentbinary.Version{}, errors.Errorf(
			"machine %q does not exist", machineName,
		).Add(machineerrors.MachineNotFound)
	} else if err != nil {
		return coreagentbinary.Version{}, errors.Errorf(
			"getting machine uuid for machine name %q: %w",
			machineName.String(), err,
		)
	}

	ver, err := s.st.GetMachineRunningAgentBinaryVersion(ctx, uuid)
	if err != nil {
		return coreagentbinary.Version{}, errors.Capture(err)
	}

	return ver, nil
}

// GetMachineTargetAgentVersion reports the target agent version that should be
// running on the provided machine identified by name. The following errors are
// possible:
// - [machineerrors.MachineNotFound]
// - [github.com/juju/juju/domain/modelagent/errors.AgentVersionNotFound]
func (s *Service) GetMachineTargetAgentVersion(
	ctx context.Context,
	machineName machine.Name,
) (coreagentbinary.Version, error) {
	uuid, err := s.st.GetMachineUUIDByName(ctx, machineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return coreagentbinary.Version{}, errors.Errorf("machine %q does not exist", machineName).Add(machineerrors.MachineNotFound)
	} else if err != nil {
		return coreagentbinary.Version{}, errors.Errorf(
			"checking if machine %q exists when getting target agent version: %w",
			machineName, err,
		)
	}

	return s.st.GetMachineTargetAgentVersion(ctx, uuid)
}

// GetUnitReportedAgentVersion returns the agent binary version that was last
// reported to be running by the agent on the unit.
// The following errors are possible:
// - [applicationerrors.UnitNotFound] when the unit in question does not exist.
// - [github.com/juju/juju/domain/model/errors.AgentVersionNotFound] when no
// agent version has been reported for the given machine.
func (s *Service) GetUnitReportedAgentVersion(
	ctx context.Context,
	unitName coreunit.Name,
) (coreagentbinary.Version, error) {
	uuid, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return coreagentbinary.Version{}, errors.Errorf(
			"unit %q does not exist", unitName,
		).Add(applicationerrors.UnitNotFound)
	} else if err != nil {
		return coreagentbinary.Version{}, errors.Errorf(
			"getting unit uuid for unit name %q: %w",
			unitName.String(), err,
		)
	}

	ver, err := s.st.GetUnitRunningAgentBinaryVersion(ctx, uuid)
	if err != nil {
		return coreagentbinary.Version{}, errors.Capture(err)
	}

	return ver, nil
}

// GetUnitTargetAgentVersion reports the target agent version that should be
// being run on the provided unit identified by name. The following errors
// are possible:
// - [applicationerrors.UnitNotFound] When the unit in question does not exist.
// - [github.com/juju/juju/domain/modelagent/errors.AgentVersionFound] if no
// agent version record exists.
func (s *Service) GetUnitTargetAgentVersion(
	ctx context.Context,
	unitName coreunit.Name,
) (coreagentbinary.Version, error) {
	uuid, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return coreagentbinary.Version{}, errors.Errorf("unit %q does not exist", unitName).Add(applicationerrors.UnitNotFound)
	} else if err != nil {
		return coreagentbinary.Version{}, errors.Errorf(
			"checking if unit %q exists when getting target agent version: %w",
			unitName, err,
		)
	}

	return s.st.GetUnitTargetAgentVersion(ctx, uuid)
}

// GetModelTargetAgentVersion returns the agent version for the specified model.
// The following errors can be returned:
// - [github.com/juju/juju/domain/modelagent/errors.AgentVersionFound] if no
// agent version record exists.
func (s *Service) GetModelTargetAgentVersion(ctx context.Context) (semversion.Number, error) {
	return s.st.GetModelTargetAgentVersion(ctx)
}

// SetMachineReportedAgentVersion sets the reported agent version for the
// supplied machine name. Reported agent version is the version that the agent
// binary on this machine has reported it is running.
//
// The following errors are possible:
// - [github.com/juju/juju/core/errors.NotValid] if the reportedVersion is not
// valid or the machine name is not valid.
// - [github.com/juju/juju/core/errors.NotSupported] if the architecture is not
// supported.
// - [machineerrors.MachineNotFound] when the machine does not exist.
// - [machineerrors.MachineDead] when the machine is dead.
func (s *Service) SetMachineReportedAgentVersion(
	ctx context.Context,
	machineName machine.Name,
	reportedVersion coreagentbinary.Version,
) error {
	if err := machineName.Validate(); err != nil {
		return errors.Errorf("setting reported agent version for machine: %w", err)
	}

	if err := reportedVersion.Validate(); err != nil {
		return errors.Errorf("reported agent version %v is not valid: %w", reportedVersion, err)
	}

	machineUUID, err := s.st.GetMachineUUIDByName(ctx, machineName)
	if err != nil {
		return errors.Errorf(
			"getting machine UUID for machine %q: %w",
			machineName,
			err,
		)
	}

	if err := s.st.SetMachineRunningAgentBinaryVersion(ctx, machineUUID, reportedVersion); err != nil {
		return errors.Errorf(
			"setting machine %q reported agent version (%s) in state: %w",
			machineUUID,
			reportedVersion.Number.String(),
			err,
		)
	}

	return nil
}

// SetUnitReportedAgentVersion sets the reported agent version for the
// supplied unit name. Reported agent version is the version that the agent
// binary on this unit has reported it is running.
//
// The following errors are possible:
// - [github.com/juju/juju/core/errors.NotValid] if the reportedVersion is not
// valid or the machine name is not valid.
// - [github.com/juju/juju/core/errors.NotSupported] if the architecture is not
// supported.
// - [applicationerrors.UnitNotFound] - when the unit does not exist.
// - [applicationerrors.UnitIsDead] - when the unit is dead.
func (s *Service) SetUnitReportedAgentVersion(
	ctx context.Context,
	unitName coreunit.Name,
	reportedVersion coreagentbinary.Version,
) error {
	if err := unitName.Validate(); err != nil {
		return errors.Errorf("unit name %q is not valid: %w", unitName, err)
	}

	if err := reportedVersion.Validate(); err != nil {
		return errors.Errorf("reported agent version %v is not valid: %w", reportedVersion, err)
	}

	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return errors.Errorf(
			"getting unit UUID for unit %q: %w",
			unitName,
			err,
		)
	}

	if err := s.st.SetUnitRunningAgentBinaryVersion(ctx, unitUUID, reportedVersion); err != nil {
		return errors.Errorf(
			"setting unit %q reported agent version (%s) in state: %w",
			unitUUID,
			reportedVersion.Number.String(),
			err,
		)
	}

	return nil
}

// WatchMachineTargetAgentVersion is responsible for watching the target agent
// version for machine and reporting when there has been a change via a
// [watcher.NotifyWatcher]. The following errors can be expected:
// - [machineerrors.NotFound] when no machine exists for the provided name.
func (s *WatchableService) WatchMachineTargetAgentVersion(
	ctx context.Context,
	machineName machine.Name,
) (watcher.NotifyWatcher, error) {
	_, err := s.st.GetMachineUUIDByName(ctx, machineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return nil, errors.Errorf("machine %q does not exist", machineName).Add(machineerrors.MachineNotFound)
	} else if err != nil {
		return nil, errors.Errorf(
			"checking if machine %q exists when watching target agent version: %w", machineName, err)
	}

	w, err := s.WatchModelTargetAgentVersion(ctx)
	if err != nil {
		return nil, errors.Errorf("getting watcher for machine %q model target agent version: %w", machineName, err)
	}
	return w, nil
}

// WatchUnitTargetAgentVersion is responsible for watching the target agent
// version for unit and reporting when there has been a change via a
// [watcher.NotifyWatcher]. The following errors can be expected:
// - [applicationerrors.UnitNotFound] when no unit exists for the provided name.
func (s *WatchableService) WatchUnitTargetAgentVersion(
	ctx context.Context,
	unitName coreunit.Name,
) (watcher.NotifyWatcher, error) {
	_, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return nil, errors.Errorf("unit %q does not exist", unitName).Add(applicationerrors.UnitNotFound)
	} else if err != nil {
		return nil, errors.Errorf("checking if unit %q exists when watching target agent version: %w", unitName, err)
	}

	w, err := s.WatchModelTargetAgentVersion(ctx)
	if err != nil {
		return nil, errors.Errorf("getting watcher for unit %q model target agent version: %w", unitName, err)
	}
	return w, nil
}

// WatchModelTargetAgentVersion is responsible for watching the target agent
// version of this model and reporting when a change has happened in the
// version.
func (s *WatchableService) WatchModelTargetAgentVersion(ctx context.Context) (watcher.NotifyWatcher, error) {
	w, err := s.watcherFactory.NewNotifyWatcher(
		eventsource.NamespaceFilter(s.st.NamespaceForWatchAgentVersion(), changestream.All),
	)
	if err != nil {
		return nil, errors.Errorf("creating watcher for agent version: %w", err)
	}
	return w, nil
}
