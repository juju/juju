// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/domain"
	usererrors "github.com/juju/juju/domain/access/errors"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	jujudb "github.com/juju/juju/internal/database"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// State represents a type for interacting with the underlying model state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State for interacting with the underlying model state.
func NewState(
	factory database.TxnRunnerFactory,
) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// CloudType is responsible for reporting the type for a given cloud name. If no
// cloud exists for the provided name then an error of [clouderrors.NotFound]
// will be returned.
func (s *State) CloudType(
	ctx context.Context,
	name string,
) (string, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	ctFunc := CloudType()

	var cloudType string
	return cloudType, db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		cloudType, err = ctFunc(ctx, tx, name)
		return err
	})
}

// CloudType returns a closure for reporting the type for a given cloud name. If
// no cloud exists for the provided name then an error of [clouderrors.NotFound]
// will be returned.
func CloudType() func(context.Context, *sql.Tx, string) (string, error) {
	stmt := `
SELECT ct.type
FROM cloud_type ct
INNER JOIN cloud c
ON c.cloud_type_id = ct.id
WHERE c.name = ?
`

	return func(ctx context.Context, tx *sql.Tx, name string) (string, error) {
		var cloudType string
		err := tx.QueryRowContext(ctx, stmt, name).Scan(&cloudType)
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w for name %q", clouderrors.NotFound, name)
		} else if err != nil {
			return "", fmt.Errorf("determining type for cloud %q: %w", name, domain.CoerceError(err))
		}
		return cloudType, nil
	}
}

// Create is responsible for creating a new model from start to finish. It will
// register the model existence and associate all of the model metadata.
//
// The following errors can be expected:
// - [modelerrors.AlreadyExists] when a model already exists with the same name
// and owner
// - [errors.NotSupported] When the new models type cannot be found.
// - [errors.NotFound] Should the provided cloud and region not be found.
// - [usererrors.NotFound] When the model owner does not exist.
// - [secretbackenderrors.NotFound] When the secret backend for the model
// cannot be found.
func (s *State) Create(
	ctx context.Context,
	modelID coremodel.UUID,
	modelType coremodel.ModelType,
	input model.ModelCreationArgs,
) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return Create(ctx, tx, modelID, modelType, input)
	})
}

// Create is responsible for creating a new model from start to finish. It will
// register the model existence and associate all of the model metadata.
//
// The following errors can be expected:
// - [modelerrors.AlreadyExists] when a model already exists with the same name
// and owner
// - [errors.NotSupported] When the new models type cannot be found.
// - [errors.NotFound] Should the provided cloud and region not be found.
// - [usererrors.NotFound] When the model owner does not exist.
// - [secretbackenderrors.NotFound] When the secret backend for the model
// cannot be found.
func Create(
	ctx context.Context,
	tx *sql.Tx,
	modelID coremodel.UUID,
	modelType coremodel.ModelType,
	input model.ModelCreationArgs,
) error {
	// This function is responsible for driving all of the facets of model
	// creation.

	// Create the initial model and associated metadata.
	if err := createModel(ctx, tx, modelID, modelType, input); err != nil {
		return fmt.Errorf(
			"creating initial model %q with id %q: %w",
			input.Name, modelID, err,
		)
	}

	// Add permissions for the model owner to be an admin of the newly created
	// model.
	if err := addAdminPermissions(ctx, tx, modelID, input.Owner); err != nil {
		return fmt.Errorf(
			"adding admin permissions to model %q with id %q for owner %q: %w",
			input.Name, modelID, input.Owner, err,
		)
	}

	// Creates a record for the newly created model and register the target
	// agent version.
	if err := createModelAgent(ctx, tx, modelID, input.AgentVersion); err != nil {

		return fmt.Errorf(
			"creating model %q with id %q agent: %w",
			input.Name, modelID, err,
		)
	}

	// Sets the secret backend to be used for the newly created model.
	if err := setModelSecretBackend(ctx, tx, modelID, input.SecretBackend); err != nil {
		return fmt.Errorf(
			"setting model %q with id %q secret backend: %w",
			input.Name, modelID, err,
		)
	}

	// Register a DQlite namespace for the model.
	if _, err := registerModelNamespace(ctx, tx, modelID); err != nil {
		return fmt.Errorf(
			"registering model %q with id %q database namespace: %w",
			input.Name, modelID, err,
		)
	}

	return nil
}

// GetModel returns the model associated with the provided uuid.
// If the model does not exist then an error satisfying [modelerrors.NotFound]
// will be returned.
func (s *State) GetModel(ctx context.Context, uuid coremodel.UUID) (coremodel.Model, error) {
	db, err := s.DB()
	if err != nil {
		return coremodel.Model{}, errors.Trace(err)
	}

	var model coremodel.Model
	return model, db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		model, err = GetModel(ctx, tx, uuid)
		return err
	})
}

// GetModelType returns the model type for the provided model uuid. If the model
// does not exist then an error satisfying [modelerrors.NotFound] will be
// returned.
func (s *State) GetModelType(ctx context.Context, uuid coremodel.UUID) (coremodel.ModelType, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	var modelType coremodel.ModelType
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		modelType, err = GetModelType(ctx, tx, uuid)
		return err
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	return modelType, nil
}

// GetModelType returns the model type for the provided model uuid. If the model
// does not exist then an error satisfying [modelerrors.NotFound] will be
// returned.
func GetModelType(
	ctx context.Context,
	tx *sql.Tx,
	uuid coremodel.UUID,
) (coremodel.ModelType, error) {
	stmt := `
SELECT model_type
FROM v_model AS m
WHERE uuid = ?
`
	row := tx.QueryRowContext(ctx, stmt, uuid)

	var modelType coremodel.ModelType
	err := row.Scan(&modelType)
	if errors.Is(err, sql.ErrNoRows) {
		return modelType, fmt.Errorf("%w for uuid %q", modelerrors.NotFound, uuid)
	} else if err != nil {
		return modelType, fmt.Errorf("getting model type for uuid %q: %w", uuid, domain.CoerceError(err))
	}
	return modelType, nil
}

// GetModel returns the model associated with the provided uuid.
// If the model does not exist then an error satisfying [modelerrors.NotFound]
// will be returned.
func GetModel(
	ctx context.Context,
	tx *sql.Tx,
	uuid coremodel.UUID,
) (coremodel.Model, error) {
	modelStmt := `
SELECT name,
       ma.target_version           AS agent_version,
       cloud_name,
       cloud_region_name,
       model_type,
       owner_uuid,
	   owner_name,
       cloud_credential_cloud_name,
       cloud_credential_owner_name,
       cloud_credential_name,
       life
FROM v_model
INNER JOIN model_agent ma ON v_model.uuid = ma.model_uuid
WHERE uuid = ?
`

	row := tx.QueryRowContext(ctx, modelStmt, uuid)

	var (
		// cloudRegion could be null
		agentVersion string
		cloudRegion  sql.NullString
		modelType    string
		userUUID     string
		credName     sql.NullString
		credOwner    sql.NullString
		credCloud    sql.NullString
		model        coremodel.Model
	)
	err := row.Scan(
		&model.Name,
		&agentVersion,
		&model.Cloud,
		&cloudRegion,
		&modelType,
		&userUUID,
		&model.OwnerName,
		&credCloud,
		&credOwner,
		&credName,
		&model.Life,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return coremodel.Model{}, fmt.Errorf("%w for uuid %q", modelerrors.NotFound, uuid)
	} else if err != nil {
		return coremodel.Model{}, fmt.Errorf("getting model %q: %w", uuid, domain.CoerceError(err))
	}

	model.AgentVersion, err = version.Parse(agentVersion)
	if err != nil {
		return coremodel.Model{}, fmt.Errorf("parsing model %q agent version %q: %w", uuid, agentVersion, err)
	}

	model.CloudRegion = cloudRegion.String
	model.ModelType = coremodel.ModelType(modelType)
	model.Owner = user.UUID(userUUID)
	model.Credential = credential.Key{
		Name:  credName.String,
		Cloud: credCloud.String,
		Owner: credOwner.String,
	}
	model.UUID = uuid

	return model, nil
}

// createModelAgent is responsible for creating a new model's agent record for
// the given model id. If a model agent record already exists for the given
// model uuid then an error satisfying [modelerrors.AlreadyExists] is returned.
// If no model exists for the provided UUID then a [modelerrors.NotFound] is
// returned.
func createModelAgent(
	ctx context.Context,
	tx *sql.Tx,
	modelUUID coremodel.UUID,
	agentVersion version.Number,
) error {
	stmt := `
INSERT INTO model_agent (model_uuid, previous_version, target_version)
    VALUES (?, ?, ?)
`

	res, err := tx.ExecContext(ctx, stmt, modelUUID, agentVersion.String(), agentVersion.String())
	if jujudb.IsErrConstraintPrimaryKey(err) {
		return fmt.Errorf(
			"%w for uuid %q while setting model agent version",
			modelerrors.AlreadyExists, modelUUID,
		)
	} else if jujudb.IsErrConstraintForeignKey(err) {
		return fmt.Errorf(
			"%w for uuid %q while setting model agent version",
			modelerrors.NotFound,
			modelUUID,
		)
	} else if err != nil {
		return fmt.Errorf("creating model %q agent information: %w", modelUUID, err)
	}

	if num, err := res.RowsAffected(); err != nil {
		return errors.Trace(err)
	} else if num != 1 {
		return fmt.Errorf("creating model agent record, expected 1 row to be inserted got %d", num)
	}

	return nil
}

// setModelSecretBackend sets the secret backend for a given model id. If the
// secret backend does not exist a [secretbackenderrors.NotFound] error will be
// returned. Should the model already have a secret backend set an error
// satisfying [modelerrors.SecretBackendAlreadySet].
func setModelSecretBackend(
	ctx context.Context,
	tx *sql.Tx,
	modelID coremodel.UUID,
	backend string,
) error {
	backendFindStmt := `
SELECT uuid from secret_backend WHERE name = ?
`

	var backendUUID string
	err := tx.QueryRowContext(ctx, backendFindStmt, backend).Scan(&backendUUID)

	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf(
			"setting model %q secret backend to %q: %w",
			modelID, backend, secretbackenderrors.NotFound,
		)
	} else if err != nil {
		return fmt.Errorf(
			"setting model %q secret backend to %q: %w",
			modelID, backend, err,
		)
	}

	stmt := `
INSERT INTO model_secret_backend (model_uuid, secret_backend_uuid) VALUES (?, ?)
`

	res, err := tx.ExecContext(ctx, stmt, modelID, backendUUID)
	if jujudb.IsErrConstraintPrimaryKey(err) {
		return fmt.Errorf(
			"model for id %q %w", modelID, modelerrors.SecretBackendAlreadySet,
		)
	} else if jujudb.IsErrConstraintForeignKey(err) {
		return fmt.Errorf(
			"%w for id %q while setting model secret backend to %q",
			modelerrors.NotFound,
			modelID,
			backend,
		)
	} else if err != nil {
		return fmt.Errorf(
			"setting model for id %q secret backend %q: %w",
			modelID, backend, err,
		)
	}

	if num, err := res.RowsAffected(); err != nil {
		return errors.Trace(err)
	} else if num != 1 {
		return fmt.Errorf("creating model secret backend record, expected 1 row to be inserted got %d", num)
	}

	return nil
}

// createModel is responsible for creating a new model record
// for the given model ID. If a model record already exists for the
// given model id then an error satisfying modelerrors.AlreadyExists is
// returned. Conversely, should the owner already have a model that exists with
// the provided name then a modelerrors.AlreadyExists error will be returned. If
// the model type supplied is not found then a errors.NotSupported error is
// returned.
//
// Should the provided cloud and region not be found an error matching
// errors.NotFound will be returned.
// If the ModelCreationArgs contains a non zero value cloud credential this func
// will also attempt to set the model cloud credential using updateCredential. In
// this  scenario the errors from updateCredential are also possible.
// If the model owner does not exist an error satisfying [usererrors.NotFound]
// will be returned.
func createModel(
	ctx context.Context,
	tx *sql.Tx,
	modelID coremodel.UUID,
	modelType coremodel.ModelType,
	input model.ModelCreationArgs,
) error {
	cloudStmt := `
SELECT uuid
FROM cloud
WHERE name = ?
`
	var cloudUUID string
	err := tx.QueryRowContext(ctx, cloudStmt, input.Cloud).Scan(&cloudUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w cloud %q", errors.NotFound, input.Cloud)
	} else if err != nil {
		return fmt.Errorf("getting cloud %q uuid: %w", input.Cloud, err)
	}

	userStmt := `
SELECT uuid
FROM user
WHERE uuid = ?
AND removed = false
`
	var userUUID string
	err = tx.QueryRowContext(ctx, userStmt, input.Owner).Scan(&userUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w for model owner %q", usererrors.UserNotFound, input.Owner)
	} else if err != nil {
		return fmt.Errorf("getting user uuid for setting model %q owner: %w", input.Name, err)
	}

	// deleteBadStateModel is here to allow models to be recreated that may have
	// failed during the full model creation process and never activated. We
	// will only ever allow this to happen if the model is not activated.
	deleteBadStateModel := `
DELETE FROM model
WHERE name = ?
AND owner_uuid = ?
AND activated = false
`

	_, err = tx.ExecContext(ctx, deleteBadStateModel, input.Name, input.Owner)
	if err != nil {
		return fmt.Errorf("cleaning up bad model state for name %q and owner %q: %w",
			input.Name, input.Owner, err,
		)
	}

	stmt := `
INSERT INTO model (uuid,
            cloud_uuid,
            model_type_id,
			life_id,
            name,
            owner_uuid)
SELECT ?, ?, model_type.id, ?, ?, ?
FROM model_type
WHERE model_type.type = ?
`

	res, err := tx.ExecContext(ctx, stmt,
		modelID, cloudUUID, life.Alive, input.Name, input.Owner, modelType,
	)
	if jujudb.IsErrConstraintPrimaryKey(err) {
		return fmt.Errorf("%w for id %q", modelerrors.AlreadyExists, modelID)
	} else if jujudb.IsErrConstraintUnique(err) {
		return fmt.Errorf("%w for name %q and owner %q", modelerrors.AlreadyExists, input.Name, input.Owner)
	} else if err != nil {
		return fmt.Errorf("setting model %q information: %w", modelID, err)
	}

	if num, err := res.RowsAffected(); err != nil {
		return errors.Trace(err)
	} else if num != 1 {
		return fmt.Errorf("creating model metadata, expected 1 row to be inserted, got %d", num)
	}

	if err := setCloudRegion(ctx, modelID, input.Cloud, input.CloudRegion, tx); err != nil {
		return fmt.Errorf("setting cloud region for model %q: %w", modelID, err)
	}

	if !input.Credential.IsZero() {
		err := updateCredential(ctx, tx, modelID, input.Credential)
		if err != nil {
			return fmt.Errorf("setting cloud credential for model %q: %w", modelID, err)
		}
	}

	return nil
}

// Delete will remove all data associated with the provided model uuid removing
// the models existence from Juju. If the model does not exist then a error
// satisfying modelerrors.NotFound will be returned.
func (s *State) Delete(
	ctx context.Context,
	uuid coremodel.UUID,
) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	deleteSecretBackend := "DELETE FROM model_secret_backend WHERE model_uuid = ?"
	deleteModelAgent := "DELETE FROM model_agent WHERE model_uuid = ?"
	deletePermissionStmt := `DELETE FROM permission WHERE grant_on = ?;`
	deleteModel := "DELETE FROM model WHERE uuid = ?"
	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		if err := unregisterModelNamespace(ctx, tx, uuid); err != nil {
			return fmt.Errorf("un-registering model %q database namespaces: %w", uuid, err)
		}

		_, err := tx.ExecContext(ctx, deleteSecretBackend, uuid)
		if err != nil {
			return fmt.Errorf("delete model %q secret backend: %w", uuid, err)
		}

		_, err = tx.ExecContext(ctx, deleteModelAgent, uuid)
		if err != nil {
			return fmt.Errorf("delete model %q agent: %w", uuid, err)
		}

		_, err = tx.ExecContext(ctx, deletePermissionStmt, uuid)
		if err != nil {
			return fmt.Errorf("deleting permissions for model %q: %w", uuid, err)
		}

		res, err := tx.ExecContext(ctx, deleteModel, uuid)
		if err != nil {
			return fmt.Errorf("deleting model %q metadata: %w", uuid, err)
		}

		if num, err := res.RowsAffected(); err != nil {
			return errors.Trace(err)
		} else if num != 1 {
			return fmt.Errorf("%w %q", modelerrors.NotFound, uuid)
		}

		return nil
	})
}

// Activate is responsible for setting a model as fully constructed and
// indicates the final system state for the model is ready for use. This is used
// because the model creation process involves several transactions with which
// anyone could fail at a given time.
//
// If no model exists for the provided id then a [modelerrors.NotFound] will be
// returned. If the model has previously been activated a
// [modelerrors.AlreadyActivated] error will be returned.
func (s *State) Activate(ctx context.Context, uuid coremodel.UUID) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	activator := GetActivator()

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return activator(ctx, tx, uuid)
	})
}

// ActivatorFunc is responsible for setting a model as fully constructed and
// indicates the final system state for the model is ready for use. This is used
// because the model creation process involves several transactions with which
// anyone could fail at a given time.
//
// If no model exists for the provided id then a [modelerrors.NotFound] will be
// returned. If the model as previously been activated a
// [modelerrors.AlreadyActivated] error will be returned.
type ActivatorFunc func(context.Context, *sql.Tx, coremodel.UUID) error

// GetActivator constructs a [ActivateFunc] that can safely be used over several
// transaction retry's.
func GetActivator() ActivatorFunc {
	existsStmt := `
SELECT activated
FROM model
WHERE uuid = ?
`
	stmt := `
UPDATE model
SET activated = TRUE
WHERE uuid = ?
`

	return func(ctx context.Context, tx *sql.Tx, uuid coremodel.UUID) error {
		var activated bool
		err := tx.QueryRowContext(ctx, existsStmt, uuid).Scan(&activated)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w for id %q", modelerrors.NotFound, uuid)
		} else if err != nil {
			return fmt.Errorf("determining activated status for model with id %q: %w", uuid, err)
		}

		if activated {
			return fmt.Errorf("%w for id %q", modelerrors.AlreadyActivated, uuid)
		}

		if _, err := tx.ExecContext(ctx, stmt, uuid); err != nil {
			return fmt.Errorf("activating model with id %q: %w", uuid, err)
		}
		return nil
	}
}

// GetModelTypes returns the slice of model.Type's supported by state.
func (s *State) GetModelTypes(ctx context.Context) ([]coremodel.ModelType, error) {
	rval := []coremodel.ModelType{}

	db, err := s.DB()
	if err != nil {
		return rval, errors.Trace(err)
	}

	stmt := `
SELECT type FROM model_type;
	`

	return rval, db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, stmt)
		if err != nil {
			return fmt.Errorf("getting supported model types: %w", err)
		}
		defer rows.Close()

		var t coremodel.ModelType
		for rows.Next() {
			if err := rows.Scan(&t); err != nil {
				return fmt.Errorf("building model type: %w", err)
			}
			rval = append(rval, t)
		}
		return nil
	})
}

// ListAllModels returns a slice of all models in the controller. If no models
// exist an empty slice is returned.
func (s *State) ListAllModels(ctx context.Context) ([]coremodel.Model, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelStmt := `
SELECT uuid,
       name,
       cloud_name,
       cloud_region_name,
	   model_type,
	   owner_uuid,
	   owner_name,
	   cloud_credential_name,
	   cloud_credential_owner_name,
	   cloud_credential_cloud_name,
	   life
FROM v_model
`

	rval := []coremodel.Model{}
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, modelStmt)
		if err != nil {
			return err
		}
		defer rows.Close()
		defer func() { _ = rows.Close() }()

		var (
			// cloudRegion could be null
			cloudRegion sql.NullString
			modelType   string
			userUUID    string
			credKey     credential.Key
			model       coremodel.Model
		)

		for rows.Next() {
			err := rows.Scan(
				&model.UUID,
				&model.Name,
				&model.Cloud,
				&cloudRegion,
				&modelType,
				&userUUID,
				&model.OwnerName,
				&credKey.Name,
				&credKey.Owner,
				&credKey.Cloud,
				&model.Life,
			)
			if err != nil {
				return err
			}

			model.CloudRegion = cloudRegion.String
			model.ModelType = coremodel.ModelType(modelType)
			model.Owner = user.UUID(userUUID)
			model.Credential = credKey
			rval = append(rval, model)
		}

		return rows.Err()
	})

	if err != nil {
		return nil, fmt.Errorf("getting all models: %w", domain.CoerceError(err))
	}

	return rval, nil
}

// ListModelIDs returns a list of all model UUIDs in the system that have not been
// deleted.
func (s *State) ListModelIDs(ctx context.Context) ([]coremodel.UUID, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var models []coremodel.UUID
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt := `SELECT uuid FROM v_model;`
		rows, err := tx.QueryContext(ctx, stmt)
		if err != nil {
			return errors.Trace(err)
		}
		defer rows.Close()

		for rows.Next() {
			var model coremodel.UUID
			if err := rows.Scan(&model); err != nil {
				return errors.Trace(err)
			}
			if err := rows.Err(); err != nil {
				return errors.Trace(err)
			}
			models = append(models, model)
		}
		return nil
	})
	return models, errors.Trace(err)
}

// ListModelsForUser returns a slice of models owned or accessible by the user
// specified by the user id. If No user or models are found an empty slice is
// returned.
func (s *State) ListModelsForUser(
	ctx context.Context,
	userID user.UUID,
) ([]coremodel.Model, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelStmt := `
SELECT uuid,
       name,
       cloud_name,
       cloud_region_name,
	   model_type,
	   owner_uuid,
	   owner_name,
	   cloud_credential_name,
	   cloud_credential_owner_name,
	   cloud_credential_cloud_name,
	   life
FROM v_model
WHERE owner_uuid = ?
OR uuid IN (SELECT grant_on
            FROM permission
            WHERE grant_to = ?
            AND access_type_id IN (0, 1, 3))
`

	rval := []coremodel.Model{}
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, modelStmt, userID, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		defer func() { _ = rows.Close() }()

		var (
			// cloudRegion could be null
			cloudRegion sql.NullString
			modelType   string
			userUUID    string
			credKey     credential.Key
			model       coremodel.Model
		)

		for rows.Next() {
			err := rows.Scan(
				&model.UUID,
				&model.Name,
				&model.Cloud,
				&cloudRegion,
				&modelType,
				&userUUID,
				&model.OwnerName,
				&credKey.Name,
				&credKey.Owner,
				&credKey.Cloud,
				&model.Life,
			)
			if err != nil {
				return err
			}

			model.CloudRegion = cloudRegion.String
			model.ModelType = coremodel.ModelType(modelType)
			model.Owner = user.UUID(userUUID)
			model.Credential = credKey
			rval = append(rval, model)
		}

		return rows.Err()
	})

	if err != nil {
		return nil, fmt.Errorf("getting models owned by user %q: %w", userID, domain.CoerceError(err))
	}

	return rval, nil
}

// ModelCloudNameAndCredential returns the cloud name and credential id for a
// model identified by the model name and the owner. If no model exists for the
// provided name and user a [modelerrors.NotFound] error is returned.
func (s *State) ModelCloudNameAndCredential(
	ctx context.Context,
	modelName string,
	modelOwnerName string,
) (string, credential.Key, error) {
	db, err := s.DB()
	if err != nil {
		return "", credential.Key{}, errors.Trace(err)
	}

	stmt := `
SELECT cloud_name,
       cloud_credential_name,
       cloud_credential_owner_name,
	   cloud_credential_cloud_name
FROM v_model
WHERE name = ?
AND owner_name = ?
`

	var (
		cloudName     string
		credentialKey credential.Key
	)
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, stmt, modelName, modelOwnerName)
		if err := row.Scan(&cloudName, &credentialKey.Name, &credentialKey.Owner, &credentialKey.Cloud); err != nil {
			return err
		}
		return nil
	})

	if errors.Is(err, sql.ErrNoRows) {
		return "", credential.Key{}, fmt.Errorf("%w for name %q and owner %q",
			modelerrors.NotFound, modelName, modelOwnerName,
		)
	} else if err != nil {
		return "", credential.Key{}, fmt.Errorf(
			"getting cloud name and credential for model %q with owner %q: %w",
			modelName, modelOwnerName, domain.CoerceError(err),
		)
	}

	return cloudName, credentialKey, nil
}

// NamespaceForModel returns the database namespace that is provisioned for a
// model id. If no model is found for the given id then a [modelerrors.NotFound]
// error is returned. If no namespace has been provisioned for the model then a
// [modelerrors.ModelNamespaceNotFound] error is returned.
func (st *State) NamespaceForModel(ctx context.Context, id coremodel.UUID) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	q := `
SELECT m.uuid, mn.namespace
FROM model m
LEFT JOIN model_namespace mn ON m.uuid = mn.model_uuid
WHERE m.uuid = ?
`
	var namespace sql.NullString
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, q, id).Scan(&id, &namespace)
	})

	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w for id %q", modelerrors.NotFound, id)
	} else if err != nil {
		return "", fmt.Errorf(
			"getting database namespace for model %q: %w",
			id,
			domain.CoerceError(err),
		)
	}

	if !namespace.Valid {
		return "", fmt.Errorf(
			"%w for id %q",
			modelerrors.ModelNamespaceNotFound,
			id,
		)
	}

	return namespace.String, nil
}

// registerModelNamespace is responsible for taking a constructed model and
// registering a new DQlite namespace for the model. If no model is found the
// provided uuid an error satisfying [modelerrors.NotFound] is returned.
func registerModelNamespace(
	ctx context.Context,
	tx *sql.Tx,
	uuid coremodel.UUID,
) (string, error) {
	q := "INSERT INTO namespace_list (namespace) VALUES (?)"

	_, err := tx.ExecContext(ctx, q, uuid.String())
	if jujudb.IsErrConstraintPrimaryKey(err) {
		return "", fmt.Errorf("database namespace already registered for model %q", uuid)
	} else if err != nil {
		return "", fmt.Errorf("registering database namespace for model %q: %w", uuid, err)
	}

	q = "INSERT INTO model_namespace (namespace, model_uuid) VALUES (?, ?)"
	_, err = tx.ExecContext(ctx, q, uuid.String(), uuid.String())
	if jujudb.IsErrConstraintUnique(err) {
		return "", fmt.Errorf("model %q already has a database namespace registered", uuid)
	} else if jujudb.IsErrConstraintForeignKey(err) {
		return "", fmt.Errorf("%w for uuid %q", modelerrors.NotFound, uuid)
	} else if err != nil {
		return "", fmt.Errorf("associating database namespace with model %q, %w", uuid, domain.CoerceError(err))
	}

	return uuid.String(), nil
}

// setCloudRegion is responsible for setting a model's cloud region. This
// operation can only be performed once and will fail with an error that
// satisfies errors.AlreadyExists on subsequent tries.
// If no cloud region is found for the model's cloud then an error that satisfies
// errors.NotFound will be returned.
func setCloudRegion(
	ctx context.Context,
	uuid coremodel.UUID,
	name, region string,
	tx *sql.Tx,
) error {
	// If the cloud region is not provided we will attempt to set the default
	// cloud region for the model from the controller model.
	var cloudRegionUUID string
	if region == "" {
		// Ensure that the controller cloud name is the same as the model cloud
		// name.
		stmt := `
SELECT m.cloud_region_uuid, c.name
FROM
model m
JOIN cloud c
ON m.cloud_uuid = c.uuid
WHERE m.name = 'controller'
AND c.name = ?`

		var controllerRegionUUID *string
		var n string
		if err := tx.QueryRowContext(ctx, stmt, name).Scan(&controllerRegionUUID, &n); errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return fmt.Errorf("getting controller cloud region uuid: %w", err)
		}

		// If the region is empty, we will not set a cloud region for the model
		// and will skip it.
		if controllerRegionUUID == nil || *controllerRegionUUID == "" {
			return nil
		}
		cloudRegionUUID = *controllerRegionUUID

	} else {
		stmt := `
SELECT cr.uuid
FROM cloud_region cr
INNER JOIN cloud c
ON c.uuid = cr.cloud_uuid
INNER JOIN model m
ON m.cloud_uuid = c.uuid
WHERE m.uuid = ?
AND cr.name = ?
`

		if err := tx.QueryRowContext(ctx, stmt, uuid, region).Scan(&cloudRegionUUID); errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w cloud region %q for model uuid %q", errors.NotFound, region, uuid)
		} else if err != nil {
			return fmt.Errorf("getting cloud region %q uuid for model %q: %w", region, uuid, err)
		}
	}

	modelMetadataStmt := `
UPDATE model
SET cloud_region_uuid = ?
WHERE uuid = ?
AND cloud_region_uuid IS NULL
`

	res, err := tx.ExecContext(ctx, modelMetadataStmt, cloudRegionUUID, uuid)
	if err != nil {
		return fmt.Errorf(
			"setting cloud region uuid %q for model uuid %q: %w",
			cloudRegionUUID,
			uuid,
			err,
		)
	}
	if num, err := res.RowsAffected(); err != nil {
		return errors.Trace(err)
	} else if num != 1 {
		return fmt.Errorf(
			"model %q already has a cloud region set%w",
			uuid,
			errors.Hide(errors.AlreadyExists),
		)
	}
	return nil
}

// unregisterModelNamespace is responsible for de-registering a models intent
// to be associated with any database namespaces going forward. If the model
// does not exist or has no namespace associations no error is returned.
func unregisterModelNamespace(
	ctx context.Context,
	tx *sql.Tx,
	uuid coremodel.UUID,
) error {
	q := "DELETE from model_namespace WHERE model_uuid = ?"
	_, err := tx.ExecContext(ctx, q, uuid.String())
	if err != nil {
		return fmt.Errorf("un-registering model %q database namespace: %w", uuid, domain.CoerceError(err))
	}

	return nil
}

// UpdateCredential is responsible for updating the cloud credential in use
// by model. If the cloud credential is not found an error that satisfies
// errors.NotFound is returned.
// If the credential being updated to is not of the same cloud that is currently
// set for the model then an error that satisfies errors.NotValid is returned.
func (s *State) UpdateCredential(
	ctx context.Context,
	uuid coremodel.UUID,
	key credential.Key,
) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return updateCredential(ctx, tx, uuid, key)
	})
}

// updateCredential is responsible for updating the cloud credential in use
// by model. If the cloud credential is not found an error that satisfies
// errors.NotFound is returned.
// If the credential being updated to is not of the same cloud that is currently
// set for the model then an error that satisfies errors.NotValid is returned.
func updateCredential(
	ctx context.Context,
	tx *sql.Tx,
	uuid coremodel.UUID,
	key credential.Key,
) error {
	cloudCredUUIDStmt := `
SELECT cc.uuid,
       c.uuid
FROM cloud_credential cc
INNER JOIN cloud c
ON c.uuid = cc.cloud_uuid
INNER JOIN user u
ON cc.owner_uuid = u.uuid
WHERE c.name = ?
AND u.name = ?
AND u.removed = false
AND cc.name = ?
`

	stmt := `
UPDATE model
SET cloud_credential_uuid = ?
WHERE uuid= ?
AND cloud_uuid = ?
`

	var cloudCredUUID, cloudUUID string
	err := tx.QueryRowContext(ctx, cloudCredUUIDStmt, key.Cloud, key.Owner, key.Name).
		Scan(&cloudCredUUID, &cloudUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf(
			"%w cloud credential %q%w",
			errors.NotFound, key, errors.Hide(err),
		)
	} else if err != nil {
		return fmt.Errorf(
			"getting cloud credential uuid for %q: %w",
			key, err,
		)
	}

	res, err := tx.ExecContext(ctx, stmt, cloudCredUUID, uuid, cloudUUID)
	if err != nil {
		return fmt.Errorf(
			"setting cloud credential %q for model %q: %w",
			key, uuid, err)
	}

	if num, err := res.RowsAffected(); err != nil {
		return errors.Trace(err)
	} else if num != 1 {
		return fmt.Errorf(
			"%w model %q has different cloud to credential %q",
			errors.NotValid, uuid, key)
	}
	return nil
}

// addAdminPermission adds an Admin permission for the supplied user to the
// given model. If the user already has admin permissions onto the model a
// [usererrors.PermissionAlreadyExists] error is returned.
func addAdminPermissions(
	ctx context.Context,
	tx *sql.Tx,
	modelUUID coremodel.UUID,
	ownerUUID coreuser.UUID,
) error {
	permUUID, err := internaluuid.NewUUID()
	if err != nil {
		return err
	}

	permStmt := `
INSERT INTO permission (uuid, access_type_id, object_type_id, grant_to, grant_on)
SELECT ?, at.id, ot.id, ?, ?
FROM   permission_access_type at,
       permission_object_type ot
WHERE  at.type = ?
AND    ot.type = ?
`
	res, err := tx.ExecContext(ctx, permStmt,
		permUUID.String(), ownerUUID, modelUUID, permission.AdminAccess, permission.Model,
	)

	if jujudb.IsErrConstraintUnique(err) {
		return fmt.Errorf("%w for model %q and owner %q", usererrors.PermissionAlreadyExists, modelUUID, ownerUUID)
	} else if err != nil {
		return fmt.Errorf("setting permission for model %q: %w", modelUUID, err)
	}

	if num, err := res.RowsAffected(); err != nil {
		return errors.Trace(err)
	} else if num != 1 {
		return fmt.Errorf("creating model permission metadata, expected 1 row to be inserted, got %d", num)
	}
	return nil
}
