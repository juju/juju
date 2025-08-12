// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/keymanager"
	keyerrors "github.com/juju/juju/domain/keymanager/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	jujudb "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
)

// State represents a type for interacting with the underlying state of a user's
// public key in a model.
type State struct {
	*domain.StateBase
}

// NewState is responsible for constructing a new [State] that can be used with
// this domains corresponding service.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// checkModelExists is responsible for checking that a given model exists within
// the controller. If the model exists no error is returned otherwise an error
// satisfying [modelerrors.NotFound] is returned.
func (s *State) checkModelExists(
	ctx context.Context,
	modelUUID model.UUID,
	tx *sqlair.TX,
) error {
	modelUUIDVal := modelUUIDValue{UUID: modelUUID.String()}

	modelExistsStmt, err := s.Prepare(`
SELECT (uuid) AS (&modelUUIDValue.model_uuid)
FROM v_model
WHERE uuid = $modelUUIDValue.model_uuid
`, modelUUIDVal)
	if err != nil {
		return errors.Errorf(
			"creating model exists statement when checking if model %q exists: %w",
			modelUUID, err,
		)
	}

	err = tx.Query(ctx, modelExistsStmt, modelUUIDVal).Get(&modelUUIDVal)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf(
			"checking model %q exists, model not found", modelUUID,
		).Add(modelerrors.NotFound)
	} else if err != nil {
		return errors.Errorf(
			"checking model %q exists: %w", modelUUID, err,
		)
	}

	return nil
}

// checkUserExists is responsible for checking that a given user exists with the
// controller. If the user exists then no error is returned otherwise an error
// satisfying [accesserrors.UserNotFound] is returned.
func (s *State) checkUserExists(
	ctx context.Context,
	userUUID user.UUID,
	tx *sqlair.TX,
) error {
	userUUIDVal := userUUIDValue{UUID: userUUID.String()}

	userExistsStmt, err := s.Prepare(`
SELECT (uuid) AS (&userUUIDValue.user_uuid)
FROM v_user_auth
WHERE uuid = $userUUIDValue.user_uuid
AND removed = false
`, userUUIDVal)
	if err != nil {
		return errors.Errorf(
			"creating user exists statement when checking if user %q exists: %w",
			userUUID, err,
		)
	}

	err = tx.Query(ctx, userExistsStmt, userUUIDVal).Get(&userUUIDVal)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf(
			"checking user %q exists, user not found",
			userUUID,
		).Add(accesserrors.UserNotFound)
	} else if err != nil {
		return errors.Errorf(
			"checking user %q exists: %w",
			userUUID, err,
		)
	}

	return nil
}

// ensureUserPublicKey ensures that a public key exists for a user returning the
// unique id to represent that key within the database.
func (s *State) ensureUserPublicKey(
	ctx context.Context,
	key userPublicKeyInsert,
	tx *sqlair.TX,
) (int64, error) {
	insertStmt, err := s.Prepare(`
INSERT INTO user_public_ssh_key (comment,
                                 fingerprint,
						         public_key,
                                 user_uuid,
                                 fingerprint_hash_algorithm_id)
SELECT $userPublicKeyInsert.comment,
       $userPublicKeyInsert.fingerprint,
       $userPublicKeyInsert.public_key,
       $userPublicKeyInsert.user_uuid,
       s.id	
FROM ssh_fingerprint_hash_algorithm s
WHERE s.algorithm = $userPublicKeyInsert.algorithm
`, userPublicKeyInsert{})

	if err != nil {
		return 0, errors.Errorf(
			"preparing insert statement for ensuring user public key: %w",
			err,
		)
	}

	selectExistingIdStmt, err := s.Prepare(`
SELECT &userPublicKeyId.id
FROM user_public_ssh_key
WHERE user_uuid = $userPublicKeyInsert.user_uuid
AND fingerprint = $userPublicKeyInsert.fingerprint
`, userPublicKeyId{}, userPublicKeyInsert{})

	if err != nil {
		return 0, errors.Errorf(
			"preparing select existing id statement for ensuring user public key: %w",
			err,
		)
	}

	row := userPublicKeyId{}
	err = tx.Query(ctx, selectExistingIdStmt, key).Get(&row)

	// If there is no errors then we can safely assume the key already
	// exists and nothing more needs to be done.
	if err == nil {
		return row.Id, nil
	} else if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return 0, errors.Errorf(
			"fetching existing user %q key id when ensuring public key: %w",
			key.UserId, err,
		)
	}

	outcome := sqlair.Outcome{}
	err = tx.Query(ctx, insertStmt, key).Get(&outcome)
	if err != nil {
		return 0, errors.Errorf(
			"inserting public key for user %q: %w", key.UserId, err,
		)
	}

	var lastInsertId int64
	lastInsertId, err = outcome.Result().LastInsertId()

	if err != nil {
		return 0, errors.Errorf(
			"fetching id for newly inserted public key on user %q: %w",
			key.UserId, err,
		)
	}
	return lastInsertId, nil
}

// AddPublicKeyForUser is responsible for adding one or more ssh public keys for
// a user to a given model.
// The following errors can be expected:
// - [keyerrors.PublicKeyAlreadyExists] - When one of the public keys being added
// for a user already exists on the model.
// - [accesserrors.UserNotFound] - When the user does not exist.
// - [modelerrors.NotFound] - When the model does not exist.
func (s *State) AddPublicKeysForUser(
	ctx context.Context,
	modelUUID model.UUID,
	userUUID user.UUID,
	publicKeys []keymanager.PublicKey,
) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Errorf(
			"getting database for adding public keys to user %q on model %q: %w",
			userUUID,
			modelUUID,
			err,
		)
	}

	insertModelAuthorisedKeyStmt, err := s.Prepare(`
INSERT INTO model_authorized_keys (*) VALUES ($modelAuthorizedKey.*)
	`, modelAuthorizedKey{})
	if err != nil {
		return errors.Errorf(
			"preparing insert statement for adding user %q public keys to model %q: %w",
			userUUID, modelUUID, err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := s.checkUserExists(ctx, userUUID, tx)
		if err != nil {
			return errors.Errorf(
				"adding public keys for user %q to model %q: %w",
				userUUID, modelUUID, err,
			)
		}

		err = s.checkModelExists(ctx, modelUUID, tx)
		if err != nil {
			return errors.Errorf(
				"adding public keys for user %q to model %q: %w",
				userUUID, modelUUID, err,
			)
		}

		keyIds := []int64{}
		for i, publicKey := range publicKeys {
			row := userPublicKeyInsert{
				Comment:                  publicKey.Comment,
				FingerprintHashAlgorithm: publicKey.FingerprintHash.String(),
				Fingerprint:              publicKey.Fingerprint,
				PublicKey:                publicKey.Key,
				UserId:                   userUUID.String(),
			}

			keyId, err := s.ensureUserPublicKey(ctx, row, tx)
			if err != nil {
				return errors.Errorf(
					"ensuring user %q public key %d on model %q: %w",
					userUUID, i, modelUUID, err,
				)
			}

			keyIds = append(keyIds, keyId)
		}

		for i, keyId := range keyIds {
			row := modelAuthorizedKey{
				UserPublicSSHKeyId: keyId,
				ModelUUID:          modelUUID.String(),
			}
			err := tx.Query(ctx, insertModelAuthorisedKeyStmt, row).Run()
			if jujudb.IsErrConstraintPrimaryKey(err) {
				return errors.Errorf(
					"adding key %d for user %q to model %q, key already exists",
					i, userUUID, modelUUID,
				).Add(keyerrors.PublicKeyAlreadyExists)
			} else if err != nil {
				return errors.Errorf(
					"adding key %d for user %q to model %q: %w",
					i, userUUID, modelUUID, err,
				)
			}
		}

		return nil
	})

	return err
}

// EnsurePublicKeysForUser will attempt to add the given set of public
// keys for the user to the specified model . If the user already has the public
// key in the model it will be skipped and no
// [keyerrors.PublicKeyAlreadyExists] error will be returned.
// The following errors can be expected:
// - [accesserrors.UserNotFound] - When the user does not exist.
// - [modelerrors.NotFound] - When the model does not exist.
func (s *State) EnsurePublicKeysForUser(
	ctx context.Context,
	modelUUID model.UUID,
	userUUID user.UUID,
	publicKeys []keymanager.PublicKey,
) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Errorf(
			"getting database for ensuring public keys on user %q in model %q: %w",
			userUUID,
			modelUUID,
			err,
		)
	}

	insertModelAuthorisedKeyStmt, err := s.Prepare(`
INSERT INTO model_authorized_keys (*)
VALUES ($modelAuthorizedKey.*)
ON CONFLICT DO NOTHING
`, modelAuthorizedKey{})

	if err != nil {
		return errors.Errorf(
			"preparing insert statement for ensuring user %q public keys on model %q: %w",
			userUUID, modelUUID, err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := s.checkUserExists(ctx, userUUID, tx)
		if err != nil {
			return errors.Errorf(
				"ensuring public keys for user %q on model %q: %w",
				userUUID, modelUUID, err,
			)
		}

		err = s.checkModelExists(ctx, modelUUID, tx)
		if err != nil {
			return errors.Errorf(
				"ensuring public keys for user %q on model %q: %w",
				userUUID, modelUUID, err,
			)
		}

		keyIds := []int64{}
		// We can't perform a bulk insert here because of the foreign key lookup
		// for the algorithm. It could be done with a temp table but this is not
		// considered a hot path.
		for i, publicKey := range publicKeys {
			row := userPublicKeyInsert{
				Comment:                  publicKey.Comment,
				FingerprintHashAlgorithm: publicKey.FingerprintHash.String(),
				Fingerprint:              publicKey.Fingerprint,
				PublicKey:                publicKey.Key,
				UserId:                   userUUID.String(),
			}

			keyId, err := s.ensureUserPublicKey(ctx, row, tx)
			if err != nil {
				return errors.Errorf(
					"ensuring user %q public key %d on model %q: %w",
					userUUID, i, modelUUID, err,
				)
			}
			keyIds = append(keyIds, keyId)
		}

		for i, keyId := range keyIds {
			row := modelAuthorizedKey{
				UserPublicSSHKeyId: keyId,
				ModelUUID:          modelUUID.String(),
			}

			err := tx.Query(ctx, insertModelAuthorisedKeyStmt, row).Run()
			if err != nil && !jujudb.IsErrConstraintPrimaryKey(err) {
				return errors.Errorf(
					"ensuring key %d for user %q on model %q: %w",
					i, userUUID, modelUUID, err,
				)
			}
		}

		return nil
	})

	return err
}

// GetAllUsersPublicKeys returns all of the public keys that are in a model and
// their respective username. This is useful for building a view during model
// migration. The following errors can be expected:
// - [modelerrors.NotFound] - When no model exists for the uuid.
func (s *State) GetAllUsersPublicKeys(
	ctx context.Context,
	modelUUID model.UUID,
) (map[user.Name][]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, err
	}

	modelUUIDVal := modelUUIDValue{UUID: modelUUID.String()}

	stmt, err := s.Prepare(`
SELECT (u.name, mak.public_key) AS (&userPublicKey.*)
FROM v_model_authorized_keys AS mak
JOIN user AS u ON u.uuid = mak.user_uuid
WHERE mak.model_uuid = $modelUUIDValue.model_uuid
`, modelUUIDVal, userPublicKey{})

	if err != nil {
		return nil, errors.Errorf(
			"preparing select statement for getting all users keys from model %q: %w",
			modelUUID, err,
		)
	}
	usersKeys := []userPublicKey{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := s.checkModelExists(ctx, modelUUID, tx)
		if err != nil {
			return errors.Errorf(
				"checking model %q exists when getting all users public keys: %w",
				modelUUID, err,
			)
		}

		err = tx.Query(ctx, stmt, modelUUIDVal).GetAll(&usersKeys)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"getting all users public keys for model %q: %w", modelUUID, err,
			)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	rval := map[user.Name][]string{}
	for _, userKey := range usersKeys {
		userName, err := user.NewName(userKey.UserName)
		if err != nil {
			return nil, errors.Errorf(
				"making user name from value %q when getting all users public keys for model %q: %w",
				userKey.UserName, modelUUID, err,
			)
		}

		rval[userName] = append(rval[userName], userKey.PublicKey)
	}

	return rval, nil
}

// GetPublicKeysForUser is responsible for returning all of the public keys for
// the user uuid in this model. If the user does not exist no error is returned.
func (s *State) GetPublicKeysForUser(
	ctx context.Context,
	modelUUID model.UUID,
	userUUID user.UUID,
) ([]coressh.PublicKey, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, err
	}

	modelUUIDVal := modelUUIDValue{UUID: modelUUID.String()}
	userUUIDVal := userUUIDValue{UUID: userUUID.String()}

	stmt, err := s.Prepare(`
SELECT &publicKey.*
FROM user_public_ssh_key AS upsk
JOIN model_authorized_keys AS m ON upsk.id = m.user_public_ssh_key_id
WHERE user_uuid = $userUUIDValue.user_uuid
AND model_uuid = $modelUUIDValue.model_uuid
`, userUUIDVal, publicKey{}, modelUUIDVal)
	if err != nil {
		return nil, errors.Errorf(
			"preparing select statement for getting public keys of user %q on model %q: %w",
			userUUID, modelUUID, err,
		)
	}

	publicKeys := []publicKey{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := s.checkUserExists(ctx, userUUID, tx)
		if err != nil {
			return errors.Errorf(
				"checking user %q exists when adding getting public keys on model %q: %w",
				userUUID, modelUUID, err,
			)
		}

		err = s.checkModelExists(ctx, modelUUID, tx)
		if err != nil {
			return errors.Errorf(
				"checking model %q exists when getting public keys for user %q: %w",
				modelUUID, userUUID, err,
			)
		}

		err = tx.Query(ctx, stmt, userUUIDVal, modelUUIDVal).GetAll(&publicKeys)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"getting public keys for user %q on model %q: %w",
				userUUID, modelUUID, err,
			)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	rval := make([]coressh.PublicKey, 0, len(publicKeys))
	for _, pk := range publicKeys {
		rval = append(rval, coressh.PublicKey{
			Fingerprint: pk.Fingerprint,
			Key:         pk.PublicKey,
		})
	}
	return rval, nil
}

// GetPublicKeysDataForUser is responsible for returning all of the public keys
// raw data for the user uuid on a given model.
// The following error can be expected:
// - accesserrors.UserNotFound if the user does not exist.
// - modelerrors.NotFound if the model does not exist.
func (s *State) GetPublicKeysDataForUser(
	ctx context.Context,
	modelUUID model.UUID,
	userUUID user.UUID,
) ([]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, err
	}

	userUUIDVal := userUUIDValue{userUUID.String()}
	modelUUIDVal := modelUUIDValue{modelUUID.String()}

	stmt, err := s.Prepare(`
SELECT &publicKeyData.public_key
FROM user_public_ssh_key AS upsk
JOIN model_authorized_keys AS m ON upsk.id = m.user_public_ssh_key_id
WHERE user_uuid = $userUUIDValue.user_uuid
AND model_uuid = $modelUUIDValue.model_uuid
`, userUUIDVal, modelUUIDVal, publicKeyData{})
	if err != nil {
		return nil, errors.Errorf(
			"preparing user %q public keys data statement on model %q: %w",
			userUUID, modelUUID, err,
		)
	}

	publicKeys := []publicKeyData{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := s.checkUserExists(ctx, userUUID, tx)
		if err != nil {
			return errors.Errorf(
				"getting public keys data for user %q on model %q: %w",
				userUUID, modelUUID, err,
			)
		}

		err = s.checkModelExists(ctx, modelUUID, tx)
		if err != nil {
			return errors.Errorf(
				"getting public keys data for user %q on model %q: %w",
				userUUID, modelUUID, err,
			)
		}

		err = tx.Query(ctx, stmt, userUUIDVal, modelUUIDVal).GetAll(&publicKeys)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"getting public keys data for user %q on model %q: %w",
				userUUID, modelUUID, err,
			)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	rval := make([]string, 0, len(publicKeys))
	for _, pk := range publicKeys {
		rval = append(rval, pk.PublicKey)
	}
	return rval, nil
}

// DeletePublicKeysForUser is responsible for removing the keys from the users
// list of public keys on the given model. keyIds represent one of the keys
// fingerprint, public key data or comment.
// The following errors can be expected:
// - [accesserrors.UserNotFound] - When the user does not exist.
// - [modelerrors.NotFound] - When the model does not exist.
func (s *State) DeletePublicKeysForUser(
	ctx context.Context,
	modelUUID model.UUID,
	userUUID user.UUID,
	keyIds []string,
) error {
	db, err := s.DB(ctx)
	if err != nil {
		return err
	}

	userUUIDVal := userUUIDValue{UUID: userUUID.String()}
	modelUUIDVal := modelUUIDValue{UUID: modelUUID.String()}

	input := make(sqlair.S, 0, len(keyIds))
	for _, keyId := range keyIds {
		input = append(input, keyId)
	}

	findKeysStmt, err := s.Prepare(`
SELECT &userPublicKeyId.*
FROM user_public_ssh_key
WHERE user_uuid = $userUUIDValue.user_uuid
AND (comment IN ($S[:])
  OR fingerprint IN ($S[:])
  OR public_key IN ($S[:]))
`, userUUIDVal, userPublicKeyId{}, input)
	if err != nil {
		return errors.Errorf(
			"preparing find keys statement when deleting public keys for user %q on model %q: %w",
			userUUID, modelUUID, err,
		)
	}

	deleteFromModelStmt, err := s.Prepare(`
DELETE FROM model_authorized_keys
WHERE user_public_ssh_key_id IN ($userPublicKeyIds[:])
AND model_uuid = $modelUUIDValue.model_uuid
`, modelUUIDVal, userPublicKeyIds{})
	if err != nil {
		return errors.Errorf(
			"preparing delete keys statement when deleting public keys for user %q on model %q: %w",
			userUUID, modelUUID, err,
		)
	}

	// deleteUnusedUserKeys is here to clean up any public keys for a user that
	// are not being referenced by a model.
	deleteUnusedUserKeys, err := s.Prepare(`
DELETE FROM user_public_ssh_key
WHERE user_uuid = $userUUIDValue.user_uuid
AND id IN (SELECT id
           FROM user_public_ssh_key AS upsk
           LEFT JOIN model_authorized_keys AS mak ON upsk.id = mak.user_public_ssh_key_id
           GROUP BY (id)
           HAVING count(user_public_ssh_key_id) == 0)
`, userUUIDVal)

	if err != nil {
		return errors.Errorf(
			"preparing delete unused user keys statement when deleting public keys for user %q on model %q: %w",
			userUUID, modelUUID, err,
		)
	}

	foundKeyIds := userPublicKeyIds{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := s.checkUserExists(ctx, userUUID, tx)
		if err != nil {
			return errors.Errorf(
				"deleting public keys for user %q on model %q: %w",
				userUUID, modelUUID, err,
			)
		}

		err = s.checkModelExists(ctx, modelUUID, tx)
		if err != nil {
			return errors.Errorf(
				"deleting public keys for user %q on model %q: %w",
				userUUID, modelUUID, err,
			)
		}

		err = tx.Query(ctx, findKeysStmt, userUUIDVal, input).GetAll(&foundKeyIds)
		if errors.Is(err, sqlair.ErrNoRows) {
			// Nothing was found so we can safely bail out and give this
			// transaction back to the pool early.
			return nil
		}
		if err != nil {
			return errors.Errorf(
				"finding public keys to delete for user %q on model %q: %w",
				userUUID, modelUUID, err,
			)
		}

		err = tx.Query(ctx, deleteFromModelStmt, modelUUIDVal, foundKeyIds).Run()
		if err != nil {
			return errors.Errorf(
				"deleting public keys for user %q on model %q: %w",
				userUUID, modelUUID, err,
			)
		}

		// At the very end of this transaction we will delete any public keys
		// for the user that are not being used in at least one model. We do
		// this to keep the table size down and also not have potential trusted
		// keys in the system that aren't used on a model.
		err = tx.Query(ctx, deleteUnusedUserKeys, userUUIDVal).Run()
		if err != nil {
			return errors.Errorf(
				"deleting unused public keys for user %q: %w",
				userUUID, err,
			)
		}
		return nil
	})

	if err != nil {
		return errors.Errorf(
			"deleting public keys for user %q: %w",
			userUUID, err,
		)
	}

	return nil
}
