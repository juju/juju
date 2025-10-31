// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	corelife "github.com/juju/juju/core/life"
	coremachine "github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/application/internal"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
)

// UnitState describes retrieval and persistence methods for
// units.
type UnitState interface {
	// AddIAASUnits adds the specified units to the application, returning their
	// names.
	AddIAASUnits(context.Context, coreapplication.UUID, ...application.AddIAASUnitArg) ([]coreunit.Name, []coremachine.Name, error)

	// AddCAASUnits adds the specified units to the application, returning their
	// names.
	AddCAASUnits(context.Context, coreapplication.UUID, ...application.AddCAASUnitArg) ([]coreunit.Name, error)

	// GetCAASUnitRegistered checks if a caas unit by the provided name is
	// already registered in the model. False is returned when no unit exists,
	// otherwise the units existing uuid and netnode uuid is returned.
	GetCAASUnitRegistered(
		context.Context, coreunit.Name,
	) (bool, coreunit.UUID, domainnetwork.NetNodeUUID, error)

	// InsertMigratingIAASUnits inserts the fully formed units for the specified
	// IAAS application. This is only used when inserting units during model
	// migration. If the application is not found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned. If any of the units
	// already exists, an error satisfying [applicationerrors.UnitAlreadyExists]
	// is returned.
	InsertMigratingIAASUnits(context.Context, coreapplication.UUID, ...application.ImportUnitArg) error

	// InsertMigratingCAASUnits inserts the fully formed units for the specified
	// CAAS application. This is only used when inserting units during model
	// migration. If the application is not found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned. If any of the units
	// already exists, an error satisfying [applicationerrors.UnitAlreadyExists]
	// is returned.
	InsertMigratingCAASUnits(context.Context, coreapplication.UUID, ...application.ImportUnitArg) error

	// RegisterCAASUnit registers the specified CAAS application unit. The
	// following errors can be expected:
	// [applicationerrors.ApplicationNotAlive] when the application is not alive
	// [applicationerrors.UnitAlreadyExists] when the unit exists
	// [applicationerrors.UnitNotAssigned] when the unit was not assigned
	RegisterCAASUnit(context.Context, string, application.RegisterCAASUnitArg) error

	// UpdateCAASUnit updates the cloud container for specified unit,
	// returning an error satisfying [applicationerrors.UnitNotFoundError]
	// if the unit doesn't exist.
	UpdateCAASUnit(context.Context, coreunit.Name, application.UpdateCAASUnitParams) error

	// UpdateUnitCharm updates the currently running charm marker for the given
	// unit.
	UpdateUnitCharm(context.Context, coreunit.Name, corecharm.ID) error

	// GetUnitUUIDByName returns the UUID for the named unit, returning an
	// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't
	// exist.
	GetUnitUUIDByName(context.Context, coreunit.Name) (coreunit.UUID, error)

	// GetUnitUUIDAndNetNodeForName returns the unit uuid and net node uuid for a
	// unit matching the supplied name.
	//
	// The following errors may be expected:
	// - [applicationerrors.UnitNotFound] if no unit exists for the supplied
	// name.
	GetUnitUUIDAndNetNodeForName(
		context.Context, coreunit.Name,
	) (coreunit.UUID, domainnetwork.NetNodeUUID, error)

	// GetUnitLife looks up the life of the specified unit, returning an error
	// satisfying [applicationerrors.UnitNotFound] if the unit is not found.
	GetUnitLife(context.Context, coreunit.Name) (life.Life, error)

	// GetUnitPrincipal gets the subordinates principal unit. If no principal unit
	// is found, for example, when the unit is not a subordinate, then false is
	// returned.
	GetUnitPrincipal(ctx context.Context, unitName coreunit.Name) (coreunit.Name, bool, error)

	// GetUnitMachineUUID gets the unit's machine uuid. If the unit does not
	// have a machine assigned, [applicationerrors.UnitMachineNotAssigned] is
	// returned.
	GetUnitMachineUUID(ctx context.Context, unitName coreunit.Name) (coremachine.UUID, error)

	// GetUnitMachineName gets the unit's machine uuid. If the unit does not
	// have a machine assigned, [applicationerrors.UnitMachineNotAssigned] is
	// returned.
	GetUnitMachineName(ctx context.Context, unitName coreunit.Name) (coremachine.Name, error)

	// GetAllUnitLifeForApplication returns a map of the unit names and their lives
	// for the given application.
	//   - If the application is not found, [applicationerrors.ApplicationNotFound]
	//     is returned.
	GetAllUnitLifeForApplication(context.Context, coreapplication.UUID) (map[string]int, error)

	// GetMachineUUIDAndNetNodeForName is responsible for identifying the uuid
	// and net node for a machine by it's name.
	//
	// The following errors may be expected:
	// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
	// machine exists for the supplied machine name.
	GetMachineUUIDAndNetNodeForName(
		context.Context, string,
	) (coremachine.UUID, domainnetwork.NetNodeUUID, error)

	// GetModelConstraints returns the currently set constraints for the model.
	// The following error types can be expected:
	// - [modelerrors.NotFound]: when no model exists to set constraints for.
	// - [modelerrors.ConstraintsNotFound]: when no model constraints have been
	// set for the model.
	// Note: This method should mirror the model domain method of the same name.
	GetModelConstraints(context.Context) (constraints.Constraints, error)

	// GetUnitRefreshAttributes returns the unit refresh attributes for the
	// specified unit. If the unit is not found, an error satisfying
	// [applicationerrors.UnitNotFound] is returned.
	// This doesn't take into account life, so it can return the life of a unit
	// even if it's dead.
	GetUnitRefreshAttributes(context.Context, coreunit.Name) (application.UnitAttributes, error)

	// GetUnitK8sPodInfo returns information about the k8s pod for the given unit.
	// The following errors may be returned:
	// - [applicationerrors.UnitNotFound] if the unit does not exist
	// - [applicationerrors.UnitIsDead] if the unit is dead
	GetUnitK8sPodInfo(context.Context, coreunit.Name) (application.K8sPodInfo, error)

	// GetUnitsK8sPodInfo returns Kubernetes related information for each unit
	// in the model that is running inside of a Kubernetes pod. If no Kubernetes
	// based units are found than an empty result is returned.
	//
	// This function WILL return ip address values for each pod in a
	// deterministic order. IP addresses will be order based on how public they
	// are, ipv6 addresses before ipv4 addresses and then natural sort over of
	// value. This ordering exists so that the user always get a deterministic
	// result.
	GetUnitsK8sPodInfo(ctx context.Context) (map[string]internal.UnitK8sInformation, error)

	// GetAllUnitNames returns a slice of all unit names in the model.
	GetAllUnitNames(context.Context) ([]coreunit.Name, error)

	// GetUnitNamesForApplication returns a slice of the unit names for the given application
	// The following errors may be returned:
	// - [applicationerrors.ApplicationIsDead] if the application is dead
	// - [applicationerrors.ApplicationNotFound] if the application does not exist
	GetUnitNamesForApplication(context.Context, coreapplication.UUID) ([]coreunit.Name, error)

	// GetUnitNamesForNetNode returns a slice of the unit names for the given net node
	GetUnitNamesForNetNode(context.Context, string) ([]coreunit.Name, error)

	// AddIAASubordinateUnit adds a new unit to the subordinate application. On
	// IAAS, the new unit will be colocated on machine with the principal unit.
	// The principal-subordinate relationship is also recorded.
	AddIAASSubordinateUnit(context.Context, application.SubordinateUnitArg) (coreunit.Name, []coremachine.Name, error)

	// GetMachineNetNodeUUIDFromName returns the net node UUID for the named
	// machine. The following errors may be returned: -
	// [applicationerrors.MachineNotFound] if the machine does not exist
	GetMachineNetNodeUUIDFromName(context.Context, coremachine.Name) (string, error)

	// SetUnitWorkloadVersion sets the workload version for the given unit.
	SetUnitWorkloadVersion(ctx context.Context, unitName coreunit.Name, version string) error

	// GetUnitWorkloadVersion returns the workload version for the given unit.
	GetUnitWorkloadVersion(ctx context.Context, unitName coreunit.Name) (string, error)

	// GetUnitSubordinates returns the names of all the subordinate units of the
	// given principal unit.
	GetUnitSubordinates(ctx context.Context, unitName coreunit.Name) ([]coreunit.Name, error)

	// GetUnitNetNodesByName returns the net node UUIDs associated with the
	// specified unit. The net nodes are selected in the same way as in
	// GetUnitAddresses, i.e. the union of the net nodes of the cloud service (if
	// any) and the net node of the unit.
	//
	// The following errors may be returned:
	// - [uniterrors.UnitNotFound] if the unit does not exist
	GetUnitNetNodesByName(ctx context.Context, name coreunit.Name) ([]string, error)

	// GetAllUnitCloudContainerIDsForApplication returns a map of the unit names
	// and their cloud container provider IDs for the given application.
	//   - If the application is dead, [applicationerrors.ApplicationIsDead] is returned.
	//   - If the application is not found, [applicationerrors.ApplicationNotFound]
	//     is returned.
	GetAllUnitCloudContainerIDsForApplication(context.Context, coreapplication.UUID) (map[coreunit.Name]string, error)
}

func (s *ProviderService) makeIAASUnitArgs(
	ctx context.Context,
	units []AddIAASUnitArg,
	storageDirectives []application.StorageDirective,
	platform deployment.Platform,
	constraints constraints.Constraints,
) ([]application.AddIAASUnitArg, error) {
	args := make([]application.AddIAASUnitArg, len(units))
	for i, u := range units {
		placement, err := deployment.ParsePlacement(u.Placement)
		if err != nil {
			return nil, errors.Errorf("invalid placement: %w", err)
		}

		var (
			machineUUID        coremachine.UUID
			machineNetNodeUUID domainnetwork.NetNodeUUID
		)
		// If the placement of the unit is on to an already established machine
		// we need to resolve this to a machine uuid and netnode uuid.
		if placement.Type == deployment.PlacementTypeMachine {
			mUUID, mNNUUID, err := s.st.GetMachineUUIDAndNetNodeForName(
				ctx, placement.Directive,
			)
			if err != nil {
				return nil, errors.Errorf(
					"getting machine %q for unit placement directive: %w",
					placement.Directive, err,
				)
			}
			machineUUID = mUUID
			machineNetNodeUUID = mNNUUID
		} else {
			// If the placement is not on to an already established machine we need
			// to generate a new machine uuid and netnode uuid for the unit.
			var err error
			machineUUID, err = coremachine.NewUUID()
			if err != nil {
				return nil, errors.Errorf(
					"generating new machine uuid for IAAS unit: %w", err,
				)
			}

			machineNetNodeUUID, err = domainnetwork.NewNetNodeUUID()
			if err != nil {
				return nil, errors.Errorf(
					"generating new machine net node uuid for IAAS unit: %w", err,
				)
			}
		}

		// make unit storage args. IAAS units always have their storage
		// attached to the machine's net node.
		unitStorageArgs, err := s.storageService.MakeUnitStorageArgs(
			ctx,
			machineNetNodeUUID,
			storageDirectives,
			nil,
		)
		if err != nil {
			return nil, errors.Errorf(
				"making storage arguments for IAAS unit: %w", err,
			)
		}
		iassUnitStorageArgs, err := s.storageService.MakeIAASUnitStorageArgs(
			ctx, unitStorageArgs)
		if err != nil {
			return nil, errors.Errorf(
				"making IAAS storage arguments for IAAS unit: %w", err,
			)
		}

		arg := application.AddIAASUnitArg{
			AddUnitArg: application.AddUnitArg{
				CreateUnitStorageArg: unitStorageArgs,
				Constraints:          constraints,
				Placement:            placement,
				// We use the same netnode uuid as the machine for the unit.
				NetNodeUUID:   machineNetNodeUUID,
				UnitStatusArg: s.makeIAASUnitStatusArgs(),
			},
			CreateIAASUnitStorageArg: iassUnitStorageArgs,
			Platform:                 platform,
			Nonce:                    u.Nonce,
			MachineNetNodeUUID:       machineNetNodeUUID,
			MachineUUID:              machineUUID,
		}
		args[i] = arg
	}

	return args, nil
}

func (s *ProviderService) makeCAASUnitArgs(
	ctx context.Context,
	units []AddUnitArg,
	storageDirectives []application.StorageDirective,
	constraints constraints.Constraints,
) ([]application.AddCAASUnitArg, error) {
	args := make([]application.AddCAASUnitArg, len(units))
	for i, u := range units {
		placement, err := deployment.ParsePlacement(u.Placement)
		if err != nil {
			return nil, errors.Errorf("invalid placement: %w", err)
		}

		netNodeUUID, err := domainnetwork.NewNetNodeUUID()
		if err != nil {
			return nil, errors.Errorf(
				"making new net node uuid for caas unit: %w", err,
			)
		}

		// make unit storage args. CAAS units always have their storage
		// attached to the unit's net node.
		unitStorageArgs, err := s.storageService.MakeUnitStorageArgs(
			ctx,
			netNodeUUID,
			storageDirectives,
			nil,
		)
		if err != nil {
			return nil, errors.Errorf("making storage for CAAS unit: %w", err)
		}

		arg := application.AddCAASUnitArg{
			AddUnitArg: application.AddUnitArg{
				CreateUnitStorageArg: unitStorageArgs,
				Constraints:          constraints,
				NetNodeUUID:          netNodeUUID,
				Placement:            placement,
				UnitStatusArg:        s.makeCAASUnitStatusArgs(),
			},
		}
		args[i] = arg
	}

	return args, nil
}

func (s *Service) makeIAASUnitStatusArgs() application.UnitStatusArg {
	return s.makeUnitStatusArgs(corestatus.MessageWaitForMachine)
}

func (s *Service) makeCAASUnitStatusArgs() application.UnitStatusArg {
	return s.makeUnitStatusArgs(corestatus.MessageInstallingAgent)
}

func (s *Service) makeUnitStatusArgs(workloadMessage string) application.UnitStatusArg {
	now := ptr(s.clock.Now())
	return application.UnitStatusArg{
		AgentStatus: &status.StatusInfo[status.UnitAgentStatusType]{
			Status: status.UnitAgentStatusAllocating,
			Since:  now,
		},
		WorkloadStatus: &status.StatusInfo[status.WorkloadStatusType]{
			Status:  status.WorkloadStatusWaiting,
			Message: workloadMessage,
			Since:   now,
		},
	}
}

// AddSubordinateUnit adds a new unit to the subordinate application. On
// IAAS, the new unit will be colocated on machine with the principal unit.
// The principal-subordinate relationship is also recorded.
//
// If there is already a subordinate unit of the application for the principal
// unit then this is a no-op.
//
// The following error types can be expected:
// - [applicationerrors.MachineNotFound] when the model type is IAAS and the
// principal unit does not have a machine.
// - [applicationerrors.SubordinateUnitAlreadyExists] when the principal unit
// already has a subordinate from this application
// - [applicationerrors.UnitNotFound] when the principal unit does not exist.
func (s *Service) AddIAASSubordinateUnit(
	ctx context.Context,
	subordinateAppID coreapplication.UUID,
	principalUnitName coreunit.Name,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := subordinateAppID.Validate(); err != nil {
		return errors.Capture(err)
	}
	if err := principalUnitName.Validate(); err != nil {
		return errors.Capture(err)
	}

	isSub, err := s.st.IsSubordinateApplication(ctx, subordinateAppID)
	if err != nil {
		return errors.Errorf("checking app is subordinate: %w", err)
	} else if !isSub {
		return applicationerrors.ApplicationNotSubordinate
	}

	princiaplUnitUUID, principalNetNodeUUID, err :=
		s.st.GetUnitUUIDAndNetNodeForName(ctx, principalUnitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return errors.Errorf(
			"principal unit for name %q does not exist", principalUnitName,
		).Add(applicationerrors.UnitNotFound)
	} else if err != nil {
		return errors.Errorf(
			"getting principal unit %q uuid and netnode: %w",
			principalUnitName, err,
		)
	}

	statusArg := s.makeIAASUnitStatusArgs()
	unitName, machineNames, err := s.st.AddIAASSubordinateUnit(
		ctx,
		application.SubordinateUnitArg{
			// TODO(storage): create storage args for subordinate unit.
			SubordinateAppID:  subordinateAppID,
			NetNodeUUID:       principalNetNodeUUID,
			PrincipalUnitUUID: princiaplUnitUUID,
			UnitStatusArg:     statusArg,
		},
	)
	if errors.Is(err, applicationerrors.UnitAlreadyHasSubordinate) {
		return nil
	} else if err != nil {
		return errors.Capture(err)
	}

	if err := s.recordUnitStatusHistory(ctx, unitName, statusArg); err != nil {
		return errors.Errorf("recording status history: %w", err)
	}
	s.recordInitMachinesStatusHistory(ctx, machineNames)

	return nil
}

// UpdateCAASUnit updates the specified CAAS unit, returning an error satisfying
// [applicationerrors.ApplicationNotAlive] if the unit's application is not
// alive.
func (s *Service) UpdateCAASUnit(ctx context.Context, unitName coreunit.Name, params UpdateCAASUnitParams) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}

	appName := unitName.Application()
	_, appLife, err := s.st.GetApplicationLifeByName(ctx, appName)
	if err != nil {
		return errors.Errorf("getting application %q life: %w", appName, err)
	}
	if appLife != life.Alive {
		return errors.Errorf("application %q is not alive", appName).Add(applicationerrors.ApplicationNotAlive)
	}

	agentStatus, err := encodeUnitAgentStatus(params.AgentStatus)
	if err != nil {
		return errors.Errorf("encoding agent status: %w", err)
	}
	workloadStatus, err := encodeWorkloadStatus(params.WorkloadStatus)
	if err != nil {
		return errors.Errorf("encoding workload status: %w", err)
	}
	k8sPodStatus, err := encodeK8sPodStatus(params.CloudContainerStatus)
	if err != nil {
		return errors.Errorf("encoding k8s pod status: %w", err)
	}

	cassUnitUpdate := application.UpdateCAASUnitParams{
		ProviderID:     params.ProviderID,
		Address:        params.Address,
		Ports:          params.Ports,
		AgentStatus:    agentStatus,
		WorkloadStatus: workloadStatus,
		K8sPodStatus:   k8sPodStatus,
	}

	if err := s.st.UpdateCAASUnit(ctx, unitName, cassUnitUpdate); err != nil {
		return errors.Errorf("updating caas unit %q: %w", unitName, err)
	}
	return nil
}

// UpdateUnitCharm updates the currently running charm marker for the given
// unit.
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] if the unit does not exist.
// - [applicationerrors.UnitIsDead] if the unit is dead.
// - [applicationerrors.CharmNotFound] if the charm charm does not exist.
func (s *Service) UpdateUnitCharm(ctx context.Context, unitName coreunit.Name, locator charm.CharmLocator) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	args := argsFromLocator(locator)
	id, err := s.getCharmID(ctx, args)
	if err != nil {
		return errors.Capture(err)
	}

	return s.st.UpdateUnitCharm(ctx, unitName, id)
}

// GetUnitUUID returns the UUID for the named unit.
//
// The following errors may be returned:
// - [github.com/juju/juju/core/unit.InvalidUnitName] if the unit name is invalid.
// - [github.com/juju/juju/domain/application/errors.UnitNotFound] if the unit doesn't exist.
func (s *Service) GetUnitUUID(ctx context.Context, unitName coreunit.Name) (coreunit.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return "", errors.Errorf("getting UUID of unit %q: %w", unitName, err)
	}
	return unitUUID, nil
}

// GetUnitLife looks up the life of the specified unit, returning an error
// satisfying [applicationerrors.UnitNotFoundError] if the unit is not found.
func (s *Service) GetUnitLife(ctx context.Context, unitName coreunit.Name) (corelife.Value, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	unitLife, err := s.st.GetUnitLife(ctx, unitName)
	if err != nil {
		return "", errors.Errorf("getting life for %q: %w", unitName, err)
	}
	return unitLife.Value()
}

// GetUnitPrincipal gets the subordinates principal unit. If no principal unit
// is found, for example, when the unit is not a subordinate, then false is
// returned.
func (s *Service) GetUnitPrincipal(ctx context.Context, unitName coreunit.Name) (coreunit.Name, bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return "", false, errors.Capture(err)
	}

	return s.st.GetUnitPrincipal(ctx, unitName)
}

// GetUnitMachineName gets the name of the unit's machine.
//
// The following errors may be returned:
//   - [applicationerrors.UnitMachineNotAssigned] if the unit does not have a
//     machine assigned.
//   - [applicationerrors.UnitNotFound] if the unit cannot be found.
//   - [applicationerrors.UnitIsDead] if the unit is dead.
func (s *Service) GetUnitMachineName(ctx context.Context, unitName coreunit.Name) (coremachine.Name, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	unitMachine, err := s.st.GetUnitMachineName(ctx, unitName)
	if err != nil {
		return "", errors.Capture(err)
	}

	return unitMachine, nil
}

// GetUnitMachineUUID gets the unit's machine UUID.
//
// The following errors may be returned:
//   - [applicationerrors.UnitMachineNotAssigned] if the unit does not have a
//     machine assigned.
//   - [applicationerrors.UnitNotFound] if the unit cannot be found.
//   - [applicationerrors.UnitIsDead] if the unit is dead.
func (s *Service) GetUnitMachineUUID(ctx context.Context, unitName coreunit.Name) (coremachine.UUID, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	unitMachine, err := s.st.GetUnitMachineUUID(ctx, unitName)
	if err != nil {
		return "", errors.Capture(err)
	}

	return unitMachine, nil
}

// GetUnitRefreshAttributes returns the unit refresh attributes for the
// specified unit. If the unit is not found, an error satisfying
// [applicationerrors.UnitNotFound] is returned.
// This doesn't take into account life, so it can return the life of a unit
// even if it's dead.
func (s *Service) GetUnitRefreshAttributes(ctx context.Context, unitName coreunit.Name) (application.UnitAttributes, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return application.UnitAttributes{}, errors.Capture(err)
	}

	return s.st.GetUnitRefreshAttributes(ctx, unitName)
}

// GetAllUnitNames returns a slice of all unit names in the model.
func (s *Service) GetAllUnitNames(ctx context.Context) ([]coreunit.Name, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	names, err := s.st.GetAllUnitNames(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return names, nil
}

// GetUnitNamesForApplication returns a slice of the unit names for the given application
// The following errors may be returned:
// - [applicationerrors.ApplicationIsDead] if the application is dead
// - [applicationerrors.ApplicationNotFound] if the application does not exist
func (s *Service) GetUnitNamesForApplication(ctx context.Context, appName string) ([]coreunit.Name, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	appUUID, err := s.st.GetApplicationUUIDByName(ctx, appName)
	if err != nil {
		return nil, errors.Capture(err)
	}
	names, err := s.st.GetUnitNamesForApplication(ctx, appUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return names, nil
}

// GetUnitNamesOnMachine returns a slice of the unit names on the given machine.
// The following errors may be returned:
// - [applicationerrors.MachineNotFound] if the machine does not exist
func (s *Service) GetUnitNamesOnMachine(ctx context.Context, machineName coremachine.Name) ([]coreunit.Name, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineName.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	netNodeUUID, err := s.st.GetMachineNetNodeUUIDFromName(ctx, machineName)
	if err != nil {
		return nil, errors.Capture(err)
	}
	names, err := s.st.GetUnitNamesForNetNode(ctx, netNodeUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return names, nil
}

// SetUnitWorkloadVersion sets the workload version for the given unit.
func (s *Service) SetUnitWorkloadVersion(ctx context.Context, unitName coreunit.Name, version string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}

	return s.st.SetUnitWorkloadVersion(ctx, unitName, version)
}

// GetUnitWorkloadVersion returns the workload version for the given unit.
func (s *Service) GetUnitWorkloadVersion(ctx context.Context, unitName coreunit.Name) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	version, err := s.st.GetUnitWorkloadVersion(ctx, unitName)
	if err != nil {
		return "", errors.Errorf("getting workload version for %q: %w", unitName, err)
	}
	return version, nil
}

// GetUnitK8sPodInfo returns information about the k8s pod for the given unit.
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] if the unit does not exist
// - [applicationerrors.UnitIsDead] if the unit is dead
func (s *Service) GetUnitK8sPodInfo(ctx context.Context, name coreunit.Name) (application.K8sPodInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := name.Validate(); err != nil {
		return application.K8sPodInfo{}, errors.Capture(err)
	}

	return s.st.GetUnitK8sPodInfo(ctx, name)
}

// GetUnitsK8sPodInfo returns information about the k8s pods for all alive units.
func (s *Service) GetUnitsK8sPodInfo(ctx context.Context) (map[coreunit.Name]application.K8sPodInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	stateUnitsInfo, err := s.st.GetUnitsK8sPodInfo(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	rval := make(map[coreunit.Name]application.K8sPodInfo, len(stateUnitsInfo))
	for _, info := range stateUnitsInfo {
		var suggestedAddress string
		if len(info.Addresses) != 0 {
			// We take the first address because the expectation is that they
			// have been order into a deterministic order. However we should
			// be returning all addresses to the caller and this needs to get
			// fixed.
			suggestedAddress = info.Addresses[0]
		}
		rval[coreunit.Name(info.UnitName)] = application.K8sPodInfo{
			Address:    suggestedAddress,
			ProviderID: info.ProviderID,
			Ports:      info.Ports,
		}
	}

	return rval, nil
}

// GetUnitSubordinates returns the names of all the subordinate units of the
// given principal unit.
//
// If the principal unit cannot be found, [applicationerrors.UnitNotFound] is
// returned.
func (s *Service) GetUnitSubordinates(ctx context.Context, unitName coreunit.Name) ([]coreunit.Name, error) {
	if err := unitName.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	return s.st.GetUnitSubordinates(ctx, unitName)
}

// GetAllUnitLifeForApplication returns a map of the unit names and their lives
// for the given application.
// The following errors may be returned:
// - [applicationerrors.ApplicationNotFound] if the application does not exist
func (s *Service) GetAllUnitLifeForApplication(ctx context.Context, appID coreapplication.UUID) (map[coreunit.Name]corelife.Value, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	namesAndLives, err := s.st.GetAllUnitLifeForApplication(ctx, appID)
	if err != nil {
		return nil, errors.Capture(err)
	}
	namesAndCoreLives := map[coreunit.Name]corelife.Value{}
	for name, lifeID := range namesAndLives {
		unitName, err := coreunit.NewName(name)
		if err != nil {
			return nil, errors.Errorf("parsing unit name %q: %w", name, err)
		}
		namesAndCoreLives[unitName], err = life.Life(lifeID).Value()
		if err != nil {
			return nil, errors.Capture(err)
		}
	}
	return namesAndCoreLives, nil
}

// GetAllUnitCloudContainerIDsForApplication returns a map of the unit names
// and their cloud container provider IDs for the given application.
//   - If the application is dead, [applicationerrors.ApplicationIsDead] is returned.
//   - If the application is not found, [applicationerrors.ApplicationNotFound]
//     is returned.
//   - If the application UUID is not valid, [coreerrors.NotValid] is returned.
func (s *Service) GetAllUnitCloudContainerIDsForApplication(ctx context.Context, appUUID coreapplication.UUID) (map[coreunit.Name]string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appUUID.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	idMap, err := s.st.GetAllUnitCloudContainerIDsForApplication(ctx, appUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return idMap, nil
}
