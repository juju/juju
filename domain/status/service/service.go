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
	coremodel "github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/status"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

// State describes retrieval and persistence methods for the statuses of
// applications and units.
type State interface {
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

	// SetApplicationStatus saves the given application status, overwriting any
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

	// ImportRelationStatus sets the given relation status. It can return the
	// following errors:
	//   - [statuserrors.RelationNotFound] if the relation doesn't exist.
	ImportRelationStatus(
		ctx context.Context,
		relationID int,
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

	// GetAllFullUnitStatusesForApplication returns the workload statuses and
	// the cloud container statuses for all units of the specified application,
	// returning:
	//   - an error satisfying [statuserrors.ApplicationNotFound] if the application
	//     doesn't exist or;
	//   - an error satisfying [statuserrors.ApplicationIsDead] if the application
	//     is dead.
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

	// GetModelStatusInfo returns information about the current model.
	// The following error types can be expected to be returned:
	// - [github.com/juju/juju/domain/model/errors.NotFound]: When the model
	// does not exist.
	GetModelStatusInfo(ctx context.Context) (status.ModelStatusInfo, error)
}

// ControllerState is the controller state required by this service. This is the
// controller database, not the model state.
type ControllerState interface {
	// GetModelState returns the model state for the given model.
	// It returns [modelerrors.NotFound] if the model does not exist for the given UUID.
	GetModelState(context.Context, coremodel.UUID) (status.ModelState, error)

	// GetModel returns the model for the given UUID.
	// It returns [modelerrors.NotFound] if the model does not exist for the given UUID.
	GetModel(context.Context, coremodel.UUID) (coremodel.Model, error)
}

// Service provides the API for working with the statuses of applications and
// units.
type Service struct {
	st                    State
	controllerState       ControllerState
	modelUUID             coremodel.UUID
	statusHistory         StatusHistory
	statusHistoryReaderFn StatusHistoryReaderFunc
	logger                logger.Logger
	clock                 clock.Clock
}

// NewService returns a new service reference wrapping the input state.
func NewService(
	st State,
	controllerState ControllerState,
	modelUUID coremodel.UUID,
	statusHistory StatusHistory,
	statusHistoryReaderFn StatusHistoryReaderFunc,
	clock clock.Clock,
	logger logger.Logger,
) *Service {
	return &Service{
		st:                    st,
		controllerState:       controllerState,
		modelUUID:             modelUUID,
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

	statuses, err := s.st.GetAllRelationStatuses(ctx)
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

// SetApplicationStatus saves the given application status, overwriting any
// current status data. If returns an error satisfying
// [statuserrors.ApplicationNotFound] if the application doesn't exist.
func (s *Service) SetApplicationStatus(
	ctx context.Context,
	applicationName string,
	statusInfo corestatus.StatusInfo,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// This will implicitly verify that the status is valid.
	encodedStatus, err := encodeWorkloadStatus(statusInfo)
	if err != nil {
		return errors.Errorf("encoding workload status: %w", err)
	}

	applicationID, err := s.st.GetApplicationIDByName(ctx, applicationName)
	if err != nil {
		return errors.Capture(err)
	}

	if err := s.st.SetApplicationStatus(ctx, applicationID, encodedStatus); err != nil {
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

	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return corestatus.StatusInfo{}, errors.Capture(err)
	}
	applicationStatus, err := s.st.GetApplicationStatus(ctx, appID)
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
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return errors.Capture(err)
	}
	if err := s.st.SetUnitWorkloadStatus(ctx, unitUUID, workloadStatus); err != nil {
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
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return corestatus.StatusInfo{}, errors.Capture(err)
	}
	workloadStatus, err := s.st.GetUnitWorkloadStatus(ctx, unitUUID)
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
	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return errors.Capture(err)
	}
	if err := s.st.SetUnitAgentStatus(ctx, unitUUID, agentStatus); err != nil {
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

	statuses, err := s.st.GetUnitWorkloadStatusesForApplication(ctx, appID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	decoded, err := decodeUnitWorkloadStatuses(statuses)
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

	unitUUID, err := s.st.GetUnitUUIDByName(ctx, unitName)
	if err != nil {
		return agent, workload, errors.Capture(err)
	}

	agentStatus, err := s.st.GetUnitAgentStatus(ctx, unitUUID)
	if err != nil {
		return agent, workload, errors.Capture(err)
	}

	workloadStatus, err := s.st.GetUnitWorkloadStatus(ctx, unitUUID)
	if err != nil {
		return agent, workload, errors.Capture(err)
	}

	k8sPodStatus, err := s.st.GetUnitK8sPodStatus(ctx, unitUUID)
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
	return s.st.SetUnitPresence(ctx, unitName)
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
	return s.st.DeleteUnitPresence(ctx, unitName)
}

// CheckUnitStatusesReadyForMigration returns an error if the statuses of any units
// in the model indicate they cannot be migrated.
func (s *Service) CheckUnitStatusesReadyForMigration(ctx context.Context) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	fullStatuses, err := s.st.GetAllUnitWorkloadAgentStatuses(ctx)
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

	statuses, err := s.st.GetApplicationAndUnitStatuses(ctx)
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

	return s.st.GetApplicationAndUnitModelStatuses(ctx)
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

	return s.st.ImportRelationStatus(ctx, relationID, relationStatus)
}

// ExportUnitStatuses returns the workload and agent statuses of all the units in
// in the model, indexed by unit name.
//
// TODO(jack-w-shaw): Export the container statuses too.
func (s *Service) ExportUnitStatuses(ctx context.Context) (map[coreunit.Name]corestatus.StatusInfo, map[coreunit.Name]corestatus.StatusInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	fullStatuses, err := s.st.GetAllUnitWorkloadAgentStatuses(ctx)
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

	appStatuses, err := s.st.GetAllApplicationStatuses(ctx)
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

	relStatuses, err := s.st.GetAllRelationStatuses(ctx)
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

func (s *Service) decodeApplicationDisplayStatus(ctx context.Context, appID coreapplication.ID, statusInfo status.StatusInfo[status.WorkloadStatusType]) (corestatus.StatusInfo, error) {
	if statusInfo.Status != status.WorkloadStatusUnset {
		return decodeApplicationStatus(statusInfo)
	}

	fullUnitStatuses, err := s.st.GetAllFullUnitStatusesForApplication(ctx, appID)
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

// GetModelStatusInfo returns information about the current model for the
// purpose of reporting its status.
// The following error types can be expected to be returned:
// - [github.com/juju/juju/domain/model/errors.NotFound]: When the model does
// not exist.
func (s *Service) GetModelStatusInfo(ctx context.Context) (status.ModelStatusInfo, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.st.GetModelStatusInfo(ctx)
}
