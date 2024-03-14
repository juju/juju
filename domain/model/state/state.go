// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	usererrors "github.com/juju/juju/domain/user/errors"
	jujudb "github.com/juju/juju/internal/database"
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

// Create is responsible for creating a new moddel from start to finish. It will
// register the model existence and associate all of the model metadata.
// If a model already exists with the same name and owner then an error
// satisfying modelerrors.AlreadyExists will be returned.
// If the model type is not found then an error satisfying errors.NotSupported
// will be returned.
func (s *State) Create(
	ctx context.Context,
	uuid coremodel.UUID,
	input model.ModelCreationArgs,
) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return Create(ctx, uuid, input, tx)
	})
}

// Create is responsible for creating a new model from start to finish. It will
// register the model existence and associate all of the model metadata.
// If a model already exists with the same name and owner then an error
// satisfying modelerrors.AlreadyExists will be returned.
// If the model type is not found then an error satisfying errors.NotSupported
// will be returned.
func Create(
	ctx context.Context,
	uuid coremodel.UUID,
	input model.ModelCreationArgs,
	tx *sql.Tx,
) error {
	if err := createModel(ctx, uuid, tx); err != nil {
		return err
	}

	if err := createModelAgent(ctx, uuid, input.AgentVersion, tx); err != nil {
		return err
	}

	if err := createModelMetadata(ctx, uuid, input, tx); err != nil {
		return err
	}

	return nil
}

// createModel is responsible for establishing the existence of a new model
// without any associated metadata. If a model with the supplied UUID already
// exists then an error that satisfies modelerrors.AlreadyExists is returned.
func createModel(ctx context.Context, uuid coremodel.UUID, tx *sql.Tx) error {
	stmt := "INSERT INTO model_list (uuid) VALUES (?);"
	result, err := tx.ExecContext(ctx, stmt, uuid)
	if jujudb.IsErrConstraintPrimaryKey(err) {
		return fmt.Errorf("%w for uuid %q", modelerrors.AlreadyExists, uuid)
	} else if err != nil {
		return errors.Trace(err)
	}

	if num, err := result.RowsAffected(); err != nil {
		return errors.Trace(err)
	} else if num != 1 {
		return errors.Errorf("expected 1 row to be inserted, got %d", num)
	}
	return nil
}

// createModelAgent is responsible for create a new model's agent record for the
// given model UUID. If a model agent record already exists for the given model
// uuid then an error satisfying [modelerrors.AlreadyExists] is returned. If no
// model exists for the provided UUID then a [modelerrors.NotFound] is returned.
func createModelAgent(
	ctx context.Context,
	modelUUID coremodel.UUID,
	agentVersion version.Number,
	tx *sql.Tx,
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

// createModelMetadata is responsible for creating a new model metadata record
// for the given model UUID. If a model metadata record already exists for the
// given model uuid then an error satisfying modelerrors.AlreadyExists is
// returned. Conversely should the owner already have a model that exists with
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
func createModelMetadata(
	ctx context.Context,
	uuid coremodel.UUID,
	input model.ModelCreationArgs,
	tx *sql.Tx,
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
		return fmt.Errorf("%w for model owner %q", usererrors.NotFound, input.Owner)
	} else if err != nil {
		return fmt.Errorf("getting user uuid for setting model %q owner: %w", input.Name, err)
	}

	stmt := `
INSERT INTO model_metadata (model_uuid,
                            cloud_uuid,
                            model_type_id,
                            name,
                            owner_uuid)
SELECT ?, ?, model_type.id, ?, ?
FROM model_type
WHERE model_type.type = ?
`

	res, err := tx.ExecContext(ctx, stmt,
		uuid, cloudUUID, input.Name, input.Owner, input.Type,
	)
	if jujudb.IsErrConstraintPrimaryKey(err) {
		return fmt.Errorf("%w for uuid %q", modelerrors.AlreadyExists, uuid)
	} else if jujudb.IsErrConstraintForeignKey(err) {
		return fmt.Errorf("%w for uuid %q", modelerrors.NotFound, uuid)
	} else if jujudb.IsErrConstraintUnique(err) {
		return fmt.Errorf("%w for name %q and owner %q", modelerrors.AlreadyExists, input.Name, input.Owner)
	} else if err != nil {
		return fmt.Errorf("setting model %q metadata: %w", uuid, err)
	}

	if num, err := res.RowsAffected(); err != nil {
		return errors.Trace(err)
	} else if num != 1 {
		return fmt.Errorf("creating model metadata, expected 1 row to be inserted, got %d", num)
	}

	if input.CloudRegion != "" {
		err := setCloudRegion(ctx, uuid, input.CloudRegion, tx)
		if err != nil {
			return err
		}
	}

	if !input.Credential.IsZero() {
		err := updateCredential(ctx, uuid, input.Credential, tx)
		if err != nil {
			return err
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

	deleteModelAgent := "DELETE FROM model_agent WHERE model_uuid = ?"
	deleteModelMetadata := "DELETE FROM model_metadata WHERE model_uuid = ?"
	deleteModelList := "DELETE FROM model_list WHERE uuid = ?"
	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, deleteModelAgent, uuid)
		if err != nil {
			return fmt.Errorf("delete model %q agent: %w", uuid, err)
		}

		_, err = tx.ExecContext(ctx, deleteModelMetadata, uuid)
		if err != nil {
			return fmt.Errorf("deleting model %q metadata: %w", uuid, err)
		}

		res, err := tx.ExecContext(ctx, deleteModelList, uuid)
		if err != nil {
			return fmt.Errorf("delete model %q: %w", uuid, err)
		}
		if num, err := res.RowsAffected(); err != nil {
			return errors.Trace(err)
		} else if num != 1 {
			return fmt.Errorf("%w %q", modelerrors.NotFound, uuid)
		}
		return nil
	})
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

// List returns a list of all model UUIDs in the system that have not been
// deleted.
func (s *State) List(ctx context.Context) ([]coremodel.UUID, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var models []coremodel.UUID
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		stmt := `SELECT uuid FROM model_list;`
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

// setCloudRegion is responsible for setting a model's cloud region. This
// operation can only be performed once and will fail with an error that
// satisfies errors.AlreadyExists on subsequent tries.
// If no cloud region is found for the model's cloud then an error that satisfies
// errors.NotFound will be returned.
func setCloudRegion(
	ctx context.Context,
	uuid coremodel.UUID,
	region string,
	tx *sql.Tx,
) error {
	cloudRegionStmt := `
SELECT cloud_region.uuid
FROM cloud_region
INNER JOIN cloud 
ON cloud.uuid = cloud_region.cloud_uuid
INNER JOIN model_metadata
ON model_metadata.cloud_uuid = cloud.uuid
WHERE model_metadata.model_uuid = ?
AND cloud_region.name = ?
`

	var cloudRegionUUID string
	err := tx.QueryRowContext(ctx, cloudRegionStmt, uuid, region).
		Scan(&cloudRegionUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf(
			"%w cloud region %q for model uuid %q",
			errors.NotFound,
			region,
			uuid,
		)
	} else if err != nil {
		return fmt.Errorf(
			"getting cloud region %q uuid for model %q: %w",
			region,
			uuid,
			err,
		)
	}

	modelMetadataStmt := `
UPDATE model_metadata
SET cloud_region_uuid = ?
WHERE model_uuid = ?
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

// UpdateCredential is responsible for updating the cloud credential in use
// by model. If the cloud credential is not found an error that satisfies
// errors.NotFound is returned.
// If the credential being updated to is not of the same cloud that is currently
// set for the model then an error that satisfies errors.NotValid is returned.
func (s *State) UpdateCredential(
	ctx context.Context,
	uuid coremodel.UUID,
	id credential.ID,
) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return updateCredential(ctx, uuid, id, tx)
	})
}

// updateCredential is responsible for updating the cloud credential in use
// by model. If the cloud credential is not found an error that satisfies
// errors.NotFound is returned.
// If the credential being updated to is not of the same cloud that is currently
// set for the model then an error that satisfies errors.NotValid is returned.
func updateCredential(
	ctx context.Context,
	uuid coremodel.UUID,
	id credential.ID,
	tx *sql.Tx,
) error {
	cloudCredUUIDStmt := `
SELECT cloud_credential.uuid,
       cloud.uuid
FROM cloud_credential
INNER JOIN cloud
ON cloud.uuid = cloud_credential.cloud_uuid
INNER JOIN user
ON cloud_credential.owner_uuid = user.uuid
WHERE cloud.name = ?
AND user.name = ?
AND user.removed = false
AND cloud_credential.name = ?
`

	stmt := `
UPDATE model_metadata
SET cloud_credential_uuid = ?
WHERE model_uuid = ?
AND cloud_uuid = ?
`

	var cloudCredUUID, cloudUUID string
	err := tx.QueryRowContext(ctx, cloudCredUUIDStmt, id.Cloud, id.Owner, id.Name).
		Scan(&cloudCredUUID, &cloudUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf(
			"%w cloud credential %q%w",
			errors.NotFound, id, errors.Hide(err),
		)
	} else if err != nil {
		return fmt.Errorf(
			"getting cloud credential uuid for %q: %w",
			id, err,
		)
	}

	res, err := tx.ExecContext(ctx, stmt, cloudCredUUID, uuid, cloudUUID)
	if err != nil {
		return fmt.Errorf(
			"setting cloud credential %q for model %q: %w",
			id, uuid, err)
	}

	if num, err := res.RowsAffected(); err != nil {
		return errors.Trace(err)
	} else if num != 1 {
		return fmt.Errorf(
			"%w model %q has different cloud to credential %q",
			errors.NotValid, uuid, id)
	}
	return nil
}

// GetModel is responsible for returning the model with the provided uuid.
func (s *State) Get(ctx context.Context, uuid coremodel.UUID) (*model.Model, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	q := `
SELECT m.model_uuid, m.name, t.type
FROM model_metadata m
INNER JOIN model_type t ON m.model_type_id = t.id
WHERE m.model_uuid = ?`[1:]
	m := model.Model{}
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, q, uuid).Scan(&m.UUID, &m.Name, &m.ModelType)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf(
			"%w model %q%w",
			errors.NotFound, uuid, errors.Hide(err),
		)
	}
	if err != nil {
		return nil, fmt.Errorf("getting model %q: %w", uuid, err)
	}
	return &m, nil
}

// SetSecretBackend is responsible for setting the secret backend for the model.
func (s *State) SetSecretBackend(ctx context.Context, modelUUID coremodel.UUID, backendName string) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}
	qSecretBackend := `
SELECT uuid
FROM secret_backend
WHERE name = ?`[1:]

	qModel := `
UPDATE model_metadata
SET secret_backend_uuid = ?
WHERE model_uuid = ?`[1:]
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var backendUUID string
		err = tx.QueryRowContext(ctx, qSecretBackend, backendName).Scan(&backendUUID)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf(
				"%w secret backend %q%w",
				errors.NotFound, backendName, errors.Hide(err),
			)
		}
		if err != nil {
			return fmt.Errorf("getting secret backend %q: %w", backendName, err)
		}

		_, err := tx.ExecContext(ctx, qModel, backendUUID, modelUUID)
		if err != nil {
			return fmt.Errorf(
				"setting secret backend %q for model %q: %w",
				backendName, modelUUID, err,
			)
		}
		return nil
	})
	return errors.Trace(err)
}

// GetSecretBackend is responsible for returning the secret backend for the model.
func (s *State) GetSecretBackend(ctx context.Context, modelUUID coremodel.UUID) (backend model.SecretBackendIdentifier, _ error) {
	db, err := s.DB()
	if err != nil {
		return backend, errors.Trace(err)
	}

	qModel := `
SELECT secret_backend_uuid
FROM model_metadata
WHERE model_uuid = ?`[1:]
	qBackend := `
SELECT name
FROM secret_backend
WHERE uuid = ?`[1:]
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var backendUUID sql.NullString
		err := tx.QueryRowContext(ctx, qModel, modelUUID).Scan(&backendUUID)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf(
				"%w model %q%w",
				errors.NotFound, modelUUID, errors.Hide(err),
			)
		}
		if err != nil {
			return fmt.Errorf("getting secret backend for model %q: %w", modelUUID, err)
		}
		if !backendUUID.Valid {
			// No backend configured for the model.
			// TODO: this should never happen once we start to
			// write the internal and k8s secret backend in the database, then
			// all the models will have a secret backend by default(either the
			// internal for IaaS or the k8s backend for CaaS model).
			return nil
		}
		backend.UUID = backendUUID.String
		err = tx.QueryRowContext(ctx, qBackend, backendUUID.String).Scan(&backend.Name)
		if errors.Is(err, sql.ErrNoRows) {
			// This should never happen because the `secret_backend_uuid` is a FK.
			return fmt.Errorf(
				"%w secret backend %q%w",
				errors.NotFound, backendUUID.String, errors.Hide(err),
			)
		}
		if err != nil {
			return fmt.Errorf("getting secret backend name for %q: %w", backendUUID.String, err)
		}
		return nil
	})
	return backend, errors.Trace(err)
}
