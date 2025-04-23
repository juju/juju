// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/leadership"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
)

// UnitState describes retrieval and persistence methods for
// units.
type UnitState interface {
	// AddIAASUnits adds the specified units to the application. If the
	// application is not found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned. If any of the units
	// already exists, an error satisfying [applicationerrors.UnitAlreadyExists]
	// is returned.
	AddIAASUnits(context.Context, string, coreapplication.ID, corecharm.ID, ...application.AddUnitArg) error

	// AddCAASUnits adds the specified units to the application. If the
	// application is not found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned. If any of the units
	// already exists, an error satisfying [applicationerrors.UnitAlreadyExists]
	// is returned.
	AddCAASUnits(context.Context, string, coreapplication.ID, corecharm.ID, ...application.AddUnitArg) error

	// InsertMigratingIAASUnits inserts the fully formed units for the specified
	// IAAS application. This is only used when inserting units during model
	// migration. If the application is not found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned. If any of the units
	// already exists, an error satisfying [applicationerrors.UnitAlreadyExists]
	// is returned.
	InsertMigratingIAASUnits(context.Context, coreapplication.ID, ...application.ImportUnitArg) error

	// InsertMigratingCAASUnits inserts the fully formed units for the specified
	// CAAS application. This is only used when inserting units during model
	// migration. If the application is not found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned. If any of the units
	// already exists, an error satisfying [applicationerrors.UnitAlreadyExists]
	// is returned.
	InsertMigratingCAASUnits(context.Context, coreapplication.ID, ...application.ImportUnitArg) error

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

	// DeleteUnit deletes the specified unit. If the unit's application is Dying
	// and no other references to it exist, true is returned to indicate the
	// application could be safely deleted. It will fail if the unit is not
	// Dead.
	DeleteUnit(context.Context, coreunit.Name) (bool, error)

	// GetUnitUUIDByName returns the UUID for the named unit, returning an
	// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't
	// exist.
	GetUnitUUIDByName(context.Context, coreunit.Name) (coreunit.UUID, error)

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
	GetUnitMachineUUID(ctx context.Context, unitName coreunit.Name) (machine.UUID, error)

	// GetUnitMachineName gets the unit's machine uuid. If the unit does not
	// have a machine assigned, [applicationerrors.UnitMachineNotAssigned] is
	// returned.
	GetUnitMachineName(ctx context.Context, unitName coreunit.Name) (machine.Name, error)

	// SetUnitLife sets the life of the specified unit.
	SetUnitLife(context.Context, coreunit.Name, life.Life) error

	// GetModelConstraints returns the currently set constraints for the model.
	// The following error types can be expected:
	// - [modelerrors.NotFound]: when no model exists to set constraints for.
	// - [modelerrors.ConstraintsNotFound]: when no model constraints have been
	// set for the model.
	// Note: This method should mirror the model domain method of the same name.
	GetModelConstraints(context.Context) (constraints.Constraints, error)

	// SetUnitConstraints sets the unit constraints for the
	// specified application ID.
	// This method overwrites the full constraints on every call.
	// If invalid constraints are provided (e.g. invalid container type or
	// non-existing space), a [applicationerrors.InvalidUnitConstraints]
	// error is returned.
	// If the unit is dead, an error satisfying [applicationerrors.UnitIsDead]
	// is returned.
	SetUnitConstraints(context.Context, coreunit.UUID, constraints.Constraints) error

	// GetUnitRefreshAttributes returns the unit refresh attributes for the
	// specified unit. If the unit is not found, an error satisfying
	// [applicationerrors.UnitNotFound] is returned.
	// This doesn't take into account life, so it can return the life of a unit
	// even if it's dead.
	GetUnitRefreshAttributes(context.Context, coreunit.Name) (application.UnitAttributes, error)

	// GetAllUnitNames returns a slice of all unit names in the model.
	GetAllUnitNames(context.Context) ([]coreunit.Name, error)

	// GetUnitNamesForApplication returns a slice of the unit names for the given application
	// The following errors may be returned:
	// - [applicationerrors.ApplicationIsDead] if the application is dead
	// - [applicationerrors.ApplicationNotFound] if the application does not exist
	GetUnitNamesForApplication(context.Context, coreapplication.ID) ([]coreunit.Name, error)

	// GetUnitNamesForNetNode returns a slice of the unit names for the given net node
	GetUnitNamesForNetNode(context.Context, string) ([]coreunit.Name, error)

	// AddSubordinateUnit adds a new unit to the subordinate application. On
	// IAAS, the new unit will be colocated on machine with the principal unit.
	// The principal-subordinate relationship is also recorded.
	AddSubordinateUnit(context.Context, application.SubordinateUnitArg) (coreunit.Name, error)

	// IsSubordinateApplication returns true if the application is a subordinate application.
	IsSubordinateApplication(context.Context, coreapplication.ID) (bool, error)

	// GetMachineNetNodeUUIDFromName returns the net node UUID for the named
	// machine. The following errors may be returned: -
	// [applicationerrors.MachineNotFound] if the machine does not exist
	GetMachineNetNodeUUIDFromName(context.Context, machine.Name) (string, error)

	// SetUnitWorkloadVersion sets the workload version for the given unit.
	SetUnitWorkloadVersion(ctx context.Context, unitName coreunit.Name, version string) error

	// GetUnitWorkloadVersion returns the workload version for the given unit.
	GetUnitWorkloadVersion(ctx context.Context, unitName coreunit.Name) (string, error)
}

func (s *Service) makeUnitArgs(modelType coremodel.ModelType, units []AddUnitArg, constraints constraints.Constraints) ([]application.AddUnitArg, error) {
	args := make([]application.AddUnitArg, len(units))
	for i, u := range units {
		if err := u.UnitName.Validate(); err != nil {
			return nil, errors.Errorf("validating unit name %q: %w", u.UnitName, err)
		}

		placement, err := deployment.ParsePlacement(u.Placement)
		if err != nil {
			return nil, errors.Errorf("invalid placement for %q: %w", u.UnitName, err)
		}

		arg := application.AddUnitArg{
			UnitName:      u.UnitName,
			Constraints:   constraints,
			Placement:     placement,
			UnitStatusArg: s.makeUnitStatusArgs(modelType),
		}
		args[i] = arg
	}

	return args, nil
}

func (s *Service) makeUnitStatusArgs(modelType coremodel.ModelType) application.UnitStatusArg {
	workloadMessage := corestatus.MessageInstallingAgent
	if modelType == coremodel.IAAS {
		workloadMessage = corestatus.MessageWaitForMachine
	}
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
//   - [applicationerrors.MachineNotFound] when the model type is IAAS and the
//     principal unit does not have a machine.
//   - [applicationerrors.SubordinateUnitAlreadyExists] when the principal unit
//     already has a subordinate from this application
func (s *Service) AddSubordinateUnit(
	ctx context.Context,
	subordinateAppID coreapplication.ID,
	principalUnitName coreunit.Name,
) error {
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

	modelType, err := s.st.GetModelType(ctx)
	if err != nil {
		return errors.Errorf("getting model type: %w", err)
	}

	statusArg := s.makeUnitStatusArgs(modelType)
	unitName, err := s.st.AddSubordinateUnit(
		ctx,
		application.SubordinateUnitArg{
			SubordinateAppID:  subordinateAppID,
			PrincipalUnitName: principalUnitName,
			UnitStatusArg:     statusArg,
			ModelType:         modelType,
		},
	)
	if errors.Is(err, applicationerrors.UnitAlreadyHasSubordinate) {
		return nil
	} else if err != nil {
		return errors.Capture(err)
	}

	if err := s.recordStatusHistory(ctx, unitName, statusArg); err != nil {
		return errors.Errorf("recording status history: %w", err)
	}

	return nil
}

// UpdateCAASUnit updates the specified CAAS unit, returning an error satisfying
// [applicationerrors.ApplicationNotAlive] if the unit's application is not
// alive.
func (s *Service) UpdateCAASUnit(ctx context.Context, unitName coreunit.Name, params UpdateCAASUnitParams) error {
	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}

	appName := unitName.Application()
	_, appLife, err := s.st.GetApplicationLife(ctx, appName)
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

// RemoveUnit is called by the deployer worker and caas application provisioner
// worker to remove from the model units which have transitioned to dead.
// TODO(units): revisit his existing logic ported from mongo Note: the callers
// of this method only do so after the unit has become dead, so there's strictly
// no need to set the life to Dead before removing. If the unit is still alive,
// an error satisfying [applicationerrors.UnitIsAlive] is returned. If the unit
// is not found, an error satisfying [applicationerrors.UnitNotFound] is
// returned.
func (s *Service) RemoveUnit(ctx context.Context, unitName coreunit.Name, leadershipRevoker leadership.Revoker) error {
	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}

	unitLife, err := s.st.GetUnitLife(ctx, unitName)
	if err != nil {
		return errors.Capture(err)
	}
	if unitLife == life.Alive {
		return errors.Errorf("cannot remove unit %q: %w", unitName, applicationerrors.UnitIsAlive)
	}
	_, err = s.st.DeleteUnit(ctx, unitName)
	if err != nil {
		return errors.Errorf("removing unit %q: %w", unitName, err)
	}
	appName := unitName.Application()
	if err := leadershipRevoker.RevokeLeadership(appName, unitName); err != nil && !errors.Is(err, leadership.ErrClaimNotHeld) {
		s.logger.Warningf(ctx, "cannot revoke lease for dead unit %q", unitName)
	}
	return nil
}

// DestroyUnit prepares a unit for removal from the model
// returning an error  satisfying [applicationerrors.UnitNotFoundError]
// if the unit doesn't exist.
func (s *Service) DestroyUnit(ctx context.Context, unitName coreunit.Name) error {
	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}

	// For now, all we do is advance the unit's life to Dying.
	err := s.st.SetUnitLife(ctx, unitName, life.Dying)
	if err != nil {
		return errors.Errorf("destroying unit %q: %w", unitName, err)
	}
	return nil
}

// EnsureUnitDead is called by the unit agent just before it terminates.
// TODO(units): revisit his existing logic ported from mongo Note: the agent
// only calls this method once it gets notification that the unit has become
// dead, so there's strictly no need to call this method as the unit is already
// dead. This method is also called during cleanup from various cleanup jobs. If
// the unit is not found, an error satisfying [applicationerrors.UnitNotFound]
// is returned.
func (s *Service) EnsureUnitDead(ctx context.Context, unitName coreunit.Name, leadershipRevoker leadership.Revoker) error {
	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}

	unitLife, err := s.st.GetUnitLife(ctx, unitName)
	if err != nil {
		return errors.Capture(err)
	}
	if unitLife == life.Dead {
		return nil
	}
	err = s.st.SetUnitLife(ctx, unitName, life.Dead)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return nil
	} else if err != nil {
		return errors.Errorf("marking unit %q is dead: %w", unitName, err)
	}
	appName := unitName.Application()
	if err := leadershipRevoker.RevokeLeadership(appName, unitName); err != nil && !errors.Is(err, leadership.ErrClaimNotHeld) {
		s.logger.Warningf(ctx, "cannot revoke lease for dead unit %q", unitName)
	}
	return nil
}

// GetUnitUUID returns the UUID for the named unit, returning an error
// satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (s *Service) GetUnitUUID(ctx context.Context, unitName coreunit.Name) (coreunit.UUID, error) {
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
func (s *Service) GetUnitMachineName(ctx context.Context, unitName coreunit.Name) (machine.Name, error) {
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
func (s *Service) GetUnitMachineUUID(ctx context.Context, unitName coreunit.Name) (machine.UUID, error) {
	if err := unitName.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	unitMachine, err := s.st.GetUnitMachineUUID(ctx, unitName)
	if err != nil {
		return "", errors.Capture(err)
	}

	return unitMachine, nil
}

// DeleteUnit deletes the specified unit.
// TODO(units) - rework when dual write is refactored
// This method is called (mostly during cleanup) after a unit
// has been removed from mongo. The mongo calls are
// DestroyMaybeRemove, DestroyWithForce, RemoveWithForce.
func (s *Service) DeleteUnit(ctx context.Context, unitName coreunit.Name) error {
	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}

	isLast, err := s.st.DeleteUnit(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return nil
	} else if err != nil {
		return errors.Errorf("deleting unit %q: %w", unitName, err)
	}
	if isLast {
		// TODO(units): schedule application cleanup
		_ = isLast
	}
	return nil
}

// GetUnitRefreshAttributes returns the unit refresh attributes for the
// specified unit. If the unit is not found, an error satisfying
// [applicationerrors.UnitNotFound] is returned.
// This doesn't take into account life, so it can return the life of a unit
// even if it's dead.
func (s *Service) GetUnitRefreshAttributes(ctx context.Context, unitName coreunit.Name) (application.UnitAttributes, error) {
	if err := unitName.Validate(); err != nil {
		return application.UnitAttributes{}, errors.Capture(err)
	}

	return s.st.GetUnitRefreshAttributes(ctx, unitName)
}

// GetAllUnitNames returns a slice of all unit names in the model.
func (s *Service) GetAllUnitNames(ctx context.Context) ([]coreunit.Name, error) {
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
	appUUID, err := s.st.GetApplicationIDByName(ctx, appName)
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
func (s *Service) GetUnitNamesOnMachine(ctx context.Context, machineName machine.Name) ([]coreunit.Name, error) {
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
	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}

	return s.st.SetUnitWorkloadVersion(ctx, unitName, version)
}

// GetUnitWorkloadVersion returns the workload version for the given unit.
func (s *Service) GetUnitWorkloadVersion(ctx context.Context, unitName coreunit.Name) (string, error) {
	if err := unitName.Validate(); err != nil {
		return "", errors.Capture(err)
	}

	version, err := s.st.GetUnitWorkloadVersion(ctx, unitName)
	if err != nil {
		return "", errors.Errorf("getting workload version for %q: %w", unitName, err)
	}
	return version, nil
}

// GetPublicAddress returns the public address for the specified unit.
// For k8s provider, it will return the first public address of the cloud
// service if any, the first public address of the cloud container otherwise.
// For machines provider, it will return the first public address of the
// machine.
//
// The following errors may be returned:
// - [uniterrors.UnitNotFound] if the unit does not exist
func (s *Service) GetPublicAddress(ctx context.Context, unitName coreunit.Name) (network.SpaceAddress, error) {
	appUUID, err := s.st.GetApplicationIDByUnitName(ctx, unitName)
	if err != nil {
		return network.SpaceAddress{}, errors.Capture(err)
	}

	serviceAddresses, err := s.st.GetCloudServiceAddresses(ctx, appUUID)
	if err != nil {
		return network.SpaceAddress{}, errors.Capture(err)
	}
	if len(serviceAddresses) > 0 {
		if addr, ok := serviceAddresses.OneMatchingScope(network.ScopeMatchPublic); ok {
			return addr, nil
		}
	}

	// Fallback to the cloud container address.
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return network.SpaceAddress{}, errors.Capture(err)
	}
	containerAddresses, err := s.st.GetCloudContainerAddresses(ctx, unitUUID)
	if err != nil {
		return network.SpaceAddress{}, errors.Capture(err)
	}
	if len(containerAddresses) > 0 {
		if addr, ok := containerAddresses.OneMatchingScope(network.ScopeMatchPublic); ok {
			return addr, nil
		}
	}
	// TODO(nvinuesa): We should also use the address taken from the
	// machine table here when we migrate machines to dqlite.
	return network.SpaceAddress{}, network.NoAddressError(string(network.ScopePublic))
}

// GetPrivateAddress returns the private address for the specified unit.
// For k8s provider, it will return the first private address of the cloud
// service if any, the first private address of the cloud container otherwise.
// For machines provider, it will return the first private address of the
// machine.
//
// The following errors may be returned:
// - [uniterrors.UnitNotFound] if the unit does not exist
func (s *Service) GetPrivateAddress(ctx context.Context, unitName coreunit.Name) (network.SpaceAddress, error) {
	appUUID, err := s.st.GetApplicationIDByUnitName(ctx, unitName)
	if err != nil {
		return network.SpaceAddress{}, errors.Capture(err)
	}
	serviceAddresses, err := s.st.GetCloudServiceAddresses(ctx, appUUID)
	if err != nil {
		return network.SpaceAddress{}, errors.Capture(err)
	}
	if len(serviceAddresses) > 0 {
		if addr, ok := serviceAddresses.OneMatchingScope(network.ScopeMatchCloudLocal); ok {
			return addr, nil
		}
	}

	// Fallback to the cloud container address.
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return network.SpaceAddress{}, errors.Capture(err)
	}
	containerAddresses, err := s.st.GetCloudContainerAddresses(ctx, unitUUID)
	if err != nil {
		return network.SpaceAddress{}, errors.Capture(err)
	}
	if len(containerAddresses) > 0 {
		if addr, ok := containerAddresses.OneMatchingScope(network.ScopeMatchCloudLocal); ok {
			return addr, nil
		} else {
			// We always return (first) the container address even if it doesn't
			// match the scope.
			return containerAddresses[0], nil
		}
	}
	// TODO(nvinuesa): We should also use the address taken from the
	// machine table here when we migrate machines to dqlite.
	return network.SpaceAddress{}, network.NoAddressError(string(network.ScopeCloudLocal))
}
