// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"time"

	"github.com/canonical/sqlair"

	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/errors"
)

// ApplicationExists returns true if a application exists with the input UUID.
func (st *State) ApplicationExists(ctx context.Context, aUUID string) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	applicationUUID := entityUUID{UUID: aUUID}
	existsStmt, err := st.Prepare(`
SELECT uuid AS &entityUUID.uuid
FROM   application
WHERE  uuid = $entityUUID.uuid`, applicationUUID)
	if err != nil {
		return false, errors.Errorf("preparing application exists query: %w", err)
	}

	var applicationExists bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, existsStmt, applicationUUID).Get(&applicationUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("running application exists query: %w", err)
		}

		applicationExists = true
		return nil
	})

	return applicationExists, errors.Capture(err)
}

// EnsureApplicationNotAlive ensures that there is no application
// identified by the input UUID, that is still alive.
func (st *State) EnsureApplicationNotAlive(ctx context.Context, aUUID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	applicationUUID := entityUUID{UUID: aUUID}
	updateUnitStmt, err := st.Prepare(`
UPDATE application
SET    life_id = 1
WHERE  uuid = $entityUUID.uuid
AND    life_id = 0`, applicationUUID)
	if err != nil {
		return errors.Errorf("preparing application life update: %w", err)
	}

	if err := errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, updateUnitStmt, applicationUUID).Run(); err != nil {
			return errors.Errorf("advancing application life: %w", err)
		}

		return nil
	})); err != nil {
		return err
	}

	return nil
}

// ApplicationScheduleRemoval schedules a removal job for the application with
// the input UUID, qualified with the input force boolean.
// We don't care if the application does not exist at this point because:
// - it should have been validated prior to calling this method,
// - the removal job executor will handle that fact.
func (st *State) ApplicationScheduleRemoval(
	ctx context.Context, removalUUID, applicationUUID string,
	force bool, when time.Time,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	removalRec := removalJob{
		UUID:          removalUUID,
		RemovalTypeID: 2,
		EntityUUID:    applicationUUID,
		Force:         force,
		ScheduledFor:  when,
	}

	stmt, err := st.Prepare("INSERT INTO removal (*) VALUES ($removalJob.*)", removalRec)
	if err != nil {
		return errors.Errorf("preparing application removal: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, removalRec).Run()
		if err != nil {
			return errors.Errorf("scheduling application removal: %w", err)
		}
		return nil
	}))
}

// GetApplicationLife returns the life of the application with the input UUID.
func (st *State) GetApplicationLife(ctx context.Context, aUUID string) (life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return -1, errors.Capture(err)
	}

	var applicationLife entityLife
	applicationUUID := entityUUID{UUID: aUUID}

	stmt, err := st.Prepare(`
SELECT &entityLife.life_id
FROM   application
WHERE  uuid = $entityUUID.uuid;`, applicationLife, applicationUUID)
	if err != nil {
		return -1, errors.Errorf("preparing application life query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, applicationUUID).Get(&applicationLife)
		if errors.Is(err, sqlair.ErrNoRows) {
			return applicationerrors.ApplicationNotFound
		} else if err != nil {
			return errors.Errorf("running application life query: %w", err)
		}

		return nil
	})

	return applicationLife.Life, errors.Capture(err)
}

// DeleteApplication removes a application from the database completely.
func (st *State) DeleteApplication(ctx context.Context, aUUID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	applicationUUIDCount := entityAssociationCount{UUID: aUUID}

	stmt, err := st.Prepare(`
SELECT COUNT(*) AS &entityAssociationCount.count
FROM   application
WHERE  uuid = $entityAssociationCount.uuid;`, applicationUUIDCount)
	if err != nil {
		return errors.Errorf("preparing application life query: %w", err)
	}

	deleteApplicationStmt, err := st.Prepare(`
DELETE FROM application
WHERE  uuid = $entityAssociationCount.uuid;`, applicationUUIDCount)
	if err != nil {
		return errors.Errorf("preparing unit delete: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, applicationUUIDCount).Get(&applicationUUIDCount)
		if errors.Is(err, sqlair.ErrNoRows) || applicationUUIDCount.Count == 0 {
			return applicationerrors.ApplicationNotFound
		} else if err != nil {
			return errors.Errorf("running application life query: %w", err)
		}

		if err := st.deleteApplicationAnnotations(ctx, tx, aUUID); err != nil {
			return errors.Errorf("deleting application annotations: %w", err)
		}

		if err := st.deleteCloudServices(ctx, tx, aUUID); err != nil {
			return errors.Errorf("deleting cloud services: %w", err)
		}

		if err := st.deleteDeviceConstraintAttributes(ctx, tx, aUUID); err != nil {
			return errors.Errorf("deleting device constraint attributes: %w", err)
		}

		if err := st.deleteSimpleApplicationReferences(ctx, tx, aUUID); err != nil {
			return errors.Errorf("deleting simple application references: %w", err)
		}

		if err := tx.Query(ctx, deleteApplicationStmt, applicationUUIDCount).Run(); err != nil {
			return errors.Errorf("deleting application: %w", err)
		}

		return nil
	}))
}

func (st *State) deleteSimpleApplicationReferences(ctx context.Context, tx *sqlair.TX, aUUID string) error {
	app := applicationUUID{UUID: aUUID}

	for _, table := range []string{
		"application_channel",
		"application_platform",
		"application_scale",
		"application_config",
		"application_config_hash",
		"application_constraint",
		"application_setting",
		"application_exposed_endpoint_space",
		"application_exposed_endpoint_cidr",
		"application_endpoint",
		"application_extra_endpoint",
		"application_storage_directive",
		"application_workload_version",
		"device_constraint",
	} {
		deleteApplicationReference := fmt.Sprintf(`DELETE FROM %s WHERE application_uuid = $applicationUUID.application_uuid`, table)
		deleteApplicationReferenceStmt, err := st.Prepare(deleteApplicationReference, app)
		if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, deleteApplicationReferenceStmt, app).Run(); err != nil {
			return errors.Errorf("deleting reference to application in %s: %w", table, err)
		}
	}
	return nil
}

func (st *State) deleteCloudServices(ctx context.Context, tx *sqlair.TX, aUUID string) error {
	app := applicationUUID{UUID: aUUID}

	deleteNodeStmt, err := st.Prepare(`
DELETE FROM net_node WHERE uuid IN (
    SELECT net_node_uuid
    FROM k8s_service
    WHERE application_uuid = $applicationUUID.application_uuid
)`, app)
	if err != nil {
		return errors.Capture(err)
	}

	deleteCloudServiceStmt, err := st.Prepare(`
DELETE FROM k8s_service
WHERE application_uuid = $applicationUUID.application_uuid
`, app)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, deleteCloudServiceStmt, app).Run(); err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, deleteNodeStmt, app).Run(); err != nil {
		return errors.Errorf("deleting net node for cloud service: %w", err)
	}
	return nil
}

func (st *State) deleteDeviceConstraintAttributes(ctx context.Context, tx *sqlair.TX, aUUID string) error {
	appID := applicationUUID{UUID: aUUID}
	deleteDeviceConstraintAttributesStmt, err := st.Prepare(`
DELETE FROM device_constraint_attribute
WHERE device_constraint_uuid IN (
    SELECT device_constraint_uuid
    FROM device_constraint
    WHERE application_uuid = $applicationUUID.application_uuid
)`, appID)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, deleteDeviceConstraintAttributesStmt, appID).Run(); err != nil {
		return errors.Errorf("deleting device constraint attributes: %w", err)
	}
	return nil
}

func (st *State) deleteApplicationAnnotations(ctx context.Context, tx *sqlair.TX, aUUID string) error {
	appID := applicationUUID{UUID: aUUID}

	deleteUnitAnnotationStmt, err := st.Prepare(`
DELETE FROM annotation_application
WHERE  uuid = $applicationUUID.application_uuid`, appID)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, deleteUnitAnnotationStmt, appID).Run(); err != nil {
		return errors.Errorf("removing application annotations: %w", err)
	}
	return nil
}
