// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/leadership"
	corelife "github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/life"
	internalerrors "github.com/juju/juju/internal/errors"
)

// UnitState describes retrieval and persistence methods for
// units.
type UnitState interface {
	// AddIAASUnits adds the specified units to the application.
	// If the application is not found, an error satisfying [applicationerrors.ApplicationNotFound] is returned.
	// If any of the units already exists, an error satisfying [applicationerrors.UnitAlreadyExists] is returned.
	AddIAASUnits(context.Context, string, coreapplication.ID, ...application.AddUnitArg) error

	// AddCAASUnits adds the specified units to the application.
	// If the application is not found, an error satisfying [applicationerrors.ApplicationNotFound] is returned.
	// If any of the units already exists, an error satisfying [applicationerrors.UnitAlreadyExists] is returned.
	AddCAASUnits(context.Context, string, coreapplication.ID, ...application.AddUnitArg) error

	// InsertMigratingIAASUnits inserts the fully formed units for the specified IAAS application.
	// This is only used when inserting units during model migration.
	// If the application is not found, an error satisfying [applicationerrors.ApplicationNotFound] is returned.
	// If any of the units already exists, an error satisfying [applicationerrors.UnitAlreadyExists] is returned.
	InsertMigratingIAASUnits(context.Context, coreapplication.ID, ...application.InsertUnitArg) error

	// InsertMigratingCAASUnits inserts the fully formed units for the specified CAAS application.
	// This is only used when inserting units during model migration.
	// If the application is not found, an error satisfying [applicationerrors.ApplicationNotFound] is returned.
	// If any of the units already exists, an error satisfying [applicationerrors.UnitAlreadyExists] is returned.
	InsertMigratingCAASUnits(context.Context, coreapplication.ID, ...application.InsertUnitArg) error

	// RegisterCAASUnit registers the specified CAAS application unit, returning an
	// error satisfying [applicationerrors.UnitAlreadyExists] if the unit exists,
	// or [applicationerrors.UnitNotAssigned] if the unit was not assigned.
	RegisterCAASUnit(context.Context, coreapplication.ID, application.RegisterCAASUnitArg) error

	// UpdateCAASUnit updates the cloud container for specified unit,
	// returning an error satisfying [applicationerrors.UnitNotFoundError]
	// if the unit doesn't exist.
	UpdateCAASUnit(context.Context, coreunit.Name, application.UpdateCAASUnitParams) error

	// SetUnitPassword updates the password for the specified unit UUID.
	SetUnitPassword(context.Context, coreunit.UUID, application.PasswordInfo) error

	// GetUnitWorkloadStatus returns the workload status of the specified unit,
	// returning:
	// - an error satisfying [applicationerrors.UnitNotFound] if the unit
	//   doesn't exist or;
	// - an error satisfying [applicationerrors.UnitStatusNotFound] if the
	//   status is not set.
	GetUnitWorkloadStatus(context.Context, coreunit.UUID) (*application.UnitStatusInfo[application.WorkloadStatusType], error)

	// SetUnitWorkloadStatus sets the workload status of the specified unit,
	// returning an error satisfying [applicationerrors.UnitNotFound] if the
	// unit doesn't exist.
	SetUnitWorkloadStatus(context.Context, coreunit.UUID, *application.StatusInfo[application.WorkloadStatusType]) error

	// GetUnitCloudContainerStatus returns the cloud container status of the
	// specified unit. It returns;
	// - an error satisfying [applicationerrors.UnitNotFound] if the unit
	//   doesn't exist or;
	// - an error satisfying [applicationerrors.UnitStatusNotFound] if the
	//   status is not set.
	GetUnitCloudContainerStatus(context.Context, coreunit.UUID) (*application.StatusInfo[application.CloudContainerStatusType], error)

	// GetUnitWorkloadStatusesForApplication returns the workload statuses for
	// all units of the specified application, returning:
	//   - an error satisfying [applicationerrors.ApplicationNotFound] if the
	//     application doesn't exist or;
	//   - error satisfying [applicationerrors.ApplicationIsDead] if the
	//     application is dead.
	GetUnitWorkloadStatusesForApplication(context.Context, coreapplication.ID) (application.UnitWorkloadStatuses, error)

	// GetUnitWorkloadAndCloudContainerStatusesForApplication returns the workload statuses
	// and the cloud container statuses for all units of the specified application, returning:
	//   - an error satisfying [applicationerrors.ApplicationNotFound] if the application
	//     doesn't exist or;
	//   - an error satisfying [applicationerrors.ApplicationIsDead] if the application
	//     is dead.
	GetUnitWorkloadAndCloudContainerStatusesForApplication(
		context.Context, coreapplication.ID,
	) (application.UnitWorkloadStatuses, application.UnitCloudContainerStatuses, error)

	// GetUnitAgentStatus returns the workload status of the specified unit,
	// returning:
	// - an error satisfying [applicationerrors.UnitNotFound] if the unit
	//   doesn't exist or;
	// - an error satisfying [applicationerrors.UnitStatusNotFound] if the
	//   status is not set.
	GetUnitAgentStatus(context.Context, coreunit.UUID) (*application.UnitStatusInfo[application.UnitAgentStatusType], error)

	// SetUnitAgentStatus sets the workload status of the specified unit,
	// returning an error satisfying [applicationerrors.UnitNotFound] if the
	// unit doesn't exist.
	SetUnitAgentStatus(context.Context, coreunit.UUID, *application.StatusInfo[application.UnitAgentStatusType]) error

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

	// SetUnitLife sets the life of the specified unit.
	SetUnitLife(context.Context, coreunit.Name, life.Life) error

	// SetUnitPresence marks the presence of the specified unit, returning an error
	// satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
	// The unit life is not considered when making this query.
	SetUnitPresence(ctx context.Context, name coreunit.Name) error

	// DeleteUnitPresence removes the presence of the specified unit. If the
	// unit isn't found it ignores the error.
	// The unit life is not considered when making this query.
	DeleteUnitPresence(ctx context.Context, name coreunit.Name) error

	// GetModelConstraints returns the currently set constraints for the model.
	// The following error types can be expected:
	// - [modelerrors.NotFound]: when no model exists to set constraints for.
	// - [modelerrors.ConstraintsNotFound]: when no model constraints have been
	// set for the model.
	// Note: This method should mirror the model domain method of the same name.
	GetModelConstraints(ctx context.Context) (constraints.Constraints, error)

	// SetUnitConstraints sets the unit constraints for the
	// specified application ID.
	// This method overwrites the full constraints on every call.
	// If invalid constraints are provided (e.g. invalid container type or
	// non-existing space), a [applicationerrors.InvalidUnitConstraints]
	// error is returned.
	// If the unit is dead, an error satisfying [applicationerrors.UnitIsDead]
	// is returned.
	SetUnitConstraints(ctx context.Context, inUnitUUID coreunit.UUID, cons constraints.Constraints) error
}

func (s *Service) makeUnitArgs(modelType coremodel.ModelType, units []AddUnitArg, constraints constraints.Constraints) ([]application.AddUnitArg, error) {
	now := ptr(s.clock.Now())
	workloadMessage := corestatus.MessageInstallingAgent
	if modelType == coremodel.IAAS {
		workloadMessage = corestatus.MessageWaitForMachine
	}

	args := make([]application.AddUnitArg, len(units))
	for i, u := range units {
		arg := application.AddUnitArg{
			UnitName:    u.UnitName,
			Constraints: constraints,
			UnitStatusArg: application.UnitStatusArg{
				AgentStatus: &application.StatusInfo[application.UnitAgentStatusType]{
					Status: application.UnitAgentStatusAllocating,
					Since:  now,
				},
				WorkloadStatus: &application.StatusInfo[application.WorkloadStatusType]{
					Status:  application.WorkloadStatusWaiting,
					Message: workloadMessage,
					Since:   now,
				},
			},
		}
		args[i] = arg
	}

	return args, nil
}

// SetUnitWorkloadStatus sets the workload status of the specified unit,
// returning an error satisfying [applicationerrors.UnitNotFound] if the unit
// doesn't exist.
func (s *Service) SetUnitWorkloadStatus(ctx context.Context, unitName coreunit.Name, status *corestatus.StatusInfo) error {
	if err := unitName.Validate(); err != nil {
		return errors.Trace(err)
	}

	if status == nil {
		return nil
	}

	// Ensure we have a valid timestamp. It's optional at the API server level.
	// but it is a requirement for the database.
	if status.Since == nil {
		status.Since = ptr(s.clock.Now())
	}

	workloadStatus, err := encodeWorkloadStatus(status)
	if err != nil {
		return internalerrors.Errorf("encoding workload status: %w", err)
	}
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return errors.Trace(err)
	}
	if err := s.st.SetUnitWorkloadStatus(ctx, unitUUID, workloadStatus); err != nil {
		return internalerrors.Errorf("setting workload status: %w", err)
	}

	if err := s.statusHistory.RecordStatus(ctx, unitWorkloadNamespace.WithID(unitName.String()), *status); err != nil {
		s.logger.Infof(ctx, "failed recording setting workload status for unit %q: %v", unitName, err)
	}
	return nil
}

// GetUnitWorkloadStatus returns the workload status of the specified unit,
// returning an error satisfying [applicationerrors.UnitNotFound] if the unit
// doesn't exist.
func (s *Service) GetUnitWorkloadStatus(ctx context.Context, unitName coreunit.Name) (*corestatus.StatusInfo, error) {
	if err := unitName.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	workloadStatus, err := s.st.GetUnitWorkloadStatus(ctx, unitUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return decodeUnitWorkloadStatus(workloadStatus)
}

// SetUnitAgentStatus sets the agent status of the specified unit,
// returning an error satisfying [applicationerrors.UnitNotFound] if the unit
// doesn't exist.
func (s *Service) SetUnitAgentStatus(ctx context.Context, unitName coreunit.Name, status *corestatus.StatusInfo) error {
	if err := unitName.Validate(); err != nil {
		return errors.Trace(err)
	}

	if status == nil {
		return nil
	}

	// Ensure we have a valid timestamp. It's optional at the API server level.
	// but it is a requirement for the database.
	if status.Since == nil {
		status.Since = ptr(s.clock.Now())
	}

	// Encoding the status will handle invalid status values.
	switch status.Status {
	case corestatus.Error:
		if status.Message == "" {
			return errors.Errorf("setting status %q without message", status.Status)
		}
	case corestatus.Lost, corestatus.Allocating:
		return errors.Errorf("setting status %q is not allowed", status.Status)
	}

	agentStatus, err := encodeUnitAgentStatus(status)
	if err != nil {
		return internalerrors.Errorf("encoding agent status: %w", err)
	}
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return errors.Trace(err)
	}
	if err := s.st.SetUnitAgentStatus(ctx, unitUUID, agentStatus); err != nil {
		return internalerrors.Errorf("setting agent status: %w", err)
	}

	if err := s.statusHistory.RecordStatus(ctx, unitAgentNamespace.WithID(unitName.String()), *status); err != nil {
		s.logger.Infof(ctx, "failed recording setting agent status for unit %q: %v", unitName, err)
	}
	return nil
}

// GetUnitAgentStatus returns the agent status of the specified unit,
// returning an error satisfying [applicationerrors.UnitNotFound] if the unit
// doesn't exist.
func (s *Service) GetUnitAgentStatus(ctx context.Context, unitName coreunit.Name) (*corestatus.StatusInfo, error) {
	if err := unitName.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	agentStatus, err := s.st.GetUnitAgentStatus(ctx, unitUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return decodeUnitAgentStatus(agentStatus)
}

// GetUnitWorkloadStatusesForApplication returns the workload statuses of all
// units in the specified application, indexed by unit name, returning an error
// satisfying [applicationerrors.ApplicationNotFound] if the application doesn't
// exist.
func (s *Service) GetUnitWorkloadStatusesForApplication(ctx context.Context, appID coreapplication.ID) (map[coreunit.Name]corestatus.StatusInfo, error) {
	if err := appID.Validate(); err != nil {
		return nil, internalerrors.Errorf("application ID: %w", err)
	}

	statuses, err := s.st.GetUnitWorkloadStatusesForApplication(ctx, appID)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	decoded, err := decodeUnitWorkloadStatuses(statuses)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}
	return decoded, nil
}

// GetUnitDisplayStatus returns the display status of the specified unit. The display
// status a function of both the unit workload status and the cloud container status.
// It returns an error satisfying [applicationerrors.UnitNotFound] if the unit doesn't
// exist.
func (s *Service) GetUnitDisplayStatus(ctx context.Context, unitName coreunit.Name) (*corestatus.StatusInfo, error) {
	if err := unitName.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	workloadStatus, err := s.st.GetUnitWorkloadStatus(ctx, unitUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	containerStatus, err := s.st.GetUnitCloudContainerStatus(ctx, unitUUID)
	if err != nil && !errors.Is(err, applicationerrors.UnitStatusNotFound) {
		return nil, errors.Trace(err)
	}
	return unitDisplayStatus(workloadStatus, containerStatus)
}

// GetUnitAndAgentDisplayStatus returns the unit and agent display status of the
// specified unit. The display status a function of both the unit workload status
// and the cloud container status. It returns an error satisfying
// [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (s *Service) GetUnitAndAgentDisplayStatus(ctx context.Context, unitName coreunit.Name) (agent *corestatus.StatusInfo, workload *corestatus.StatusInfo, _ error) {
	if err := unitName.Validate(); err != nil {
		return nil, nil, errors.Trace(err)
	}

	// TODO (stickupkid) This should just be 1 or 2 calls to the state layer
	// to get the agent and workload status.

	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	agentStatus, err := s.st.GetUnitAgentStatus(ctx, unitUUID)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	workloadStatus, err := s.st.GetUnitWorkloadStatus(ctx, unitUUID)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	containerStatus, err := s.st.GetUnitCloudContainerStatus(ctx, unitUUID)
	if err != nil && !errors.Is(err, applicationerrors.UnitStatusNotFound) {
		return nil, nil, errors.Trace(err)
	}

	return decodeUnitAgentWorkloadStatus(agentStatus, workloadStatus, containerStatus)
}

// SetUnitPassword updates the password for the specified unit, returning an error
// satisfying [applicationerrors.NotNotFound] if the unit doesn't exist.
func (s *Service) SetUnitPassword(ctx context.Context, unitName coreunit.Name, password string) error {
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return errors.Trace(err)
	}
	return s.st.SetUnitPassword(ctx, unitUUID, application.PasswordInfo{
		PasswordHash:  password,
		HashAlgorithm: application.HashAlgorithmSHA256,
	})
}

// RegisterCAASUnit creates or updates the specified application unit in a caas
// model, returning an error satisfying
// [applicationerrors.ApplicationNotFoundError] if the application doesn't
// exist. If the unit life is Dead, an error satisfying
// [applicationerrors.UnitAlreadyExists] is returned.
func (s *Service) RegisterCAASUnit(ctx context.Context, appName string, args application.RegisterCAASUnitArg) error {
	if args.PasswordHash == "" {
		return errors.NotValidf("password hash")
	}
	if args.ProviderID == "" {
		return errors.NotValidf("provider id")
	}
	if !args.OrderedScale {
		return errors.NewNotImplemented(nil, "registering CAAS units not supported without ordered unit IDs")
	}
	if args.UnitName == "" {
		return errors.NotValidf("missing unit name")
	}

	appUUID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return internalerrors.Errorf("getting application ID: %w", err)
	}
	err = s.st.RegisterCAASUnit(ctx, appUUID, args)
	if err != nil {
		return internalerrors.Errorf("saving caas unit %q: %w", args.UnitName, err)
	}
	return nil
}

// UpdateCAASUnit updates the specified CAAS unit, returning an error satisfying
// [applicationerrors.ApplicationNotAlive] if the unit's application is not
// alive.
func (s *Service) UpdateCAASUnit(ctx context.Context, unitName coreunit.Name, params UpdateCAASUnitParams) error {
	appName := unitName.Application()
	_, appLife, err := s.st.GetApplicationLife(ctx, appName)
	if err != nil {
		return internalerrors.Errorf("getting application %q life: %w", appName, err)
	}
	if appLife != life.Alive {
		return internalerrors.Errorf("application %q is not alive%w", appName, errors.Hide(applicationerrors.ApplicationNotAlive))
	}

	agentStatus, err := encodeUnitAgentStatus(params.AgentStatus)
	if err != nil {
		return internalerrors.Errorf("encoding agent status: %w", err)
	}
	workloadStatus, err := encodeWorkloadStatus(params.WorkloadStatus)
	if err != nil {
		return internalerrors.Errorf("encoding workload status: %w", err)
	}
	cloudContainerStatus, err := encodeCloudContainerStatus(params.CloudContainerStatus)
	if err != nil {
		return internalerrors.Errorf("encoding cloud container status: %w", err)
	}

	cassUnitUpdate := application.UpdateCAASUnitParams{
		ProviderID:           params.ProviderID,
		Address:              params.Address,
		Ports:                params.Ports,
		AgentStatus:          agentStatus,
		WorkloadStatus:       workloadStatus,
		CloudContainerStatus: cloudContainerStatus,
	}

	if err := s.st.UpdateCAASUnit(ctx, unitName, cassUnitUpdate); err != nil {
		return internalerrors.Errorf("updating caas unit %q: %w", unitName, err)
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
	unitLife, err := s.st.GetUnitLife(ctx, unitName)
	if err != nil {
		return errors.Trace(err)
	}
	if unitLife == life.Alive {
		return fmt.Errorf("cannot remove unit %q: %w", unitName, applicationerrors.UnitIsAlive)
	}
	_, err = s.st.DeleteUnit(ctx, unitName)
	if err != nil {
		return errors.Annotatef(err, "removing unit %q", unitName)
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
	// For now, all we do is advance the unit's life to Dying.
	err := s.st.SetUnitLife(ctx, unitName, life.Dying)
	if err != nil {
		return internalerrors.Errorf("destroying unit %q: %w", unitName, err)
	}
	return nil
}

// EnsureUnitDead is called by the unit agent just before it terminates.
// TODO(units): revisit his existing logic ported from mongo
// Note: the agent only calls this method once it gets notification
// that the unit has become dead, so there's strictly no need to call
// this method as the unit is already dead.
// This method is also called during cleanup from various cleanup jobs.
// If the unit is not found, an error satisfying [applicationerrors.UnitNotFound]
// is returned.
func (s *Service) EnsureUnitDead(ctx context.Context, unitName coreunit.Name, leadershipRevoker leadership.Revoker) error {
	unitLife, err := s.st.GetUnitLife(ctx, unitName)
	if err != nil {
		return errors.Trace(err)
	}
	if unitLife == life.Dead {
		return nil
	}
	err = s.st.SetUnitLife(ctx, unitName, life.Dead)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return nil
	} else if err != nil {
		return errors.Annotatef(err, "marking unit %q is dead", unitName)
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
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return "", internalerrors.Errorf("getting UUID of unit %q: %w", unitName, err)
	}
	return unitUUID, nil
}

// GetUnitLife looks up the life of the specified unit, returning an error
// satisfying [applicationerrors.UnitNotFoundError] if the unit is not found.
func (s *Service) GetUnitLife(ctx context.Context, unitName coreunit.Name) (corelife.Value, error) {
	unitLife, err := s.st.GetUnitLife(ctx, unitName)
	if err != nil {
		return "", internalerrors.Errorf("getting life for %q: %w", unitName, err)
	}
	return unitLife.Value(), nil
}

// DeleteUnit deletes the specified unit.
// TODO(units) - rework when dual write is refactored
// This method is called (mostly during cleanup) after a unit
// has been removed from mongo. The mongo calls are
// DestroyMaybeRemove, DestroyWithForce, RemoveWithForce.
func (s *Service) DeleteUnit(ctx context.Context, unitName coreunit.Name) error {
	isLast, err := s.st.DeleteUnit(ctx, unitName)
	if errors.Is(err, applicationerrors.UnitNotFound) {
		return nil
	} else if err != nil {
		return errors.Annotatef(err, "deleting unit %q", unitName)
	}
	if isLast {
		// TODO(units): schedule application cleanup
		_ = isLast
	}
	return nil
}

// CAASUnitTerminating should be called by the CAASUnitTerminationWorker when
// the agent receives a signal to exit. UnitTerminating will return how the
// agent should shutdown.
//
// We pass in a CAAS broker to get app details from the k8s cluster - we will
// probably make it a service attribute once more use cases emerge.
func (s *Service) CAASUnitTerminating(ctx context.Context, appName string, unitNum int, broker Broker) (bool, error) {
	// TODO(sidecar): handle deployment other than statefulset
	deploymentType := caas.DeploymentStateful
	restart := true

	switch deploymentType {
	case caas.DeploymentStateful:
		caasApp := broker.Application(appName, caas.DeploymentStateful)
		appState, err := caasApp.State()
		if err != nil {
			return false, errors.Trace(err)
		}
		appID, err := s.st.GetApplicationIDByName(ctx, appName)
		if err != nil {
			return false, errors.Trace(err)
		}
		scaleInfo, err := s.st.GetApplicationScaleState(ctx, appID)
		if err != nil {
			return false, errors.Trace(err)
		}
		if unitNum >= scaleInfo.Scale || unitNum >= appState.DesiredReplicas {
			restart = false
		}
	case caas.DeploymentStateless, caas.DeploymentDaemon:
		// Both handled the same way.
		restart = true
	default:
		return false, errors.NotSupportedf("unknown deployment type")
	}
	return restart, nil
}

// SetUnitPresence marks the presence of the unit in the model. It is called by
// the unit agent accesses the API server. If the unit is not found, an error
// satisfying [applicationerrors.UnitNotFound] is returned. The unit life is not
// considered when setting the presence.
func (s *Service) SetUnitPresence(ctx context.Context, unitName coreunit.Name) error {
	if err := unitName.Validate(); err != nil {
		return errors.Trace(err)
	}
	return s.st.SetUnitPresence(ctx, unitName)
}

// DeleteUnitPresence removes the presence of the unit in the model. If the unit
// is not found, it ignores the error. The unit life is not considered when
// deleting the presence.
func (s *Service) DeleteUnitPresence(ctx context.Context, unitName coreunit.Name) error {
	if err := unitName.Validate(); err != nil {
		return errors.Trace(err)
	}
	return s.st.DeleteUnitPresence(ctx, unitName)
}
