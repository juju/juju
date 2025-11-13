// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	"github.com/juju/juju/domain/removal/internal"
	"github.com/juju/juju/internal/errors"
)

// ApplicationExists returns true if a application exists with the input UUID.
// Returns false (with an error) if the application is a remote application
func (st *State) ApplicationExists(ctx context.Context, aUUID string) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	applicationUUID := entityUUID{UUID: aUUID}
	existsStmt, err := st.Prepare(`
SELECT &entityUUID.uuid
FROM   application
WHERE  uuid = $entityUUID.uuid`, applicationUUID)
	if err != nil {
		return false, errors.Errorf("preparing application exists query: %w", err)
	}

	isRemoteOffererStmt, err := st.Prepare(`
SELECT COUNT(*) AS &count.count
FROM   application_remote_offerer
WHERE  application_uuid = $entityUUID.uuid`, count{}, applicationUUID)
	if err != nil {
		return false, errors.Errorf("preparing application remote offerer exists query: %w", err)
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

		var countRemoteOfferer count
		err = tx.Query(ctx, isRemoteOffererStmt, applicationUUID).Get(&countRemoteOfferer)
		if err != nil {
			return errors.Errorf("running application remote offerer exists query: %w", err)
		}

		if countRemoteOfferer.Count > 0 {
			return errors.Errorf("application %q is a remote application", aUUID).
				Add(removalerrors.ApplicationIsRemoteOfferer)
		}

		// TODO: Handle remote application consumers

		return nil
	})
	if err != nil {
		return false, errors.Capture(err)
	}

	return applicationExists, errors.Capture(err)
}

// EnsureApplicationNotAliveCascade ensures that there is no application
// identified by the input application UUID, that is still alive. If the
// application has units, they are also guaranteed to be no longer alive,
// cascading. The affected unit UUIDs are returned. If the units are also
// the last ones on their machines, it will cascade and the machines are
// also set to dying. The affected machine UUIDs are returned.
func (st *State) EnsureApplicationNotAliveCascade(
	ctx context.Context, aUUID string, destroyStorage bool, force bool,
) (internal.CascadedApplicationLives, error) {
	var res internal.CascadedApplicationLives

	db, err := st.DB(ctx)
	if err != nil {
		return res, errors.Capture(err)
	}

	applicationUUID := entityUUID{UUID: aUUID}
	checkOfferConnectionsStmt, err := st.Prepare(`
SELECT COUNT(*) AS &count.count
FROM   offer_connection AS oc
JOIN   offer AS o ON oc.offer_uuid = o.uuid
JOIN   offer_endpoint AS oe ON o.uuid = oe.offer_uuid
JOIN   application_endpoint AS ae ON oe.endpoint_uuid = ae.uuid
WHERE  ae.application_uuid = $entityUUID.uuid
	`, count{}, applicationUUID)
	if err != nil {
		return res, errors.Errorf("preparing offer connections query: %w", err)
	}

	updateApplicationStmt, err := st.Prepare(`
UPDATE application
SET    life_id = 1
WHERE  uuid = $entityUUID.uuid
AND    life_id = 0`, applicationUUID)
	if err != nil {
		return res, errors.Errorf("preparing application life update: %w", err)
	}

	// Also ensure that any other entities that are associated with the
	// application are also set to dying. This has to be done in a single
	// transaction because we want to ensure that the application is not
	// alive, and that no units are alive at the same time. Preventing any
	// races.
	selectRelationUUIDsStmt, err := st.Prepare(`
SELECT &entityUUID.uuid
FROM   v_relation_endpoint AS re
JOIN   relation AS r ON re.relation_uuid = r.uuid
WHERE  r.life_id = 0
AND    re.application_uuid = $entityUUID.uuid
`, applicationUUID)
	if err != nil {
		return res, errors.Errorf("preparing relation uuids query: %w", err)
	}

	updateRelationStmt, err := st.Prepare(`
UPDATE relation
SET    life_id = 1
WHERE  uuid IN ($uuids[:])
AND    life_id = 0`, uuids{})
	if err != nil {
		return res, errors.Errorf("preparing relation life update: %w", err)
	}

	selectUnitUUIDsStmt, err := st.Prepare(`
SELECT &entityUUID.uuid
FROM   unit
WHERE  application_uuid = $entityUUID.uuid
AND    life_id = 0`, applicationUUID)
	if err != nil {
		return res, errors.Errorf("preparing unit uuids query: %w", err)
	}

	if err := errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if !force {
			var count count
			if err := tx.Query(ctx, checkOfferConnectionsStmt, applicationUUID).Get(&count); err != nil {
				return errors.Errorf("checking offer connections: %w", err)
			} else if count.Count > 0 {
				return errors.Errorf("cannot remove application %q, it has %d offer connection(s)", aUUID, count.Count).
					Add(removalerrors.ApplicationHasOfferConnections).
					Add(removalerrors.ForceRequired)
			}
		}

		if err := tx.Query(ctx, updateApplicationStmt, applicationUUID).Run(); err != nil {
			return errors.Errorf("advancing application life: %w", err)
		}

		var relationUUIDs []entityUUID
		if err := tx.Query(
			ctx, selectRelationUUIDsStmt, applicationUUID,
		).GetAll(&relationUUIDs); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("selecting relation UUIDs: %w", err)
		}
		res.RelationUUIDs = transform.Slice(relationUUIDs, func(e entityUUID) string { return e.UUID })

		if len(res.RelationUUIDs) > 0 {
			if err := tx.Query(ctx, updateRelationStmt, uuids(res.RelationUUIDs)).Run(); err != nil {
				return errors.Errorf("advancing relation life: %w", err)
			}
		}

		var unitUUIDsRec []entityUUID
		if err := tx.Query(
			ctx, selectUnitUUIDsStmt, applicationUUID,
		).GetAll(&unitUUIDsRec); errors.Is(err, sqlair.ErrNoRows) {
			// If there are no units associated with the application,
			// we can just return nil, as there is nothing to update.
			return nil
		} else if err != nil {
			return errors.Errorf("selecting associated application unit lives: %w", err)
		}

		const checkEmptyMachine = true
		res.UnitUUIDs = transform.Slice(unitUUIDsRec, func(e entityUUID) string { return e.UUID })
		for _, u := range res.UnitUUIDs {
			cascaded, err := st.ensureUnitNotAliveCascade(ctx, tx, u, checkEmptyMachine, destroyStorage)
			if err != nil {
				return errors.Errorf("cascading unit %q life advancement: %w", u, err)
			}

			if cascaded.MachineUUID != nil {
				res.MachineUUIDs = append(res.MachineUUIDs, *cascaded.MachineUUID)
			}

			res.CascadedStorageLives = res.CascadedStorageLives.Merge(cascaded.CascadedStorageLives)
		}

		return nil
	})); err != nil {
		return res, errors.Capture(err)
	}

	return res, nil
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
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	removalRec := removalJob{
		UUID:          removalUUID,
		RemovalTypeID: uint64(removal.ApplicationJob),
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
	db, err := st.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}

	var l life.Life
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		l, err = st.getApplicationLife(ctx, tx, aUUID)
		return err
	})
	if err != nil {
		return -1, errors.Capture(err)
	}

	return l, nil
}

// DeleteApplication removes a application from the database completely.
func (st *State) DeleteApplication(ctx context.Context, aUUID string, force bool) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	applicationUUID := entityUUID{UUID: aUUID}

	unitsStmt, err := st.Prepare(`
SELECT COUNT(*) AS &count.count
FROM unit
WHERE application_uuid = $entityUUID.uuid
`, count{}, applicationUUID)
	if err != nil {
		return errors.Errorf("preparing application unit count query: %w", err)
	}

	relationsStmt, err := st.Prepare(`
SELECT COUNT(*) AS &count.count
FROM v_relation_endpoint
WHERE application_uuid = $entityUUID.uuid
`, count{}, applicationUUID)
	if err != nil {
		return errors.Errorf("preparing application relation count query: %w", err)
	}

	getOffersStmt, err := st.Prepare(`
SELECT oe.offer_uuid AS &entityUUID.uuid
FROM offer_endpoint AS oe
JOIN application_endpoint AS ae ON oe.endpoint_uuid = ae.uuid
WHERE ae.application_uuid = $entityUUID.uuid
`, entityUUID{})
	if err != nil {
		return errors.Errorf("preparing application offers query: %w", err)
	}

	deleteApplicationStmt, err := st.Prepare(`
DELETE FROM application
WHERE  uuid = $entityUUID.uuid;`, applicationUUID)
	if err != nil {
		return errors.Errorf("preparing application delete: %w", err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
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

		// If we're not forcing, we need to ensure that there are no units
		// or relations associated with the application.
		if !force {
			// Check that there are no units.
			var numUnits count
			err = tx.Query(ctx, unitsStmt, applicationUUID).Get(&numUnits)
			if err != nil {
				return errors.Errorf("querying application units: %w", err)
			} else if numUnits := numUnits.Count; numUnits > 0 {
				// It is required that all units have been completely removed
				// before the application can be removed.
				return errors.Errorf("cannot delete application as it still has %d unit(s)", numUnits).
					Add(applicationerrors.ApplicationHasUnits).
					Add(removalerrors.RemovalJobIncomplete)
			}

			var numRelations count
			err = tx.Query(ctx, relationsStmt, applicationUUID).Get(&numRelations)
			if err != nil {
				return errors.Errorf("querying application relations: %w", err)
			} else if numRelations := numRelations.Count; numRelations > 0 {
				// It is required that all relations have been completely removed
				// before the application can be removed.
				return errors.Errorf("cannot delete application as it still has %d relation(s)", numRelations).
					Add(applicationerrors.ApplicationHasRelations).
					Add(removalerrors.RemovalJobIncomplete)
			}
		}

		var offers []entityUUID
		if err := tx.Query(ctx, getOffersStmt, applicationUUID).GetAll(&offers); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting offers: %w", err)
		}

		for _, offer := range offers {
			// We don't need to worry about whether the offer has connections here.
			// Either we are using force, meaning we don't care, or we are not forcing,
			// meaning we checked there are no connections before setting to dying.
			if err := st.deleteOffer(ctx, tx, offer, force); err != nil {
				return errors.Errorf("deleting offer %q: %w", offer.UUID, err)
			}
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

		if err := st.deleteApplicationResources(ctx, tx, aUUID); err != nil {
			return errors.Errorf("deleting application resources: %w", err)
		}

		if err := st.deleteApplicationUnits(ctx, tx, aUUID, force); err != nil {
			return errors.Errorf("deleting application units: %w", err)
		}

		if err := st.deleteSimpleApplicationReferences(ctx, tx, aUUID); err != nil {
			return errors.Errorf("deleting simple application references: %w", err)
		}

		if force {
			if err := st.deleteSecretApplicationOwner(ctx, tx, aUUID); err != nil {
				return errors.Errorf("deleting simple application references: %w", err)
			}
		}

		// Delete the application itself.
		if err := tx.Query(ctx, deleteApplicationStmt, applicationUUID).Run(); err != nil {
			return errors.Errorf("deleting application: %w", err)
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("delete application transaction: %w", err)
	}

	return nil
}

// DeleteCharmIfUnused deletes the charm with the input UUID if it is not used
// by any application or unit.
func (st *State) DeleteCharmIfUnused(ctx context.Context, charmUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.deleteCharmIfUnusedByUUID(ctx, tx, charmUUID)
	}))
}

// DeleteOrphanedResources deletes any resources associated with the input
// charm UUID that are no longer referenced by any application.
func (st *State) DeleteOrphanedResources(ctx context.Context, charmUUID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.deleteOrphanedResources(ctx, tx, charmUUID)
	}))
}

func (st *State) deleteSimpleApplicationReferences(ctx context.Context, tx *sqlair.TX, aUUID string) error {
	app := entityUUID{UUID: aUUID}

	for _, table := range []string{
		"DELETE FROM application_channel WHERE application_uuid = $entityUUID.uuid",
		"DELETE FROM application_platform WHERE application_uuid = $entityUUID.uuid",
		"DELETE FROM application_scale WHERE application_uuid = $entityUUID.uuid",
		"DELETE FROM application_config WHERE application_uuid = $entityUUID.uuid",
		"DELETE FROM application_config_hash WHERE application_uuid = $entityUUID.uuid",
		"DELETE FROM application_constraint WHERE application_uuid = $entityUUID.uuid",
		"DELETE FROM application_controller WHERE application_uuid = $entityUUID.uuid",
		"DELETE FROM application_setting WHERE application_uuid = $entityUUID.uuid",
		"DELETE FROM application_exposed_endpoint_space WHERE application_uuid = $entityUUID.uuid",
		"DELETE FROM application_exposed_endpoint_cidr WHERE application_uuid = $entityUUID.uuid",
		"DELETE FROM application_endpoint WHERE application_uuid = $entityUUID.uuid",
		"DELETE FROM application_extra_endpoint WHERE application_uuid = $entityUUID.uuid",
		"DELETE FROM application_storage_directive WHERE application_uuid = $entityUUID.uuid",
		"DELETE FROM application_status WHERE application_uuid = $entityUUID.uuid",
		"DELETE FROM application_agent WHERE application_uuid = $entityUUID.uuid",
		"DELETE FROM application_workload_version WHERE application_uuid = $entityUUID.uuid",
		"DELETE FROM device_constraint WHERE application_uuid = $entityUUID.uuid",
	} {
		deleteApplicationReferenceStmt, err := st.Prepare(table, app)
		if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, deleteApplicationReferenceStmt, app).Run(); err != nil {
			return errors.Errorf("deleting reference to application in %s: %w", table, err)
		}
	}
	return nil
}

func (st *State) deleteSecretApplicationOwner(ctx context.Context, tx *sqlair.TX, aUUID string) error {
	app := entityUUID{UUID: aUUID}

	deleteSecretStmt, err := st.Prepare(`
DELETE FROM secret_application_owner
WHERE application_uuid = $entityUUID.uuid
`, app)
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, deleteSecretStmt, app).Run(); err != nil {
		return errors.Errorf("deleting secret application reference: %w", err)
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
WITH constraint_uuids AS (
	SELECT uuid
	FROM device_constraint
	WHERE application_uuid = $entityUUID.uuid
)
DELETE FROM device_constraint_attribute
WHERE device_constraint_uuid IN constraint_uuids
`, appID)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, deleteDeviceConstraintAttributesStmt, appID).Run(); err != nil {
		return errors.Errorf("deleting device constraint attributes: %w", err)
	}
	return nil
}

func (st *State) deleteApplicationResources(ctx context.Context, tx *sqlair.TX, aUUID string) error {
	appID := entityUUID{UUID: aUUID}

	getApplicationResourcesStmt, err := st.Prepare(`
SELECT resource_uuid AS &entityUUID.uuid
FROM application_resource
WHERE application_uuid = $entityUUID.uuid
`, appID)
	if err != nil {
		return errors.Capture(err)
	}

	deleteApplicationResourceStmt, err := st.Prepare(`
DELETE FROM application_resource
WHERE application_uuid = $entityUUID.uuid
`, appID)
	if err != nil {
		return errors.Capture(err)
	}

	var resourceUUIDs = []entityUUID{}
	err = tx.Query(ctx, getApplicationResourcesStmt, appID).GetAll(&resourceUUIDs)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("getting charm resources: %w", err)
	}

	if err := tx.Query(ctx, deleteApplicationResourceStmt, appID).Run(); err != nil {
		return errors.Errorf("deleting application resource reference: %w", err)
	}

	if err := st.deleteResources(ctx, tx, resourceUUIDs); err != nil {
		return errors.Errorf("deleting charm resources: %w", err)
	}
	return nil
}

// deleteOrphanedResources deletes resources that are associated to a charm_uuid,
// and are not associated with any application_resource.
func (st *State) deleteOrphanedResources(ctx context.Context, tx *sqlair.TX, charmUUID string) error {
	// If the charm UUID is empty, we can skip the deletion.
	if charmUUID == "" {
		return nil
	}
	uuid := entityUUID{UUID: charmUUID}

	resourceIDs := []entityUUID{}
	stmt, err := st.Prepare(`
WITH
application_resources AS (
	SELECT resource_uuid
	FROM application_resource
)
SELECT uuid AS &entityUUID.uuid
FROM resource
WHERE charm_uuid = $entityUUID.uuid
AND uuid NOT IN application_resources
`, entityUUID{})
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, uuid).GetAll(&resourceIDs); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("get dangling resource ids: %w", err)
	}

	if len(resourceIDs) == 0 {
		return nil
	}

	err = st.deleteResources(ctx, tx, resourceIDs)
	if err != nil {
		return errors.Errorf("deleting dangling resources: %w", err)
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

func (st *State) deleteApplicationUnits(ctx context.Context, tx *sqlair.TX, aUUID string, force bool) error {
	appID := entityUUID{UUID: aUUID}

	getUnitUUIDsStmt, err := st.Prepare(`
SELECT uuid AS &entityUUID.uuid
FROM unit
WHERE application_uuid = $entityUUID.uuid
`, entityUUID{})

	if err != nil {
		return errors.Capture(err)
	}

	var unitUUIDs []entityUUID
	if err := tx.Query(ctx, getUnitUUIDsStmt, appID).GetAll(&unitUUIDs); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("getting application unit UUIDs: %w", err)
	}

	for _, unit := range unitUUIDs {
		if err := st.deleteUnit(ctx, tx, unit.UUID, force); err != nil {
			return errors.Errorf("deleting unit %q: %w", unit.UUID, err)
		}
	}
	return nil
}

// GetCharmForApplication returns the charm UUID for the application with
// the input application UUID.
// It returns an empty string if there is no charm associated with the application.
func (st *State) GetCharmForApplication(ctx context.Context, appUUID string) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}
	var charmUUID string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		charmUUID, err = st.getCharmUUIDForApplication(ctx, tx, appUUID)
		return err
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	return charmUUID, nil
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

	for _, table := range []string{
		"DELETE FROM charm_config WHERE charm_uuid = $entityUUID.uuid",
		"DELETE FROM charm_manifest_base WHERE charm_uuid = $entityUUID.uuid",
		"DELETE FROM charm_action WHERE charm_uuid = $entityUUID.uuid",
		"DELETE FROM charm_container_mount WHERE charm_uuid = $entityUUID.uuid",
		"DELETE FROM charm_container WHERE charm_uuid = $entityUUID.uuid",
		"DELETE FROM charm_term WHERE charm_uuid = $entityUUID.uuid",
		"DELETE FROM charm_resource WHERE charm_uuid = $entityUUID.uuid",
		"DELETE FROM charm_device WHERE charm_uuid = $entityUUID.uuid",
		"DELETE FROM charm_storage_property WHERE charm_uuid = $entityUUID.uuid",
		"DELETE FROM charm_storage WHERE charm_uuid = $entityUUID.uuid",
		"DELETE FROM charm_tag WHERE charm_uuid = $entityUUID.uuid",
		"DELETE FROM charm_category WHERE charm_uuid = $entityUUID.uuid",
		"DELETE FROM charm_extra_binding WHERE charm_uuid = $entityUUID.uuid",
		"DELETE FROM charm_relation WHERE charm_uuid = $entityUUID.uuid",
		"DELETE FROM charm_hash WHERE charm_uuid = $entityUUID.uuid",
		"DELETE FROM charm_metadata WHERE charm_uuid = $entityUUID.uuid",
		"DELETE FROM charm_download_info WHERE charm_uuid = $entityUUID.uuid",
	} {
		deleteApplicationReferenceStmt, err := st.Prepare(table, charmUUID)
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
		if err := st.deleteFromObjectStoreIfUnused(ctx, tx, objectStoreUUID.UUID.V); err != nil {
			return errors.Errorf("deleting charm object store entry: %w", err)
		}
	}

	return nil
}

func (st *State) deleteResources(ctx context.Context, tx *sqlair.TX, resources []entityUUID) error {
	type resourceUUIDs []string
	resourceUUIDsRec := resourceUUIDs(transform.Slice(resources, func(e entityUUID) string { return e.UUID }))

	getObjectStoreEntryStmt, err := st.Prepare(`
SELECT store_uuid AS &entityUUID.uuid
FROM resource_file_store
WHERE resource_uuid IN ($resourceUUIDs[:])
`, entityUUID{}, resourceUUIDsRec)
	if err != nil {
		return errors.Capture(err)
	}

	var objectStoreUUIDs []entityUUID
	err = tx.Query(ctx, getObjectStoreEntryStmt, resourceUUIDsRec).GetAll(&objectStoreUUIDs)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("getting object store UUIDs for resource: %w", err)
	}

	for _, table := range []string{
		"DELETE FROM pending_application_resource WHERE resource_uuid IN ($resourceUUIDs[:])",
		"DELETE FROM resource_retrieved_by WHERE resource_uuid IN ($resourceUUIDs[:])",
		"DELETE FROM resource_file_store WHERE resource_uuid IN ($resourceUUIDs[:])",
		"DELETE FROM resource_image_store WHERE resource_uuid IN ($resourceUUIDs[:])",
	} {
		deleteResourceReferenceStmt, err := st.Prepare(table, resourceUUIDsRec)
		if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, deleteResourceReferenceStmt, resourceUUIDsRec).Run(); err != nil {
			return errors.Errorf("deleting reference to resource in %s: %w", table, err)
		}
	}

	for _, objectStoreUUID := range objectStoreUUIDs {
		if err := st.deleteFromObjectStoreIfUnused(ctx, tx, objectStoreUUID.UUID); err != nil {
			return errors.Errorf("deleting object store entry: %w", err)
		}
	}

	deleteResourceStmt, err := st.Prepare(`
DELETE FROM resource
WHERE uuid IN ($resourceUUIDs[:])
`, resourceUUIDsRec)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, deleteResourceStmt, resourceUUIDsRec).Run(); err != nil {
		return errors.Errorf("deleting resources %q: %w", resourceUUIDsRec, err)
	}

	return nil
}

func (st *State) deleteFromObjectStoreIfUnused(ctx context.Context, tx *sqlair.TX, objectStoreUUID string) error {
	uuidCount := entityAssociationCount{UUID: objectStoreUUID}

	resourceStmt, err := st.Prepare(`
SELECT COUNT(*) AS &entityAssociationCount.count
FROM resource_file_store
WHERE store_uuid = $entityAssociationCount.uuid
`, uuidCount)
	if err != nil {
		return errors.Capture(err)
	}

	// It is possible for an underlying object store item to be used by multiple resources.
	// Only delete the object store entry if it is not used by any resources.
	var resourceCount entityAssociationCount
	if err := tx.Query(ctx, resourceStmt, uuidCount).Get(&resourceCount); err != nil {
		return errors.Errorf("running resource usage query: %w", err)
	} else if resourceCount.Count > 0 {
		st.logger.Infof(ctx, "object store %q is still used by %d resource(s), not deleting", objectStoreUUID, resourceCount.Count)
		return nil
	}

	charmStmt, err := st.Prepare(`
SELECT COUNT(*) AS &entityAssociationCount.count
FROM charm
WHERE object_store_uuid = $entityAssociationCount.uuid
`, uuidCount)
	if err != nil {
		return errors.Capture(err)
	}

	// It is also possible for an underlying object store item to be used by a charm.
	// Only delete the object store entry if it is not used by any charms.
	var charmCount entityAssociationCount
	if err := tx.Query(ctx, charmStmt, uuidCount).Get(&charmCount); err != nil {
		return errors.Errorf("running charm usage query: %w", err)
	} else if charmCount.Count > 0 {
		st.logger.Infof(ctx, "object store %q is still used by %d charm(s), not deleting", objectStoreUUID, charmCount.Count)
		return nil
	}

	return st.deleteFromObjectStore(ctx, tx, objectStoreUUID)
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
		return errors.Errorf("deleting object store path: %w", err)
	}

	if err := tx.Query(ctx, deleteObjectStoreStmt, ident).Run(); err != nil {
		return errors.Errorf("deleting object store entry: %w", err)
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

	return life.Life(applicationLife.Life), nil
}
