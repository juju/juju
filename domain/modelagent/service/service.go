// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/juju/juju/core/agentbinary"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	domainagentbinary "github.com/juju/juju/domain/agentbinary"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
	"github.com/juju/juju/internal/errors"
)

// AgentBinaryFinder defines a helper for asserting if agent binaries are
// available for provided agent version.
type AgentBinaryFinder interface {
	// HasBinariesForVersion will interrogate agent binaries available in the
	// system and return true or false if agent binaries exist for the provided
	// version.
	HasBinariesForVersion(semversion.Number) (bool, error)
}

// agentBinaryFinderFunc provides a util func type for satisfying the
// [AgentBinaryFinder] interface.
type agentBinaryFinderFunc func(semversion.Number) (bool, error)

type ModelState interface {
	// GetMachineAgentBinaryMetadata reports the agent binary metadata that is
	// currently running a given machine.
	//
	// The following errors can be expected:
	// - [machineerrors.MachineNotFound] when the machine being asked for does
	// not exist.
	// - [modelagenterrors.MissingAgentBinaries] when the agent binaries don't
	// exist for one or more machines in the model.
	GetMachineAgentBinaryMetadata(ctx context.Context, machineName string) (agentbinary.Metadata, error)

	// GetMachinesAgentBinaryMetadata reports the agent binary metadata that each
	// machine in the model is currently running. This is a bulk call to support
	// operations such as model export where it is expected that the state of a
	// model stays relatively static over the operation. This function will
	// never provide enough granuality into what machine fails as part of the
	// checks.
	//
	// The following errors can be expected:
	// - [modelagenterrors.AgentVersionNotSet] when one or more machines
	// in the model do not have their agent version set.
	// - [modelagenterrors.MissingAgentBinaries] when the agent binaries don't
	// exist for one or more machines in the model.
	GetMachinesAgentBinaryMetadata(context.Context) (map[machine.Name]agentbinary.Metadata, error)

	// GetMachinesNotAtTargetAgentVersion returns the list of machines where
	// their agent version is not the same as the model's target agent version
	// or who have no agent version reproted at all. If no machines exist that
	// match this criteria an empty slice is returned.
	GetMachinesNotAtTargetAgentVersion(context.Context) ([]machine.Name, error)

	// GetMachineRunningAgentBinaryVersion returns the running machine agent
	// binary version for the given machine uuid.
	// The following errors can be expected:
	// - [machineerrors.MachineNotFound] when the machine being asked for does
	// not exist.
	// - [github.com/juju/juju/domain/modelagent/errors.AgentVersionNotFound]
	// when no running agent version has been set for the given machine.
	GetMachineRunningAgentBinaryVersion(context.Context, string) (agentbinary.Version, error)

	// GetMachineTargetAgentVersion returns the target agent version for the specified machine.
	// The following error types can be expected:
	// - [github.com/juju/juju/domain/modelagent/errors.AgentVersionNotFound] when
	// the agent version does not exist.
	GetMachineTargetAgentVersion(context.Context, string) (agentbinary.Version, error)

	// GetMachineUUIDByName returns the UUID of a machine identified by its
	// name. If no machine exists for this name an error satisfying
	// [machineerroros.MachineNotFound] is returned.
	GetMachineUUIDByName(context.Context, machine.Name) (string, error)

	// GetModelTargetAgentVersion returns the target agent version for this model.
	GetModelTargetAgentVersion(context.Context) (semversion.Number, error)

	// GetUnitsAgentBinaryMetadata reports the agent binary metadata that each
	// unit in the model is currently running. This is a bulk call to support
	// operations such as model export where it is expected that the state of a
	// model stays relatively static over the operation. This function will
	// never provide enough granuality into what unit fails as part of the
	// checks.
	//
	// The following errors can be expected:
	// - [modelagenterrors.AgentVersionNotSet] when one or more units in
	// the model do not have their agent version set.
	// - [modelagenterrors.MissingAgentBinaries] when the agent binaries don't
	// exist for one or more units in the model.
	GetUnitsAgentBinaryMetadata(context.Context) (map[coreunit.Name]agentbinary.Metadata, error)

	// GetUnitsNotAtTargetAgentVersion returns the list of units where their
	// agent version is not the same as the models target agent version or who
	// have no agent version reported at all. If no units exist that match the
	// criteria an empty slice is returned.
	GetUnitsNotAtTargetAgentVersion(context.Context) ([]coreunit.Name, error)

	// GetUnitRunningAgentBinaryVersion returns the running unit agent binary
	// version for the given unit uuid.
	// The following errors can be expected:
	// - [applicationerrors.UnitNotFound] when the unit in question does not
	// exist.
	// - [github.com/juju/juju/domain/modelagent/errors.AgentVersionNotFound] when no
	// running agent version has been reported for the given machine.
	GetUnitRunningAgentBinaryVersion(context.Context, coreunit.UUID) (agentbinary.Version, error)

	// GetUnitTargetAgentVersion returns the target agent version for the specified unit.
	// The following error types can be expected:
	// - [github.com/juju/juju/domain/application/errors.UnitNotFound] when the
	// unit does not exist.
	// - [github.com/juju/juju/domain/modelagent/errors.AgentVersionNotFound] when
	// the agent version does not exist.
	GetUnitTargetAgentVersion(context.Context, coreunit.UUID) (agentbinary.Version, error)

	// GetUnitUUIDByName returns the UUID for the named unit, returning an
	// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't
	// exist.
	GetUnitUUIDByName(context.Context, coreunit.Name) (coreunit.UUID, error)

	// IsControllerModel indicates if this model is running the Juju controller
	// that owns this model. True is returned when this is the case.
	IsControllerModel(context.Context) (bool, error)

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
	SetMachineRunningAgentBinaryVersion(context.Context, string, agentbinary.Version) error

	// SetModelAgentStream is responsible for setting the agent stream that is
	// in use by the current model.
	SetModelAgentStream(context.Context, domainagentbinary.Stream) error

	// SetModelTargetAgentVersion is responsible for setting the current target
	// agent version of the model. This function expects a precondition version
	// to be supplied. The model's target version at the time the operation is
	// applied must match the preCondition version or else an error is returned.
	SetModelTargetAgentVersion(
		ctx context.Context,
		preCondition semversion.Number,
		toVersion semversion.Number,
	) error

	// SetModelTargetAgentVersionAndStream is responsible for setting the
	// current target agent version of the model and the agent stream that is
	// used. This function expects a precondition version to be supplied. The
	// model's target version at the time the operation is applied must match
	// the preCondition version or else an error is returned.
	SetModelTargetAgentVersionAndStream(
		ctx context.Context,
		preCondition semversion.Number,
		toVersion semversion.Number,
		stream domainagentbinary.Stream,
	) error

	// UpdateLatestAgentVersion persists the latest available agent version.
	UpdateLatestAgentVersion(context.Context, semversion.Number) error

	// SetUnitRunningAgentBinaryVersion sets the running agent version for the unit.
	// The following error types can be expected:
	// - [applicationerrors.UnitNotFound] - when the unit does not exist.
	// - [applicationerrors.UnitIsDead] - when the unit is dead.
	// - [github.com/juju/juju/core/errors.NotSupported] if the architecture is
	// not known to the database.
	SetUnitRunningAgentBinaryVersion(context.Context, coreunit.UUID, agentbinary.Version) error

	// GetAllMachinesWithBase returns a map of
	// machine UUIDs to their resolved platform base.
	//
	// Machines for which the channel field is NULL are skipped and do not appear in the
	// returned map.
	//
	// Machines for which the OS and channel field are both empty
	// will result in a corresponding zero value base returned.
	//
	// This method may return the following errors:
	//   - [coreerrors.NotValid] if, for any machine, either the OS or channel field but not both is non-empty.
	GetAllMachinesWithBase(ctx context.Context) (map[string]corebase.Base, error)
}

// ControllerState defines the interface for interacting with the
// underlying model that hosts the current controller(s).
type ControllerState interface {
	// GetControllerAgentVersions has the responsibility of
	// getting the agent versions of all the controllers.
	GetControllerAgentVersions(context.Context) ([]semversion.Number, error)
}

// WatcherFactory provides a factory for constructing new watchers.
type WatcherFactory interface {
	// NewNotifyWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
		ctx context.Context,
		summary string,
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
}

// Service is used to interact with the agent binary version of a model and its
// entities.
type Service struct {
	agentBinaryFinder AgentBinaryFinder
	modelSt           ModelState
	controllerSt      ControllerState
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
func NewService(
	agentBinaryFinder AgentBinaryFinder,
	modelSt ModelState,
	controllerSt ControllerState,
) *Service {
	return &Service{
		agentBinaryFinder: agentBinaryFinder,
		modelSt:           modelSt,
		controllerSt:      controllerSt,
	}
}

// NewWatchableService returns a new [WatchableService].
func NewWatchableService(
	agentBinaryFinder AgentBinaryFinder,
	modelSt ModelState,
	controllerSt ControllerState,
	watcherFactory WatcherFactory,
) *WatchableService {
	return &WatchableService{
		Service: Service{
			agentBinaryFinder: agentBinaryFinder,
			modelSt:           modelSt,
			controllerSt:      controllerSt,
		},
		watcherFactory: watcherFactory,
	}
}

// DefaultAgentBinaryFinder provides a default implementation of
// [AgentBinaryFinder] to construct a [Service] with. This implementation
// currently does not check if agent binaries exist and always returns true.
//
// There will be future work to wire up this support.
func DefaultAgentBinaryFinder() AgentBinaryFinder {
	return agentBinaryFinderFunc(func(v semversion.Number) (bool, error) {
		return true, nil
	})
}

// GetMachinesNotAtTargetAgentVersion reports all of the machines in the model that
// are currently not at the desired target version. This also returns machines
// that have no reported agent version set. If all units are up to the
// target version or no uints exist in the model a zero length slice is
// returned.
func (s *Service) GetMachinesNotAtTargetAgentVersion(
	ctx context.Context,
) ([]machine.Name, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.modelSt.GetMachinesNotAtTargetAgentVersion(ctx)
}

// GetMachineReportedAgentVersion returns the agent binary version that was last
// reported to be running by the agent on the machine.
// The following errors are possible:
// - [machineerrors.MachineNotFound] when the machine being asked for does not
// exist.
// - [github.com/juju/juju/domain/modelagent/errors.AgentVersionNotFound] when no
// agent version has been reported for the given machine.
func (s *Service) GetMachineReportedAgentVersion(
	ctx context.Context,
	machineName machine.Name,
) (agentbinary.Version, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	uuid, err := s.modelSt.GetMachineUUIDByName(ctx, machineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return agentbinary.Version{}, errors.Errorf(
			"machine %q does not exist", machineName,
		).Add(machineerrors.MachineNotFound)
	} else if err != nil {
		return agentbinary.Version{}, errors.Errorf(
			"getting machine uuid for machine name %q: %w",
			machineName.String(), err,
		)
	}

	ver, err := s.modelSt.GetMachineRunningAgentBinaryVersion(ctx, uuid)
	if err != nil {
		return agentbinary.Version{}, errors.Capture(err)
	}

	return ver, nil
}

// GetMachineAgentBinaryMetadata reports the agent binary metadata that is
// currently running a given machine.
//
// The following errors can be expected:
// - [machineerrors.MachineNotFound] when the machine being asked for does
// not exist.
// - [modelagenterrors.MissingAgentBinaries] when the agent binaries don't
// exist for one or more machines in the model.
func (s *Service) GetMachineAgentBinaryMetadata(ctx context.Context, machineName machine.Name) (agentbinary.Metadata, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineName.Validate(); err != nil {
		return agentbinary.Metadata{}, errors.Errorf("getting machine agent binary metadata for machine: %w", err)
	}

	return s.modelSt.GetMachineAgentBinaryMetadata(ctx, machineName.String())
}

// GetMachinesAgentBinaryMetadata returns the agent binary metadata that is
// running for each machine in the model. This call expects that every
// machine in the model has their agent binary version set and there exist agent
// binaries available for each machine and the version that it is running.
//
// This is a bulk call to support operations such as model export where it will
// never provide enough granuality into what machine fails as part of the
// checks.
//
// The following error types can be expected:
// - [modelagenterrors.AgentVersionNotSet] when one or more machines in
// the model do not have their agent binary version set.
// - [modelagenterrors.MissingAgentBinaries] when the agent binaries don't exist
// for one or more machines in the model.
func (s *Service) GetMachinesAgentBinaryMetadata(
	ctx context.Context,
) (map[machine.Name]agentbinary.Metadata, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.modelSt.GetMachinesAgentBinaryMetadata(ctx)
}

// GetMachineTargetAgentVersion reports the target agent version that should be
// running on the provided machine identified by name. The following errors are
// possible:
// - [machineerrors.MachineNotFound]
// - [github.com/juju/juju/domain/modelagent/errors.AgentVersionNotFound]
func (s *Service) GetMachineTargetAgentVersion(
	ctx context.Context,
	machineName machine.Name,
) (agentbinary.Version, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	uuid, err := s.modelSt.GetMachineUUIDByName(ctx, machineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return agentbinary.Version{}, errors.Errorf("machine %q does not exist", machineName).Add(machineerrors.MachineNotFound)
	} else if err != nil {
		return agentbinary.Version{}, errors.Errorf(
			"checking if machine %q exists when getting target agent version: %w",
			machineName, err,
		)
	}

	return s.modelSt.GetMachineTargetAgentVersion(ctx, uuid)
}

// GetMissingAgentTargetVersions returns missing architectures for the
// target agent version.
func (s *Service) GetMissingAgentTargetVersions(ctx context.Context) (semversion.Number, []string, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return semversion.Zero, nil, nil
}

// GetUnitsAgentBinaryMetadata returns the agent binary metadata that is running
// for each unit in the model. This call expects that every unit in the model
// has their agent binary version set and there exist agent binaries available
// for each unit and the version that it is running.
//
// This is a bulk call to support operations such as model export where it will
// never provide enough granuality into what unit fails as part of the checks.
//
// The following error types can be expected:
// - [modelagenterrors.AgentVersionNotSet] when one or more units in the
// model do not have their agent binary version set.
// - [modelagenterrors.MissingAgentBinaries] when the agent binaries don't exist
// for one or more units in the model.
func (s *Service) GetUnitsAgentBinaryMetadata(
	ctx context.Context,
) (map[coreunit.Name]agentbinary.Metadata, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	return s.modelSt.GetUnitsAgentBinaryMetadata(ctx)
}

// GetUnitsNotAtTargetAgentVersion reports all of the units in the model that
// are currently not at the desired target agent version. This also returns
// units that have no reported agent version set. If all units are up to the
// target version or no units exist in the model a zero length slice is
// returned.
func (s *Service) GetUnitsNotAtTargetAgentVersion(
	ctx context.Context,
) ([]coreunit.Name, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.modelSt.GetUnitsNotAtTargetAgentVersion(ctx)
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
) (agentbinary.Version, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	uuid, err := s.modelSt.GetUnitUUIDByName(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return agentbinary.Version{}, errors.Errorf(
			"unit %q does not exist", unitName,
		).Add(applicationerrors.UnitNotFound)
	} else if err != nil {
		return agentbinary.Version{}, errors.Errorf(
			"getting unit uuid for unit name %q: %w",
			unitName.String(), err,
		)
	}

	ver, err := s.modelSt.GetUnitRunningAgentBinaryVersion(ctx, uuid)
	if err != nil {
		return agentbinary.Version{}, errors.Capture(err)
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
) (agentbinary.Version, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	uuid, err := s.modelSt.GetUnitUUIDByName(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return agentbinary.Version{}, errors.Errorf("unit %q does not exist", unitName).Add(applicationerrors.UnitNotFound)
	} else if err != nil {
		return agentbinary.Version{}, errors.Errorf(
			"checking if unit %q exists when getting target agent version: %w",
			unitName, err,
		)
	}

	return s.modelSt.GetUnitTargetAgentVersion(ctx, uuid)
}

// GetModelTargetAgentVersion returns the agent version for the specified model.
// The following errors can be returned:
// - [github.com/juju/juju/domain/modelagent/errors.AgentVersionFound] if no
// agent version record exists.
func (s *Service) GetModelTargetAgentVersion(ctx context.Context) (semversion.Number, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.modelSt.GetModelTargetAgentVersion(ctx)
}

// HasBinariesForVersion checks if there exists agent binaries available for the
// supplied version. True is returned when agent binaries exist for the version.
//
// Implements the [AgentBinaryFinder] interface.
func (a agentBinaryFinderFunc) HasBinariesForVersion(
	version semversion.Number,
) (bool, error) {
	return a(version)
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
	reportedVersion agentbinary.Version,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineName.Validate(); err != nil {
		return errors.Errorf("setting reported agent version for machine: %w", err)
	}

	if err := reportedVersion.Validate(); err != nil {
		return errors.Errorf("reported agent version %v is not valid: %w", reportedVersion, err)
	}

	machineUUID, err := s.modelSt.GetMachineUUIDByName(ctx, machineName)
	if err != nil {
		return errors.Errorf(
			"getting machine UUID for machine %q: %w",
			machineName,
			err,
		)
	}

	if err := s.modelSt.SetMachineRunningAgentBinaryVersion(ctx, machineUUID, reportedVersion); err != nil {
		return errors.Errorf(
			"setting machine %q reported agent version (%s) in state: %w",
			machineUUID,
			reportedVersion.Number.String(),
			err,
		)
	}

	return nil
}

// SetModelAgentStream is responsible for setting the agent stream that is in
// use for the current model. If the agent stream supplied is not a recognised
// value an error satisfying [coreerrors.NotValid] is returned.
func (s *Service) SetModelAgentStream(
	ctx context.Context,
	agentStream domainagentbinary.Stream,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	if !agentStream.IsValid() {
		return errors.Errorf(
			"agent stream %q is not valid or understood", agentStream,
		).Add(coreerrors.NotValid)
	}

	if err := s.modelSt.SetModelAgentStream(ctx, agentStream); err != nil {
		return errors.Errorf(
			"setting model agent stream %q to value %d in state: %w",
			agentStream, agentStream, err,
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
	reportedVersion agentbinary.Version,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return errors.Errorf("unit name %q is not valid: %w", unitName, err)
	}

	if err := reportedVersion.Validate(); err != nil {
		return errors.Errorf("reported agent version %v is not valid: %w", reportedVersion, err)
	}

	unitUUID, err := s.modelSt.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return errors.Errorf(
			"getting unit UUID for unit %q: %w",
			unitName,
			err,
		)
	}

	if err := s.modelSt.SetUnitRunningAgentBinaryVersion(ctx, unitUUID, reportedVersion); err != nil {
		return errors.Errorf(
			"setting unit %q reported agent version (%s) in state: %w",
			unitUUID,
			reportedVersion.Number.String(),
			err,
		)
	}

	return nil
}

// UpgradeModelTargetAgentVersion is responsible for upgrading the target agent
// version of the current model to latest version available. The version that is
// upgraded to is returned.
//
// All agents that run on behalf of entities within the model will be entities
// within the model will be eventually upgraded to the new version after this
// call successfully returns.
//
// The following errors may be expected:
// - [modelagenterrors.CannotUpgradeControllerModel] when the current model is
// the model running the Juju controller.
// - [modelagenterrors.ModelUpgradeBlocker] when there exists a blocker in the
// model that prevents the model from being upgraded.
func (s *Service) UpgradeModelTargetAgentVersion(
	ctx context.Context,
) (semversion.Number, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	recommendedVersion, err := s.getRecommendedVersion(ctx)
	if err != nil {
		return semversion.Zero, errors.Capture(err)
	}
	err = s.UpgradeModelAgentToTargetVersion(ctx, recommendedVersion)

	// NOTE (tlm): Because this func uses
	// [Service.UpgradeModelAgentToTargetVersion] to compose its
	// implementation. This func must handle the contract of
	// UpgradeModelTargetAgentVersion. Specifically the errors returned don't
	// align with the expecations of the caller. The below switch statement
	// re-writes the error cases to better explain the very unlikely error that
	// has occurred. These exists to point a developer at the problem and not to
	// offer any actionable item for a caller.
	switch {
	case errors.Is(err, modelagenterrors.DowngradeNotSupported):
		return semversion.Zero, errors.Errorf(
			"upgrading model to recommended version %q is considered a downgrade of the model agent version",
			recommendedVersion,
		)
	case errors.Is(err, modelagenterrors.AgentVersionNotSupported):
		return semversion.Zero, errors.Errorf(
			"upgrading model to recommended version %q is not a supported version number",
			recommendedVersion,
		)
	case errors.Is(err, modelagenterrors.MissingAgentBinaries):
		return semversion.Zero, errors.Errorf(
			"upgrading model to recommended version %q is missing agent binaries for version",
			recommendedVersion,
		)
	case err != nil:
		return semversion.Zero, errors.Errorf(
			"upgrading model to recommended version %q: %w",
			recommendedVersion, err,
		)
	}

	return recommendedVersion, nil
}

// UpgradeModelTargetAgentVersionWithStream is responsible for upgrading the target
// agent version of the current model to the latest version available. While
// performing the upgrade the agent stream for the model will also be changed.
// The version that is upgraded to is returned.
//
// All agents that run on behalf of entities within the model will be entities
// within the model will be eventually upgraded to the new version after this
// call successfully returns.
//
// The following errors may be expected:
// - [coreerrors.NotValid] when the agent stream is not valid.
// - [modelagenterrors.CannotUpgradeControllerModel] when the current model is
// the model running the Juju controller.
// - [modelagenterrors.ModelUpgradeBlocker] when their exists a blocker in the
// model that prevents the model from being upgraded.
func (s *Service) UpgradeModelTargetAgentVersionWithStream(
	ctx context.Context,
	agentStream domainagentbinary.Stream,
) (semversion.Number, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	recommendedVersion, err := s.getRecommendedVersion(ctx)
	if err != nil {
		return semversion.Zero, errors.Capture(err)
	}
	err = s.UpgradeModelTargetAgentVersionStreamTo(
		ctx, recommendedVersion, agentStream,
	)

	// NOTE (tlm): Because this func uses
	// [Service.UpgradeModelTargetAgentVersionStreamTo] to compose it's
	// implementation. This func must handle the contract of
	// UpgradeModelTargetAgentVersion. Specifically the errors returned don't
	// align with the expectations of the caller. The below switch statement
	// re-writes the error cases to better explain the very unlikely error that
	// has occurred. These exists to point a developer at the problem and not to
	// offer any actionable item for a caller.
	switch {
	case errors.Is(err, modelagenterrors.DowngradeNotSupported):
		return semversion.Zero, errors.Errorf(
			"upgrading model to recommended version %q is considered a downgrade of the model agent version",
			recommendedVersion,
		)
	case errors.Is(err, modelagenterrors.AgentVersionNotSupported):
		return semversion.Zero, errors.Errorf(
			"upgrading model to recommended version %q is not a supported version number",
			recommendedVersion,
		)
	case errors.Is(err, modelagenterrors.MissingAgentBinaries):
		return semversion.Zero, errors.Errorf(
			"upgrading model to recommended version %q is missing agent binaries for version",
			recommendedVersion,
		)
	case err != nil:
		return semversion.Zero, errors.Errorf(
			"upgrading model to recommended version %q: %w",
			recommendedVersion, err,
		)
	}

	return recommendedVersion, nil
}

// UpgradeModelAgentToTargetVersion upgrades a model to a new target agent
// version. All agents that run on behalf of entities within the model will be
// eventually upgraded to the new version after this call successfully returns.
//
// The version supplied must not be a downgrade from the current target agent
// version of the model. It must also not be greater than the maximum supported
// version of the controller.
//
// The following errors may be expected:
// - [modelagenterrors.DowngradeNotSupported] when the caller is attempting to
// change the model target agent version to one that is lower than the current
// version.
// - [modelagenterrors.AgentVersionNotSupported] when the caller is attempting
// to upgrade the version to one that is greater than the maximum supported
// version of the controller hosting the model.
// - [modelagenterrors.MissingAgentBinaries] when agent binaries do not exist
// for the desired version. The model cannot be upgraded to a version where
// no agent binaries exist.
// - [modelagenterrors.CannotUpgradeControllerModel] when the current model is
// the model running the Juju controller.
// - [modelagenterrors.ModelUpgradeBlocker] when their exists a blocker in the
// model that prevents the model from being upgraded.
func (s *Service) UpgradeModelAgentToTargetVersion(
	ctx context.Context,
	desiredTargetVersion semversion.Number,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	currentTargetVersion, err := s.RunPreUpgradeChecksToVersion(ctx, desiredTargetVersion)
	if err != nil {
		return errors.Capture(err)
	}

	// Short circuit any further action if upgrade is to the same version that
	// the model is currently at. This check must be performed after validation
	// so correct signal is given to the caller.
	if currentTargetVersion.Compare(desiredTargetVersion) == 0 {
		return nil
	}

	err = s.modelSt.SetModelTargetAgentVersion(
		ctx, currentTargetVersion, desiredTargetVersion,
	)
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// UpgradeModelTargetAgentVersionStreamTo upgrades a model to a new target agent
// version and updates the agent stream that is in use. All agents that run on
// behalf of entities within the model will be eventually upgraded to the new
// version after this call successfully returns.
//
// The version supplied must not be a downgrade from the current target agent
// version of the model. It must also not be greater than the maximum supported
// version of the controller.
//
// The following errors may be expected:
// - [coreerrors.NotValid] when the agent stream is not valid.
// - [modelagenterrors.DowngradeNotSupported] when the caller is attempting to
// change the model target agent version to one that is lower than the current
// version.
// - [modelagenterrors.AgentVersionNotSupported] when the caller is attempting
// to upgrade the version to one that is greater than the maximum supported
// version of the controller hosting the model.
// - [modelagenterrors.MissingAgentBinaries] when agent binaries do not exist
// for the desired version. The model cannot be upgraded to a version where
// no agent binaries exist.
// - [modelagenterrors.CannotUpgradeControllerModel] when the current model is
// the model running the Juju controller.
// - [modelagenterrors.ModelUpgradeBlocker] when their exists a blocker in the
// model that prevents the model from being upgraded.
func (s *Service) UpgradeModelTargetAgentVersionStreamTo(
	ctx context.Context,
	desiredTargetVersion semversion.Number,
	agentStream domainagentbinary.Stream,
) error {
	// NOTE (tlm): We don't try and short circuit version upgrading if the model
	// is already at the current desired version. This is because this
	// upgrade also has considered the context of an agent stream change.
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	currentTargetVersion, err := s.RunPreUpgradeChecksToVersionWithStream(ctx, desiredTargetVersion, agentStream)
	if err != nil {
		return errors.Capture(err)
	}

	err = s.modelSt.SetModelTargetAgentVersionAndStream(
		ctx, currentTargetVersion, desiredTargetVersion, agentStream,
	)
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// UpdateLatestAgentVersion persists the latest available agent version.
func (s *Service) UpdateLatestAgentVersion(ctx context.Context, version semversion.Number) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	return s.modelSt.UpdateLatestAgentVersion(ctx, version)
}

// validateModelCanBeUpgraded checks if the current model is currently in a
// state that can have its target agent version changed/"upgraded". This check
// does not concern itself with the proposed upgrade version. It only cares
// about the state of the model and if it suitable to be upgraded.
//
// This check always assumes that the caller is upgrading the version and never
// downgrading. The validation performed is not guaranteed to still be valid
// after this func returns.
//
// The following errors may be expected:
// - [modelagenterrors.CannotUpgradeControllerModel] when the current model is
// the model running the Juju controller.
// - [modelagenterrors.ModelUpgradeBlocker] when their exists a blocker in the
// model that prevents the model from being upgraded.
func (s *Service) validateModelCanBeUpgraded(
	ctx context.Context,
) error {
	isControllerModel, err := s.modelSt.IsControllerModel(ctx)
	if err != nil {
		return errors.Errorf(
			"checking if model is considered the controller's model: %w", err,
		)
	}
	if isControllerModel {
		return errors.New(
			"model upgrades cannot be performed on the controller model",
		).Add(modelagenterrors.CannotUpgradeControllerModel)
	}

	machineBases, err := s.modelSt.GetAllMachinesWithBase(ctx)
	if err != nil {
		return errors.Errorf("getting machine bases from state: %w", err)
	}

	maps.DeleteFunc(machineBases, machineUsesSupportedBase(corebase.WorkloadBases()))
	if len(machineBases) > 0 {
		return modelagenterrors.ModelUpgradeBlocker{
			Reason: fmt.Sprintf(
				"model has %d machines using unsupported bases, the supported bases are: %v",
				len(machineBases), corebase.WorkloadBases(),
			),
		}
	}
	return nil
}

// machineUsesSupportedBase returns a predicate for maps.DeleteFunc that
// removes machines whose base matches one of the supported bases.
// Bases are considered equal if their OS and track match while risk and branch are ignored.
func machineUsesSupportedBase(supported []corebase.Base) func(uuid string, b corebase.Base) bool {
	return func(_ string, b corebase.Base) bool {
		// We only compare OS and Track.
		return slices.ContainsFunc(supported, func(s corebase.Base) bool {
			return b.OS == s.OS && b.Channel.Track == s.Channel.Track
		})
	}
}

// validateModelCanBeUpgradedTo checks to see if the model can be upgraded to
// the new desired target version and that it is within the supported realm of
// versions. Checks are also performed to ensure the model itself is in a state
// that can be upgraded.
//
// The following errors may be expected:
// - [modelagenterrors.DowngradeNotSupported] when the caller is attempting to
// downgrade the model agent version.
// - [modelagenterrors.AgentVersionNotSupported] when the caller is attempting
// to upgrade to a version that is greater than the max supported version of the
// controller. Or if the version being upgraded to is not not defined (zero).
// - [modelagenterrors.MissingAgentBinaries] when the agent binaries do not
// exist for the desired version.
// - [modelagenterrors.CannotUpgradeControllerModel] when the current model is
// the model running the Juju controller.
// - [modelagenterrors.ModelUpgradeBlocker] when their exists a blocker in the
// model that prevents the model from being upgraded.
func (s *Service) validateModelCanBeUpgradedTo(
	ctx context.Context,
	currentTargetVersion semversion.Number,
	desiredTargetVersion semversion.Number,
) error {
	if desiredTargetVersion.IsZero() {
		return errors.New(
			"invalid agent version supplied",
		).Add(modelagenterrors.AgentVersionNotSupported)
	}

	// Check that the caller is not attempting to downgrade the target agent
	// version of the model.
	if currentTargetVersion.Compare(desiredTargetVersion) > 0 {
		return errors.New(
			"model agent version downgrades are not supported",
		).Add(modelagenterrors.DowngradeNotSupported)
	}

	currentControllerVersion, err := s.getRecommendedVersion(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	// Check that the caller is not attempting to upgrade to a version that is
	// greater than that of the controller.
	if currentControllerVersion.Compare(desiredTargetVersion.ToPatch()) < 0 {
		return errors.Errorf(
			"upgrade model agent version is greated than max supported version %q",
			currentControllerVersion,
		).Add(modelagenterrors.AgentVersionNotSupported)
	}

	hasBinaries, err := s.agentBinaryFinder.HasBinariesForVersion(desiredTargetVersion)
	if err != nil {
		return errors.Errorf(
			"checking if binaries exist for version %q: %w",
			desiredTargetVersion, err,
		)
	}
	if !hasBinaries {
		return errors.Errorf(
			"no agent binaries exist for version %q", desiredTargetVersion,
		).Add(modelagenterrors.MissingAgentBinaries)
	}

	if err := s.validateModelCanBeUpgraded(ctx); err != nil {
		return errors.Errorf(
			"validating model can be upgraded: %w", err,
		)
	}

	return nil
}

// getRecommendedVersion has the responsibility of getting the agent versions
// for all controllers.
// It is then sorted to find the highest version in which it is returned to the
// caller.
func (s *Service) getRecommendedVersion(
	ctx context.Context,
) (semversion.Number, error) {
	versions, err := s.controllerSt.GetControllerAgentVersions(ctx)
	if err != nil {
		return semversion.Zero, errors.Capture(err)
	}

	if len(versions) == 0 {
		return semversion.Zero, errors.New("no recommended versions found")
	}

	// Sort it descendingly so the highest version is the first element.
	slices.SortFunc(versions, func(a, b semversion.Number) int {
		return a.Compare(b) * -1
	})

	return versions[0], nil
}

// RunPreUpgradeChecks performs a series of pre-upgrade validation checks
// to ensure that the model can be safely upgraded to the controller’s
// currently recommended version.
//
// This method determines the recommended version from the controller,
// then calls [Service.RunPreUpgradeChecksToVersion] to validate that the
// model can be upgraded to that version.
//
// The following errors may be returned:
//   - [modelagenterrors.DowngradeNotSupported] if the upgrade target version
//     is lower than the model’s current agent version.
//   - [modelagenterrors.AgentVersionNotSupported] if the target version is
//     greater than the controller’s supported maximum or is not defined.
//   - [modelagenterrors.MissingAgentBinaries] if the agent binaries for the
//     target version cannot be found.
//   - [modelagenterrors.CannotUpgradeControllerModel] if the model is the
//     controller model itself.
//   - [modelagenterrors.ModelUpgradeBlocker] if the model contains blockers
//     that prevent upgrading (e.g., unsupported machine bases).
//
// Returns the controller’s recommended version if all checks pass.
func (s *Service) RunPreUpgradeChecks(
	ctx context.Context,
) (semversion.Number, error) {
	recommendedVersion, err := s.getRecommendedVersion(ctx)
	if err != nil {
		return semversion.Zero, errors.Capture(err)
	}

	_, err = s.RunPreUpgradeChecksToVersion(ctx, recommendedVersion)
	if err != nil {
		return semversion.Zero, err
	}

	return recommendedVersion, nil
}

// RunPreUpgradeChecksToVersion performs pre-upgrade validation checks
// to ensure that the model can be safely upgraded to a specific desired
// target agent version.
//
// This function compares the current model’s target version with the
// desired target version, validates upgrade compatibility and supported
// ranges, ensures required binaries exist, and checks that the model
// itself is eligible for upgrade.
//
// The following errors may be returned:
//   - [modelagenterrors.DowngradeNotSupported] if a downgrade is requested.
//   - [modelagenterrors.AgentVersionNotSupported] if the desired version
//     exceeds the controller’s supported version or is undefined.
//   - [modelagenterrors.MissingAgentBinaries] if binaries for the target
//     version do not exist.
//   - [modelagenterrors.CannotUpgradeControllerModel] if the model is the
//     controller model.
//   - [modelagenterrors.ModelUpgradeBlocker] if there are blockers within
//     the model preventing an upgrade.
//
// Returns the current model target version if validation passes.
func (s *Service) RunPreUpgradeChecksToVersion(
	ctx context.Context,
	desiredTargetVersion semversion.Number,
) (semversion.Number, error) {
	currentTargetVersion, err := s.modelSt.GetModelTargetAgentVersion(ctx)
	if err != nil {
		return semversion.Zero, errors.Errorf(
			"getting current model target agent version: %w", err,
		)
	}

	err = s.validateModelCanBeUpgradedTo(
		ctx,
		currentTargetVersion,
		desiredTargetVersion,
	)
	if err != nil {
		return semversion.Zero, errors.Capture(err)
	}

	return currentTargetVersion, nil
}

// RunPreUpgradeChecksWithStream performs pre-upgrade validation checks
// similar to [Service.RunPreUpgradeChecks], but additionally validates
// against a specific [domainagentbinary.Stream].
//
// It ensures that the provided agent stream is valid, retrieves the
// controller’s recommended version, and verifies that the model can be
// upgraded to that version using [Service.RunPreUpgradeChecksToVersionWithStream].
//
// The following errors may be returned:
//   - [coreerrors.NotValid] if the supplied agent stream is invalid.
//   - [modelagenterrors.DowngradeNotSupported] if a downgrade is requested.
//   - [modelagenterrors.AgentVersionNotSupported] if the target version
//     exceeds the controller’s supported version or is undefined.
//   - [modelagenterrors.MissingAgentBinaries] if binaries for the target
//     version are missing.
//   - [modelagenterrors.CannotUpgradeControllerModel] if the model is the
//     controller model.
//   - [modelagenterrors.ModelUpgradeBlocker] if upgrade blockers exist.
//
// Returns the controller’s recommended version if all checks pass.
func (s *Service) RunPreUpgradeChecksWithStream(
	ctx context.Context,
	stream domainagentbinary.Stream,
) (semversion.Number, error) {
	desiredTargetVersion, err := s.getRecommendedVersion(ctx)
	if err != nil {
		return semversion.Zero, errors.Capture(err)
	}

	_, err = s.RunPreUpgradeChecksToVersionWithStream(
		ctx,
		desiredTargetVersion,
		stream,
	)
	if err != nil {
		return semversion.Zero, errors.Capture(err)
	}

	return desiredTargetVersion, nil
}

// RunPreUpgradeChecksToVersionWithStream performs pre-upgrade validation
// checks for a specific desired target agent version within a given
// [domainagentbinary.Stream].
//
// This function validates the supplied agent stream, retrieves the
// model’s current target agent version, and checks that upgrading to
// the desired target version is supported, safe, and possible.
//
// The following errors may be returned:
//   - [coreerrors.NotValid] if the provided agent stream is invalid.
//   - [modelagenterrors.DowngradeNotSupported] if a downgrade is requested.
//   - [modelagenterrors.AgentVersionNotSupported] if the desired version
//     exceeds the controller’s supported version or is undefined.
//   - [modelagenterrors.MissingAgentBinaries] if binaries for the target
//     version do not exist.
//   - [modelagenterrors.CannotUpgradeControllerModel] if the model is the
//     controller model.
//   - [modelagenterrors.ModelUpgradeBlocker] if upgrade blockers exist.
//
// Returns the current model target version if validation succeeds.
func (s *Service) RunPreUpgradeChecksToVersionWithStream(
	ctx context.Context,
	desiredTargetVersion semversion.Number,
	stream domainagentbinary.Stream,
) (semversion.Number, error) {
	if !stream.IsValid() {
		return semversion.Zero, errors.New("agent stream is not valid").
			Add(coreerrors.NotValid)
	}

	currentTargetVersion, err := s.modelSt.GetModelTargetAgentVersion(ctx)
	if err != nil {
		return semversion.Zero, errors.Errorf(
			"getting current model target agent version: %w", err,
		)
	}

	err = s.validateModelCanBeUpgradedTo(
		ctx,
		currentTargetVersion,
		desiredTargetVersion,
	)
	if err != nil {
		return semversion.Zero, errors.Capture(err)
	}
	return currentTargetVersion, nil
}

// WatchMachineTargetAgentVersion is responsible for watching the target agent
// version for machine and reporting when there has been a change via a
// [watcher.NotifyWatcher]. The following errors can be expected:
// - [machineerrors.NotFound] when no machine exists for the provided name.
func (s *WatchableService) WatchMachineTargetAgentVersion(
	ctx context.Context,
	machineName machine.Name,
) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if _, err := s.modelSt.GetMachineUUIDByName(ctx, machineName); errors.Is(err, machineerrors.MachineNotFound) {
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if _, err := s.modelSt.GetUnitUUIDByName(ctx, unitName); errors.Is(err, applicationerrors.UnitNotFound) {
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
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	w, err := s.watcherFactory.NewNotifyWatcher(
		ctx,
		"model target agent version watcher",
		eventsource.NamespaceFilter(s.modelSt.NamespaceForWatchAgentVersion(), changestream.All),
	)
	if err != nil {
		return nil, errors.Errorf("creating watcher for agent version: %w", err)
	}
	return w, nil
}
