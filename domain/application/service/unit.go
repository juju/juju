// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/agentbinary"
	coreapplication "github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/k8s"
	"github.com/juju/juju/core/leadership"
	corelife "github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/errors"
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

	// SetRunningAgentBinaryVersion sets the running agent version for the unit.
	// The following error types can be expected:
	// - [applicationerrors.UnitNotFound] - when the unit does not exist.
	// - [applicationerrors.UnitIsDead] - when the unit is dead.
	// - [coreerrors.NotSupported] - when the architecture is not supported.
	SetRunningAgentBinaryVersion(context.Context, coreunit.UUID, agentbinary.Version) error
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

// SetUnitPassword updates the password for the specified unit, returning an error
// satisfying [applicationerrors.NotNotFound] if the unit doesn't exist.
func (s *Service) SetUnitPassword(ctx context.Context, unitName coreunit.Name, password string) error {
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return errors.Capture(err)
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
		return errors.Errorf("password hash %w", coreerrors.NotValid)
	}
	if args.ProviderID == "" {
		return errors.Errorf("provider id %w", coreerrors.NotValid)
	}
	if !args.OrderedScale {
		return errors.Errorf("registering CAAS units not supported without ordered unit IDs").Add(coreerrors.NotImplemented)
	}
	if args.UnitName == "" {
		return errors.Errorf("missing unit name %w", coreerrors.NotValid)
	}

	appUUID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return errors.Errorf("getting application ID: %w", err)
	}
	err = s.st.RegisterCAASUnit(ctx, appUUID, args)
	if err != nil {
		return errors.Errorf("saving caas unit %q: %w", args.UnitName, err)
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
	cloudContainerStatus, err := encodeCloudContainerStatus(params.CloudContainerStatus)
	if err != nil {
		return errors.Errorf("encoding cloud container status: %w", err)
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
		return errors.Errorf("updating caas unit %q: %w", unitName, err)
	}
	return nil
}

// SetReportedUnitAgentVersion sets the reported agent version for the
// supplied unit name. Reported agent version is the version that the agent
// binary on this unit has reported it is running.
//
// The following errors are possible:
// - [coreerrors.NotValid] - when the reportedVersion is not valid.
// - [coreerrors.NotSupported] - when the architecture is not supported.
// - [applicationerrors.UnitNotFound] - when the unit does not exist.
// - [applicationerrors.UnitIsDead] - when the unit is dead.
func (s *Service) SetReportedUnitAgentVersion(ctx context.Context, unitName coreunit.Name, reportedVersion agentbinary.Version) error {
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

	if err := s.st.SetRunningAgentBinaryVersion(ctx, unitUUID, reportedVersion); err != nil {
		return errors.Errorf(
			"setting unit %q reported agent version (%s) in state: %w",
			unitUUID,
			reportedVersion.Number.String(),
			err,
		)
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
	// For now, all we do is advance the unit's life to Dying.
	err := s.st.SetUnitLife(ctx, unitName, life.Dying)
	if err != nil {
		return errors.Errorf("destroying unit %q: %w", unitName, err)
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
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return "", errors.Errorf("getting UUID of unit %q: %w", unitName, err)
	}
	return unitUUID, nil
}

// GetUnitLife looks up the life of the specified unit, returning an error
// satisfying [applicationerrors.UnitNotFoundError] if the unit is not found.
func (s *Service) GetUnitLife(ctx context.Context, unitName coreunit.Name) (corelife.Value, error) {
	unitLife, err := s.st.GetUnitLife(ctx, unitName)
	if err != nil {
		return "", errors.Errorf("getting life for %q: %w", unitName, err)
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
		return errors.Errorf("deleting unit %q: %w", unitName, err)
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
	deploymentType := k8s.K8sDeploymentStateful
	restart := true

	switch deploymentType {
	case k8s.K8sDeploymentStateful:
		caasApp := broker.Application(appName, k8s.K8sDeploymentStateful)
		appState, err := caasApp.State()
		if err != nil {
			return false, errors.Capture(err)
		}
		appID, err := s.st.GetApplicationIDByName(ctx, appName)
		if err != nil {
			return false, errors.Capture(err)
		}
		scaleInfo, err := s.st.GetApplicationScaleState(ctx, appID)
		if err != nil {
			return false, errors.Capture(err)
		}
		if unitNum >= scaleInfo.Scale || unitNum >= appState.DesiredReplicas {
			restart = false
		}
	case k8s.K8sDeploymentStateless, k8s.K8sDeploymentDaemon:
		// Both handled the same way.
		restart = true
	default:
		return false, errors.Errorf("unknown deployment type %w", coreerrors.NotSupported)
	}
	return restart, nil
}
