// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/caas"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/leadership"
	corelife "github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	internalerrors "github.com/juju/juju/internal/errors"
)

// AddUnits adds the specified units to the application, returning an error
// satisfying [applicationerrors.ApplicationNotFoundError] if the application
// doesn't exist.
// If no units are provided, it will return nil.
func (s *Service) AddUnits(ctx context.Context, appName string, units ...AddUnitArg) error {
	if !isValidApplicationName(appName) {
		return applicationerrors.ApplicationNameNotValid
	}

	if len(units) == 0 {
		return nil
	}

	appUUID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return internalerrors.Errorf("getting application %q id: %w", appName, err)
	}

	modelType, err := s.st.GetModelType(ctx)
	if err != nil {
		return internalerrors.Errorf("getting model type: %w", err)
	}

	args, err := s.makeUnitArgs(modelType, units)
	if err != nil {
		return internalerrors.Errorf("making unit args: %w", err)
	}

	if err := s.st.AddUnits(ctx, appUUID, args); err != nil {
		return internalerrors.Errorf("adding units to application %q: %w", appName, err)
	}

	for _, arg := range args {
		unitName := arg.UnitName.String()

		if agentStatus, err := decodeUnitAgentStatus(arg.UnitStatusArg.AgentStatus); err == nil && agentStatus != nil {
			if err := s.statusHistory.RecordStatus(ctx, unitAgentNamespace.WithID(unitName), *agentStatus); err != nil {
				return internalerrors.Errorf("recording agent status: %w", err)
			}
		}

		if workloadStatus, err := decodeWorkloadStatus(arg.UnitStatusArg.WorkloadStatus); err == nil && workloadStatus != nil {
			if err := s.statusHistory.RecordStatus(ctx, unitWorkloadNamespace.WithID(unitName), *workloadStatus); err != nil {
				return internalerrors.Errorf("recording workload status: %w", err)
			}
		}
	}

	return nil
}

func (s *Service) makeUnitArgs(modelType coremodel.ModelType, units []AddUnitArg) ([]application.AddUnitArg, error) {
	now := ptr(s.clock.Now())
	workloadMessage := corestatus.MessageInstallingAgent
	if modelType == coremodel.IAAS {
		workloadMessage = corestatus.MessageWaitForMachine
	}

	args := make([]application.AddUnitArg, len(units))
	for i, u := range units {
		arg := application.AddUnitArg{
			UnitName: u.UnitName,
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

// SetUnitWorkloadStatus sets the workload status of the specified unit, returning an
// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (s *Service) SetUnitWorkloadStatus(ctx context.Context, unitName coreunit.Name, status *corestatus.StatusInfo) error {
	if err := unitName.Validate(); err != nil {
		return errors.Trace(err)
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

	if status == nil {
		return nil
	}

	return s.statusHistory.RecordStatus(ctx, unitWorkloadNamespace.WithID(unitName.String()), *status)
}

// GetUnitWorkloadStatusesForApplication returns the workload statuses of all
// units in the specified application, indexed by unit name, returning an error satisfying
// [applicationerrors.ApplicationNotFound] if the application doesn't exist.
func (s *Service) GetUnitWorkloadStatusesForApplication(ctx context.Context, appID coreapplication.ID) (map[coreunit.Name]corestatus.StatusInfo, error) {
	if err := appID.Validate(); err != nil {
		return nil, internalerrors.Errorf("application ID: %w", err)
	}

	statuses, err := s.st.GetUnitWorkloadStatusesForApplication(ctx, appID)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	ret := make(map[coreunit.Name]corestatus.StatusInfo, len(statuses))
	for unitName, status := range statuses {
		info, err := decodeWorkloadStatus(&status)
		if err != nil {
			return nil, internalerrors.Capture(err)
		}
		ret[unitName] = *info
	}
	return ret, nil
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
	if errors.Is(err, applicationerrors.UnitStatusNotFound) {
		return unitDisplayStatus(workloadStatus, nil)
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return unitDisplayStatus(workloadStatus, containerStatus)
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

// GetUnitWorkloadStatus returns the workload status of the specified unit, returning an
// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
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
	return decodeWorkloadStatus(workloadStatus)
}

// RegisterCAASUnit creates or updates the specified application unit in a caas model,
// returning an error satisfying [applicationerrors.ApplicationNotFoundError]
// if the application doesn't exist. If the unit life is Dead, an error
// satisfying [applicationerrors.UnitAlreadyExists] is returned.
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
	err = s.st.InsertCAASUnit(ctx, appUUID, args)
	if err != nil {
		return internalerrors.Errorf("saving caas unit %q: %w", args.UnitName, err)
	}
	return nil
}

// UpdateCAASUnit updates the specified CAAS unit, returning an error
// satisfying applicationerrors.ApplicationNotAlive if the unit's
// application is not alive.
func (s *Service) UpdateCAASUnit(ctx context.Context, unitName coreunit.Name, params UpdateCAASUnitParams) error {
	appName, err := names.UnitApplication(unitName.String())
	if err != nil {
		return errors.Trace(err)
	}
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

// RemoveUnit is called by the deployer worker and caas application provisioner worker to
// remove from the model units which have transitioned to dead.
// TODO(units): revisit his existing logic ported from mongo
// Note: the callers of this method only do so after the unit has become dead, so
// there's strictly no need to set the life to Dead before removing.
// If the unit is still alive, an error satisfying [applicationerrors.UnitIsAlive]
// is returned. If the unit is not found, an error satisfying
// [applicationerrors.UnitNotFound] is returned.
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
	appName, _ := names.UnitApplication(unitName.String())
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
	appName, _ := names.UnitApplication(unitName.String())
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
// the agent receives a signal to exit. UnitTerminating will return how
// the agent should shutdown.
// We pass in a CAAS broker to get app details from the k8s cluster - we will probably
// make it a service attribute once more use cases emerge.
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
