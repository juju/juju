// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/juju/clock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	corerelation "github.com/juju/juju/core/relation"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/status"
	statuserrors "github.com/juju/juju/domain/status/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

// ModelState describes retrieval and persistence methods for the statuses of
// applications and units.
type ModelState interface {
	StorageState
	// GetAllRelationStatuses returns all the relation statuses of the given model.
	GetAllRelationStatuses(ctx context.Context) ([]status.RelationStatusInfo, error)

	// GetApplicationIDByName returns the application ID for the named
	// application. If no application is found, an error satisfying
	// [statuserrors.ApplicationNotFound] is returned.
	GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error)

	// GetApplicationIDAndNameByUnitName returns the application ID and name for
	// the named unit, returning an error satisfying
	// [statuserrors.UnitNotFound] if the unit doesn't exist.
	GetApplicationIDAndNameByUnitName(ctx context.Context, name coreunit.Name) (coreapplication.ID, string, error)

	// GetApplicationStatus looks up the status of the specified application,
	// returning an error satisfying [statuserrors.ApplicationNotFound] if the
	// application is not found.
	GetApplicationStatus(ctx context.Context, appID coreapplication.ID) (status.StatusInfo[status.WorkloadStatusType], error)

	// SetApplicationStatus sets the given application status, overwriting any
	// current status data. If returns an error satisfying
	// [statuserrors.ApplicationNotFound] if the application doesn't exist.
	SetApplicationStatus(
		ctx context.Context,
		applicationID coreapplication.ID,
		status status.StatusInfo[status.WorkloadStatusType],
	) error

	// SetRelationStatus sets the given relation status and checks that the
	// transition to the new status from the current status is valid. It can
	// return the following errors:
	//   - [statuserrors.RelationNotFound] if the relation doesn't exist.
	//   - [statuserrors.RelationStatusTransitionNotValid] if the current
	//     relation status cannot transition to the new relation status. the
	//     relation does not exist.
	SetRelationStatus(
		ctx context.Context,
		relationUUID corerelation.UUID,
		sts status.StatusInfo[status.RelationStatusType],
	) error

	// GetRelationUUIDByID returns the UUID for the given relation ID.
	// It can return the following errors:
	//   - [statuserrors.RelationNotFound] if the relation doesn't exist.
	GetRelationUUIDByID(
		ctx context.Context,
		id int,
	) (corerelation.UUID, error)

	// ImportRelationStatus sets the given relation status. It can return the
	// following errors:
	//   - [statuserrors.RelationNotFound] if the relation doesn't exist.
	ImportRelationStatus(
		ctx context.Context,
		relationUUID corerelation.UUID,
		sts status.StatusInfo[status.RelationStatusType],
	) error

	// GetUnitUUIDByName returns the UUID for the named unit, returning an
	// error satisfying [statuserrors.UnitNotFound] if the unit doesn't
	// exist.
	GetUnitUUIDByName(context.Context, coreunit.Name) (coreunit.UUID, error)

	// GetUnitWorkloadStatus returns the workload status of the specified unit,
	// returning:
	// - an error satisfying [statuserrors.UnitNotFound] if the unit
	//   doesn't exist or;
	// - an error satisfying [statuserrors.UnitStatusNotFound] if the
	//   status is not set.
	GetUnitWorkloadStatus(context.Context, coreunit.UUID) (status.UnitStatusInfo[status.WorkloadStatusType], error)

	// SetUnitWorkloadStatus sets the workload status of the specified unit,
	// returning an error satisfying [statuserrors.UnitNotFound] if the
	// unit doesn't exist.
	SetUnitWorkloadStatus(context.Context, coreunit.UUID, status.StatusInfo[status.WorkloadStatusType]) error

	// GetUnitK8sPodStatus returns the k8s pod status of the
	// specified unit. It returns;
	// - an error satisfying [statuserrors.UnitNotFound] if the unit
	//   doesn't exist
	GetUnitK8sPodStatus(context.Context, coreunit.UUID) (status.StatusInfo[status.K8sPodStatusType], error)

	// GetUnitWorkloadStatusesForApplication returns the workload statuses for
	// all units of the specified application, returning:
	//   - an error satisfying [statuserrors.ApplicationNotFound] if the
	//     application doesn't exist or;
	//   - error satisfying [statuserrors.ApplicationIsDead] if the
	//     application is dead.
	GetUnitWorkloadStatusesForApplication(context.Context, coreapplication.ID) (status.UnitWorkloadStatuses, error)

	// GetUnitAgentStatusesForApplication returns the agent statuses for
	// all units of the specified application, returning:
	//   - an error satisfying [statuserrors.ApplicationNotFound] if the
	//     application doesn't exist or;
	//   - error satisfying [statuserrors.ApplicationIsDead] if the
	//     application is dead.
	GetUnitAgentStatusesForApplication(context.Context, coreapplication.ID) (status.UnitAgentStatuses, error)

	// GetAllFullUnitStatusesForApplication returns the workload statuses and
	// the cloud container statuses for all units of the specified application,
	// returning:
	//   - an error satisfying [statuserrors.ApplicationNotFound] if the
	//     application doesn't exist or;
	//   - an error satisfying [statuserrors.ApplicationIsDead] if the
	//     application is dead.
	GetAllFullUnitStatusesForApplication(
		context.Context, coreapplication.ID,
	) (status.FullUnitStatuses, error)

	// GetUnitAgentStatus returns the workload status of the specified unit,
	// returning:
	// - an error satisfying [statuserrors.UnitNotFound] if the unit
	//   doesn't exist or;
	// - an error satisfying [statuserrors.UnitStatusNotFound] if the
	//   status is not set.
	GetUnitAgentStatus(context.Context, coreunit.UUID) (status.UnitStatusInfo[status.UnitAgentStatusType], error)

	// SetUnitAgentStatus sets the workload status of the specified unit,
	// returning an error satisfying [statuserrors.UnitNotFound] if the
	// unit doesn't exist.
	SetUnitAgentStatus(context.Context, coreunit.UUID, status.StatusInfo[status.UnitAgentStatusType]) error

	// GetAllUnitWorkloadAgentStatuses retrieves the presence, workload status,
	// and agent status of every unit in the model. Returns an error satisfying
	// [statuserrors.UnitStatusNotFound] if any units do not have statuses.
	GetAllUnitWorkloadAgentStatuses(context.Context) (status.UnitWorkloadAgentStatuses, error)

	// GetAllApplicationStatuses returns the statuses of all the applications in
	// the model, indexed by application name, if they have a status set.
	GetAllApplicationStatuses(context.Context) (map[string]status.StatusInfo[status.WorkloadStatusType], error)

	// SetUnitPresence marks the presence of the specified unit, returning an
	// error satisfying [applicationerrors.UnitNotFound] if the unit doesn't
	// exist. The unit life is not considered when making this query.
	SetUnitPresence(ctx context.Context, name coreunit.Name) error

	// DeleteUnitPresence removes the presence of the specified unit. If the
	// unit isn't found it ignores the error.
	// The unit life is not considered when making this query.
	DeleteUnitPresence(ctx context.Context, name coreunit.Name) error

	// GetApplicationAndUnitStatuses returns the application statuses of all the
	// applications in the model, indexed by application name.
	GetApplicationAndUnitStatuses(ctx context.Context) (map[string]status.Application, error)

	// GetApplicationAndUnitModelStatuses returns the application name and unit
	// count for each model for the model status request.
	GetApplicationAndUnitModelStatuses(ctx context.Context) (map[string]int, error)

	// GetMachineStatus returns the status of the specified machine.
	// This method may return the following errors:
	// - [machineerrors.MachineNotFound] if the machine does not exist.
	// - [statuserrors.MachineStatusNotFound] if the status is not set.
	GetMachineStatus(ctx context.Context, machineName string) (status.StatusInfo[status.MachineStatusType], error)

	// SetMachineStatus sets the status of the specified machine.
	// This method may return the following errors:
	// - [machineerrors.MachineNotFound] if the machine does not exist.
	SetMachineStatus(ctx context.Context, machineName string, status status.StatusInfo[status.MachineStatusType]) error

	// GetAllMachineStatuses returns all the machine statuses for the model,
	// indexed by machine name.
	GetAllMachineStatuses(context.Context) (map[string]status.StatusInfo[status.MachineStatusType], error)

	// GetMachineFullStatuses returns all the machine statuses for the model,
	// indexed by machine name.
	GetMachineFullStatuses(context.Context) (map[machine.Name]status.Machine, error)

	// GetInstanceStatus returns the cloud specific instance status for the
	// given machine.
	// This method may return the following errors:
	// - [machineerrors.MachineNotFound] if the machine does not exist or;
	// - [statuserrors.MachineStatusNotFound] if the status is not set.
	GetInstanceStatus(ctx context.Context, machineName string) (status.StatusInfo[status.InstanceStatusType], error)

	// GetAllInstanceStatuses returns all the instance statuses for the model,
	// indexed by machine name.
	GetAllInstanceStatuses(context.Context) (map[string]status.StatusInfo[status.InstanceStatusType], error)

	// SetInstanceStatus sets the cloud specific instance status for this
	// machine.
	// This method may return the following errors:
	// - [machineerrors.NotProvisioned] if the machine does not exist.
	SetInstanceStatus(ctx context.Context, machienName string, status status.StatusInfo[status.InstanceStatusType]) error

	// GetModelStatusInfo returns information about the current model.
	// The following error types can be expected to be returned:
	// - [github.com/juju/juju/domain/model/errors.NotFound]: When the model
	// does not exist.
	GetModelStatusInfo(ctx context.Context) (status.ModelStatusInfo, error)
}

// ControllerState is the controller state required by the service.
type ControllerState interface {
	// GetModelStatusContext returns the status context for the given model.
	// It returns [github.com/juju/juju/domain/model/errors.NotFound] if the model no longer exists.
	GetModelStatusContext(context.Context) (status.ModelStatusContext, error)
}

// Service provides the API for working with the statuses of applications and
// units and the model.
type Service struct {
	modelState            ModelState
	controllerState       ControllerState
	statusHistory         StatusHistory
	statusHistoryReaderFn StatusHistoryReaderFunc
	logger                logger.Logger
	clock                 clock.Clock
}

// NewService returns a new service reference wrapping the input state.
func NewService(
	modelState ModelState,
	controllerState ControllerState,
	statusHistory StatusHistory,
	statusHistoryReaderFn StatusHistoryReaderFunc,
	clock clock.Clock,
	logger logger.Logger,
) *Service {
	return &Service{
		modelState:            modelState,
		controllerState:       controllerState,
		statusHistory:         statusHistory,
		statusHistoryReaderFn: statusHistoryReaderFn,
		logger:                logger,
		clock:                 clock,
	}
}

// GetAllRelationStatuses returns all the relation statuses of the given model.
func (s *Service) GetAllRelationStatuses(ctx context.Context) (map[corerelation.UUID]corestatus.StatusInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	statuses, err := s.modelState.GetAllRelationStatuses(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	result := make(map[corerelation.UUID]corestatus.StatusInfo, len(statuses))
	for _, sts := range statuses {
		decodedStatus, err := decodeRelationStatusType(sts.StatusInfo.Status)
		if err != nil {
			return nil, errors.Capture(err)
		}
		result[sts.RelationUUID] = corestatus.StatusInfo{
			Status:  decodedStatus,
			Message: sts.StatusInfo.Message,
			Since:   sts.StatusInfo.Since,
		}
	}
	return result, nil
}

// SetApplicationStatus validates and sets the given application status, overwriting any
// current status data. If returns an error satisfying
// [statuserrors.ApplicationNotFound] if the application doesn't exist.
func (s *Service) SetApplicationStatus(
	ctx context.Context,
	applicationName string,
	statusInfo corestatus.StatusInfo,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// This will also verify that the status is valid.
	encodedStatus, err := encodeWorkloadStatus(statusInfo)
	if err != nil {
		return errors.Errorf("encoding workload status: %w", err)
	}

	applicationID, err := s.modelState.GetApplicationIDByName(ctx, applicationName)
	if err != nil {
		return errors.Capture(err)
	}

	if err := s.modelState.SetApplicationStatus(ctx, applicationID, encodedStatus); err != nil {
		return errors.Capture(err)
	}

	if err := s.statusHistory.RecordStatus(ctx, status.ApplicationNamespace.WithID(applicationID.String()), statusInfo); err != nil {
		s.logger.Infof(ctx, "failed recording setting application status history: %v", err)
	}

	return nil
}

// GetApplicationDisplayStatus returns the display status of the specified
// application. The display status is equal to the application status if it is
// set, otherwise it is derived from the unit display statuses. If no
// application is found, an error satisfying [statuserrors.ApplicationNotFound]
// is returned.
func (s *Service) GetApplicationDisplayStatus(ctx context.Context, appName string) (corestatus.StatusInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	appID, err := s.modelState.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return corestatus.StatusInfo{}, errors.Capture(err)
	}
	applicationStatus, err := s.modelState.GetApplicationStatus(ctx, appID)
	if err != nil {
		return corestatus.StatusInfo{}, errors.Capture(err)
	}
	return s.decodeApplicationDisplayStatus(ctx, appID, applicationStatus)
}

// SetUnitWorkloadStatus sets the workload status of the specified unit,
// returning an error satisfying [statuserrors.UnitNotFound] if the unit
// doesn't exist.
func (s *Service) SetUnitWorkloadStatus(ctx context.Context, unitName coreunit.Name, statusInfo corestatus.StatusInfo) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}

	workloadStatus, err := encodeWorkloadStatus(statusInfo)
	if err != nil {
		return errors.Errorf("encoding workload status: %w", err)
	}
	unitUUID, err := s.modelState.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return errors.Capture(err)
	}
	if err := s.modelState.SetUnitWorkloadStatus(ctx, unitUUID, workloadStatus); err != nil {
		return errors.Errorf("setting workload status: %w", err)
	}

	if err := s.statusHistory.RecordStatus(ctx, status.UnitWorkloadNamespace.WithID(unitName.String()), statusInfo); err != nil {
		s.logger.Infof(ctx, "failed recording setting workload status for unit %q: %v", unitName, err)
	}
	return nil
}

// GetUnitWorkloadStatus returns the workload status of the specified unit,
// returning an error satisfying [statuserrors.UnitNotFound] if the unit
// doesn't exist.
func (s *Service) GetUnitWorkloadStatus(ctx context.Context, unitName coreunit.Name) (corestatus.StatusInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return corestatus.StatusInfo{}, errors.Capture(err)
	}
	unitUUID, err := s.modelState.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return corestatus.StatusInfo{}, errors.Capture(err)
	}
	workloadStatus, err := s.modelState.GetUnitWorkloadStatus(ctx, unitUUID)
	if err != nil {
		return corestatus.StatusInfo{}, errors.Capture(err)
	}

	return decodeUnitWorkloadStatus(workloadStatus.StatusInfo, workloadStatus.Present)
}

// SetUnitAgentStatus sets the agent status of the specified unit,
// returning an error satisfying [statuserrors.UnitNotFound] if the unit
// doesn't exist.
func (s *Service) SetUnitAgentStatus(ctx context.Context, unitName coreunit.Name, statusInfo corestatus.StatusInfo) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}

	// Encoding the status will handle invalid status values.
	switch statusInfo.Status {
	case corestatus.Error:
		if statusInfo.Message == "" {
			return errors.Errorf("setting status %q without message", statusInfo.Status)
		}
	case corestatus.Lost, corestatus.Allocating:
		return errors.Errorf("setting status %q is not allowed", statusInfo.Status)
	}

	agentStatus, err := encodeUnitAgentStatus(statusInfo)
	if err != nil {
		return errors.Errorf("encoding agent status: %w", err)
	}
	unitUUID, err := s.modelState.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return errors.Capture(err)
	}
	if err := s.modelState.SetUnitAgentStatus(ctx, unitUUID, agentStatus); err != nil {
		return errors.Errorf("setting agent status: %w", err)
	}

	if err := s.statusHistory.RecordStatus(ctx, status.UnitAgentNamespace.WithID(unitName.String()), statusInfo); err != nil {
		s.logger.Infof(ctx, "failed recording setting agent status for unit %q: %v", unitName, err)
	}
	return nil
}

// GetUnitWorkloadStatusesForApplication returns the workload statuses of all
// units in the specified application, indexed by unit name, returning an error
// satisfying [statuserrors.ApplicationNotFound] if the application doesn't
// exist.
func (s *Service) GetUnitWorkloadStatusesForApplication(ctx context.Context, appID coreapplication.ID) (map[coreunit.Name]corestatus.StatusInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appID.Validate(); err != nil {
		return nil, errors.Errorf("application ID: %w", err)
	}

	statuses, err := s.modelState.GetUnitWorkloadStatusesForApplication(ctx, appID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	decoded, err := decodeUnitWorkloadStatuses(statuses)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return decoded, nil
}

// GetUnitAgentStatusesForApplication returns the agent statuses of all
// units in the specified application, indexed by unit name, returning an error
// satisfying [statuserrors.ApplicationNotFound] if the application doesn't
// exist.
func (s *Service) GetUnitAgentStatusesForApplication(ctx context.Context, appID coreapplication.ID) (map[coreunit.Name]corestatus.StatusInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := appID.Validate(); err != nil {
		return nil, errors.Errorf("application ID: %w", err)
	}

	statuses, err := s.modelState.GetUnitAgentStatusesForApplication(ctx, appID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	decoded, err := decodeUnitAgentStatusesWithoutPresence(statuses)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return decoded, nil
}

// GetUnitDisplayAndAgentStatus returns the unit and agent display status of the
// specified unit. The display status a function of both the unit workload
// status and the cloud container status. It returns an error satisfying
// [statuserrors.UnitNotFound] if the unit doesn't exist.
func (s *Service) GetUnitDisplayAndAgentStatus(ctx context.Context, unitName coreunit.Name) (agent corestatus.StatusInfo, workload corestatus.StatusInfo, err error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return agent, workload, errors.Capture(err)
	}

	// TODO (stickupkid/jack-w-shaw) This should just be 1 or 2 calls to the
	// state layer to get the statuses. We even have a view for this!

	unitUUID, err := s.modelState.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return agent, workload, errors.Capture(err)
	}

	agentStatus, err := s.modelState.GetUnitAgentStatus(ctx, unitUUID)
	if err != nil {
		return agent, workload, errors.Capture(err)
	}

	workloadStatus, err := s.modelState.GetUnitWorkloadStatus(ctx, unitUUID)
	if err != nil {
		return agent, workload, errors.Capture(err)
	}

	k8sPodStatus, err := s.modelState.GetUnitK8sPodStatus(ctx, unitUUID)
	if err != nil {
		return agent, workload, errors.Capture(err)
	}

	return decodeUnitDisplayAndAgentStatus(status.FullUnitStatus{
		WorkloadStatus: workloadStatus.StatusInfo,
		AgentStatus:    agentStatus.StatusInfo,
		K8sPodStatus:   k8sPodStatus,
		Present:        workloadStatus.Present,
	})
}

// SetUnitPresence marks the presence of the unit in the model. It is called by
// the unit agent accesses the API server. If the unit is not found, an error
// satisfying [applicationerrors.UnitNotFound] is returned. The unit life is not
// considered when setting the presence.
func (s *Service) SetUnitPresence(ctx context.Context, unitName coreunit.Name) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}
	return s.modelState.SetUnitPresence(ctx, unitName)
}

// DeleteUnitPresence removes the presence of the unit in the model. If the unit
// is not found, it ignores the error. The unit life is not considered when
// deleting the presence.
func (s *Service) DeleteUnitPresence(ctx context.Context, unitName coreunit.Name) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}
	return s.modelState.DeleteUnitPresence(ctx, unitName)
}

// CheckUnitStatusesReadyForMigration returns an error if the statuses of any units
// in the model indicate they cannot be migrated.
func (s *Service) CheckUnitStatusesReadyForMigration(ctx context.Context) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	fullStatuses, err := s.modelState.GetAllUnitWorkloadAgentStatuses(ctx)
	if err != nil {
		return errors.Errorf("getting unit statuses: %w", err)
	}

	var failedChecks []string
	for unitName, fullStatus := range fullStatuses {
		present, agentStatus, workloadStatus, err := decodeUnitWorkloadAgentStatus(fullStatus)
		if err != nil {
			return errors.Errorf("decoding full unit status for unit %q: %w", unitName, err)
		}
		if !present {
			failedChecks = append(failedChecks, fmt.Sprintf("- unit %q is not logged into the controller", unitName))
		}
		if !corestatus.IsAgentPresent(agentStatus) {
			failedChecks = append(failedChecks, fmt.Sprintf("- unit %q agent not idle or executing", unitName))
		}
		if !corestatus.IsUnitWorkloadPresent(workloadStatus) {
			failedChecks = append(failedChecks, fmt.Sprintf("- unit %q workload not active or viable", unitName))
		}
	}

	if len(failedChecks) > 0 {
		return errors.Errorf(
			"model unit(s) are not ready for migration:\n%s", strings.Join(failedChecks, "\n"))
	}
	return nil
}

// GetApplicationAndUnitStatuses returns the application statuses of all the
// applications in the model, indexed by application name.
func (s *Service) GetApplicationAndUnitStatuses(ctx context.Context) (map[string]Application, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	statuses, err := s.modelState.GetApplicationAndUnitStatuses(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	results := make(map[string]Application, len(statuses))
	for appName, app := range statuses {
		decoded, err := s.decodeApplicationStatusDetails(ctx, app)
		if err != nil {
			return nil, errors.Errorf("decoding application status for %q: %w", appName, err)
		}
		results[appName] = decoded
	}

	return results, nil
}

// GetApplicationAndUnitModelStatuses returns the application name and unit
// count for each model for the model status request.
func (s *Service) GetApplicationAndUnitModelStatuses(ctx context.Context) (map[string]int, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.modelState.GetApplicationAndUnitModelStatuses(ctx)
}

// ImportRelationStatus sets the status of the relation to the status provided.
// It can return the following errors:
//   - [statuserrors.RelationNotFound] if the relation doesn't exist.
func (s *Service) ImportRelationStatus(
	ctx context.Context,
	relationID int,
	info corestatus.StatusInfo,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// Encode status.
	relationStatus, err := encodeRelationStatus(info)
	if err != nil {
		return errors.Errorf("encoding relation status: %w", err)
	}

	relationUUID, err := s.modelState.GetRelationUUIDByID(ctx, relationID)
	if err != nil {
		return errors.Errorf("getting UUID for relation %d: %w", relationID, err)
	}

	return s.modelState.ImportRelationStatus(ctx, relationUUID, relationStatus)
}

// GetInstanceStatus returns the cloud specific instance status for this
// machine.
// This method may return the following errors:
// - [machineerrors.MachineNotFound] if the machine does not exist or;
// - [statuserrors.MachineStatusNotFound] if the status is not set.
func (s *Service) GetInstanceStatus(ctx context.Context, machineName machine.Name) (corestatus.StatusInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineName.Validate(); err != nil {
		return corestatus.StatusInfo{}, errors.Errorf("validating machine name: %w", err)
	}

	instanceStatus, err := s.modelState.GetInstanceStatus(ctx, machineName.String())
	if err != nil {
		return corestatus.StatusInfo{}, errors.Errorf("retrieving instance status for machine %q: %w", machineName, err)
	}

	return decodeInstanceStatus(instanceStatus)
}

// SetInstanceStatus sets the cloud specific instance status for this machine.
// This method may return the following errors:
// - [machineerrors.NotProvisioned] if the machine instance does not exist.
// - [statuserrors.InvalidStatus] if the given status is not a known status value.
func (s *Service) SetInstanceStatus(ctx context.Context, machineName machine.Name, statusInfo corestatus.StatusInfo) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineName.Validate(); err != nil {
		return errors.Errorf("validating machine name %q: %w", machineName, err)
	}

	if !statusInfo.Status.KnownInstanceStatus() {
		return statuserrors.InvalidStatus
	}

	instanceStatus, err := encodeInstanceStatus(statusInfo)
	if err != nil {
		return errors.Errorf("encoding status for machine %q: %w", machineName, err)
	}

	if err := s.modelState.SetInstanceStatus(ctx, machineName.String(), instanceStatus); err != nil {
		return errors.Errorf("setting instance status for machine %q: %w", machineName, err)
	}

	if err := s.statusHistory.RecordStatus(ctx, status.MachineInstanceNamespace.WithID(machineName.String()), statusInfo); err != nil {
		s.logger.Infof(ctx, "failed recording instance status history: %w", err)
	}

	return nil
}

// GetMachineStatus returns the status of the specified machine.
// This method may return the following errors:
// - [machineerrors.MachineNotFound] if the machine does not exist.
// - [statuserrors.MachineStatusNotFound] if the status is not set.
func (s *Service) GetMachineStatus(ctx context.Context, machineName machine.Name) (corestatus.StatusInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineName.Validate(); err != nil {
		return corestatus.StatusInfo{}, errors.Errorf("validating machine name: %w", err)
	}

	machineStatus, err := s.modelState.GetMachineStatus(ctx, machineName.String())
	if err != nil {
		return corestatus.StatusInfo{}, errors.Errorf("retrieving machine status for machine %q: %w", machineName, err)
	}
	return decodeMachineStatus(machineStatus)
}

// GetAllMachineStatuses returns all the machine statuses for the model, indexed
// by machine name.
func (s *Service) GetAllMachineStatuses(ctx context.Context) (map[machine.Name]corestatus.StatusInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	machineStatuses, err := s.modelState.GetAllMachineStatuses(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make(map[machine.Name]corestatus.StatusInfo, len(machineStatuses))
	for name, status := range machineStatuses {
		machineName := machine.Name(name)
		if err := machineName.Validate(); err != nil {
			return nil, errors.Errorf("validating returned machine name %q: %w", name, err)
		}
		result[machineName], err = decodeMachineStatus(status)
		if err != nil {
			return nil, errors.Errorf("decoding machine status for machine %q: %w", machineName, err)
		}
	}
	return result, nil
}

// GetMachineFullStatuses returns all the machine statuses for the model, indexed
// by machine name.
func (s *Service) GetMachineFullStatuses(ctx context.Context) (map[machine.Name]Machine, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	machineStatuses, err := s.modelState.GetMachineFullStatuses(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make(map[machine.Name]Machine, len(machineStatuses))
	for name, m := range machineStatuses {
		if err := name.Validate(); err != nil {
			return nil, errors.Errorf("validating returned machine name %q: %w", name, err)
		}

		decodedStatus, err := s.decodeMachineStatusDetails(name, m)
		if err != nil {
			return nil, errors.Errorf("decoding machine status for %q: %w", name, err)
		}
		result[name] = decodedStatus
	}
	return result, nil
}

// SetMachineStatus sets the status of the specified machine.
// This method may return the following errors:
//   - [machineerrors.MachineNotFound] if the machine does not exist.
//   - [statuserrors.InvalidStatus] if the given status is not a known status
//     value.
func (s *Service) SetMachineStatus(ctx context.Context, machineName machine.Name, statusInfo corestatus.StatusInfo) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineName.Validate(); err != nil {
		return errors.Errorf("validating machine name %q: %w", machineName, err)
	}

	if !statusInfo.Status.KnownMachineStatus() {
		return statuserrors.InvalidStatus
	}

	machineStatus, err := encodeMachineStatus(statusInfo)
	if err != nil {
		return errors.Errorf("encoding status for machine %q: %w", machineName, err)
	}

	if err := s.modelState.SetMachineStatus(ctx, machineName.String(), machineStatus); err != nil {
		return errors.Errorf("setting machine status for machine %q: %w", machineName, err)
	}

	if err := s.statusHistory.RecordStatus(ctx, status.MachineNamespace.WithID(machineName.String()), statusInfo); err != nil {
		s.logger.Infof(ctx, "failed recording machine status history: %w", err)
	}

	return nil
}

// GetModelStatusInfo returns information about the current model for the
// purpose of reporting its status.
// The following error types can be expected to be returned:
//   - [github.com/juju/juju/domain/model/errors.NotFound]: When the model no
//     longer exists.
func (s *Service) GetModelStatusInfo(ctx context.Context) (status.ModelStatusInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.modelState.GetModelStatusInfo(ctx)
}

// GetModelStatus returns the current status of the model.
//
// The following error types can be expected to be returned:
//   - [github.com/juju/juju/domain/model/errors.NotFound]: When the model no
//     longer exists.
func (s *Service) GetModelStatus(ctx context.Context) (corestatus.StatusInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()
	modelState, err := s.controllerState.GetModelStatusContext(ctx)
	if err != nil {
		return corestatus.StatusInfo{}, errors.Capture(err)
	}
	return s.statusFromModelContext(modelState), nil
}

// CheckMachineStatusesReadyForMigration returns an error if the statuses of any
// machines in the model indicate they cannot be migrated.
func (s *Service) CheckMachineStatusesReadyForMigration(ctx context.Context) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	machineStatuses, err := s.modelState.GetAllMachineStatuses(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	instanceStatuses, err := s.modelState.GetAllInstanceStatuses(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	if len(machineStatuses) != len(instanceStatuses) {
		return errors.Errorf("some machines have unset statuses")
	}

	var failedChecks []string
	for machineName, mStatus := range machineStatuses {
		iStatus, ok := instanceStatuses[machineName]
		if !ok {
			return errors.Errorf("some machines have unset statuses")
		}

		machineStatus, err := decodeMachineStatus(mStatus)
		if err != nil {
			return errors.Errorf("decoding machine status for machine %q: %w", machineName, err)
		}
		instanceStatus, err := decodeInstanceStatus(iStatus)
		if err != nil {
			return errors.Errorf("decoding instance status for machine %q: %w", machineName, err)
		}

		if !corestatus.IsMachinePresent(machineStatus) {
			failedChecks = append(failedChecks,
				fmt.Sprintf("- machine %q status is not started", machineName))
		}
		if !corestatus.IsInstancePresent(instanceStatus) {
			failedChecks = append(failedChecks,
				fmt.Sprintf("- machine %q instance status is not running", machineName))
		}
	}

	if len(failedChecks) > 0 {
		return errors.Errorf(
			"model machines(s) are not ready for migration:\n%s", strings.Join(failedChecks, "\n"))
	}
	return nil
}

// ExportUnitStatuses workload and agent statuses of all the units in
// in the model, indexed by unit name.
//
// TODO(jack-w-shaw): Export the container statuses too.
func (s *Service) ExportUnitStatuses(ctx context.Context) (map[coreunit.Name]corestatus.StatusInfo, map[coreunit.Name]corestatus.StatusInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	fullStatuses, err := s.modelState.GetAllUnitWorkloadAgentStatuses(ctx)
	if err != nil {
		return nil, nil, errors.Errorf("getting unit statuses: %w", err)
	}

	workloadStatuses := make(map[coreunit.Name]corestatus.StatusInfo, len(fullStatuses))
	agentStatuses := make(map[coreunit.Name]corestatus.StatusInfo, len(fullStatuses))
	for unitName, fullStatus := range fullStatuses {
		_, agentStatus, workloadStatus, err := decodeUnitWorkloadAgentStatus(fullStatus)
		if err != nil {
			return nil, nil, errors.Errorf("decoding full unit status for unit %q: %w", unitName, err)
		}
		workloadStatuses[unitName] = workloadStatus
		agentStatuses[unitName] = agentStatus
	}
	return workloadStatuses, agentStatuses, nil
}

// ExportApplicationStatuses returns the statuses of all applications in the model,
// indexed by application name, if they have a status set.
func (s *Service) ExportApplicationStatuses(ctx context.Context) (map[string]corestatus.StatusInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	appStatuses, err := s.modelState.GetAllApplicationStatuses(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	ret := make(map[string]corestatus.StatusInfo, len(appStatuses))
	for name, status := range appStatuses {
		decoded, err := decodeApplicationStatus(status)
		if err != nil {
			return nil, errors.Errorf("decoding application status for %q: %w", name, err)
		}
		ret[name] = decoded
	}

	return ret, nil
}

// ExportRelationStatuses returns the statuses of all relations in the model.
func (s *Service) ExportRelationStatuses(ctx context.Context) (map[int]corestatus.StatusInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	relStatuses, err := s.modelState.GetAllRelationStatuses(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make(map[int]corestatus.StatusInfo, len(relStatuses))
	for _, sts := range relStatuses {
		decodedStatus, err := decodeRelationStatusType(sts.StatusInfo.Status)
		if err != nil {
			return nil, errors.Capture(err)
		}
		result[sts.RelationID] = corestatus.StatusInfo{
			Status:  decodedStatus,
			Message: sts.StatusInfo.Message,
			Since:   sts.StatusInfo.Since,
		}
	}

	return result, nil
}

func (s *Service) decodeApplicationStatusDetails(ctx context.Context, app status.Application) (Application, error) {
	life, err := app.Life.Value()
	if err != nil {
		return Application{}, errors.Errorf("decoding application life: %w", err)
	}
	decodedStatus, err := s.decodeApplicationDisplayStatus(ctx, app.ID, app.Status)
	if err != nil {
		return Application{}, errors.Errorf("decoding application status: %w", err)
	}
	lxdProfile, err := s.decodeLXDProfile(app.LXDProfile)
	if err != nil {
		return Application{}, errors.Errorf("decoding LXD profile: %w", err)
	}

	units, err := s.decodeUnitsStatusDetails(app.Units)
	if err != nil {
		return Application{}, errors.Errorf("decoding unit statuses: %w", err)
	}

	return Application{
		Life:            life,
		Status:          decodedStatus,
		Relations:       app.Relations,
		Subordinate:     app.Subordinate,
		CharmLocator:    app.CharmLocator,
		CharmVersion:    app.CharmVersion,
		Platform:        app.Platform,
		Channel:         app.Channel,
		Exposed:         app.Exposed,
		LXDProfile:      lxdProfile,
		Scale:           app.Scale,
		WorkloadVersion: app.WorkloadVersion,
		K8sProviderID:   app.K8sProviderID,
		Units:           units,
	}, nil
}

func (s *Service) decodeApplicationDisplayStatus(
	ctx context.Context,
	appID coreapplication.ID,
	statusInfo status.StatusInfo[status.WorkloadStatusType],
) (corestatus.StatusInfo, error) {
	if statusInfo.Status != status.WorkloadStatusUnset {
		return decodeApplicationStatus(statusInfo)
	}

	fullUnitStatuses, err := s.modelState.GetAllFullUnitStatusesForApplication(ctx, appID)
	if err != nil {
		return corestatus.StatusInfo{}, errors.Capture(err)
	}

	derivedApplicationStatus, err := applicationDisplayStatusFromUnits(fullUnitStatuses)
	if err != nil {
		return corestatus.StatusInfo{}, errors.Capture(err)
	}
	return derivedApplicationStatus, nil
}

func (s *Service) decodeLXDProfile(lxdProfile []byte) (*internalcharm.LXDProfile, error) {
	if len(lxdProfile) == 0 {
		return nil, nil
	}

	var profile *internalcharm.LXDProfile
	if err := json.Unmarshal(lxdProfile, &profile); err != nil {
		return nil, errors.Errorf("decoding LXD profile: %w", err)
	}
	return profile, nil
}

func (s *Service) decodeUnitsStatusDetails(unitStatuses map[coreunit.Name]status.Unit) (map[coreunit.Name]Unit, error) {
	results := make(map[coreunit.Name]Unit, len(unitStatuses))
	for unitName, unitStatus := range unitStatuses {
		unit, err := s.decodeUnitStatusDetails(unitStatus)
		if err != nil {
			return nil, errors.Errorf("decoding unit status for %q: %w", unitName, err)
		}
		results[unitName] = unit
	}
	return results, nil
}

func (s *Service) decodeUnitStatusDetails(unit status.Unit) (Unit, error) {
	life, err := unit.Life.Value()
	if err != nil {
		return Unit{}, errors.Errorf("decoding unit life: %w", err)
	}

	agentStatus, workloadStatus, err := decodeUnitDisplayAndAgentStatus(status.FullUnitStatus{
		AgentStatus:    unit.AgentStatus,
		WorkloadStatus: unit.WorkloadStatus,
		K8sPodStatus:   unit.K8sPodStatus,
		Present:        unit.Present,
	})
	if err != nil {
		return Unit{}, errors.Errorf("decoding unit status: %w", err)
	}

	k8sPodStatus, err := decodeK8sPodStatus(unit.K8sPodStatus)
	if err != nil {
		return Unit{}, errors.Errorf("decoding k8s pod status: %w", err)
	}

	var subordinateNames []coreunit.Name
	for name := range unit.SubordinateNames {
		subordinateNames = append(subordinateNames, name)
	}

	return Unit{
		Life:             life,
		ApplicationName:  unit.ApplicationName,
		MachineName:      unit.MachineName,
		AgentStatus:      agentStatus,
		WorkloadStatus:   workloadStatus,
		K8sPodStatus:     k8sPodStatus,
		Present:          unit.Present,
		Subordinate:      unit.Subordinate,
		PrincipalName:    unit.PrincipalName,
		SubordinateNames: subordinateNames,
		CharmLocator:     unit.CharmLocator,
		AgentVersion:     unit.AgentVersion,
		WorkloadVersion:  unit.WorkloadVersion,
		K8sProviderID:    unit.K8sProviderID,
	}, nil
}

func (s *Service) decodeMachineStatusDetails(machineName machine.Name, machine status.Machine) (Machine, error) {
	life, err := machine.Life.Value()
	if err != nil {
		return Machine{}, errors.Errorf("decoding machine life: %w", err)
	}

	machineStatus, err := decodeMachineStatus(machine.MachineStatus)
	if err != nil {
		return Machine{}, errors.Errorf("decoding machine status: %w", err)
	}

	instanceStatus, err := decodeInstanceStatus(machine.InstanceStatus)
	if err != nil {
		return Machine{}, errors.Errorf("decoding instance status: %w", err)
	}

	return Machine{
		Name:                    machineName,
		Life:                    life,
		Hostname:                machine.Hostname,
		DisplayName:             machine.DisplayName,
		DNSName:                 machine.DNSName,
		IPAddresses:             machine.IPAddresses,
		InstanceID:              machine.InstanceID,
		MachineStatus:           machineStatus,
		InstanceStatus:          instanceStatus,
		Platform:                machine.Platform,
		Constraints:             constraints.EncodeConstraints(machine.Constraints),
		HardwareCharacteristics: machine.HardwareCharacteristics,
		LXDProfiles:             machine.LXDProfiles,
	}, nil
}

// statusFromModelContext is responsible for converting a
// [status.ModelStatusContext] into a [corestatus.StatusInfo].
// It computes the model's status based on various environmental indicators at a given point in time.
// Model status is not a fixed or stored value but is dynamically derived from these indicators.
func (s *Service) statusFromModelContext(
	modelStatusCtx status.ModelStatusContext,
) corestatus.StatusInfo {
	now := s.clock.Now()
	if modelStatusCtx.HasInvalidCloudCredential {
		return corestatus.StatusInfo{
			Status:  corestatus.Suspended,
			Message: "suspended since cloud credential is not valid",
			Data:    map[string]interface{}{"reason": modelStatusCtx.InvalidCloudCredentialReason},
			Since:   &now,
		}
	}
	if modelStatusCtx.IsDestroying {
		return corestatus.StatusInfo{
			Status: corestatus.Destroying,
			Since:  &now,
		}
	}
	if modelStatusCtx.IsMigrating {
		return corestatus.StatusInfo{
			Status:  corestatus.Busy,
			Message: "the model is being migrated",
			Since:   &now,
		}
	}

	return corestatus.StatusInfo{
		Status: corestatus.Available,
		Since:  &now,
	}
}
