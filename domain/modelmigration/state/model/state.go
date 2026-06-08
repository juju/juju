// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	modelmigrationinternal "github.com/juju/juju/domain/modelmigration/internal"
	"github.com/juju/juju/internal/errors"
)

// State represents the access method for interacting the underlying model
// during model migration.
type State struct {
	*domain.StateBase

	modelUUID model.UUID
}

// New creates a new [State]
func New(modelFactory database.TxnRunnerFactory, modelUUID model.UUID) *State {
	return &State{
		StateBase: domain.NewStateBase(modelFactory),
		modelUUID: modelUUID,
	}
}

// GetControllerUUID is responsible for returning the controller's unique id
// from state.
func (s *State) GetControllerUUID(
	ctx context.Context,
) (string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", errors.Errorf("cannot get database to retrieve controller uuid: %w", err)
	}

	stmt, err := s.Prepare(`
SELECT (controller_uuid) AS (&modelInfo.*)
FROM model`, modelInfo{})

	if err != nil {
		return "", errors.Errorf("preparing get controller uuid statement: %w", err)
	}

	result := modelInfo{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New(
				"cannot get controller uuid, model information is missing from database",
			).Add(err)
		} else if err != nil {
			return errors.Errorf(
				"cannot get controller uuid on model database: %w",
				err,
			)
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	return result.ControllerUUID, nil
}

// GetAllInstanceIDs returns all instance IDs from the current model as
// juju/collections set.
func (s *State) GetAllInstanceIDs(ctx context.Context) (set.Strings, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Errorf("cannot get database to retrieve instance IDs: %w", err)
	}

	query := `
SELECT &instanceID.instance_id
FROM   machine_cloud_instance`
	queryStmt, err := s.Prepare(query, instanceID{})
	if err != nil {
		return nil, errors.Errorf("preparing retrieve all instance IDs statement: %w", err)
	}

	var result []instanceID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, queryStmt).GetAll(&result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("retrieving all instance IDs: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	instanceIDs := make(set.Strings, len(result))
	for _, instanceID := range result {
		instanceIDs.Add(instanceID.ID)
	}
	return instanceIDs, nil
}

// GetOfferUUIDs returns the UUIDs of all offers hosted by this model. These are
// used by the controller-DB side to read the offer-scoped permission rows that
// must travel with the model migration.
func (s *State) GetOfferUUIDs(ctx context.Context) ([]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Errorf("cannot get database to retrieve offer UUIDs: %w", err)
	}

	stmt, err := s.Prepare(`SELECT &entityUUID.uuid FROM offer`, entityUUID{})
	if err != nil {
		return nil, errors.Errorf("preparing retrieve offer UUIDs statement: %w", err)
	}

	var result []entityUUID
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result = nil
		err := tx.Query(ctx, stmt).GetAll(&result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("retrieving offer UUIDs: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	uuids := make([]string, 0, len(result))
	for _, r := range result {
		uuids = append(uuids, r.UUID)
	}
	return uuids, nil
}

// GetOffererModels returns the distinct (offerer controller, offerer model)
// pairs referenced by this model's remote applications, excluding rows with a
// null offerer controller UUID. The controller-DB side reads the matching
// third-party external_controller and external_model rows from these pairs.
func (s *State) GetOffererModels(ctx context.Context) ([]modelmigrationinternal.OffererModel, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Errorf("cannot get database to retrieve offerer models: %w", err)
	}

	stmt, err := s.Prepare(`
SELECT DISTINCT (offerer_controller_uuid, offerer_model_uuid) AS (&offererModel.*)
FROM   application_remote_offerer
WHERE  offerer_controller_uuid IS NOT NULL
`, offererModel{})
	if err != nil {
		return nil, errors.Errorf("preparing retrieve offerer models statement: %w", err)
	}

	var result []offererModel
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result = nil
		err := tx.Query(ctx, stmt).GetAll(&result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("retrieving offerer models: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	models := make([]modelmigrationinternal.OffererModel, 0, len(result))
	for _, r := range result {
		models = append(models, modelmigrationinternal.OffererModel{
			ControllerUUID: r.ControllerUUID,
			ModelUUID:      r.ModelUUID,
		})
	}
	return models, nil
}

// GetMigrationAgents returns all agents that must report migration minion
// progress for this model.
func (s *State) GetMigrationAgents(ctx context.Context) (modelmigrationinternal.MigrationAgents, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return modelmigrationinternal.MigrationAgents{}, errors.Errorf("cannot get database to retrieve migration agents: %w", err)
	}

	modelTypeStmt, err := s.Prepare(`
SELECT &modelType.*
FROM   model
`, modelType{})
	if err != nil {
		return modelmigrationinternal.MigrationAgents{}, errors.Capture(err)
	}
	machineStmt, err := s.Prepare(`
SELECT &agentName.name
FROM   machine
`, agentName{})
	if err != nil {
		return modelmigrationinternal.MigrationAgents{}, errors.Capture(err)
	}
	unitStmt, err := s.Prepare(`
SELECT &agentName.name
FROM   unit
`, agentName{})
	if err != nil {
		return modelmigrationinternal.MigrationAgents{}, errors.Capture(err)
	}
	applicationAgentStmt, err := s.Prepare(`
SELECT &agentName.name
FROM   application AS a
JOIN   application_agent AS aa ON aa.application_uuid = a.uuid
`, agentName{})
	if err != nil {
		return modelmigrationinternal.MigrationAgents{}, errors.Capture(err)
	}

	var (
		modelTypeValue modelType
		machines       []agentName
		units          []agentName
		applications   []agentName
	)
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		modelTypeValue = modelType{}
		machines = nil
		units = nil
		applications = nil

		if err := tx.Query(ctx, modelTypeStmt).Get(&modelTypeValue); err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.New("model information is missing from database")
			}
			return errors.Errorf("querying model type: %w", err)
		}

		if model.ModelType(modelTypeValue.Type) == model.CAAS {
			if err := tx.Query(ctx, applicationAgentStmt).GetAll(&applications); err != nil &&
				!errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("querying application agents: %w", err)
			}
		} else if err := tx.Query(ctx, machineStmt).GetAll(&machines); err != nil &&
			!errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying machine agents: %w", err)
		}

		if err := tx.Query(ctx, unitStmt).GetAll(&units); err != nil &&
			!errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying unit agents: %w", err)
		}
		return nil
	}); err != nil {
		return modelmigrationinternal.MigrationAgents{}, errors.Capture(err)
	}

	agents := modelmigrationinternal.MigrationAgents{
		Machines:     make([]string, 0, len(machines)),
		Units:        make([]string, 0, len(units)),
		Applications: make([]string, 0, len(applications)),
	}
	for _, m := range machines {
		agents.Machines = append(agents.Machines, m.Name)
	}
	for _, u := range units {
		agents.Units = append(agents.Units, u.Name)
	}
	for _, a := range applications {
		agents.Applications = append(agents.Applications, a.Name)
	}
	return agents, nil
}

// DeleteModelImportingStatus removes the entry from the model_migrating table
// in the model database, indicating that the model import has completed or been
// aborted.
func (s *State) DeleteModelImportingStatus(ctx context.Context) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Errorf("cannot get database to delete importing status: %w", err)
	}

	modelUUIDArg := entityUUID{
		UUID: s.modelUUID.String(),
	}

	stmt, err := s.Prepare(`
DELETE FROM model_migrating
WHERE model_uuid = $entityUUID.uuid
	`, modelUUIDArg)
	if err != nil {
		return errors.Errorf("preparing delete importing status statement: %w", err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, modelUUIDArg).Run(); err != nil {
			return errors.Errorf("deleting importing status for model %q: %w", s.modelUUID, err)
		}
		return nil
	})
}

// GetModelTargetAgentVersion returns the target agent version currently set for
// the model. This func expects that the target agent version for the model has
// already been set.
func (s *State) GetModelTargetAgentVersion(
	ctx context.Context,
) (string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var currentVersion string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		currentVersion, err = s.getModelTargetAgentVersion(ctx, tx)
		return err
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	return currentVersion, nil
}

// SetModelTargetAgentVersion is responsible for setting the current target
// agent version of the model. This function expects a precondition
// version to be supplied. The model's target agent version at the time the
// operation is applied must match the preCondition version or else an error is
// returned.
func (s *State) SetModelTargetAgentVersion(
	ctx context.Context,
	preCondition, toVersion string,
) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	toVersionInput := setAgentVersionTarget{
		TargetVersion:   toVersion,
		PreviousVersion: preCondition}
	setAgentVersionStmt, err := s.Prepare(`
UPDATE agent_version
SET    target_version = $setAgentVersionTarget.target_version
WHERE  target_version = $setAgentVersionTarget.previous_version
`,
		toVersionInput,
	)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var outcome sqlair.Outcome
		if err := tx.Query(ctx, setAgentVersionStmt, toVersionInput).Get(&outcome); err != nil {
			return errors.Errorf(
				"setting target agent version to %q for model: %w",
				toVersion, err,
			)
		}
		if affected, err := outcome.Result().RowsAffected(); err != nil {
			return errors.Errorf(
				"checking rows affected when setting target agent version to %q for model: %w",
				toVersion, err,
			)
		} else if affected == 0 {
			return errors.Errorf(
				"setting target agent version to %q for model: expected current version %q",
				toVersion, preCondition,
			)
		}
		return nil
	})

	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

func (s *State) getModelTargetAgentVersion(
	ctx context.Context,
	tx *sqlair.TX,
) (string, error) {
	var dbVal agentVersionTarget
	stmt, err := s.Prepare("SELECT &agentVersionTarget.* FROM agent_version", dbVal)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, stmt).Get(&dbVal)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.New("no target agent version has previously been set for the controller's model")
	}

	return dbVal.TargetVersion, err
}
