// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	removalerrors "github.com/juju/juju/domain/removal/errors"
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

// EnsureApplicationNotAliveCascade ensures that there is no application
// identified by the input application UUID, that is still alive. If the
// application has units, they are also guaranteed to be no longer alive,
// cascading. The affected unit UUIDs are returned. If the units are also
// the last ones on their machines, it will cascade and the machines are
// also set to dying. The affected machine UUIDs are returned.
func (st *State) EnsureApplicationNotAliveCascade(ctx context.Context, aUUID string) (unitUUIDs, machineUUIDs []string, err error) {
	db, err := st.DB()
	if err != nil {
		return nil, nil, errors.Capture(err)
	}

	applicationUUID := entityUUID{UUID: aUUID}
	updateApplicationStmt, err := st.Prepare(`
UPDATE application
SET    life_id = 1
WHERE  uuid = $entityUUID.uuid
AND    life_id = 0`, applicationUUID)
	if err != nil {
		return nil, nil, errors.Errorf("preparing application life update: %w", err)
	}

	// Also ensure that any units that are associated with the application
	// are also set to dying. This has to be done in a single transaction
	// because we want to ensure that the application is not alive, and
	// that no units are alive at the same time. Preventing any races.
	selectUnitUUIDsStmt, err := st.Prepare(`
SELECT uuid AS &entityUUID.uuid
FROM   unit
WHERE  application_uuid = $entityUUID.uuid
AND    life_id = 0`, applicationUUID)
	if err != nil {
		return nil, nil, errors.Errorf("preparing unit life query: %w", err)
	}

	updateUnitStmt, err := st.Prepare(`
UPDATE unit
SET    life_id = 1
WHERE  application_uuid = $entityUUID.uuid
AND    life_id = 0`, applicationUUID)
	if err != nil {
		return nil, nil, errors.Errorf("preparing unit life update: %w", err)
	}

	var unitUUIDsRec []entityUUID
	var machineUUIDsRec []string
	if err := errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, updateApplicationStmt, applicationUUID).Run(); err != nil {
			return errors.Errorf("advancing application life: %w", err)
		}

		if err := tx.Query(ctx, selectUnitUUIDsStmt, applicationUUID).GetAll(&unitUUIDsRec); errors.Is(err, sqlair.ErrNoRows) {
			// If there are no units associated with the application,
			// we can just return nil, as there is nothing to update.
			return nil
		} else if err != nil {
			return errors.Errorf("selecting associated application unit lives: %w", err)
		}

		// We guarantee to have at least one unit UUID here.

		if err := tx.Query(ctx, updateUnitStmt, applicationUUID).Run(); err != nil {
			return errors.Errorf("advancing associated application unit lives: %w", err)
		}

		// If any units are also the last ones on their machines, we also
		// set the machines to dying. This will ensure that any of those
		// machines will be stopped from being used.

		for _, unit := range unitUUIDsRec {
			machineUUID, err := st.markMachineAsDyingIfAllUnitsAreNotAlive(ctx, tx, unit.UUID)
			if err != nil {
				return errors.Errorf("marking last unit on machine as dying: %w", err)
			} else if machineUUID == "" {
				// The unit was not the last one on the machine, so we can
				// skip to the next unit.
				continue
			}

			machineUUIDsRec = append(machineUUIDsRec, machineUUID)
		}

		return nil
	})); err != nil {
		return nil, nil, err
	}

	return transform.Slice(unitUUIDsRec, func(e entityUUID) string {
		return e.UUID
	}), machineUUIDsRec, nil
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

	unitsStmt, err := st.Prepare(`
SELECT count(*) AS &entityAssociationCount.count
FROM unit
WHERE application_uuid = $entityAssociationCount.uuid
`, applicationUUIDCount)
	if err != nil {
		return errors.Capture(err)
	}

	deleteApplicationStmt, err := st.Prepare(`
DELETE FROM application
WHERE  uuid = $entityAssociationCount.uuid;`, applicationUUIDCount)
	if err != nil {
		return errors.Errorf("preparing application delete: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// TODO (stickupkid): We should ensure that the application is not
		// in a dying state, but nothing calls MarkApplicationAsDead. It is
		// assumed that, as long as all units are removed then we can
		// delete the application.
		aLife, err := st.getApplicationLife(ctx, tx, aUUID)
		if err != nil {
			return errors.Errorf("getting application life: %w", err)
		} else if aLife == life.Alive {
			// The application is still alive, we cannot delete it.
			return errors.Errorf("cannot delete application %q as it is still alive", aUUID).
				Add(removalerrors.EntityStillAlive)
		}

		// Check that there are no units.
		var numUnits entityAssociationCount
		err = tx.Query(ctx, unitsStmt, applicationUUIDCount).Get(&numUnits)
		if err != nil {
			return errors.Errorf("querying application units: %w", err)
		} else if numUnits := numUnits.Count; numUnits > 0 {
			// It is required that all units have been completely removed
			// before the application can be removed.
			return errors.Errorf("cannot delete application as it still has %d unit(s)", numUnits).
				Add(applicationerrors.ApplicationHasUnits).
				Add(removalerrors.RemovalJobIncomplete)
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

		// Get the charm UUID before we delete the application.
		charmUUID, err := st.getCharmUUIDForApplication(ctx, tx, aUUID)
		if err != nil {
			return errors.Errorf("getting charm UUID for application: %w", err)
		}

		// Delete the application itself.
		if err := tx.Query(ctx, deleteApplicationStmt, applicationUUIDCount).Run(); err != nil {
			return errors.Errorf("deleting application: %w", err)
		}

		// See if it's possible to delete the charm any more.
		if err := st.deleteCharmIfUnusedByUUID(ctx, tx, charmUUID); err != nil {
			return errors.Errorf("deleting charm if unused: %w", err)
		}

		return nil
	}))
}

func (st *State) deleteSimpleApplicationReferences(ctx context.Context, tx *sqlair.TX, aUUID string) error {
	app := entityUUID{UUID: aUUID}

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
		"application_resource",
		"application_status",
		"application_workload_version",
		"device_constraint",
	} {
		deleteApplicationReference := fmt.Sprintf(`DELETE FROM %s WHERE application_uuid = $entityUUID.uuid`, table)
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
	app := entityUUID{UUID: aUUID}

	deleteNodeStmt, err := st.Prepare(`
DELETE FROM net_node WHERE uuid IN (
    SELECT net_node_uuid
    FROM k8s_service
    WHERE application_uuid = $entityUUID.uuid
)`, app)
	if err != nil {
		return errors.Capture(err)
	}

	deleteCloudServiceStmt, err := st.Prepare(`
DELETE FROM k8s_service
WHERE application_uuid = $entityUUID.uuid
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
	appID := entityUUID{UUID: aUUID}
	deleteDeviceConstraintAttributesStmt, err := st.Prepare(`
DELETE FROM device_constraint_attribute
WHERE device_constraint_uuid IN (
    SELECT device_constraint_uuid
    FROM device_constraint
    WHERE application_uuid = $entityUUID.uuid
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
	appID := entityUUID{UUID: aUUID}

	deleteApplicationAnnotationStmt, err := st.Prepare(`
DELETE FROM annotation_application
WHERE  uuid = $entityUUID.uuid`, appID)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, deleteApplicationAnnotationStmt, appID).Run(); err != nil {
		return errors.Errorf("removing application annotations: %w", err)
	}
	return nil
}

func (st *State) getCharmUUIDForApplication(ctx context.Context, tx *sqlair.TX, aUUID string) (string, error) {
	appID := entityUUID{UUID: aUUID}

	stmt, err := st.Prepare(`
SELECT charm_uuid AS &entityUUID.uuid
FROM   application
WHERE  uuid = $entityUUID.uuid`, appID)
	if err != nil {
		return "", errors.Errorf("preparing charm UUID query: %w", err)
	}

	var result entityUUID
	if err := tx.Query(ctx, stmt, appID).Get(&result); errors.Is(err, sqlair.ErrNoRows) {
		// No charm associated with the application, so we can skip this.
		return "", nil
	} else if err != nil {
		return "", errors.Errorf("running charm UUID query: %w", err)
	}
	return result.UUID, nil
}

func (st *State) deleteCharmIfUnusedByUUID(ctx context.Context, tx *sqlair.TX, charmUUID string) error {
	// If the charm UUID is empty, we can skip the deletion.
	if charmUUID == "" {
		return nil
	}

	uuidCount := entityAssociationCount{UUID: charmUUID}

	// Check if the charm is still used by any application.
	// Split the query into two parts, so we can output a better log message.
	appStmt, err := st.Prepare(`
SELECT COUNT(*) AS &entityAssociationCount.count
FROM   application
WHERE  charm_uuid = $entityAssociationCount.uuid`, uuidCount)
	if err != nil {
		return errors.Errorf("preparing application charm usage query: %w", err)
	}

	unitStmt, err := st.Prepare(`
SELECT COUNT(*) AS &entityAssociationCount.count
FROM   unit
WHERE  charm_uuid = $entityAssociationCount.uuid`, uuidCount)
	if err != nil {
		return errors.Errorf("preparing unit charm usage query: %w", err)
	}

	if err := tx.Query(ctx, appStmt, uuidCount).Get(&uuidCount); err != nil {
		return errors.Errorf("running application charm usage query: %w", err)
	} else if uuidCount.Count > 0 {
		st.logger.Infof(ctx, "charm %q is still used by %d application(s), not deleting", charmUUID, uuidCount.Count)
		return nil
	}

	if err := tx.Query(ctx, unitStmt, uuidCount).Get(&uuidCount); err != nil {
		return errors.Errorf("running unit charm usage query: %w", err)
	} else if uuidCount.Count > 0 {
		st.logger.Infof(ctx, "charm %q is still used by %d unit(s), not deleting", charmUUID, uuidCount.Count)
		return nil
	}

	return st.deleteCharm(ctx, tx, charmUUID)
}

func (st *State) deleteCharm(ctx context.Context, tx *sqlair.TX, cUUID string) error {
	charmUUID := entityUUID{UUID: cUUID}

	getCharmResourcesStmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM resource
WHERE charm_uuid = $entityUUID.uuid
`, charmUUID)
	if err != nil {
		return errors.Capture(err)
	}

	var resourceUUIDs = []entityUUID{}
	err = tx.Query(ctx, getCharmResourcesStmt, charmUUID).GetAll(&resourceUUIDs)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("getting charm resources: %w", err)
	}
	for _, resourceUUID := range resourceUUIDs {
		if err := st.deleteResource(ctx, tx, resourceUUID); err != nil {
			return errors.Errorf("deleting charm resource: %w", err)
		}
	}

	for _, table := range []string{
		"charm_config",
		"charm_manifest_base",
		"charm_action",
		"charm_container_mount",
		"charm_container",
		"charm_term",
		"charm_resource",
		"charm_device",
		"charm_storage_property",
		"charm_storage",
		"charm_tag",
		"charm_category",
		"charm_extra_binding",
		"charm_relation",
		"charm_hash",
		"charm_metadata",
		"charm_download_info",
	} {
		deleteApplicationReference := fmt.Sprintf(`DELETE FROM %s WHERE charm_uuid = $entityUUID.uuid`, table)
		deleteApplicationReferenceStmt, err := st.Prepare(deleteApplicationReference, charmUUID)
		if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, deleteApplicationReferenceStmt, charmUUID).Run(); err != nil {
			return errors.Errorf("deleting reference to charm in %s: %w", table, err)
		}
	}

	getObjectStoreEntryStmt, err := st.Prepare(`
SELECT object_store_uuid AS &objectStoreUUID.uuid
FROM charm
WHERE uuid = $entityUUID.uuid
`, objectStoreUUID{}, charmUUID)
	if err != nil {
		return errors.Capture(err)
	}

	// retrieve the object store UUID to clean up later
	var objectStoreUUID objectStoreUUID
	err = tx.Query(ctx, getObjectStoreEntryStmt, charmUUID).Get(&objectStoreUUID)
	if err != nil {
		return errors.Capture(err)
	}

	// Delete the charm itself.
	deleteCharmStmt, err := st.Prepare(`
DELETE FROM charm
WHERE uuid = $entityUUID.uuid`, charmUUID)
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, deleteCharmStmt, charmUUID).Run(); err != nil {
		return errors.Errorf("deleting charm %q: %w", cUUID, err)
	}

	if objectStoreUUID.UUID.Valid {
		if err := st.deleteFromObjectStore(ctx, tx, objectStoreUUID.UUID.V); err != nil {
			return errors.Errorf("deleting charm object store entry: %w", err)
		}
	}

	return nil
}

func (st *State) deleteResource(ctx context.Context, tx *sqlair.TX, resourceUUID entityUUID) error {
	getObjectStoreEntryStmt, err := st.Prepare(`
SELECT store_uuid AS &entityUUID.uuid
FROM resource_file_store
WHERE resource_uuid = $entityUUID.uuid 
`, resourceUUID)
	if err != nil {
		return errors.Capture(err)
	}

	var objectStoreUUIDs []entityUUID
	err = tx.Query(ctx, getObjectStoreEntryStmt, resourceUUID).GetAll(&objectStoreUUIDs)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("getting object store UUIDs for resource: %w", err)
	}

	for _, table := range []string{
		"pending_application_resource",
		"resource_retrieved_by",
		"resource_file_store",
		"resource_image_store",
	} {
		deleteResourceReference := fmt.Sprintf(`DELETE FROM %s WHERE resource_uuid = $entityUUID.uuid`, table)
		deleteResourceReferenceStmt, err := st.Prepare(deleteResourceReference, resourceUUID)
		if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, deleteResourceReferenceStmt, resourceUUID).Run(); err != nil {
			return errors.Errorf("deleting reference to resource in %s: %w", table, err)
		}
	}

	for _, objectStoreUUID := range objectStoreUUIDs {
		if err := st.deleteFromObjectStore(ctx, tx, objectStoreUUID.UUID); err != nil {
			return errors.Errorf("deleting resource %q object store entry: %w", resourceUUID.UUID, err)
		}
	}

	deleteResourceStmt, err := st.Prepare(`
DELETE FROM resource
WHERE uuid = $entityUUID.uuid
`, resourceUUID)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, deleteResourceStmt, resourceUUID).Run(); err != nil {
		return errors.Errorf("deleting resource %q: %w", resourceUUID, err)
	}

	return nil
}

func (st *State) deleteFromObjectStore(ctx context.Context, tx *sqlair.TX, objectStoreUUID string) error {
	ident := entityUUID{UUID: objectStoreUUID}

	deleteObjectStorePathStmt, err := st.Prepare(`
	DELETE FROM object_store_metadata_path
	WHERE metadata_uuid = $entityUUID.uuid
	`, ident)
	if err != nil {
		return errors.Capture(err)
	}

	// Delete the associated object store entry.
	deleteObjectStoreStmt, err := st.Prepare(`
DELETE FROM object_store_metadata
WHERE uuid = $entityUUID.uuid
`, ident)
	if err != nil {
		return errors.Errorf("preparing object store delete: %w", err)
	}

	if err := tx.Query(ctx, deleteObjectStorePathStmt, ident).Run(); err != nil {
		return errors.Errorf("deleting charm object store path: %w", err)
	}

	if err := tx.Query(ctx, deleteObjectStoreStmt, ident).Run(); err != nil {
		return errors.Errorf("deleting charm object store entry: %w", err)
	}

	return nil
}

func (st *State) getApplicationLife(ctx context.Context, tx *sqlair.TX, aUUID string) (life.Life, error) {
	var applicationLife entityLife
	applicationUUID := entityUUID{UUID: aUUID}

	stmt, err := st.Prepare(`
SELECT &entityLife.life_id
FROM   application
WHERE  uuid = $entityUUID.uuid;`, applicationLife, applicationUUID)
	if err != nil {
		return -1, errors.Errorf("preparing application life query: %w", err)
	}

	err = tx.Query(ctx, stmt, applicationUUID).Get(&applicationLife)
	if errors.Is(err, sqlair.ErrNoRows) {
		return -1, applicationerrors.ApplicationNotFound
	} else if err != nil {
		return -1, errors.Errorf("running application life query: %w", err)
	}

	return applicationLife.Life, errors.Capture(err)
}
