// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/cloud"
	corecloud "github.com/juju/juju/core/cloud"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain"
	accesserrors "github.com/juju/juju/domain/access/errors"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	jujudb "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
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

// CheckModelExists is a check that allows the caller to find out if a model
// exists and is active within the controller. True or false is returned
// indicating if the model exists.
func (s *State) CheckModelExists(
	ctx context.Context,
	modelUUID coremodel.UUID,
) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	exists := false
	return exists, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		exists, err = s.checkModelExists(ctx, tx, modelUUID)
		return err
	})
}

// checkModelExists is a check that allows the caller to find out if a model
// exists in the controller. True is returned when the model has been found.
// This func does not work with models that have not been activated.
func (s *State) checkModelExists(
	ctx context.Context,
	tx *sqlair.TX,
	modelUUID coremodel.UUID,
) (bool, error) {

	dbModelUUID := dbModelUUID{UUID: modelUUID.String()}
	stmt, err := s.Prepare(`
SELECT &dbModelUUID.* FROM v_model WHERE uuid = $dbModelUUID.uuid
`, dbModelUUID)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, dbModelUUID).Get(&dbModelUUID)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return false, errors.Errorf("checking model %q exists: %w", modelUUID, err)
	}
	return !errors.Is(err, sqlair.ErrNoRows), nil
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
		return "", errors.Capture(err)
	}

	ctFunc := CloudType()

	var cloudType string
	return cloudType, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		cloudType, err = ctFunc(ctx, s, tx, name)
		return err
	})
}

// CloudType returns a closure for reporting the type for a given cloud name. If
// no cloud exists for the provided name then an error of [clouderrors.NotFound]
// will be returned.
func CloudType() func(context.Context, domain.Preparer, *sqlair.TX, string) (string, error) {
	return func(ctx context.Context, preparer domain.Preparer, tx *sqlair.TX, name string) (string, error) {
		n := dbName{Name: name}

		stmt, err := preparer.Prepare(`
SELECT (ct.type) AS (&dbCloudType.*)
FROM cloud_type ct
INNER JOIN cloud c
ON c.cloud_type_id = ct.id
WHERE c.name = $dbName.name
		`, dbCloudType{}, n)
		if err != nil {
			return "", errors.Capture(err)
		}

		var cloudType dbCloudType

		if err := tx.Query(ctx, stmt, n).Get(&cloudType); errors.Is(err, sqlair.ErrNoRows) {
			return "", errors.Errorf("%w for name %q", clouderrors.NotFound, name)
		} else if err != nil {
			return "", errors.Errorf("determining type for cloud %q: %w", name, err)
		}
		return cloudType.Type, nil
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
	input model.GlobalModelCreationArgs,
) error {
	db, err := s.DB()
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return Create(ctx, s, tx, modelID, modelType, input)
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
	preparer domain.Preparer,
	tx *sqlair.TX,
	modelID coremodel.UUID,
	modelType coremodel.ModelType,
	input model.GlobalModelCreationArgs,
) error {
	// This function is responsible for driving all of the facets of model
	// creation.

	// Create the initial model and associated metadata.
	if err := createModel(ctx, preparer, tx, modelID, modelType, input); err != nil {
		return errors.Errorf(
			"creating initial model %q with id %q: %w",
			input.Name, modelID, err,
		)
	}

	// Add permissions for the model owner to be an admin of the newly created
	// model.
	if err := addAdminPermissions(ctx, preparer, tx, modelID, input.Owner); err != nil {
		return errors.Errorf(
			"adding admin permissions to model %q with id %q for owner %q: %w",
			input.Name, modelID, input.Owner, err,
		)
	}

	// Sets the secret backend to be used for the newly created model.
	if err := setModelSecretBackend(ctx, preparer, tx, modelID, input.SecretBackend); err != nil {
		return errors.Errorf(
			"setting model %q with id %q secret backend: %w",
			input.Name, modelID, err,
		)
	}

	// Register a DQlite namespace for the model.
	if _, err := registerModelNamespace(ctx, preparer, tx, modelID); err != nil {
		return errors.Errorf(
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
		return coremodel.Model{}, errors.Capture(err)
	}

	var model coremodel.Model
	return model, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		model, err = GetModel(ctx, tx, uuid)
		return err
	})
}

// GetModelByName returns the model found for the given username and model name
// for which there can only be one. Should no model be found for the provided
// search criteria an error satisfying [modelerrors.NotFound] will be returned.
func (s *State) GetModelByName(
	ctx context.Context,
	username user.Name,
	modelName string,
) (coremodel.Model, error) {
	db, err := s.DB()
	if err != nil {
		return coremodel.Model{}, errors.Capture(err)
	}

	dbNames := dbNames{
		ModelName: modelName,
		OwnerName: username.Name(),
	}

	var model dbModel
	stmt, err := s.Prepare(`
SELECT &dbModel.*
FROM v_model
WHERE name = $dbNames.name
AND owner_name = $dbNames.owner_name
`, model, dbNames)
	if err != nil {
		return coremodel.Model{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, dbNames).Get(&model); errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"%w for user %q and name %q",
				modelerrors.NotFound,
				username,
				modelName,
			)
		} else if err != nil {
			return errors.Errorf(
				"cannot find model for user %q and name %q: %w",
				username,
				modelName,
				err,
			)
		}
		return nil
	})
	if err != nil {
		return coremodel.Model{}, errors.Capture(err)
	}

	return model.toCoreModel()
}

// GetModelState is responsible for returning a set of boolean indicators for
// key aspects about a model so that a model's status can be derived from this
// information. If no model exists for the provided UUID then an error
// satisfying [modelerrors.NotFound] will be returned.
func (s *State) GetModelState(ctx context.Context, uuid coremodel.UUID) (model.ModelState, error) {
	db, err := s.DB()
	if err != nil {
		return model.ModelState{}, errors.Capture(err)
	}

	modelUUIDVal := dbModelUUID{UUID: uuid.String()}
	modelState := dbModelState{}

	stmt, err := s.Prepare(`
SELECT &dbModelState.* FROM v_model_state WHERE uuid = $dbModelUUID.uuid
`, modelUUIDVal, modelState)
	if err != nil {
		return model.ModelState{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, modelUUIDVal).Get(&modelState)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New("model does not exist").Add(modelerrors.NotFound)
		}
		return err
	})

	if err != nil {
		return model.ModelState{}, errors.Errorf(
			"getting model %q state: %w", uuid, err,
		)
	}

	return model.ModelState{
		Destroying:                   modelState.Destroying,
		Migrating:                    modelState.Migrating,
		HasInvalidCloudCredential:    modelState.CredentialInvalid,
		InvalidCloudCredentialReason: modelState.CredentialInvalidReason,
	}, nil
}

// GetControllerModelUUID returns the model uuid for the controller model.
// If no controller model exists then an error satisfying [modelerrors.NotFound]
// is returned.
func (s *State) GetControllerModelUUID(ctx context.Context) (coremodel.UUID, error) {
	db, err := s.DB()
	if err != nil {
		return coremodel.UUID(""), errors.Capture(err)
	}

	controllerModelUUID := dbModelUUIDRef{}
	stmt, err := s.Prepare(`
SELECT &dbModelUUIDRef.model_uuid
FROM   controller
`, controllerModelUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&controllerModelUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return modelerrors.NotFound
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	})

	if err != nil {
		return coremodel.UUID(""), errors.Errorf(
			"getting controller model uuid: %w", err,
		)
	}
	return coremodel.UUID(controllerModelUUID.ModelUUID), nil
}

// GetControllerModel returns the model the controller is running in. If no
// controller model exists then an error satisfying [modelerrors.NotFound] is
// returned.
func (s *State) GetControllerModel(ctx context.Context) (coremodel.Model, error) {
	db, err := s.DB()
	if err != nil {
		return coremodel.Model{}, errors.Capture(err)
	}

	controllerModelUUID := dbModelUUIDRef{}
	stmt, err := s.Prepare(`
SELECT &dbModelUUIDRef.model_uuid
FROM   controller
`, controllerModelUUID)
	if err != nil {
		return coremodel.Model{}, errors.Capture(err)
	}

	var model coremodel.Model
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&controllerModelUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New("no controller model exists").Add(
				modelerrors.NotFound,
			)
		} else if err != nil {
			return errors.Capture(err)
		}
		model, err = GetModel(ctx, tx, coremodel.UUID(controllerModelUUID.ModelUUID))
		return errors.Capture(err)
	})
	if err != nil {
		return coremodel.Model{}, errors.Errorf("getting controller model: %w", err)
	}
	return model, nil
}

// GetModelSeedInformation returns information related to a model for the
// purposes of seeding this information into other parts of a Juju controller.
// This method is similar to [State.GetModel] but it allows for the returning of
// information on models that are not activated yet.
//
// This is needed to seed the static read only information of a model into the
// model's database or build a service on behalf of the model that should run
// regardless if the model is activated like logging.
//
// The following error types can be expected:
// - [modelerrors.NotFound]: When the model is not found for the given uuid
// regardless of the activated status.
func (s *State) GetModelSeedInformation(
	ctx context.Context,
	modelUUID coremodel.UUID,
) (coremodel.ModelInfo, error) {
	// WARNING (tlm): You are potentially working with half initialized model
	// information in this function be careful!
	db, err := s.DB()
	if err != nil {
		return coremodel.ModelInfo{}, errors.Capture(err)
	}

	q := `
SELECT &dbModel.*
FROM v_model_all
WHERE uuid = $dbModel.uuid
`
	model := dbModel{UUID: modelUUID.String()}
	stmt, err := s.Prepare(q, model)
	if err != nil {
		return coremodel.ModelInfo{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, model).Get(&model)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("%w for uuid %q", modelerrors.NotFound, modelUUID)
		} else if err != nil {
			return errors.Errorf("getting model %q: %w", modelUUID, err)
		}
		return nil
	})
	if err != nil {
		return coremodel.ModelInfo{}, errors.Capture(err)
	}

	info := coremodel.ModelInfo{
		UUID:           coremodel.UUID(model.UUID),
		Name:           model.Name,
		Type:           coremodel.ModelType(model.ModelType),
		Cloud:          model.CloudName,
		CloudType:      model.CloudType,
		CloudRegion:    model.CloudRegion.String,
		CredentialName: model.CredentialName.String,
	}

	if owner := model.CredentialOwnerName; owner != "" {
		info.CredentialOwner, err = user.NewName(owner)
		if err != nil {
			return coremodel.ModelInfo{}, errors.Errorf(
				"parsing model %q owner username %q: %w",
				model.UUID, owner, err)

		}
	}

	info.ControllerUUID, err = uuid.UUIDFromString(model.ControllerUUID)
	if err != nil {
		return coremodel.ModelInfo{}, errors.Errorf(
			"parsing controller uuid %q for model %q: %w",
			model.ControllerUUID, model.UUID, err)

	}

	return info, nil
}

// GetModelType returns the model type for the provided model uuid. If the model
// does not exist then an error satisfying [modelerrors.NotFound] will be
// returned.
func GetModelType(
	ctx context.Context,
	preparer domain.Preparer,
	tx *sqlair.TX,
	uuid coremodel.UUID,
) (coremodel.ModelType, error) {
	mUUID := dbUUID{UUID: uuid.String()}

	stmt, err := preparer.Prepare(`
SELECT &dbModelType.*
FROM v_model AS m
WHERE uuid = $dbUUID.uuid
`, dbModelType{}, mUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	var modelType dbModelType
	if err := tx.Query(ctx, stmt, mUUID).Get(&modelType); errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("%w for uuid %q", modelerrors.NotFound, uuid)
	} else if err != nil {
		return "", errors.Errorf("getting model type for uuid %q: %w", uuid, err)
	}
	return coremodel.ModelType(modelType.Type), nil
}

// GetModel returns the model associated with the provided uuid.
// If the model does not exist then an error satisfying [modelerrors.NotFound]
// will be returned.
func GetModel(
	ctx context.Context,
	tx *sqlair.TX,
	uuid coremodel.UUID,
) (coremodel.Model, error) {
	q := `
SELECT &dbModel.*
FROM v_model
WHERE uuid = $dbModel.uuid
`
	model := dbModel{UUID: uuid.String()}
	stmt, err := sqlair.Prepare(q, model)
	if err != nil {
		return coremodel.Model{}, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, model).Get(&model)
	if errors.Is(err, sqlair.ErrNoRows) {
		return coremodel.Model{}, errors.Errorf("%w for uuid %q", modelerrors.NotFound, uuid)
	} else if err != nil {
		return coremodel.Model{}, errors.Errorf("getting model %q: %w", uuid, err)
	}

	coreModel, err := model.toCoreModel()
	if err != nil {
		return coremodel.Model{}, errors.Capture(err)
	}
	return coreModel, nil
}

// setModelSecretBackend sets the secret backend for a given model id. If the
// secret backend does not exist a [secretbackenderrors.NotFound] error will be
// returned. Should the model already have a secret backend set an error
// satisfying [modelerrors.SecretBackendAlreadySet].
func setModelSecretBackend(
	ctx context.Context,
	preparer domain.Preparer,
	tx *sqlair.TX,
	modelID coremodel.UUID,
	backend string,
) error {

	backendName := dbName{Name: backend}
	var backendUUID dbUUID
	backendFindStmt, err := preparer.Prepare(`
SELECT &dbUUID.uuid from secret_backend WHERE name = $dbName.name
`, backendName, backendUUID)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, backendFindStmt, backendName).Get(&backendUUID)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf(
			"setting model %q secret backend to %q: %w",
			modelID, backend, secretbackenderrors.NotFound,
		)
	} else if err != nil {
		return errors.Errorf(
			"setting model %q secret backend to %q: %w",
			modelID, backend, err,
		)
	}

	modelSecretBackend := dbModelSecretBackend{
		ModelUUID:         modelID.String(),
		SecretBackendUUID: backendUUID.UUID,
	}

	stmt, err := preparer.Prepare(`
INSERT INTO model_secret_backend (*) VALUES ($dbModelSecretBackend.*)
`, modelSecretBackend)
	if err != nil {
		return errors.Capture(err)
	}

	var outcome sqlair.Outcome
	err = tx.Query(ctx, stmt, modelSecretBackend).Get(&outcome)
	if jujudb.IsErrConstraintPrimaryKey(err) {
		return errors.Errorf(
			"model for id %q %w", modelID, modelerrors.SecretBackendAlreadySet,
		)
	} else if jujudb.IsErrConstraintForeignKey(err) {
		return errors.Errorf(
			"%w for id %q while setting model secret backend to %q",
			modelerrors.NotFound,
			modelID,
			backend,
		)
	} else if err != nil {
		return errors.Errorf(
			"setting model for id %q secret backend %q: %w",
			modelID, backend, err,
		)
	}

	if num, err := outcome.Result().RowsAffected(); err != nil {
		return errors.Capture(err)
	} else if num != 1 {
		return errors.Errorf("creating model secret backend record, expected 1 row to be inserted got %d", num)
	}

	return nil
}

// createModel is responsible for creating a new model record
// for the given model UUID. If a model record already exists for the
// given model id then an error satisfying modelerrors.AlreadyExists is
// returned. Conversely, should the owner already have a model that exists with
// the provided name then a modelerrors.AlreadyExists error will be returned. If
// the model type supplied is not found then a errors.NotSupported error is
// returned.
//
// Should the provided cloud and region not be found an error matching
// errors.NotFound will be returned.
// If the GlobalModelCreationArgs contains a non zero value cloud credential this func
// will also attempt to set the model cloud credential using updateCredential. In
// this  scenario the errors from updateCredential are also possible.
// If the model owner does not exist an error satisfying [usererrors.NotFound]
// will be returned.
func createModel(
	ctx context.Context,
	preparer domain.Preparer,
	tx *sqlair.TX,
	modelUUID coremodel.UUID,
	modelType coremodel.ModelType,
	input model.GlobalModelCreationArgs,
) error {
	cloudName := dbName{Name: input.Cloud}

	cloudStmt, err := preparer.Prepare(`SELECT &dbUUID.* FROM cloud WHERE name = $dbName.name`, dbUUID{}, cloudName)
	if err != nil {
		return errors.Capture(err)
	}

	var cloudUUID dbUUID
	if err := tx.Query(ctx, cloudStmt, cloudName).Get(&cloudUUID); errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("%w: %q", clouderrors.NotFound, input.Cloud)
	} else if err != nil {
		return errors.Errorf("getting cloud %q uuid: %w", input.Cloud, err)
	}

	ownerUUID := dbUserUUID{UUID: input.Owner.String()}
	userStmt, err := preparer.Prepare(`
		SELECT &dbUserUUID.uuid
		FROM user
		WHERE uuid = $dbUserUUID.uuid
		AND removed = false
	`, ownerUUID)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, userStmt, ownerUUID).Get(&ownerUUID)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("%w for model owner %q", accesserrors.UserNotFound, input.Owner)
	} else if err != nil {
		return errors.Errorf("getting user uuid for setting model %q owner: %w", input.Name, err)
	}

	// If a model with this name/owner was previously created, clean it up
	// before creating the new model.
	if err := cleanupBrokenModel(ctx, preparer, tx, input.Name, input.Owner); err != nil {
		return errors.Errorf("deleting broken model with name %q and owner %q: %w", input.Name, input.Owner, err)
	}

	model := dbInitialModel{
		UUID:      modelUUID.String(),
		CloudUUID: cloudUUID.UUID,
		ModelType: modelType.String(),
		LifeID:    int(life.Alive),
		Name:      input.Name,
		OwnerUUID: input.Owner.String(),
	}

	stmt, err := preparer.Prepare(`
		INSERT INTO model (uuid,
		            cloud_uuid,
		            model_type_id,
		            life_id,
		            name,
		            owner_uuid)
		SELECT  $dbInitialModel.uuid,
				$dbInitialModel.cloud_uuid,
				model_type.id,
				$dbInitialModel.life_id,
				$dbInitialModel.name,
				$dbInitialModel.owner_uuid
		FROM model_type
		WHERE model_type.type = $dbInitialModel.model_type
		`, model)
	if err != nil {
		return errors.Capture(err)
	}

	var outcome sqlair.Outcome
	err = tx.Query(ctx, stmt, model).Get(&outcome)
	if jujudb.IsErrConstraintPrimaryKey(err) {
		return errors.Errorf("%w for id %q", modelerrors.AlreadyExists, modelUUID)
	} else if jujudb.IsErrConstraintUnique(err) {
		return errors.Errorf("%w for name %q and owner %q", modelerrors.AlreadyExists, input.Name, input.Owner)
	} else if err != nil {
		return errors.Errorf("setting model %q information: %w", modelUUID, err)
	}

	if num, err := outcome.Result().RowsAffected(); err != nil {
		return errors.Capture(err)
	} else if num != 1 {
		return errors.Errorf("creating model metadata, expected 1 row to be inserted, got %d", num)
	}

	if err := setCloudRegion(ctx, preparer, tx, modelUUID, input.Cloud, input.CloudRegion); err != nil {
		return errors.Errorf("setting cloud region for model %q: %w", modelUUID, err)
	}

	if !input.Credential.IsZero() {
		err := updateCredential(ctx, preparer, tx, modelUUID, input.Credential)
		if err != nil {
			return errors.Errorf("setting cloud credential for model %q: %w", modelUUID, err)
		}
	}

	return nil
}

// Delete will remove all data associated with the provided model uuid removing
// the models existence from Juju. If the model does not exist then a error
// satisfying modelerrors.NotFound will be returned.
// The following items are removed as part of deleting a model:
// - Authorized keys for a model.
// - Secret backends
// - Secret backend ref counting
// - Model agent information
// - Model permissions
// - Model login information
func (s *State) Delete(
	ctx context.Context,
	uuid coremodel.UUID,
) error {
	db, err := s.DB()
	if err != nil {
		return errors.Capture(err)
	}

	mUUID := dbUUID{UUID: uuid.String()}

	queries := []string{
		`DELETE FROM model_secret_backend WHERE model_uuid = $dbUUID.uuid`,
		`DELETE FROM secret_backend_reference WHERE model_uuid = $dbUUID.uuid`,
		`DELETE FROM model_authorized_keys WHERE model_uuid = $dbUUID.uuid`,
		`DELETE FROM permission WHERE grant_on = $dbUUID.uuid`,
		`DELETE FROM model_last_login WHERE model_uuid = $dbUUID.uuid`,
	}

	var stmts []*sqlair.Statement
	for _, query := range queries {
		stmt, err := s.Prepare(query, mUUID)
		if err != nil {
			return errors.Capture(err)
		}
		stmts = append(stmts, stmt)
	}

	// The model statement is required, and the output needs to be checked.
	mStmt, err := s.Prepare(`DELETE FROM model WHERE uuid = $dbUUID.uuid`, mUUID)
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := unregisterModelNamespace(ctx, s, tx, uuid); err != nil {
			return errors.Errorf("un-registering model %q database namespaces: %w", uuid, err)
		}

		for _, stmt := range stmts {
			if err := tx.Query(ctx, stmt, mUUID).Run(); errors.Is(err, sqlair.ErrNoRows) {
				continue
			} else if err != nil {
				return errors.Capture(err)
			}
		}

		var outcome sqlair.Outcome
		if err := tx.Query(ctx, mStmt, mUUID).Get(&outcome); errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("%w for uuid %q", modelerrors.NotFound, uuid)
		} else if err != nil {
			return errors.Errorf("deleting model %q: %w", uuid, err)
		}

		if affected, err := outcome.Result().RowsAffected(); err != nil {
			return errors.Errorf("deleting model %q: %w", uuid, err)
		} else if affected == 0 {
			return errors.Errorf("%w for uuid %q", modelerrors.NotFound, uuid)
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
		return errors.Capture(err)
	}

	activator := GetActivator()

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return activator(ctx, s, tx, uuid)
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
type ActivatorFunc func(context.Context, domain.Preparer, *sqlair.TX, coremodel.UUID) error

// GetActivator constructs a [ActivateFunc] that can safely be used over several
// transaction retries.
func GetActivator() ActivatorFunc {
	return func(ctx context.Context, preparer domain.Preparer, tx *sqlair.TX, uuid coremodel.UUID) error {
		mUUID := dbUUID{UUID: uuid.String()}

		existsStmt, err := preparer.Prepare(`
SELECT &dbModelActivated.*
FROM model
WHERE uuid = $dbUUID.uuid
		`, dbModelActivated{}, mUUID)
		if err != nil {
			return errors.Capture(err)
		}

		stmt, err := preparer.Prepare(`
UPDATE model
SET activated = TRUE
WHERE uuid = $dbUUID.uuid
		`, mUUID)
		if err != nil {
			return errors.Capture(err)
		}

		var activated dbModelActivated
		if err := tx.Query(ctx, existsStmt, mUUID).Get(&activated); errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("%w for id %q", modelerrors.NotFound, uuid)
		} else if err != nil {
			return errors.Errorf("determining activated status for model with id %q: %w", uuid, err)
		}

		if activated.Activated {
			return errors.Errorf("%w for id %q", modelerrors.AlreadyActivated, uuid)
		}

		var outcome sqlair.Outcome
		if err := tx.Query(ctx, stmt, mUUID).Get(&outcome); err != nil {
			return errors.Errorf("activating model with id %q: %w", uuid, err)
		}
		if affected, err := outcome.Result().RowsAffected(); err != nil {
			return errors.Errorf("activating model with id %q: %w", uuid, err)
		} else if affected == 0 {
			return errors.Errorf("model not activated")
		}
		return nil
	}
}

// GetModelTypes returns the slice of model.Type's supported by state.
func (s *State) GetModelTypes(ctx context.Context) ([]coremodel.ModelType, error) {
	rval := []coremodel.ModelType{}

	db, err := s.DB()
	if err != nil {
		return rval, errors.Capture(err)
	}

	stmt, err := s.Prepare(`SELECT &dbModelType.* FROM model_type;
`, dbModelType{})
	if err != nil {
		return rval, errors.Capture(err)
	}

	return rval, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result []dbModelType
		if err := tx.Query(ctx, stmt).GetAll(&result); errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}

		for _, r := range result {
			mt := coremodel.ModelType(r.Type)
			if !mt.IsValid() {
				return errors.Errorf("invalid model type %q", r.Type)
			}
			rval = append(rval, mt)
		}
		return nil
	})
}

// ListAllModels returns a slice of all models in the controller. If no models
// exist an empty slice is returned.
func (s *State) ListAllModels(ctx context.Context) ([]coremodel.Model, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	modelStmt, err := s.Prepare(`SELECT &dbModel.* FROM v_model`, dbModel{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	rval := []coremodel.Model{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result []dbModel
		if err := tx.Query(ctx, modelStmt).GetAll(&result); errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}

		for _, r := range result {
			model, err := r.toCoreModel()
			if err != nil {
				return errors.Capture(err)
			}

			rval = append(rval, model)
		}

		return nil
	})

	if err != nil {
		return nil, errors.Errorf("getting all models: %w", err)
	}

	return rval, nil
}

// ListModelUUIDs returns a list of all model UUIDs in the system that are active.
func (s *State) ListModelUUIDs(ctx context.Context) ([]coremodel.UUID, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := s.Prepare(`SELECT &dbUUID.uuid FROM v_model;`, dbUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbResult []dbUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&dbResult)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting all model UUIDs: %w", err)
		}
		return nil
	})

	if err != nil {
		return nil, errors.Capture(err)
	}

	modelUUIDs := make([]coremodel.UUID, 0, len(dbResult))
	for _, r := range dbResult {
		modelUUIDs = append(modelUUIDs, coremodel.UUID(r.UUID))
	}
	return modelUUIDs, nil
}

// ListModelUUIDsForUser returns a list of all the model uuids that a user has
// access to in the controller.
// The following errors can be expected:
// - [accesserrors.UserNotFound] when the user does not exist.
func (s *State) ListModelUUIDsForUser(
	ctx context.Context,
	userUUID user.UUID,
) ([]coremodel.UUID, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	userUUIDVal := dbUUID{UUID: userUUID.String()}
	stmt, err := s.Prepare(`
SELECT &dbUUID.*
FROM  v_model
WHERE owner_uuid = $dbUUID.uuid
OR    uuid IN (SELECT grant_on
               FROM   permission
               WHERE  grant_to = $dbUUID.uuid
               AND    access_type_id IN (0, 1, 3))
`,
		userUUIDVal)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbResult []dbUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := s.checkUserExists(ctx, tx, userUUID)
		if err != nil {
			return errors.Capture(err)
		}
		if !exists {
			return errors.Errorf(
				"user %q does not exist", userUUID,
			).Add(accesserrors.UserNotFound)
		}

		err = tx.Query(ctx, stmt, userUUIDVal).GetAll(&dbResult)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting all user %q model UUIDs: %w", userUUID, err)
		}
		return nil
	})

	if err != nil {
		return nil, errors.Capture(err)
	}

	modelUUIDs := make([]coremodel.UUID, 0, len(dbResult))
	for _, r := range dbResult {
		modelUUIDs = append(modelUUIDs, coremodel.UUID(r.UUID))
	}
	return modelUUIDs, nil
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
		return nil, errors.Capture(err)
	}

	uUUID := dbUUID{UUID: userID.String()}

	modelStmt, err := s.Prepare(`
SELECT &dbModel.*
FROM   v_model
WHERE  owner_uuid = $dbUUID.uuid
OR     uuid IN (SELECT grant_on
                FROM   permission
                WHERE  grant_to = $dbUUID.uuid
                AND    access_type_id IN (0, 1, 3))
`, dbModel{}, uUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var rval []coremodel.Model
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result []dbModel
		if err := tx.Query(ctx, modelStmt, uUUID).GetAll(&result); errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}

		for _, r := range result {
			mod, err := r.toCoreModel()
			if err != nil {
				return errors.Capture(err)
			}

			rval = append(rval, mod)
		}

		return nil
	})

	if err != nil {
		return nil, errors.Errorf("getting models owned by user %q: %w", userID, err)
	}

	return rval, nil
}

// GetModelUsers will retrieve basic information about all users with
// permissions on the given model UUID.
// If the model cannot be found it will return [modelerrors.NotFound].
func (st *State) GetModelUsers(ctx context.Context, modelUUID coremodel.UUID) ([]coremodel.ModelUserInfo, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}
	q := `
SELECT    (u.name, u.display_name, mll.time, p.access_type) AS (&dbModelUserInfo.*)
FROM      v_user_auth u
JOIN      v_permission p ON u.uuid = p.grant_to AND p.grant_on = $dbModelUUIDRef.model_uuid
LEFT JOIN model_last_login mll ON mll.user_uuid = u.uuid AND mll.model_uuid = p.grant_on
WHERE     u.disabled = false
AND       u.removed = false
`

	uuid := dbModelUUIDRef{ModelUUID: modelUUID.String()}
	stmt, err := st.Prepare(q, dbModelUserInfo{}, uuid)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var modelUsers []dbModelUserInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, uuid).GetAll(&modelUsers)
		if errors.Is(err, sqlair.ErrNoRows) {
			if _, err := GetModel(ctx, tx, modelUUID); err != nil {
				return errors.Capture(err)
			}
			return errors.New("no users found on model")
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Errorf("getting model users from database: %w", err)
	}

	var userInfo []coremodel.ModelUserInfo
	for _, modelUser := range modelUsers {
		mui, err := modelUser.toModelUserInfo()
		if err != nil {
			return nil, errors.Capture(err)
		}
		userInfo = append(userInfo, mui)
	}

	return userInfo, nil
}

// GetModelSummary provides summary based information for the model identified
// by the uuid. The information returned is intended to augment the information
// that lives in the model.
// The following error types can be expected:
// - [modelerrors.NotFound] when the model is not found for the given model
// uuid.
func (s *State) GetModelSummary(
	ctx context.Context,
	modelUUID coremodel.UUID,
) (model.ModelSummary, error) {
	db, err := s.DB()
	if err != nil {
		return model.ModelSummary{}, errors.Capture(err)
	}

	q := `
SELECT (m.owner_name, ms.destroying, ms.cloud_credential_invalid,
        ms.cloud_credential_invalid_reason, ms.migrating) AS (&dbModelSummary.*)
FROM   v_model_state ms
JOIN   v_model m ON m.uuid = ms.uuid
WHERE  ms.uuid = $dbModelUUID.uuid
`
	modelUUIDVal := dbModelUUID{UUID: modelUUID.String()}
	stmt, err := s.Prepare(q, dbModelSummary{}, modelUUIDVal)
	if err != nil {
		return model.ModelSummary{}, errors.Capture(err)
	}

	var modelSummaryVals dbModelSummary
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		modelExists, err := s.checkModelExists(ctx, tx, modelUUID)
		if err != nil {
			return errors.Errorf(
				"checking if model %q exists: %w", modelUUID.String(), err,
			)
		}
		if !modelExists {
			return errors.Errorf(
				"model %q does not exist", modelUUID.String(),
			).Add(modelerrors.NotFound)
		}

		err = tx.Query(ctx, stmt, modelUUIDVal).Get(&modelSummaryVals)
		if err != nil {
			return errors.Errorf(
				"getting model %q summary: %w", modelUUID.String(), err,
			)
		}
		return nil
	})

	if err != nil {
		return model.ModelSummary{}, errors.Capture(err)
	}

	ownerName, err := user.NewName(modelSummaryVals.OwnerName)
	if err != nil {
		return model.ModelSummary{}, errors.Errorf(
			"parsing model %q owner username %q: %w",
			modelUUID, modelSummaryVals.OwnerName, err,
		)
	}

	return model.ModelSummary{
		OwnerName: ownerName,
		State: model.ModelState{
			Destroying:                   modelSummaryVals.Destroying,
			HasInvalidCloudCredential:    modelSummaryVals.CredentialInvalid,
			InvalidCloudCredentialReason: modelSummaryVals.CredentialInvalidReason,
			Migrating:                    modelSummaryVals.Migrating,
		},
	}, nil
}

// checkUserExists checks if a user exists in the database. True or false is
// returned indicating this fact.
func (s *State) checkUserExists(
	ctx context.Context,
	tx *sqlair.TX,
	userUUID user.UUID,
) (bool, error) {
	userExistsQ := `
SELECT &dbUserUUID.*
FROM   v_user_auth
WHERE  uuid = $dbUserUUID.uuid
AND    removed = false
`
	userUUIDVal := dbUserUUID{UUID: userUUID.String()}
	userExistsStmt, err := s.Prepare(userExistsQ, userUUIDVal)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, userExistsStmt, userUUIDVal).Get(&userUUIDVal)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return false, errors.Errorf(
			"checking user %q exists: %w", userUUID.String(), err,
		)
	}

	return !errors.Is(err, sqlair.ErrNoRows), nil
}

// GetUserModelSummary returns a summary of the model information that is only
// available in the controller database from the perspective of the user. This
// assumes that the user has access to the model.
// The following error types can be expected:
// - [modelerrors.NotFound] when the model is not found for the given model
// uuid.
// - [accesserrors.UserNotFound] when the user is not found for the given user
// uuid.
// - [accesserrors.AccessNotFound] when the user does not have access to the
// model.
func (s *State) GetUserModelSummary(
	ctx context.Context,
	userUUID user.UUID,
	modelUUID coremodel.UUID,
) (model.UserModelSummary, error) {
	db, err := s.DB()
	if err != nil {
		return model.UserModelSummary{}, errors.Capture(err)
	}

	q := `
SELECT    (p.access_type, mll.time, ms.destroying, ms.cloud_credential_invalid,
           ms.cloud_credential_invalid_reason, ms.migrating, m.owner_name
          ) AS (&dbUserModelSummary.*)
FROM      v_user_auth u
JOIN      v_permission p ON p.grant_to = u.uuid
JOIN      v_model_state ms ON ms.uuid = p.grant_on
JOIN      v_model m ON m.uuid = ms.uuid
LEFT JOIN model_last_login mll ON ms.uuid = mll.model_uuid AND mll.user_uuid = u.uuid
WHERE     u.removed = false
AND       u.uuid = $dbUserUUID.uuid
AND       ms.uuid = $dbModelUUID.uuid
`

	userUUIDVal := dbUserUUID{UUID: userUUID.String()}
	modelUUIDVal := dbModelUUID{UUID: modelUUID.String()}
	stmt, err := s.Prepare(q, dbUserModelSummary{}, userUUIDVal, modelUUIDVal)
	if err != nil {
		return model.UserModelSummary{}, errors.Capture(err)
	}

	var userModelSummaryVals dbUserModelSummary
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := s.checkUserExists(ctx, tx, userUUID)
		if err != nil {
			return errors.Capture(err)
		}
		if !exists {
			return errors.Errorf(
				"user %q does not exist", userUUID.String(),
			).Add(accesserrors.UserNotFound)
		}

		modelExists, err := s.checkModelExists(ctx, tx, modelUUID)
		if err != nil {
			return errors.Capture(err)
		}
		if !modelExists {
			return errors.Errorf(
				"model %q does not exist", modelUUID,
			).Add(modelerrors.NotFound)
		}

		err = tx.Query(ctx, stmt, userUUIDVal, modelUUIDVal).Get(&userModelSummaryVals)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"user %q does not have access to model %q",
				userUUID, modelUUID,
			).Add(accesserrors.AccessNotFound)
		} else if err != nil {
			return errors.Errorf(
				"getting user %q model %q summary: %w",
				userUUID, modelUUID, err,
			)
		}
		return nil
	})
	if err != nil {
		return model.UserModelSummary{}, errors.Capture(err)
	}

	ownerName, err := user.NewName(userModelSummaryVals.OwnerName)
	if err != nil {
		return model.UserModelSummary{}, errors.Errorf(
			"parsing model %q owner username %q: %w",
			modelUUID, userModelSummaryVals.OwnerName, err,
		)
	}

	return model.UserModelSummary{
		ModelSummary: model.ModelSummary{
			OwnerName: ownerName,
			State: model.ModelState{
				Destroying:                   userModelSummaryVals.Destroying,
				Migrating:                    userModelSummaryVals.Migrating,
				HasInvalidCloudCredential:    userModelSummaryVals.CredentialInvalid,
				InvalidCloudCredentialReason: userModelSummaryVals.CredentialInvalidReason,
			},
		},
		UserAccess:         userModelSummaryVals.Access,
		UserLastConnection: userModelSummaryVals.UserLastConnection,
	}, nil
}

// GetModelCloudInfo returns the cloud name, cloud region name.  If no model exists for the provided uuid a
// [modelerrors.NotFound] error is returned.
func (s *State) GetModelCloudInfo(
	ctx context.Context,
	uuid coremodel.UUID,
) (string, string, error) {
	db, err := s.DB()
	if err != nil {
		return "", "", errors.Capture(err)
	}

	args := dbModelUUID{
		UUID: uuid.String(),
	}

	stmt, err := s.Prepare(`
SELECT &dbCloudCredential.*
FROM   v_model
WHERE  uuid = $dbModelUUID.uuid
`, dbCloudCredential{}, args)
	if err != nil {
		return "", "", errors.Capture(err)
	}

	var (
		cloudName       string
		cloudRegionName string
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result dbCloudCredential
		err := tx.Query(ctx, stmt, args).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return modelerrors.NotFound
		} else if err != nil {
			return err
		}

		cloudName = result.Name
		cloudRegionName = result.CloudRegionName
		return nil
	})
	if err != nil {
		return "", "", errors.Errorf(
			"getting model %q cloud name and credential: %w", uuid, err,
		)
	}
	return cloudName, cloudRegionName, nil
}

// GetModelCloudAndCredential returns the cloud and credential UUID for the model.
// The following errors can be expected:
// - [modelerrors.NotFound] if the model is not found.
func (st *State) GetModelCloudAndCredential(
	ctx context.Context,
	modelUUID coremodel.UUID,
) (corecloud.UUID, credential.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", "", errors.Capture(err)
	}

	cloudCred := dbModelCloudCredentialUUID{
		ModelUUID: modelUUID,
	}
	query := `
SELECT m.* AS &dbModelCloudCredentialUUID.*
FROM   v_model m
WHERE  m.uuid = $dbModelCloudCredentialUUID.uuid
`
	stmt, err := st.Prepare(query, cloudCred)
	if err != nil {
		return "", "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, cloudCred).Get(&cloudCred)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("model %q not found", modelUUID).Add(modelerrors.NotFound)
		}
		return errors.Capture(err)
	})
	if err != nil {
		return "", "", errors.Capture(err)
	}
	return cloudCred.CloudUUID, cloudCred.CredentialUUID, nil
}

// NamespaceForModel returns the database namespace that is provisioned for a
// model id. If no model is found for the given id then a [modelerrors.NotFound]
// error is returned. If no namespace has been provisioned for the model then a
// [modelerrors.ModelNamespaceNotFound] error is returned.
func (s *State) NamespaceForModel(ctx context.Context, id coremodel.UUID) (string, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	mUUID := dbUUID{UUID: id.String()}

	stmt, err := s.Prepare(`
SELECT m.uuid AS &dbModelNamespace.model_uuid,
       mn.namespace AS &dbModelNamespace.namespace
FROM model m
LEFT JOIN model_namespace mn ON m.uuid = mn.model_uuid
WHERE m.uuid = $dbUUID.uuid
`, dbModelNamespace{}, mUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	var namespace sql.NullString
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result dbModelNamespace
		if err := tx.Query(ctx, stmt, mUUID).Get(&result); errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("%w for id %q", modelerrors.NotFound, id)
		} else if err != nil {
			return errors.Errorf("getting database namespace for model %q: %w", id, err)
		}
		namespace = result.Namespace
		return nil
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	if !namespace.Valid {
		return "", errors.Errorf(
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
	preparer domain.Preparer,
	tx *sqlair.TX,
	uuid coremodel.UUID,
) (string, error) {
	modelNamespace := dbModelNamespace{
		UUID: uuid.String(),
		Namespace: sql.NullString{
			String: uuid.String(),
			Valid:  true,
		},
	}
	insertNamespaceStmt, err := preparer.Prepare(`
INSERT INTO namespace_list (namespace) VALUES ($dbModelNamespace.namespace)
	`, modelNamespace)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, insertNamespaceStmt, modelNamespace).Run()
	if jujudb.IsErrConstraintPrimaryKey(err) {
		return "", errors.Errorf("database namespace already registered for model %q", uuid)
	} else if err != nil {
		return "", errors.Errorf("registering database namespace for model %q: %w", uuid, err)
	}

	insertModelNamespaceStmt, err := preparer.Prepare(`
INSERT INTO model_namespace (*) VALUES ($dbModelNamespace.*)
	`, modelNamespace)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, insertModelNamespaceStmt, modelNamespace).Run()
	if jujudb.IsErrConstraintUnique(err) {
		return "", errors.Errorf("model %q already has a database namespace registered", uuid)
	} else if jujudb.IsErrConstraintForeignKey(err) {
		return "", errors.Errorf("model %q does not exist", uuid).Add(modelerrors.NotFound)
	} else if err != nil {
		return "", errors.Errorf("associating database namespace with model %q, %w", uuid, err)
	}

	return uuid.String(), nil
}

// cleanupBrokenModel removes broken models from the database. This is here to
// allow models to be recreated that may have failed during the full model
// creation process and never activated. We will only ever allow this to happen
// if the model is not activated.
func cleanupBrokenModel(
	ctx context.Context,
	preparer domain.Preparer,
	tx *sqlair.TX,
	modelName string, modelOwner user.UUID,
) error {
	var uuid = dbUUID{}
	nameAndOwner := dbModelNameAndOwner{
		Name:      modelName,
		OwnerUUID: modelOwner.String(),
	}
	// Find the UUID for the broken model
	findBrokenModelStmt, err := preparer.Prepare(`
SELECT &dbUUID.uuid FROM model
WHERE name = $dbModelNameAndOwner.name
AND owner_uuid = $dbModelNameAndOwner.owner_uuid
AND activated = false
`, uuid, nameAndOwner)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, findBrokenModelStmt, nameAndOwner).Get(&uuid)
	if errors.Is(err, sqlair.ErrNoRows) {
		// Model doesn't exist so nothing to cleanup.
		return nil
	}
	if err != nil {
		return errors.Errorf("finding broken model for name %q and owner %q: %w",
			modelName, modelOwner, err,
		)
	}

	// Delete model namespace
	deleteBadStateModelNamespace, err := preparer.Prepare(`
DELETE FROM model_namespace
WHERE model_uuid = $dbUUID.uuid
`, uuid)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, deleteBadStateModelNamespace, uuid).Run()
	if err != nil {
		return errors.Errorf("cleaning up bad model namespace for model with UUID %q: %w",
			uuid.UUID, err,
		)
	}

	// Delete model secret backend
	deleteBrokenModelSecretBackend, err := preparer.Prepare(`
DELETE FROM model_secret_backend
WHERE model_uuid = $dbUUID.uuid
`, uuid)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, deleteBrokenModelSecretBackend, uuid).Run()
	if err != nil {
		return errors.Errorf("cleaning up model secret backend for model with UUID %q: %w",
			uuid.UUID, err,
		)
	}

	// Delete model last login
	deleteBrokenModelLastLogin, err := sqlair.Prepare(`
DELETE FROM model_last_login
WHERE model_uuid = $dbUUID.uuid
`, uuid)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, deleteBrokenModelLastLogin, uuid).Run()
	if err != nil {
		return errors.Errorf("cleaning up model last login for model with UUID %q: %w",
			uuid.UUID, err,
		)
	}

	// Finally, delete the model from the model table.
	deleteBadStateModel, err := preparer.Prepare(`
DELETE FROM model
WHERE uuid = $dbUUID.uuid
`, uuid)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, deleteBadStateModel, uuid).Run()
	if err != nil {
		return errors.Errorf("cleaning up bad model state for model with UUID %q: %w",
			uuid.UUID, err,
		)
	}

	return nil
}

// setCloudRegion is responsible for setting a model's cloud region. This
// operation can only be performed once and will fail with an error that
// satisfies errors.AlreadyExists on subsequent tries.
// If no cloud region is found for the model's cloud then an error that satisfies
// errors.NotFound will be returned.
func setCloudRegion(
	ctx context.Context,
	preparer domain.Preparer,
	tx *sqlair.TX,
	uuid coremodel.UUID,
	name, region string,
) error {
	modelUUID := dbUUID{UUID: uuid.String()}
	// If the cloud region is not provided we will attempt to set the default
	// cloud region for the model from the controller model.
	var cloudRegionUUID dbCloudRegionUUID
	if region == "" {
		// Ensure that the controller cloud name is the same as the model cloud
		// name.
		cloudName := dbName{
			Name: name,
		}

		stmt, err := preparer.Prepare(`
SELECT m.cloud_region_uuid AS &dbCloudRegionUUID.uuid
FROM   model m
JOIN   cloud c ON m.cloud_uuid = c.uuid
WHERE  m.name = 'controller'
AND    c.name = $dbName.name
`, cloudName, cloudRegionUUID)
		if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, stmt, cloudName).Get(&cloudRegionUUID); errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("getting controller cloud region uuid: %w", err)
		}

		// If the region is empty, we will not set a cloud region for the model
		// and will skip it.
		if cloudRegionUUID.CloudRegionUUID == "" {
			return nil
		}
	} else {
		cloudRegionName := dbName{
			Name: region,
		}
		stmt, err := preparer.Prepare(`
SELECT cr.uuid AS &dbCloudRegionUUID.uuid
FROM cloud_region cr
INNER JOIN cloud c
ON c.uuid = cr.cloud_uuid
INNER JOIN model m
ON m.cloud_uuid = c.uuid
WHERE m.uuid = $dbUUID.uuid
AND cr.name = $dbName.name
`, cloudRegionName, modelUUID, cloudRegionUUID)
		if err != nil {
			return errors.Capture(err)
		}

		if err := tx.Query(ctx, stmt, modelUUID, cloudRegionName).Get(&cloudRegionUUID); errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("cloud region %q for model uuid %q not found", region, uuid).Add(coreerrors.NotFound)
		} else if err != nil {
			return errors.Errorf("getting cloud region %q uuid for model %q: %w", region, uuid, err)
		}
	}

	modelMetadataStmt, err := preparer.Prepare(`
UPDATE model
SET cloud_region_uuid = $dbCloudRegionUUID.uuid
WHERE uuid = $dbUUID.uuid
AND cloud_region_uuid IS NULL
`, modelUUID, cloudRegionUUID)
	if err != nil {
		return errors.Capture(err)
	}

	var outcome sqlair.Outcome
	err = tx.Query(ctx, modelMetadataStmt, cloudRegionUUID, modelUUID).Get(&outcome)
	if err != nil {
		return errors.Errorf(
			"setting cloud region uuid %q for model uuid %q: %w",
			cloudRegionUUID.CloudRegionUUID,
			uuid,
			err,
		)
	}
	if num, err := outcome.Result().RowsAffected(); err != nil {
		return errors.Capture(err)
	} else if num != 1 {
		return errors.Errorf(
			"model %q already has a cloud region set",
			uuid,
		).Add(coreerrors.AlreadyExists)
	}
	return nil
}

// unregisterModelNamespace is responsible for de-registering a models intent
// to be associated with any database namespaces going forward. If the model
// does not exist or has no namespace associations no error is returned.
func unregisterModelNamespace(
	ctx context.Context,
	preparer domain.Preparer,
	tx *sqlair.TX,
	uuid coremodel.UUID,
) error {
	mUUID := dbUUID{UUID: uuid.String()}

	stmt, err := preparer.Prepare("DELETE from model_namespace WHERE model_uuid = $dbUUID.uuid", mUUID)
	if err != nil {
		return errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, mUUID).Run(); err != nil {
		return errors.Capture(err)
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
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return updateCredential(ctx, s, tx, uuid, key)
	})
}

// CloudSupportsAuthType allows the caller to ask if a given auth type is
// currently supported by the given cloud name. If no cloud is found for
// name an error matching [clouderrors.NotFound] is returned.
func (s *State) CloudSupportsAuthType(
	ctx context.Context,
	cloudName string,
	authType cloud.AuthType,
) (bool, error) {
	db, err := s.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	supports := false
	return supports, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		supports, err = CloudSupportsAuthType(ctx, s, tx, cloudName, authType)
		return err
	})
}

// CloudSupportsAuthType allows the caller to ask if a given auth type is
// currently supported by the given cloud name. If no cloud is found for
// name an error matching [clouderrors.NotFound] is returned.
func CloudSupportsAuthType(
	ctx context.Context,
	preparer domain.Preparer,
	tx *sqlair.TX,
	cloudName string,
	authType cloud.AuthType,
) (bool, error) {
	type dbCloudUUID struct {
		UUID string `db:"uuid"`
	}
	type dbCloudName struct {
		Name string `db:"name"`
	}

	cloudNameVal := dbCloudName{cloudName}
	cloudUUIDVal := dbCloudUUID{}

	cloudExistsStmt, err := preparer.Prepare(`
SELECT &dbCloudUUID.* FROM cloud WHERE name = $dbCloudName.name
`, cloudUUIDVal, cloudNameVal)
	if err != nil {
		return false, errors.Capture(err)
	}

	type dbCloudAuthType struct {
		AuthType string `db:"type"`
	}
	authTypeVal := dbCloudAuthType{authType.String()}

	authTypeStmt := `
SELECT auth_type.type AS &dbCloudAuthType.type
FROM cloud
JOIN cloud_auth_type ON cloud.uuid = cloud_auth_type.cloud_uuid
JOIN auth_type ON cloud_auth_type.auth_type_id = auth_type.id
WHERE cloud.uuid = $dbCloudUUID.uuid
AND auth_type.type = $dbCloudAuthType.type
`
	selectCloudAuthTypeStmt, err := preparer.Prepare(authTypeStmt, cloudUUIDVal, authTypeVal)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, cloudExistsStmt, cloudNameVal).Get(&cloudUUIDVal)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, errors.Errorf("cloud %q does not exist", cloudName).Add(clouderrors.NotFound)
	} else if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(
		ctx,
		selectCloudAuthTypeStmt,
		authTypeVal,
		cloudUUIDVal,
	).Get(&authTypeVal)

	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	return true, nil
}

// updateCredential is responsible for updating the cloud credential in use
// by model. If the cloud credential is not found an error that satisfies
// errors.NotFound is returned.
// If the credential being updated to is not of the same cloud that is currently
// set for the model then an error that satisfies errors.NotValid is returned.
func updateCredential(
	ctx context.Context,
	preparer domain.Preparer,
	tx *sqlair.TX,
	uuid coremodel.UUID,
	key credential.Key,
) error {
	selectArgs := dbCredKey{
		CloudName:           key.Cloud,
		OwnerName:           key.Owner.Name(),
		CloudCredentialName: key.Name,
	}

	cloudCredUUIDStmt, err := preparer.Prepare(`
SELECT cc.uuid AS &dbUpdateCredentialResult.cloud_credential_uuid,
       c.uuid AS &dbUpdateCredentialResult.cloud_uuid
FROM cloud_credential cc
INNER JOIN cloud c
ON c.uuid = cc.cloud_uuid
INNER JOIN user u
ON cc.owner_uuid = u.uuid
WHERE c.name = $dbCredKey.cloud_name
AND u.name = $dbCredKey.owner_name
AND u.removed = false
AND cc.name = $dbCredKey.cloud_credential_name
`, selectArgs, dbUpdateCredentialResult{})
	if err != nil {
		return errors.Errorf("preparing select cloud credential statement: %w", err)
	}

	var result dbUpdateCredentialResult
	err = tx.Query(ctx, cloudCredUUIDStmt, selectArgs).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf(
			"%w cloud credential %q",
			coreerrors.NotFound, key,
		).Add(err)
	} else if err != nil {
		return errors.Errorf(
			"getting cloud credential uuid for %q: %w",
			key, err,
		)
	}

	updateArgs := dbUpdateCredential{
		UUID:                uuid.String(),
		CloudCredentialUUID: result.CloudCredentialUUID,
		CloudUUID:           result.CloudUUID,
	}

	updateCloudCredStmt, err := preparer.Prepare(`
UPDATE model
SET cloud_credential_uuid = $dbUpdateCredential.cloud_credential_uuid
WHERE uuid= $dbUpdateCredential.uuid
AND cloud_uuid = $dbUpdateCredential.cloud_uuid
`, updateArgs)
	if err != nil {
		return errors.Errorf("preparing update model cloud credential statement: %w", err)
	}

	var outcome sqlair.Outcome
	if err := tx.Query(ctx, updateCloudCredStmt, updateArgs).Get(&outcome); err != nil {
		return errors.Errorf(
			"setting cloud credential %q for model %q: %w",
			key, uuid, err)
	}
	if num, err := outcome.Result().RowsAffected(); err != nil {
		return errors.Capture(err)
	} else if num != 1 {
		return errors.Errorf(
			"%w model %q has different cloud to credential %q",
			coreerrors.NotValid, uuid, key)
	}
	return nil
}

// addAdminPermission adds an Admin permission for the supplied user to the
// given model. If the user already has admin permissions onto the model a
// [usererrors.PermissionAlreadyExists] error is returned.
func addAdminPermissions(
	ctx context.Context,
	preparer domain.Preparer,
	tx *sqlair.TX,
	modelUUID coremodel.UUID,
	ownerUUID user.UUID,
) error {
	permUUID, err := uuid.NewUUID()
	if err != nil {
		return err
	}

	adminPermission := dbPermission{
		UUID:       permUUID.String(),
		GrantOn:    modelUUID.String(),
		GrantTo:    ownerUUID.String(),
		AccessType: permission.AdminAccess.String(),
		ObjectType: permission.Model.String(),
	}

	permStmt, err := preparer.Prepare(`
INSERT INTO permission (uuid, access_type_id, object_type_id, grant_to, grant_on)
SELECT $dbPermission.uuid, at.id, ot.id, $dbPermission.grant_to, $dbPermission.grant_on
FROM   permission_access_type at,
       permission_object_type ot
WHERE  at.type = $dbPermission.access_type
AND    ot.type = $dbPermission.object_type
`, adminPermission)
	if err != nil {
		return errors.Capture(err)
	}

	var outcome sqlair.Outcome
	err = tx.Query(ctx, permStmt, adminPermission).Get(&outcome)
	if jujudb.IsErrConstraintUnique(err) {
		return errors.Errorf("%w for model %q and owner %q", accesserrors.PermissionAlreadyExists, modelUUID, ownerUUID)
	} else if err != nil {
		return errors.Errorf("setting permission for model %q: %w", modelUUID, err)
	}

	if num, err := outcome.Result().RowsAffected(); err != nil {
		return errors.Capture(err)
	} else if num != 1 {
		return errors.Errorf("creating model permission metadata, expected 1 row to be inserted, got %d", num)
	}
	return nil
}

// InitialWatchActivatedModelsStatement returns a SQL statement that will get
// all the activated models uuids in the controller.
func (st *State) InitialWatchActivatedModelsStatement() (string, string) {
	return "model", "SELECT uuid from model WHERE activated = true"
}

// InitialWatchModelTableName returns the name of the model table to be used
// for the initial watch statement.
func (st *State) InitialWatchModelTableName() string {
	return "model"
}

// GetActivatedModelUUIDs returns the subset of model uuids from the supplied
// list that are activated. If no models associated with the uuids are activated
// then an empty slice is returned.
func (st *State) GetActivatedModelUUIDs(ctx context.Context, uuids []coremodel.UUID) ([]coremodel.UUID, error) {
	if len(uuids) == 0 {
		return nil, nil
	}
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	type modelUUID struct {
		UUID coremodel.UUID `db:"uuid"`
	}

	stmt, err := st.Prepare(`
SELECT &modelUUID.*
FROM   v_model
WHERE uuid IN ($S[:])
`, sqlair.S{}, modelUUID{})

	if err != nil {
		return nil, errors.Capture(err)
	}

	args := sqlair.S(transform.Slice(uuids, func(s coremodel.UUID) any { return s }))
	modelUUIDs := []modelUUID{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, args).GetAll(&modelUUIDs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Errorf("getting activated model UUIDs: %w", err)
	}

	res := transform.Slice(modelUUIDs, func(m modelUUID) coremodel.UUID { return m.UUID })
	return res, nil
}

// GetModelLife returns the life associated with the provided uuid.
// The following error types can be expected to be returned:
// - [modelerrors.NotFound]: When the model does not exist.
// - [modelerrors.NotActivated]: When the model has not been activated.
func (st *State) GetModelLife(ctx context.Context, uuid coremodel.UUID) (life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return -1, errors.Capture(err)
	}

	life := dbModelLife{
		UUID: uuid,
	}

	stmt, err := st.Prepare(`
SELECT &dbModelLife.*
FROM   model
WHERE uuid = $dbModelLife.uuid;
	`, dbModelLife{})
	if err != nil {
		return -1, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, life).Get(&life)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("%w for %q", modelerrors.NotFound, uuid)
		} else if err != nil {
			return errors.Capture(err)
		} else if !life.Activated {
			return errors.Errorf("%w for %q", modelerrors.NotActivated, uuid)
		}

		return nil
	})
	if err != nil {
		return -1, errors.Errorf("getting model life for %q: %w", uuid, err)
	}

	return life.Life, nil
}
