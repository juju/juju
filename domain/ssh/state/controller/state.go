// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	coressh "github.com/juju/juju/core/ssh"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain"
	accesserrors "github.com/juju/juju/domain/access/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	domainssh "github.com/juju/juju/domain/ssh"
	"github.com/juju/juju/internal/errors"
)

// State represents controller-scoped SSH host key state.
type State struct {
	*domain.StateBase
}

// NewState returns a new controller-scoped SSH state.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{StateBase: domain.NewStateBase(factory)}
}

// GetSSHServerHostKey returns the controller jump host key.
func (st *State) GetSSHServerHostKey(ctx context.Context) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	id := controllerSSHHostKeyID{ID: domainssh.SSHServerHostKeyUUID}
	stmt, err := st.Prepare(`
SELECT &controllerSSHHostKey.ssh_key
FROM controller_ssh_host_key
WHERE id = $controllerSSHHostKeyID.id`, controllerSSHHostKey{}, controllerSSHHostKeyID{})
	if err != nil {
		return "", errors.Capture(err)
	}

	var key controllerSSHHostKey
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		key = controllerSSHHostKey{}

		err := tx.Query(ctx, stmt, id).Get(&key)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("controller SSH host key not found").Add(coreerrors.NotFound)
		}
		if err != nil {
			return errors.Errorf("querying controller SSH host key: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	return key.SSHKey, nil
}

// GetSSHServerHostPublicKey returns the marshalled public host key of the
// controller SSH jump server. The public key is derived once at bootstrap and
// stored alongside the private key, so this method never handles private key
// material.
func (st *State) GetSSHServerHostPublicKey(ctx context.Context) ([]byte, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	id := controllerSSHHostKeyID{ID: domainssh.SSHServerHostKeyUUID}
	stmt, err := st.Prepare(`
SELECT &controllerSSHHostKey.public_key
FROM controller_ssh_host_key
WHERE id = $controllerSSHHostKeyID.id`, controllerSSHHostKey{}, controllerSSHHostKeyID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var key controllerSSHHostKey
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		key = controllerSSHHostKey{}

		err := tx.Query(ctx, stmt, id).Get(&key)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("controller SSH host key not found").Add(coreerrors.NotFound)
		}
		if err != nil {
			return errors.Errorf("querying controller SSH host public key: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return key.PublicKey, nil
}

// GetPublicKeysForUser returns all public keys registered for a user. Keys are
// stored globally in the controller database and are not scoped to a model.
func (st *State) GetPublicKeysForUser(ctx context.Context, username user.Name) ([]coressh.PublicKey, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	arg := userName{Name: username.Name()}
	stmt, err := st.Prepare(`
SELECT &userPublicSSHKey.comment,
       &userPublicSSHKey.public_key
FROM user_public_ssh_key AS userPublicSSHKey
JOIN v_user_auth AS userAuth ON userPublicSSHKey.user_uuid = userAuth.uuid
WHERE userAuth.name = $userName.name
  AND userAuth.removed = FALSE
  AND userAuth.disabled = FALSE`, userPublicSSHKey{}, arg)
	if err != nil {
		return nil, errors.Capture(err)
	}

	rows := []userPublicSSHKey{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, arg).GetAll(&rows)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Errorf("getting public SSH keys for user %q: %w", username, err)
	}

	keys := make([]coressh.PublicKey, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, coressh.PublicKey{
			Comment: row.Comment,
			Key:     row.PublicKey,
		})
	}
	return keys, nil
}

// GetPublicKeysForUserInModel returns the public keys the named user is
// authorized to use in the supplied model.
func (st *State) GetPublicKeysForUserInModel(ctx context.Context, modelUUID, username string) ([]coressh.PublicKey, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	userArg := entityName{Name: username}
	modelArg := modelUUIDValue{UUID: modelUUID}
	userStmt, err := st.Prepare(`
SELECT uuid AS &entityUUID.uuid
FROM user
WHERE name = $entityName.name
  AND removed = FALSE`, entityUUID{}, userArg)
	if err != nil {
		return nil, errors.Capture(err)
	}
	modelStmt, err := st.Prepare(`
SELECT uuid AS &entityUUID.uuid
FROM model
WHERE uuid = $modelUUIDValue.uuid`, entityUUID{}, modelArg)
	if err != nil {
		return nil, errors.Capture(err)
	}
	keysStmt, err := st.Prepare(`
SELECT upsk.public_key AS &modelAuthorizedPublicKey.public_key
FROM user_public_ssh_key AS upsk
JOIN model_authorized_keys AS mak
  ON mak.user_public_ssh_key_id = upsk.id
WHERE upsk.user_uuid = $entityUUID.uuid
  AND mak.model_uuid = $modelUUIDValue.uuid`, modelAuthorizedPublicKey{}, entityUUID{}, modelArg)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var keys []modelAuthorizedPublicKey
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var user entityUUID
		if err := tx.Query(ctx, userStmt, userArg).Get(&user); errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("user %q: %w", username, accesserrors.UserNotFound)
		} else if err != nil {
			return errors.Errorf("getting user %q: %w", username, err)
		}
		var model entityUUID
		if err := tx.Query(ctx, modelStmt, modelArg).Get(&model); errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("model %q: %w", modelUUID, modelerrors.NotFound)
		} else if err != nil {
			return errors.Errorf("getting model %q: %w", modelUUID, err)
		}
		if err := tx.Query(ctx, keysStmt, user, modelArg).GetAll(&keys); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting public SSH keys for user %q: %w", username, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	result := make([]coressh.PublicKey, 0, len(keys))
	for _, key := range keys {
		result = append(result, coressh.PublicKey{Key: key.Key})
	}
	return result, nil
}

type controllerSSHHostKey struct {
	ID              string `db:"id"`
	AlgorithmTypeID int    `db:"algorithm_type_id"`
	SSHKey          string `db:"ssh_key"`
	PublicKey       []byte `db:"public_key"`
}

type controllerSSHHostKeyID struct {
	ID string `db:"id"`
}

type userName struct {
	Name string `db:"name"`
}

type userPublicSSHKey struct {
	Comment   string `db:"comment"`
	PublicKey string `db:"public_key"`
}

type entityName struct {
	Name string `db:"name"`
}

type entityUUID struct {
	UUID string `db:"uuid"`
}

type modelUUIDValue struct {
	UUID string `db:"uuid"`
}

type modelAuthorizedPublicKey struct {
	Key string `db:"public_key"`
}
