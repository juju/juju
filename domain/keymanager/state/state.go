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

// ensureUserPublicKey provides a closure that can be run from within a
// transaction to ensure that a public key exists for a user returning the
// unique id to represents that key within the database.
func (s *State) ensureUserPublicKey() (func(context.Context, userPublicKeyInsert, *sqlair.TX) (int64, error), error) {
	insertStatement, err := s.Prepare(`
INSERT INTO user_public_ssh_key (comment,
                                 fingerprint,
						         public_key,
                                 user_id,
                                 fingerprint_hash_algorithm_id)
SELECT $userPublicKeyInsert.comment,
       $userPublicKeyInsert.fingerprint,
       $userPublicKeyInsert.public_key,
       $userPublicKeyInsert.user_id,
       s.id	
FROM ssh_fingerprint_hash_algorithm s
WHERE s.algorithm = $userPublicKeyInsert.algorithm
`, userPublicKeyInsert{})

	if err != nil {
		return nil, errors.Errorf(
			"cannot prepare insert statement for ensuring user public key: %w",
			err,
		)
	}

	selectExistingIdStatement, err := s.Prepare(`
SELECT (id) AS (&userPublicKeyId.*)
FROM user_public_ssh_key
WHERE user_id = $userPublicKeyInsert.user_id
AND public_key = $userPublicKeyInsert.public_key
`, userPublicKeyId{}, userPublicKeyInsert{})

	if err != nil {
		return nil, errors.Errorf(
			"cannot prepare select existing id statement for ensuring user public key: %w",
			err,
		)
	}

	return func(
		ctx context.Context,
		key userPublicKeyInsert,
		tx *sqlair.TX,
	) (int64, error) {
		outcome := sqlair.Outcome{}
		err := tx.Query(ctx, insertStatement, key).Get(&outcome)
		if err != nil && !jujudb.IsErrConstraintUnique(err) {
			return 0, errors.Errorf(
				"cannot insert public key for user %q: %w", key.UserId, err,
			)
		}

		var keyId int64
		if jujudb.IsErrConstraintUnique(err) {
			row := userPublicKeyId{}
			err = tx.Query(ctx, selectExistingIdStatement, key).Get(&row)
			keyId = row.Id
		} else {
			var lastInsertId int64
			lastInsertId, err = outcome.Result().LastInsertId()
			keyId = lastInsertId
		}

		if err != nil {
			return 0, errors.Errorf(
				"cannot get unique id for user %q public key: %w",
				key.UserId, err,
			)
		}
		return keyId, nil
	}, nil
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
	modelId model.UUID,
	userId user.UUID,
	publicKeys []keymanager.PublicKey,
) error {
	db, err := s.DB()
	if err != nil {
		return errors.Errorf(
			"cannot get database for adding public keys to user %q on model %q: %w",
			userId,
			modelId,
			err,
		)
	}

	userIdVal := userIdValue{UserId: userId.String()}

	userRemovedStatement, err := s.Prepare(`
SELECT (uuid) AS (&userIdValue.user_id)
FROM v_user_auth
WHERE uuid = $userIdValue.user_id
AND removed = false
`, userIdVal)
	if err != nil {
		return errors.Errorf(
			"cannot prepare user removed statement when preparing to add public keys for user %q to model %q: %w",
			userId, modelId, err,
		)
	}

	ensurePublicKeyFunc, err := s.ensureUserPublicKey()
	if err != nil {
		return errors.Errorf(
			"cannot get ensure user public key closure when adding public keys for user %q to model %q: %w",
			userId, modelId, err,
		)
	}

	insertModelStatement, err := s.Prepare(`
	INSERT INTO model_authorized_keys (*)
	VALUES ($modelAuthorizedKey.*)
	`, modelAuthorizedKey{})
	if err != nil {
		return errors.Errorf(
			"cannot prepare insert statement for adding user %q public keys to model %q: %w",
			userId, modelId, err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, userRemovedStatement, userIdVal).Get(&userIdVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"cannot add keys for user %q to model %q because the user does not exist",
				userId, modelId,
			).Add(accesserrors.UserNotFound)
		}
		if err != nil {
			return errors.Errorf(
				"cannot check if user %q exists when add public keys to model %q: %w",
				userId, modelId, err,
			)
		}

		keyIds := []int64{}
		for i, publicKey := range publicKeys {
			row := userPublicKeyInsert{
				Comment:                  publicKey.Comment,
				FingerprintHashAlgorithm: publicKey.FingerprintHash.String(),
				Fingerprint:              publicKey.Fingerprint,
				PublicKey:                publicKey.Key,
				UserId:                   userId.String(),
			}

			keyId, err := ensurePublicKeyFunc(ctx, row, tx)
			if err != nil {
				return errors.Errorf(
					"cannot ensure user %q public key %d on model %q: %w",
					userId, i, modelId, err,
				)
			}

			keyIds = append(keyIds, keyId)
		}

		for i, keyId := range keyIds {
			row := modelAuthorizedKey{
				UserPublicSSHKeyId: keyId,
				ModelId:            modelId.String(),
			}
			err := tx.Query(ctx, insertModelStatement, row).Run()
			if jujudb.IsErrConstraintForeignKey(err) {
				return errors.Errorf(
					"cannot add public key %d for user %q to model %q, model does not exist",
					i, userId, modelId,
				).Add(modelerrors.NotFound)
			} else if jujudb.IsErrConstraintUnique(err) {
				return errors.Errorf(
					"cannot add key %d for user %q to model %q, key already exists",
					i, userId, modelId,
				).Add(keyerrors.PublicKeyAlreadyExists)
			} else if err != nil {
				return errors.Errorf(
					"cannot add key %d for user %q to model %q: %w",
					i, userId, modelId, err,
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
	modelId model.UUID,
	userId user.UUID,
	publicKeys []keymanager.PublicKey,
) error {
	db, err := s.DB()
	if err != nil {
		return errors.Errorf(
			"cannot get database for ensuring public keys on user %q in model %q: %w",
			userId,
			modelId,
			err,
		)
	}

	userIdVal := userIdValue{UserId: userId.String()}

	userRemovedStatement, err := s.Prepare(`
SELECT (uuid) AS (&userIdValue.user_id)
FROM v_user_auth
WHERE uuid = $userIdValue.user_id
AND removed = false
`, userIdVal)
	if err != nil {
		return errors.Errorf(
			"cannot prepare user removed statement when preparing to ensure public keys for user %q on model %q: %w",
			userId, modelId, err,
		)
	}

	ensurePublicKeyFunc, err := s.ensureUserPublicKey()
	if err != nil {
		return errors.Errorf(
			"cannot get ensure user public key closure when adding public keys for user %q to model %q: %w",
			userId, modelId, err,
		)
	}

	insertModelStatement, err := s.Prepare(`
INSERT INTO model_authorized_keys (*)
VALUES ($modelAuthorizedKey.*)
ON CONFLICT DO NOTHING
`, modelAuthorizedKey{})

	if err != nil {
		return errors.Errorf(
			"cannot prepare insert statement for ensuring user %q public keys on model %q: %w",
			userId, modelId, err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, userRemovedStatement, userIdVal).Get(&userIdVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"cannot ensure keys for user %q on model %q because the user does not exist",
				userId, modelId,
			).Add(accesserrors.UserNotFound)
		}
		if err != nil {
			return errors.Errorf(
				"cannot check if user %q exists when ensuring public keys on model %q: %w",
				userId, modelId, err,
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
				UserId:                   userId.String(),
			}

			keyId, err := ensurePublicKeyFunc(ctx, row, tx)
			if err != nil {
				return errors.Errorf(
					"cannot ensure user %q public key %d on model %q: %w",
					userId, i, modelId, err,
				)
			}
			keyIds = append(keyIds, keyId)
		}

		for i, keyId := range keyIds {
			row := modelAuthorizedKey{
				UserPublicSSHKeyId: keyId,
				ModelId:            modelId.String(),
			}

			err := tx.Query(ctx, insertModelStatement, row).Run()
			if jujudb.IsErrConstraintForeignKey(err) {
				return errors.Errorf(
					"cannot ensure public key %d for user %q on model %q: model does not exist",
					i, userId, modelId,
				).Add(modelerrors.NotFound)
			} else if err != nil {
				return errors.Errorf(
					"cannot ensure key %d for user %q on model %q: %w",
					i, userId, modelId, err,
				)
			}
		}

		return nil
	})

	return err
}

// GetPublicKeysForUser is responsible for returning all of the public
// keys for the user id on a model. If the user does not exist no error is
// returned.
// The following errors can be expected:
// - [accesserrors.UserNotFound] if the user does not exist.
// - [modelerrors.NotFound] if the model does not exist.
func (s *State) GetPublicKeysForUser(
	ctx context.Context,
	modelId model.UUID,
	userId user.UUID,
) ([]coressh.PublicKey, error) {
	db, err := s.DB()
	if err != nil {
		return nil, err
	}

	modelIdVal := modelIdValue{ModelId: modelId.String()}
	userIdVal := userIdValue{UserId: userId.String()}

	userRemovedStatement, err := s.Prepare(`
SELECT (uuid) AS (&userIdValue.user_id)
FROM v_user_auth
WHERE uuid = $userIdValue.user_id
AND removed = false
`, userIdVal)
	if err != nil {
		return nil, errors.Errorf(
			"cannot prepare user removed statement when getting public keys for user %q on model %q: %w",
			userId, modelId, err,
		)
	}

	modelExistsStatement, err := s.Prepare(`
SELECT (uuid) AS (&modelIdValue.model_id)
FROM v_model
WHERE uuid = $modelIdValue.model_id
`, modelIdVal)
	if err != nil {
		return nil, errors.Errorf(
			"cannot prepare model exists statement when getting public keys for user %q on model %q: %w",
			userId, modelId, err,
		)
	}

	stmt, err := s.Prepare(`
SELECT (upsk.public_key, upsk.fingerprint) AS (&publicKey.*)
FROM user_public_ssh_key AS upsk
INNER JOIN model_authorized_keys AS m ON upsk.user_public_ssh_key_id = m.user_public_ssh_key_id
WHERE user_id = $userIdVal.user_id
AND model_id = $modelIdValue.model_id
`, userIdVal, publicKey{}, modelIdVal)
	if err != nil {
		return nil, errors.Errorf(
			"preparing select statement for getting public keys of user %q on model %q: %w",
			userId, modelId, err,
		)
	}

	publicKeys := []publicKey{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, userRemovedStatement, userIdVal).Get(&userIdVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"cannot get public keys for user %q on model %q because the user does not exist",
				userId, modelId,
			).Add(accesserrors.UserNotFound)
		}
		if err != nil {
			return errors.Errorf(
				"cannot check that user %q exists when getting public keys on model %q: %w",
				userId, modelId, err,
			)
		}

		err = tx.Query(ctx, modelExistsStatement, modelIdVal).Get(&modelIdVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"cannot get public keys for user %q on model %q because the model does not exists",
				userId, modelId,
			).Add(modelerrors.NotFound)
		}
		if err != nil {
			return errors.Errorf(
				"cannot check that model %q exists when getting public keys for user %q: %w",
				modelId, userId, err,
			)
		}

		err = tx.Query(ctx, stmt, userIdVal, modelIdVal).GetAll(&publicKeys)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"cannot get public keys for user %q on model %q: %w",
				userId, modelId, err,
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
// raw data for the user id on a given model.
// The following error can be expected:
// - accesserrors.UserNotFound if the user does not exist.
// - modelerrors.NotFound if the model does not exist.
func (s *State) GetPublicKeysDataForUser(
	ctx context.Context,
	modelId model.UUID,
	userId user.UUID,
) ([]string, error) {
	db, err := s.DB()
	if err != nil {
		return nil, err
	}

	userIdVal := userIdValue{userId.String()}
	modelIdVal := modelIdValue{modelId.String()}

	userRemovedStatement, err := s.Prepare(`
SELECT (uuid) AS (&userIdValue.user_id)
FROM v_user_auth
WHERE uuid = $userIdValue.user_id
AND removed = false
`, userIdVal)
	if err != nil {
		return nil, errors.Errorf(
			"cannot prepare user removed statement when getting public keys data for user %q on model %q: %w",
			userId, modelId, err,
		)
	}

	modelExistsStatement, err := s.Prepare(`
SELECT (uuid) AS (&modelIdValue.model_id)
FROM v_model
WHERE uuid = $modelIdValue.model_id
`, modelIdVal)
	if err != nil {
		return nil, errors.Errorf(
			"cannot prepare model exists statement when getting public keys data for user %q on model %q: %w",
			userId, modelId, err,
		)
	}

	stmt, err := s.Prepare(`
SELECT (public_key) AS (&publicKeyData.*)
FROM user_public_ssh_key AS upsk
INNER JOIN model_authorized_keys AS m ON upsk.id = m.user_public_ssh_key_id
WHERE user_id = $userIdValue.user_id
AND model_id = $modelIdValue.model_id
`, userIdVal, modelIdVal, publicKeyData{})
	if err != nil {
		return nil, errors.Errorf(
			"cannot prepare user %q public keys data statement on model %q: %w",
			userId, modelId, err,
		)
	}

	publicKeys := []publicKeyData{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, userRemovedStatement, userIdVal).Get(&userIdVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"cannot get public keys data for user %q on model %q because the user does not exist",
				userId, modelId,
			).Add(accesserrors.UserNotFound)
		}
		if err != nil {
			return errors.Errorf(
				"cannot check that user %q exists when getting public keys data on model %q: %w",
				userId, modelId, err,
			)
		}

		err = tx.Query(ctx, modelExistsStatement, modelIdVal).Get(&modelIdVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"cannot get public keys data for user %q on model %q because the model does not exist",
				userId, modelId,
			).Add(modelerrors.NotFound)
		}
		if err != nil {
			return errors.Errorf(
				"cannot check that model %q exists when getting public keys data for user %q: %w",
				modelId, userId, err,
			)
		}

		err = tx.Query(ctx, stmt, userIdVal, modelIdVal).GetAll(&publicKeys)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"cannot get public keys data for user %q on model %q: %w",
				userId, modelId, err,
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
	modelId model.UUID,
	userId user.UUID,
	keyIds []string,
) error {
	db, err := s.DB()
	if err != nil {
		return err
	}

	userIdVal := userIdValue{UserId: userId.String()}
	modelIdVal := modelIdValue{ModelId: modelId.String()}

	userRemovedStatement, err := s.Prepare(`
SELECT (uuid) AS (&userIdValue.user_id)
FROM v_user_auth
WHERE uuid = $userIdValue.user_id
AND removed = false
`, userIdVal)

	if err != nil {
		return errors.Errorf(
			"cannot prepare user removed statement when deleting public keys for user %q on model %q: %w",
			userId, modelId, err,
		)
	}

	modelExistsStatement, err := s.Prepare(`
SELECT (uuid) AS (&modelIdValue.model_id)
FROM v_model
WHERE uuid = $modelIdValue.model_id
`, modelIdVal)
	if err != nil {
		return errors.Errorf(
			"cannot prepare model exists statement when deleting public keys for user %q on model %q: %w",
			userId, modelId, err,
		)
	}

	input := make(sqlair.S, 0, len(keyIds))
	for _, keyId := range keyIds {
		input = append(input, keyId)
	}

	findKeysStatement, err := s.Prepare(`
SELECT (id) AS (&userPublicKeyId.*)
FROM user_public_ssh_key
WHERE user_id = $userIdValue.user_id
AND (comment IN ($S[:])
  OR fingerprint IN ($S[:])
  OR public_key IN ($S[:]))
`, userIdVal, userPublicKeyId{}, input)
	if err != nil {
		return errors.Errorf(
			"cannot prepare find keys statement when deleting public keys for user %q on model %q: %w",
			userId, modelId, err,
		)
	}

	deleteFromModelStatement, err := s.Prepare(`
DELETE FROM model_authorized_keys
WHERE user_public_ssh_key_id IN ($userPublicKeyIds[:])
AND model_id = $modelIdValue.model_id
`, modelIdVal, userPublicKeyIds{})
	if err != nil {
		return errors.Errorf(
			"cannot prepare delete keys statement when deleting public keys for user %q on model %q: %w",
			userId, modelId, err,
		)
	}

	// deleteUnusedUserKeys is here to clean up any public keys for a user that
	// are not being referenced by a model.
	deleteUnusedUserKeys, err := s.Prepare(`
DELETE FROM user_public_ssh_key
WHERE user_id = $userIdValue.user_id
AND id IN (SELECT id
           FROM user_public_ssh_key AS upsk
           LEFT JOIN model_authorized_keys AS mak ON upsk.id = mak.user_public_ssh_key_id
           GROUP BY (id)
		   HAVING count(user_public_ssh_key_id) == 0)
`, userIdVal)

	if err != nil {
		return errors.Errorf(
			"cannot prepare delete unused user keys statement when deleting public keys for user %q on model %q: %w",
			userId, modelId, err,
		)
	}

	foundKeyIds := userPublicKeyIds{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, userRemovedStatement, userIdVal).Get(&userIdVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"cannot delete public keys for user %q on model %q, user does not exist",
				userId, modelId,
			).Add(accesserrors.UserNotFound)
		}
		if err != nil {
			return errors.Errorf(
				"cannot check that user %q exists when deleting public keys on model %q: %w",
				userId, modelId, err,
			)
		}

		err = tx.Query(ctx, modelExistsStatement, modelIdVal).Get(&modelIdVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"cannot delete public keys for user %q on model %q because the model does not exist",
				userId, modelId,
			).Add(modelerrors.NotFound)
		}
		if err != nil {
			return errors.Errorf(
				"cannot check that model %q exists when deleting public keys for user %q: %w",
				modelId, userId, err,
			)
		}

		err = tx.Query(ctx, findKeysStatement, userIdVal, input).GetAll(&foundKeyIds)
		if errors.Is(err, sqlair.ErrNoRows) {
			// Nothing was found so we can safely bail out and give this
			// transaction back to the pool early.
			return nil
		}
		if err != nil {
			return errors.Errorf(
				"cannot find public keys to delete for user %q on model %q: %w",
				userId, modelId, err,
			)
		}

		err = tx.Query(ctx, deleteFromModelStatement, modelIdVal, foundKeyIds).Run()
		if err != nil {
			return errors.Errorf(
				"cannot delete public keys for user %q on model %q: %w",
				userId, modelId, err,
			)
		}

		// At the very end of this transaction we will delete any public keys
		// for the user that are not being used in at least one model. We do
		// this to keep the table size down and also not have potential trusted
		// keys in the system that aren't used on a model.
		err = tx.Query(ctx, deleteUnusedUserKeys, userIdVal).Run()
		if err != nil {
			return errors.Errorf(
				"cannot delete unused public keys for user %q: %w",
				userId, err,
			)
		}
		return nil
	})

	if err != nil {
		return errors.Errorf(
			"cannot delete public keys for user %q: %w",
			userId, err,
		)
	}

	return nil
}
