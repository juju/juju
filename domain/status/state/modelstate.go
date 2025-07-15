// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	corerelation "github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/status"
	statuserrors "github.com/juju/juju/domain/status/errors"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// ModelState represents the persistence layer for the statuses of applications and units.
type ModelState struct {
	*domain.StateBase
	clock  clock.Clock
	logger logger.Logger
}

// NewModelState returns a new state reference.
func NewModelState(factory database.TxnRunnerFactory, clock clock.Clock, logger logger.Logger) *ModelState {
	return &ModelState{
		StateBase: domain.NewStateBase(factory),
		clock:     clock,
		logger:    logger,
	}
}

// GetModelStatusInfo returns information about the current model.
// The following error types can be expected to be returned:
// - [modelerrors.NotFound]: When the model does not exist.
func (st *ModelState) GetModelStatusInfo(ctx context.Context) (status.ModelStatusInfo, error) {
	db, err := st.DB()
	if err != nil {
		return status.ModelStatusInfo{}, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &modelInfo.*
FROM   model
`, modelInfo{})
	if err != nil {
		return status.ModelStatusInfo{}, errors.Capture(err)
	}

	var m modelInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&m)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New("model does not exist").Add(modelerrors.NotFound)
		} else if err != nil {
			return errors.Errorf(
				"getting model status information from database: %w", err,
			)
		}
		return err
	})
	if err != nil {
		return status.ModelStatusInfo{}, errors.Capture(err)
	}

	return status.ModelStatusInfo{Type: coremodel.ModelType(m.Type)}, nil

}

// GetAllRelationStatuses returns all the relation statuses of the given model.
func (st *ModelState) GetAllRelationStatuses(ctx context.Context) ([]status.RelationStatusInfo, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &relationStatusAndID.*
FROM   relation_status rs
JOIN   relation r ON r.uuid = rs.relation_uuid
`, relationStatusAndID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var statuses []relationStatusAndID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt).GetAll(&statuses)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Errorf("getting all relations statuses: %w", err)
	}

	relationStatuses := make([]status.RelationStatusInfo, len(statuses))
	for i, relStatus := range statuses {
		statusType, err := status.DecodeRelationStatus(relStatus.StatusID)
		if err != nil {
			return nil, errors.Capture(err)
		}
		relationStatuses[i] = status.RelationStatusInfo{
			RelationUUID: relStatus.RelationUUID,
			RelationID:   relStatus.RelationID,
			StatusInfo: status.StatusInfo[status.RelationStatusType]{
				Status:  statusType,
				Message: relStatus.Reason,
				Since:   relStatus.Since,
			},
		}
	}

	return relationStatuses, errors.Capture(err)
}

// GetApplicationIDByName returns the application ID for the named application.
// If no application is found, an error satisfying
// [statuserrors.ApplicationNotFound] is returned.
func (st *ModelState) GetApplicationIDByName(ctx context.Context, name string) (coreapplication.ID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	var id coreapplication.ID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		id, err = st.lookupApplication(ctx, tx, name)
		return err
	}); err != nil {
		return "", errors.Capture(err)
	}
	return id, nil
}

// GetApplicationIDAndNameByUnitName returns the application ID and name for the
// named unit.
//
// Returns an error satisfying [statuserrors.UnitNotFound] if the unit
// doesn't exist.
func (st *ModelState) GetApplicationIDAndNameByUnitName(
	ctx context.Context,
	name coreunit.Name,
) (coreapplication.ID, string, error) {
	db, err := st.DB()
	if err != nil {
		return "", "", errors.Capture(err)
	}

	unit := unitName{Name: name}
	queryUnit := `
SELECT a.uuid AS &applicationIDAndName.uuid,
       a.name AS &applicationIDAndName.name
FROM unit u
JOIN application a ON a.uuid = u.application_uuid
WHERE u.name = $unitName.name;
`
	query, err := st.Prepare(queryUnit, applicationIDAndName{}, unit)
	if err != nil {
		return "", "", errors.Errorf("preparing query for unit %q: %w", name, err)
	}

	var app applicationIDAndName
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, unit).Get(&app)
		if errors.Is(err, sqlair.ErrNoRows) {
			return statuserrors.UnitNotFound
		}
		return err
	})
	if err != nil {
		return "", "", errors.Errorf("querying unit %q application id: %w", name, err)
	}
	return app.ID, app.Name, nil
}

// GetApplicationStatus looks up the status of the specified application,
// returning an error satisfying [statuserrors.ApplicationNotFound] if the
// application is not found.
func (st *ModelState) GetApplicationStatus(ctx context.Context, appID coreapplication.ID) (status.StatusInfo[status.WorkloadStatusType], error) {
	db, err := st.DB()
	if err != nil {
		return status.StatusInfo[status.WorkloadStatusType]{}, errors.Capture(err)
	}

	identID := applicationID{ID: appID}
	query, err := st.Prepare(`
SELECT &statusInfo.*
FROM application_status
WHERE application_uuid = $applicationID.uuid;
`, identID, statusInfo{})
	if err != nil {
		return status.StatusInfo[status.WorkloadStatusType]{}, errors.Capture(err)
	}
	var sts statusInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := st.checkApplicationNotDead(ctx, tx, identID); err != nil {
			return errors.Capture(err)
		}
		if err := tx.Query(ctx, query, identID).Get(&sts); errors.Is(err, sqlair.ErrNoRows) {
			// If the application status is not set, we should return a default
			// unset status. It's then it's up to the caller to either return that
			// information or use derive the status from the units.
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return status.StatusInfo[status.WorkloadStatusType]{}, errors.Capture(err)
	}

	statusType, err := status.DecodeWorkloadStatus(sts.StatusID)
	if err != nil {
		return status.StatusInfo[status.WorkloadStatusType]{}, errors.Capture(err)
	}

	return status.StatusInfo[status.WorkloadStatusType]{
		Status:  statusType,
		Message: sts.Message,
		Data:    sts.Data,
		Since:   sts.UpdatedAt,
	}, nil
}

// SetApplicationStatus saves the given application status, overwriting any
// current status data. If returns an error satisfying
// [statuserrors.ApplicationNotFound] if the application doesn't exist.
func (st *ModelState) SetApplicationStatus(
	ctx context.Context,
	applicationID coreapplication.ID,
	sts status.StatusInfo[status.WorkloadStatusType],
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	statusID, err := status.EncodeWorkloadStatus(sts.Status)
	if err != nil {
		return errors.Capture(err)
	}

	statusInfo := applicationStatusInfo{
		ApplicationID: applicationID,
		StatusID:      statusID,
		Message:       sts.Message,
		Data:          sts.Data,
		UpdatedAt:     sts.Since,
	}
	stmt, err := st.Prepare(`
INSERT INTO application_status (*) VALUES ($applicationStatusInfo.*)
ON CONFLICT(application_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at,
    data = excluded.data;
`, statusInfo)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, statusInfo).Run(); internaldatabase.IsErrConstraintForeignKey(err) {
			return errors.Errorf("%w: %q", statuserrors.ApplicationNotFound, applicationID)
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return errors.Errorf("updating application status for %q: %w", applicationID, err)
	}
	return nil
}

// GetRelationStatus gets the status of the given relation. It returns an error
// satisfying [statuserrors.RelationNotFound] if the relation doesn't exist.
func (st *ModelState) getRelationStatus(
	ctx context.Context,
	tx *sqlair.TX,
	uuid corerelation.UUID,
) (status.StatusInfo[status.RelationStatusType], error) {
	empty := status.StatusInfo[status.RelationStatusType]{}
	id := relationUUID{
		RelationUUID: uuid,
	}
	var sts relationStatus

	stmt, err := st.Prepare(`
SELECT &relationStatus.*
FROM   relation_status
WHERE  relation_uuid = $relationUUID.relation_uuid
`, id, sts)
	if err != nil {
		return empty, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, id).Get(&sts)
	if errors.Is(err, sqlair.ErrNoRows) {
		return empty, statuserrors.RelationNotFound
	} else if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return empty, errors.Capture(err)
	}

	statusType, err := status.DecodeRelationStatus(sts.StatusID)
	if err != nil {
		return empty, errors.Capture(err)
	}
	return status.StatusInfo[status.RelationStatusType]{
		Status:  statusType,
		Message: sts.Reason,
		Since:   sts.Since,
	}, nil
}

// SetRelationStatus sets the given relation status and checks that the
// transition to the new status from the current status is valid. It can
// return the following errors:
//   - [statuserrors.RelationNotFound] if the relation doesn't exist.
//   - [statuserrors.RelationStatusTransitionNotValid] if the current relation
//     status cannot transition to the new relation status. the relation does
//     not exist.
func (st *ModelState) SetRelationStatus(
	ctx context.Context,
	relationUUID corerelation.UUID,
	sts status.StatusInfo[status.RelationStatusType],
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Get current status.
		currentStatus, err := st.getRelationStatus(ctx, tx, relationUUID)
		if err != nil {
			return errors.Errorf("getting current relation status: %w", err)
		}

		// Check we can transition from current status to the new status.
		err = status.RelationStatusTransitionValid(currentStatus, sts)
		if err != nil {
			return errors.Capture(err)
		}

		// If transitioning from Suspending to Suspended and the new message is
		// empty, retain any existing message so that any previous reason for
		// suspending is retained.
		if sts.Message == "" &&
			currentStatus.Status == status.RelationStatusTypeSuspending &&
			sts.Status == status.RelationStatusTypeSuspended {
			sts.Message = currentStatus.Message
		}
		return st.updateRelationStatus(ctx, tx, relationUUID, sts)
	})
	if err != nil {
		return errors.Errorf("updating relation status for %q: %w", relationUUID, err)
	}
	return nil
}

// ImportRelationStatus sets the given relation status. It can return the
// following errors:
//   - [statuserrors.RelationNotFound] if the relation doesn't exist.
func (st *ModelState) ImportRelationStatus(
	ctx context.Context,
	relationUUID corerelation.UUID,
	sts status.StatusInfo[status.RelationStatusType],
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.updateRelationStatus(ctx, tx, relationUUID, sts)
	})
}

// GetRelationUUIDByID returns the UUID for the given relation ID.
// It can return the following errors:
//   - [statuserrors.RelationNotFound] if the relation doesn't exist.
func (st *ModelState) GetRelationUUIDByID(
	ctx context.Context,
	id int,
) (corerelation.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	type relationID struct {
		ID   int               `db:"relation_id"`
		UUID corerelation.UUID `db:"uuid"`
	}
	arg := relationID{
		ID: id,
	}

	stmt, err := st.Prepare(`
SELECT &relationID.uuid
FROM   relation
WHERE  relation_id = $relationID.relation_id
`, arg)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, arg).Get(&arg)
		if errors.Is(err, sqlair.ErrNoRows) {
			return statuserrors.RelationNotFound
		} else if err != nil {
			return errors.Errorf("getting relation UUID for %q: %w", id, err)
		}
		return nil
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	return arg.UUID, nil
}

func (st *ModelState) updateRelationStatus(
	ctx context.Context,
	tx *sqlair.TX,
	relationUUID corerelation.UUID,
	sts status.StatusInfo[status.RelationStatusType],
) error {
	statusID, err := status.EncodeRelationStatus(sts.Status)
	if err != nil {
		return errors.Capture(err)
	}

	statusInfo := relationStatus{
		RelationUUID: relationUUID,
		StatusID:     statusID,
		Reason:       sts.Message,
		Since:        sts.Since,
	}
	stmt, err := st.Prepare(`
INSERT INTO relation_status (*) VALUES ($relationStatus.*)
ON CONFLICT(relation_uuid) DO UPDATE SET
    relation_status_type_id = excluded.relation_status_type_id,
    suspended_reason = excluded.suspended_reason,
    updated_at = excluded.updated_at
WHERE relation_uuid = $relationStatus.relation_uuid
`, statusInfo)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, statusInfo).Run()
	if internaldatabase.IsErrConstraintForeignKey(err) {
		return statuserrors.RelationNotFound
	}
	return errors.Capture(err)
}

// GetUnitUUIDByName returns the UUID for the named unit, returning an error
// satisfying [statuserrors.UnitNotFound] if the unit doesn't exist.
func (st *ModelState) GetUnitUUIDByName(ctx context.Context, name coreunit.Name) (coreunit.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}
	unitName := unitName{Name: name}

	query, err := st.Prepare(`
SELECT &unitUUID.*
FROM unit
WHERE name = $unitName.name
`, unitUUID{}, unitName)
	if err != nil {
		return "", errors.Errorf("preparing query: %w", err)
	}

	unitUUID := unitUUID{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, query, unitName).Get(&unitUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("unit %q not found", name).Add(statuserrors.UnitNotFound)
		}
		return err
	})
	if err != nil {
		return "", errors.Errorf("querying unit name: %w", err)
	}

	return unitUUID.UnitUUID, nil
}

// GetUnitAgentStatus returns the agent status of the specified unit, returning:
// - an error satisfying [statuserrors.UnitNotFound] if the unit doesn't exist or;
// - an error satisfying [statuserrors.UnitIsDead] if the unit is dead or;
// - an error satisfying [statuserrors.UnitStatusNotFound] if the status is not set.
func (st *ModelState) GetUnitAgentStatus(ctx context.Context, uuid coreunit.UUID) (status.UnitStatusInfo[status.UnitAgentStatusType], error) {
	db, err := st.DB()
	if err != nil {
		return status.UnitStatusInfo[status.UnitAgentStatusType]{}, errors.Capture(err)
	}

	unitUUID := unitUUID{UnitUUID: uuid}
	getUnitStatusStmt, err := st.Prepare(`
SELECT &unitPresentStatusInfo.* FROM v_unit_agent_status WHERE unit_uuid = $unitUUID.uuid
`, unitPresentStatusInfo{}, unitUUID)
	if err != nil {
		return status.UnitStatusInfo[status.UnitAgentStatusType]{}, errors.Capture(err)
	}

	var unitStatusInfo unitPresentStatusInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.checkUnitNotDead(ctx, tx, unitUUID)
		if err != nil {
			return errors.Errorf("checking unit %q exists: %w", uuid, err)
		}

		err = tx.Query(ctx, getUnitStatusStmt, unitUUID).Get(&unitStatusInfo)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("agent status for unit %q not found", unitUUID).Add(statuserrors.UnitStatusNotFound)
		}
		return err
	})
	if err != nil {
		return status.UnitStatusInfo[status.UnitAgentStatusType]{}, errors.Errorf("getting agent status for unit %q: %w", unitUUID, err)
	}

	statusID, err := status.DecodeAgentStatus(unitStatusInfo.StatusID)
	if err != nil {
		return status.UnitStatusInfo[status.UnitAgentStatusType]{}, errors.Errorf("decoding agent status ID for unit %q: %w", unitUUID, err)
	}

	return status.UnitStatusInfo[status.UnitAgentStatusType]{
		StatusInfo: status.StatusInfo[status.UnitAgentStatusType]{
			Status:  statusID,
			Message: unitStatusInfo.Message,
			Data:    unitStatusInfo.Data,
			Since:   unitStatusInfo.UpdatedAt,
		},
		Present: unitStatusInfo.Present,
	}, nil
}

// SetUnitAgentStatus updates the agent status of the specified unit,
// returning an error satisfying [statuserrors.UnitNotFound] if the unit
// doesn't exist.
func (st *ModelState) SetUnitAgentStatus(ctx context.Context, unitUUID coreunit.UUID, status status.StatusInfo[status.UnitAgentStatusType]) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.setUnitAgentStatus(ctx, tx, unitUUID, status)
	})
	if err != nil {
		return errors.Errorf("setting agent status for unit %q: %w", unitUUID, err)
	}
	return nil
}

// GetUnitWorkloadStatus returns the workload status of the specified unit, returning:
// - an error satisfying [statuserrors.UnitNotFound] if the unit doesn't exist or;
// - an error satisfying [statuserrors.UnitIsDead] if the unit is dead or;
// - an error satisfying [statuserrors.UnitStatusNotFound] if the status is not set.
func (st *ModelState) GetUnitWorkloadStatus(ctx context.Context, uuid coreunit.UUID) (status.UnitStatusInfo[status.WorkloadStatusType], error) {
	db, err := st.DB()
	if err != nil {
		return status.UnitStatusInfo[status.WorkloadStatusType]{}, errors.Capture(err)
	}

	unitUUID := unitUUID{UnitUUID: uuid}
	getUnitStatusStmt, err := st.Prepare(`
SELECT &unitPresentStatusInfo.* FROM v_unit_workload_status WHERE unit_uuid = $unitUUID.uuid
`, unitPresentStatusInfo{}, unitUUID)
	if err != nil {
		return status.UnitStatusInfo[status.WorkloadStatusType]{}, errors.Capture(err)
	}

	var unitStatusInfo unitPresentStatusInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.checkUnitNotDead(ctx, tx, unitUUID)
		if err != nil {
			return errors.Errorf("checking unit %q exists: %w", uuid, err)
		}

		err = tx.Query(ctx, getUnitStatusStmt, unitUUID).Get(&unitStatusInfo)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("workload status for unit %q not found", unitUUID).Add(statuserrors.UnitStatusNotFound)
		}
		return err
	})
	if err != nil {
		return status.UnitStatusInfo[status.WorkloadStatusType]{}, errors.Errorf("getting workload status for unit %q: %w", unitUUID, err)
	}

	statusID, err := status.DecodeWorkloadStatus(unitStatusInfo.StatusID)
	if err != nil {
		return status.UnitStatusInfo[status.WorkloadStatusType]{}, errors.Errorf("decoding workload status ID for unit %q: %w", unitUUID, err)
	}

	return status.UnitStatusInfo[status.WorkloadStatusType]{
		StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
			Status:  statusID,
			Message: unitStatusInfo.Message,
			Data:    unitStatusInfo.Data,
			Since:   unitStatusInfo.UpdatedAt,
		},
		Present: unitStatusInfo.Present,
	}, nil
}

// SetUnitWorkloadStatus updates the workload status of the specified unit,
// returning an error satisfying [statuserrors.UnitNotFound] if the unit
// doesn't exist.
func (st *ModelState) SetUnitWorkloadStatus(ctx context.Context, unitUUID coreunit.UUID, status status.StatusInfo[status.WorkloadStatusType]) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.setUnitWorkloadStatus(ctx, tx, unitUUID, status)
	})
	if err != nil {
		return errors.Errorf("setting workload status for unit %q: %w", unitUUID, err)
	}
	return nil
}

// GetUnitK8sPodStatus returns the cloud container status of the specified
// unit. It returns;
// - an error satisfying [statuserrors.UnitNotFound] if the unit doesn't exist or;
// - an error satisfying [statuserrors.UnitIsDead] if the unit is dead or;
func (st *ModelState) GetUnitK8sPodStatus(ctx context.Context, uuid coreunit.UUID) (status.StatusInfo[status.K8sPodStatusType], error) {
	db, err := st.DB()
	if err != nil {
		return status.StatusInfo[status.K8sPodStatusType]{}, errors.Capture(err)
	}

	unitUUID := unitUUID{UnitUUID: uuid}
	getUnitStatusStmt, err := st.Prepare(`
SELECT &statusInfo.*
FROM   k8s_pod_status
WHERE  unit_uuid = $unitUUID.uuid
	`, statusInfo{}, unitUUID)
	if err != nil {
		return status.StatusInfo[status.K8sPodStatusType]{}, errors.Capture(err)
	}

	var containerStatusInfo statusInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.checkUnitNotDead(ctx, tx, unitUUID)
		if err != nil {
			return errors.Errorf("checking unit %q exists: %w", uuid, err)
		}

		if err := tx.Query(ctx, getUnitStatusStmt, unitUUID).Get(&containerStatusInfo); errors.Is(err, sql.ErrNoRows) {
			// If the unit has not container status, this is fine. Container status
			// is optional since non-CAAS models do no have containers. In this
			// case, return a default unset status
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return status.StatusInfo[status.K8sPodStatusType]{}, errors.Errorf("getting cloud container status for unit %q: %w", unitUUID, err)
	}

	statusID, err := status.DecodeK8sPodStatus(containerStatusInfo.StatusID)
	if err != nil {
		return status.StatusInfo[status.K8sPodStatusType]{}, errors.Errorf("decoding cloud container status ID for unit %q: %w", uuid, err)
	}
	return status.StatusInfo[status.K8sPodStatusType]{
		Status:  statusID,
		Message: containerStatusInfo.Message,
		Data:    containerStatusInfo.Data,
		Since:   containerStatusInfo.UpdatedAt,
	}, nil
}

// GetUnitWorkloadStatusesForApplication returns the workload statuses for all units
// of the specified application, returning:
//   - an error satisfying [statuserrors.ApplicationNotFound] if the application
//     doesn't exist or;
//   - error satisfying [statuserrors.ApplicationIsDead] if the application
//     is dead.
func (st *ModelState) GetUnitWorkloadStatusesForApplication(
	ctx context.Context, appID coreapplication.ID,
) (status.UnitWorkloadStatuses, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}
	ident := applicationID{ID: appID}

	var unitStatuses status.UnitWorkloadStatuses
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		unitStatuses, err = st.getUnitWorkloadStatusesForApplication(ctx, tx, ident)
		return err
	})
	if err != nil {
		return nil, errors.Errorf("getting workload statuses for application %q: %w", appID, err)
	}
	return unitStatuses, nil
}

// GetUnitAgentStatusesForApplication returns the agent statuses for all units
// of the specified application, returning:
//   - an error satisfying [statuserrors.ApplicationNotFound] if the application
//     doesn't exist or;
//   - error satisfying [statuserrors.ApplicationIsDead] if the application
//     is dead.
func (st *ModelState) GetUnitAgentStatusesForApplication(
	ctx context.Context, appID coreapplication.ID,
) (status.UnitAgentStatuses, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}
	ident := applicationID{ID: appID}

	var unitAgentStatuses status.UnitAgentStatuses
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		unitAgentStatuses, err = st.getUnitAgentStatusesForApplication(ctx, tx, ident)
		return err
	})
	if err != nil {
		return nil, errors.Errorf("getting workload statuses for application %q: %w", appID, err)
	}
	return unitAgentStatuses, nil
}

// GetAllFullUnitStatusesForApplication returns the workload, agent and
// container statuses for all units of the specified application, returning:
//   - an error satisfying [statuserrors.ApplicationNotFound] if the application
//     doesn't exist or;
//   - an error satisfying [statuserrors.ApplicationIsDead] if the application
//     is dead.
func (st *ModelState) GetAllFullUnitStatusesForApplication(
	ctx context.Context, appID coreapplication.ID,
) (
	status.FullUnitStatuses, error,
) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}
	ident := applicationID{ID: appID}

	stmt, err := st.Prepare(`
SELECT &fullUnitStatus.*
FROM v_full_unit_status
WHERE application_uuid = $applicationID.uuid
`, fullUnitStatus{}, ident)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var fullUnitStatuses []fullUnitStatus
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.checkApplicationNotDead(ctx, tx, ident)
		if err != nil {
			return errors.Errorf("checking application not dead: %w", err)
		}
		err = tx.Query(ctx, stmt, ident).GetAll(&fullUnitStatuses)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Errorf("getting unit statuses for application %q: %w", appID, err)
	}
	ret := make(status.FullUnitStatuses, len(fullUnitStatuses))
	for _, s := range fullUnitStatuses {
		if s.WorkloadStatusID == nil {
			return nil, errors.Errorf("workload status for unit %q not found", s.UnitName).Add(statuserrors.UnitStatusNotFound)
		}
		if s.AgentStatusID == nil {
			return nil, errors.Errorf("agent status for unit %q not found", s.UnitName).Add(statuserrors.UnitStatusNotFound)
		}
		workloadStatusID, err := status.DecodeWorkloadStatus(*s.WorkloadStatusID)
		if err != nil {
			return nil, errors.Errorf("decoding workload status ID for unit %q: %w", s.UnitName, err)
		}
		agentStatusID, err := status.DecodeAgentStatus(*s.AgentStatusID)
		if err != nil {
			return nil, errors.Errorf("decoding agent status ID for unit %q: %w", s.UnitName, err)
		}

		// Container status is optional.
		k8sPodStatus := status.StatusInfo[status.K8sPodStatusType]{
			Status: status.K8sPodStatusUnset,
		}
		if s.K8sPodStatusID != nil {
			k8sPodStatusID, err := status.DecodeK8sPodStatus(*s.K8sPodStatusID)
			if err != nil {
				return nil, errors.Errorf("decoding K8ssPodStatus status ID for unit %q: %w", s.UnitName, err)
			}
			k8sPodStatus = status.StatusInfo[status.K8sPodStatusType]{
				Status:  k8sPodStatusID,
				Message: s.K8sPodMessage,
				Data:    s.K8sPodData,
				Since:   s.K8sPodUpdatedAt,
			}
		}

		ret[s.UnitName] = status.FullUnitStatus{
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  workloadStatusID,
				Message: s.WorkloadMessage,
				Data:    s.WorkloadData,
				Since:   s.WorkloadUpdatedAt,
			},
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  agentStatusID,
				Message: s.AgentMessage,
				Data:    s.AgentData,
				Since:   s.AgentUpdatedAt,
			},
			K8sPodStatus: k8sPodStatus,
			Present:      s.Present,
		}
	}
	return ret, nil
}

// GetAllUnitWorkloadAgentStatuses retrieves the presence, workload status, and agent status
// of every unit in the model. Returns an error satisfying [statuserrors.UnitStatusNotFound]
// if any units do not have statuses.
func (st *ModelState) GetAllUnitWorkloadAgentStatuses(ctx context.Context) (status.UnitWorkloadAgentStatuses, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	query, err := st.Prepare(`SELECT &workloadAgentStatus.* FROM v_unit_workload_agent_status`, workloadAgentStatus{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var statuses []workloadAgentStatus
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query).GetAll(&statuses)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	ret := make(status.UnitWorkloadAgentStatuses, len(statuses))
	for _, s := range statuses {
		if s.WorkloadStatusID == nil {
			return nil, errors.Errorf("workload status for unit %q not found", s.UnitName).Add(statuserrors.UnitStatusNotFound)
		}
		if s.AgentStatusID == nil {
			return nil, errors.Errorf("agent status for unit %q not found", s.UnitName).Add(statuserrors.UnitStatusNotFound)
		}
		workloadStatusID, err := status.DecodeWorkloadStatus(*s.WorkloadStatusID)
		if err != nil {
			return nil, errors.Errorf("decoding workload status for unit %q: %w", s.UnitName, err)
		}
		agentStatusID, err := status.DecodeAgentStatus(*s.AgentStatusID)
		if err != nil {
			return nil, errors.Errorf("decoding workload status for unit %q: %w", s.UnitName, err)
		}

		ret[s.UnitName] = status.UnitWorkloadAgentStatus{
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  workloadStatusID,
				Message: s.WorkloadMessage,
				Data:    s.WorkloadData,
				Since:   s.WorkloadUpdatedAt,
			},
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  agentStatusID,
				Message: s.AgentMessage,
				Data:    s.AgentData,
				Since:   s.AgentUpdatedAt,
			},
			Present: s.Present,
		}
	}
	return ret, nil
}

// GetAllApplicationStatuses returns the statuses of all the applications in the model,
// indexed by application name, if they have a status set.
func (st *ModelState) GetAllApplicationStatuses(ctx context.Context) (map[string]status.StatusInfo[status.WorkloadStatusType], error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	query, err := st.Prepare(`
SELECT &applicationNameStatusInfo.*
FROM application_status
JOIN application ON application.uuid = application_status.application_uuid
`, applicationNameStatusInfo{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var statuses []applicationNameStatusInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query).GetAll(&statuses)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	ret := make(map[string]status.StatusInfo[status.WorkloadStatusType], len(statuses))
	for _, s := range statuses {
		statusType, err := status.DecodeWorkloadStatus(s.StatusID)
		if err != nil {
			return nil, errors.Capture(err)
		}
		ret[s.ApplicationName] = status.StatusInfo[status.WorkloadStatusType]{
			Status:  statusType,
			Message: s.Message,
			Data:    s.Data,
			Since:   s.UpdatedAt,
		}
	}
	return ret, nil
}

// SetUnitPresence marks the presence of the specified unit, returning an error
// satisfying [statuserrors.UnitNotFound] if the unit doesn't exist.
// The unit life is not considered when making this query.
func (st *ModelState) SetUnitPresence(ctx context.Context, name coreunit.Name) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	unit := unitName{Name: name}
	var uuid unitUUID

	queryUnit := `SELECT &unitUUID.uuid FROM unit WHERE name = $unitName.name;`
	queryUnitStmt, err := st.Prepare(queryUnit, unit, uuid)
	if err != nil {
		return errors.Capture(err)
	}

	recordUnit := `
INSERT INTO unit_agent_presence (*) VALUES ($unitPresence.*)
ON CONFLICT(unit_uuid) DO UPDATE SET
	last_seen = excluded.last_seen;
`
	var presence unitPresence
	recordUnitStmt, err := st.Prepare(recordUnit, presence)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, queryUnitStmt, unit).Get(&uuid); errors.Is(err, sql.ErrNoRows) {
			return statuserrors.UnitNotFound
		} else if err != nil {
			return errors.Capture(err)
		}

		presence := unitPresence{
			UnitUUID: uuid.UnitUUID,
			LastSeen: st.clock.Now(),
		}

		if err := tx.Query(ctx, recordUnitStmt, presence).Run(); err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// DeleteUnitPresence removes the presence of the specified unit. If the
// unit isn't found it ignores the error.
// The unit life is not considered when making this query.
func (st *ModelState) DeleteUnitPresence(ctx context.Context, name coreunit.Name) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	unit := unitName{Name: name}

	deleteStmt, err := st.Prepare(`
DELETE FROM unit_agent_presence
WHERE unit_uuid = (
	SELECT uuid FROM unit
	WHERE name = $unitName.name
);
`, unit)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, deleteStmt, unit).Run(); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})

	return errors.Capture(err)
}

// lookupApplication looks up the application by name and returns the
// application.ID.
// If no application is found, an error satisfying
// [statuserrors.ApplicationNotFound] is returned.
func (st *ModelState) lookupApplication(ctx context.Context, tx *sqlair.TX, name string) (coreapplication.ID, error) {
	app := applicationIDAndName{Name: name}
	queryApplicationStmt, err := st.Prepare(`
SELECT uuid AS &applicationIDAndName.uuid
FROM application
WHERE name = $applicationIDAndName.name
`, app)
	if err != nil {
		return "", errors.Capture(err)
	}
	err = tx.Query(ctx, queryApplicationStmt, app).Get(&app)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("%w: %s", statuserrors.ApplicationNotFound, name)
	} else if err != nil {
		return "", errors.Errorf("looking up UUID for application %q: %w", name, err)
	}
	return app.ID, nil
}

// checkApplicationNotDead checks if the application exists and is not dead. It's
// possible to access alive and dying applications, but not dead ones.
//   - If the application is dead, [statuserrors.ApplicationIsDead] is returned.
//   - If the application is not found, [statuserrors.ApplicationNotFound]
//     is returned.
func (st *ModelState) checkApplicationNotDead(ctx context.Context, tx *sqlair.TX, ident applicationID) error {
	type life struct {
		LifeID domainlife.Life `db:"life_id"`
	}

	query := `
SELECT &life.*
FROM application
WHERE uuid = $applicationID.uuid;
`
	stmt, err := st.Prepare(query, ident, life{})
	if err != nil {
		return errors.Errorf("preparing query for application %q: %w", ident.ID, err)
	}

	var result life
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return statuserrors.ApplicationNotFound
	} else if err != nil {
		return errors.Errorf("checking application %q exists: %w", ident.ID, err)
	}

	switch result.LifeID {
	case domainlife.Dead:
		return statuserrors.ApplicationIsDead
	default:
		return nil
	}
}

// checkUnitNotDead checks if the unit exists and is not dead. It's possible to
// access alive and dying units, but not dead ones:
// - If the unit is not found, [statuserrors.UnitNotFound] is returned.
// - If the unit is dead, [statuserrors.UnitIsDead] is returned.
func (st *ModelState) checkUnitNotDead(ctx context.Context, tx *sqlair.TX, ident unitUUID) error {
	type life struct {
		LifeID domainlife.Life `db:"life_id"`
	}

	query := `
SELECT &life.*
FROM unit
WHERE uuid = $unitUUID.uuid;
`
	stmt, err := st.Prepare(query, ident, life{})
	if err != nil {
		return errors.Errorf("preparing query for unit %q: %w", ident.UnitUUID, err)
	}

	var result life
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return statuserrors.UnitNotFound
	} else if err != nil {
		return errors.Errorf("checking unit %q exists: %w", ident.UnitUUID, err)
	}

	switch result.LifeID {
	case domainlife.Dead:
		return statuserrors.UnitIsDead
	default:
		return nil
	}
}

func (st *ModelState) getUnitWorkloadStatusesForApplication(
	ctx context.Context, tx *sqlair.TX, ident applicationID,
) (
	status.UnitWorkloadStatuses, error,
) {
	getUnitStatusesStmt, err := st.Prepare(`
SELECT &statusInfoAndUnitNameAndPresence.*
FROM   v_unit_workload_status
WHERE  application_uuid = $applicationID.uuid
`, statusInfoAndUnitNameAndPresence{}, ident)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var unitStatuses []statusInfoAndUnitNameAndPresence
	err = st.checkApplicationNotDead(ctx, tx, ident)
	if err != nil {
		return nil, errors.Capture(err)
	}
	err = tx.Query(ctx, getUnitStatusesStmt, ident).GetAll(&unitStatuses)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Capture(err)
	}

	statuses := make(status.UnitWorkloadStatuses, len(unitStatuses))
	for _, unitStatus := range unitStatuses {
		statusID, err := status.DecodeWorkloadStatus(unitStatus.StatusID)
		if err != nil {
			return nil, errors.Errorf("decoding workload status ID for unit %q: %w", unitStatus.UnitName, err)
		}
		statuses[unitStatus.UnitName] = status.UnitStatusInfo[status.WorkloadStatusType]{
			StatusInfo: status.StatusInfo[status.WorkloadStatusType]{
				Status:  statusID,
				Message: unitStatus.Message,
				Data:    unitStatus.Data,
				Since:   unitStatus.UpdatedAt,
			},
			Present: unitStatus.Present,
		}
	}

	return statuses, nil
}

func (st *ModelState) getUnitAgentStatusesForApplication(
	ctx context.Context, tx *sqlair.TX, ident applicationID,
) (
	status.UnitAgentStatuses, error,
) {
	getUnitAgentStatusesStmt, err := st.Prepare(`
SELECT &statusInfoAndUnitName.*
FROM   v_unit_agent_status
WHERE  application_uuid = $applicationID.uuid
`, statusInfoAndUnitName{}, ident)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var unitAgentStatuses []statusInfoAndUnitName
	err = st.checkApplicationNotDead(ctx, tx, ident)
	if err != nil {
		return nil, errors.Capture(err)
	}
	err = tx.Query(ctx, getUnitAgentStatusesStmt, ident).GetAll(&unitAgentStatuses)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, errors.Capture(err)
	}

	statuses := make(status.UnitAgentStatuses, len(unitAgentStatuses))
	for _, agentStatus := range unitAgentStatuses {
		statusID, err := status.DecodeAgentStatus(agentStatus.StatusID)
		if err != nil {
			return nil, errors.Errorf("decoding agent status ID for unit %q: %w", agentStatus.UnitName, err)
		}
		statuses[agentStatus.UnitName] = status.StatusInfo[status.UnitAgentStatusType]{
			Status:  statusID,
			Message: agentStatus.Message,
			Data:    agentStatus.Data,
			Since:   agentStatus.UpdatedAt,
		}
	}

	return statuses, nil
}

// setUnitAgentStatus saves the given unit agent status, overwriting any
// current status data. If returns an error satisfying
// [statuserrors.UnitNotFound] if the unit doesn't exist.
func (st *ModelState) setUnitAgentStatus(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID coreunit.UUID,
	sts status.StatusInfo[status.UnitAgentStatusType],
) error {
	statusID, err := status.EncodeAgentStatus(sts.Status)
	if err != nil {
		return errors.Capture(err)
	}

	statusInfo := unitStatusInfo{
		UnitUUID:  unitUUID,
		StatusID:  statusID,
		Message:   sts.Message,
		Data:      sts.Data,
		UpdatedAt: sts.Since,
	}
	stmt, err := st.Prepare(`
INSERT INTO unit_agent_status (*) VALUES ($unitStatusInfo.*)
ON CONFLICT(unit_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at,
    data = excluded.data;
`, statusInfo)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, statusInfo).Run(); internaldatabase.IsErrConstraintForeignKey(err) {
		return errors.Errorf("%w: %q", statuserrors.UnitNotFound, unitUUID)
	} else if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// setUnitWorkloadStatus saves the given unit workload status, overwriting any
// current status data. If returns an error satisfying
// [statuserrors.UnitNotFound] if the unit doesn't exist.
func (st *ModelState) setUnitWorkloadStatus(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID coreunit.UUID,
	sts status.StatusInfo[status.WorkloadStatusType],
) error {
	statusID, err := status.EncodeWorkloadStatus(sts.Status)
	if err != nil {
		return errors.Capture(err)
	}

	statusInfo := unitStatusInfo{
		UnitUUID:  unitUUID,
		StatusID:  statusID,
		Message:   sts.Message,
		Data:      sts.Data,
		UpdatedAt: sts.Since,
	}
	stmt, err := st.Prepare(`
INSERT INTO unit_workload_status (*) VALUES ($unitStatusInfo.*)
ON CONFLICT(unit_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at,
    data = excluded.data;
`, statusInfo)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, statusInfo).Run(); internaldatabase.IsErrConstraintForeignKey(err) {
		return errors.Errorf("%w: %q", statuserrors.UnitNotFound, unitUUID)
	} else if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// setK8sPodStatus saves the given cloud container status, overwriting
// any current status data. If returns an error satisfying
// [statuserrors.UnitNotFound] if the unit doesn't exist.
func (st *ModelState) setK8sPodStatus(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID coreunit.UUID,
	sts status.StatusInfo[status.K8sPodStatusType],
) error {
	statusID, err := status.EncodeK8sPodStatus(sts.Status)
	if err != nil {
		return errors.Capture(err)
	}

	statusInfo := unitStatusInfo{
		UnitUUID:  unitUUID,
		StatusID:  statusID,
		Message:   sts.Message,
		Data:      sts.Data,
		UpdatedAt: sts.Since,
	}
	stmt, err := st.Prepare(`
INSERT INTO k8s_pod_status (*) VALUES ($unitStatusInfo.*)
ON CONFLICT(unit_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at,
    data = excluded.data;
`, statusInfo)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, statusInfo).Run(); internaldatabase.IsErrConstraintForeignKey(err) {
		return errors.Errorf("%w: %q", statuserrors.UnitNotFound, unitUUID)
	} else if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// GetApplicationAndUnitStatuses returns the application and unit statuses of
// all the applications in the model, indexed by application name.
func (st *ModelState) GetApplicationAndUnitStatuses(ctx context.Context) (map[string]status.Application, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result map[string]status.Application
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		if result, err = st.getApplicationsStatuses(ctx, tx); err != nil {
			return errors.Errorf("getting application statuses: %w", err)
		}

		unitStatues, err := st.getUnitsStatuses(ctx, tx)
		if err != nil {
			return errors.Errorf("getting unit statuses: %w", err)
		}

		// Assign the unit statuses to the applications.
		for appName, units := range unitStatues {
			app, ok := result[appName]
			if !ok {
				// A orphaned unit exists, but the application doesn't. This is
				// probably a bug, but we can ignore it for now.
				st.logger.Debugf(ctx, "application %q not found in application statuses", appName)
				continue
			}

			app.Units = units
			result[appName] = app
		}

		return err
	}); err != nil {
		return nil, errors.Errorf("getting application statuses: %w", err)
	}

	return result, nil
}

func (st *ModelState) getApplicationsStatuses(ctx context.Context, tx *sqlair.TX) (map[string]status.Application, error) {
	// Get all the applications.
	query, err := st.Prepare(`
SELECT
	a.name AS &applicationStatusDetails.name,
	a.uuid AS &applicationStatusDetails.uuid,
	a.life_id AS &applicationStatusDetails.life_id,
	ap.os_id AS &applicationStatusDetails.platform_os_id,
	ap.channel AS &applicationStatusDetails.platform_channel,
	ap.architecture_id AS &applicationStatusDetails.platform_architecture_id,
	ac.track AS &applicationStatusDetails.channel_track,
	ac.risk AS &applicationStatusDetails.channel_risk,
	ac.branch AS &applicationStatusDetails.channel_branch,
	cm.subordinate AS &applicationStatusDetails.subordinate,
	s.status_id AS &applicationStatusDetails.status_id,
	s.message AS &applicationStatusDetails.message,
	s.data AS &applicationStatusDetails.data,
	s.updated_at AS &applicationStatusDetails.updated_at,
	re.relation_uuid AS &applicationStatusDetails.relation_uuid,
	c.reference_name AS &applicationStatusDetails.charm_reference_name,
	c.revision AS &applicationStatusDetails.charm_revision,
	c.source_id AS &applicationStatusDetails.charm_source_id,
	c.architecture_id AS &applicationStatusDetails.charm_architecture_id,
	c.version AS &applicationStatusDetails.charm_version,
	c.lxd_profile AS &applicationStatusDetails.lxd_profile,
	aps.scale AS &applicationStatusDetails.scale,
	k8s.provider_id AS &applicationStatusDetails.k8s_provider_id,
	EXISTS(
		SELECT 1 FROM v_application_exposed_endpoint AS ae
		WHERE ae.application_uuid = a.uuid
	) AS &applicationStatusDetails.exposed,
	awv.version AS &applicationStatusDetails.workload_version
FROM application AS a
JOIN application_platform AS ap ON ap.application_uuid = a.uuid
LEFT JOIN application_channel AS ac ON ac.application_uuid = a.uuid
JOIN charm AS c ON c.uuid = a.charm_uuid
JOIN charm_metadata AS cm ON cm.charm_uuid = c.uuid
LEFT JOIN application_status AS s ON s.application_uuid = a.uuid
LEFT JOIN k8s_service AS k8s ON k8s.application_uuid = a.uuid
LEFT JOIN application_scale AS aps ON aps.application_uuid = a.uuid
LEFT JOIN v_relation_endpoint AS re ON re.application_uuid = a.uuid
LEFT JOIN application_workload_version AS awv ON awv.application_uuid = a.uuid
ORDER BY a.name, re.relation_uuid;
`, applicationStatusDetails{})
	if err != nil {
		return nil, errors.Errorf("preparing application query: %w", err)
	}

	var appStatuses []applicationStatusDetails
	if err := tx.Query(ctx, query).GetAll(&appStatuses); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}

	result := make(map[string]status.Application)
	for _, s := range appStatuses {
		appName := s.Name

		var relationUUID corerelation.UUID
		if s.RelationUUID.Valid {
			relationUUID = corerelation.UUID(s.RelationUUID.V)
			if err := relationUUID.Validate(); err != nil {
				return nil, errors.Errorf("invalid relation UUID %q: %w", s.RelationUUID.V, err)
			}
		}

		// If the application already exists, append the relation UUID to its
		// relations.
		if entry, exists := result[appName]; exists && s.RelationUUID.Valid {
			entry.Relations = append(entry.Relations, relationUUID)
			result[appName] = entry
			continue
		} else if exists {
			// This should never happen, but if it does, we have a duplicate
			// application name with no relation UUID. This is a problem.
			return nil, errors.Errorf("duplicate application name %q", appName)
		}

		// We've got a new application, so create a new status.
		var relations []corerelation.UUID
		if s.RelationUUID.Valid {
			relations = append(relations, relationUUID)
		}

		statusID, err := status.DecodeWorkloadStatus(s.StatusID)
		if err != nil {
			return nil, errors.Errorf("decoding workload status ID for application %q: %w", appName, err)
		}

		charmLocator, err := decodeCharmLocator(s.CharmLocatorDetails)
		if err != nil {
			return nil, errors.Errorf("decoding charm locator for application %q: %w", appName, err)
		}

		platform, err := decodePlatform(s.PlatformChannel, s.PlatformOSID, s.PlatformArchitectureID)
		if err != nil {
			return nil, errors.Errorf("decoding platform: %w", err)
		}

		channel, err := decodeChannel(s.ChannelTrack, s.ChannelRisk, s.ChannelBranch)
		if err != nil {
			return nil, errors.Errorf("decoding channel: %w", err)
		}

		var lxdProfile []byte
		if s.LXDProfile.Valid {
			lxdProfile = s.LXDProfile.V
		}

		var scale *int
		if s.Scale.Valid {
			scale = &s.Scale.V
		}

		var k8sProviderID *string
		if s.K8sProviderID.Valid {
			k8sProviderID = &s.K8sProviderID.V
		}

		var workloadVersion *string
		if s.WorkloadVersion.Valid && s.WorkloadVersion.V != "" {
			workloadVersion = &s.WorkloadVersion.V
		}

		result[appName] = status.Application{
			ID:          s.UUID,
			Life:        s.LifeID,
			Subordinate: s.Subordinate,
			Status: status.StatusInfo[status.WorkloadStatusType]{
				Status:  statusID,
				Message: s.Message,
				Data:    s.Data,
				Since:   s.UpdatedAt,
			},
			Relations:       relations,
			CharmLocator:    charmLocator,
			CharmVersion:    s.CharmVersion,
			LXDProfile:      lxdProfile,
			Platform:        platform,
			Channel:         channel,
			Exposed:         s.Exposed,
			Scale:           scale,
			WorkloadVersion: workloadVersion,
			K8sProviderID:   k8sProviderID,
		}
	}

	return result, nil
}

func (st *ModelState) getUnitsStatuses(ctx context.Context, tx *sqlair.TX) (map[string]map[coreunit.Name]status.Unit, error) {
	// Get all the units.
	query, err := st.Prepare(`
WITH unit_subordinate AS (
	SELECT u.name AS subordinate_name, principal_uuid
	FROM unit_principal
	JOIN unit AS u ON u.uuid = unit_principal.unit_uuid
)
SELECT
	u.name AS &unitStatusDetails.name,
	u.uuid AS &unitStatusDetails.uuid,
	u.life_id AS &unitStatusDetails.life_id,
	a.name AS &unitStatusDetails.application_name,
	m.name AS &unitStatusDetails.machine_name,
	c.reference_name AS &unitStatusDetails.charm_reference_name,
	c.revision AS &unitStatusDetails.charm_revision,
	c.source_id AS &unitStatusDetails.charm_source_id,
	c.architecture_id AS &unitStatusDetails.charm_architecture_id,
	cm.subordinate AS &unitStatusDetails.subordinate,
	upu.name AS &unitStatusDetails.principal_name,
	us.subordinate_name AS &unitStatusDetails.subordinate_name,
	uas.status_id AS &unitStatusDetails.agent_status_id,
	uas.message AS &unitStatusDetails.agent_message,
	uas.data AS &unitStatusDetails.agent_data,
	uas.updated_at AS &unitStatusDetails.agent_updated_at,
	uws.status_id AS &unitStatusDetails.workload_status_id,
	uws.message AS &unitStatusDetails.workload_message,
	uws.data AS &unitStatusDetails.workload_data,
	uws.updated_at AS &unitStatusDetails.workload_updated_at,
	uks.status_id AS &unitStatusDetails.k8s_pod_status_id,
	uks.message AS &unitStatusDetails.k8s_pod_message,
	uks.data AS &unitStatusDetails.k8s_pod_data,
	uws.updated_at AS &unitStatusDetails.k8s_pod_updated_at,
	k8s.provider_id AS &unitStatusDetails.k8s_provider_id,
	EXISTS(
		SELECT 1 FROM unit_agent_presence AS uap
		WHERE u.uuid = uap.unit_uuid
	) AS &unitStatusDetails.present,
	uav.version AS &unitStatusDetails.agent_version,
	awv.version AS &unitStatusDetails.workload_version
FROM unit AS u
JOIN application AS a ON a.uuid = u.application_uuid
JOIN net_node AS n ON n.uuid = u.net_node_uuid
JOIN charm AS c ON c.uuid = a.charm_uuid
JOIN charm_metadata AS cm ON cm.charm_uuid = c.uuid
LEFT JOIN machine AS m ON m.uuid = u.net_node_uuid
LEFT JOIN unit_agent_status AS uas ON uas.unit_uuid = u.uuid
LEFT JOIN unit_workload_status AS uws ON uws.unit_uuid = u.uuid
LEFT JOIN k8s_pod_status AS uks ON uks.unit_uuid = u.uuid
LEFT JOIN k8s_pod AS k8s ON k8s.unit_uuid = u.uuid
LEFT JOIN unit_principal AS up ON u.uuid = up.unit_uuid
LEFT JOIN unit AS upu ON up.principal_uuid = upu.uuid
LEFT JOIN unit_subordinate AS us ON us.principal_uuid = u.uuid
LEFT JOIN unit_agent_version AS uav ON uav.unit_uuid = u.uuid
LEFT JOIN unit_workload_version AS awv ON awv.unit_uuid = u.uuid
ORDER BY u.name;
`, unitStatusDetails{})
	if err != nil {
		return nil, errors.Errorf("preparing unit query: %w", err)
	}

	var unitStatuses []unitStatusDetails
	if err := tx.Query(ctx, query).GetAll(&unitStatuses); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}

	result := make(map[string]map[coreunit.Name]status.Unit)
	for _, s := range unitStatuses {
		appName := s.ApplicationName
		unitName := s.Name

		if _, exists := result[appName]; !exists {
			result[appName] = make(map[coreunit.Name]status.Unit)
		}

		var subordinateName coreunit.Name
		if s.SubordinateName.Valid {
			subordinateName = s.SubordinateName.V
		}

		if a, exists := result[appName][unitName]; exists && s.SubordinateName.Valid {
			// If we already have a subordinate unit, we don't need to add it
			// again.
			if _, ok := a.SubordinateNames[subordinateName]; ok {
				continue
			}

			a.SubordinateNames[subordinateName] = struct{}{}
			result[appName][unitName] = a
			continue
		} else if exists {
			// This should never happen, but if it does, we have a duplicate
			// unit name with no subordinate name. This is a problem.
			return nil, errors.Errorf("duplicate unit name %q", unitName)
		}

		charmLocator, err := decodeCharmLocator(s.CharmLocatorDetails)
		if err != nil {
			return nil, errors.Errorf("decoding charm locator for application %q: %w", appName, err)
		}

		var subordinates map[coreunit.Name]struct{}
		if s.SubordinateName.Valid {
			subordinates = make(map[coreunit.Name]struct{})
			subordinates[subordinateName] = struct{}{}
		}

		agentStatusID, err := status.DecodeAgentStatus(s.AgentStatusID)
		if err != nil {
			return nil, errors.Errorf("decoding agent status ID for unit %q: %w", unitName, err)
		}
		workloadStatusID, err := status.DecodeWorkloadStatus(s.WorkloadStatusID)
		if err != nil {
			return nil, errors.Errorf("decoding workload status ID for unit %q: %w", unitName, err)
		}
		k8sPodStatusID, err := status.DecodeK8sPodStatus(s.K8sPodStatusID)
		if err != nil {
			return nil, errors.Errorf("decoding k8s pod status ID for unit %q: %w", unitName, err)
		}

		var machineName *coremachine.Name
		if s.MachineName.Valid {
			machineName = &s.MachineName.V
		}

		var principalName *coreunit.Name
		if s.PrincipalName.Valid {
			principalName = &s.PrincipalName.V
		}

		var k8sProviderID *string
		if s.K8sProviderID.Valid {
			k8sProviderID = &s.K8sProviderID.V
		}

		var workloadVersion *string
		if s.WorkloadVersion.Valid && s.WorkloadVersion.V != "" {
			workloadVersion = &s.WorkloadVersion.V
		}

		result[appName][unitName] = status.Unit{
			ApplicationName:  s.ApplicationName,
			MachineName:      machineName,
			Life:             s.LifeID,
			CharmLocator:     charmLocator,
			Subordinate:      s.Subordinate,
			PrincipalName:    principalName,
			SubordinateNames: subordinates,
			AgentStatus: status.StatusInfo[status.UnitAgentStatusType]{
				Status:  agentStatusID,
				Message: s.AgentMessage,
				Data:    s.AgentData,
				Since:   s.AgentUpdatedAt,
			},
			WorkloadStatus: status.StatusInfo[status.WorkloadStatusType]{
				Status:  workloadStatusID,
				Message: s.WorkloadMessage,
				Data:    s.WorkloadData,
				Since:   s.WorkloadUpdatedAt,
			},
			K8sPodStatus: status.StatusInfo[status.K8sPodStatusType]{
				Status:  k8sPodStatusID,
				Message: s.K8sPodMessage,
				Data:    s.K8sPodData,
				Since:   s.K8sPodUpdatedAt,
			},
			Present:         s.Present,
			AgentVersion:    s.AgentVersion,
			WorkloadVersion: workloadVersion,
			K8sProviderID:   k8sProviderID,
		}
	}

	return result, nil
}

// GetApplicationAndUnitModelStatuses returns the application name and unit
// count for each model for the model status request.
func (st *ModelState) GetApplicationAndUnitModelStatuses(ctx context.Context) (map[string]int, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	query, err := st.Prepare(`
SELECT application.name AS &applicationNameUnitCount.name,
	   COUNT(u.uuid) AS &applicationNameUnitCount.unit_count
FROM application
LEFT JOIN unit AS u ON u.application_uuid = application.uuid
GROUP BY u.application_uuid, application.name;
`, applicationNameUnitCount{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var statuses []applicationNameUnitCount
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query).GetAll(&statuses)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	ret := make(map[string]int)
	for _, s := range statuses {
		ret[s.Name] = s.UnitCount
	}
	return ret, nil
}

// GetMachineStatus returns the status of the specified machine.
// This method may return the following errors:
// - [machineerrors.MachineNotFound] if the machine does not exist.
// - [statuserrors.MachineStatusNotFound] if the status is not set.
func (st *ModelState) GetMachineStatus(ctx context.Context, mName string) (status.StatusInfo[status.MachineStatusType], error) {
	db, err := st.DB()
	if err != nil {
		return status.StatusInfo[status.MachineStatusType]{}, errors.Capture(err)
	}

	nameIdent := machineName{Name: mName}

	var uuid machineUUID
	uuidQuery := `SELECT uuid AS &machineUUID.uuid FROM machine WHERE name = $machineName.name;`
	uuidQueryStmt, err := st.Prepare(uuidQuery, nameIdent, uuid)
	if err != nil {
		return status.StatusInfo[status.MachineStatusType]{}, errors.Capture(err)
	}

	var mStatus machineStatus
	statusQuery := `
SELECT &machineStatus.*
FROM v_machine_status AS st
WHERE st.machine_uuid = $machineUUID.uuid;
`
	statusCombinedQueryStmt, err := st.Prepare(statusQuery, uuid, mStatus)
	if err != nil {
		return status.StatusInfo[status.MachineStatusType]{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine uuid
		err := tx.Query(ctx, uuidQueryStmt, nameIdent).Get(&uuid)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting machine status for %q: %w", mName, machineerrors.MachineNotFound)
		} else if err != nil {
			return errors.Errorf("querying uuid for machine %q: %w", mName, err)
		}

		// Query for the machine cloud instance status and status data combined
		err = tx.Query(ctx, statusCombinedQueryStmt, uuid).Get(&mStatus)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine: %q: %w", mName, statuserrors.MachineStatusNotFound)
		} else if err != nil {
			return errors.Errorf("querying machine status for machine %q: %w", mName, err)
		}

		return nil
	})

	if err != nil {
		return status.StatusInfo[status.MachineStatusType]{}, errors.Capture(err)
	}

	// Convert the internal status id from the (machine_status_value table)
	// into the core status.Status type.
	machineStatus, err := status.DecodeMachineStatus(mStatus.Status)
	if err != nil {
		return status.StatusInfo[status.MachineStatusType]{}, errors.Errorf("decoding machine status for machine %q: %w", mName, err)
	}

	var since *time.Time
	if mStatus.Updated.Valid {
		since = &mStatus.Updated.V
	}
	var data []byte
	if len(mStatus.Data) > 0 {
		data = mStatus.Data
	}

	return status.StatusInfo[status.MachineStatusType]{
		Status:  machineStatus,
		Message: mStatus.Message,
		Since:   since,
		Data:    data,
	}, nil
}

// GetAllMachineStatuses returns all the machine statuses for the model, indexed
// by machine name.
func (st *ModelState) GetAllMachineStatuses(ctx context.Context) (map[string]status.StatusInfo[status.MachineStatusType], error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &machineNameStatus.*
FROM v_machine_status AS ms
JOIN machine AS m ON ms.machine_uuid = m.uuid
	`, machineNameStatus{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var res []machineNameStatus
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&res)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}

	result := make(map[string]status.StatusInfo[status.MachineStatusType])
	for _, mStatus := range res {
		// Convert the internal status id from the (machine_status_value table)
		// into the core status.Status type.
		machineStatus, err := status.DecodeMachineStatus(mStatus.Status)
		if err != nil {
			return nil, errors.Errorf("decoding machine status for machine %q: %w", mStatus.Name, err)
		}
		var since *time.Time
		if mStatus.Updated.Valid {
			since = &mStatus.Updated.V
		}
		var data []byte
		if len(mStatus.Data) > 0 {
			data = mStatus.Data
		}
		result[mStatus.Name] = status.StatusInfo[status.MachineStatusType]{
			Status:  machineStatus,
			Message: mStatus.Message,
			Since:   since,
			Data:    data,
		}
	}
	return result, nil
}

// GetMachineStatuses returns all the machine statuses for the model, indexed
// by machine name.
func (st *ModelState) GetMachineStatuses(ctx context.Context) (map[coremachine.Name]status.Machine, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT
  m.name AS &machineStatusDetails.name,
  m.uuid AS &machineStatusDetails.uuid,
  m.life_id AS &machineStatusDetails.life_id,
  m.hostname AS &machineStatusDetails.hostname,
  mci.instance_id AS &machineStatusDetails.instance_id,
  mci.display_name AS &machineStatusDetails.display_name,
  mci.arch AS &machineStatusDetails.instance_arch,
  mci.cpu_cores AS &machineStatusDetails.instance_cpu_cores, 
  mci.cpu_power AS &machineStatusDetails.instance_cpu_power,
  mci.mem AS &machineStatusDetails.instance_mem,
  mci.root_disk AS &machineStatusDetails.instance_root_disk,
  mci.root_disk_source AS &machineStatusDetails.instance_root_disk_source,
  mci.virt_type AS &machineStatusDetails.instance_virt_type,
  az.name AS &machineStatusDetails.availability_zone_name, 
  p.os_id AS &machineStatusDetails.platform_os_id,
  p.channel AS &machineStatusDetails.platform_channel,
  p.architecture_id AS &machineStatusDetails.platform_architecture_id,
  ms.status_id AS &machineStatusDetails.machine_status_id,
  ms.message AS &machineStatusDetails.machine_message,
  ms.data AS &machineStatusDetails.machine_data,
  ms.updated_at AS &machineStatusDetails.machine_updated_at,
  mcis.status_id AS &machineStatusDetails.instance_status_id,
  mcis.message AS &machineStatusDetails.instance_message,
  mcis.data AS &machineStatusDetails.instance_data,
  mcis.updated_at AS &machineStatusDetails.instance_updated_at,
  c.arch AS &machineStatusDetails.constraint_arch,
  c.cpu_cores AS &machineStatusDetails.constraint_cpu_cores,
  c.cpu_power AS &machineStatusDetails.constraint_cpu_power,
  c.mem AS &machineStatusDetails.constraint_mem,
  c.root_disk AS &machineStatusDetails.constraint_root_disk,
  c.root_disk_source AS &machineStatusDetails.constraint_root_disk_source,
  c.instance_role AS &machineStatusDetails.constraint_instance_role,
  c.instance_type AS &machineStatusDetails.constraint_instance_type,
  ct.value AS &machineStatusDetails.constraint_container_type,
  c.virt_type AS &machineStatusDetails.constraint_virt_type,
  c.allocate_public_ip AS &machineStatusDetails.constraint_allocate_public_ip,
  c.image_id AS &machineStatusDetails.constraint_image_id
FROM machine AS m
LEFT JOIN machine_status AS ms ON ms.machine_uuid = m.uuid
LEFT JOIN machine_platform AS p ON p.machine_uuid = m.uuid
LEFT JOIN machine_cloud_instance AS mci ON mci.machine_uuid = m.uuid
LEFT JOIN machine_cloud_instance_status mcis ON mcis.machine_uuid = m.uuid
LEFT JOIN availability_zone AS az ON az.uuid = mci.availability_zone_uuid
LEFT JOIN machine_constraint AS mc ON mc.machine_uuid = m.uuid
LEFT JOIN "constraint" AS c ON c.uuid = mc.constraint_uuid
LEFT JOIN container_type AS ct ON c.container_type_id = ct.id;
`, machineStatusDetails{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	tagsStmt, err := st.Prepare(` SELECT &instanceTag.* FROM instance_tag`, instanceTag{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var (
		res     []machineStatusDetails
		resTags []instanceTag
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&res)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, tagsStmt).GetAll(&resTags)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}

	tags := make(map[string][]string)
	for _, t := range resTags {
		tags[t.MachineUUID] = append(tags[t.MachineUUID], t.Tag)
	}

	result := make(map[coremachine.Name]status.Machine)
	for _, s := range res {
		instanceID := instance.UnknownId
		if s.InstanceID.Valid {
			instanceID = instance.Id(s.InstanceID.V)
		}

		var displayName string
		if s.DisplayName.Valid {
			displayName = s.DisplayName.V
		}

		var hostname string
		if s.Hostname.Valid {
			hostname = s.Hostname.V
		}

		platform, err := decodePlatform(s.PlatformChannel, s.PlatformOSID, s.PlatformArchitectureID)
		if err != nil {
			return nil, errors.Errorf("decoding platform: %w", err)
		}

		hwc := decodeHardwareCharacteristics(
			s.InstanceArch, s.InstanceCPUCores, s.InstanceCPUPower,
			s.InstanceMem, s.InstanceRootDisk, s.InstanceRootDiskSource,
			s.InstanceVirtType, tags[s.UUID.String()], s.InstanceAvailabilityZone,
		)

		cons := decodeConstraints(
			s.ConstraintArch, s.ConstraintCPUCores, s.ConstraintCPUPower,
			s.ConstraintMem, s.ConstraintRootDisk, s.ConstraintRootDiskSource,
			s.ConstraintVirtType, s.ConstraintInstanceRole,
			s.ConstraintInstanceType, s.ConstraintContainerType,
			s.ConstraintAllocatePublicIP, s.ConstraintImageID,
		)

		result[s.Name] = status.Machine{
			UUID:        s.UUID,
			Life:        s.LifeID,
			Hostname:    hostname,
			InstanceID:  instanceID,
			DisplayName: displayName,
			Platform:    platform,
			MachineStatus: status.StatusInfo[status.MachineStatusType]{
				Status:  s.MachineStatusID,
				Message: s.MachineMessage,
				Data:    s.MachineData,
				Since:   s.MachineUpdatedAt,
			},
			InstanceStatus: status.StatusInfo[status.InstanceStatusType]{
				Status:  s.InstanceStatusID,
				Message: s.InstanceMessage,
				Data:    s.InstanceData,
				Since:   s.InstanceUpdatedAt,
			},
			HardwareCharacteristics: hwc,
			Constraints:             cons,
		}
	}
	return result, nil
}

// SetMachineStatus sets the status of the specified machine.
// This method may return the following errors:
// - [machineerrors.MachineNotFound] if the machine does not exist.
func (st *ModelState) SetMachineStatus(ctx context.Context, mName string, newStatus status.StatusInfo[status.MachineStatusType]) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	statusID, err := status.EncodeMachineStatus(newStatus.Status)
	if err != nil {
		return errors.Capture(err)
	}

	nameIdent := machineName{Name: mName}
	queryMachine := `SELECT uuid AS &machineUUID.* FROM machine WHERE name = $machineName.name;`
	queryMachineStmt, err := st.Prepare(queryMachine, nameIdent, machineUUID{})
	if err != nil {
		return errors.Capture(err)
	}

	statusQuery := `
INSERT INTO machine_status (*)
VALUES ($setMachineStatus.*)
  ON CONFLICT (machine_uuid)
  DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at,
    data = excluded.data;
`
	statusQueryStmt, err := st.Prepare(statusQuery, setMachineStatus{})
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var mUUID machineUUID
		if err := tx.Query(ctx, queryMachineStmt, nameIdent).Get(&mUUID); errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q: %w", mName, machineerrors.MachineNotFound)
		} else if err != nil {
			return errors.Errorf("querying uuid for machine %q: %w", mName, err)
		}

		err = tx.Query(ctx, statusQueryStmt, setMachineStatus{
			MachineUUID: mUUID.UUID,
			StatusID:    statusID,
			Message:     newStatus.Message,
			Data:        newStatus.Data,
			UpdatedAt:   newStatus.Since,
		}).Run()
		if err != nil {
			return errors.Errorf("setting machine status for machine %q: %w", mName, err)
		}

		return nil
	})
}

// GetInstanceStatus returns the cloud specific instance status for the given
// machine.
// This method may return the following errors:
// - [machineerrors.MachineNotFound] if the machine does not exist or;
// - [statuserrors.MachineStatusNotFound] if the status is not set.
func (st *ModelState) GetInstanceStatus(ctx context.Context, mName string) (status.StatusInfo[status.InstanceStatusType], error) {
	db, err := st.DB()
	if err != nil {
		return status.StatusInfo[status.InstanceStatusType]{}, errors.Capture(err)
	}

	nameIdent := machineName{Name: mName}

	var uuid machineUUID
	uuidQuery := `SELECT uuid AS &machineUUID.* FROM machine WHERE name = $machineName.name`
	uuidQueryStmt, err := st.Prepare(uuidQuery, nameIdent, uuid)
	if err != nil {
		return status.StatusInfo[status.InstanceStatusType]{}, errors.Capture(err)
	}

	var mStatus machineStatus
	statusCombinedQuery := `
SELECT &machineStatus.*
FROM v_machine_cloud_instance_status AS st
WHERE st.machine_uuid = $machineUUID.uuid`
	statusCombinedQueryStmt, err := st.Prepare(statusCombinedQuery, uuid, mStatus)
	if err != nil {
		return status.StatusInfo[status.InstanceStatusType]{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine uuid
		err := tx.Query(ctx, uuidQueryStmt, nameIdent).Get(&uuid)
		if errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		} else if err != nil {
			return errors.Errorf("querying uuid for machine %q: %w", mName, err)
		}

		// Query for the machine cloud instance status and status data combined
		err = tx.Query(ctx, statusCombinedQueryStmt, uuid).Get(&mStatus)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine instance: %q: %w", mName, statuserrors.MachineStatusNotFound)
		} else if err != nil {
			return errors.Errorf("querying cloud instance status and status data for machine %q: %w", mName, err)
		}

		return nil
	})
	if err != nil {
		return status.StatusInfo[status.InstanceStatusType]{}, errors.Capture(err)
	}

	// Convert the internal status id from the
	// (machine_cloud_instance_status_value table) into the core status.Status
	// type.
	machineStatus, err := status.DecodeCloudInstanceStatus(mStatus.Status)
	if err != nil {
		return status.StatusInfo[status.InstanceStatusType]{}, errors.Errorf("decoding cloud instance status for machine %q: %w", mName, err)
	}

	var since *time.Time
	if mStatus.Updated.Valid {
		since = &mStatus.Updated.V
	}
	var data []byte
	if len(mStatus.Data) > 0 {
		data = mStatus.Data
	}
	return status.StatusInfo[status.InstanceStatusType]{
		Status:  machineStatus,
		Message: mStatus.Message,
		Since:   since,
		Data:    data,
	}, nil
}

// GetAllInstanceStatuses returns all the instance statuses for the model, indexed
// by machine name.
func (st *ModelState) GetAllInstanceStatuses(ctx context.Context) (map[string]status.StatusInfo[status.InstanceStatusType], error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &machineNameStatus.*
FROM v_machine_cloud_instance_status AS ms
JOIN machine AS m ON ms.machine_uuid = m.uuid
	`, machineNameStatus{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var res []machineNameStatus
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&res)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	}); err != nil {
		return nil, errors.Capture(err)
	}

	result := make(map[string]status.StatusInfo[status.InstanceStatusType])
	for _, mStatus := range res {
		machineStatus, err := status.DecodeCloudInstanceStatus(mStatus.Status)
		if err != nil {
			return nil, errors.Errorf("decoding cloud instance status for machine %q: %w", mStatus.Name, err)
		}
		var since *time.Time
		if mStatus.Updated.Valid {
			since = &mStatus.Updated.V
		}
		var data []byte
		if len(mStatus.Data) > 0 {
			data = mStatus.Data
		}
		result[mStatus.Name] = status.StatusInfo[status.InstanceStatusType]{
			Status:  machineStatus,
			Message: mStatus.Message,
			Since:   since,
			Data:    data,
		}
	}
	return result, nil
}

// SetInstanceStatus sets the cloud specific instance status for this
// machine.
// This method may return the following errors:
// - [machineerrors.MachineNotFound] if the machine does not exist.
func (st *ModelState) SetInstanceStatus(ctx context.Context, mName string, newStatus status.StatusInfo[status.InstanceStatusType]) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	statusID, err := status.EncodeCloudInstanceStatus(newStatus.Status)
	if err != nil {
		return errors.Capture(err)
	}

	nameIdent := machineName{Name: mName}
	queryMachine := `SELECT uuid AS &machineUUID.* FROM machine WHERE name = $machineName.name;`
	queryMachineStmt, err := st.Prepare(queryMachine, nameIdent, machineUUID{})
	if err != nil {
		return errors.Capture(err)
	}

	statusQuery := `
INSERT INTO machine_cloud_instance_status (*)
VALUES ($setMachineStatus.*)
  ON CONFLICT (machine_uuid)
  DO UPDATE SET 
    status_id = excluded.status_id, 
    message = excluded.message, 
    updated_at = excluded.updated_at,
    data = excluded.data;
`
	statusQueryStmt, err := st.Prepare(statusQuery, setMachineStatus{})
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var mUUID machineUUID
		if err := tx.Query(ctx, queryMachineStmt, nameIdent).Get(&mUUID); errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q: %w", mName, machineerrors.MachineNotFound)
		} else if err != nil {
			return errors.Errorf("querying uuid for machine %q: %w", mName, err)
		}

		// Query for setting the machine cloud instance status
		err = tx.Query(ctx, statusQueryStmt, setMachineStatus{
			MachineUUID: mUUID.UUID,
			StatusID:    statusID,
			Message:     newStatus.Message,
			Data:        newStatus.Data,
			UpdatedAt:   newStatus.Since,
		}).Run()
		if err != nil {
			return errors.Errorf("setting machine status for machine %q: %w", mName, err)
		}
		return nil
	})
}

func ptr[T any](v T) *T {
	return &v
}
