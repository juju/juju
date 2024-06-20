// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/keymanager"
	keyerrors "github.com/juju/juju/domain/keymanager/errors"
	jujudb "github.com/juju/juju/internal/database"
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

// AddPublicKeyForUser is responsible for adding one or more ssh public keys for
// a user. If no keys are provided to the operation then nothing will happen and
// no error produced. The following errors can be expected.
// [keyerrors.PublicKeyAlreadExists] - When one of the public keys being added
// for a user already exists.
func (s *State) AddPublicKeysForUser(
	ctx context.Context,
	userId user.UUID,
	publicKeys []keymanager.PublicKey,
) error {
	if len(publicKeys) == 0 {
		return nil
	}

	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

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
		return fmt.Errorf(
			"preparing insert statement for adding user %q public keys: %w",
			userId,
			domain.CoerceError(err),
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// We can't perform a bulk insert here because of the foreign key lookup
		// for the algorithm. It could be done with a temp table but this is not
		// considered a hot path.
		for _, publicKey := range publicKeys {
			row := userPublicKeyInsert{
				Comment:                  publicKey.Comment,
				FingerprintHashAlgorithm: publicKey.FingerprintHash.String(),
				Fingerprint:              publicKey.Fingerprint,
				PublicKey:                publicKey.Key,
				UserId:                   userId.String(),
			}

			if err := tx.Query(ctx, insertStatement, row).Run(); err != nil {
				return err
			}
		}
		return nil
	})

	if jujudb.IsErrConstraintUnique(err) {
		return fmt.Errorf(
			"cannot add %d keys for user %q, one or more keys already exist%w",
			len(publicKeys), userId, errors.Hide(keyerrors.PublicKeyAlreadyExists),
		)
	} else if err != nil {
		return fmt.Errorf(
			"cannot add %d keys for user %q: %w",
			len(publicKeys), userId, domain.CoerceError(err),
		)
	}

	return err
}

// AddPublicKeyForUserIfNotFound will attempt to add the given set of public
// keys for the user. If the user already contains the public key it will be
// skipped. and no [keyserrors.PublicKeyAlreadyExists] error will be returned.
func (s *State) AddPublicKeyForUserIfNotFound(
	ctx context.Context,
	userId user.UUID,
	publicKeys []keymanager.PublicKey,
) error {
	if len(publicKeys) == 0 {
		return nil
	}

	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

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
		return fmt.Errorf(
			"preparing insert statement for adding user %q public keys: %w",
			userId,
			domain.CoerceError(err),
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// We can't perform a bulk insert here because of the foreign key lookup
		// for the algorithm. It could be done with a temp table but this is not
		// considered a hot path.
		for _, publicKey := range publicKeys {
			row := userPublicKeyInsert{
				Comment:                  publicKey.Comment,
				FingerprintHashAlgorithm: publicKey.FingerprintHash.String(),
				Fingerprint:              publicKey.Fingerprint,
				PublicKey:                publicKey.Key,
				UserId:                   userId.String(),
			}

			err := tx.Query(ctx, insertStatement, row).Run()
			// We want to ignore duplicate key addition as it is not considered
			// an error for this operation.
			if jujudb.IsErrConstraintUnique(err) {
				continue
			} else if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf(
			"cannot add %d keys for user %q: %w",
			len(publicKeys), userId, domain.CoerceError(err),
		)
	}
	return err
}

// GetPublicKeysForUser is responsible for returning all of the public keys for
// the user id in this model. If the user does not exist no error is returned.
func (s *State) GetPublicKeysForUser(
	ctx context.Context,
	id user.UUID,
) ([]string, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	stmt, err := s.Prepare(`
SELECT public_key AS &publicKey.*
FROM user_public_ssh_key
WHERE user_id = $userId.user_id
`, userId{}, publicKey{})

	if err != nil {
		return nil, fmt.Errorf(
			"preparing select statement for getting public keys of user %q: %w",
			id, domain.CoerceError(err),
		)
	}

	userId := userId{id.String()}
	publicKeys := []publicKey{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, userId).GetAll(&publicKeys)
	})

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf(
			"cannot get public keys for user %q: %w", id, err,
		)
	}

	rval := make([]string, 0, len(publicKeys))
	for _, pk := range publicKeys {
		rval = append(rval, pk.PublicKey)
	}
	return rval, nil
}

// DeletePublicKeysForUser is responsible for removing the keys from the users
// list of public keys where the keyIds represent one of the keys fingerprint,
// public key data or comment. If no user exists for the given id a nill error
// will be returned.
func (s *State) DeletePublicKeysForUser(
	ctx context.Context,
	id user.UUID,
	keyIds []string,
) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	stmt, err := s.Prepare(`
DELETE FROM user_public_ssh_key
WHERE user_id = $userId.user_id
AND (comment IN ($S[:])
  OR fingerprint IN ($S[:])
  OR public_key IN ($S[:]))
`, sqlair.S{}, userId{})

	if err != nil {
		return fmt.Errorf(
			"preparing delete statement for removing public keys for user %q: %w",
			id, domain.CoerceError(err),
		)
	}

	input := make(sqlair.S, 0, len(keyIds))
	for _, keyId := range keyIds {
		input = append(input, keyId)
	}

	userId := userId{id.String()}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, userId, input).Run()
	})

	if err != nil {
		return fmt.Errorf(
			"cannot delete public keys for user %q: %w",
			id, err,
		)
	}

	return nil
}
