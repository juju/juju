// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"fmt"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/domain/life"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/removal"
	removalerrors "github.com/juju/juju/domain/removal/errors"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// ModelExists returns true if a model exists with the input UUID.
// This uses the *model* database table, not the *controller* model table.
// The model table with one row should exist until the model is removed.
func (st *State) ModelExists(ctx context.Context, mUUID string) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	modelUUID := entityUUID{UUID: mUUID}
	existsStmt, err := st.Prepare(`
SELECT uuid AS &entityUUID.uuid
FROM   model
WHERE  uuid = $entityUUID.uuid`, modelUUID)
	if err != nil {
		return false, errors.Errorf("preparing model exists query: %w", err)
	}

	var modelExists bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, existsStmt, modelUUID).Get(&modelUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("running model exists query: %w", err)
		}

		modelExists = true
		return nil
	})

	return modelExists, errors.Capture(err)
}

// EnsureModelNotAliveCascade ensures that there is no model identified
// by the input model UUID, that is still alive.
func (st *State) EnsureModelNotAliveCascade(ctx context.Context, modelUUID string, force bool) (removal.ModelArtifacts, error) {
	db, err := st.DB()
	if err != nil {
		return removal.ModelArtifacts{}, errors.Capture(err)
	}

	eUUID := entityUUID{
		UUID: modelUUID,
	}

	// Cascading of the dying state of the model means that we will also set the entities to dying all of the
	// following:
	// - All units in the model.
	// - All applications in the model.
	// - All relations in the model.
	// - All machines in the model.
	selectUnits, err := st.Prepare(`SELECT uuid AS &entityUUID.* FROM unit WHERE life_id = 0`, eUUID)
	if err != nil {
		return removal.ModelArtifacts{}, errors.Errorf("preparing select units query: %w", err)
	}
	selectApplications, err := st.Prepare(`SELECT uuid AS &entityUUID.* FROM application WHERE life_id = 0`, eUUID)
	if err != nil {
		return removal.ModelArtifacts{}, errors.Errorf("preparing select applications query: %w", err)
	}
	selectRelations, err := st.Prepare(`SELECT uuid AS &entityUUID.* FROM relation WHERE life_id = 0`, eUUID)
	if err != nil {
		return removal.ModelArtifacts{}, errors.Errorf("preparing select relations query: %w", err)
	}
	selectMachines, err := st.Prepare(`SELECT uuid AS &entityUUID.* FROM machine WHERE life_id = 0`, eUUID)
	if err != nil {
		return removal.ModelArtifacts{}, errors.Errorf("preparing select machines query: %w", err)
	}

	updateModelLife, err := st.Prepare(`UPDATE model_life SET life_id = 1 WHERE model_uuid = $entityUUID.uuid AND life_id = 0`, eUUID)
	if err != nil {
		return removal.ModelArtifacts{}, errors.Errorf("preparing update model life query: %w", err)
	}
	updateUnits, err := st.Prepare(`UPDATE unit SET life_id = 1 WHERE uuid IN ($uuids[:]) AND life_id = 0`, uuids{})
	if err != nil {
		return removal.ModelArtifacts{}, errors.Errorf("preparing update units query: %w", err)
	}
	updateApplications, err := st.Prepare(`UPDATE application SET life_id = 1 WHERE uuid IN ($uuids[:]) AND life_id = 0`, uuids{})
	if err != nil {
		return removal.ModelArtifacts{}, errors.Errorf("preparing update applications query: %w", err)
	}
	updateRelations, err := st.Prepare(`UPDATE relation SET life_id = 1 WHERE uuid IN ($uuids[:]) AND life_id = 0`, uuids{})
	if err != nil {
		return removal.ModelArtifacts{}, errors.Errorf("preparing update relations query: %w", err)
	}
	updateMachines, err := st.Prepare(`UPDATE machine SET life_id = 1 WHERE uuid IN ($uuids[:]) AND life_id = 0`, uuids{})
	if err != nil {
		return removal.ModelArtifacts{}, errors.Errorf("preparing update machines query: %w", err)
	}
	updateMachineInstances, err := st.Prepare(`UPDATE machine_cloud_instance SET life_id = 1 WHERE machine_uuid IN ($uuids[:]) AND life_id = 0`, uuids{})
	if err != nil {
		return removal.ModelArtifacts{}, errors.Errorf("preparing update machine instances query: %w", err)
	}

	var (
		units, apps, relations, machines []entityUUID
		artifacts                        removal.ModelArtifacts
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Update the model life to dying.
		if err := tx.Query(ctx, updateModelLife, eUUID).Run(); err != nil {
			return errors.Errorf("setting model life to dying: %w", err)
		}

		if err := tx.Query(ctx, selectUnits).GetAll(&units); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("selecting units: %w", err)
		}
		if err := tx.Query(ctx, selectApplications).GetAll(&apps); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("selecting applications: %w", err)
		}
		if err := tx.Query(ctx, selectRelations).GetAll(&relations); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("selecting relations: %w", err)
		}
		if err := tx.Query(ctx, selectMachines).GetAll(&machines); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("selecting machines: %w", err)
		}

		// Update the life of each entity to dying.

		if len(units) > 0 {
			u := transform.Slice(units, func(e entityUUID) string { return e.UUID })
			if err := tx.Query(ctx, updateUnits, uuids(u)).Run(); err != nil {
				return errors.Errorf("updating units: %w", err)
			}
		}
		if len(apps) > 0 {
			u := transform.Slice(apps, func(e entityUUID) string { return e.UUID })
			if err := tx.Query(ctx, updateApplications, uuids(u)).Run(); err != nil {
				return errors.Errorf("updating applications: %w", err)
			}
		}
		if len(relations) > 0 {
			u := transform.Slice(relations, func(e entityUUID) string { return e.UUID })
			if err := tx.Query(ctx, updateRelations, uuids(u)).Run(); err != nil {
				return errors.Errorf("updating relations: %w", err)
			}
		}
		if len(machines) > 0 {
			u := transform.Slice(machines, func(e entityUUID) string { return e.UUID })
			if err := tx.Query(ctx, updateMachines, uuids(u)).Run(); err != nil {
				return errors.Errorf("updating machines: %w", err)
			}
			if err := tx.Query(ctx, updateMachineInstances, uuids(u)).Run(); err != nil {
				return errors.Errorf("updating machine instances: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return removal.ModelArtifacts{}, errors.Errorf("ensuring model %q is not alive: %w", modelUUID, err)
	}

	artifacts.UnitUUIDs = make([]string, len(units))
	for i, u := range units {
		artifacts.UnitUUIDs[i] = u.UUID
	}
	artifacts.ApplicationUUIDs = make([]string, len(apps))
	for i, a := range apps {
		artifacts.ApplicationUUIDs[i] = a.UUID
	}
	artifacts.RelationUUIDs = make([]string, len(relations))
	for i, r := range relations {
		artifacts.RelationUUIDs[i] = r.UUID
	}
	artifacts.MachineUUIDs = make([]string, len(machines))
	for i, m := range machines {
		artifacts.MachineUUIDs[i] = m.UUID
	}

	return artifacts, nil
}

// ModelScheduleRemoval schedules the removal job for a model.
//
// We don't care if the model does not exist at this point because:
// - it should have been validated prior to calling this method,
// - the removal job executor will handle that fact.
func (st *State) ModelScheduleRemoval(
	ctx context.Context,
	removalUUID, modelUUID string,
	force bool, when time.Time,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	removalDoc := removalJob{
		UUID:          removalUUID,
		RemovalTypeID: 4,
		EntityUUID:    modelUUID,
		Force:         force,
		ScheduledFor:  when,
	}

	stmt, err := st.Prepare("INSERT INTO removal (*) VALUES ($removalJob.*)", removalDoc)
	if err != nil {
		return errors.Errorf("preparing model  removal: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, removalDoc).Run(); err != nil {
			return errors.Errorf("scheduling model  removal: %w", err)
		}
		return nil
	}))
}

// GetModelLife retrieves the life state of a model.
func (st *State) GetModelLife(ctx context.Context, mUUID string) (life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return -1, errors.Capture(err)
	}

	var life life.Life
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		life, err = st.getModelLife(ctx, tx, mUUID)

		return errors.Capture(err)
	})

	return life, errors.Capture(err)
}

// MarkModelAsDead marks the model with the input UUID as dead.
// If there are model dependents, then this will return an error.
func (st *State) MarkModelAsDead(ctx context.Context, mUUID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	modelUUID := entityUUID{UUID: mUUID}
	updateStmt, err := st.Prepare(`
UPDATE model_life
SET    life_id = 2
WHERE  model_uuid = $entityUUID.uuid
AND    life_id = 1`, modelUUID)
	if err != nil {
		return errors.Errorf("preparing model life update: %w", err)
	}
	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if l, err := st.getModelLife(ctx, tx, mUUID); err != nil {
			return errors.Errorf("getting model life: %w", err)
		} else if l == life.Dead {
			return nil
		} else if l == life.Alive {
			return removalerrors.EntityStillAlive
		}

		err = st.checkNoModelDependents(ctx, tx)
		if err != nil {
			return errors.Capture(err)
		}

		err := tx.Query(ctx, updateStmt, modelUUID).Run()
		if err != nil {
			return errors.Errorf("marking model as dead: %w", err)
		}

		return nil
	}))
}

// DeleteModelArtifacts deletes all artifacts associated with a model.
func (st *State) DeleteModelArtifacts(ctx context.Context, mUUID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	modelUUIDParam := entityUUID{UUID: mUUID}

	// Prepare query for deleting model row.
	deleteModel := `
DELETE FROM model 
WHERE uuid = $entityUUID.uuid;
`
	deleteModelStmt, err := st.Prepare(deleteModel, modelUUIDParam)
	if err != nil {
		return errors.Capture(err)
	}

	// Once we get to this point, the model is hosed. We don't expect the
	// model to be in use.
	modelTriggerStmt, err := st.Prepare(`DROP TRIGGER IF EXISTS trg_model_immutable_delete;`)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		mLife, err := st.getModelLife(ctx, tx, modelUUIDParam.UUID)
		if err != nil {
			return errors.Errorf("getting model life: %w", err)
		} else if mLife == life.Alive {
			return errors.Errorf("cannot delete model %q, model is still alive", modelUUIDParam.UUID).
				Add(removalerrors.EntityStillAlive)
		} else if mLife == life.Dying {
			return errors.Errorf("waiting for model to be removed before deletion").
				Add(removalerrors.RemovalJobIncomplete)
		}

		err = st.checkNoModelDependents(ctx, tx)
		if err != nil {
			return errors.Errorf("checking for dependents: %w", err).Add(removalerrors.RemovalJobIncomplete)
		}

		// Remove all basic model data associated with the model.
		if err := st.removeBasicModelData(ctx, tx, modelUUIDParam.UUID); err != nil {
			return errors.Errorf("removing basic model data: %w", err)
		}

		if err := tx.Query(ctx, modelTriggerStmt).Run(); err != nil && !internaldatabase.IsExtendedErrorCode(err) {
			return errors.Errorf("deleting model trigger %w", err)
		}

		if err := tx.Query(ctx, deleteModelStmt, modelUUIDParam).Run(); err != nil {
			return errors.Errorf("deleting model: %w", err)
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("deleting model: %w", err)
	}
	return nil
}

func (st *State) getModelLife(ctx context.Context, tx *sqlair.TX, mUUID string) (life.Life, error) {
	var model entityLife
	modelUUID := entityUUID{UUID: mUUID}

	stmt, err := st.Prepare(`
SELECT &entityLife.life_id
FROM   model_life
WHERE  model_uuid = $entityUUID.uuid;`, model, modelUUID)
	if err != nil {
		return -1, errors.Errorf("preparing model life query: %w", err)
	}

	err = tx.Query(ctx, stmt, modelUUID).Get(&model)
	if errors.Is(err, sqlair.ErrNoRows) {
		return -1, modelerrors.NotFound
	} else if err != nil {
		return -1, errors.Errorf("running model life query: %w", err)
	}

	return life.Life(model.Life), nil
}

func (st *State) checkNoModelDependents(ctx context.Context, tx *sqlair.TX) error {
	var count count

	// We only care about applications and machines (for IAAS models). We assume
	// that all dependant entities for each of these entities are already
	// removed (units for applications etc). So if a model has no applications
	// or machines, then it is empty.

	applicationsStmt, err := st.Prepare(`SELECT COUNT(*) AS &count.count FROM application`, count)
	if err != nil {
		return errors.Errorf("preparing application count query: %w", err)
	}

	err = tx.Query(ctx, applicationsStmt).Get(&count)
	if err != nil {
		return errors.Errorf("getting application count: %w", err)
	} else if count.Count > 0 {
		return errors.Errorf("still %d application still exist it", count.Count).Add(removalerrors.EntityStillAlive)
	}

	machinesStmt, err := st.Prepare(`SELECT COUNT(*) AS &count.count FROM machine`, count)
	if err != nil {
		return errors.Errorf("preparing machine count query: %w", err)
	}

	err = tx.Query(ctx, machinesStmt).Get(&count)
	if err != nil {
		return errors.Errorf("getting machine count: %w", err)
	} else if count.Count > 0 {
		return errors.Errorf("still %d machines still exist", count.Count).Add(removalerrors.EntityStillAlive)
	}

	return nil
}

func (st *State) removeBasicModelData(ctx context.Context, tx *sqlair.TX, mUUID string) error {
	modelUUIDRec := entityUUID{UUID: mUUID}

	tables := []string{
		"model_life",
		"model_constraint",
		"model_agent",
	}

	for _, table := range tables {
		query := fmt.Sprintf("DELETE FROM %s WHERE model_uuid = $entityUUID.uuid", table)
		stmt, err := st.Prepare(query, modelUUIDRec)
		if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, stmt, modelUUIDRec).Run(); err != nil {
			return errors.Errorf("deleting reference to model in table %q: %w", table, err)
		}
	}
	return nil
}
