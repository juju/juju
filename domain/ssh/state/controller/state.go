// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/errors"
)

const sshServerHostKeyID = "sshServerHostKey"

// State represents controller-scoped SSH host key state.
type State struct {
	*domain.StateBase
}

// NewState returns a new controller-scoped SSH state.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{StateBase: domain.NewStateBase(factory)}
}

// GetSSHServerHostKey returns the controller jump host key.
// The boolean indicates whether the key row exists.
func (st *State) GetSSHServerHostKey(ctx context.Context) (string, bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", false, errors.Capture(err)
	}

	id := controllerSSHHostKeyID{ID: sshServerHostKeyID}
	stmt, err := st.Prepare(`
SELECT &controllerSSHHostKey.ssh_key
FROM controller_ssh_host_key
WHERE id = $controllerSSHHostKeyID.id`, controllerSSHHostKey{}, controllerSSHHostKeyID{})
	if err != nil {
		return "", false, errors.Capture(err)
	}

	var key controllerSSHHostKey
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		key = controllerSSHHostKey{}

		err := tx.Query(ctx, stmt, id).Get(&key)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errors.Errorf("querying controller SSH host key: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", false, errors.Capture(err)
	}
	if key.SSHKey == "" {
		return "", false, nil
	}
	return key.SSHKey, true, nil
}

// SetSSHServerHostKey persists the controller jump host key.
func (st *State) SetSSHServerHostKey(ctx context.Context, sshKey string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	record := controllerSSHHostKey{ID: sshServerHostKeyID, SSHKey: sshKey}
	stmt, err := st.Prepare(`
INSERT INTO controller_ssh_host_key (id, ssh_key)
VALUES ($controllerSSHHostKey.*)
ON CONFLICT(id) DO UPDATE SET ssh_key = excluded.ssh_key`, controllerSSHHostKey{})
	if err != nil {
		return errors.Capture(err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, record).Run(); err != nil {
			return errors.Errorf("persisting controller SSH host key: %w", err)
		}
		return nil
	}))
}

type controllerSSHHostKey struct {
	ID     string `db:"id"`
	SSHKey string `db:"ssh_key"`
}

type controllerSSHHostKeyID struct {
	ID string `db:"id"`
}
