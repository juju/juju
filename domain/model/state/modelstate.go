// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	networkerrors "github.com/juju/juju/domain/network/errors"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// NONEContainerType is the default container type.
var NONEContainerType = instance.NONE

// ModelState represents a type for interacting with the underlying model
// database state.
type ModelState struct {
	*domain.StateBase
	logger logger.Logger
}

// NewModelState returns a new State for interacting with the underlying model
// database state.
func NewModelState(
	factory database.TxnRunnerFactory,
	logger logger.Logger,
) *ModelState {
	return &ModelState{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// Create creates a new read-only model.
func (s *ModelState) Create(ctx context.Context, args model.ModelDetailArgs) error {
	db, err := s.DB()
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return CreateReadOnlyModel(ctx, args, s, tx)
	})
}

// Delete deletes a model.
func (s *ModelState) Delete(ctx context.Context, uuid coremodel.UUID) error {
	db, err := s.DB()
	if err != nil {
		return errors.Capture(err)
	}

	mUUID := dbUUID{UUID: uuid.String()}

	modelStmt, err := s.Prepare(`DELETE FROM model WHERE uuid = $dbUUID.uuid;`, mUUID)
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
		err := tx.Query(ctx, modelTriggerStmt).Run()
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New("model does not exist").Add(modelerrors.NotFound)
		} else if err != nil && !internaldatabase.IsExtendedErrorCode(err) {
			return fmt.Errorf("deleting model trigger %w", err)
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

func (s *ModelState) getModelUUID(ctx context.Context, tx *sqlair.TX) (coremodel.UUID, error) {
	var modelUUID dbUUID
	stmt, err := s.Prepare(`SELECT &dbUUID.uuid FROM model;`, dbUUID{})
	if err != nil {
		return coremodel.UUID(""), errors.Capture(err)
	}

	err = tx.Query(ctx, stmt).Get(&modelUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return coremodel.UUID(""), errors.New("model does not exist").Add(modelerrors.NotFound)
	}
	if err != nil {
		return coremodel.UUID(""), errors.Errorf("getting model uuid: %w", err)
	}

	return coremodel.UUID(modelUUID.UUID), nil
}

// GetModelConstraints returns the current model constraints.
// It returns an error satisfying [modelerrors.NotFound] if the model does not exist.
// It returns an empty constraints.Value if the model does not have a constraint configured.
func (s *ModelState) GetModelConstraints(ctx context.Context) (constraints.Value, error) {
	db, err := s.DB()
	if err != nil {
		return constraints.Value{}, errors.Capture(err)
	}

	selectTagStmt, err := s.Prepare(`
SELECT (ct.*) AS (&dbConstraintTag.*)
FROM constraint_tag ct
    JOIN "constraint" c ON ct.constraint_uuid = c.uuid
WHERE c.uuid = $dbConstraint.uuid`, dbConstraintTag{}, dbConstraint{})
	if err != nil {
		return constraints.Value{}, errors.Capture(err)
	}

	selectSpaceStmt, err := s.Prepare(`
SELECT (cs.*) AS (&dbConstraintSpace.*)
FROM constraint_space cs
    JOIN "constraint" c ON cs.constraint_uuid = c.uuid
WHERE c.uuid = $dbConstraint.uuid`, dbConstraintSpace{}, dbConstraint{})
	if err != nil {
		return constraints.Value{}, errors.Capture(err)
	}

	selectZoneStmt, err := s.Prepare(`
SELECT (cz.*) AS (&dbConstraintZone.*)
FROM constraint_zone cz
    JOIN "constraint" c ON cz.constraint_uuid = c.uuid
WHERE c.uuid = $dbConstraint.uuid`, dbConstraintZone{}, dbConstraint{})
	if err != nil {
		return constraints.Value{}, errors.Capture(err)
	}

	var (
		cons   dbConstraint
		tags   []dbConstraintTag
		spaces []dbConstraintSpace
		zones  []dbConstraintZone
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		modelUUID, err := s.getModelUUID(ctx, tx)
		if err != nil {
			return errors.Errorf("getting model uuid: %w", err)
		}
		cons, err = s.getModelConstraints(ctx, modelUUID, tx)
		if err != nil {
			return errors.Capture(err)
		}
		if cons.UUID == "" {
			// No constraint exists for the model, no furhter queries are needed.
			return nil
		}
		err = tx.Query(ctx, selectTagStmt, cons).GetAll(&tags)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting constraint tags: %w", err)
		}
		err = tx.Query(ctx, selectSpaceStmt, cons).GetAll(&spaces)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting constraint spaces: %w", err)
		}
		err = tx.Query(ctx, selectZoneStmt, cons).GetAll(&zones)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting constraint zones: %w", err)
		}
		return nil
	})
	if err != nil {
		return constraints.Value{}, errors.Capture(err)
	}
	return cons.toValue(tags, spaces, zones)
}

func (s *ModelState) getConstrainUUID(ctx context.Context, modelUUID coremodel.UUID, tx *sqlair.TX) (string, error) {
	stmt, err := s.Prepare(`
SELECT constraint_uuid AS &dbModelConstraint.constraint_uuid
FROM   model_constraint
WHERE  model_uuid = $dbModelConstraint.model_uuid`, dbModelConstraint{})
	if err != nil {
		return "", errors.Capture(err)
	}
	modelConstraint := dbModelConstraint{
		ModelUUID: modelUUID.String(),
	}
	err = tx.Query(ctx, stmt, modelConstraint).Get(&modelConstraint)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", errors.Errorf("getting constraint UUID for model %q: %w", modelUUID, err)
	}
	return modelConstraint.ConstraintUUID, nil
}

func (s *ModelState) getModelConstraints(ctx context.Context, modelUUID coremodel.UUID, tx *sqlair.TX) (dbConstraint, error) {
	stmt, err := s.Prepare(`
SELECT c.uuid AS &dbConstraint.uuid,
       c.arch AS &dbConstraint.arch,
       c.cpu_cores AS &dbConstraint.cpu_cores,
       c.cpu_power AS &dbConstraint.cpu_power,
       c.mem AS &dbConstraint.mem,
       c.root_disk AS &dbConstraint.root_disk,
       c.root_disk_source AS &dbConstraint.root_disk_source,
       c.instance_role AS &dbConstraint.instance_role,
       c.instance_type AS &dbConstraint.instance_type,
       ct.value AS &dbConstraint.container_type,
       c.virt_type AS &dbConstraint.virt_type,
       c.allocate_public_ip AS &dbConstraint.allocate_public_ip,
       c.image_id AS &dbConstraint.image_id
FROM   model_constraint mc
       JOIN "constraint" c ON c.uuid = mc.constraint_uuid
       JOIN container_type ct ON ct.id = c.container_type_id
WHERE  mc.model_uuid = $dbModelConstraint.model_uuid
`, dbConstraint{}, dbModelConstraint{})
	if err != nil {
		return dbConstraint{}, errors.Capture(err)
	}

	modelConstraint := dbModelConstraint{ModelUUID: modelUUID.String()}
	var constraint dbConstraint
	err = tx.Query(ctx, stmt, modelConstraint).Get(&constraint)
	if errors.Is(err, sql.ErrNoRows) {
		return dbConstraint{}, nil
	}
	if err != nil {
		return dbConstraint{}, errors.Errorf("getting model constraint for model %q: %w", modelUUID, err)
	}
	s.logger.Criticalf("getModelConstraints : %#v", constraint)
	return constraint, nil
}

func (s *ModelState) removeModelConstraints(ctx context.Context, modelUUID coremodel.UUID, tx *sqlair.TX) error {
	constraintUUID, err := s.getConstrainUUID(ctx, modelUUID, tx)
	if err != nil {
		return errors.Errorf("getting model constraint uuid: %w", err)
	}

	if constraintUUID == "" {
		// No constraint exists for the model, nothing to remove.
		return nil
	}

	stmt, err := s.Prepare(`DELETE FROM model_constraint`)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt).Run()
	if err != nil {
		return errors.Errorf("removing model constraints: %w", err)
	}

	stmt, err = s.Prepare(`
DELETE FROM constraint_tag 
WHERE constraint_uuid = $dbConstraintTag.constraint_uuid`, dbConstraintTag{})
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, dbConstraintTag{ConstraintUUID: constraintUUID}).Run()
	if err != nil {
		return errors.Errorf("removing constraint tags: %w", err)
	}

	stmt, err = s.Prepare(`
DELETE FROM constraint_space
WHERE constraint_uuid = $dbConstraintSpace.constraint_uuid`, dbConstraintSpace{})
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, dbConstraintSpace{ConstraintUUID: constraintUUID}).Run()
	if err != nil {
		return errors.Errorf("removing constraint spaces: %w", err)
	}

	stmt, err = s.Prepare(`
DELETE FROM constraint_zone
WHERE constraint_uuid = $dbConstraintZone.constraint_uuid`, dbConstraintZone{})
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, dbConstraintZone{ConstraintUUID: constraintUUID}).Run()
	if err != nil {
		return errors.Errorf("removing constraint zones: %w", err)
	}

	stmt, err = s.Prepare(`DELETE FROM "constraint" WHERE uuid = $dbConstraint.uuid`, dbConstraint{})
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, dbConstraint{UUID: constraintUUID}).Run()
	if err != nil {
		return errors.Errorf("removing constraint %q: %w", constraintUUID, err)
	}
	return nil
}

// SetModelConstraints sets the model constraints, including tags, spaces, and zones.
// It returns an error satisfying [networkerrors.SpaceNotFound] if a space to set does not exist,
// [modelerrors.NotFound] if the model does not exist.
func (s *ModelState) SetModelConstraints(ctx context.Context, consValue constraints.Value) error {
	db, err := s.DB()
	if err != nil {
		return errors.Capture(err)
	}
	if consValue.Container == nil {
		consValue.Container = &NONEContainerType
	}

	insertModelConstraintStmt, err := s.Prepare(`
INSERT INTO model_constraint (*)
VALUES ($dbModelConstraint.*)`, dbModelConstraint{})
	if err != nil {
		return errors.Capture(err)
	}

	upsertConstraintStmt, err := s.Prepare(`
INSERT INTO "constraint" (
    uuid,
    arch,
    cpu_cores,
    cpu_power,
    mem,
    root_disk,
    root_disk_source,
    instance_role,
    instance_type,
    container_type_id,
    virt_type,
    allocate_public_ip,
    image_id
)
SELECT $dbConstraint.uuid,
       $dbConstraint.arch,
       $dbConstraint.cpu_cores,
       $dbConstraint.cpu_power,
       $dbConstraint.mem,
       $dbConstraint.root_disk,
       $dbConstraint.root_disk_source,
       $dbConstraint.instance_role,
       $dbConstraint.instance_type,
       ct.id,
       $dbConstraint.virt_type,
       $dbConstraint.allocate_public_ip,
       $dbConstraint.image_id
FROM container_type ct
WHERE ct.value = $dbConstraint.container_type`, dbConstraint{})
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		modelUUID, err := s.getModelUUID(ctx, tx)
		if err != nil {
			return errors.Errorf("getting model uuid: %w", err)
		}

		err = s.removeModelConstraints(ctx, modelUUID, tx)
		if err != nil {
			return errors.Errorf("removing existing model constraints: %w", err)
		}

		cons := dbConstraint{}
		id, err := uuid.NewUUID()
		if err != nil {
			return errors.Errorf("generating new constraint uuid: %w", err)
		}
		cons.UUID = id.String()

		if consValue.Arch != nil {
			cons.Arch = sql.NullString{String: *consValue.Arch, Valid: true}
		}
		if consValue.CpuCores != nil {
			cons.CPUCores = sql.NullInt64{Int64: int64(*consValue.CpuCores), Valid: true}
		}
		if consValue.CpuPower != nil {
			cons.CPUPower = sql.NullInt64{Int64: int64(*consValue.CpuPower), Valid: true}
		}
		if consValue.Mem != nil {
			cons.Mem = sql.NullInt64{Int64: int64(*consValue.Mem), Valid: true}
		}
		if consValue.RootDisk != nil {
			cons.RootDisk = sql.NullInt64{Int64: int64(*consValue.RootDisk), Valid: true}
		}
		if consValue.RootDiskSource != nil {
			cons.RootDiskSource = sql.NullString{String: *consValue.RootDiskSource, Valid: true}
		}
		if consValue.InstanceRole != nil {
			cons.InstanceRole = sql.NullString{String: *consValue.InstanceRole, Valid: true}
		}
		if consValue.InstanceType != nil {
			cons.InstanceType = sql.NullString{String: *consValue.InstanceType, Valid: true}
		}
		if consValue.Container != nil {
			cons.ContainerType = sql.NullString{String: string(*consValue.Container), Valid: true}
		}
		if consValue.VirtType != nil {
			cons.VirtType = sql.NullString{String: *consValue.VirtType, Valid: true}
		}
		if consValue.AllocatePublicIP != nil {
			cons.AllocatePublicIP = sql.NullBool{Bool: *consValue.AllocatePublicIP, Valid: true}
		}
		if consValue.ImageID != nil {
			cons.ImageID = sql.NullString{String: *consValue.ImageID, Valid: true}
		}

		err = tx.Query(ctx, upsertConstraintStmt, cons).Run()
		if err != nil {
			return errors.Errorf("upserting constraint: %w", err)
		}

		err = tx.Query(ctx, insertModelConstraintStmt, dbModelConstraint{
			ModelUUID:      modelUUID.String(),
			ConstraintUUID: cons.UUID,
		}).Run()
		if err != nil {
			return errors.Errorf("inserting model constraint: %w", err)
		}

		if consValue.Tags != nil {
			err = s.insertContraintTags(ctx, tx, cons.UUID, *consValue.Tags)
			if err != nil {
				return errors.Errorf("upserting constraint tags for constraint %q: %w", cons.UUID, err)
			}
		}

		if consValue.Spaces != nil {
			err = s.insertContraintSpaces(ctx, tx, cons.UUID, *consValue.Spaces)
			if err != nil {
				return errors.Errorf("upserting constraint spaces for constraint %q: %w", cons.UUID, err)
			}
		}

		if consValue.Zones != nil {
			err = s.insertContraintZones(ctx, tx, cons.UUID, *consValue.Zones)
			if err != nil {
				return errors.Errorf("upserting constraint zones for constraint %q: %w", cons.UUID, err)
			}
		}
		return nil
	})
	return errors.Capture(err)
}

func (s *ModelState) insertContraintTags(ctx context.Context, tx *sqlair.TX, constraintUUID string, tags []string) error {
	insertConstraintTagStmt, err := s.Prepare(`
INSERT INTO constraint_tag (*)
VALUES ($dbConstraintTag.*)`, dbConstraintTag{})
	if err != nil {
		return errors.Capture(err)
	}

	if len(tags) == 0 {
		return nil
	}

	var data []dbConstraintTag
	for _, tag := range tags {
		if tag == "" {
			continue
		}
		data = append(data, dbConstraintTag{
			ConstraintUUID: constraintUUID,
			Tag:            tag,
		})
	}
	err = tx.Query(ctx, insertConstraintTagStmt, data).Run()
	if err != nil {
		return errors.Errorf("inserting constraint tags %w", err)
	}
	return nil
}

func (s *ModelState) insertContraintSpaces(ctx context.Context, tx *sqlair.TX, constraintUUID string, spaces []string) error {
	insertConstraintSpaceStmt, err := s.Prepare(`
INSERT INTO constraint_space (*)
VALUES ($dbConstraintSpace.*)`, dbConstraintSpace{})
	if err != nil {
		return errors.Capture(err)
	}

	if len(spaces) == 0 {
		return nil
	}

	var data []dbConstraintSpace
	for _, space := range spaces {
		if space == "" {
			continue
		}
		data = append(data, dbConstraintSpace{
			ConstraintUUID: constraintUUID,
			Space:          space,
		})
	}
	err = tx.Query(ctx, insertConstraintSpaceStmt, data).Run()
	if internaldatabase.IsErrConstraintForeignKey(err) {
		return errors.Errorf("inserting model space constraints").Add(networkerrors.SpaceNotFound)
	}
	if err != nil {
		return errors.Errorf("inserting constraint space: %w", err)
	}
	return nil
}

func (s *ModelState) insertContraintZones(ctx context.Context, tx *sqlair.TX, constraintUUID string, zones []string) error {
	insertConstraintZoneStmt, err := s.Prepare(`
INSERT INTO constraint_zone (*)
VALUES ($dbConstraintZone.*)`, dbConstraintZone{})
	if err != nil {
		return errors.Capture(err)
	}

	if len(zones) == 0 {
		return nil
	}

	var data []dbConstraintZone
	for _, zone := range zones {
		if zone == "" {
			continue
		}
		data = append(data, dbConstraintZone{
			ConstraintUUID: constraintUUID,
			Zone:           zone,
		})
	}
	err = tx.Query(ctx, insertConstraintZoneStmt, data).Run()
	if err != nil {
		return errors.Errorf("inserting constraint zone: %w", err)
	}
>>>>>>> ba8d2cbc4a (feat: implement model constraints getter and setter state methods;)
	return nil
}

// GetModel returns a read-only model information that has been set in the
// database. If no model has been set then an error satisfying
// [modelerrors.NotFound] is returned.
func (s *ModelState) GetModel(ctx context.Context) (coremodel.ModelInfo, error) {
	db, err := s.DB()
	if err != nil {
		return coremodel.ModelInfo{}, errors.Capture(err)
	}

	m := dbReadOnlyModel{}
	stmt, err := s.Prepare(`SELECT &dbReadOnlyModel.* FROM model`, m)
	if err != nil {
		return coremodel.ModelInfo{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&m)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("model does not exist").Add(modelerrors.NotFound)
		}
		return err
	})

	if err != nil {
		return coremodel.ModelInfo{}, errors.Errorf(
			"getting model read only information: %w", err,
		)
	}

	model := coremodel.ModelInfo{
		UUID:              coremodel.UUID(m.UUID),
		Name:              m.Name,
		Type:              coremodel.ModelType(m.Type),
		Cloud:             m.Cloud,
		CloudType:         m.CloudType,
		CloudRegion:       m.CloudRegion,
		CredentialName:    m.CredentialName,
		IsControllerModel: m.IsControllerModel,
	}

	if owner := m.CredentialOwner; owner != "" {
		model.CredentialOwner, err = user.NewName(owner)
		if err != nil {
			return coremodel.ModelInfo{}, errors.Errorf(
				"parsing model %q owner username %q: %w",
				m.UUID, owner, err,
			)
		}
	} else {
		s.logger.Infof("model %s: cloud credential owner name is empty", model.Name)
	}

	var agentVersion string
	if m.TargetAgentVersion.Valid {
		agentVersion = m.TargetAgentVersion.String
	}

	model.AgentVersion, err = version.Parse(agentVersion)
	if err != nil {
		return coremodel.ModelInfo{}, errors.Errorf(
			"parsing model %q agent version %q: %w",
			m.UUID, agentVersion, err,
		)
	}

	model.ControllerUUID, err = uuid.UUIDFromString(m.ControllerUUID)
	if err != nil {
		return coremodel.ModelInfo{}, errors.Errorf(
			"parsing controller uuid %q for model %q: %w",
			m.ControllerUUID, m.UUID, err,
		)
	}
	return model, nil
}

// GetModelMetrics the current model info and its associated metrics.
// If no model has been set then an error satisfying
// [modelerrors.NotFound] is returned.
func (s *ModelState) GetModelMetrics(ctx context.Context) (coremodel.ModelMetrics, error) {
	readOnlyModel, err := s.GetModel(ctx)
	if err != nil {
		return coremodel.ModelMetrics{}, err
	}

	db, err := s.DB()
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
		Model:            readOnlyModel,
		ApplicationCount: modelMetrics.ApplicationCount,
		MachineCount:     modelMetrics.MachineCount,
		UnitCount:        modelMetrics.UnitCount,
	}, nil
}

// GetModelCloudType returns the cloud type from a model that has been
// set in the database. If no model exists then an error satisfying
// [modelerrors.NotFound] is returned.
func (s *ModelState) GetModelCloudType(ctx context.Context) (string, error) {
	db, err := s.DB()
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

// CreateReadOnlyModel is responsible for creating a new model within the model
// database. If the model already exists then an error satisfying
// [modelerrors.AlreadyExists] is returned.
func CreateReadOnlyModel(ctx context.Context, args model.ModelDetailArgs, preparer domain.Preparer, tx *sqlair.TX) error {
	// This is some defensive programming. The zero value of agent version is
	// still valid but should really be considered null for the purposes of
	// allowing the DDL to assert constraints.
	var agentVersion sql.NullString
	if args.AgentVersion != version.Zero {
		agentVersion.String = args.AgentVersion.String()
		agentVersion.Valid = true
	}

	uuid := dbUUID{UUID: args.UUID.String()}
	checkExistsStmt, err := preparer.Prepare(`
SELECT &dbUUID.uuid
FROM model
	`, uuid)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, checkExistsStmt).Get(&uuid)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf(
			"checking if model %q already exists: %w",
			args.UUID, err,
		)
	} else if err == nil {
		return errors.Errorf(
			"creating readonly model %q information but model already exists",
			args.UUID,
		).Add(modelerrors.AlreadyExists)
	}

	m := dbReadOnlyModel{
		UUID:               args.UUID.String(),
		ControllerUUID:     args.ControllerUUID.String(),
		Name:               args.Name,
		Type:               args.Type.String(),
		TargetAgentVersion: agentVersion,
		Cloud:              args.Cloud,
		CloudType:          args.CloudType,
		CloudRegion:        args.CloudRegion,
		CredentialOwner:    args.CredentialOwner.Name(),
		CredentialName:     args.CredentialName,
		IsControllerModel:  args.IsControllerModel,
	}

	insertStmt, err := preparer.Prepare(`
INSERT INTO model (*) VALUES ($dbReadOnlyModel.*)
`, dbReadOnlyModel{})
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, insertStmt, m).Run(); err != nil {
		return errors.Errorf(
			"creating readonly model %q information: %w", args.UUID, err,
		)
	}

	return nil
}
