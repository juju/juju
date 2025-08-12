// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
	networkerrors "github.com/juju/juju/domain/network/errors"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// ModelState represents a type for interacting with the underlying model
// database state.
type ModelState struct {
	*domain.StateBase
	logger logger.Logger
}

// NewModelState returns a new State for interacting with the underlying model
// database state.
func NewState(
	factory database.TxnRunnerFactory,
	logger logger.Logger,
) *ModelState {
	return &ModelState{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// Create inserts all of the information about a newly created model.
func (s *ModelState) Create(ctx context.Context, args model.ModelDetailArgs) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return InsertModelInfo(ctx, args, s, tx)
	})
}

// Delete deletes a model.
func (s *ModelState) Delete(ctx context.Context, uuid coremodel.UUID) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	mUUID := dbUUID{UUID: uuid.String()}

	modelStmt, err := s.Prepare(`DELETE FROM model WHERE uuid = $dbUUID.uuid;`, mUUID)
	if err != nil {
		return errors.Capture(err)
	}

	modelLife, err := s.Prepare(`DELETE FROM model_life WHERE model_uuid = $dbUUID.uuid;`, mUUID)
	if err != nil {
		return errors.Capture(err)
	}

	// Once we get to this point, the model is hosed. We don't expect the
	// model to be in use. The model migration will reinforce the schema once
	// the migration is tried again. Failure to do that will result in the
	// model being deleted unexpected scenarios.
	modelTriggerStmt, err := s.Prepare(`DROP TRIGGER IF EXISTS trg_model_immutable_delete;`)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, modelLife, mUUID).Run(); err != nil {
			return errors.Errorf("deleting model life %q: %w", uuid, err)
		}

		err := tx.Query(ctx, modelTriggerStmt).Run()
		if err != nil && !internaldatabase.IsExtendedErrorCode(err) {
			return errors.Errorf("deleting model trigger %w", err)
		}

		var outcome sqlair.Outcome
		err = tx.Query(ctx, modelStmt, mUUID).Get(&outcome)
		if err != nil {
			return errors.Errorf("deleting readonly model information: %w", err)
		}

		if affected, err := outcome.Result().RowsAffected(); err != nil {
			return errors.Errorf("getting result from removing readonly model information: %w", err)
		} else if affected == 0 {
			return modelerrors.NotFound
		}
		return nil
	})
	if err != nil {
		return errors.Errorf("deleting model %q from model database: %w", uuid, err)
	}

	return nil
}

func getModelUUID(ctx context.Context, preparer domain.Preparer, tx *sqlair.TX) (coremodel.UUID, error) {
	var modelUUID dbUUID
	stmt, err := preparer.Prepare(`SELECT &dbUUID.uuid FROM model;`, modelUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, stmt).Get(&modelUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New("model does not exist").Add(modelerrors.NotFound)
	}
	if err != nil {
		return "", errors.Errorf("getting model uuid: %w", err)
	}

	return coremodel.UUID(modelUUID.UUID), nil
}

// GetModelConstraints returns the currently set constraints for the model.
// The following error types can be expected:
// - [modelerrors.NotFound]: when no model exists to set constraints for.
// - [modelerrors.ConstraintsNotFound]: when no model constraints have been
// set for the model.
func (s *ModelState) GetModelConstraints(
	ctx context.Context,
) (constraints.Constraints, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return constraints.Constraints{}, errors.Capture(err)
	}

	selectTagStmt, err := s.Prepare(
		"SELECT &dbConstraintTag.* FROM v_model_constraint_tag", dbConstraintTag{},
	)
	if err != nil {
		return constraints.Constraints{}, errors.Capture(err)
	}

	selectSpaceStmt, err := s.Prepare(
		"SELECT &dbConstraintSpace.* FROM v_model_constraint_space", dbConstraintSpace{},
	)
	if err != nil {
		return constraints.Constraints{}, errors.Capture(err)
	}

	selectZoneStmt, err := s.Prepare(
		"SELECT &dbConstraintZone.* FROM v_model_constraint_zone", dbConstraintZone{})
	if err != nil {
		return constraints.Constraints{}, errors.Capture(err)
	}

	var (
		cons   dbConstraint
		tags   []dbConstraintTag
		spaces []dbConstraintSpace
		zones  []dbConstraintZone
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		_, err := getModelUUID(ctx, s, tx)
		if err != nil {
			return errors.Errorf("checking if model exists: %w", err)
		}

		cons, err = s.getModelConstraints(ctx, tx)
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, selectTagStmt).GetAll(&tags)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting constraint tags: %w", err)
		}
		err = tx.Query(ctx, selectSpaceStmt).GetAll(&spaces)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting constraint spaces: %w", err)
		}
		err = tx.Query(ctx, selectZoneStmt).GetAll(&zones)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting constraint zones: %w", err)
		}
		return nil
	})
	if err != nil {
		return constraints.Constraints{}, errors.Capture(err)
	}

	return cons.toValue(tags, spaces, zones)
}

// getModelConstraintsUUID returns the constraint uuid that is active for the
// model. If model does not have any constraints then an error satisfying
// [modelerrors.ConstraintsNotFound] is returned.
func getModelConstraintsUUID(
	ctx context.Context,
	preparer domain.Preparer,
	tx *sqlair.TX,
) (string, error) {
	var constraintUUID dbConstraintUUID

	stmt, err := preparer.Prepare(
		"SELECT &dbConstraintUUID.* FROM v_model_constraint",
		constraintUUID,
	)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, stmt).Get(&constraintUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errors.New(
			"no constraints set for model",
		).Add(modelerrors.ConstraintsNotFound)
	} else if err != nil {
		return "", errors.Errorf("getting constraint UUID for model: %w", err)
	}

	return constraintUUID.UUID, nil
}

// getModelConstraints returns the values set in the constraints table for the
// current model. If no constraints are currently set
// for the model an error satisfying [modelerrors.ConstraintsNotFound] will be
// returned.
func (s *ModelState) getModelConstraints(
	ctx context.Context,
	tx *sqlair.TX,
) (dbConstraint, error) {
	var constraint dbConstraint

	stmt, err := s.Prepare("SELECT &dbConstraint.* FROM v_model_constraint", constraint)
	if err != nil {
		return dbConstraint{}, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt).Get(&constraint)
	if errors.Is(err, sql.ErrNoRows) {
		return dbConstraint{}, errors.New(
			"no constraints set for model",
		).Add(modelerrors.ConstraintsNotFound)
	}
	if err != nil {
		return dbConstraint{}, errors.Errorf("getting model constraints: %w", err)
	}
	return constraint, nil
}

// deleteModelConstraints deletes all constraints currently set on the current
// model. If no constraints are set for the current model or no model exists
// then no error is raised.
func deleteModelConstraints(
	ctx context.Context,
	preparer domain.Preparer,
	tx *sqlair.TX,
) error {
	constraintUUID, err := getModelConstraintsUUID(ctx, preparer, tx)
	if errors.Is(err, modelerrors.ConstraintsNotFound) {
		return nil
	} else if err != nil {
		return errors.Errorf("getting constraints uuid for model: %w", err)
	}

	stmt, err := preparer.Prepare(`DELETE FROM model_constraint`)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt).Run()
	if err != nil {
		return errors.Errorf("delete constraints %q for model: %w", constraintUUID, err)
	}

	dbConstraintUUID := dbConstraintUUID{UUID: constraintUUID}

	stmt, err = preparer.Prepare(
		"DELETE FROM constraint_tag WHERE constraint_uuid = $dbConstraintUUID.uuid",
		dbConstraintUUID,
	)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, dbConstraintUUID).Run()
	if err != nil {
		return errors.Errorf("deleting model constraint %q tags: %w", constraintUUID, err)
	}

	stmt, err = preparer.Prepare(
		"DELETE FROM constraint_space WHERE constraint_uuid = $dbConstraintUUID.uuid",
		dbConstraintUUID,
	)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, dbConstraintUUID).Run()
	if err != nil {
		return errors.Errorf("deleting model constraint %q spaces: %w", constraintUUID, err)
	}

	stmt, err = preparer.Prepare(
		"DELETE FROM constraint_zone WHERE constraint_uuid = $dbConstraintUUID.uuid",
		dbConstraintUUID,
	)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, dbConstraintUUID).Run()
	if err != nil {
		return errors.Errorf("deleting model constraint %q zones: %w", constraintUUID, err)
	}

	stmt, err = preparer.Prepare(
		`DELETE FROM "constraint" WHERE uuid = $dbConstraintUUID.uuid`,
		dbConstraintUUID,
	)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, dbConstraintUUID).Run()
	if err != nil {
		return errors.Errorf("deleting model constraint %q: %w", constraintUUID, err)
	}
	return nil
}

// SetModelConstraints sets the model constraints to the new values removing
// any previously set values.
// The following error types can be expected:
// - [networkerrors.SpaceNotFound]: when a space constraint is set but the
// space does not exist.
// - [machineerrors.InvalidContainerType]: when the container type set on the
// constraints is invalid.
// - [modelerrors.NotFound]: when no model exists to set constraints for.
func SetModelConstraints(
	ctx context.Context,
	preparer domain.Preparer,
	tx *sqlair.TX,
	cons constraints.Constraints,
) error {
	constraintsUUID, err := uuid.NewUUID()
	if err != nil {
		return errors.Errorf("generating new model constraint uuid: %w", err)
	}

	constraintInsertValues := constraintsToDBInsert(constraintsUUID, cons)

	selectContainerTypeStmt, err := preparer.Prepare(`
SELECT &dbContainerTypeId.*
FROM container_type
WHERE value = $dbContainerTypeValue.value
`, dbContainerTypeId{}, dbContainerTypeValue{})
	if err != nil {
		return errors.Capture(err)
	}

	insertModelConstraintStmt, err := preparer.Prepare(
		"INSERT INTO model_constraint (*) VALUES ($dbModelConstraint.*)",
		dbModelConstraint{},
	)
	if err != nil {
		return errors.Capture(err)
	}

	insertConstraintStmt, err := preparer.Prepare(
		`INSERT INTO "constraint" (*) VALUES($dbConstraintInsert.*)`,
		constraintInsertValues,
	)
	if err != nil {
		return errors.Capture(err)
	}

	err = deleteModelConstraints(ctx, preparer, tx)
	if err != nil {
		return errors.Errorf("deleting existing model constraints: %w", err)
	}

	if cons.Container != nil {
		containerTypeId := dbContainerTypeId{}
		err = tx.Query(ctx, selectContainerTypeStmt, dbContainerTypeValue{
			Value: string(*cons.Container),
		}).Get(&containerTypeId)

		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"container type %q is not valid",
				*cons.Container,
			).Add(machineerrors.InvalidContainerType)
		} else if err != nil {
			return errors.Errorf(
				"finding container type %q id: %w",
				string(*cons.Container), err,
			)
		}

		constraintInsertValues.ContainerTypeId = sql.NullInt64{
			Int64: containerTypeId.Id,
			Valid: true,
		}
	}

	err = tx.Query(ctx, insertConstraintStmt, constraintInsertValues).Run()
	if err != nil {
		return errors.Errorf("setting new constraints for model: %w", err)
	}

	modelUUID, err := getModelUUID(ctx, preparer, tx)
	if err != nil {
		return errors.Errorf("getting model uuid: %w", err)
	}

	err = tx.Query(ctx, insertModelConstraintStmt, dbModelConstraint{
		ModelUUID:      modelUUID.String(),
		ConstraintUUID: constraintsUUID.String(),
	}).Run()
	if err != nil {
		return errors.Errorf("setting model constraints: %w", err)
	}

	if cons.Tags != nil {
		err = insertConstraintTags(ctx, preparer, tx, constraintsUUID, *cons.Tags)
		if err != nil {
			return errors.Errorf("setting constraint tags for model: %w", err)
		}
	}

	if cons.Spaces != nil {
		err = insertConstraintSpaces(ctx, preparer, tx, constraintsUUID, *cons.Spaces)
		if err != nil {
			return errors.Errorf("setting constraint spaces for model: %w", err)
		}
	}

	if cons.Zones != nil {
		err = insertConstraintZones(ctx, preparer, tx, constraintsUUID, *cons.Zones)
		if err != nil {
			return errors.Errorf("setting constraint zones for model: %w", err)
		}
	}
	return nil
}

// SetModelConstraints sets the model constraints to the new values removing
// any previously set values.
// The following error types can be expected:
// - [networkerrors.SpaceNotFound]: when a space constraint is set but the
// space does not exist.
// - [machineerrors.InvalidContainerType]: when the container type set on the
// constraints is invalid.
// - [modelerrors.NotFound]: when no model exists to set constraints for.
func (s *ModelState) SetModelConstraints(
	ctx context.Context,
	cons constraints.Constraints,
) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return SetModelConstraints(ctx, s, tx, cons)
	})
}

// insertConstraintTags is responsible for setting the specified tags for the
// supplied constraint uuid. Any previously set tags for the constraint UUID
// will not be removed. Any conflicts that exist between what has been set to be
// set will result in an error and not be handled.
func insertConstraintTags(
	ctx context.Context,
	preparer domain.Preparer,
	tx *sqlair.TX,
	constraintUUID uuid.UUID,
	tags []string,
) error {
	if len(tags) == 0 {
		return nil
	}

	insertConstraintTagStmt, err := preparer.Prepare(
		"INSERT INTO constraint_tag (*) VALUES ($dbConstraintTag.*)",
		dbConstraintTag{},
	)
	if err != nil {
		return errors.Capture(err)
	}

	data := make([]dbConstraintTag, 0, len(tags))
	for _, tag := range tags {
		data = append(data, dbConstraintTag{
			ConstraintUUID: constraintUUID.String(),
			Tag:            tag,
		})
	}
	err = tx.Query(ctx, insertConstraintTagStmt, data).Run()
	if err != nil {
		return errors.Errorf("inserting constraint %q tags %w", constraintUUID, err)
	}
	return nil
}

// insertConstraintSpaces is responsible for setting the specified network
// spaces as constraints for the provided constraint uuid. Any previously set
// spaces for the constraint UUID will not be removed. Any conflicts that exist
// between what has been set to be set will result in an error and not be
// handled.
// If one or more of the spaces provided does not exist an error satisfying
// [networkerrors.SpaceNotFound] will be returned.
func insertConstraintSpaces(
	ctx context.Context,
	preparer domain.Preparer,
	tx *sqlair.TX,
	constraintUUID uuid.UUID,
	spaces []constraints.SpaceConstraint,
) error {
	if len(spaces) == 0 {
		return nil
	}

	spaceVals := make(sqlair.S, 0, len(spaces))
	for _, space := range spaces {
		spaceVals = append(spaceVals, space.SpaceName)
	}
	spaceCount := dbAggregateCount{}

	spacesExistStmt, err := preparer.Prepare(`
SELECT count(name) AS &dbAggregateCount.count
FROM space
WHERE name in ($S[:])
`, spaceCount, spaceVals)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, spacesExistStmt, spaceVals).Get(&spaceCount)
	if err != nil {
		return errors.Errorf(
			"checking that spaces for constraint %q exist: %w",
			constraintUUID, err,
		)
	}

	if spaceCount.Count != len(spaceVals) {
		return errors.Errorf(
			"inserting constraints %q spaces, space(s) %v does not exist",
			constraintUUID, spaces,
		).Add(networkerrors.SpaceNotFound)
	}

	insertConstraintSpaceStmt, err := preparer.Prepare(
		"INSERT INTO constraint_space (*) VALUES ($dbConstraintSpace.*)",
		dbConstraintSpace{},
	)
	if err != nil {
		return errors.Capture(err)
	}

	data := make([]dbConstraintSpace, 0, len(spaces))
	for _, space := range spaces {
		data = append(data, dbConstraintSpace{
			ConstraintUUID: constraintUUID.String(),
			Space:          space.SpaceName,
			Exclude:        space.Exclude,
		})
	}

	err = tx.Query(ctx, insertConstraintSpaceStmt, data).Run()
	if err != nil {
		return errors.Errorf("inserting constraint %q space(s): %w", constraintUUID, err)
	}

	return nil
}

// insertConstraintZones is responsible for setting the specified zones as
// constraints on the provided constraint uuid. Any previously set zones for the
// constraint UUID will not be removed. Any conflicts that exist between what
// has been set to be set will result in an error and not be handled.
func insertConstraintZones(
	ctx context.Context,
	preparer domain.Preparer,
	tx *sqlair.TX,
	constraintUUID uuid.UUID,
	zones []string,
) error {
	if len(zones) == 0 {
		return nil
	}

	insertConstraintZoneStmt, err := preparer.Prepare(
		"INSERT INTO constraint_zone (*) VALUES ($dbConstraintZone.*)",
		dbConstraintZone{},
	)
	if err != nil {
		return errors.Capture(err)
	}

	data := make([]dbConstraintZone, 0, len(zones))
	for _, zone := range zones {
		data = append(data, dbConstraintZone{
			ConstraintUUID: constraintUUID.String(),
			Zone:           zone,
		})
	}
	err = tx.Query(ctx, insertConstraintZoneStmt, data).Run()
	if err != nil {
		return errors.Errorf("inserting constraint zone: %w", err)
	}
	return nil
}

// GetControllerUUID returns the controller UUID for the model.
// The following errors can be expected:
// - [modelerrors.NotFound] when no model record has been established in the
// database.
func (s *ModelState) GetControllerUUID(ctx context.Context) (uuid.UUID, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return uuid.UUID{}, errors.Capture(err)
	}

	var controllerUUID dbControllerUUID
	stmt, err := s.Prepare(`SELECT &dbControllerUUID.* FROM model`, controllerUUID)
	if err != nil {
		return uuid.UUID{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&controllerUUID)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("model does not exist").Add(modelerrors.NotFound)
		}
		return errors.Capture(err)
	})

	if err != nil {
		return uuid.UUID{}, errors.Errorf(
			"getting model controller uuid: %w", err,
		)
	}

	return uuid.UUIDFromString(controllerUUID.UUID)
}

// GetModel returns model information that has been set in the database.
// If no model has been set then an error satisfying
// [modelerrors.NotFound] is returned.
func (s *ModelState) GetModel(ctx context.Context) (coremodel.ModelInfo, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return coremodel.ModelInfo{}, errors.Capture(err)
	}

	var model dbReadOnlyModel
	roStmt, err := s.Prepare(`SELECT &dbReadOnlyModel.* FROM model`, model)
	if err != nil {
		return coremodel.ModelInfo{}, errors.Capture(err)
	}

	var agentVersion dbModelAgent
	avStmt, err := s.Prepare(`SELECT &dbModelAgent.* FROM agent_version`, agentVersion)
	if err != nil {
		return coremodel.ModelInfo{}, errors.Capture(err)
	}

	var l dbModelLife
	lStmt, err := s.Prepare(`SELECT &dbModelLife.life_id FROM model_life`, l)
	if err != nil {
		return coremodel.ModelInfo{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, roStmt).Get(&model)
		if errors.Is(err, sql.ErrNoRows) {
			return modelerrors.NotFound
		} else if err != nil {
			return errors.Capture(err)
		}

		err = tx.Query(ctx, avStmt).Get(&agentVersion)
		if errors.Is(err, sql.ErrNoRows) {
			return modelagenterrors.AgentVersionNotFound
		} else if err != nil {
			return errors.Errorf("getting model agent version: %w", err)
		}

		err = tx.Query(ctx, lStmt).Get(&l)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("model life does not exist").Add(modelerrors.NotFound)
		} else if err != nil {
			return errors.Errorf("getting model life: %w", err)
		}

		return nil
	})

	if err != nil {
		return coremodel.ModelInfo{}, errors.Errorf(
			"getting model read only information: %w", err,
		)
	}

	agentVersionNumber, err := parseNullableVersion(agentVersion.TargetVersion)
	if err != nil {
		return coremodel.ModelInfo{}, errors.Errorf(
			"parsing agent version %q: %w", agentVersion.TargetVersion.V, err,
		)
	}

	life, err := l.Life.Value()
	if err != nil {
		return coremodel.ModelInfo{}, errors.Errorf(
			"parsing model life %q: %w", l.Life, err,
		)
	}

	latestAgentVersionNumber, err := parseNullableVersion(agentVersion.LatestVersion)
	if err != nil {
		return coremodel.ModelInfo{}, errors.Errorf(
			"parsing latest agent version %q: %w", agentVersion.LatestVersion.V, err,
		)
	}

	info := coremodel.ModelInfo{
		UUID:               coremodel.UUID(model.UUID),
		Name:               model.Name,
		Qualifier:          coremodel.Qualifier(model.Qualifier),
		Type:               coremodel.ModelType(model.Type),
		Cloud:              model.Cloud,
		CloudType:          model.CloudType,
		CloudRegion:        model.CloudRegion,
		CredentialName:     model.CredentialName,
		IsControllerModel:  model.IsControllerModel,
		AgentVersion:       agentVersionNumber,
		LatestAgentVersion: latestAgentVersionNumber,
		Life:               life,
	}

	if owner := model.CredentialOwner; owner != "" {
		info.CredentialOwner, err = user.NewName(owner)
		if err != nil {
			return coremodel.ModelInfo{}, errors.Errorf(
				"parsing model %q owner username %q: %w",
				model.UUID, owner, err,
			)
		}
	} else {
		s.logger.Infof(ctx, "model %s: cloud credential owner name is empty", model.Name)
	}

	info.ControllerUUID, err = uuid.UUIDFromString(model.ControllerUUID)
	if err != nil {
		return coremodel.ModelInfo{}, errors.Errorf(
			"parsing controller uuid %q for model %q: %w",
			model.ControllerUUID, model.UUID, err,
		)
	}
	return info, nil
}

// GetModelMetrics the current model info and its associated metrics.
// If no model has been set then an error satisfying
// [modelerrors.NotFound] is returned.
func (s *ModelState) GetModelMetrics(ctx context.Context) (coremodel.ModelMetrics, error) {
	modelInfo, err := s.GetModel(ctx)
	if err != nil {
		return coremodel.ModelMetrics{}, err
	}

	db, err := s.DB(ctx)
	if err != nil {
		return coremodel.ModelMetrics{}, errors.Capture(err)
	}

	var modelMetrics dbModelMetrics
	stmt, err := s.Prepare(`SELECT &dbModelMetrics.* FROM v_model_metrics;`, modelMetrics)
	if err != nil {
		return coremodel.ModelMetrics{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&modelMetrics)
		if err != nil {
			return errors.Errorf("getting model metrics: %w", err)
		}
		return nil
	})
	if err != nil {
		return coremodel.ModelMetrics{}, err
	}

	return coremodel.ModelMetrics{
		Model:            modelInfo,
		ApplicationCount: modelMetrics.ApplicationCount,
		MachineCount:     modelMetrics.MachineCount,
		UnitCount:        modelMetrics.UnitCount,
	}, nil
}

// GetModelCloudType returns the cloud type from a model that has been
// set in the database. If no model exists then an error satisfying
// [modelerrors.NotFound] is returned.
func (s *ModelState) GetModelCloudType(ctx context.Context) (string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	m := dbReadOnlyModel{}
	stmt, err := s.Prepare(`SELECT &dbReadOnlyModel.cloud_type FROM model`, m)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&m)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("model does not exist").Add(modelerrors.NotFound)
		}
		return err
	})

	if err != nil {
		return "", errors.Capture(err)
	}

	return m.CloudType, nil
}

// GetModelType returns the [coremodel.ModelType] for the current model.
// The following errors can be expected:
// - [modelerrors.NotFound] when no read only model has been established in the
// model's state layer.
func (s *ModelState) GetModelType(ctx context.Context) (coremodel.ModelType, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	m := dbModelType{}
	stmt, err := s.Prepare(`SELECT &dbModelType.* FROM model`, m)
	if err != nil {
		return "", errors.Capture(err)
	}

	return coremodel.ModelType(m.Type),
		db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt).Get(&m)
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("model does not exist").Add(modelerrors.NotFound)
			} else if err != nil {
				return errors.Errorf("querying model type: %w", err)
			}
			return nil
		})
}

// GetModelCloudRegionAndCredential returns the cloud name, region name and
// credential key for a  model identified by uuid.
// The following errors can be returned:
// - [modelerrors.NotFound] wen the model does not exist.
func (s *ModelState) GetModelCloudRegionAndCredential(
	ctx context.Context,
	uuid coremodel.UUID,
) (string, string, credential.Key, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", "", credential.Key{}, errors.Capture(err)
	}

	args := dbModelUUID{
		UUID: uuid.String(),
	}

	stmt, err := s.Prepare(`
SELECT &dbModelCloudRegionCredential.*
FROM model
WHERE uuid = $dbModelUUID.uuid
`, dbModelCloudRegionCredential{}, args)
	if err != nil {
		return "", "", credential.Key{}, errors.Capture(err)
	}

	var (
		cloudName       string
		cloudRegion     string
		credentialKey   credential.Key
		credentialOwner sql.NullString
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result dbModelCloudRegionCredential
		err := tx.Query(ctx, stmt, args).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return modelerrors.NotFound
		} else if err != nil {
			return err
		}

		cloudName = result.CloudName
		cloudRegion = result.CloudRegionName
		credentialKey = credential.Key{
			Name:  result.CredentialName.String,
			Cloud: result.CloudName,
		}
		credentialOwner = result.CredentialOwnerName
		return nil
	})
	if err != nil {
		return "", "", credential.Key{}, errors.Errorf(
			"getting model %q cloud name and credential: %w", uuid, err,
		)
	}

	if credentialOwner.Valid && credentialOwner.String != "" {
		ownerName, err := user.NewName(credentialOwner.String)
		if err != nil {
			return "", "", credential.Key{}, errors.Errorf("credential owner: %w", err)
		}
		credentialKey.Owner = ownerName
	}

	return cloudName, cloudRegion, credentialKey, nil
}

// GetModelInfoSummary returns a summary of the model information contained in
// this database.
// The following errors can be expected:
// - [modelerrors.NotFound] if no model has been established in this model
// database.
func (s *ModelState) GetModelInfoSummary(
	ctx context.Context,
) (model.ModelInfoSummary, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return model.ModelInfoSummary{}, errors.Capture(err)
	}

	// TODO (tlm): We need to provide a core count ¯\_(ツ)_/¯
	stmt, err := s.Prepare(`
WITH machine_count AS (
	SELECT count(*) AS machine_count FROM machine
),
unit_count AS (
	SELECT count(*) AS unit_count FROM unit
),
core_count AS (
	SELECT 0 AS core_count
)
SELECT &dbModelInfoSummary.*, &dbModelCountSummary.*
FROM   model, agent_version, machine_count, unit_count, core_count
`,
		dbModelInfoSummary{}, dbModelCountSummary{},
	)
	if err != nil {
		return model.ModelInfoSummary{}, errors.Capture(err)
	}

	infoSummary := dbModelInfoSummary{}
	countSummary := dbModelCountSummary{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&infoSummary, &countSummary)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New("model does not exist").Add(modelerrors.NotFound)
		} else if err != nil {
			return errors.Errorf("getting model information summary: %w", err)
		}
		return nil
	})

	if err != nil {
		return model.ModelInfoSummary{}, errors.Capture(err)
	}

	rval := model.ModelInfoSummary{
		Name:           infoSummary.Name,
		Qualifier:      coremodel.Qualifier(infoSummary.Qualifier),
		UUID:           coremodel.UUID(infoSummary.UUID),
		ModelType:      coremodel.ModelType(infoSummary.Type),
		CloudName:      infoSummary.Cloud,
		CloudType:      infoSummary.CloudType,
		CloudRegion:    infoSummary.CloudRegion.String,
		ControllerUUID: infoSummary.ControllerUUID,
		IsController:   infoSummary.IsControllerModel,
		MachineCount:   countSummary.MachineCount,
		CoreCount:      countSummary.CoreCount,
		UnitCount:      countSummary.UnitCount,
	}

	agentVerison, err := semversion.Parse(infoSummary.TargetAgentVersion)
	if err != nil {
		return model.ModelInfoSummary{}, errors.Errorf(
			"parsing target agent version %q: %w",
			infoSummary.TargetAgentVersion, err,
		)
	}
	rval.AgentVersion = agentVerison

	return rval, nil
}

// InsertModelInfo is responsible for creating a new model within the model
// database. If the model already exists then an error satisfying
// [modelerrors.AlreadyExists] is returned.
func InsertModelInfo(
	ctx context.Context, args model.ModelDetailArgs, preparer domain.Preparer, tx *sqlair.TX,
) error {
	// This is some defensive programming. The zero value of agent version is
	// still valid but should really be considered null for the purposes of
	// allowing the DDL to assert constraints.
	var agentVersion sql.Null[string]
	if args.AgentVersion != semversion.Zero {
		agentVersion.V = args.AgentVersion.String()
		agentVersion.Valid = true
	}
	var latestVersion sql.Null[string]
	if args.LatestAgentVersion != semversion.Zero {
		latestVersion.V = args.LatestAgentVersion.String()
		latestVersion.Valid = true
	}

	mID := dbUUID{UUID: args.UUID.String()}
	checkExistsStmt, err := preparer.Prepare("SELECT &dbUUID.uuid FROM model", mID)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, checkExistsStmt).Get(&mID)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("checking if model already exists: %w", err)
	} else if err == nil {
		return errors.Errorf("read-only model record already exists: %w", modelerrors.AlreadyExists)
	}

	model := dbReadOnlyModel{
		UUID:              args.UUID.String(),
		ControllerUUID:    args.ControllerUUID.String(),
		Name:              args.Name,
		Qualifier:         args.Qualifier.String(),
		Type:              args.Type.String(),
		Cloud:             args.Cloud,
		CloudType:         args.CloudType,
		CloudRegion:       args.CloudRegion,
		CredentialOwner:   args.CredentialOwner.Name(),
		CredentialName:    args.CredentialName,
		IsControllerModel: args.IsControllerModel,
	}

	modelStmt, err := preparer.Prepare("INSERT INTO model (*) VALUES ($dbReadOnlyModel.*)", model)
	if err != nil {
		return errors.Capture(err)
	}

	modelAgentVersion := dbModelAgent{
		StreamID:      int(args.AgentStream),
		TargetVersion: agentVersion,
		LatestVersion: latestVersion,
	}
	versionStmt, err := preparer.Prepare("INSERT INTO agent_version (*) VALUES ($dbModelAgent.*)", modelAgentVersion)
	if err != nil {
		return errors.Capture(err)
	}

	ml := dbModelLife{
		UUID: args.UUID,
		Life: life.Alive,
	}

	mlStmt, err := preparer.Prepare(`INSERT INTO model_life (model_uuid, life_id) VALUES ($dbModelLife.uuid, $dbModelLife.life_id)`, ml)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, modelStmt, model).Run(); err != nil {
		return errors.Errorf("creating model read-only record for %q: %w", args.UUID, err)
	}

	if err := tx.Query(ctx, versionStmt, modelAgentVersion).Run(); err != nil {
		return errors.Errorf("creating agent_version record for %q: %w", args.UUID, err)
	}

	if err := tx.Query(ctx, mlStmt, ml).Run(); err != nil {
		return errors.Errorf("creating model_life record for %q: %w", args.UUID, err)
	}

	return nil
}

// IsControllerModel returns true if the model is the controller model.
// The following errors may be returned:
// - [modelerrors.NotFound] when the model does not exist.
func (s *ModelState) IsControllerModel(ctx context.Context) (bool, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	m := dbReadOnlyModel{}
	stmt, err := s.Prepare(`SELECT &dbReadOnlyModel.is_controller_model FROM model`, m)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&m)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("model does not exist").Add(modelerrors.NotFound)
		}
		return err
	})

	if err != nil {
		return false, errors.Capture(err)
	}

	return m.IsControllerModel, nil
}

func parseNullableVersion(version sql.Null[string]) (semversion.Number, error) {
	if !version.Valid {
		return semversion.Zero, nil
	}
	return semversion.Parse(version.V)
}
