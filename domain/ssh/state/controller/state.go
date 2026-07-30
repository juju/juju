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
SELECT &userPublicSSHKey.public_key
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
		keys = append(keys, coressh.PublicKey{Key: row.PublicKey})
	}
	return keys, nil
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
	PublicKey string `db:"public_key"`
}
